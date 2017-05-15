package common

import (
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

// Add a user to the system.
func AddUser(auth0ID string, userName string, password string, email string) error {
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
			log.Printf("Error when checking if Minio bucket already exists: %v\n", err)
			return err
		}
	}

	// Generate a new HTTPS client certificate for the user
	cert, err := GenerateClientCert(userName, 14) // 14 days validity while developing
	if err != nil {
		log.Printf("Error when generating client certificate for '%s': %v\n", userName, err)
		return err
	}

	// Add the new user to the database
	insertQuery := `
		INSERT INTO users (auth0id, username, email, password_hash, client_certificate, minio_bucket)
		VALUES ($1, $2, $3, $4, $5, $6)`
	commandTag, err := pdb.Exec(insertQuery, auth0ID, userName, email, hash, cert, bucket)
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

// Add a new SQLite database for a user.
func AddDatabase(dbOwner string, dbFolder string, dbName string, dbVer int, shaSum []byte, dbSize int, public bool, bucket string, id string) error {
	// If it's a new database, add its details to the main PG sqlite_databases table
	var dbQuery string
	if dbVer == 1 {
		dbQuery = `
			WITH root_db_value AS (
				SELECT nextval('sqlite_databases_idnum_seq')
			)
			INSERT INTO sqlite_databases (username, folder, dbname, public, idnum, minio_bucket, root_database)
			VALUES ($1, $2, $3, $4, (SELECT nextval FROM root_db_value), $5, (SELECT nextval FROM root_db_value))`
		commandTag, err := pdb.Exec(dbQuery, dbOwner, dbFolder, dbName, public, bucket)
		if err != nil {
			log.Printf("Adding database to PostgreSQL failed: %v\n", err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf("Wrong number of rows (%v) affected when creating initial sqlite_databases "+
				"entry for '%s%s/%s'\n", numRows, dbOwner, dbFolder, dbName)
		}
	}

	// Add the database to database_versions
	dbQuery = `
		WITH databaseid AS (
			SELECT idnum
			FROM sqlite_databases
			WHERE username = $1
				AND dbname = $2)
		INSERT INTO database_versions (db, size, version, sha256, minioid)
		SELECT idnum, $3, $4, $5, $6 FROM databaseid`
	commandTag, err := pdb.Exec(dbQuery, dbOwner, dbName, dbSize, dbVer, hex.EncodeToString(shaSum[:]), id)
	if err != nil {
		log.Printf("Adding version info to PostgreSQL failed: %v\n", err)
		return err
	}

	// Update the last_modified date for the database in sqlite_databases
	dbQuery = `
		UPDATE sqlite_databases
		SET last_modified = (
			SELECT last_modified
			FROM database_versions
			WHERE db = (
				SELECT idnum
				FROM sqlite_databases
				WHERE username = $1
					AND dbname = $2)
				AND version = $3)
		WHERE username = $1
			AND dbname = $2`
	commandTag, err = pdb.Exec(dbQuery, dbOwner, dbName, dbVer)
	if err != nil {
		log.Printf("Updating last_modified date in PostgreSQL failed: %v\n", err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows affected: %v, user: %s, database: %v\n", numRows, dbOwner, dbName)
	}

	return nil
}

// Check if a database has been starred by a given user.  The boolean return value is only valid when err is nil.
func CheckDBStarred(loggedInUser string, dbOwner string, dbFolder string, dbName string) (bool, error) {
	dbQuery := `
		SELECT count(db)
		FROM database_stars
		WHERE database_stars.username = $1
		AND database_stars.db = (
			SELECT idnum
			FROM sqlite_databases
			WHERE username = $2
				AND folder = $3
				AND dbname = $4)`
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
		SELECT count(username)
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

// Checks if a given MinioID string is available for use by a user. Returns true if available, false if not.  Only
// if err returns a non-nil value.
func CheckMinioIDAvail(userName string, id string) (bool, error) {
	// Check if an existing database for the user already uses the given MinioID
	var dbVer int
	dbQuery := `
		WITH user_databases AS (
			SELECT idnum
			FROM sqlite_databases
			WHERE username = $1)
		SELECT ver.version
		FROM database_versions AS ver, user_databases AS db
		WHERE ver.db = db.idnum
			AND ver.minioid = $2`
	err := pdb.QueryRow(dbQuery, userName, id).Scan(&dbVer)
	if err != nil {
		if err == pgx.ErrNoRows {
			// Not a real error, there just wasn't a matching row
			return true, nil
		}

		// A real database error occurred
		log.Printf("Error checking if a MinioID is already taken: %v\n", err)
		return false, err
	}

	if dbVer == 0 {
		// Nothing already using the MinioID, so it's available for use
		return true, nil
	}

	// The MinioID is already in use
	return false, nil
}

// Check if a user has access to a specific version of a database.
func CheckUserDBVAccess(dbOwner string, dbFolder string, dbName string, dbVer int, loggedInUser string) (bool, error) {
	dbQuery := `
		SELECT version
		FROM database_versions
		WHERE db = (
			SELECT idnum
			FROM sqlite_databases
			WHERE username = $1
				AND folder = $2
				AND dbname = $3`
	if dbOwner != loggedInUser {
		dbQuery += ` AND public = true `
	}
	dbQuery += `
			)
			AND version = $4`
	var numRows int
	err := pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName, dbVer).Scan(&numRows)
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
		SELECT count(username)
		FROM users
		WHERE username = $1`
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
		SELECT client_certificate
		FROM users
		WHERE username = $1`, userName).Scan(&cert)
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

	// Log successful connection message
	log.Printf("Connected to PostgreSQL server: %v:%v\n", conf.Pg.Server, uint16(conf.Pg.Port))

	return nil
}

// Returns the ID number for a given user's database.
func databaseID(dbOwner string, dbName string) (dbID int, err error) {
	// Retrieve the database id
	dbQuery := `
		SELECT idnum
		FROM sqlite_databases
		WHERE username = $1
			AND dbname = $2`
	err = pdb.QueryRow(dbQuery, dbOwner, dbName).Scan(&dbID)
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
		WITH user_db_list AS (
			SELECT DISTINCT ON (idnum) idnum, last_modified
			FROM sqlite_databases
			WHERE username = $1
		), most_recent_user_db AS (
			SELECT idnum, last_modified
			FROM user_db_list
			ORDER BY last_modified DESC
			LIMIT 1
		), public_dbs AS (
			SELECT idnum, last_modified
			FROM sqlite_databases
			WHERE public = true
			ORDER BY last_modified DESC
		), public_users AS (
			SELECT DISTINCT ON (db.username) db.username, db.last_modified
			FROM public_dbs as pub, sqlite_databases AS db, most_recent_user_db AS usr
			WHERE db.idnum = pub.idnum OR db.idnum = usr.idnum
			ORDER BY db.username, db.last_modified DESC
		)
		SELECT username, last_modified FROM public_users
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
func DBDetails(DB *SQLiteDBinfo, loggedInUser string, dbOwner string, dbFolder string, dbName string, dbVersion int) error {
	dbQuery := `
		SELECT ver.minioid, db.date_created, db.last_modified, ver.size, ver.version, db.watchers, db.stars,
			db.discussions, db.pull_requests, db.updates, db.branches, db.releases, db.contributors,
			db.description, db.readme, db.minio_bucket, db.default_table, db.public
		FROM sqlite_databases AS db, database_versions AS ver
		WHERE db.username = $1
			AND db.folder = $2
			AND db.dbname = $3
			AND db.idnum = ver.db`
	if loggedInUser != dbOwner {
		// * The request is for another users database, so it needs to be a public one *
		dbQuery += `
			AND db.public = true`
	}
	if dbVersion == 0 {
		// No specific database version was requested, so use the highest available
		dbQuery += `
			ORDER BY version DESC
			LIMIT 1`
	} else {
		dbQuery += `
			AND ver.version = $4`
	}

	// Generate a predictable cache key for this functions' metadata.  Probably not sharable with other functions
	// cached metadata
	mdataCacheKey := MetadataCacheKey("meta", loggedInUser, dbOwner, dbFolder, dbName, dbVersion)

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
	var Desc, Readme, defTable pgx.NullString
	if dbVersion == 0 {
		err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&DB.MinioId, &DB.Info.DateCreated,
			&DB.Info.LastModified, &DB.Info.Size, &DB.Info.Version, &DB.Info.Watchers, &DB.Info.Stars,
			&DB.Info.Discussions, &DB.Info.MRs, &DB.Info.Updates, &DB.Info.Branches, &DB.Info.Releases,
			&DB.Info.Contributors, &Desc, &Readme, &DB.MinioBkt, &defTable, &DB.Info.Public)
	} else {
		err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName, dbVersion).Scan(&DB.MinioId,
			&DB.Info.DateCreated, &DB.Info.LastModified, &DB.Info.Size, &DB.Info.Version,
			&DB.Info.Watchers, &DB.Info.Stars, &DB.Info.Discussions, &DB.Info.MRs, &DB.Info.Updates,
			&DB.Info.Branches, &DB.Info.Releases, &DB.Info.Contributors, &Desc, &Readme, &DB.MinioBkt,
			&defTable, &DB.Info.Public)
	}
	if err != nil {
		return errors.New("The requested database doesn't exist")
	}
	if !Desc.Valid {
		DB.Info.Description = "No description"
	} else {
		DB.Info.Description = Desc.String
	}
	if !Readme.Valid {
		DB.Info.Readme = "No full description"
	} else {
		DB.Info.Readme = Readme.String
	}
	if !defTable.Valid {
		DB.Info.DefaultTable = ""
	} else {
		DB.Info.DefaultTable = defTable.String
	}

	// Fill out the fields we already have data for
	DB.Info.Database = dbName
	DB.Info.Folder = dbFolder

	// Retrieve latest fork count
	// TODO: This can probably be folded into the above SQL query as a subselect, as a minor optimisation
	dbQuery = `
		SELECT forks
		FROM sqlite_databases
		WHERE idnum = (
			SELECT root_database
			FROM sqlite_databases
			WHERE username = $1
			AND folder = $2
			AND dbname = $3)`
	err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&DB.Info.Forks)
	if err != nil {
		log.Printf("Error retrieving fork count for '%s%s': %v\n", dbOwner, dbName, err)
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
func DBStars(dbOwner string, dbName string) (starCount int, err error) {
	// Get the ID number of the database
	dbID, err := databaseID(dbOwner, dbName)
	if err != nil {
		return -1, err
	}

	// Retrieve the updated star count
	dbQuery := `
		SELECT stars
		FROM sqlite_databases
		WHERE idnum = $1`
	err = pdb.QueryRow(dbQuery, dbID).Scan(&starCount)
	if err != nil {
		log.Printf("Error looking up star count for database '%s/%s'. Error: %v\n", dbOwner, dbName, err)
		return -1, err
	}
	return starCount, nil
}

// Returns the list of all database versions available to the requesting user
func DBVersions(loggedInUser string, dbOwner string, dbFolder string, dbName string) ([]int, error) {
	dbQuery := `
		SELECT version
		FROM database_versions
		WHERE db = (
			SELECT idnum
			FROM sqlite_databases
			WHERE username = $1
				AND folder = $2
				AND dbname = $3`
	if loggedInUser != dbOwner {
		// The request is for another users database, so only return public versions
		dbQuery += `
				AND public is true`
	}
	dbQuery += `
			)
		ORDER BY version DESC`
	rows, err := pdb.Query(dbQuery, dbOwner, dbFolder, dbName)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return nil, err
	}
	defer rows.Close()
	var verList []int
	for rows.Next() {
		var ver int
		err = rows.Scan(&ver)
		if err != nil {
			log.Printf("Error retrieving version list for '%s%s%s': %v\n", dbOwner, dbFolder, dbName,
				err)
			return nil, err
		}
		verList = append(verList, ver)
	}

	// Safety checks
	numResults := len(verList)
	if numResults == 0 {
		return nil, errors.New("Empty list returned instead of version list.  This shouldn't happen")
	}

	return verList, nil
}

