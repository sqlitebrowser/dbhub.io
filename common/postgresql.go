package common

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/hectane/hectane/email"
	"github.com/hectane/hectane/queue"
	"github.com/jackc/pgx"
	gfm "github.com/sqlitebrowser/github_flavored_markdown"
	"golang.org/x/crypto/bcrypt"
)

var (
	// PostgreSQL connection pool handle
	pdb *pgx.ConnPool
)

// Add the default user to the system, used so the referential integrity of licence user_id 0 works.
func AddDefaultUser() error {
	// Add the new user to the database
	dbQuery := `
		INSERT INTO users (auth0_id, user_name, email, password_hash, client_cert, display_name)
		VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := pdb.Exec(dbQuery, RandomString(16), "default", "default@dbhub.io", RandomString(16), "",
		"Default system user")
	if err != nil {
		// For now, don't bother logging a failure here.  This *might* need changing later on
		return err
	}

	// Log addition of the default user
	log.Println("Added default user to the database")

	return nil
}

// Add a user to the system.
func AddUser(auth0ID string, userName string, password string, email string, displayName string, avatarURL string) error {
	// Hash the user's password
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Failed to hash user password. User: '%v', error: %v.\n", userName, err)
		return err
	}

	// Generate a new HTTPS client certificate for the user
	var cert []byte
	if Conf.Sign.Enabled {
		cert, err = GenerateClientCert(userName)
		if err != nil {
			log.Printf("Error when generating client certificate for '%s': %v\n", userName, err)
			return err
		}
	}

	// If the display name or avatar URL are an empty string, we insert a NULL instead
	var av, dn pgx.NullString
	if displayName != "" {
		dn.String = displayName
		dn.Valid = true
	}
	if avatarURL != "" {
		av.String = avatarURL
		av.Valid = true
	}

	// Add the new user to the database
	insertQuery := `
		INSERT INTO users (auth0_id, user_name, email, password_hash, client_cert, display_name, avatar_url)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	commandTag, err := pdb.Exec(insertQuery, auth0ID, userName, email, hash, cert, dn, av)
	if err != nil {
		log.Printf("Adding user to database failed: %v\n", err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows affected when creating user: %v, username: %v\n", numRows, userName)
	}

	// Log the user registration
	log.Printf("User registered: '%s' Email: '%s'\n", userName, email)

	return nil
}

// Check if a database exists
// If an error occurred, the true/false value should be ignored, as only the error value is valid.
func CheckDBExists(loggedInUser string, dbOwner string, dbFolder string, dbName string) (bool, error) {
	dbQuery := `
		SELECT count(db_id)
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND folder = $2
			AND db_name = $3
			AND is_deleted = false`
	// If the request is from someone who's not logged in, or is for another users database, ensure we only consider
	// public databases
	if strings.ToLower(loggedInUser) != strings.ToLower(dbOwner) || loggedInUser == "" {
		dbQuery += `
			AND public = true`
	}
	var DBCount int
	err := pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&DBCount)
	if err != nil {
		log.Printf("Checking if a database exists failed: %v\n", err)
		return true, err
	}
	if DBCount == 0 {
		// Database isn't in our system
		return false, nil
	}

	// Database exists
	return true, nil
}

// Check if a given database ID is available, and return it's folder/name so the caller can determine if it has been
// renamed.  If an error occurs, the true/false value should be ignored, as only the error value is valid.
func CheckDBID(loggedInUser string, dbOwner string, dbID int64) (avail bool, dbFolder string, dbName string, err error) {
	dbQuery := `
		SELECT folder, db_name
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db_id = $2
			AND is_deleted = false`
	// If the request is from someone who's not logged in, or is for another users database, ensure we only consider
	// public databases
	if strings.ToLower(loggedInUser) != strings.ToLower(dbOwner) || loggedInUser == "" {
		dbQuery += `
			AND public = true`
	}
	err = pdb.QueryRow(dbQuery, dbOwner, dbID).Scan(&dbFolder, &dbName)
	if err != nil {
		if err == pgx.ErrNoRows {
			avail = false
			return
		} else {
			log.Printf("Checking if a database exists failed: %v\n", err)
			return
		}
	}

	// Database exists
	avail = true
	return
}

// Check if a database has been starred by a given user.  The boolean return value is only valid when err is nil.
func CheckDBStarred(loggedInUser string, dbOwner string, dbFolder string, dbName string) (bool, error) {
	dbQuery := `
		SELECT count(db_id)
		FROM database_stars
		WHERE database_stars.user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($4)
			)
			AND database_stars.db_id = (
					SELECT db_id
					FROM sqlite_databases
					WHERE user_id = (
							SELECT user_id
							FROM users
							WHERE lower(user_name) = lower($1)
						)
						AND folder = $2
						AND db_name = $3
						AND is_deleted = false)`
	var starCount int
	err := pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName, loggedInUser).Scan(&starCount)
	if err != nil {
		log.Printf("Error looking up star count for database. User: '%s' DB: '%s/%s'. Error: %v\n",
			loggedInUser, dbOwner, dbName, err)
		return true, err
	}
	if starCount == 0 {
		// Database hasn't been starred by the user
		return false, nil
	}

	// Database HAS been starred by the user
	return true, nil
}

// Check if a database is being watched by a given user.  The boolean return value is only valid when err is nil.
func CheckDBWatched(loggedInUser string, dbOwner string, dbFolder string, dbName string) (bool, error) {
	dbQuery := `
		SELECT count(db_id)
		FROM watchers
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($4)
			)
			AND db_id = (
					SELECT db_id
					FROM sqlite_databases
					WHERE user_id = (
							SELECT user_id
							FROM users
							WHERE lower(user_name) = lower($1)
						)
						AND folder = $2
						AND db_name = $3
						AND is_deleted = false)`
	var watchCount int
	err := pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName, loggedInUser).Scan(&watchCount)
	if err != nil {
		log.Printf("Error looking up watchers count for database. User: '%s' DB: '%s%s%s'. Error: %v\n",
			loggedInUser, dbOwner, dbFolder, dbName, err)
		return true, err
	}
	if watchCount == 0 {
		// Database isn't being watched by the user
		return false, nil
	}

	// Database IS being watched by the user
	return true, nil
}

// Check if an email address already exists in our system. Returns true if the email is already in the system, false
// if not.  If an error occurred, the true/false value should be ignored, as only the error value is valid.
func CheckEmailExists(email string) (bool, error) {
	// Check if the email address is already in our system
	dbQuery := `
		SELECT count(user_name)
		FROM users
		WHERE email = $1`
	var emailCount int
	err := pdb.QueryRow(dbQuery, email).Scan(&emailCount)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return true, err
	}
	if emailCount == 0 {
		// Email address isn't yet in our system
		return false, nil
	}

	// Email address IS already in our system
	return true, nil

}

// Checks if a licence exists in our system.
func CheckLicenceExists(userName string, licenceName string) (exists bool, err error) {
	dbQuery := `
		SELECT count(*)
		FROM database_licences
		WHERE friendly_name = $2
			AND (user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = 'default'
			)
			OR user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			))`
	var count int
	err = pdb.QueryRow(dbQuery, userName, licenceName).Scan(&count)
	if err != nil {
		log.Printf("Error checking if licence '%s' exists for user '%s' in database: %v\n", licenceName,
			userName, err)
		return false, err
	}
	if count == 0 {
		// The requested licence wasn't found
		return false, nil
	}
	return true, nil
}

// Check if a username already exists in our system.  Returns true if the username is already taken, false if not.
// If an error occurred, the true/false value should be ignored, and only the error return code used.
func CheckUserExists(userName string) (bool, error) {
	dbQuery := `
		SELECT count(user_id)
		FROM users
		WHERE lower(user_name) = lower($1)`
	var userCount int
	err := pdb.QueryRow(dbQuery, userName).Scan(&userCount)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return true, err
	}
	if userCount == 0 {
		// Username isn't in system
		return false, nil
	}
	// Username IS in system
	return true, nil
}

// Returns the certificate for a given user.
func ClientCert(userName string) ([]byte, error) {
	var cert []byte
	err := pdb.QueryRow(`
		SELECT client_cert
		FROM users
		WHERE lower(user_name) = lower($1)`, userName).Scan(&cert)
	if err != nil {
		log.Printf("Retrieving client cert for '%s' from database failed: %v\n", userName, err)
		return nil, err
	}

	return cert, nil
}

// Creates a connection pool to the PostgreSQL server.
func ConnectPostgreSQL() (err error) {
	pgPoolConfig := pgx.ConnPoolConfig{*pgConfig, Conf.Pg.NumConnections, nil, 2 * time.Second}
	pdb, err = pgx.NewConnPool(pgPoolConfig)
	if err != nil {
		return errors.New(fmt.Sprintf("Couldn't connect to PostgreSQL server: %v\n", err))
	}

	// Log successful connection
	log.Printf("Connected to PostgreSQL server: %v:%v\n", Conf.Pg.Server, uint16(Conf.Pg.Port))

	return nil
}

// Returns the ID number for a given user's database.
func databaseID(dbOwner string, dbFolder string, dbName string) (dbID int, err error) {
	// Retrieve the database id
	dbQuery := `
		SELECT db_id
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1))
			AND folder = $2
			AND db_name = $3
			AND is_deleted = false`
	err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&dbID)
	if err != nil {
		log.Printf("Error looking up database id. Owner: '%s', Database: '%s'. Error: %v\n", dbOwner, dbName,
			err)
	}
	return
}

