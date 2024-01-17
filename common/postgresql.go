package common

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/sqlitebrowser/dbhub.io/common/config"
	"github.com/sqlitebrowser/dbhub.io/common/database"

	"github.com/aquilax/truncate"
	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/smtp2go-oss/smtp2go-go"
)

// DB4SDefaultList returns a list of 1) users with public databases, 2) along with the logged in users' most recently
// modified database (including their private one(s))
func DB4SDefaultList(loggedInUser string) (UserInfoSlice, error) {
	// Retrieve the list of all users with public databases
	dbQuery := `
		WITH public_dbs AS (
			SELECT db_id, last_modified
			FROM sqlite_databases
			WHERE public = true
			AND is_deleted = false
			ORDER BY last_modified DESC
		), public_users AS (
			SELECT DISTINCT ON (db.user_id) db.user_id, db.last_modified
			FROM public_dbs as pub, sqlite_databases AS db
			WHERE db.db_id = pub.db_id
			ORDER BY db.user_id, db.last_modified DESC
		)
		SELECT user_name, last_modified
		FROM public_users AS pu, users
		WHERE users.user_id = pu.user_id
			AND users.user_name != $1
		ORDER BY last_modified DESC`
	rows, err := database.DB.Query(context.Background(), dbQuery, loggedInUser)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return nil, err
	}
	defer rows.Close()
	unsorted := make(map[string]UserInfo)
	for rows.Next() {
		var oneRow UserInfo
		err = rows.Scan(&oneRow.Username, &oneRow.LastModified)
		if err != nil {
			log.Printf("Error list of users with public databases: %v", err)
			return nil, err
		}
		unsorted[oneRow.Username] = oneRow
	}

	// Sort the list by last_modified order, from most recent to oldest
	publicList := make(UserInfoSlice, 0, len(unsorted))
	for _, j := range unsorted {
		publicList = append(publicList, j)
	}
	sort.Sort(publicList)

	// Retrieve the last modified timestamp for the most recent database of the logged in user (if they have any)
	dbQuery = `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), user_db_list AS (
			SELECT DISTINCT ON (db_id) db_id, last_modified
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
			AND is_deleted = false
		), most_recent_user_db AS (
			SELECT udb.last_modified
			FROM user_db_list AS udb
			ORDER BY udb.last_modified DESC
			LIMIT 1
		)
		SELECT last_modified
		FROM most_recent_user_db`
	userRow := UserInfo{Username: loggedInUser}
	rows, err = database.DB.Query(context.Background(), dbQuery, loggedInUser)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return nil, err
	}
	defer rows.Close()
	userHasDB := false
	for rows.Next() {
		userHasDB = true
		err = rows.Scan(&userRow.LastModified)
		if err != nil {
			log.Printf("Error retrieving database list for user: %v", err)
			return nil, err
		}
	}

	// If the user doesn't have any databases, just return the list of users with public databases
	if !userHasDB {
		return publicList, nil
	}

	// The user does have at least one database, so include them at the top of the list
	completeList := make(UserInfoSlice, 0, len(unsorted)+1)
	completeList = append(completeList, userRow)
	completeList = append(completeList, publicList...)
	return completeList, nil
}