// Disconnects the PostgreSQL database connection.
func DisconnectPostgreSQL() {
	pdb.Close()
}

// Fork the PostgreSQL entry for a SQLite database from one user to another
func ForkDatabase(srcOwner string, srcFolder string, dbName string, srcVer int, dstOwner string,
	dstFolder string, dstMinioID string) (int, error) {

	// Retrieve the Minio bucket for the owner
	dstBucket, err := MinioUserBucket(dstOwner)
	if err != nil {
		log.Printf("Error looking up Minio bucket for user '%s': %v\n", dstOwner, err.Error())
		return 0, err
	}

	// Copy the main database entry
	dbQuery := `
		INSERT INTO sqlite_databases (username, folder, dbname, public, forks, description, readme, minio_bucket, root_database, forked_from)
		SELECT $1, $2, dbname, public, forks, description, readme, $3, root_database, idnum
		FROM sqlite_databases
		WHERE username = $4
			AND folder = $5
			AND dbname = $6`
	commandTag, err := pdb.Exec(dbQuery, dstOwner, dstFolder, dstBucket, srcOwner, srcFolder, dbName)
	if err != nil {
		log.Printf("Forking database '%s%s/%s' version %d entry in PostgreSQL failed: %v\n",
			srcOwner, srcFolder, dbName, srcVer, err)
		return 0, err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows affected (%d) when forking main database entry: "+
			"'%s%s%s' version %d to '%s%s%s'\n", numRows, srcOwner, srcFolder, dbName, srcVer, dstOwner,
			dstFolder, dbName)
	}

	// Add a new database version entry
	dbQuery = `
		WITH new_db AS (
			SELECT idnum
			FROM sqlite_databases
			WHERE username = $1
				AND folder = $2
				AND dbname = $3
		)
		INSERT INTO database_versions (db, size, version, sha256, minioid)
		SELECT new_db.idnum, ver.size, 1, ver.sha256, $4
		FROM new_db, database_versions AS ver
		WHERE db = (
			SELECT idnum
			FROM sqlite_databases
			WHERE username = $5
				AND folder = $6
				AND dbname = $3
			)
			AND version = $7`
	commandTag, err = pdb.Exec(dbQuery, dstOwner, dstFolder, dbName, dstMinioID, srcOwner, srcFolder, srcVer)
	if err != nil {
		log.Printf("Forking database entry in PostgreSQL failed: %v\n", err)
		return 0, err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows affected (%d) when forking database version entry: "+
			"'%s%s%s' version %d to '%s%s%s'\n", numRows, srcOwner, srcFolder, dbName, srcVer, dstOwner,
			dstFolder, dbName)
	}

	// Increment the forks count for the root database
	dbQuery = `
		UPDATE sqlite_databases
		SET forks = forks + 1
		WHERE idnum = (
			SELECT root_database
			FROM sqlite_databases
			WHERE username = $1
				AND folder = $2
				AND dbname = $3
			)
		RETURNING forks`
	var newForks int
	err = pdb.QueryRow(dbQuery, dstOwner, dstFolder, dbName).Scan(&newForks)
	if err != nil {
		log.Printf("Updating fork count in PostgreSQL failed: %v\n", err)
		return 0, err
	}

	return newForks, nil
}