// Return a list of 1) users with public databases, 2) along with the logged in users' most recently modified database
// (including their private one(s)).
func DB4SDefaultList(loggedInUser string) (map[string]UserInfo, error) {
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
		ORDER BY last_modified DESC`
	rows, err := pdb.Query(dbQuery)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return nil, err
	}
	defer rows.Close()
	list := make(map[string]UserInfo)
	for rows.Next() {
		var oneRow UserInfo
		err = rows.Scan(&oneRow.Username, &oneRow.LastModified)
		if err != nil {
			log.Printf("Error list of users with public databases: %v\n", err)
			return nil, err
		}
		list[oneRow.Username] = oneRow
	}

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
	rows, err = pdb.Query(dbQuery, loggedInUser)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		oneRow := UserInfo{Username: loggedInUser}
		err = rows.Scan(&oneRow.LastModified)
		if err != nil {
			log.Printf("Error retrieving database list for user: %v\n", err)
			return nil, err
		}
		list[oneRow.Username] = oneRow
	}

	return list, nil
}

// Retrieve the details for a specific database
func DBDetails(DB *SQLiteDBinfo, loggedInUser string, dbOwner string, dbFolder string, dbName string, commitID string) error {
	// If no commit ID was supplied, we retrieve the latest commit one from the default branch
	var err error
	if commitID == "" {
		commitID, err = DefaultCommit(dbOwner, dbFolder, dbName)
		if err != nil {
			return err
		}
	}

	// Retrieve the database details
	dbQuery := `
		SELECT db.date_created, db.last_modified, db.watchers, db.stars, db.discussions, db.merge_requests,
			$4::text AS commit_id, db.commit_list->$4::text->'tree'->'entries'->0 AS db_entry,
			db.branches, db.release_count, db.contributors, db.one_line_description, db.full_description,
			db.default_table, db.public, db.source_url, db.tags, db.default_branch
		FROM sqlite_databases AS db
		WHERE db.user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db.folder = $2
			AND db.db_name = $3
			AND db.is_deleted = false`

	// If the request is for another users database, ensure we only look up public ones
	if strings.ToLower(loggedInUser) != strings.ToLower(dbOwner) {
		dbQuery += `
			AND db.public = true`
	}

	// Generate a predictable cache key for this functions' metadata.  Probably not sharable with other functions
	// cached metadata
	mdataCacheKey := MetadataCacheKey("meta", loggedInUser, dbOwner, dbFolder, dbName, commitID)

	// Use a cached version of the query response if it exists
	ok, err := GetCachedData(mdataCacheKey, &DB)
	if err != nil {
		log.Printf("Error retrieving data from cache: %v\n", err)
	}
	if ok {
		// Data was in cache, so we use that
		return nil
	}

	// Retrieve the requested database details
	var defTable, fullDesc, oneLineDesc, sourceURL pgx.NullString
	err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName, commitID).Scan(&DB.Info.DateCreated,
		&DB.Info.RepoModified, &DB.Info.Watchers, &DB.Info.Stars, &DB.Info.Discussions, &DB.Info.MRs,
		&DB.Info.CommitID,
		&DB.Info.DBEntry,
		&DB.Info.Branches, &DB.Info.Releases, &DB.Info.Contributors, &oneLineDesc, &fullDesc, &defTable,
		&DB.Info.Public, &sourceURL, &DB.Info.Tags, &DB.Info.DefaultBranch)

	if err != nil {
		log.Printf("Error when retrieving database details: %v\n", err.Error())
		return errors.New("The requested database doesn't exist")
	}
	if !oneLineDesc.Valid {
		DB.Info.OneLineDesc = "No description"
	} else {
		DB.Info.OneLineDesc = oneLineDesc.String
	}
	if !fullDesc.Valid {
		DB.Info.FullDesc = "No full description"
	} else {
		DB.Info.FullDesc = fullDesc.String
	}
	if !defTable.Valid {
		DB.Info.DefaultTable = ""
	} else {
		DB.Info.DefaultTable = defTable.String
	}
	if !sourceURL.Valid {
		DB.Info.SourceURL = ""
	} else {
		DB.Info.SourceURL = sourceURL.String
	}

	// Fill out the fields we already have data for
	DB.Info.Database = dbName
	DB.Info.Folder = dbFolder

	// Retrieve latest fork count
	// TODO: This can probably be folded into the above SQL query as a sub-select, as a minor optimisation
	dbQuery = `
		SELECT forks
		FROM sqlite_databases
		WHERE db_id = (
			SELECT root_database
			FROM sqlite_databases
			WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1))
			AND folder = $2
			AND db_name = $3)`
	err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&DB.Info.Forks)
	if err != nil {
		log.Printf("Error retrieving fork count for '%s%s%s': %v\n", dbOwner, dbFolder, dbName, err)
		return err
	}

	// Cache the database details
	err = CacheData(mdataCacheKey, DB, Conf.Memcache.DefaultCacheTime)
	if err != nil {
		log.Printf("Error when caching page data: %v\n", err)
	}

	return nil
}

// Returns the star count for a given database.
func DBStars(dbOwner string, dbFolder string, dbName string) (starCount int, err error) {
	// Retrieve the updated star count
	dbQuery := `
		SELECT stars
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND folder = $2
			AND db_name = $3
			AND is_deleted = false`
	err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&starCount)
	if err != nil {
		log.Printf("Error looking up star count for database '%s/%s'. Error: %v\n", dbOwner, dbName, err)
		return -1, err
	}
	return starCount, nil
}

// Returns the watchers count for a given database.
func DBWatchers(dbOwner string, dbFolder string, dbName string) (watcherCount int, err error) {
	// Retrieve the updated watchers count
	dbQuery := `
		SELECT watchers
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND folder = $2
			AND db_name = $3
			AND is_deleted = false`
	err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&watcherCount)
	if err != nil {
		log.Printf("Error looking up watcher count for database '%s%s%s'. Error: %v\n", dbOwner, dbFolder,
			dbName, err)
		return -1, err
	}
	return watcherCount, nil
}

// Retrieve the default commit ID for a specific database
func DefaultCommit(dbOwner string, dbFolder string, dbName string) (string, error) {
	// If no commit ID was supplied, we retrieve the latest commit ID from the default branch
	dbQuery := `
		SELECT branch_heads->default_branch->'commit' AS commit_id
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
			)
			AND folder = $2
			AND db_name = $3
			AND is_deleted = false`
	var commitID string
	err := pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&commitID)
	if err != nil {
		log.Printf("Error when retrieving head commit ID of default branch: %v\n", err.Error())
		return "", errors.New("Internal error when looking up database details")
	}
	return commitID, nil
}

// Delete a specific comment from a discussion
func DeleteComment(dbOwner string, dbFolder string, dbName string, discID int, comID int) error {
	// Begin a transaction
	tx, err := pdb.Begin()
	if err != nil {
		return err
	}
	// Set up an automatic transaction roll back if the function exits without committing
	defer tx.Rollback()

	// Delete the requested discussion comment
	dbQuery := `
		WITH d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db
			WHERE db.user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND folder = $2
				AND db_name = $3
		), int AS (
			SELECT internal_id AS int_id
			FROM discussions
			WHERE db_id = (SELECT db_id FROM d)
			AND disc_id = $4
		)
		DELETE FROM discussion_comments
		WHERE db_id = (SELECT db_id FROM d)
			AND disc_id = (SELECT int_id FROM int)
			AND com_id = $5`
	commandTag, err := tx.Exec(dbQuery, dbOwner, dbFolder, dbName, discID, comID)
	if err != nil {
		log.Printf("Deleting comment '%d' from '%s%s%s', discussion '%d' failed: %v\n", comID, dbOwner,
			dbFolder, dbName, discID, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%v) affected when deleting comment '%d' from database '%s%s%s, discussion '%d''\n",
			numRows, comID, dbOwner, dbFolder, dbName, discID)
	}

	// Update the comment count and last modified date for the discussion
	dbQuery = `
		WITH d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db
			WHERE db.user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND folder = $2
				AND db_name = $3
		), int AS (
			SELECT internal_id AS int_id
			FROM discussions
			WHERE db_id = (SELECT db_id FROM d)
			AND disc_id = $4
		), new AS (
			SELECT count(*)
			FROM discussion_comments
			WHERE db_id = (SELECT db_id FROM d)
				AND disc_id = (SELECT int_id FROM int)
				AND entry_type = 'txt'
		)
		UPDATE discussions
		SET comment_count = (SELECT count FROM new), last_modified = now()
		WHERE internal_id = (SELECT int_id FROM int)`
	commandTag, err = tx.Exec(dbQuery, dbOwner, dbFolder, dbName, discID)
	if err != nil {
		log.Printf("Updating comment count for discussion '%v' of '%s%s%s' in PostgreSQL failed: %v\n",
			discID, dbOwner, dbFolder, dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%v) affected when updating comment count for discussion '%v' in "+
			"'%s%s%s'\n", numRows, discID, dbOwner, dbFolder, dbName)
	}

	// Commit the transaction
	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

// Deletes a database from PostgreSQL.
func DeleteDatabase(dbOwner string, dbFolder string, dbName string) error {
	// TODO: At some point we'll need to figure out a garbage collection approach to remove databases from Minio which
	// TODO  are no longer pointed to by anything

	// Begin a transaction
	tx, err := pdb.Begin()
	if err != nil {
		return err
	}
	// Set up an automatic transaction roll back if the function exits without committing
	defer tx.Rollback()

	// Check if there are any forks of this database
	dbQuery := `
		WITH this_db AS (
			SELECT db_id
			FROM sqlite_databases
			WHERE user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND folder = $2
				AND db_name = $3
		)
		SELECT count(*)
		FROM sqlite_databases AS db, this_db
		WHERE db.forked_from = this_db.db_id`
	var numForks int
	err = tx.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&numForks)
	if err != nil {
		log.Printf("Retreving fork list failed for database '%s%s%s': %v\n", dbOwner, dbFolder, dbName, err)
		return err
	}
	if numForks == 0 {
		// * There are no forks for this database, so we just remove it's entry from sqlite_databases.  The 'ON DELETE
		// CASCADE' definition for the database_stars table/field should automatically remove any references to the
		// now deleted entry *

		// Update the fork count for the root database
		dbQuery = `
			WITH root_db AS (
				SELECT root_database AS id
				FROM sqlite_databases
				WHERE user_id = (
						SELECT user_id
						FROM users
						WHERE lower(user_name) = lower($1)
					)
					AND folder = $2
					AND db_name = $3
			), new_count AS (
				SELECT count(*) AS forks
				FROM sqlite_databases AS db, root_db
				WHERE db.root_database = root_db.id
				AND db.is_deleted = false
			)
			UPDATE sqlite_databases
			SET forks = new_count.forks - 2
			FROM new_count, root_db
			WHERE sqlite_databases.db_id = root_db.id`
		commandTag, err := tx.Exec(dbQuery, dbOwner, dbFolder, dbName)
		if err != nil {
			log.Printf("Updating fork count for '%s%s%s' in PostgreSQL failed: %v\n", dbOwner, dbFolder, dbName,
				err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf("Wrong number of rows (%v) affected (spot 1) when updating fork count for database '%s%s%s'\n",
				numRows, dbOwner, dbFolder, dbName)
		}

		// Do the database deletion in PostgreSQL (needs to come after the above fork count update, else the fork count
		// won't be able to find the root database id)
		dbQuery = `
			DELETE
			FROM sqlite_databases
			WHERE user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND folder = $2
				AND db_name = $3`
		commandTag, err = tx.Exec(dbQuery, dbOwner, dbFolder, dbName)
		if err != nil {
			log.Printf("Deleting database entry failed for database '%s%s%s': %v\n", dbOwner, dbFolder, dbName,
				err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf(
				"Wrong number of rows (%v) affected when deleting database '%s%s%s'\n", numRows, dbOwner,
				dbFolder, dbName)
		}

		// Commit the transaction
		err = tx.Commit()
		if err != nil {
			return err
		}

		// Log the database deletion
		log.Printf("Database '%s%s%s' deleted\n", dbOwner, dbFolder, dbName)
		return nil
	}

	// * If there are any forks of this database, we need to leave stub/placeholder info for its entry so the fork tree
	// doesn't go weird.  We also set the "is_deleted" boolean to true for its entry, so our database query functions
	// know to skip it *

	// Delete all stars referencing the database stub
	dbQuery = `
		DELETE FROM database_stars
		WHERE db_id = (
			SELECT db_id
			FROM sqlite_databases
			WHERE user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND folder = $2
				AND db_name = $3
			)`
	commandTag, err := tx.Exec(dbQuery, dbOwner, dbFolder, dbName)
	if err != nil {
		log.Printf("Deleting (forked) database stars failed for database '%s%s%s': %v\n", dbOwner, dbFolder,
			dbName, err)
		return err
	}

	// Generate a random string to be used in the deleted database's name field, so if the user adds a database with
	// the deleted one's name then the unique constraint on the database won't reject it
	newName := "deleted-database-" + RandomString(20)

	// Replace the database entry in sqlite_databases with a stub
	dbQuery = `
		UPDATE sqlite_databases AS db
		SET is_deleted = true, public = false, db_name = $4, last_modified = now()
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND folder = $2
			AND db_name = $3`
	commandTag, err = tx.Exec(dbQuery, dbOwner, dbFolder, dbName, newName)
	if err != nil {
		log.Printf("Deleting (forked) database entry failed for database '%s%s%s': %v\n", dbOwner, dbFolder,
			dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf(
			"Wrong number of rows (%v) affected when deleting (forked) database '%s%s%s'\n", numRows, dbOwner,
			dbFolder, dbName)
	}

	// Update the fork count for the root database
	dbQuery = `
		WITH root_db AS (
			SELECT root_database AS id
			FROM sqlite_databases
			WHERE user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND folder = $2
				AND db_name = $3
		), new_count AS (
			SELECT count(*) AS forks
			FROM sqlite_databases AS db, root_db
			WHERE db.root_database = root_db.id
			AND db.is_deleted = false
		)
		UPDATE sqlite_databases
		SET forks = new_count.forks - 1
		FROM new_count, root_db
		WHERE sqlite_databases.db_id = root_db.id`
	commandTag, err = tx.Exec(dbQuery, dbOwner, dbFolder, newName)
	if err != nil {
		log.Printf("Updating fork count for '%s%s%s' in PostgreSQL failed: %v\n", dbOwner, dbFolder, dbName,
			err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%v) affected (spot 2) when updating fork count for database '%s%s%s'\n",
			numRows, dbOwner, dbFolder, dbName)
	}

	// Commit the transaction
	err = tx.Commit()
	if err != nil {
		return err
	}

	// Log the database deletion
	log.Printf("(Forked) database '%s%s%s' deleted\n", dbOwner, dbFolder, dbName)
	return nil
}

// Removes a (user supplied) database licence from the system.
func DeleteLicence(userName string, licenceName string) (err error) {
	// Begin a transaction
	tx, err := pdb.Begin()
	if err != nil {
		return err
	}
	// Set up an automatic transaction roll back if the function exits without committing
	defer tx.Rollback()

	// Don't allow deletion of the default licences
	switch licenceName {
	case "Not specified":
	case "CC0":
	case "CC-BY-4.0":
	case "CC-BY-SA-4.0":
	case "CC-BY-NC-4.0":
	case "CC-BY-IGO-3.0":
	case "ODbL-1.0":
	case "UK-OGL-3":
		return errors.New("Default licences can't be removed")
	}

	// Retrieve the SHA256 for the licence
	licSHA, err := GetLicenceSha256FromName(userName, licenceName)
	if err != nil {
		return err
	}

	// * Check if there are databases present which use this licence.  If there are, then abort. *

	// TODO: Get around to adding appropriate GIN indexes

	// Note - This uses the JsQuery extension for PostgreSQL, which needs compiling on the server and adding in.
	//        However, this seems like it'll be much more straight forward and usable for writing queries with than
	//        than to use straight PG SQL
	//        JsQuery repo: https://github.com/postgrespro/jsquery
	//        Some useful examples: https://postgrespro.ru/media/2017/04/04/jsonb-pgconf.us-2017.pdf
	dbQuery := `
		SELECT DISTINCT count(*)
		FROM sqlite_databases AS db
		WHERE db.commit_list @@ '*.licence = %s'
			AND (user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = 'default'
			)
			OR user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			))`
	// We do this because licSHA needs to be unquoted for JsQuery to work, and the Go PG driver mucks things
	// up if it's given as a parameter to that (eg as $2)
	dbQuery = fmt.Sprintf(dbQuery, licSHA)

	// The same query in straight PG:
	//dbQuery := `
	//	WITH working_set AS (
	//		SELECT DISTINCT db.db_id
	//		FROM sqlite_databases AS db
	//			CROSS JOIN jsonb_each(db.commit_list) AS firstjoin
	//			CROSS JOIN jsonb_array_elements(firstjoin.value -> 'tree' -> 'entries') AS secondjoin
	//		WHERE secondjoin ->> 'licence' = $2
	//			AND (
	//				user_id = (
	//					SELECT user_id
	//					FROM users
	//					WHERE user_name = 'default'
	//				)
	//				OR user_id = (
	//					SELECT user_id
	//					FROM users
	//					WHERE lower(user_name) = lower($1)
	//				)
	//			)
	//	)
	//	SELECT count(*)
	//	FROM working_set`

	var DBCount int
	err = pdb.QueryRow(dbQuery, userName).Scan(&DBCount)
	if err != nil {
		log.Printf("Checking if the licence is in use failed: %v\n", err)
		return err
	}
	if DBCount != 0 {
		// Database isn't in our system
		return errors.New("Can't delete the licence, as it's already being used by databases")
	}

	// Delete the licence
	dbQuery = `
		DELETE FROM database_licences
		WHERE lic_sha256 = $2
			AND friendly_name = $3
			AND (user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = 'default'
			)
			OR user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			))`
	commandTag, err := tx.Exec(dbQuery, userName, licSHA, licenceName)
	if err != nil {
		log.Printf("Error when retrieving sha256 for licence '%s', user '%s' from database: %v\n", licenceName,
			userName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%v) affected when deleting licence '%s' for user '%s'\n",
			numRows, licenceName, userName)
	}

	// Commit the transaction
	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

// Disconnects the PostgreSQL database connection.
func DisconnectPostgreSQL() {
	pdb.Close()

	// Log successful disconnection
	log.Printf("Disconnected from PostgreSQL server: %v:%v\n", Conf.Pg.Server, uint16(Conf.Pg.Port))
}

// Returns the list of discussions or MRs for a given database.
// If a non-0 discID value is passed, it will only return the details for that specific discussion/MR.  Otherwise it
// will return a list of all discussions or MRs for a given database
// Note - This returns a slice of DiscussionEntry, instead of a map.  We use a slice because it lets us use an ORDER
//        BY clause in the SQL and preserve the returned order (maps don't preserve order).  If in future we no longer
//        need to preserve the order, it might be useful to switch to using a map instead since they're often simpler
//        to work with.
func Discussions(dbOwner string, dbFolder string, dbName string, discType DiscussionType, discID int) (list []DiscussionEntry, err error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
				AND db.folder = $2
				AND db.db_name = $3)
		SELECT disc.disc_id, disc.title, disc.open, disc.date_created, users.user_name, users.email, users.avatar_url,
			disc.description, last_modified, comment_count, mr_source_db_id, mr_source_db_branch,
			mr_destination_branch, mr_state, mr_commits
		FROM discussions AS disc, d, users
		WHERE disc.db_id = d.db_id
			AND disc.discussion_type = $4
			AND disc.creator = users.user_id`
	if discID != 0 {
		dbQuery += fmt.Sprintf(`
			AND disc_id = %d`, discID)
	}
	dbQuery += `
		ORDER BY last_modified DESC`
	var rows *pgx.Rows
	rows, err = pdb.Query(dbQuery, dbOwner, dbFolder, dbName, discType)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return
	}
	for rows.Next() {
		var av, em, sb, db pgx.NullString
		var sdb pgx.NullInt64
		var oneRow DiscussionEntry
		err = rows.Scan(&oneRow.ID, &oneRow.Title, &oneRow.Open, &oneRow.DateCreated, &oneRow.Creator, &em, &av,
			&oneRow.Body, &oneRow.LastModified, &oneRow.CommentCount, &sdb, &sb, &db, &oneRow.MRDetails.State,
			&oneRow.MRDetails.Commits)
		if err != nil {
			log.Printf("Error retrieving discussion/MR list for database '%s%s%s': %v\n", dbOwner, dbFolder,
				dbName, err)
			rows.Close()
			return
		}
		if av.Valid {
			oneRow.AvatarURL = av.String
		} else {
			// If no avatar URL is presently stored, default to a gravatar based on the users email (if known)
			if em.Valid {
				picHash := md5.Sum([]byte(em.String))
				oneRow.AvatarURL = fmt.Sprintf("https://www.gravatar.com/avatar/%x?d=identicon&s=30", picHash)
			}
		}
		if discType == MERGE_REQUEST && sdb.Valid {
			oneRow.MRDetails.SourceDBID = sdb.Int64
		}
		if sb.Valid {
			oneRow.MRDetails.SourceBranch = sb.String
		}
		if db.Valid {
			oneRow.MRDetails.DestBranch = db.String
		}
		oneRow.BodyRendered = string(gfm.Markdown([]byte(oneRow.Body)))
		list = append(list, oneRow)
	}

	// For merge requests, turn the source database ID's into full owner/folder/name strings
	if discType == MERGE_REQUEST {
		for i, j := range list {
			// Retrieve the owner/folder/name for a database id
			dbQuery = `
				SELECT users.user_name, db.folder, db.db_name
				FROM sqlite_databases AS db, users
				WHERE db.db_id = $1
					AND db.user_id = users.user_id`
			var o, f, n pgx.NullString
			err2 := pdb.QueryRow(dbQuery, j.MRDetails.SourceDBID).Scan(&o, &f, &n)
			if err2 != nil && err2 != pgx.ErrNoRows {
				log.Printf("Retrieving source database owner/folder/name failed: %v\n", err)
				return
			}
			if o.Valid {
				list[i].MRDetails.SourceOwner = o.String
			}
			if f.Valid {
				list[i].MRDetails.SourceFolder = f.String
			}
			if n.Valid {
				list[i].MRDetails.SourceDBName = n.String
			}
		}
	}

	rows.Close()
	return
}

// Returns the list of comments for a given discussion.
// If a non-0 comID value is passed, it will only return the details for that specific comment in the discussion.
// Otherwise it will return a list of all comments for a given discussion
// Note - This returns a slice instead of a map.  We use a slice because it lets us use an ORDER BY clause in the SQL
//        and preserve the returned order (maps don't preserve order).  If in future we no longer need to preserve the
//        order, it might be useful to switch to using a map instead since they're often simpler to work with.
func DiscussionComments(dbOwner string, dbFolder string, dbName string, discID int, comID int) (list []DiscussionCommentEntry, err error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
				AND db.folder = $2
				AND db.db_name = $3
		), int AS (
				SELECT internal_id AS int_id
				FROM discussions
				WHERE db_id = (SELECT db_id FROM d)
				AND disc_id = $4
			)
		SELECT com.com_id, users.user_name, users.email, users.avatar_url, com.date_created, com.body, com.entry_type
		FROM discussion_comments AS com, d, users
		WHERE com.db_id = d.db_id
			AND com.disc_id = (SELECT int_id FROM int)
			AND com.commenter = users.user_id`
	if comID != 0 {
		dbQuery += fmt.Sprintf(`
			AND com.com_id = %d`, comID)
	}
	dbQuery += `
		ORDER BY date_created ASC`
	var rows *pgx.Rows
	rows, err = pdb.Query(dbQuery, dbOwner, dbFolder, dbName, discID)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return
	}
	for rows.Next() {
		var av, em pgx.NullString
		var oneRow DiscussionCommentEntry
		err = rows.Scan(&oneRow.ID, &oneRow.Commenter, &em, &av, &oneRow.DateCreated, &oneRow.Body, &oneRow.EntryType)
		if err != nil {
			log.Printf("Error retrieving comment list for database '%s%s%s', discussion '%d': %v\n", dbOwner,
				dbFolder, dbName, discID, err)
			rows.Close()
			return
		}

		if av.Valid {
			oneRow.AvatarURL = av.String
		} else {
			if em.Valid {
				picHash := md5.Sum([]byte(em.String))
				oneRow.AvatarURL = fmt.Sprintf("https://www.gravatar.com/avatar/%x?d=identicon&s=30", picHash)
			}
		}

		oneRow.BodyRendered = string(gfm.Markdown([]byte(oneRow.Body)))
		list = append(list, oneRow)
	}
	rows.Close()
	return
}