// FlushViewCount periodically flushes the database view count from Memcache to PostgreSQL
func FlushViewCount() {
	type dbEntry struct {
		Owner string
		Name  string
	}

	// Log the start of the loop
	log.Printf("%s: periodic view count flushing loop started.  %d second refresh.", config.Conf.Live.Nodename, config.Conf.Memcache.ViewCountFlushDelay)

	// Start the endless flush loop
	var rows pgx.Rows
	var err error
	for {
		// Retrieve the list of all public databases
		dbQuery := `
			SELECT users.user_name, db.db_name
			FROM sqlite_databases AS db, users
			WHERE db.public = true
				AND db.is_deleted = false
				AND db.user_id = users.user_id`
		rows, err = database.DB.Query(context.Background(), dbQuery)
		if err != nil {
			log.Printf("Database query failed: %v", err)
			continue
		}
		var dbList []dbEntry
		for rows.Next() {
			var oneRow dbEntry
			err = rows.Scan(&oneRow.Owner, &oneRow.Name)
			if err != nil {
				log.Printf("Error retrieving database list for view count flush thread: %v", err)
				rows.Close()
				continue
			}
			dbList = append(dbList, oneRow)
		}
		rows.Close()

		// For each public database, retrieve the latest view count from memcache and save it back to PostgreSQL
		for _, db := range dbList {
			dbOwner := db.Owner
			dbName := db.Name

			// Retrieve the view count from Memcached
			newValue, err := GetViewCount(dbOwner, dbName)
			if err != nil {
				log.Printf("Error when getting memcached view count for %s/%s: %s", dbOwner, dbName,
					err.Error())
				continue
			}

			// We use a value of -1 to indicate there wasn't an entry in memcache for the database
			if newValue != -1 {
				// Update the view count in PostgreSQL
				dbQuery = `
					UPDATE sqlite_databases
					SET page_views = $3
					WHERE user_id = (
							SELECT user_id
							FROM users
							WHERE lower(user_name) = lower($1)
						)
						AND db_name = $2`
				commandTag, err := database.DB.Exec(context.Background(), dbQuery, dbOwner, dbName, newValue)
				if err != nil {
					log.Printf("Flushing view count for '%s/%s' failed: %v", dbOwner, dbName, err)
					continue
				}
				if numRows := commandTag.RowsAffected(); numRows != 1 {
					log.Printf("Wrong number of rows affected (%v) when flushing view count for '%s/%s'",
						numRows, dbOwner, dbName)
					continue
				}
			}
		}

		// Wait before running the loop again
		time.Sleep(config.Conf.Memcache.ViewCountFlushDelay * time.Second)
	}

	// If somehow the endless loop finishes, then record that in the server logs
	log.Printf("%s: WARN: periodic view count flushing loop stopped.", config.Conf.Live.Nodename)
}

// LiveGenerateMinioNames generates Minio bucket and object names for a live database
func LiveGenerateMinioNames(userName string) (bucketName, objectName string, err error) {
	// If the user already has a Minio bucket name assigned, then we use it
	z, err := database.User(userName)
	if err != nil {
		return
	}
	if z.MinioBucket != "" {
		bucketName = z.MinioBucket
	} else {
		// They don't have a bucket name assigned yet, so we generate one and assign it to them
		bucketName = fmt.Sprintf("live-%s", RandomString(10))

		// Add this bucket name to the user's details in the PG backend
		dbQuery := `
			UPDATE users
			SET live_minio_bucket_name = $2
			WHERE user_name = $1
			AND live_minio_bucket_name is null` // This should ensure we never overwrite an existing bucket name for the user
		var commandTag pgconn.CommandTag
		commandTag, err = database.DB.Exec(context.Background(), dbQuery, userName, bucketName)
		if err != nil {
			log.Printf("Updating Minio bucket name for user '%s' failed: %v", userName, err)
			return
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf("Wrong number of rows (%d) affected while updating the Minio bucket name for user '%s'",
				numRows, userName)
		}
	}

	// We only generate the name here, we *do not* try to update anything in the database with it.  This is because
	// when this function is called, the SQLite database may not yet have a record in the PG backend
	objectName = RandomString(6)
	return
}

// LiveGetMinioNames retrieves the Minio bucket and object names for a live database
func LiveGetMinioNames(loggedInUser, dbOwner, dbName string) (bucketName, objectName string, err error) {
	// Retrieve user details
	usr, err := database.User(dbOwner)
	if err != nil {
		return
	}

	// Retrieve database details
	var db database.SQLiteDBinfo
	err = database.DBDetails(&db, loggedInUser, dbOwner, dbName, "")
	if err != nil {
		return
	}

	// If either the user bucket name or the minio object name is empty, then the database is likely stored using
	// the initial naming scheme
	if usr.MinioBucket == "" || db.MinioId == "" {
		bucketName = fmt.Sprintf("live-%s", dbOwner)
		objectName = dbName
	} else {
		// It's using the new naming scheme
		bucketName = usr.MinioBucket
		objectName = db.MinioId
	}
	return
}

