package common

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx"
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
	_, err := pdb.Exec(dbQuery, RandomString(16), "default", "", RandomString(16), "",
		"Default system user")
	if err != nil {
		// For now, don't bother logging a failure here.  This *might* need changing later on
		//log.Printf("Adding default user to database failed: %v\n", err)
		return err
	}
	return nil
}

// Add a user to the system.
func AddUser(auth0ID string, userName string, password string, email string, displayName string) error {
	// Hash the user's password
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Failed to hash user password. User: '%v', error: %v.\n", userName, err)
		return err
	}

	// Generate a unique bucket name for the user
	var bucket string
	newBucket := true
	for newBucket == true {
		bucket = RandomString(16) + ".bkt"
		newBucket, err = MinioBucketExists(bucket) // Drops out of the loop when the name hasn't been used yet
		if err != nil {
			return err
		}
	}

	// Generate a new HTTPS client certificate for the user
	cert, err := GenerateClientCert(userName, 14) // 14 days validity while developing
	if err != nil {
		log.Printf("Error when generating client certificate for '%s': %v\n", userName, err)
		return err
	}

	// If the displayName variable is an empty string, we insert a NULL instead
	var dn pgx.NullString
	if displayName == "" {
		dn.Valid = false
	} else {
		dn.String = displayName
		dn.Valid = true
	}

	// Add the new user to the database
	insertQuery := `
		INSERT INTO users (auth0_id, user_name, email, password_hash, client_cert, display_name)
		VALUES ($1, $2, $3, $4, $5, $6)`
	commandTag, err := pdb.Exec(insertQuery, auth0ID, userName, email, hash, cert, dn)
	if err != nil {
		log.Printf("Adding user to database failed: %v\n", err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows affected when creating user: %v, username: %v\n", numRows, userName)
	}

	// Create a new bucket for the user in Minio
	err = CreateMinioBucket(bucket)
	if err != nil {
		log.Printf("Error creating new bucket: %v\n", err)
		return err
	}

	// Log the user registration
	log.Printf("User registered: '%s' Email: '%s'\n", userName, email)

	return nil
}