// Periodically flushes the database view count from memcache to PostgreSQL
func FlushViewCount() {
	type dbEntry struct {
		Owner  string
		Folder string
		Name   string
	}

	// Log the start of the loop
	log.Printf("Periodic view count flushing loop started.  %d second refresh.\n",
		Conf.Memcache.ViewCountFlushDelay)

	// Start the endless flush loop
	var rows *pgx.Rows
	var err error
	for true {
		// Retrieve the list of all public databases
		dbQuery := `
			SELECT users.user_name, db.folder, db.db_name
			FROM sqlite_databases AS db, users
			WHERE db.public = true
				AND db.is_deleted = false
				AND db.user_id = users.user_id`
		rows, err = pdb.Query(dbQuery)
		if err != nil {
			log.Printf("Database query failed: %v\n", err)
			return
		}
		var dbList []dbEntry
		for rows.Next() {
			var oneRow dbEntry
			err = rows.Scan(&oneRow.Owner, &oneRow.Folder, &oneRow.Name)
			if err != nil {
				log.Printf("Error retrieving database list for view count flush thread: %v\n", err)
				rows.Close()
				return
			}
			dbList = append(dbList, oneRow)
		}
		rows.Close()

		// For each public database, retrieve the latest view count from memcache and save it back to PostgreSQL
		for _, db := range dbList {
			dbOwner := db.Owner
			dbFolder := db.Folder
			dbName := db.Name

			// Retrieve the view count from Memcached
			newValue, err := GetViewCount(dbOwner, dbFolder, dbName)
			if err != nil {
				log.Printf("Error when getting memcached view count for %s%s%s: %s\n", dbOwner, dbFolder, dbName,
					err.Error())
				continue
			}

			// We use a value of -1 to indicate there wasn't an entry in memcache for the database
			if newValue != -1 {
				// Update the view count in PostgreSQL
				dbQuery = `
					UPDATE sqlite_databases
					SET page_views = $4
					WHERE user_id = (
							SELECT user_id
							FROM users
							WHERE lower(user_name) = lower($1)
						)
						AND folder = $2
						AND db_name = $3`
				commandTag, err := pdb.Exec(dbQuery, dbOwner, dbFolder, dbName, newValue)
				if err != nil {
					log.Printf("Flushing view count for '%s%s%s' failed: %v\n", dbOwner, dbFolder, dbName, err)
					continue
				}
				if numRows := commandTag.RowsAffected(); numRows != 1 {
					log.Printf("Wrong number of rows affected (%v) when flushing view count for '%s%s%s'\n",
						numRows, dbOwner, dbFolder, dbName)
					continue
				}
			}
		}

		// Wait before running the loop again
		time.Sleep(Conf.Memcache.ViewCountFlushDelay * time.Second)
	}
	return
}

// Fork the PostgreSQL entry for a SQLite database from one user to another
func ForkDatabase(srcOwner string, dbFolder string, dbName string, dstOwner string) (newForkCount int, err error) {
	// Copy the main database entry
	dbQuery := `
		WITH dst_u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		INSERT INTO sqlite_databases (user_id, folder, db_name, public, forks, one_line_description, full_description,
			branches, contributors, root_database, default_table, source_url, commit_list, branch_heads, tags,
			default_branch, forked_from)
		SELECT dst_u.user_id, folder, db_name, public, 0, one_line_description, full_description, branches,
			contributors, root_database, default_table, source_url, commit_list, branch_heads, tags, default_branch,
			db_id
		FROM sqlite_databases, dst_u
		WHERE sqlite_databases.user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($2)
			)
			AND folder = $3
			AND db_name = $4`
	commandTag, err := pdb.Exec(dbQuery, dstOwner, srcOwner, dbFolder, dbName)
	if err != nil {
		log.Printf("Forking database '%s%s%s' in PostgreSQL failed: %v\n", srcOwner, dbFolder, dbName, err)
		return 0, err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows affected (%d) when forking main database entry: "+
			"'%s%s%s' to '%s%s%s'\n", numRows, srcOwner, dbFolder, dbName, dstOwner, dbFolder, dbName)
	}

	// Update the fork count for the root database
	dbQuery = `
		WITH root_db AS (
			SELECT root_database AS id
			FROM sqlite_databases
			WHERE user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND folder = $2
				AND db_name = $3
		), new_count AS (
			SELECT count(*) AS forks
			FROM sqlite_databases AS db, root_db
			WHERE db.root_database = root_db.id
			AND db.is_deleted = false
		)
		UPDATE sqlite_databases
		SET forks = new_count.forks - 1
		FROM new_count, root_db
		WHERE sqlite_databases.db_id = root_db.id
		RETURNING new_count.forks - 1`
	err = pdb.QueryRow(dbQuery, dstOwner, dbFolder, dbName).Scan(&newForkCount)
	if err != nil {
		log.Printf("Updating fork count in PostgreSQL failed: %v\n", err)
		return 0, err
	}
	return newForkCount, nil
}

// Checks if the given database was forked from another, and if so returns that one's owner, folder and database name
func ForkedFrom(dbOwner string, dbFolder string, dbName string) (forkOwn string, forkFol string, forkDB string,
	forkDel bool, err error) {
	// Check if the database was forked from another
	var dbID, forkedFrom pgx.NullInt64
	dbQuery := `
		SELECT db_id, forked_from
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1))
			AND folder = $2
			AND db_name = $3`
	err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&dbID, &forkedFrom)
	if err != nil {
		log.Printf("Error checking if database was forked from another '%s%s%s'. Error: %v\n", dbOwner,
			dbFolder, dbName, err)
		return "", "", "", false, err
	}
	if !forkedFrom.Valid {
		// The database wasn't forked, so return empty strings
		return "", "", "", false, nil
	}

	// Return the details of the database this one was forked from
	dbQuery = `
		SELECT u.user_name, db.folder, db.db_name, db.is_deleted
		FROM users AS u, sqlite_databases AS db
		WHERE db.db_id = $1
			AND u.user_id = db.user_id`
	err = pdb.QueryRow(dbQuery, forkedFrom).Scan(&forkOwn, &forkFol, &forkDB, &forkDel)
	if err != nil {
		log.Printf("Error retrieving forked database information for '%s%s%s'. Error: %v\n", dbOwner,
			dbFolder, dbName, err)
		return "", "", "", false, err
	}

	// If the database this one was forked from has been deleted, indicate that and clear the database name value
	if forkDel {
		forkDB = ""
	}
	return forkOwn, forkFol, forkDB, forkDel, nil
}

// Return the parent of a database, if there is one (and it's accessible to the logged in user).  If no parent was
// found, the returned Owner/Folder/DBName values will be empty strings
func ForkParent(loggedInUser string, dbOwner string, dbFolder string, dbName string) (parentOwner string,
	parentFolder string, parentDBName string, err error) {
	dbQuery := `
		SELECT users.user_name, db.folder, db.db_name, db.public, db.db_id, db.forked_from, db.is_deleted
		FROM sqlite_databases AS db, users
		WHERE db.root_database = (
				SELECT root_database
				FROM sqlite_databases
				WHERE user_id = (
						SELECT user_id
						FROM users
						WHERE lower(user_name) = lower($1)
					)
					AND folder = $2
					AND db_name = $3
				)
			AND db.user_id = users.user_id
		ORDER BY db.forked_from NULLS FIRST`
	rows, err := pdb.Query(dbQuery, dbOwner, dbFolder, dbName)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return
	}
	defer rows.Close()
	dbList := make(map[int]ForkEntry)
	for rows.Next() {
		var frk pgx.NullInt64
		var oneRow ForkEntry
		err = rows.Scan(&oneRow.Owner, &oneRow.Folder, &oneRow.DBName, &oneRow.Public, &oneRow.ID, &frk, &oneRow.Deleted)
		if err != nil {
			log.Printf("Error retrieving fork parent for '%s%s%s': %v\n", dbOwner, dbFolder, dbName,
				err)
			return
		}
		if frk.Valid {
			oneRow.ForkedFrom = int(frk.Int64)
		}
		dbList[oneRow.ID] = oneRow
	}

	// Safety check
	numResults := len(dbList)
	if numResults == 0 {
		err = fmt.Errorf("Empty list returned instead of fork tree.  This shouldn't happen")
		return
	}

	// Get the ID of the database being called
	dbID, err := databaseID(dbOwner, dbFolder, dbName)
	if err != nil {
		return
	}

	// Find the closest (not-deleted) parent for the database
	dbEntry, ok := dbList[dbID]
	if !ok {
		// The database itself wasn't found in the list.  This shouldn't happen
		err = fmt.Errorf("Internal error when retrieving fork parent info.  This shouldn't happen.")
		return
	}
	for dbEntry.ForkedFrom != 0 {
		dbEntry, ok = dbList[dbEntry.ForkedFrom]
		if !ok {
			// Parent database entry wasn't found in the list.  This shouldn't happen either
			err = fmt.Errorf("Internal error when retrieving fork parent info (#2).  This shouldn't happen.")
			return
		}
		if !dbEntry.Deleted {
			// Found a parent (that's not deleted).  We'll use this and stop looping
			parentOwner = dbEntry.Owner
			parentFolder = dbEntry.Folder
			parentDBName = dbEntry.DBName
			break
		}
	}

	return
}

// Return the complete fork tree for a given database
func ForkTree(loggedInUser string, dbOwner string, dbFolder string, dbName string) (outputList []ForkEntry, err error) {
	dbQuery := `
		SELECT users.user_name, db.folder, db.db_name, db.public, db.db_id, db.forked_from, db.is_deleted
		FROM sqlite_databases AS db, users
		WHERE db.root_database = (
				SELECT root_database
				FROM sqlite_databases
				WHERE user_id = (
						SELECT user_id
						FROM users
						WHERE lower(user_name) = lower($1)
					)
					AND folder = $2
					AND db_name = $3
				)
			AND db.user_id = users.user_id
		ORDER BY db.forked_from NULLS FIRST`
	rows, err := pdb.Query(dbQuery, dbOwner, dbFolder, dbName)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return nil, err
	}
	defer rows.Close()
	var dbList []ForkEntry
	for rows.Next() {
		var frk pgx.NullInt64
		var oneRow ForkEntry
		err = rows.Scan(&oneRow.Owner, &oneRow.Folder, &oneRow.DBName, &oneRow.Public, &oneRow.ID, &frk, &oneRow.Deleted)
		if err != nil {
			log.Printf("Error retrieving fork list for '%s%s%s': %v\n", dbOwner, dbFolder, dbName,
				err)
			return nil, err
		}
		if frk.Valid {
			oneRow.ForkedFrom = int(frk.Int64)
		}
		dbList = append(dbList, oneRow)
	}

	// Safety checks
	numResults := len(dbList)
	if numResults == 0 {
		return nil, errors.New("Empty list returned instead of fork tree.  This shouldn't happen")
	}
	if dbList[0].ForkedFrom != 0 {
		// The first entry has a non-zero forked_from field, indicating it's not the root entry.  That
		// shouldn't happen, so return an error.
		return nil, errors.New("Incorrect root entry data in retrieved database list")
	}

	// * Process the root entry *

	var iconDepth int
	var forkTrail []int

	// Set the root database ID
	rootID := dbList[0].ID

	// Set the icon list for display in the browser
	dbList[0].IconList = append(dbList[0].IconList, ROOT)

	// If the root database is no longer public, then use placeholder details instead
	if !dbList[0].Public && (strings.ToLower(dbList[0].Owner) != strings.ToLower(loggedInUser)) {
		dbList[0].DBName = "private database"
	}

	// If the root database is deleted, use a placeholder indicating that instead
	if dbList[0].Deleted {
		dbList[0].DBName = "deleted database"
	}

	// Append this completed database line to the output list
	outputList = append(outputList, dbList[0])

	// Append the root database ID to the fork trail
	forkTrail = append(forkTrail, rootID)

	// Mark the root database entry as processed
	dbList[0].Processed = true

	// Increment the icon depth
	iconDepth = 1

	// * Sort the remaining entries for correct display *
	numUnprocessedEntries := numResults - 1
	for numUnprocessedEntries > 0 {
		var forkFound bool
		outputList, forkTrail, forkFound = nextChild(loggedInUser, &dbList, &outputList, &forkTrail, iconDepth)
		if forkFound {
			numUnprocessedEntries--
			iconDepth++

			// Add stems and branches to the output icon list
			numOutput := len(outputList)

			myID := outputList[numOutput-1].ID
			myForkedFrom := outputList[numOutput-1].ForkedFrom

			// Scan through the earlier output list for any sibling entries
			var siblingFound bool
			for i := numOutput; i > 0 && siblingFound == false; i-- {
				thisID := outputList[i-1].ID
				thisForkedFrom := outputList[i-1].ForkedFrom

				if thisForkedFrom == myForkedFrom && thisID != myID {
					// Sibling entry found
					siblingFound = true
					sibling := outputList[i-1]

					// Change the last sibling icon to a branch icon
					sibling.IconList[iconDepth-1] = BRANCH

					// Change appropriate spaces to stems in the output icon list
					for l := numOutput - 1; l > i; l-- {
						thisEntry := outputList[l-1]
						if thisEntry.IconList[iconDepth-1] == SPACE {
							thisEntry.IconList[iconDepth-1] = STEM
						}
					}
				}
			}
		} else {
			// No child was found, so remove an entry from the fork trail then continue looping
			forkTrail = forkTrail[:len(forkTrail)-1]

			iconDepth--
		}
	}

	return outputList, nil
}

func GetActivityStats() (stats ActivityStats, err error) {
	// Retrieve a list of which databases are the most starred
	dbQuery := `
		WITH most_starred AS (
			SELECT s.db_id, COUNT(s.db_id), max(s.date_starred)
			FROM database_stars AS s, sqlite_databases AS db
			WHERE s.db_id = db.db_id
				AND db.public = true
				AND db.is_deleted = false
			GROUP BY s.db_id
			ORDER BY count DESC
			LIMIT 5
		)
		SELECT users.user_name, db.db_name, stars.count
		FROM most_starred AS stars, sqlite_databases AS db, users
		WHERE stars.db_id = db.db_id
			AND users.user_id = db.user_id
		ORDER BY count DESC, max ASC`
	starRows, err := pdb.Query(dbQuery)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return
	}
	defer starRows.Close()
	for starRows.Next() {
		var oneRow ActivityRow
		err = starRows.Scan(&oneRow.Owner, &oneRow.DBName, &oneRow.Count)
		if err != nil {
			log.Printf("Error retrieving list of most starred databases: %v\n", err)
			return
		}
		stats.Starred = append(stats.Starred, oneRow)
	}

	// Retrieve a list of which databases are the most forked
	dbQuery = `
		SELECT users.user_name, db.db_name, db.forks
		FROM sqlite_databases AS db, users
		WHERE db.forks > 0
			AND db.public = true
			AND db.is_deleted = false
			AND db.user_id = users.user_id
		ORDER BY db.forks DESC, db.last_modified
		LIMIT 5`
	forkRows, err := pdb.Query(dbQuery)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return
	}
	defer forkRows.Close()
	for forkRows.Next() {
		var oneRow ActivityRow
		err = forkRows.Scan(&oneRow.Owner, &oneRow.DBName, &oneRow.Count)
		if err != nil {
			log.Printf("Error retrieving list of most forked databases: %v\n", err)
			return
		}
		stats.Forked = append(stats.Forked, oneRow)
	}

	// Retrieve a list of the most recent uploads
	dbQuery = `
		SELECT user_name, db.db_name, db.last_modified
		FROM sqlite_databases AS db, users
		WHERE db.forked_from IS NULL
			AND db.public = true
			AND db.is_deleted = false
			AND db.user_id = users.user_id
		ORDER BY db.last_modified DESC
		LIMIT 5`
	upRows, err := pdb.Query(dbQuery)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return
	}
	defer upRows.Close()
	for upRows.Next() {
		var oneRow UploadRow
		err = upRows.Scan(&oneRow.Owner, &oneRow.DBName, &oneRow.UploadDate)
		if err != nil {
			log.Printf("Error retrieving list of most recent uploads: %v\n", err)
			return
		}
		stats.Uploads = append(stats.Uploads, oneRow)
	}

	// Retrieve a list of which databases have been downloaded the most times by someone other than their owner
	dbQuery = `
		SELECT users.user_name, db.db_name, db.download_count
		FROM sqlite_databases AS db, users
		WHERE db.download_count > 0
			AND db.public = true
			AND db.is_deleted = false
			AND db.user_id = users.user_id
		ORDER BY db.download_count DESC, db.last_modified
		LIMIT 5`
	dlRows, err := pdb.Query(dbQuery)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return
	}
	defer dlRows.Close()
	for dlRows.Next() {
		var oneRow ActivityRow
		err = dlRows.Scan(&oneRow.Owner, &oneRow.DBName, &oneRow.Count)
		if err != nil {
			log.Printf("Error retrieving list of most downloaded databases: %v\n", err)
			return
		}
		stats.Downloads = append(stats.Downloads, oneRow)
	}

	// Retrieve the list of databases which have been viewed the most times
	dbQuery = `
		SELECT users.user_name, db.db_name, db.page_views
		FROM sqlite_databases AS db, users
		WHERE db.page_views > 0
			AND db.public = true
			AND db.is_deleted = false
			AND db.user_id = users.user_id
		ORDER BY db.page_views DESC, db.last_modified
		LIMIT 5`
	viewRows, err := pdb.Query(dbQuery)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return
	}
	defer viewRows.Close()
	for viewRows.Next() {
		var oneRow ActivityRow
		err = viewRows.Scan(&oneRow.Owner, &oneRow.DBName, &oneRow.Count)
		if err != nil {
			log.Printf("Error retrieving list of most viewed databases: %v\n", err)
			return
		}
		stats.Viewed = append(stats.Viewed, oneRow)
	}
	return
}