// LiveUserDBs returns the list of live databases owned by the user
func LiveUserDBs(dbOwner string, public database.AccessType) (list []database.DBInfo, err error) {
	dbQuery := `
		SELECT db_name, date_created, last_modified, public, live_db, live_node,
			db.watchers, db.stars, discussions, contributors,
			coalesce(one_line_description, ''), coalesce(source_url, ''),
			download_count, page_views
		FROM sqlite_databases AS db, users
		WHERE users.user_id = db.user_id
			AND lower(users.user_name) = lower($1)
			AND is_deleted = false
			AND live_db = true`

	switch public {
	case database.DB_PUBLIC:
		// Only public databases
		dbQuery += ` AND public = true`
	case database.DB_PRIVATE:
		// Only private databases
		dbQuery += ` AND public = false`
	case database.DB_BOTH:
		// Both public and private, so no need to add a query clause
	default:
		// This clause shouldn't ever be reached
		return nil, fmt.Errorf("Incorrect 'public' value '%v' passed to LiveUserDBs() function.", public)
	}
	dbQuery += " ORDER BY date_created DESC"

	rows, err := database.DB.Query(context.Background(), dbQuery, dbOwner)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var oneRow database.DBInfo
		var liveNode string
		err = rows.Scan(&oneRow.Database, &oneRow.DateCreated, &oneRow.RepoModified, &oneRow.Public, &oneRow.IsLive, &liveNode,
			&oneRow.Watchers, &oneRow.Stars, &oneRow.Discussions, &oneRow.Contributors,
			&oneRow.OneLineDesc, &oneRow.SourceURL, &oneRow.Downloads, &oneRow.Views)
		if err != nil {
			log.Printf("Error when retrieving list of live databases for user '%s': %v", dbOwner, err)
			return nil, err
		}

		// Ask the job queue backend for the database file size
		oneRow.Size, err = LiveSize(liveNode, dbOwner, dbOwner, oneRow.Database)
		if err != nil {
			log.Printf("Error when retrieving size of live databases for user '%s': %v", dbOwner, err)
			return nil, err
		}

		list = append(list, oneRow)
	}
	return
}

// MinioLocation returns the Minio bucket and ID for a given  dbOwner & dbName are from
// owner/database URL fragment, loggedInUser is the name for the currently logged in user, for access permission
// check.  Use an empty string ("") as the loggedInUser parameter if the true value isn't set or known.
// If the requested database doesn't exist, or the loggedInUser doesn't have access to it, then an error will be
// returned
func MinioLocation(dbOwner, dbName, commitID, loggedInUser string) (minioBucket, minioID string, lastModified time.Time, err error) {
	// Check permissions
	allowed, err := database.CheckDBPermissions(loggedInUser, dbOwner, dbName, false)
	if err != nil {
		return
	}
	if !allowed {
		err = errors.New("Database not found")
		return
	}

	// If no commit was provided, we grab the default one
	if commitID == "" {
		commitID, err = database.DefaultCommit(dbOwner, dbName)
		if err != nil {
			return // Bucket and ID are still the initial default empty string
		}
	}

	// Retrieve the sha256 and last modified date for the requested commits database file
	var dbQuery string
	dbQuery = `
		SELECT commit_list->$3::text->'tree'->'entries'->0->>'sha256' AS sha256,
			commit_list->$3::text->'tree'->'entries'->0->>'last_modified' AS last_modified
		FROM sqlite_databases AS db
		WHERE db.user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db.db_name = $2
			AND db.is_deleted = false`
	var sha, mod pgtype.Text
	err = database.DB.QueryRow(context.Background(), dbQuery, dbOwner, dbName, commitID).Scan(&sha, &mod)
	if err != nil {
		log.Printf("Error retrieving MinioID for '%s/%s' version '%v' by logged in user '%v': %v",
			dbOwner, dbName, commitID, loggedInUser, err)
		return // Bucket and ID are still the initial default empty string
	}

	if !sha.Valid || sha.String == "" {
		// The requested database doesn't exist, or the logged in user doesn't have access to it
		err = fmt.Errorf("The requested database wasn't found")
		return // Bucket and ID are still the initial default empty string
	}

	lastModified, err = time.Parse(time.RFC3339, mod.String)
	if err != nil {
		return // Bucket and ID are still the initial default empty string
	}

	shaStr := sha.String
	minioBucket = shaStr[:MinioFolderChars]
	minioID = shaStr[MinioFolderChars:]
	return
}