// Checks if the given database was forked from another, and if so returns that one's owner, folder and database name
func ForkedFrom(dbOwner string, dbFolder string, dbName string) (forkOwn string, forkFol string, forkDB string,
	err error) {
	// Check if the database was forked from another
	var idnum, forkedFrom pgx.NullInt32
	dbQuery := `
		SELECT idnum, forked_from
		FROM sqlite_databases
		WHERE username = $1
			AND folder = $2
			AND dbname = $3`
	err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&idnum, &forkedFrom)
	if err != nil {
		log.Printf("Error checking if database was forked from another '%s%s%s'. Error: %v\n", dbOwner,
			dbFolder, dbName, err)
		return "", "", "", err
	}
	if !forkedFrom.Valid {
		// The database wasn't forked, so return empty strings
		return "", "", "", nil
	}

	// Return the details of the database this one was forked from
	dbQuery = `
		SELECT username, folder, dbname
		FROM sqlite_databases
		WHERE idnum = $1`
	err = pdb.QueryRow(dbQuery, forkedFrom).Scan(&forkOwn, &forkFol, &forkDB)
	if err != nil {
		log.Printf("Error retrieving forked database information for '%s%s%s'. Error: %v\n", dbOwner,
			dbFolder, dbName, err)
		return "", "", "", err
	}
	return forkOwn, forkFol, forkDB, nil
}