// Load the branch heads for a database.
// TODO: It might be better to have the default branch name be returned as part of this list, by indicating in the list
// TODO  which of the branches is the default.
func GetBranches(dbOwner string, dbFolder string, dbName string) (branches map[string]BranchEntry, err error) {
	dbQuery := `
		SELECT db.branch_heads
		FROM sqlite_databases AS db
		WHERE db.user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db.folder = $2
			AND db.db_name = $3`
	err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&branches)
	if err != nil {
		log.Printf("Error when retrieving branch heads for database '%s%s%s': %v\n", dbOwner, dbFolder, dbName,
			err)
		return nil, err
	}
	return branches, nil
}

// Retrieves the full commit list for a database.
func GetCommitList(dbOwner string, dbFolder string, dbName string) (map[string]CommitEntry, error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		SELECT commit_list as commits
		FROM sqlite_databases AS db, u
		WHERE db.user_id = u.user_id
			AND db.folder = $2
			AND db.db_name = $3
			AND db.is_deleted = false`
	var l map[string]CommitEntry
	err := pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&l)
	if err != nil {
		log.Printf("Retrieving commit list for '%s%s%s' failed: %v\n", dbOwner, dbFolder, dbName, err)
		return map[string]CommitEntry{}, err
	}
	return l, nil
}

// Returns the default branch name for a database.
func GetDefaultBranchName(dbOwner string, dbFolder string, dbName string) (branchName string, err error) {
	dbQuery := `
		SELECT db.default_branch
		FROM sqlite_databases AS db
		WHERE db.user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db.folder = $2
			AND db.db_name = $3
			AND db.is_deleted = false`
	var b pgx.NullString
	err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&b)
	if err != nil {
		if err != pgx.ErrNoRows {
			log.Printf("Error when retrieving default branch name for database '%s%s%s': %v\n", dbOwner,
				dbFolder, dbName, err)
			return
		} else {
			log.Printf("No default branch name exists for database '%s%s%s'. This shouldn't happen\n", dbOwner,
				dbFolder, dbName)
			return
		}
	}
	if b.Valid {
		branchName = b.String
	}
	return
}

// Returns the default table name for a database.
func GetDefaultTableName(dbOwner string, dbFolder string, dbName string) (tableName string, err error) {
	dbQuery := `
		SELECT db.default_table
		FROM sqlite_databases AS db
		WHERE db.user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db.folder = $2
			AND db.db_name = $3
			AND db.is_deleted = false`
	var t pgx.NullString
	err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&t)
	if err != nil {
		if err != pgx.ErrNoRows {
			log.Printf("Error when retrieving default table name for database '%s%s%s': %v\n", dbOwner,
				dbFolder, dbName, err)
			return
		}
	}
	if t.Valid {
		tableName = t.String
	}
	return
}

// Returns the discussion and merge request counts for a database
// TODO: The only reason this function exists atm, is because we're incorrectly caching the discussion and MR data in
// TODO  a way that makes invalidating it correctly hard/impossible.  We should redo our memcached approach to solve the
// TODO  issue properly
func GetDiscussionAndMRCount(dbOwner string, dbFolder string, dbName string) (discCount int, mrCount int, err error) {
	dbQuery := `
		SELECT db.discussions, db.merge_requests
		FROM sqlite_databases AS db
		WHERE db.user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db.folder = $2
			AND db.db_name = $3
			AND db.is_deleted = false`
	err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&discCount, &mrCount)
	if err != nil {
		if err != pgx.ErrNoRows {
			log.Printf("Error when retrieving discussion and MR count for database '%s%s%s': %v\n", dbOwner,
				dbFolder, dbName, err)
			return
		} else {
			log.Printf("Database '%s%s%s' not found when attempting to retrieve discussion and MR count. This"+
				"shouldn't happen\n", dbOwner, dbFolder, dbName)
			return
		}
	}
	return
}

// Returns the text for a given licence.
func GetLicence(userName string, licenceName string) (txt string, format string, err error) {
	dbQuery := `
		SELECT licence_text, file_format
		FROM database_licences
		WHERE friendly_name ILIKE $2
		AND (
				user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				) OR
				user_id = (
					SELECT user_id
					FROM users
					WHERE user_name = 'default'
				)
			)`
	err = pdb.QueryRow(dbQuery, userName, licenceName).Scan(&txt, &format)
	if err != nil {
		if err == pgx.ErrNoRows {
			// The requested licence text wasn't found
			return "", "", errors.New("unknown licence")
		}
		log.Printf("Error when retrieving licence '%s', user '%s': %v\n", licenceName, userName, err)
		return "", "", err
	}
	return txt, format, nil
}

// Returns the list of licences available to a user.
func GetLicences(user string) (map[string]LicenceEntry, error) {
	dbQuery := `
		SELECT friendly_name, full_name, lic_sha256, licence_url, file_format, display_order
		FROM database_licences
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = 'default'
			)
			OR user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)`
	rows, err := pdb.Query(dbQuery, user)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return nil, err
	}
	defer rows.Close()
	lics := make(map[string]LicenceEntry)
	for rows.Next() {
		var name string
		var oneRow LicenceEntry
		err = rows.Scan(&name, &oneRow.FullName, &oneRow.Sha256, &oneRow.URL, &oneRow.FileFormat, &oneRow.Order)
		if err != nil {
			log.Printf("Error retrieving licence list: %v\n", err)
			return nil, err
		}
		lics[name] = oneRow
	}
	return lics, nil
}

// Returns the friendly name + licence URL for the licence matching a given sha256.
// Note - When user defined licence has the same sha256 as a default one we return the user defined licences' friendly
// name.
func GetLicenceInfoFromSha256(userName string, sha256 string) (lName string, lURL string, err error) {
	dbQuery := `
		SELECT u.user_name, dl.friendly_name, dl.licence_url
		FROM database_licences AS dl, users AS u
		WHERE dl.lic_sha256 = $2
			AND dl.user_id = u.user_id
			AND (dl.user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = 'default'
			)
			OR dl.user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			))`
	rows, err := pdb.Query(dbQuery, userName, sha256)
	if err != nil {
		log.Printf("Error when retrieving friendly name for licence sha256 '%s', user '%s': %v\n", sha256,
			userName, err)
		return "", "", err
	}
	defer rows.Close()
	type lic struct {
		Licence string
		Name    string
		User    string
	}
	var list []lic
	for rows.Next() {
		var oneRow lic
		err = rows.Scan(&oneRow.User, &oneRow.Name, &oneRow.Licence)
		if err != nil {
			log.Printf("Error retrieving friendly name for licence sha256 '%s', user: %v\n", sha256, err)
			return "", "", err
		}
		list = append(list, oneRow)
	}

	// Decide what to return based upon the number of licence matches
	numLics := len(list)
	switch numLics {
	case 0:
		// If there are no matching sha256's, something has gone wrong
		return "", "", errors.New("No matching licence found, something has gone wrong!")
	case 1:
		// If there's only one matching sha256, we return the corresponding licence name + url
		lName = list[0].Name
		lURL = list[0].Licence
		return lName, lURL, nil
	default:
		// If more than one name was found for the matching sha256, that seems a bit trickier.  At least one of them
		// would have to be a user defined licence, so we'll return the first one of those instead of the default
		// licence name.  This seems to allow users to define their own friendly name's for the default licences which
		// is probably not a bad thing
		for _, j := range list {
			if j.User == userName {
				lName = j.Name
				lURL = j.Licence
				break
			}
		}
	}
	if lName == "" {
		// Multiple licence friendly names were returned, but none of them matched the requesting user.  Something has
		// gone wrong
		return "", "", fmt.Errorf("Multiple matching licences found, but belonging to user %s\n", userName)
	}

	// To get here we must have successfully picked a user defined licence out of several matches.  This seems like
	// an acceptable scenario
	return lName, lURL, nil
}

// Returns the sha256 for a given licence.
func GetLicenceSha256FromName(userName string, licenceName string) (sha256 string, err error) {
	dbQuery := `
		SELECT lic_sha256
		FROM database_licences
		WHERE friendly_name = $2
			AND (user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = 'default'
			)
			OR user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			))`
	err = pdb.QueryRow(dbQuery, userName, licenceName).Scan(&sha256)
	if err != nil {
		log.Printf("Error when retrieving sha256 for licence '%s', user '%s' from database: %v\n", licenceName,
			userName, err)
		return "", err
	}
	if sha256 == "" {
		// The requested licence wasn't found
		return "", errors.New("Licence not found")
	}
	return sha256, nil
}

// Retrieve the list of releases for a database.
func GetReleases(dbOwner string, dbFolder string, dbName string) (releases map[string]ReleaseEntry, err error) {
	dbQuery := `
		SELECT release_list
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND folder = $2
			AND db_name = $3`
	err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&releases)
	if err != nil {
		log.Printf("Error when retrieving releases for database '%s%s%s': %v\n", dbOwner, dbFolder, dbName, err)
		return nil, err
	}
	if releases == nil {
		// If there aren't any releases yet, return an empty set instead of nil
		releases = make(map[string]ReleaseEntry)
	}
	return releases, nil
}

// Retrieve the tags for a database.
func GetTags(dbOwner string, dbFolder string, dbName string) (tags map[string]TagEntry, err error) {
	dbQuery := `
		SELECT tag_list
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND folder = $2
			AND db_name = $3`
	err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&tags)
	if err != nil {
		log.Printf("Error when retrieving tags for database '%s%s%s': %v\n", dbOwner, dbFolder, dbName, err)
		return nil, err
	}
	if tags == nil {
		// If there aren't any tags yet, return an empty set instead of nil
		tags = make(map[string]TagEntry)
	}
	return tags, nil
}

// Returns the username associated with an email address.
func GetUsernameFromEmail(email string) (userName string, avatarURL string, err error) {
	dbQuery := `
		SELECT user_name, avatar_url
		FROM users
		WHERE email = $1`
	var av pgx.NullString
	err = pdb.QueryRow(dbQuery, email).Scan(&userName, &av)
	if err != nil {
		if err == pgx.ErrNoRows {
			// No matching username of the email
			err = nil
			return
		}
		log.Printf("Looking up username for email address '%s' failed: %v\n", email, err)
		return
	}

	// If no avatar URL is presently stored, default to a gravatar based on the users email (if known)
	if !av.Valid {
		picHash := md5.Sum([]byte(email))
		avatarURL = fmt.Sprintf("https://www.gravatar.com/avatar/%x?d=identicon", picHash)
	} else {
		avatarURL = av.String
	}
	return
}

// Retrieves a saved set of visualisation query results
func GetVisualisationData(dbOwner string, dbFolder string, dbName string, commitID string, hash string) (data []VisRowV1, ok bool, err error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
				AND folder = $2
				AND db_name = $3
		)
		SELECT results
		FROM vis_result_cache as vis_cache, u, d
		WHERE vis_cache.db_id = d.db_id
			AND vis_cache.user_id = u.user_id
			AND vis_cache.commit_id = $4
			AND vis_cache.hash = $5`
	e := pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName, commitID, hash).Scan(&data)
	if e != nil {
		if e == pgx.ErrNoRows {
			// There weren't any saved parameters for this database visualisation
			return
		}

		// A real database error occurred
		err = e
		log.Printf("Checking if a database exists failed: %v\n", e)
		return
	}

	// Data was successfully retrieved
	ok = true
	return
}