// Check if a database exists
// If an error occurred, the true/false value should be ignored, as only the error value is valid.
func CheckDBExists(dbOwner string, dbFolder string, dbName string) (bool, error) {
	dbQuery := `
		SELECT count(db_id)
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = $1
			)
			AND folder = $2
			AND db_name = $3
			AND is_deleted = false`
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

// Check if a database has been starred by a given user.  The boolean return value is only valid when err is nil.
func CheckDBStarred(loggedInUser string, dbOwner string, dbFolder string, dbName string) (bool, error) {
	dbQuery := `
		SELECT count(db_id)
		FROM database_stars
		WHERE database_stars.user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = $1
			)
			AND database_stars.db_id = (
					SELECT db_id
					FROM sqlite_databases
					WHERE user_id = (
							SELECT user_id
							FROM users
							WHERE user_name = $2
						)
						AND folder = $3
						AND db_name = $4
						AND is_deleted = false)`
	var starCount int
	err := pdb.QueryRow(dbQuery, loggedInUser, dbOwner, dbFolder, dbName).Scan(&starCount)
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

// Check if a user has access to a database.
// Returns true if it's accessible to them, false if not.  If err returns as non-nil, the true/false value isn't valid.
func CheckUserDBAccess(dbOwner string, dbFolder string, dbName string, loggedInUser string) (bool, error) {
	dbQuery := `
		SELECT count(*)
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = $1
			)
			AND folder = $2
			AND db_name = $3
			AND is_deleted = false`
	if dbOwner != loggedInUser {
		dbQuery += ` AND public = true `
	}
	var numRows int
	err := pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&numRows)
	if err != nil {
		if err == pgx.ErrNoRows {
			// The requested database version isn't available to the given user
			return false, nil
		}
		log.Printf("Error when checking user's access to database '%s%s%s'. User: '%s' Error: %v\n",
			dbOwner, dbFolder, dbName, loggedInUser, err.Error())
		return false, err
	}

	// A row was returned, so the requested database IS available to the given user
	return true, nil
}

// Check if a username already exists in our system.  Returns true if the username is already taken, false if not.
// If an error occurred, the true/false value should be ignored, and only the error return code used.
func CheckUserExists(userName string) (bool, error) {
	dbQuery := `
		SELECT count(user_id)
		FROM users
		WHERE user_name = $1`
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
		WHERE user_name = $1`, userName).Scan(&cert)
	if err != nil {
		log.Printf("Retrieving client cert for '%s' from database failed: %v\n", userName, err)
		return nil, err
	}

	return cert, nil
}

// Creates a connection pool to the PostgreSQL server.
func ConnectPostgreSQL() (err error) {
	pgPoolConfig := pgx.ConnPoolConfig{*pgConfig, PGConnections, nil, 2 * time.Second}
	pdb, err = pgx.NewConnPool(pgPoolConfig)
	if err != nil {
		return errors.New(fmt.Sprintf("Couldn't connect to PostgreSQL server: %v\n", err))
	}

	// Log successful connection
	log.Printf("Connected to PostgreSQL server: %v:%v\n", conf.Pg.Server, uint16(conf.Pg.Port))

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
				WHERE user_name = $1)
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

// Return a list of 1) users with public databases, 2) along with the logged in user's most recently modified database,
// including their private one(s).
func DB4SDefaultList(loggedInUser string) ([]UserInfo, error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE user_name = $1
		), user_db_list AS (
			SELECT DISTINCT ON (db_id) db_id, last_modified
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
			AND is_deleted = false
		), most_recent_user_db AS (
			SELECT udb.db_id, udb.last_modified
			FROM user_db_list AS udb
			ORDER BY udb.last_modified DESC
			LIMIT 1
		), public_dbs AS (
			SELECT db_id, last_modified
			FROM sqlite_databases
			WHERE public = true
			AND is_deleted = false
			ORDER BY last_modified DESC
		), public_users AS (
			SELECT DISTINCT ON (db.user_id) db.user_id, db.last_modified
			FROM public_dbs as pub, sqlite_databases AS db, most_recent_user_db AS usr
			WHERE db.db_id = pub.db_id OR db.db_id = usr.db_id
			ORDER BY db.user_id, db.last_modified DESC
		)
		SELECT user_name, last_modified
		FROM public_users AS pu, users
		WHERE users.user_id = pu.user_id
		ORDER BY last_modified DESC`
	rows, err := pdb.Query(dbQuery, loggedInUser)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return nil, err
	}
	defer rows.Close()
	var list []UserInfo
	for rows.Next() {
		var oneRow UserInfo
		err = rows.Scan(&oneRow.Username, &oneRow.LastModified)
		if err != nil {
			log.Printf("Error retrieving database list for user: %v\n", err)
			return nil, err
		}
		list = append(list, oneRow)
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
			db.branches, db.releases, db.contributors, db.one_line_description, db.full_description,
			db.default_table, db.public, db.source_url, db.tags, db.default_branch
		FROM sqlite_databases AS db
		WHERE db.user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = $1
			)
			AND db.folder = $2
			AND db.db_name = $3
			AND db.is_deleted = false`

	// If the request is for another users database, ensure we only look up public ones
	if loggedInUser != dbOwner {
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
		&DB.Info.LastModified, &DB.Info.Watchers, &DB.Info.Stars, &DB.Info.Discussions, &DB.Info.MRs,
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
				WHERE user_name = $1)
			AND folder = $2
			AND db_name = $3)`
	err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&DB.Info.Forks)
	if err != nil {
		log.Printf("Error retrieving fork count for '%s%s%s': %v\n", dbOwner, dbFolder, dbName, err)
		return err
	}

	// Cache the database details
	err = CacheData(mdataCacheKey, DB, 120)
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
				WHERE user_name = $1
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