// Return the complete fork tree for a given database
func ForkTree(dbOwner string, dbFolder string, dbName string) (outputList []ForkEntry, err error) {
	dbQuery := `
		SELECT username, folder, dbname, public, idnum, forked_from
		FROM sqlite_databases
		WHERE root_database = (
				SELECT root_database
				FROM sqlite_databases
				WHERE username = $1
					AND folder = $2
					AND dbname = $3
				)
		ORDER BY forked_from NULLS FIRST`
	rows, err := pdb.Query(dbQuery, dbOwner, dbFolder, dbName)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return nil, err
	}
	defer rows.Close()
	var dbList []ForkEntry
	for rows.Next() {
		var frk pgx.NullInt32
		var oneRow ForkEntry
		err = rows.Scan(&oneRow.Owner, &oneRow.Folder, &oneRow.DBName, &oneRow.Public, &oneRow.ID, &frk)
		if err != nil {
			log.Printf("Error retrieving fork list for '%s%s%s': %v\n", dbOwner, dbFolder, dbName,
				err)
			return nil, err
		}
		if frk.Valid {
			oneRow.ForkedFrom = int(frk.Int32)
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
	if !dbList[0].Public {
		dbList[0].DBName = "private database"
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
		outputList, forkTrail, forkFound = nextChild(&dbList, &outputList, &forkTrail, iconDepth)
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

// Retrieve the highest version number of a database (if any), available to a given user.
// Use the empty string "" to retrieve the highest available public version.
func HighestDBVersion(dbOwner string, dbName string, dbFolder string, loggedInUser string) (ver int, err error) {
	dbQuery := `
		SELECT version
		FROM database_versions
		WHERE db = (
			SELECT idnum
			FROM sqlite_databases
			WHERE username = $1
				AND dbname = $2
				AND folder = $3`
	if dbOwner != loggedInUser {
		dbQuery += `
				AND public = true`
	}
	dbQuery += `
			)
		ORDER BY version DESC
		LIMIT 1`
	err = pdb.QueryRow(dbQuery, dbOwner, dbName, dbFolder).Scan(&ver)
	if err != nil && err != pgx.ErrNoRows {
		log.Printf("Error when retrieving highest database version # for '%s/%s'. Error: %v\n", dbOwner,
			dbName, err)
		return -1, err
	}
	if err == pgx.ErrNoRows {
		// No database versions seem to be present
		return 0, nil
	}
	return ver, nil
}

// Return the Minio bucket name for a given user.
func MinioUserBucket(userName string) (string, error) {
	var minioBucket string
	err := pdb.QueryRow(`
		SELECT minio_bucket
		FROM users
		WHERE username = $1`, userName).Scan(&minioBucket)
	if err != nil {
		if err == pgx.ErrNoRows {
			log.Printf("No known Minio bucket for user '%s'\n", userName)
			return "", errors.New("No known Minio bucket for that user")
		} else {
			log.Printf("Error when looking up Minio bucket name for user '%v': %v\n", userName, err)
			return "", err
		}
	}

	return minioBucket, nil
}

// Return the Minio bucket and ID for a given database. dbOwner & dbName are from owner/database URL fragment,
// loggedInUser is the name for the currently logged in user, for access permission check.  Use an empty string ("")
// as the loggedInUser parameter if the true value isn't set or known.  If the requested database doesn't exist, or
// the loggedInUser doesn't have access to it, then an error will be returned.
func MinioBucketID(dbOwner string, dbName string, dbVersion int, loggedInUser string) (bkt string, id string, err error) {
	var dbQuery string
	if loggedInUser != dbOwner {
		// The request is for another users database, so it needs to be a public one
		dbQuery = `
			SELECT db.minio_bucket, ver.minioid
			FROM database_versions AS ver, sqlite_databases AS db
			WHERE ver.db = db.idnum
				AND db.username = $1
				AND db.dbname = $2
				AND ver.version = $3
				AND db.public = true`
	} else {
		dbQuery = `
			SELECT db.minio_bucket, ver.minioid
			FROM database_versions AS ver, sqlite_databases AS db
			WHERE ver.db = db.idnum
				AND db.username = $1
				AND db.dbname = $2
				AND ver.version = $3`
	}
	err = pdb.QueryRow(dbQuery, dbOwner, dbName, dbVersion).Scan(&bkt, &id)
	if err != nil {
		log.Printf("Error retrieving MinioID for %s/%s version %v: %v\n", dbOwner, dbName, dbVersion, err)
		return "", "", err
	}

	if bkt == "" || id == "" {
		// The requested database doesn't exist, or the logged in user doesn't have access to it
		return "", "", errors.New("The requested database wasn't found")
	}

	return bkt, id, nil
}

// Return the user's preference for maximum number of SQLite rows to display.
func PrefUserMaxRows(loggedInUser string) int {
	// Retrieve the user preference data
	dbQuery := `
		SELECT pref_max_rows
		FROM users
		WHERE username = $1`
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
			SELECT DISTINCT ON (username) username, last_modified
			FROM sqlite_databases
			WHERE public = true
			ORDER BY username, last_modified DESC
		)
		SELECT username, last_modified
		FROM public_dbs
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

// Remove a database version from PostgreSQL.
func RemoveDBVersion(dbOwner string, folder string, dbName string, dbVersion int) error {
	dbQuery := `
		DELETE from database_versions
		WHERE db  = (	SELECT idnum
				FROM sqlite_databases
				WHERE username = $1
					AND folder = $2
					AND dbname = $3)
			AND version = $4`
	commandTag, err := pdb.Exec(dbQuery, dbOwner, folder, dbName, dbVersion)
	if err != nil {
		log.Printf("%s: Removing database entry '%s' / '%s' / '%s' version %v failed: %v\n",
			dbOwner, folder, dbName, dbVersion, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong # of rows (%v) affected when removing database entry for '%s' / '%s' / '%s' version %v\n",
			numRows, dbOwner, folder, dbName, dbVersion)
		return nil
	}

	// Check if other versions of the database still exist
	dbQuery = `
		SELECT count(*) FROM database_versions
		WHERE db  = (	SELECT idnum
				FROM sqlite_databases
				WHERE username = $1
					AND folder = $2
					AND dbname = $3)`
	var numDBs int
	err = pdb.QueryRow(dbQuery, dbOwner, folder, dbName).Scan(&numDBs)
	if err != nil {
		// A real database error occurred
		log.Printf("Error checking if any further versions of database exist: %v\n", err)
		return err
	}

	// The database still has other versions, so there's nothing further to do
	if numDBs != 0 {
		return nil
	}

	// We removed the last version of the database, so now clean up the entry in the sqlite_databases table
	dbQuery = `
		DELETE FROM sqlite_databases
		WHERE username = $1
			AND folder = $2
			AND dbname = $3`
	commandTag, err = pdb.Exec(dbQuery, dbOwner, folder, dbName)
	if err != nil {
		log.Printf("%s: Removing main entry for '%s' / '%s' / '%s' failed: %v\n", dbOwner, folder,
			dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong # of rows (%v) affected when removing main database entry for '%s' / '%s' / '%s'\n",
			numRows, dbOwner, folder, dbName)
	}

	return nil
}

// Rename a SQLite daatabase.
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
func SaveDBSettings(userName string, dbFolder string, dbName string, descrip string, readme string, defTable string, public bool) error {
	// Check for values which should be NULL
	var nullableDescrip, nullableReadme pgx.NullString
	if descrip == "" {
		nullableDescrip.Valid = false
	} else {
		nullableDescrip.String = descrip
		nullableDescrip.Valid = true
	}
	if readme == "" {
		nullableReadme.Valid = false
	} else {
		nullableReadme.String = readme
		nullableReadme.Valid = true
	}

	// Save the database settings
	SQLQuery := `
		UPDATE sqlite_databases
		SET description = $4, readme = $5, default_table = $6, public = $7
		WHERE username = $1
			AND folder = $2
			AND dbname = $3`
	commandTag, err := pdb.Exec(SQLQuery, userName, dbFolder, dbName, nullableDescrip, nullableReadme, defTable, public)
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
	err = InvalidateCacheEntry(userName, userName, dbFolder, dbName, 0) // 0 indicates "for all versions"
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
		SET client_certificate = $1
		WHERE username = $2`
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
func SetPrefUserMaxRows(userName string, maxRows int) error {
	dbQuery := `
		UPDATE users
		SET pref_max_rows = $1
		WHERE username = $2`
	commandTag, err := pdb.Exec(dbQuery, maxRows, userName)
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

// Set the email address for a user.
func SetUserEmail(userName string, email string) error {
	dbQuery := `
		UPDATE users
		SET email = $1
		WHERE username = $2`
	commandTag, err := pdb.Exec(dbQuery, email, userName)
	if err != nil {
		log.Printf("Updating user email failed: %v\n", err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong # of rows affected (%v) when updating details for user '%v'\n", numRows, userName)
	}

	return nil
}

// Set the email address and password hash for a user.
func SetUserEmailPHash(userName string, email string, pHash []byte) error {
	dbQuery := `
		UPDATE users
		SET email = $1, password_hash = $2
		WHERE username = $3`
	commandTag, err := pdb.Exec(dbQuery, email, pHash, userName)
	if err != nil {
		log.Printf("Updating user email & password hash failed: %v\n", err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong # of rows affected (%v) when updating details for user '%v'\n", numRows, userName)
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
		WHERE username = $1
			AND folder = $2
			AND dbname = $3`
	err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&st)
	if err != nil {
		log.Printf("Error retrieving star count for '%s%s%s': %v\n", dbOwner, dbFolder,
			dbName, err)
		return -1, -1, -1, err
	}

	// Retrieve latest fork count
	dbQuery = `
		SELECT forks
		FROM sqlite_databases
		WHERE idnum = (
			SELECT root_database
			FROM sqlite_databases
			WHERE username = $1
			AND folder = $2
			AND dbname = $3)`
	err = pdb.QueryRow(dbQuery, dbOwner, dbFolder, dbName).Scan(&fo)
	if err != nil {
		log.Printf("Error retrieving fork count for '%s%s%s': %v\n", dbOwner, dbFolder,
			dbName, err)
		return -1, -1, -1, err
	}

	// TODO: Implement watchers
	return 0, st, fo, nil
}

// Toggle on or off the starring of a database by a user.
func ToggleDBStar(loggedInUser string, dbOwner string, dbFolder string, dbName string) error {
	// Check if the database is already starred
	starred, err := CheckDBStarred(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		return err
	}

	// Get the ID number of the database
	dbID, err := databaseID(dbOwner, dbName)
	if err != nil {
		return err
	}

	// Add or remove the star
	if !starred {
		// Star the database
		insertQuery := `
			INSERT INTO database_stars (db, username)
			VALUES ($1, $2)`
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
		WHERE db = $1
			AND username = $2`
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
			SELECT count(db)
			FROM database_stars
			WHERE db = $1
		) WHERE idnum = $1`
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

// Returns details for a user.
func User(userName string) (user UserDetails, err error) {
	dbQuery := `
		SELECT username, email, password_hash, date_joined, client_certificate
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
	WITH dbs AS (
		SELECT db.dbname, db.folder, db.date_created, db.last_modified, ver.size, ver.version, db.public,
			ver.sha256, db.watchers, db.stars, db.discussions, db.pull_requests, db.updates, db.branches,
			db.releases, db.contributors, db.description
		FROM sqlite_databases AS db, database_versions AS ver
		WHERE db.idnum = ver.db
			AND db.username = $1`
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
		ORDER BY dbname, version DESC
	), unique_dbs AS (
		SELECT DISTINCT ON (dbname) * FROM dbs ORDER BY dbname
	)
	SELECT * FROM unique_dbs ORDER BY last_modified DESC`
	rows, err := pdb.Query(dbQuery, userName)
	if err != nil {
		log.Printf("Getting list of databases for user failed: %v\n", err)
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var desc pgx.NullString
		var oneRow DBInfo
		err = rows.Scan(&oneRow.Database, &oneRow.Folder, &oneRow.DateCreated, &oneRow.LastModified,
			&oneRow.Size, &oneRow.Version, &oneRow.Public, &oneRow.SHA256, &oneRow.Watchers, &oneRow.Stars,
			&oneRow.Discussions, &oneRow.MRs, &oneRow.Updates, &oneRow.Branches, &oneRow.Releases,
			&oneRow.Contributors, &desc)
		if err != nil {
			log.Printf("Error retrieving database list for user: %v\n", err)
			return nil, err
		}
		if !desc.Valid {
			oneRow.Description = ""
		} else {
			oneRow.Description = fmt.Sprintf(": %s", desc.String)
		}
		list = append(list, oneRow)
	}

	// Get fork count for each of the databases
	for i, j := range list {
		// Retrieve latest fork count
		dbQuery = `
		SELECT forks
		FROM sqlite_databases
		WHERE idnum = (
			SELECT root_database
			FROM sqlite_databases
			WHERE username = $1
			AND folder = $2
			AND dbname = $3)`
		err = pdb.QueryRow(dbQuery, userName, j.Folder, j.Database).Scan(&list[i].Forks)
		if err != nil {
			log.Printf("Error retrieving fork count for '%s%s%s': %v\n", userName, j.Folder,
				j.Database, err)
			return nil, err
		}
	}

	return list, nil
}

// Remove the user from the database.  This automatically removes their entries from sqlite_databases too, due
// to the ON DELETE CASCADE referential integrity constraint.
func UserDelete(userName string) error {
	dbQuery := `
		DELETE FROM users
		WHERE username = $1`
	commandTag, err := pdb.Exec(dbQuery, userName)
	if err != nil {
		log.Printf("Deleting user '%s' from the database failed: %v\n", userName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong # of rows affected (%v) when deleting user '%s'\n", numRows, userName)
		return err
	}

	return nil
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
		SELECT username
		FROM users
		WHERE auth0id = $1`
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

// Returns the password hash for a user.
func UserPasswordHash(userName string) ([]byte, error) {
	row := pdb.QueryRow("SELECT password_hash FROM public.users WHERE username = $1", userName)
	var passHash []byte
	err := row.Scan(&passHash)
	if err != nil {
		log.Printf("Error looking up password hash for username '%s'. Error: %v\n", userName, err)
		return nil, err
	}

	return passHash, nil
}

// Returns the list of databases starred by a user.
func UserStarredDBs(userName string) (list []DBEntry, err error) {
	dbQuery := `
		WITH stars AS (
			SELECT db, date_starred
			FROM database_stars
			WHERE username = $1
		)
		SELECT dbs.username, dbs.dbname, stars.date_starred
		FROM sqlite_databases AS dbs, stars
		WHERE dbs.idnum = stars.db
		ORDER BY date_starred DESC`
	rows, err := pdb.Query(dbQuery, userName)
	if err != nil {
		log.Printf("Database query failed: %v\n", err)
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var oneRow DBEntry
		err = rows.Scan(&oneRow.Owner, &oneRow.DBName, &oneRow.DateEntry)
		if err != nil {
			log.Printf("Error retrieving stars list for user: %v\n", err)
			return nil, err
		}
		list = append(list, oneRow)
	}

	return list, nil
}

// Returns the list of users who starred a database.
func UsersStarredDB(dbOwner string, dbName string) (list []DBEntry, err error) {
	dbQuery := `
		WITH star_users AS (
			SELECT DISTINCT ON (username) username, date_starred
			FROM database_stars
			WHERE db = (
				SELECT idnum
				FROM sqlite_databases
				WHERE username = $1
					AND dbname = $2
				)
			ORDER BY username DESC
		)
		SELECT username, date_starred
		FROM star_users
		ORDER BY date_starred DESC`
	rows, err := pdb.Query(dbQuery, dbOwner, dbName)
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