// SaveDBSettings saves updated database settings to PostgreSQL
func SaveDBSettings(userName, dbName, oneLineDesc, fullDesc, defaultTable string, public bool, sourceURL, defaultBranch string) error {
	// Check for values which should be NULL
	var nullable1LineDesc, nullableFullDesc, nullableSourceURL pgtype.Text
	if oneLineDesc == "" {
		nullable1LineDesc.Valid = false
	} else {
		nullable1LineDesc.String = oneLineDesc
		nullable1LineDesc.Valid = true
	}
	if fullDesc == "" {
		nullableFullDesc.Valid = false
	} else {
		nullableFullDesc.String = fullDesc
		nullableFullDesc.Valid = true
	}
	if sourceURL == "" {
		nullableSourceURL.Valid = false
	} else {
		nullableSourceURL.String = sourceURL
		nullableSourceURL.Valid = true
	}

	// Save the database settings
	SQLQuery := `
		UPDATE sqlite_databases
		SET one_line_description = $3, full_description = $4, default_table = $5, public = $6, source_url = $7,
			default_branch = $8
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db_name = $2`
	commandTag, err := database.DB.Exec(context.Background(), SQLQuery, userName, dbName, nullable1LineDesc, nullableFullDesc, defaultTable,
		public, nullableSourceURL, defaultBranch)
	if err != nil {
		log.Printf("Updating description for database '%s/%s' failed: %v", SanitiseLogString(userName),
			SanitiseLogString(dbName), err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		errMsg := fmt.Sprintf("Wrong number of rows affected (%d) when updating description for '%s/%s'",
			numRows, userName, dbName)
		log.Printf(SanitiseLogString(errMsg))
		return errors.New(errMsg)
	}

	// Invalidate the old memcached entry for the database
	err = InvalidateCacheEntry(userName, userName, dbName, "") // Empty string indicates "for all versions"
	if err != nil {
		// Something went wrong when invalidating memcached entries for the database
		log.Printf("Error when invalidating memcache entries: %s", err.Error())
		return err
	}
	return nil
}

// SendEmails sends status update emails to people watching databases
func SendEmails() {
	// If the SMTP2Go API key hasn't been configured, there's no use in trying to send emails
	if config.Conf.Event.Smtp2GoKey == "" && os.Getenv("SMTP2GO_API_KEY") == "" {
		return
	}

	for {
		// Retrieve unsent emails from the email_queue
		type eml struct {
			Address string
			Body    string
			ID      int64
			Subject string
		}
		var emailList []eml
		dbQuery := `
				SELECT email_id, mail_to, subject, body
				FROM email_queue
				WHERE sent = false`
		rows, err := database.DB.Query(context.Background(), dbQuery)
		if err != nil {
			log.Printf("Database query failed: %v", err.Error())
			return // Abort, as we don't want to continuously resend the same emails
		}
		for rows.Next() {
			var oneRow eml
			err = rows.Scan(&oneRow.ID, &oneRow.Address, &oneRow.Subject, &oneRow.Body)
			if err != nil {
				log.Printf("Error retrieving queued emails: %v", err.Error())
				rows.Close()
				return // Abort, as we don't want to continuously resend the same emails
			}
			emailList = append(emailList, oneRow)
		}
		rows.Close()

		// Send emails
		for _, j := range emailList {
			e := smtp2go.Email{
				From:     "updates@dbhub.io",
				To:       []string{j.Address},
				Subject:  j.Subject,
				TextBody: j.Body,
				HtmlBody: j.Body,
			}
			_, err = smtp2go.Send(&e)
			if err != nil {
				log.Println(err)
			}

			log.Printf("Email with subject '%v' sent to '%v'",
				truncate.Truncate(j.Subject, 35, "...", truncate.PositionEnd), j.Address)

			// We only attempt delivery via smtp2go once (retries are handled on their end), so mark message as sent
			dbQuery := `
				UPDATE email_queue
				SET sent = true, sent_timestamp = now()
				WHERE email_id = $1`
			commandTag, err := database.DB.Exec(context.Background(), dbQuery, j.ID)
			if err != nil {
				log.Printf("Changing email status to sent failed for email '%v': '%v'", j.ID, err.Error())
				return // Abort, as we don't want to continuously resend the same emails
			}
			if numRows := commandTag.RowsAffected(); numRows != 1 {
				log.Printf("Wrong # of rows (%v) affected when changing email status to sent for email '%v'",
					numRows, j.ID)
			}
		}

		// Pause before running the loop again
		time.Sleep(config.Conf.Event.EmailQueueProcessingDelay * time.Second)
	}
}

// StatusUpdatesLoop periodically generates status updates (alert emails TBD) from the event queue
func StatusUpdatesLoop() {
	// Ensure a warning message is displayed on the console if the status update loop exits
	defer func() {
		log.Printf("%s: WARN: Status update loop exited", config.Conf.Live.Nodename)
	}()

	// Log the start of the loop
	log.Printf("%s: status update processing loop started.  %d second refresh.", config.Conf.Live.Nodename, config.Conf.Event.Delay)

	// Start the endless status update processing loop
	var err error
	type evEntry struct {
		dbID      int64
		details   database.EventDetails
		eType     database.EventType
		eventID   int64
		timeStamp time.Time
	}
	for {
		// Wait at the start of the loop (simpler code then adding a delay before each continue statement below)
		time.Sleep(config.Conf.Event.Delay * time.Second)

		// Begin a transaction
		var tx pgx.Tx
		tx, err = database.DB.Begin(context.Background())
		if err != nil {
			log.Printf("%s: couldn't begin database transaction for status update processing loop: %s",
				config.Conf.Live.Nodename, err.Error())
			continue
		}

		// Retrieve the list of outstanding events
		// NOTE - We gather the db_id here instead of dbOwner/dbName as it should be faster for PG to deal
		//        with when generating the watcher list
		dbQuery := `
			SELECT event_id, event_timestamp, db_id, event_type, event_data
			FROM events
			ORDER BY event_id ASC`
		rows, err := tx.Query(context.Background(), dbQuery)
		if err != nil {
			log.Printf("Generating status update event list failed: %v", err)
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) {
				log.Println(pgErr.Message)
				log.Println(pgErr.Code)
			}
			tx.Rollback(context.Background())
			continue
		}
		evList := make(map[int64]evEntry)
		for rows.Next() {
			var ev evEntry
			err = rows.Scan(&ev.eventID, &ev.timeStamp, &ev.dbID, &ev.eType, &ev.details)
			if err != nil {
				log.Printf("Error retrieving event list for status updates thread: %v", err)
				rows.Close()
				tx.Rollback(context.Background())
				continue
			}
			evList[ev.eventID] = ev
		}
		rows.Close()

		// For each event, add a status update to the status_updates list for each watcher it's for
		for id, ev := range evList {
			// Retrieve the list of watchers for the database the event occurred on
			dbQuery = `
				SELECT user_id
				FROM watchers
				WHERE db_id = $1`
			rows, err = tx.Query(context.Background(), dbQuery, ev.dbID)
			if err != nil {
				log.Printf("Error retrieving user list for status updates thread: %v", err)
				tx.Rollback(context.Background())
				continue
			}
			var users []int64
			for rows.Next() {
				var user int64
				err = rows.Scan(&user)
				if err != nil {
					log.Printf("Error retrieving user list for status updates thread: %v", err)
					rows.Close()
					tx.Rollback(context.Background())
					continue
				}
				users = append(users, user)
			}

			// For each watcher, add the new status update to their existing list
			// TODO: It might be better to store this list in Memcached instead of hitting the database like this
			for _, u := range users {
				// Retrieve the current status updates list for the user
				var eml pgtype.Text
				var userEvents map[string][]database.StatusUpdateEntry
				var userName string
				dbQuery = `
					SELECT user_name, email, status_updates
					FROM users
					WHERE user_id = $1`
				err = tx.QueryRow(context.Background(), dbQuery, u).Scan(&userName, &eml, &userEvents)
				if err != nil {
					if !errors.Is(err, pgx.ErrNoRows) {
						// A real error occurred
						log.Printf("Database query failed: %s", err)
						tx.Rollback(context.Background())
					}
					continue
				}
				if len(userEvents) == 0 {
					userEvents = make(map[string][]database.StatusUpdateEntry)
				}

				// If the user generated this event themselves, skip them
				if userName == ev.details.UserName {
					log.Printf("User '%v' generated this event (id: %v), so not adding it to their event list",
						userName, ev.eventID)
					continue
				}

				// * Add the new event to the users status updates list *

				// Group the status updates by database, and coalesce multiple updates for the same discussion or MR
				// into a single entry (keeping the most recent one of each)
				dbName := fmt.Sprintf("%s/%s", ev.details.Owner, ev.details.DBName)
				var a database.StatusUpdateEntry
				lst, ok := userEvents[dbName]
				if ev.details.Type == database.EVENT_NEW_DISCUSSION || ev.details.Type == database.EVENT_NEW_MERGE_REQUEST || ev.details.Type == database.EVENT_NEW_COMMENT {
					if ok {
						// Check if an entry already exists for the discussion/MR/comment
						for i, j := range lst {
							if j.DiscID == ev.details.DiscID {
								// Yes, there's already an existing entry for the discussion/MR/comment so delete the old entry
								lst = append(lst[:i], lst[i+1:]...) // Delete the old element
							}
						}
					}
				}

				// Add the new entry
				a.DiscID = ev.details.DiscID
				a.Title = ev.details.Title
				a.URL = ev.details.URL
				lst = append(lst, a)
				userEvents[dbName] = lst

				// Save the updated list for the user back to PG
				dbQuery = `
					UPDATE users
					SET status_updates = $2
					WHERE user_id = $1`
				commandTag, err := tx.Exec(context.Background(), dbQuery, u, userEvents)
				if err != nil {
					log.Printf("Adding status update for database ID '%d' to user id '%d' failed: %v", ev.dbID,
						u, err)
					tx.Rollback(context.Background())
					continue
				}
				if numRows := commandTag.RowsAffected(); numRows != 1 {
					log.Printf("Wrong number of rows affected (%d) when adding status update for database ID "+
						"'%d' to user id '%d'", numRows, ev.dbID, u)
					tx.Rollback(context.Background())
					continue
				}

				// Count the number of status updates for the user, to be displayed in the webUI header row
				var numUpdates int
				for _, i := range userEvents {
					numUpdates += len(i)
				}

				// Add an entry to memcached for the user, indicating they have outstanding status updates available
				err = SetUserStatusUpdates(userName, numUpdates)
				if err != nil {
					log.Printf("Error when updating user status updates # in memcached: %v", err)
					continue
				}

				// TODO: Add a email for the status notification to the outgoing email queue
				var msg, subj string
				switch ev.details.Type {
				case database.EVENT_NEW_DISCUSSION:
					msg = fmt.Sprintf("A new discussion has been created for %s/%s.\n\nVisit https://%s%s "+
						"for the details", ev.details.Owner, ev.details.DBName, config.Conf.Web.ServerName,
						ev.details.URL)
					subj = fmt.Sprintf("DBHub.io: New discussion created on %s/%s", ev.details.Owner,
						ev.details.DBName)
				case database.EVENT_NEW_MERGE_REQUEST:
					msg = fmt.Sprintf("A new merge request has been created for %s/%s.\n\nVisit https://%s%s "+
						"for the details", ev.details.Owner, ev.details.DBName, config.Conf.Web.ServerName,
						ev.details.URL)
					subj = fmt.Sprintf("DBHub.io: New merge request created on %s/%s", ev.details.Owner,
						ev.details.DBName)
				case database.EVENT_NEW_COMMENT:
					msg = fmt.Sprintf("A new comment has been created for %s/%s.\n\nVisit https://%s%s for "+
						"the details", ev.details.Owner, ev.details.DBName, config.Conf.Web.ServerName,
						ev.details.URL)
					subj = fmt.Sprintf("DBHub.io: New comment on %s/%s", ev.details.Owner,
						ev.details.DBName)
				default:
					log.Printf("Unknown message type when creating email message")
				}
				if eml.Valid {
					// If the email address is of the form username@this_server (which indicates a non-functional email address), then skip it
					serverName := strings.Split(config.Conf.Web.ServerName, ":")
					if strings.HasSuffix(eml.String, serverName[0]) {
						log.Printf("Skipping email '%v' to destination '%v', as it ends in '%v'",
							truncate.Truncate(subj, 35, "...", truncate.PositionEnd), eml.String, serverName[0])
						continue
					}

					// Add the email to the queue
					dbQuery = `
						INSERT INTO email_queue (mail_to, subject, body)
						VALUES ($1, $2, $3)`
					commandTag, err = tx.Exec(context.Background(), dbQuery, eml.String, subj, msg)
					if err != nil {
						log.Printf("Adding status update to email queue for user '%v' failed: %v", u, err)
						tx.Rollback(context.Background())
						continue
					}
					if numRows := commandTag.RowsAffected(); numRows != 1 {
						log.Printf("Wrong number of rows affected (%d) when adding status update to email"+
							"queue for user '%v'", numRows, u)
						tx.Rollback(context.Background())
						continue
					}
				}
			}

			// Remove the processed event from PG
			dbQuery = `
				DELETE FROM events
				WHERE event_id = $1`
			commandTag, err := tx.Exec(context.Background(), dbQuery, id)
			if err != nil {
				log.Printf("Removing event ID '%d' failed: %v", id, err)
				continue
			}
			if numRows := commandTag.RowsAffected(); numRows != 1 {
				log.Printf("Wrong number of rows affected (%d) when removing event ID '%d'", numRows, id)
				continue
			}
		}

		// Commit the transaction
		err = tx.Commit(context.Background())
		if err != nil {
			log.Printf("Could not commit transaction when processing status updates: %v", err.Error())
			continue
		}
	}
	return
}