// Retrieve the default commit ID for a specific database
func DefaultCommit(dbOwner string, dbFolder string, dbName string) (string, error) {
	// If no commit ID was supplied, we retrieve the latest commit ID from the default branch
	dbQuery := `
		SELECT branch_heads->default_branch->'commit' AS commit_id
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
					FROM users
					WHERE user_name = $1
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
					WHERE user_name = $1
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
		dbQuery = `
			DELETE
			FROM sqlite_databases
			WHERE user_id = (
					SELECT user_id
					FROM users
					WHERE user_name = $1
				)
				AND folder = $2
				AND db_name = $3`
		commandTag, err := tx.Exec(dbQuery, dbOwner, dbFolder, dbName)
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
					WHERE user_name = $1
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
				WHERE user_name = $1
			)
			AND folder = $2
			AND db_name = $3`
	commandTag, err = tx.Exec(dbQuery, dbOwner, dbFolder, dbName, newName)
	if err != nil {
		log.Printf("Deleting (forked) database entry failed for database '%s%s%s': %v\n", dbOwner, dbFolder, dbName,
			err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf(
			"Wrong number of rows (%v) affected when deleting (forked) database '%s%s%s'\n", numRows, dbOwner,
			dbFolder, dbName)
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

// Deletes the latest commit from a given branch.
func DeleteLatestBranchCommit(dbOwner string, dbFolder string, dbName string, branchName string) error {
	// Begin a transaction
	tx, err := pdb.Begin()
	if err != nil {
		return err
	}
	// Set up an automatic transaction roll back if the function exits without committing
	defer tx.Rollback()

	// Retrieve the branch list for the database, as we'll use it a few times in this function
	dbQuery := `
		SELECT branch_heads
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = $1
			)
			AND folder = $2
			AND db_name = $3
			AND is_deleted = false`
	var branchList map[string]BranchEntry
	err = tx.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&branchList)
	if err != nil {
		log.Printf("Retreving branch list failed for database '%s%s%s': %v\n", dbOwner, dbFolder, dbName, err)
		return err
	}

	// Grab the Commit ID of the branch head
	branch, ok := branchList[branchName]
	if !ok {
		// We weren't able to retrieve the branch information, so it's likely the branch doesn't exist any more, or
		// some other weirdness is happening
		log.Printf("Although no database error occurred, we couldn't retrieve a commit ID for branch '%s' of "+
			"database '%s%s%s'.", branchName, dbOwner, dbFolder, dbName)
		return errors.New("Database error when attempting to delete the commit")
	}
	commitID := branch.Commit

	// Retrieve the entire commit list for the database, as we'll use it a few times in this function
	dbQuery = `
		SELECT commit_list
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = $1
			)
			AND folder = $2
			AND db_name = $3
			AND is_deleted = false`
	var commitList map[string]CommitEntry
	err = tx.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&commitList)
	if err != nil {
		log.Printf("Retreving commit list failed for database '%s%s%s': %v\n", dbOwner, dbFolder, dbName, err)
		return err
	}

	// Ensure we're not being asked to delete the last commit of a branch (eg ensure it has a non empty Parent field)
	headCommit, ok := commitList[commitID]
	if !ok {
		log.Printf("Something went wrong retrieving commit '%s' from the commit list of database "+
			"'%s%s%s'\n", commitID, dbOwner, dbFolder, dbName)
		return errors.New("Error when retrieving commit information for the database")
	}
	if headCommit.Parent == "" {
		log.Printf("Error.  Not going to remove the last commit of branch '%s' on database '%s%s%s'\n",
			branchName, dbOwner, dbFolder, dbName)
		return errors.New("Removing the only remaining commit for a branch isn't allowed")
	}

	// Walk the other branches, checking if the commit is used in any of them.  If it is, we'll still move the branch
	// head back by one, but we'd better not remove the commit itself from the commit_list in the database
	foundElsewhere := false
	for bName, bEntry := range branchList {
		if bName == branchName {
			// No need to walk the tree for the branch we're deleting from
			continue
		}
		c := CommitEntry{Parent: bEntry.Commit}
		for c.Parent != "" {
			c, ok = commitList[c.Parent]
			if !ok {
				log.Printf("Error when walking the commit history of '%s%s%s', looking for commit '%s' in branch '%s'\n",
					dbOwner, dbFolder, dbName, c.Parent, bName)
				return errors.New("Error when attempting to remove the commit")
			}
			if c.ID == commitID {
				// The commit is being used by other branches, so we'd better not delete it from the commit_list in
				// the database
				foundElsewhere = true
				break
			}
		}
	}

	// Count the number of commits in the updated branch
	c := commitList[headCommit.Parent]
	commitCount := 1
	for c.Parent != "" {
		commitCount++
		c, ok = commitList[c.Parent]
		if !ok {
			log.Printf("Error when counting # of commits in branch '%s' of database '%s%s%s'\n", branchName,
				dbOwner, dbFolder, dbName)
			return errors.New("Error when counting commits during commit deletion")
		}
	}

	// Update the branch head to point at the previous commit
	branch.Commit = headCommit.Parent
	branch.CommitCount = commitCount
	branchList[branchName] = branch
	dbQuery = `
		WITH our_db AS (
			SELECT db_id
			FROM sqlite_databases
			WHERE user_id = (
					SELECT user_id
					FROM users
					WHERE user_name = $1
				)
				AND folder = $2
				AND db_name = $3
				AND is_deleted = false
		)
		UPDATE sqlite_databases AS db
		SET branch_heads = $4
		FROM our_db
		WHERE db.db_id = our_db.db_id`
	commandTag, err := tx.Exec(dbQuery, dbOwner, dbFolder, dbName, branchList)
	if err != nil {
		log.Printf("Moving branch '%s' back one commit failed for database '%s%s%s': %v\n", branchName, dbOwner,
			dbFolder, dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf(
			"Wrong number of rows (%v) affected when moving branch '%s' back one commit for database '%s%s%s'\n",
			numRows, branchName, dbOwner, dbFolder, dbName)
	}

	// If needed remove the commit from the commit list
	delete(commitList, commitID)
	if !foundElsewhere {
		dbQuery = `
		WITH our_db AS (
			SELECT db_id
			FROM sqlite_databases
			WHERE user_id = (
					SELECT user_id
					FROM users
					WHERE user_name = $1
				)
				AND folder = $2
				AND db_name = $3
				AND is_deleted = false
		)
		UPDATE sqlite_databases AS db
		SET commit_list = $4
		FROM our_db
		WHERE db.db_id = our_db.db_id`
		commandTag, err := tx.Exec(dbQuery, dbOwner, dbFolder, dbName, commitList)
		if err != nil {
			log.Printf("Removing commit '%s' failed for database '%s%s%s': %v\n", commitID, dbOwner, dbFolder, dbName,
				err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf(
				"Wrong number of rows (%v) affected when removing commit '%s' for database '%s%s%s'\n", numRows,
				commitID, dbOwner, dbFolder, dbName)
		}

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
	log.Printf("Disconnected from PostgreSQL server: %v:%v\n", conf.Pg.Server, uint16(conf.Pg.Port))
}

// Fork the PostgreSQL entry for a SQLite database from one user to another
func ForkDatabase(srcOwner string, dbFolder string, dbName string, dstOwner string) (int, error) {
	// Copy the main database entry
	dbQuery := `
		WITH dst_u AS (
			SELECT user_id
			FROM users
			WHERE user_name = $1
		)
		INSERT INTO sqlite_databases (user_id, folder, db_name, public, forks, one_line_description, full_description,
			commits,  branches, contributors,
			root_database, default_table, source_url, commit_list, branch_heads, tags, default_branch,
			forked_from)
		SELECT dst_u.user_id, folder, db_name, public, forks, one_line_description, full_description,
			commits,  branches, contributors,
			root_database, default_table, source_url, commit_list, branch_heads, tags, default_branch,
			db_id
		FROM sqlite_databases, dst_u
		WHERE sqlite_databases.user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = $2
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

	// Increment the forks count for the root database
	dbQuery = `
		UPDATE sqlite_databases
		SET forks = forks + 1
		WHERE db_id = (
			SELECT root_database
			FROM sqlite_databases
			WHERE user_id = (
					SELECT user_id
					FROM users
					WHERE user_name = $1
				)
				AND folder = $2
				AND db_name = $3
			)
		RETURNING forks`
	var newForks int
	err = pdb.QueryRow(dbQuery, dstOwner, dbFolder, dbName).Scan(&newForks)
	if err != nil {
		log.Printf("Updating fork count in PostgreSQL failed: %v\n", err)
		return 0, err
	}
	return newForks, nil
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
				WHERE user_name = $1)
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
						WHERE user_name = $1
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
	if !dbList[0].Public && (dbList[0].Owner != loggedInUser) {
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
				WHERE user_name = $1
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

// Retrieve the default branch name for a database.
func GetDefaultBranchName(dbOwner string, dbFolder string, dbName string) (string, error) {
	// Return the default branch name
	dbQuery := `
		SELECT db.default_branch
		FROM sqlite_databases AS db
		WHERE db.user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = $1
			)
			AND db.folder = $2
			AND db.db_name = $3
			AND db.is_deleted = false`
	var branchName string
	err := pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&branchName)
	if err != nil {
		if err != pgx.ErrNoRows {
			log.Printf("Error when retrieving default branch name for database '%s%s%s': %v\n", dbOwner,
				dbFolder, dbName, err)
			return "", err
		} else {
			log.Printf("No default branch name exists for database '%s%s%s'. This shouldn't happen\n", dbOwner,
				dbFolder, dbName)
			return "", err
		}
	}
	return branchName, nil
}

// Retrieves the full commit list for a database.
func GetCommitList(dbOwner string, dbFolder string, dbName string) (map[string]CommitEntry, error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE user_name = $1
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

// Returns the text for a given licence.
func GetLicence(userName string, licenceName string) (txt string, err error) {
	dbQuery := `
		SELECT licence_text
		FROM database_licences
		WHERE friendly_name = $2
		AND (user_id IS NULL
				OR user_id = (
					SELECT user_id
					FROM users
					WHERE user_name = $1
				)`
	err = pdb.QueryRow(dbQuery, userName, licenceName).Scan(&txt)
	if err != nil {
		log.Printf("Error when retrieving licence '%s', user '%s': %v\n", licenceName, userName, err)
		return "", err
	}
	if txt == "" {
		// The requested licence text wasn't found
		return "", errors.New("Licence text not found")
	}
	return txt, nil
}

// Returns the list of licences available to a user.
func GetLicences(user string) (map[string]LicenceEntry, error) {
	dbQuery := `
		SELECT friendly_name, lic_sha256, licence_url, display_order
		FROM database_licences
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = 'default'
			)
			OR user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = $1
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
		err = rows.Scan(&name, &oneRow.Sha256, &oneRow.URL, &oneRow.Order)
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
				WHERE user_name = $1
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
				WHERE user_name = $1
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

// Retrieve the tags for a database.
func GetTags(dbOwner string, dbFolder string, dbName string) (tags map[string]TagEntry, err error) {
	dbQuery := `
		SELECT tag_list
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = $1
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

// Retrieve display name and email address for a given user.
func GetUserDetails(userName string) (displayName string, email string, err error) {
	// Retrieve the values from the database
	dbQuery := `
		SELECT display_name, email
		FROM users
		WHERE user_name = $1`
	var dn, em pgx.NullString
	err = pdb.QueryRow(dbQuery, userName).Scan(&dn, &em)
	if err != nil {
		log.Printf("Error when retrieving display name and email for user '%s': %v\n", userName, err)
		return "", "", err
	}

	// Return the values which aren't NULL.  For those which are, return an empty string.
	if dn.Valid {
		displayName = dn.String
	}
	if em.Valid {
		email = em.String
	}
	return displayName, email, err
}

// Returns the username associated with an email address.
func GetUsernameFromEmail(email string) (userName string, err error) {
	dbQuery := `
		SELECT user_name
		FROM users
		WHERE email = $1`
	err = pdb.QueryRow(dbQuery, email).Scan(&userName)
	if err != nil {
		if err == pgx.ErrNoRows {
			// No matching username of the email
			return "", nil
		}
		log.Printf("Looking up username for email address '%s' failed: %v\n", email, err)
		return "", err
	}
	return userName, nil
}

// Return the Minio bucket and ID for a given database. dbOwner, dbFolder, & dbName are from owner/folder/database URL
// fragment, // loggedInUser is the name for the currently logged in user, for access permission check.  Use an empty
// string ("") as the loggedInUser parameter if the true value isn't set or known.
// If the requested database doesn't exist, or the loggedInUser doesn't have access to it, then an error will be
// returned.
func MinioLocation(dbOwner string, dbFolder string, dbName string, commitID string, loggedInUser string) (minioBucket string,
	minioID string, err error) {

	// TODO: This will likely need updating to query the "database_files" table to retrieve the Minio server name

	// If no commit was provided, we grab the default one
	if commitID == "" {
		var err error
		commitID, err = DefaultCommit(dbOwner, dbFolder, dbName)
		if err != nil {
			return minioBucket, minioID, err // Bucket and ID are still the initial default empty string
		}
	}

	// Retrieve the sha256 for the requested commit's database file
	var dbQuery string
	dbQuery = `
		SELECT commit_list->$4::text->'tree'->'entries'->0->'sha256' AS sha256
		FROM sqlite_databases AS db
		WHERE db.user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = $1
			)
			AND db.folder = $2
			AND db.db_name = $3
			AND db.is_deleted = false`

	// If the request is for another users database, it needs to be a public one
	if loggedInUser != dbOwner {
		dbQuery += `
				AND db.public = true`
	}

	var sha256 string
	err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName, commitID).Scan(&sha256)
	if err != nil {
		log.Printf("Error retrieving MinioID for %s/%s version %v: %v\n", dbOwner, dbName, commitID, err)
		return minioBucket, minioID, err // Bucket and ID are still the initial default empty string
	}

	if sha256 == "" {
		// The requested database doesn't exist, or the logged in user doesn't have access to it
		return minioBucket, minioID, errors.New("The requested database wasn't found") // Bucket and ID are still the initial default empty string
	}

	minioBucket = sha256[:MinioFolderChars]
	minioID = sha256[MinioFolderChars:]
	return minioBucket, minioID, nil
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
			WHERE user_name = $1)`
	var maxRows int
	err := pdb.QueryRow(dbQuery, loggedInUser).Scan(&maxRows)
	if err != nil {
		log.Printf("Error retrieving user '%s' preference data: %v\n", loggedInUser, err)
		return DefaultNumDisplayRows // Use the default value
	}

	return maxRows
}

// Return a list of users with public databases.
func PublicUserDBs() ([]UserInfo, error) {
	dbQuery := `
		WITH public_dbs AS (
			SELECT DISTINCT ON (user_id) user_id, last_modified
			FROM sqlite_databases
			WHERE public = true
			AND is_deleted = false
			ORDER BY user_id, last_modified DESC
		)
		SELECT users.user_name, dbs.last_modified
		FROM public_dbs AS dbs, users
		WHERE users.user_id = dbs.user_id
		ORDER BY last_modified DESC`
	rows, err := pdb.Query(dbQuery)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return nil, err
	}
	defer rows.Close()
	var list []UserInfo
	for rows.Next() {
		var oneRow UserInfo
		err = rows.Scan(&oneRow.Username, &oneRow.LastModified)
		if err != nil {
			log.Printf("Error retrieving database list for user: %v\n", err)
			return nil, err
		}
		list = append(list, oneRow)
	}

	return list, nil
}

// Rename a SQLite database.
func RenameDatabase(userName string, dbFolder string, dbName string, newName string) error {
	// Save the database settings
	SQLQuery := `
		UPDATE sqlite_databases
		SET dbname = $4
		WHERE username = $1
			AND folder = $2
			AND dbname = $3`
	commandTag, err := pdb.Exec(SQLQuery, userName, dbFolder, dbName, newName)
	if err != nil {
		log.Printf("Renaming database '%s%s%s' failed: %v\n", userName, dbFolder,
			dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		errMsg := fmt.Sprintf("Wrong number of rows affected (%v) when renaming '%s%s%s' to '%s%s%s'\n",
			numRows, userName, dbFolder, dbName, userName, dbFolder, newName)
		log.Printf(errMsg)
		return errors.New(errMsg)
	}

	// Log the rename
	log.Printf("Database renamed from '%s%s%s' to '%s%s%s'\n", userName, dbFolder, dbName, userName,
		dbFolder, newName)

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
				WHERE user_name = $1
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

// Stores a certificate for a given client.
func SetClientCert(newCert []byte, userName string) error {
	SQLQuery := `
		UPDATE users
		SET client_cert = $1
		WHERE user_name = $2`
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
func SetPrefUserMaxRows(userName string, maxRows int, displayName string, email string) error {
	dbQuery := `
		UPDATE users
		SET pref_max_rows = $2, display_name = $3, email = $4
		WHERE user_name = $1`
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
				WHERE user_name = $1
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
					WHERE user_name = $1
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

// Updates the branches list for a database.
func StoreBranches(dbOwner string, dbFolder string, dbName string, branches map[string]BranchEntry) error {
	dbQuery := `
		UPDATE sqlite_databases
		SET branch_heads = $4, branches = $5
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = $1
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

// Updates the commit list for a database.
func StoreCommits(dbOwner string, dbFolder string, dbName string, commitList map[string]CommitEntry) error {
	dbQuery := `
		UPDATE sqlite_databases
		SET commit_list = $4, commits = $5
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = $1
				)
			AND folder = $2
			AND db_name = $3`
	commandTag, err := pdb.Exec(dbQuery, dbOwner, dbFolder, dbName, commitList, len(commitList))
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
	pub bool, buf []byte, sha string, oneLineDesc string, fullDesc string, createDefBranch bool, branchName string,
	sourceURL string) error {
	// Store the database file
	err := StoreDatabaseFile(buf, sha)
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
			WHERE user_name = $1), (SELECT val FROM root), $2, $3, $4, $5, $6, $8, (SELECT val FROM root), $7`
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
				WHERE user_name = $1
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

// Store a licence.
func StoreLicence(userName string, licenceName string, txt []byte, url string, orderNum int) error {
	// Store the licence in PostgreSQL
	sha := sha256.Sum256(txt)
	dbQuery := `
	WITH u AS (
		SELECT user_id
		FROM users
		WHERE user_name = $1
	)
	INSERT INTO database_licences (user_id, friendly_name, lic_sha256, licence_text, licence_url, display_order)
	SELECT (SELECT user_id FROM u), $2, $3, $4, $5, $6
	ON CONFLICT (user_id, friendly_name)
		DO UPDATE
		SET friendly_name = $2,
			lic_sha256 = $3,
			licence_text = $4,
			licence_url = $5,
			user_id = (SELECT user_id FROM u),
			display_order = $6`
	commandTag, err := pdb.Exec(dbQuery, userName, licenceName, hex.EncodeToString(sha[:]), txt, url, orderNum)
	if err != nil {
		log.Printf("Inserting licence '%v' in database failed: %v\n", licenceName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%v) affected when storing licence '%v'\n", numRows, licenceName)
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
				WHERE user_name = $1
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
				WHERE user_name = $2
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
				WHERE user_name = $2
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
				WHERE user_name = $1
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

// Returns details for a user.
func User(userName string) (user UserDetails, err error) {
	dbQuery := `
		SELECT user_name, email, password_hash, date_joined, client_cert
		FROM users
		WHERE username = $1`
	err = pdb.QueryRow(dbQuery, userName).Scan(&user.Username, &user.Email, &user.PHash, &user.DateJoined,
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

	return user, nil
}

// Returns the list of databases for a user.
func UserDBs(userName string, public AccessType) (list []DBInfo, err error) {
	// Construct SQL query for retrieving the requested database list
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE user_name = $1
		), default_commits AS (
			SELECT DISTINCT ON (db.db_name) db_name, db.db_id, db.branch_heads->db.default_branch->>'commit' AS id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
		), dbs AS (
			SELECT DISTINCT ON (db.db_name) db.db_name, db.folder, db.date_created, db.last_modified, db.public,
				db.watchers, db.stars, db.discussions, db.merge_requests, db.branches, db.releases, db.tags,
				db.contributors, db.one_line_description, default_commits.id,
				db.commit_list->default_commits.id->'tree'->'entries'->0, db.source_url
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
		var desc, source pgx.NullString
		var oneRow DBInfo
		err = rows.Scan(&oneRow.Database, &oneRow.Folder, &oneRow.DateCreated, &oneRow.LastModified, &oneRow.Public,
			&oneRow.Watchers, &oneRow.Stars, &oneRow.Discussions, &oneRow.MRs, &oneRow.Branches,
			&oneRow.Releases, &oneRow.Tags, &oneRow.Contributors, &desc, &oneRow.CommitID, &oneRow.DBEntry, &source)
		if err != nil {
			log.Printf("Error retrieving database list for user: %v\n", err)
			return nil, err
		}
		if !desc.Valid {
			oneRow.OneLineDesc = ""
		} else {
			oneRow.OneLineDesc = fmt.Sprintf(": %s", desc.String)
		}
		if !source.Valid {
			oneRow.SourceURL = ""
		} else {
			oneRow.SourceURL = source.String
		}
		oneRow.Size = oneRow.DBEntry.Size

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
				WHERE user_name = $1
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

// Returns a list of all DBHub.io users.
func UserList() ([]UserDetails, error) {
	dbQuery := `
		SELECT username, email, password_hash, date_joined
		FROM users
		ORDER BY username ASC`
	rows, err := pdb.Query(dbQuery)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return nil, err
	}
	defer rows.Close()

	// Assemble the row data into a list
	var userList []UserDetails
	for rows.Next() {
		var u UserDetails
		err = rows.Scan(&u.Username, &u.Email, &u.PHash, &u.DateJoined)
		if err != nil {
			log.Printf("Error retrieving user list from database: %v\n", err)
			return nil, err
		}
		userList = append(userList, u)
	}

	return userList, nil
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
			WHERE user_name = $1
		),
		stars AS (
			SELECT st.db_id, st.date_starred
			FROM database_stars AS st, u
			WHERE st.user_id = u.user_id
		),
		db_users AS (
			SELECT db.user_id, db.db_id, db.folder, db.db_name, stars.date_starred
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
						WHERE user_name = $1
					)
					AND folder = $2
					AND db_name = $3
					AND is_deleted = false
				)
		)
		SELECT users.user_name, star_users.date_starred
		FROM users, star_users
		WHERE users.user_id = star_users.user_id`
	rows, err := pdb.Query(dbQuery, dbOwner, dbFolder, dbName)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var oneRow DBEntry
		err = rows.Scan(&oneRow.Owner, &oneRow.DateEntry)
		if err != nil {
			log.Printf("Error retrieving list of stars for %s/%s: %v\n", dbOwner, dbName, err)
			return nil, err
		}
		list = append(list, oneRow)
	}
	return list, nil
}