// Retrieves a saved set of visualisation parameters
func GetVisualisationParams(dbOwner string, dbFolder string, dbName string, visName string) (params VisParamsV2, ok bool, err error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
				AND folder = $2
				AND db_name = $3
		)
		SELECT parameters
		FROM vis_params as vis, u, d
		WHERE vis.db_id = d.db_id
			AND vis.user_id = u.user_id
			AND vis.name = $4`
	e := pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName, visName).Scan(&params)
	if e != nil {
		if e == pgx.ErrNoRows {
			// There weren't any saved parameters for this database visualisation
			return
		}

		// A real database error occurred
		err = e
		log.Printf("Retrieving visualisation parameters for '%s%s%s', visualisation '%s' failed: %v\n", dbOwner, dbFolder, dbName, visName, e)
		return
	}

	// Parameters were successfully retrieved
	ok = true
	return
}

// Increments the download count for a database
func IncrementDownloadCount(dbOwner string, dbFolder string, dbName string) error {
	dbQuery := `
		UPDATE sqlite_databases
		SET download_count = download_count + 1
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND folder = $2
			AND db_name = $3`
	commandTag, err := pdb.Exec(dbQuery, dbOwner, dbFolder, dbName)
	if err != nil {
		log.Printf("Increment download count for '%s%s%s' failed: %v\n", dbOwner, dbFolder,
			dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		errMsg := fmt.Sprintf("Wrong number of rows affected (%v) when incrementing download count for '%s%s%s'\n",
			numRows, dbOwner, dbFolder, dbName)
		log.Printf(errMsg)
		return errors.New(errMsg)
	}
	return nil
}

// Create a DB4S default browse list entry
func LogDB4SConnect(userAcc string, ipAddr string, userAgent string, downloadDate time.Time) error {
	if Conf.DB4S.Debug {
		log.Printf("User '%s' just connected with '%s' and generated the default browse list\n", userAcc, userAgent)
	}

	// If the user account isn't "public", then we look up the user id and store the info with the request
	userID := 0
	if userAcc != "public" {
		dbQuery := `
			SELECT user_id
			FROM users
			WHERE user_name = $1`

		err := pdb.QueryRow(dbQuery, userAcc).Scan(&userID)
		if err != nil {
			log.Printf("Looking up the user ID failed: %v\n", err)
			return err
		}
		if userID == 0 {
			// The username wasn't found in our system (!!!)
			return fmt.Errorf("The user wasn't found in our system!")
		}
	}

	// Store the high level connection info, so we can check for growth over time
	dbQuery := `
		INSERT INTO db4s_connects (user_id, ip_addr, user_agent, connect_date)
		VALUES ($1, $2, $3, $4)`
	commandTag, err := pdb.Exec(dbQuery, userID, ipAddr, userAgent, downloadDate)
	if err != nil {
		log.Printf("Storing record of DB4S connection failed: %v\n", err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%v) affected while storing DB4S connection record for user '%v'\n", numRows, userAcc)
	}
	return nil
}

// Create a download log entry
func LogDownload(dbOwner string, dbFolder string, dbName string, loggedInUser string, ipAddr string, serverSw string,
	userAgent string, downloadDate time.Time, sha string) error {
	// If the downloader isn't a logged in user, use a NULL value for that column
	var downloader pgx.NullString
	if loggedInUser != "" {
		downloader.String = loggedInUser
		downloader.Valid = true
	}

	// Store the download details
	dbQuery := `
		WITH d AS (
			SELECT db.db_id, db.folder, db.db_name
			FROM sqlite_databases AS db
			WHERE user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND db.folder = $2
				AND db.db_name = $3
		)
		INSERT INTO database_downloads (db_id, user_id, ip_addr, server_sw, user_agent, download_date, db_sha256)
		SELECT (SELECT db_id FROM d), (SELECT user_id FROM users WHERE lower(user_name) = lower($4)), $5, $6, $7, $8, $9`
	commandTag, err := pdb.Exec(dbQuery, dbOwner, dbFolder, dbName, downloader, ipAddr, serverSw, userAgent,
		downloadDate, sha)
	if err != nil {
		log.Printf("Storing record of download '%s%s%s', sha '%s' by '%s' failed: %v\n", dbOwner, dbFolder,
			dbName, sha, downloader, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%v) affected while storing download record for '%s%s%s'\n", numRows,
			dbOwner, dbFolder, dbName)
	}
	return nil
}

// Add memory allocation stats for the execution run of a user supplied SQLite query
func LogSQLiteQueryAfter(insertID, memUsed, memHighWater int64) (err error) {
	dbQuery := `
		UPDATE vis_query_runs
		SET memory_used = $2, memory_high_water = $3
		WHERE query_run_id = $1`
	commandTag, err := pdb.Exec(dbQuery, insertID, memUsed, memHighWater)
	if err != nil {
		log.Printf("Adding memory stats for SQLite query run '%d' failed: %v\n", insertID, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%v) affected while adding memory stats for SQLite query run '%d'\n",
			numRows, insertID)
	}
	return nil
}

// Log the basic info for a user supplied SQLite query
func LogSQLiteQueryBefore(dbOwner, dbFolder, dbName, loggedInUser, ipAddr, userAgent, query string) (int64, error) {
	// If the user isn't logged in, use a NULL value for that column
	var queryUser pgx.NullString
	if loggedInUser != "" {
		queryUser.String = loggedInUser
		queryUser.Valid = true
	}

	// Base64 encode the SQLite query string, just to be as safe as possible
	encodedQuery := base64.StdEncoding.EncodeToString([]byte(query))

	// Store the query details
	dbQuery := `
		WITH d AS (
			SELECT db.db_id, db.folder, db.db_name
			FROM sqlite_databases AS db
			WHERE user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND db.folder = $2
				AND db.db_name = $3
		)
		INSERT INTO vis_query_runs (db_id, user_id, ip_addr, user_agent, query_string)
		SELECT (SELECT db_id FROM d), (SELECT user_id FROM users WHERE lower(user_name) = lower($4)), $5, $6, $7
		RETURNING query_run_id`
	var insertID int64
	err := pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName, queryUser, ipAddr, userAgent, encodedQuery).Scan(&insertID)
	if err != nil {
		log.Printf("Storing record of user SQLite query '%v' on '%s%s%s' failed: %v\n", encodedQuery, dbOwner,
			dbFolder, dbName, err)
		return 0, err
	}
	return insertID, nil
}

// Create an upload log entry
func LogUpload(dbOwner string, dbFolder string, dbName string, loggedInUser string, ipAddr string, serverSw string,
	userAgent string, uploadDate time.Time, sha string) error {
	// If the uploader isn't a logged in user, use a NULL value for that column
	var uploader pgx.NullString
	if loggedInUser != "" {
		uploader.String = loggedInUser
		uploader.Valid = true
	}

	// Store the upload details
	dbQuery := `
		WITH d AS (
			SELECT db.db_id, db.folder, db.db_name
			FROM sqlite_databases AS db
			WHERE user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND db.folder = $2
				AND db.db_name = $3
		)
		INSERT INTO database_uploads (db_id, user_id, ip_addr, server_sw, user_agent, upload_date, db_sha256)
		SELECT (SELECT db_id FROM d), (SELECT user_id FROM users WHERE lower(user_name) = lower($4)), $5, $6, $7, $8, $9`
	commandTag, err := pdb.Exec(dbQuery, dbOwner, dbFolder, dbName, uploader, ipAddr, serverSw, userAgent,
		uploadDate, sha)
	if err != nil {
		log.Printf("Storing record of upload '%s%s%s', sha '%s' by '%s' failed: %v\n", dbOwner, dbFolder,
			dbName, sha, uploader, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%v) affected while storing upload record for '%s%s%s'\n", numRows,
			dbOwner, dbFolder, dbName)
	}
	return nil
}

// Return the Minio bucket and ID for a given database. dbOwner, dbFolder, & dbName are from owner/folder/database URL
// fragment, loggedInUser is the name for the currently logged in user, for access permission check.  Use an empty
// string ("") as the loggedInUser parameter if the true value isn't set or known.
// If the requested database doesn't exist, or the loggedInUser doesn't have access to it, then an error will be
// returned.
func MinioLocation(dbOwner string, dbFolder string, dbName string, commitID string, loggedInUser string) (minioBucket string,
	minioID string, lastModified time.Time, err error) {

	// If no commit was provided, we grab the default one
	if commitID == "" {
		commitID, err = DefaultCommit(dbOwner, dbFolder, dbName)
		if err != nil {
			return // Bucket and ID are still the initial default empty string
		}
	}

	// Retrieve the sha256 and last modified date for the requested commit's database file
	var dbQuery string
	dbQuery = `
		SELECT commit_list->$4::text->'tree'->'entries'->0->'sha256' AS sha256,
			commit_list->$4::text->'tree'->'entries'->0->'last_modified' AS last_modified
		FROM sqlite_databases AS db
		WHERE db.user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db.folder = $2
			AND db.db_name = $3
			AND db.is_deleted = false`

	// If the request is for another users database, it needs to be a public one
	if strings.ToLower(loggedInUser) != strings.ToLower(dbOwner) {
		dbQuery += `
				AND db.public = true`
	}

	var sha, mod string
	err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName, commitID).Scan(&sha, &mod)
	if err != nil {
		log.Printf("Error retrieving MinioID for '%s/%s' version '%v' by logged in user '%v': %v\n", dbOwner,
			dbName, commitID, loggedInUser, err)
		return // Bucket and ID are still the initial default empty string
	}

	if sha == "" {
		// The requested database doesn't exist, or the logged in user doesn't have access to it
		err = fmt.Errorf("The requested database wasn't found")
		return // Bucket and ID are still the initial default empty string
	}

	lastModified, err = time.Parse(time.RFC3339, mod)
	if err != nil {
		return // Bucket and ID are still the initial default empty string
	}

	minioBucket = sha[:MinioFolderChars]
	minioID = sha[MinioFolderChars:]
	return
}

// Adds an event entry to PostgreSQL
func NewEvent(details EventDetails) (err error) {
	dbQuery := `
		WITH d AS (
			SELECT db_id
			FROM sqlite_databases
			WHERE user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND folder = $2
				AND db_name = $3
				AND is_deleted = false
		)
		INSERT INTO events (db_id, event_type, event_data)
		VALUES ((SELECT db_id FROM d), $4, $5)`
	_, err = pdb.Exec(dbQuery, details.Owner, details.Folder, details.DBName, details.Type, details)
	if err != nil {
		return err
	}
	return
}

// Return the user's preference for maximum number of SQLite rows to display.
func PrefUserMaxRows(loggedInUser string) int {
	// Retrieve the user preference data
	dbQuery := `
		SELECT pref_max_rows
		FROM users
		WHERE user_id = (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1))`
	var maxRows int
	err := pdb.QueryRow(dbQuery, loggedInUser).Scan(&maxRows)
	if err != nil {
		log.Printf("Error retrieving user '%s' preference data: %v\n", loggedInUser, err)
		return DefaultNumDisplayRows // Use the default value
	}

	return maxRows
}

// Rename a SQLite database.
func RenameDatabase(userName string, dbFolder string, dbName string, newName string) error {
	// Save the database settings
	dbQuery := `
		UPDATE sqlite_databases
		SET db_name = $4
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND folder = $2
			AND db_name = $3`
	commandTag, err := pdb.Exec(dbQuery, userName, dbFolder, dbName, newName)
	if err != nil {
		log.Printf("Renaming database '%s%s%s' failed: %v\n", userName, dbFolder, dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		errMsg := fmt.Sprintf("Wrong number of rows affected (%v) when renaming '%s%s%s' to '%s%s%s'\n",
			numRows, userName, dbFolder, dbName, userName, dbFolder, newName)
		log.Printf(errMsg)
		return errors.New(errMsg)
	}

	// Log the rename
	log.Printf("Database renamed from '%s%s%s' to '%s%s%s'\n", userName, dbFolder, dbName, userName, dbFolder,
		newName)

	return nil
}

// Saves updated database settings to PostgreSQL.
func SaveDBSettings(userName string, dbFolder string, dbName string, oneLineDesc string, fullDesc string,
	defaultTable string, public bool, sourceURL string, defaultBranch string) error {
	// Check for values which should be NULL
	var nullable1LineDesc, nullableFullDesc, nullableSourceURL pgx.NullString
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
		SET one_line_description = $4, full_description = $5, default_table = $6, public = $7, source_url = $8,
			default_branch = $9
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND folder = $2
			AND db_name = $3`
	commandTag, err := pdb.Exec(SQLQuery, userName, dbFolder, dbName, nullable1LineDesc, nullableFullDesc, defaultTable,
		public, nullableSourceURL, defaultBranch)
	if err != nil {
		log.Printf("Updating description for database '%s%s%s' failed: %v\n", userName, dbFolder,
			dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		errMsg := fmt.Sprintf("Wrong number of rows affected (%v) when updating description for '%s%s%s'\n",
			numRows, userName, dbFolder, dbName)
		log.Printf(errMsg)
		return errors.New(errMsg)
	}

	// Invalidate the old memcached entry for the database
	err = InvalidateCacheEntry(userName, userName, dbFolder, dbName, "") // Empty string indicates "for all versions"
	if err != nil {
		// Something went wrong when invalidating memcached entries for the database
		log.Printf("Error when invalidating memcache entries: %s\n", err.Error())
		return err
	}

	return nil
}

// Sends status update emails to people watching databases
func SendEmails() {
	// Create Hectane email queue
	cfg := &queue.Config{
		Directory:              Conf.Event.EmailQueueDir,
		DisableSSLVerification: true,
	}
	q, err := queue.NewQueue(cfg)
	if err != nil {
		log.Printf("Couldn't start Hectane queue: %s", err.Error())
		return
	}
	log.Printf("Created Hectane email queue in '%s'.  Queue processing loop refreshes every %d seconds",
		Conf.Event.EmailQueueDir, Conf.Event.EmailQueueProcessingDelay)

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
		rows, err := pdb.Query(dbQuery)
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
			e := &email.Email{
				From:    "updates@dbhub.io",
				To:      []string{j.Address},
				Subject: j.Subject,
				Text:    j.Body,
			}
			msgs, err := e.Messages(q.Storage)
			if err != nil {
				log.Printf("Queuing email in Hectane failed: %v", err.Error())
				return // Abort, as we don't want to continuously resend the same emails
			}
			for _, m := range msgs {
				q.Deliver(m)
			}

			// Mark message as sent
			dbQuery := `
				UPDATE email_queue
				SET sent = true, sent_timestamp = now()
				WHERE email_id = $1`
			commandTag, err := pdb.Exec(dbQuery, j.ID)
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
		time.Sleep(Conf.Event.EmailQueueProcessingDelay * time.Second)
	}
}

// Stores a certificate for a given client.
func SetClientCert(newCert []byte, userName string) error {
	SQLQuery := `
		UPDATE users
		SET client_cert = $1
		WHERE lower(user_name) = lower($2)`
	commandTag, err := pdb.Exec(SQLQuery, newCert, userName)
	if err != nil {
		log.Printf("Updating client certificate for '%s' failed: %v\n", userName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		errMsg := fmt.Sprintf("Wrong number of rows affected (%v) when storing client cert for '%s'\n",
			numRows, userName)
		log.Printf(errMsg)
		return errors.New(errMsg)
	}

	return nil
}

// Sets the user's preference for maximum number of SQLite rows to display.
func SetUserPreferences(userName string, maxRows int, displayName string, email string) error {
	dbQuery := `
		UPDATE users
		SET pref_max_rows = $2, display_name = $3, email = $4
		WHERE lower(user_name) = lower($1)`
	commandTag, err := pdb.Exec(dbQuery, userName, maxRows, displayName, email)
	if err != nil {
		log.Printf("Updating user preferences failed for user '%s'. Error: '%v'\n", userName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong # of rows (%v) affected when updating user preferences. User: '%s'\n", numRows,
			userName)
	}
	return nil
}

// Retrieve the latest social stats for a given database.
func SocialStats(dbOwner string, dbFolder string, dbName string) (wa int, st int, fo int, err error) {

	// TODO: Implement caching of these stats

	// Retrieve latest star count
	dbQuery := `
		SELECT stars
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND folder = $2
			AND db_name = $3`
	err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&st)
	if err != nil {
		log.Printf("Error retrieving star count for '%s%s%s': %v\n", dbOwner, dbFolder, dbName, err)
		return -1, -1, -1, err
	}

	// Retrieve latest fork count
	dbQuery = `
		SELECT forks
		FROM sqlite_databases
		WHERE db_id = (
				SELECT root_database
				FROM sqlite_databases
				WHERE user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
					)
			AND folder = $2
			AND db_name = $3)`
	err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&fo)
	if err != nil {
		log.Printf("Error retrieving fork count for '%s%s%s': %v\n", dbOwner, dbFolder, dbName, err)
		return -1, -1, -1, err
	}

	// TODO: Implement watchers
	return 0, st, fo, nil
}

// Retrieve the list of outstanding status updates for a user
func StatusUpdates(loggedInUser string) (statusUpdates map[string][]StatusUpdateEntry, err error) {
	dbQuery := `
		SELECT status_updates
		FROM users
		WHERE user_name = $1`
	err = pdb.QueryRow(dbQuery, loggedInUser).Scan(&statusUpdates)
	if err != nil {
		log.Printf("Error retrieving status updates list for user '%s': %v", loggedInUser, err)
		return
	}
	return
}