// StoreDatabase stores database details in PostgreSQL, and the database data itself in Minio
func StoreDatabase(dbOwner, dbName string, branches map[string]database.BranchEntry, c database.CommitEntry, pub bool,
	buf *os.File, sha string, dbSize int64, oneLineDesc, fullDesc string, createDefBranch bool, branchName,
	sourceURL string) error {
	// Store the database file
	err := StoreDatabaseFile(buf, sha, dbSize)
	if err != nil {
		return err
	}

	// Check for values which should be NULL
	var nullable1LineDesc, nullableFullDesc pgtype.Text
	if oneLineDesc == "" {
		nullable1LineDesc.Valid = false
	} else {
		nullable1LineDesc.String = oneLineDesc
		nullable1LineDesc.Valid = true
	}
	if fullDesc == "" {
		nullableFullDesc.Valid = false
	} else {
		nullableFullDesc.String = fullDesc
		nullableFullDesc.Valid = true
	}

	// Store the database metadata
	cMap := map[string]database.CommitEntry{c.ID: c}
	var commandTag pgconn.CommandTag
	dbQuery := `
		WITH root AS (
			SELECT nextval('sqlite_databases_db_id_seq') AS val
		)
		INSERT INTO sqlite_databases (user_id, db_id, db_name, public, one_line_description, full_description,
			branch_heads, root_database, commit_list`
	if sourceURL != "" {
		dbQuery += `, source_url`
	}
	dbQuery +=
		`)
		SELECT (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)), (SELECT val FROM root), $2, $3, $4, $5, $7, (SELECT val FROM root), $6`
	if sourceURL != "" {
		dbQuery += `, $8`
	}
	dbQuery += `
		ON CONFLICT (user_id, db_name)
			DO UPDATE
			SET commit_list = sqlite_databases.commit_list || $6,
				branch_heads = sqlite_databases.branch_heads || $7,
				last_modified = now()`
	if sourceURL != "" {
		dbQuery += `,
			source_url = $8`
		commandTag, err = database.DB.Exec(context.Background(), dbQuery, dbOwner, dbName, pub, nullable1LineDesc, nullableFullDesc,
			cMap, branches, sourceURL)
	} else {
		commandTag, err = database.DB.Exec(context.Background(), dbQuery, dbOwner, dbName, pub, nullable1LineDesc, nullableFullDesc,
			cMap, branches)
	}
	if err != nil {
		log.Printf("Storing database '%s/%s' failed: %v", SanitiseLogString(dbOwner),
			SanitiseLogString(dbName), err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected while storing database '%s/%s'", numRows, SanitiseLogString(dbOwner),
			SanitiseLogString(dbName))
	}

	if createDefBranch {
		err = database.StoreDefaultBranchName(dbOwner, dbName, branchName)
		if err != nil {
			log.Printf("Storing default branch '%s' name for '%s/%s' failed: %v", SanitiseLogString(branchName),
				SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
			return err
		}
	}
	return nil
}