// Periodically generates status updates (alert emails TBD) from the event queue
func StatusUpdatesLoop() {
	// Ensure a warning message is displayed on the console if the status update loop exits
	defer func() {
		log.Printf("WARN: Status update loop exited")
	}()

	// Log the start of the loop
	log.Printf("Status update processing loop started.  %d second refresh.", Conf.Event.Delay)

	// Start the endless status update processing loop
	var rows *pgx.Rows
	var err error
	type evEntry struct {
		dbID      int64
		details   EventDetails
		eType     EventType
		eventID   int64
		timeStamp time.Time
	}
	for {
		// Begin a transaction
		var tx *pgx.Tx
		tx, err = pdb.Begin()
		if err != nil {
			log.Printf("Couldn't begin database transaction for status update processing loop: %s", err.Error())
			continue
		}

		// Retrieve the list of outstanding events
		// NOTE - We gather the db_id here instead of dbOwner/dbFolder/dbName as it should be faster for PG to deal
		//        with when generating the watcher list
		dbQuery := `
			SELECT event_id, event_timestamp, db_id, event_type, event_data
			FROM events
			ORDER BY event_id ASC`
		rows, err = tx.Query(dbQuery)
		if err != nil {
			log.Printf("Generating status update event list failed: %v\n", err)
			tx.Rollback()
			continue
		}
		evList := make(map[int64]evEntry)
		for rows.Next() {
			var ev evEntry
			err = rows.Scan(&ev.eventID, &ev.timeStamp, &ev.dbID, &ev.eType, &ev.details)
			if err != nil {
				log.Printf("Error retrieving event list for status updates thread: %v\n", err)
				rows.Close()
				tx.Rollback()
				continue
			}
			evList[ev.eventID] = ev
		}
		rows.Close()

		// For each event, add a status update to the status_updates list for each watcher it's for
		for id, ev := range evList {
			// Retrieve the list of watchers for the database the event occurred on
			dbQuery := `
				SELECT user_id
				FROM watchers
				WHERE db_id = $1`
			rows, err = tx.Query(dbQuery, ev.dbID)
			if err != nil {
				log.Printf("Database query failed: %v\n", err)
				tx.Rollback()
				continue
			}
			var users []int64
			for rows.Next() {
				var user int64
				err = rows.Scan(&user)
				if err != nil {
					log.Printf("Error retrieving user list for status updates thread: %v\n", err)
					rows.Close()
					tx.Rollback()
					continue
				}
				users = append(users, user)
			}
			rows.Close()

			// For each watcher, add the new status update to their existing list
			// TODO: It might be better to store this list in Memcached instead of hitting the database like this
			for _, u := range users {
				// Retrieve the current status updates list for the user
				var eml pgx.NullString
				dbQuery := `
					SELECT user_name, email, status_updates
					FROM users
					WHERE user_id = $1`
				userEvents := make(map[string][]StatusUpdateEntry)
				var userName string
				err := tx.QueryRow(dbQuery, u).Scan(&userName, &eml, &userEvents)
				if err != nil {
					log.Printf("Database query failed: %v\n", err)
					tx.Rollback()
					continue
				}

				// * Add the new event to the users status updates list *

				// Group the status updates by database, and coalesce multiple updates for the same discussion or MR
				// into a single entry (keeping the most recent one of each)
				dbName := fmt.Sprintf("%s%s%s", ev.details.Owner, ev.details.Folder, ev.details.DBName)
				var a StatusUpdateEntry
				lst, ok := userEvents[dbName]
				if ev.details.Type == EVENT_NEW_DISCUSSION || ev.details.Type == EVENT_NEW_MERGE_REQUEST || ev.details.Type == EVENT_NEW_COMMENT {
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
				commandTag, err := tx.Exec(dbQuery, u, userEvents)
				if err != nil {
					log.Printf("Adding status update for database ID '%d' to user '%s' failed: %v", ev.dbID,
						u, err)
					tx.Rollback()
					continue
				}
				if numRows := commandTag.RowsAffected(); numRows != 1 {
					log.Printf("Wrong number of rows affected (%v) when adding status update for database ID "+
						"'%d' to user '%s'", numRows, ev.dbID, u)
					tx.Rollback()
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
				case EVENT_NEW_DISCUSSION:
					msg = fmt.Sprintf("A new discussion has been created for %s%s%s.\n\nVisit https://%s%s "+
						"for the details", ev.details.Owner, ev.details.Folder, ev.details.DBName, Conf.Web.ServerName,
						ev.details.URL)
					subj = fmt.Sprintf("DBHub.io: New discussion created on %s%s%s", ev.details.Owner,
						ev.details.Folder, ev.details.DBName)
				case EVENT_NEW_MERGE_REQUEST:
					msg = fmt.Sprintf("A new merge request has been created for %s%s%s.\n\nVisit https://%s%s "+
						"for the details", ev.details.Owner, ev.details.Folder, ev.details.DBName, Conf.Web.ServerName,
						ev.details.URL)
					subj = fmt.Sprintf("DBHub.io: New merge request created on %s%s%s", ev.details.Owner,
						ev.details.Folder, ev.details.DBName)
				case EVENT_NEW_COMMENT:
					msg = fmt.Sprintf("A new comment has been created for %s%s%s.\n\nVisit https://%s%s for "+
						"the details", ev.details.Owner, ev.details.Folder, ev.details.DBName, Conf.Web.ServerName,
						ev.details.URL)
					subj = fmt.Sprintf("DBHub.io: New comment on %s%s%s", ev.details.Owner, ev.details.Folder,
						ev.details.DBName)
				default:
					log.Printf("Unknown message type when creating email message")
				}
				if eml.Valid {
					// TODO: Check if the email is username@thisserver, which indicates a non-functional email address
					dbQuery = `
						INSERT INTO email_queue (mail_to, subject, body)
						VALUES ($1, $2, $3)`
					commandTag, err = tx.Exec(dbQuery, eml.String, subj, msg)
					if err != nil {
						log.Printf("Adding status update to email queue for user '%s' failed: %v", u, err)
						tx.Rollback()
						continue
					}
					if numRows := commandTag.RowsAffected(); numRows != 1 {
						log.Printf("Wrong number of rows affected (%v) when adding status update to email"+
							"queue for user '%s'", numRows, u)
						tx.Rollback()
						continue
					}
				}
			}

			// Remove the processed event from PG
			dbQuery = `
				DELETE FROM events
				WHERE event_id = $1`
			commandTag, err := tx.Exec(dbQuery, id)
			if err != nil {
				log.Printf("Removing event ID '%d' failed: %v", id, err)
				continue
			}
			if numRows := commandTag.RowsAffected(); numRows != 1 {
				log.Printf("Wrong number of rows affected (%v) when removing event ID '%d'", numRows, id)
				continue
			}
		}

		// Commit the transaction
		err = tx.Commit()
		if err != nil {
			log.Printf("Could not commit transaction when processing status updates: %v", err.Error())
			continue
		}

		// Wait before running the loop again
		time.Sleep(Conf.Event.Delay * time.Second)
	}
	return
}

// Updates the branches list for a database.
func StoreBranches(dbOwner string, dbFolder string, dbName string, branches map[string]BranchEntry) error {
	dbQuery := `
		UPDATE sqlite_databases
		SET branch_heads = $4, branches = $5
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
				)
			AND folder = $2
			AND db_name = $3`
	commandTag, err := pdb.Exec(dbQuery, dbOwner, dbFolder, dbName, branches, len(branches))
	if err != nil {
		log.Printf("Updating branch heads for database '%s%s%s' to '%v' failed: %v\n", dbOwner, dbFolder,
			dbName, branches, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf(
			"Wrong number of rows (%v) affected when updating branch heads for database '%s%s%s' to '%v'\n",
			numRows, dbOwner, dbFolder, dbName, branches)
	}
	return nil
}

// Adds a comment to a discussion.
func StoreComment(dbOwner string, dbFolder string, dbName string, commenter string, discID int, comText string,
	discClose bool, mrState MergeRequestState) error {
	// Begin a transaction
	tx, err := pdb.Begin()
	if err != nil {
		return err
	}
	// Set up an automatic transaction roll back if the function exits without committing
	defer tx.Rollback()

	// Get the current details for the discussion or MR
	var discCreator string
	var discState bool
	var discType int64
	var discTitle string
	dbQuery := `
		SELECT disc.open, u.user_name, disc.discussion_type, disc.title
		FROM discussions AS disc, users AS u
		WHERE disc.db_id = (
				SELECT db.db_id
				FROM sqlite_databases AS db
				WHERE db.user_id = (
						SELECT user_id
						FROM users
						WHERE lower(user_name) = lower($1)
					)
					AND folder = $2
					AND db_name = $3
			)
			AND disc.disc_id = $4
			AND disc.creator = u.user_id`
	err = tx.QueryRow(dbQuery, dbOwner, dbFolder, dbName, discID).Scan(&discState, &discCreator, &discType, &discTitle)
	if err != nil {
		log.Printf("Error retrieving current open state for '%s%s%s', discussion '%d': %v\n", dbOwner,
			dbFolder, dbName, discID, err)
		return err
	}

	// If the discussion is to be closed or reopened, ensure the person doing so is either the database owner or the
	// person who started the discussion
	if discClose == true {
		if (strings.ToLower(commenter) != strings.ToLower(dbOwner)) && (strings.ToLower(commenter) != strings.ToLower(discCreator)) {
			return errors.New("Not authorised")
		}
	}

	// If comment text was provided, insert it into the database
	var commandTag pgx.CommandTag
	var comID int64
	if comText != "" {
		dbQuery = `
			WITH d AS (
				SELECT db.db_id
				FROM sqlite_databases AS db
				WHERE db.user_id = (
						SELECT user_id
						FROM users
						WHERE lower(user_name) = lower($1)
					)
					AND folder = $2
					AND db_name = $3
			), int AS (
				SELECT internal_id AS int_id
				FROM discussions
				WHERE db_id = (SELECT db_id FROM d)
				AND disc_id = $5
			)
			INSERT INTO discussion_comments (db_id, disc_id, commenter, body, entry_type)
			SELECT (SELECT db_id FROM d), (SELECT int_id FROM int), (SELECT user_id FROM users WHERE lower(user_name) = lower($4)), $6, 'txt'
			RETURNING com_id`
		err = tx.QueryRow(dbQuery, dbOwner, dbFolder, dbName, commenter, discID, comText).Scan(&comID)
		if err != nil {
			log.Printf("Adding comment for database '%s%s%s', discussion '%d' failed: %v\n", dbOwner, dbFolder,
				dbName, discID, err)
			return err
		}
	}

	// If the discussion is to be closed or reopened, insert a close or reopen record as appropriate
	if discClose == true {
		var eventTxt, eventType string
		if discState {
			// Discussion is open, so a close event should be inserted
			eventTxt = "close"
			eventType = "cls"
		} else {
			// Discussion is closed, so a re-open event should be inserted
			eventTxt = "reopen"
			eventType = "rop"
		}

		// Insert the appropriate close or reopen record
		dbQuery = `
			WITH d AS (
				SELECT db.db_id
				FROM sqlite_databases AS db
				WHERE db.user_id = (
						SELECT user_id
						FROM users
						WHERE lower(user_name) = lower($1)
					)
					AND folder = $2
					AND db_name = $3
			), int AS (
				SELECT internal_id AS int_id
				FROM discussions
				WHERE db_id = (SELECT db_id FROM d)
				AND disc_id = $5
			)
			INSERT INTO discussion_comments (db_id, disc_id, commenter, body, entry_type)
			SELECT (SELECT db_id FROM d), (SELECT int_id FROM int), (SELECT user_id FROM users WHERE lower(user_name) = lower($4)), $6, $7`
		commandTag, err = tx.Exec(dbQuery, dbOwner, dbFolder, dbName, commenter, discID, eventTxt, eventType)
		if err != nil {
			log.Printf("Adding comment for database '%s%s%s', discussion '%d' failed: %v\n", dbOwner, dbFolder,
				dbName, discID, err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf(
				"Wrong number of rows (%v) affected when adding a comment to database '%s%s%s', discussion '%d'\n",
				numRows, dbOwner, dbFolder, dbName, discID)
		}
	}

	// Update the merge request state for MR's being closed
	if discClose == true && discType == MERGE_REQUEST {
		dbQuery = `
			UPDATE discussions
			SET mr_state = $5
			WHERE db_id = (
					SELECT db.db_id
					FROM sqlite_databases AS db
					WHERE db.user_id = (
							SELECT user_id
							FROM users
							WHERE lower(user_name) = lower($1)
						)
						AND folder = $2
						AND db_name = $3
				)
				AND disc_id = $4`
		commandTag, err = tx.Exec(dbQuery, dbOwner, dbFolder, dbName, discID, mrState)
		if err != nil {
			log.Printf("Updating MR state for database '%s%s%s', discussion '%d' failed: %v\n", dbOwner,
				dbFolder, dbName, discID, err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf(
				"Wrong number of rows (%v) affected when updating MR state for database '%s%s%s', discussion '%d'\n",
				numRows, dbOwner, dbFolder, dbName, discID)
		}
	}

	// Update the last_modified date for the parent discussion
	dbQuery = `
		UPDATE discussions
		SET last_modified = now()`
	if discClose == true {
		if discState {
			// Discussion is open, so set it to closed
			dbQuery += `, open = false`
		} else {
			// Discussion is closed, so set it to open
			dbQuery += `, open = true`
		}
	}
	if comText != "" {
		dbQuery += `, comment_count = comment_count + 1`
	}
	dbQuery += `
		WHERE db_id = (
				SELECT db.db_id
				FROM sqlite_databases AS db
				WHERE db.user_id = (
						SELECT user_id
						FROM users
						WHERE lower(user_name) = lower($1)
					)
					AND folder = $2
					AND db_name = $3
			)
			AND disc_id = $4`
	commandTag, err = tx.Exec(dbQuery, dbOwner, dbFolder, dbName, discID)
	if err != nil {
		log.Printf("Updating last modified date for database '%s%s%s', discussion '%d' failed: %v\n", dbOwner,
			dbFolder, dbName, discID, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf(
			"Wrong number of rows (%v) affected when updating last_modified date for database '%s%s%s', discussion '%d'\n",
			numRows, dbOwner, dbFolder, dbName, discID)
	}

	// Update the open discussion and MR counters for the database
	dbQuery = `
		WITH d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db
			WHERE db.user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND folder = $2
				AND db_name = $3
		)
		UPDATE sqlite_databases
		SET discussions = (
				SELECT count(disc.*)
				FROM discussions AS disc, d
				WHERE disc.db_id = d.db_id
					AND open = true
					AND discussion_type = 0
			),
			merge_requests = (
				SELECT count(disc.*)
				FROM discussions AS disc, d
				WHERE disc.db_id = d.db_id
					AND open = true
					AND discussion_type = 1
			)
		WHERE db_id = (SELECT db_id FROM d)`
	commandTag, err = tx.Exec(dbQuery, dbOwner, dbFolder, dbName)
	if err != nil {
		log.Printf("Updating discussion count for database '%s%s%s' failed: %v\n", dbOwner, dbFolder, dbName,
			err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf(
			"Wrong number of rows (%v) affected when updating discussion count for database '%s%s%s'\n",
			numRows, dbOwner, dbFolder, dbName)
	}

	// If comment text was provided, generate an event about the new comment
	if comText != "" {
		var commentURL string
		if discType == MERGE_REQUEST {
			commentURL = fmt.Sprintf("/merge/%s%s%s?id=%d#c%d", url.PathEscape(dbOwner), dbFolder,
				url.PathEscape(dbName), discID, comID)
		} else {
			commentURL = fmt.Sprintf("/discuss/%s%s%s?id=%d#c%d", url.PathEscape(dbOwner), dbFolder,
				url.PathEscape(dbName), discID, comID)
		}
		details := EventDetails{
			DBName:   dbName,
			DiscID:   discID,
			Folder:   dbFolder,
			Owner:    dbOwner,
			Type:     EVENT_NEW_COMMENT,
			Title:    discTitle,
			URL:      commentURL,
			UserName: commenter,
		}
		err = NewEvent(details)
		if err != nil {
			log.Printf("Error when creating a new event: %s\n", err.Error())
			return err
		}
	}

	// Commit the transaction
	err = tx.Commit()
	if err != nil {
		return err
	}

	return nil
}

// Updates the commit list for a database.
func StoreCommits(dbOwner string, dbFolder string, dbName string, commitList map[string]CommitEntry) error {
	dbQuery := `
		UPDATE sqlite_databases
		SET commit_list = $4, last_modified = now()
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
				)
			AND folder = $2
			AND db_name = $3`
	commandTag, err := pdb.Exec(dbQuery, dbOwner, dbFolder, dbName, commitList)
	if err != nil {
		log.Printf("Updating commit list for database '%s%s%s' failed: %v\n", dbOwner, dbFolder, dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf(
			"Wrong number of rows (%v) affected when updating commit list for database '%s%s%s'\n", numRows,
			dbOwner, dbFolder, dbName)
	}
	return nil
}

// Stores database details in PostgreSQL, and the database data itself in Minio.
func StoreDatabase(dbOwner string, dbFolder string, dbName string, branches map[string]BranchEntry, c CommitEntry,
	pub bool, buf *os.File, sha string, dbSize int64, oneLineDesc string, fullDesc string, createDefBranch bool,
	branchName string, sourceURL string) error {
	// Store the database file
	err := StoreDatabaseFile(buf, sha, dbSize)
	if err != nil {
		return err
	}

	// Check for values which should be NULL
	var nullable1LineDesc, nullableFullDesc pgx.NullString
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
	cMap := map[string]CommitEntry{c.ID: c}
	var commandTag pgx.CommandTag
	dbQuery := `
		WITH root AS (
			SELECT nextval('sqlite_databases_db_id_seq') AS val
		)
		INSERT INTO sqlite_databases (user_id, db_id, folder, db_name, public, one_line_description, full_description,
			branch_heads, root_database, commit_list`
	if sourceURL != "" {
		dbQuery += `, source_url`
	}
	dbQuery +=
		`)
		SELECT (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)), (SELECT val FROM root), $2, $3, $4, $5, $6, $8, (SELECT val FROM root), $7`
	if sourceURL != "" {
		dbQuery += `, $9`
	}
	dbQuery += `
		ON CONFLICT (user_id, folder, db_name)
			DO UPDATE
			SET commit_list = sqlite_databases.commit_list || $7,
				branch_heads = sqlite_databases.branch_heads || $8,
				last_modified = now()`
	if sourceURL != "" {
		dbQuery += `,
			source_url = $9`
		commandTag, err = pdb.Exec(dbQuery, dbOwner, dbFolder, dbName, pub, nullable1LineDesc, nullableFullDesc,
			cMap, branches, sourceURL)
	} else {
		commandTag, err = pdb.Exec(dbQuery, dbOwner, dbFolder, dbName, pub, nullable1LineDesc, nullableFullDesc,
			cMap, branches)
	}
	if err != nil {
		log.Printf("Storing database '%s%s%s' failed: %v\n", dbOwner, dbFolder, dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%v) affected while storing database '%s%s%s'\n", numRows, dbOwner,
			dbFolder, dbName)
	}

	if createDefBranch {
		err = StoreDefaultBranchName(dbOwner, dbFolder, dbName, branchName)
		if err != nil {
			log.Printf("Storing default branch '%s' name for '%s%s%s' failed: %v\n", branchName, dbOwner,
				dbFolder, dbName, err)
			return err
		}
	}
	return nil
}

// Stores the default branch name for a database.
func StoreDefaultBranchName(dbOwner string, folder string, dbName string, branchName string) error {
	dbQuery := `
		UPDATE sqlite_databases
		SET default_branch = $4
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
				)
			AND folder = $2
			AND db_name = $3`
	commandTag, err := pdb.Exec(dbQuery, dbOwner, folder, dbName, branchName)
	if err != nil {
		log.Printf("Changing default branch for database '%v' to '%v' failed: %v\n", dbName, branchName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%v) affected during update: database: %v, new branch name: '%v'\n",
			numRows, dbName, branchName)
	}
	return nil
}

// Stores the default table name for a database.
func StoreDefaultTableName(dbOwner string, folder string, dbName string, tableName string) error {
	var t pgx.NullString
	if tableName != "" {
		t.String = tableName
		t.Valid = true
	}
	dbQuery := `
		UPDATE sqlite_databases
		SET default_table = $4
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
				)
			AND folder = $2
			AND db_name = $3`
	commandTag, err := pdb.Exec(dbQuery, dbOwner, folder, dbName, t)
	if err != nil {
		log.Printf("Changing default table for database '%v' to '%v' failed: %v\n", dbName, tableName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%v) affected during update: database: %v, new table name: '%v'\n",
			numRows, dbName, tableName)
	}
	return nil
}

// Stores a new discussion for a database.
func StoreDiscussion(dbOwner string, dbFolder string, dbName string, loggedInUser string, title string, text string,
	discType DiscussionType, mr MergeRequestEntry) (newID int, err error) {

	// Begin a transaction
	tx, err := pdb.Begin()
	if err != nil {
		return
	}
	// Set up an automatic transaction roll back if the function exits without committing
	defer tx.Rollback()

	// Add the discussion details to PostgreSQL
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
				AND db.folder = $2
				AND db.db_name = $3
		), next_id AS (
			SELECT coalesce(max(disc.disc_id), 0) + 1 AS id
			FROM discussions AS disc, d
			WHERE disc.db_id = d.db_id
		)
		INSERT INTO discussions (db_id, disc_id, creator, title, description, open, discussion_type`
	if discType == MERGE_REQUEST {
		dbQuery += `, mr_source_db_id, mr_source_db_branch, mr_destination_branch, mr_commits`
	}
	dbQuery += `
			)
		SELECT (SELECT db_id FROM d),
			(SELECT id FROM next_id),
			(SELECT user_id FROM users WHERE lower(user_name) = lower($4)),
			$5,
			$6,
			true,
			$7`
	if discType == MERGE_REQUEST {
		dbQuery += `,(
			SELECT db_id
			FROM sqlite_databases
			WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($8))
			AND folder = $9
			AND db_name = $10
			AND is_deleted = false
		), $11, $12, $13`
	}
	dbQuery += `
		RETURNING (SELECT id FROM next_id)`
	if discType == MERGE_REQUEST {
		err = tx.QueryRow(dbQuery, dbOwner, dbFolder, dbName, loggedInUser, title, text, discType, mr.SourceOwner,
			mr.SourceFolder, mr.SourceDBName, mr.SourceBranch, mr.DestBranch, mr.Commits).Scan(&newID)
	} else {
		err = tx.QueryRow(dbQuery, dbOwner, dbFolder, dbName, loggedInUser, title, text, discType).Scan(&newID)
	}
	if err != nil {
		log.Printf("Adding new discussion or merge request '%s' for '%s%s%s' failed: %v\n", title, dbOwner,
			dbFolder, dbName, err)
		return
	}

	// Increment the discussion or merge request counter for the database
	dbQuery = `
		UPDATE sqlite_databases`
	if discType == DISCUSSION {
		dbQuery += `
			SET discussions = discussions + 1`
	} else {
		dbQuery += `
			SET merge_requests = merge_requests + 1`
	}
	dbQuery += `
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND folder = $2
			AND db_name = $3`
	commandTag, err := tx.Exec(dbQuery, dbOwner, dbFolder, dbName)
	if err != nil {
		log.Printf("Updating discussion counter for '%s%s%s' failed: %v\n", dbOwner, dbFolder, dbName, err)
		return
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%v) affected when updating discussion counter for '%s%s%s'\n",
			numRows, dbOwner, dbFolder, dbName)
	}

	// Commit the transaction
	err = tx.Commit()
	if err != nil {
		return
	}
	return
}

// Store a licence.
func StoreLicence(userName string, licenceName string, txt []byte, url string, orderNum int, fullName string,
	fileFormat string) error {
	// Store the licence in PostgreSQL
	sha := sha256.Sum256(txt)
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		INSERT INTO database_licences (user_id, friendly_name, lic_sha256, licence_text, licence_url, display_order,
			full_name, file_format)
		SELECT (SELECT user_id FROM u), $2, $3, $4, $5, $6, $7, $8
		ON CONFLICT (user_id, friendly_name)
			DO UPDATE
			SET friendly_name = $2,
				lic_sha256 = $3,
				licence_text = $4,
				licence_url = $5,
				user_id = (SELECT user_id FROM u),
				display_order = $6,
				full_name = $7,
				file_format = $8`
	commandTag, err := pdb.Exec(dbQuery, userName, licenceName, hex.EncodeToString(sha[:]), txt, url, orderNum,
		fullName, fileFormat)
	if err != nil {
		log.Printf("Inserting licence '%v' in database failed: %v\n", licenceName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%v) affected when storing licence '%v'\n", numRows, licenceName)
	}
	return nil
}

// Store the releases for a database.
func StoreReleases(dbOwner string, dbFolder string, dbName string, releases map[string]ReleaseEntry) error {
	dbQuery := `
		UPDATE sqlite_databases
		SET release_list = $4, release_count = $5
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND folder = $2
			AND db_name = $3`
	commandTag, err := pdb.Exec(dbQuery, dbOwner, dbFolder, dbName, releases, len(releases))
	if err != nil {
		log.Printf("Storing releases for database '%s%s%s' failed: %v\n", dbOwner, dbFolder, dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%v) affected when storing releases for database: '%s%s%s'\n", numRows,
			dbOwner, dbFolder, dbName)
	}
	return nil
}

// Store the status updates list for a user
func StoreStatusUpdates(userName string, statusUpdates map[string][]StatusUpdateEntry) error {
	dbQuery := `
		UPDATE users
		SET status_updates = $2
		WHERE user_name = $1`
	commandTag, err := pdb.Exec(dbQuery, userName, statusUpdates)
	if err != nil {
		log.Printf("Adding status update for user '%s' failed: %v", userName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows affected (%v) when storing status update for user '%s'", numRows,
			userName)
		return err
	}
	return nil
}

// Store the tags for a database.
func StoreTags(dbOwner string, dbFolder string, dbName string, tags map[string]TagEntry) error {
	dbQuery := `
		UPDATE sqlite_databases
		SET tag_list = $4, tags = $5
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND folder = $2
			AND db_name = $3`
	commandTag, err := pdb.Exec(dbQuery, dbOwner, dbFolder, dbName, tags, len(tags))
	if err != nil {
		log.Printf("Storing tags for database '%s%s%s' failed: %v\n", dbOwner, dbFolder, dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%v) affected when storing tags for database: '%s%s%s'\n", numRows,
			dbOwner, dbFolder, dbName)
	}
	return nil
}

// Toggle on or off the starring of a database by a user.
func ToggleDBStar(loggedInUser string, dbOwner string, dbFolder string, dbName string) error {
	// Check if the database is already starred
	starred, err := CheckDBStarred(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		return err
	}

	// Get the ID number of the database
	dbID, err := databaseID(dbOwner, dbFolder, dbName)
	if err != nil {
		return err
	}

	// Add or remove the star
	if !starred {
		// Star the database
		insertQuery := `
			WITH u AS (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($2)
			)
			INSERT INTO database_stars (db_id, user_id)
			SELECT $1, u.user_id
			FROM u`
		commandTag, err := pdb.Exec(insertQuery, dbID, loggedInUser)
		if err != nil {
			log.Printf("Adding star to database failed. Database ID: '%v' Username: '%s' Error '%v'\n",
				dbID, loggedInUser, err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf("Wrong # of rows affected (%v) when starring database ID: '%v' Username: '%s'\n",
				numRows, dbID, loggedInUser)
		}
	} else {
		// Unstar the database
		deleteQuery := `
		DELETE FROM database_stars
		WHERE db_id = $1
			AND user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($2)
			)`
		commandTag, err := pdb.Exec(deleteQuery, dbID, loggedInUser)
		if err != nil {
			log.Printf("Removing star from database failed. Database ID: '%v' Username: '%s' Error: '%v'\n",
				dbID, loggedInUser, err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf("Wrong # of rows (%v) affected when unstarring database ID: '%v' Username: '%s'\n",
				numRows, dbID, loggedInUser)
		}
	}

	// Refresh the main database table with the updated star count
	updateQuery := `
		UPDATE sqlite_databases
		SET stars = (
			SELECT count(db_id)
			FROM database_stars
			WHERE db_id = $1
		) WHERE db_id = $1`
	commandTag, err := pdb.Exec(updateQuery, dbID)
	if err != nil {
		log.Printf("Updating star count in database failed: %v\n", err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong # of rows affected (%v) when updating star count. Database ID: '%v'\n", numRows, dbID)
	}
	return nil
}

// Toggle on or off the watching of a database by a user.
func ToggleDBWatch(loggedInUser string, dbOwner string, dbFolder string, dbName string) error {
	// Check if the database is already being watched
	watched, err := CheckDBWatched(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		return err
	}

	// Add or remove the user from the watchers list
	if !watched {
		// Watch the database
		insertQuery := `
			WITH u AS (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($4)
			), d AS (
				SELECT db_id
				FROM sqlite_databases
				WHERE user_id = (
						SELECT user_id
						FROM users
						WHERE lower(user_name) = lower($1)
					)
					AND folder = $2
					AND db_name = $3
					AND is_deleted = false
			)
			INSERT INTO watchers (db_id, user_id)
			SELECT d.db_id, u.user_id
			FROM d, u`
		commandTag, err := pdb.Exec(insertQuery, dbOwner, dbFolder, dbName, loggedInUser)
		if err != nil {
			log.Printf("Adding '%s' to watchers list for database '%s%s%s' failed: Error '%v'\n", loggedInUser,
				dbOwner, dbFolder, dbName, err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf("Wrong # of rows affected (%v) when adding '%s' to watchers list for database '%s%s%s'",
				numRows, loggedInUser, dbOwner, dbFolder, dbName)
		}
	} else {
		// Unwatch the database
		deleteQuery := `
		DELETE FROM watchers
		WHERE db_id = (
				SELECT db_id
				FROM sqlite_databases
				WHERE user_id = (
						SELECT user_id
						FROM users
						WHERE lower(user_name) = lower($1)
					)
					AND folder = $2
					AND db_name = $3
					AND is_deleted = false
			)
			AND user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($4)
			)`
		commandTag, err := pdb.Exec(deleteQuery, dbOwner, dbFolder, dbName, loggedInUser)
		if err != nil {
			log.Printf("Removing '%s' from watchers list for database '%s%s%s' failed: Error '%v'\n",
				loggedInUser, dbOwner, dbFolder, dbName, err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf("Wrong # of rows affected (%v) when removing '%s' from watchers list for database '%s%s%s'",
				numRows, loggedInUser, dbOwner, dbFolder, dbName)
		}
	}

	// Refresh the main database table with the updated watchers count
	updateQuery := `
		WITH d AS (
				SELECT db_id
				FROM sqlite_databases
				WHERE user_id = (
						SELECT user_id
						FROM users
						WHERE lower(user_name) = lower($1)
					)
					AND folder = $2
					AND db_name = $3
					AND is_deleted = false
		)
		UPDATE sqlite_databases
		SET watchers = (
			SELECT count(db_id)
			FROM watchers
			WHERE db_id = (SELECT db_id FROM d)
		) WHERE db_id = (SELECT db_id FROM d)`
	commandTag, err := pdb.Exec(updateQuery, dbOwner, dbFolder, dbName)
	if err != nil {
		log.Printf("Updating watchers count for '%s%s%s' failed: %v", dbOwner, dbFolder, dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong # of rows affected (%v) when updating watchers count for '%s%s%s'", numRows, dbOwner,
			dbFolder, dbName)
	}
	return nil
}

// Updates the Avatar URL for a user.
func UpdateAvatarURL(userName string, avatarURL string) error {
	dbQuery := `
		UPDATE users
		SET avatar_url = $2
		WHERE lower(user_name) = lower($1)`
	commandTag, err := pdb.Exec(dbQuery, userName, avatarURL)
	if err != nil {
		log.Printf("Updating avatar URL failed for user '%s'. Error: '%v'\n", userName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong # of rows (%v) affected when updating avatar URL. User: '%s'\n", numRows,
			userName)
	}
	return nil
}

// Updates the contributors count for a database.
func UpdateContributorsCount(dbOwner string, dbFolder, dbName string) error {
	// Get the commit list for the database
	commitList, err := GetCommitList(dbOwner, dbFolder, dbName)
	if err != nil {
		return err
	}

	// Work out the new contributor count
	d := map[string]struct{}{}
	for _, k := range commitList {
		d[k.AuthorEmail] = struct{}{}
	}
	n := len(d)

	// Store the new contributor count in the database
	dbQuery := `
		UPDATE sqlite_databases
		SET contributors = $4
			WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
				AND folder = $2
				AND db_name = $3`
	commandTag, err := pdb.Exec(dbQuery, dbOwner, dbFolder, dbName, n)
	if err != nil {
		log.Printf("Updating contributor count in database '%s%s%s' failed: %v\n", dbOwner, dbFolder, dbName,
			err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong # of rows affected (%v) when updating contributor count for database '%s%s%s'\n",
			numRows, dbOwner, dbFolder, dbName)
	}
	return nil
}

// Updates the text for a comment
func UpdateComment(dbOwner string, dbFolder string, dbName string, loggedInUser string, discID int, comID int, newText string) error {
	// Begin a transaction
	tx, err := pdb.Begin()
	if err != nil {
		return err
	}
	// Set up an automatic transaction roll back if the function exits without committing
	defer tx.Rollback()

	// Retrieve the username of whoever created the comment
	var comCreator string
	dbQuery := `
		WITH d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db
			WHERE db.user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND folder = $2
				AND db_name = $3
		), int AS (
			SELECT internal_id AS int_id
			FROM discussions
			WHERE db_id = (SELECT db_id FROM d)
			AND disc_id = $4
		)
		SELECT u.user_name
		FROM discussion_comments AS com, users AS u
		WHERE com.db_id = (SELECT db_id FROM d)
			AND com.disc_id = (SELECT int_id FROM int)
			AND com.com_id = $5
			AND com.commenter = u.user_id`
	err = tx.QueryRow(dbQuery, dbOwner, dbFolder, dbName, discID, comID).Scan(&comCreator)
	if err != nil {
		log.Printf("Error retrieving name of comment creator for '%s%s%s', discussion '%d', comment '%d': %v\n",
			dbOwner, dbFolder, dbName, discID, comID, err)
		return err
	}

	// Ensure only the database owner or comment creator can update the comment
	if (strings.ToLower(loggedInUser) != strings.ToLower(dbOwner)) && (strings.ToLower(loggedInUser) != strings.ToLower(comCreator)) {
		return errors.New("Not authorised")
	}

	// Update the comment body
	dbQuery = `
		WITH d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db
			WHERE db.user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND folder = $2
				AND db_name = $3
		), int AS (
			SELECT internal_id AS int_id
			FROM discussions
			WHERE db_id = (SELECT db_id FROM d)
			AND disc_id = $4
		)
		UPDATE discussion_comments AS com
		SET body = $6
		WHERE com.db_id = (SELECT db_id FROM d)
			AND com.disc_id = (SELECT int_id FROM int)
			AND com.com_id = $5`
	commandTag, err := tx.Exec(dbQuery, dbOwner, dbFolder, dbName, discID, comID, newText)
	if err != nil {
		log.Printf("Updating comment for database '%s%s%s', discussion '%d', comment '%d' failed: %v\n",
			dbOwner, dbFolder, dbName, discID, comID, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf(
			"Wrong number of rows (%v) affected when updating comment for database '%s%s%s', discussion '%d', comment '%d'\n",
			numRows, dbOwner, dbFolder, dbName, discID, comID)
	}

	// Commit the transaction
	err = tx.Commit()
	if err != nil {
		return err
	}
	return nil
}

// Updates the text for a discussion
func UpdateDiscussion(dbOwner string, dbFolder string, dbName string, loggedInUser string, discID int, newTitle string, newText string) error {
	// Begin a transaction
	tx, err := pdb.Begin()
	if err != nil {
		return err
	}
	// Set up an automatic transaction roll back if the function exits without committing
	defer tx.Rollback()

	// Retrieve the name of the discussion creator
	var discCreator string
	dbQuery := `
		SELECT u.user_name
		FROM discussions AS disc, users AS u
		WHERE disc.db_id = (
				SELECT db.db_id
				FROM sqlite_databases AS db
				WHERE db.user_id = (
						SELECT user_id
						FROM users
						WHERE lower(user_name) = lower($1)
					)
					AND folder = $2
					AND db_name = $3
			)
			AND disc.disc_id = $4
			AND disc.creator = u.user_id`
	err = tx.QueryRow(dbQuery, dbOwner, dbFolder, dbName, discID).Scan(&discCreator)
	if err != nil {
		log.Printf("Error retrieving name of discussion creator for '%s%s%s', discussion '%d': %v\n",
			dbOwner, dbFolder, dbName, discID, err)
		return err
	}

	// Ensure only the database owner or discussion starter can update the discussion
	if (strings.ToLower(loggedInUser) != strings.ToLower(dbOwner)) && (strings.ToLower(loggedInUser) != strings.ToLower(discCreator)) {
		return errors.New("Not authorised")
	}

	// Update the discussion body
	dbQuery = `
		WITH d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db
			WHERE db.user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND folder = $2
				AND db_name = $3
		)
		UPDATE discussions AS disc
		SET title = $5, description = $6, last_modified = now()
		WHERE disc.db_id = (SELECT db_id FROM d)
			AND disc.disc_id = $4`
	commandTag, err := tx.Exec(dbQuery, dbOwner, dbFolder, dbName, discID, newTitle, newText)
	if err != nil {
		log.Printf("Updating discussion for database '%s%s%s', discussion '%d' failed: %v\n", dbOwner,
			dbFolder, dbName, discID, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf(
			"Wrong number of rows (%v) affected when updating discussion for database '%s%s%s', discussion '%d'\n",
			numRows, dbOwner, dbFolder, dbName, discID)
	}

	// Commit the transaction
	err = tx.Commit()
	if err != nil {
		return err
	}
	return nil
}

// Updates the commit list for a Merge Request
func UpdateMergeRequestCommits(dbOwner string, dbFolder string, dbName string, discID int, mrCommits []CommitEntry) (err error) {
	dbQuery := `
		WITH d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db
			WHERE db.user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND folder = $2
				AND db_name = $3
		)
		UPDATE discussions AS disc
		SET mr_commits = $5
		WHERE disc.db_id = (SELECT db_id FROM d)
			AND disc.disc_id = $4`
	commandTag, err := pdb.Exec(dbQuery, dbOwner, dbFolder, dbName, discID, mrCommits)
	if err != nil {
		log.Printf("Updating commit list for database '%s%s%s', MR '%d' failed: %v\n", dbOwner,
			dbFolder, dbName, discID, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf(
			"Wrong number of rows (%v) affected when updating commit list for database '%s%s%s', MR '%d'\n",
			numRows, dbOwner, dbFolder, dbName, discID)
	}
	return nil
}

// Returns details for a user.
func User(userName string) (user UserDetails, err error) {
	dbQuery := `
		SELECT user_name, display_name, email, avatar_url, password_hash, date_joined, client_cert
		FROM users
		WHERE lower(user_name) = lower($1)`
	var av, dn, em pgx.NullString
	err = pdb.QueryRow(dbQuery, userName).Scan(&user.Username, &dn, &em, &av, &user.PHash, &user.DateJoined,
		&user.ClientCert)
	if err != nil {
		if err == pgx.ErrNoRows {
			// The error was just "no such user found"
			return user, nil
		}

		// A real occurred
		log.Printf("Error retrieving details for user '%s' from database: %v\n", userName, err)
		return user, nil
	}

	// Return the display name and email values (if not empty)
	if dn.Valid {
		user.DisplayName = dn.String
	}
	if em.Valid {
		user.Email = em.String
	}

	// Determine an appropriate URL to the users's profile pic
	if av.Valid {
		user.AvatarURL = av.String
	} else {
		// No avatar URL is presently stored, so default to a gravatar based on users email (if known)
		if user.Email != "" {
			picHash := md5.Sum([]byte(user.Email))
			user.AvatarURL = fmt.Sprintf("https://www.gravatar.com/avatar/%x?d=identicon", picHash)
		}
	}

	return user, nil
}

// Returns the list of databases for a user.
func UserDBs(userName string, public AccessType) (list []DBInfo, err error) {
	// Construct SQL query for retrieving the requested database list
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), default_commits AS (
			SELECT DISTINCT ON (db.db_name) db_name, db.db_id, db.branch_heads->db.default_branch->>'commit' AS id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
		), dbs AS (
			SELECT DISTINCT ON (db.db_name) db.db_name, db.folder, db.date_created, db.last_modified, db.public,
				db.watchers, db.stars, db.discussions, db.merge_requests, db.branches, db.release_count, db.tags,
				db.contributors, db.one_line_description, default_commits.id,
				db.commit_list->default_commits.id->'tree'->'entries'->0, db.source_url, db.default_branch,
				db.download_count, db.page_views
			FROM sqlite_databases AS db, default_commits
			WHERE db.db_id = default_commits.db_id
				AND db.is_deleted = false`
	switch public {
	case DB_PUBLIC:
		// Only public databases
		dbQuery += ` AND db.public = true`
	case DB_PRIVATE:
		// Only private databases
		dbQuery += ` AND db.public = false`
	case DB_BOTH:
		// Both public and private, so no need to add a query clause
	default:
		// This clause shouldn't ever be reached
		return nil, fmt.Errorf("Incorrect 'public' value '%v' passed to UserDBs() function.", public)
	}
	dbQuery += `
		)
		SELECT *
		FROM dbs
		ORDER BY last_modified DESC`
	rows, err := pdb.Query(dbQuery, userName)
	if err != nil {
		log.Printf("Getting list of databases for user failed: %v\n", err)
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var defBranch, desc, source pgx.NullString
		var oneRow DBInfo
		err = rows.Scan(&oneRow.Database, &oneRow.Folder, &oneRow.DateCreated, &oneRow.RepoModified, &oneRow.Public,
			&oneRow.Watchers, &oneRow.Stars, &oneRow.Discussions, &oneRow.MRs, &oneRow.Branches,
			&oneRow.Releases, &oneRow.Tags, &oneRow.Contributors, &desc, &oneRow.CommitID, &oneRow.DBEntry, &source,
			&defBranch, &oneRow.Downloads, &oneRow.Views)
		if err != nil {
			log.Printf("Error retrieving database list for user: %v\n", err)
			return nil, err
		}
		if defBranch.Valid {
			oneRow.DefaultBranch = defBranch.String
		}
		if desc.Valid {
			oneRow.OneLineDesc = desc.String
		}
		if source.Valid {
			oneRow.SourceURL = source.String
		}
		oneRow.LastModified = oneRow.DBEntry.LastModified
		oneRow.Size = oneRow.DBEntry.Size
		oneRow.SHA256 = oneRow.DBEntry.Sha256

		// Work out the licence name and url for the database entry
		licSHA := oneRow.DBEntry.LicenceSHA
		if licSHA != "" {
			oneRow.Licence, oneRow.LicenceURL, err = GetLicenceInfoFromSha256(userName, licSHA)
			if err != nil {
				return nil, err
			}
		} else {
			oneRow.Licence = "Not specified"
		}
		list = append(list, oneRow)
	}

	// Get fork count for each of the databases
	for i, j := range list {
		// Retrieve latest fork count
		dbQuery = `
			WITH u AS (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			SELECT forks
			FROM sqlite_databases, u
			WHERE db_id = (
				SELECT root_database
				FROM sqlite_databases
				WHERE user_id = u.user_id
					AND folder = $2
					AND db_name = $3)`
		err = pdb.QueryRow(dbQuery, userName, j.Folder, j.Database).Scan(&list[i].Forks)
		if err != nil {
			log.Printf("Error retrieving fork count for '%s%s%s': %v\n", userName, j.Folder,
				j.Database, err)
			return nil, err
		}
	}

	return list, nil
}

// Returns the username for a given Auth0 ID.
func UserNameFromAuth0ID(auth0id string) (string, error) {
	// Query the database for a username matching the given Auth0 ID
	dbQuery := `
		SELECT user_name
		FROM users
		WHERE auth0_id = $1`
	var userName string
	err := pdb.QueryRow(dbQuery, auth0id).Scan(&userName)
	if err != nil {
		if err == pgx.ErrNoRows {
			// No matching user for the given Auth0 ID
			return "", nil
		}

		// A real occurred
		log.Printf("Error looking up username in database: %v\n", err)
		return "", nil
	}

	return userName, nil
}

// Returns the list of databases starred by a user.
func UserStarredDBs(userName string) (list []DBEntry, err error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		),
		stars AS (
			SELECT st.db_id, st.date_starred
			FROM database_stars AS st, u
			WHERE st.user_id = u.user_id
		),
		db_users AS (
			SELECT db.user_id, db.folder, db.db_name, stars.date_starred
			FROM sqlite_databases AS db, stars
			WHERE db.db_id = stars.db_id
		)
		SELECT users.user_name, db_users.folder, db_users.db_name, db_users.date_starred
		FROM users, db_users
		WHERE users.user_id = db_users.user_id
		ORDER BY date_starred DESC`
	rows, err := pdb.Query(dbQuery, userName)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var oneRow DBEntry
		err = rows.Scan(&oneRow.Owner, &oneRow.Folder, &oneRow.DBName, &oneRow.DateEntry)
		if err != nil {
			log.Printf("Error retrieving stars list for user: %v\n", err)
			return nil, err
		}
		list = append(list, oneRow)
	}

	return list, nil
}

// Returns the list of users who starred a database.
func UsersStarredDB(dbOwner string, dbFolder string, dbName string) (list []DBEntry, err error) {
	dbQuery := `
		WITH star_users AS (
			SELECT user_id, date_starred
			FROM database_stars
			WHERE db_id = (
				SELECT db_id
				FROM sqlite_databases
				WHERE user_id = (
						SELECT user_id
						FROM users
						WHERE lower(user_name) = lower($1)
					)
					AND folder = $2
					AND db_name = $3
					AND is_deleted = false
				)
		)
		SELECT users.user_name, users.display_name, star_users.date_starred
		FROM users, star_users
		WHERE users.user_id = star_users.user_id
		ORDER BY star_users.date_starred DESC`
	rows, err := pdb.Query(dbQuery, dbOwner, dbFolder, dbName)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var oneRow DBEntry
		var dn pgx.NullString
		err = rows.Scan(&oneRow.Owner, &dn, &oneRow.DateEntry)
		if err != nil {
			log.Printf("Error retrieving list of stars for %s/%s: %v\n", dbOwner, dbName, err)
			return nil, err
		}

		// If the user hasn't filled out their display name, use their username instead
		if dn.Valid {
			oneRow.OwnerDisplayName = dn.String
		} else {
			oneRow.OwnerDisplayName = oneRow.Owner
		}
		list = append(list, oneRow)
	}
	return list, nil
}

// Returns the list of users watching a database.
func UsersWatchingDB(dbOwner string, dbFolder string, dbName string) (list []DBEntry, err error) {
	dbQuery := `
		WITH lst AS (
			SELECT user_id, date_watched
			FROM watchers
			WHERE db_id = (
				SELECT db_id
				FROM sqlite_databases
				WHERE user_id = (
						SELECT user_id
						FROM users
						WHERE lower(user_name) = lower($1)
					)
					AND folder = $2
					AND db_name = $3
					AND is_deleted = false
				)
		)
		SELECT users.user_name, users.display_name, lst.date_watched
		FROM users, lst
		WHERE users.user_id = lst.user_id
		ORDER BY lst.date_watched DESC`
	rows, err := pdb.Query(dbQuery, dbOwner, dbFolder, dbName)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var oneRow DBEntry
		var dn pgx.NullString
		err = rows.Scan(&oneRow.Owner, &dn, &oneRow.DateEntry)
		if err != nil {
			log.Printf("Error retrieving list of watchers for %s%s%s: %v\n", dbOwner, dbFolder, dbName, err)
			return nil, err
		}

		// If the user hasn't filled out their display name, use their username instead
		if dn.Valid {
			oneRow.OwnerDisplayName = dn.String
		} else {
			oneRow.OwnerDisplayName = oneRow.Owner
		}
		list = append(list, oneRow)
	}
	return list, nil
}

// Returns the list of databases watched by a user.
func UserWatchingDBs(userName string) (list []DBEntry, err error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		),
		watching AS (
			SELECT w.db_id, w.date_watched
			FROM watchers AS w, u
			WHERE w.user_id = u.user_id
		),
		db_users AS (
			SELECT db.user_id, db.folder, db.db_name, watching.date_watched
			FROM sqlite_databases AS db, watching
			WHERE db.db_id = watching.db_id
		)
		SELECT users.user_name, db_users.folder, db_users.db_name, db_users.date_watched
		FROM users, db_users
		WHERE users.user_id = db_users.user_id
		ORDER BY date_watched DESC`
	rows, err := pdb.Query(dbQuery, userName)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var oneRow DBEntry
		err = rows.Scan(&oneRow.Owner, &oneRow.Folder, &oneRow.DBName, &oneRow.DateEntry)
		if err != nil {
			log.Printf("Error retrieving database watch list for user: %v\n", err)
			return nil, err
		}
		list = append(list, oneRow)
	}

	return list, nil
}

// Returns the view counter for a specific database
func ViewCount(dbOwner string, dbFolder string, dbName string) (viewCount int, err error) {
	dbQuery := `
		SELECT page_views
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND folder = $2
			AND db_name = $3`
	err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&viewCount)
	if err != nil {
		log.Printf("Retrieving view count for '%s%s%s' failed: %v\n", dbOwner, dbFolder, dbName, err)
		return 0, err
	}
	return
}

// Saves visualisation result data for later retrieval
func VisualisationSaveData(dbOwner string, dbFolder string, dbName string, commitID string, hash string, visData []VisRowV1) (err error) {
	var commandTag pgx.CommandTag
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
				AND folder = $2
				AND db_name = $3
		)
		INSERT INTO vis_result_cache (user_id, db_id, commit_id, hash, results)
		SELECT (SELECT user_id FROM u), (SELECT db_id FROM d), $4, $5, $6
		ON CONFLICT (db_id, user_id, commit_id, hash)
			DO UPDATE
			SET results = $6`
	commandTag, err = pdb.Exec(dbQuery, dbOwner, dbFolder, dbName, commitID, hash, visData)
	if err != nil {
		log.Printf("Saving visualisation data for database '%s%s%s', commit '%s', hash '%s' failed: %v\n", dbOwner, dbFolder, dbName, commitID, hash, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%v) affected while saving visualisation data for database '%s%s%s', commit '%s', hash '%s'\n", numRows, dbOwner, dbFolder, dbName, commitID, hash)
	}
	return
}

// Saves a set of visualisation parameters for later retrieval
func VisualisationSaveParams(dbOwner string, dbFolder string, dbName string, visName string, visParams VisParamsV2) (err error) {
	var commandTag pgx.CommandTag
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
				AND folder = $2
				AND db_name = $3
		)
		INSERT INTO vis_params (user_id, db_id, name, parameters)
		SELECT (SELECT user_id FROM u), (SELECT db_id FROM d), $4, $5
		ON CONFLICT (db_id, user_id, name)
			DO UPDATE
			SET parameters = $5`
	commandTag, err = pdb.Exec(dbQuery, dbOwner, dbFolder, dbName, visName, visParams)
	if err != nil {
		log.Printf("Saving visualisation '%s' for database '%s%s%s' failed: %v\n", visName, dbOwner, dbFolder, dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%v) affected while saving visualisation '%s' for database '%s%s%s'\n", numRows, visName, dbOwner, dbFolder, dbName)
	}
	return
}
