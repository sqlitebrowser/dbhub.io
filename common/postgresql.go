package common

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aquilax/truncate"
	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	pgpool "github.com/jackc/pgx/v5/pgxpool"
	"github.com/smtp2go-oss/smtp2go-go"
	gfm "github.com/sqlitebrowser/github_flavored_markdown"
	"golang.org/x/crypto/bcrypt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

var (
	// PostgreSQL connection pool handle
	pdb *pgpool.Pool
)

// AddDefaultUser adds the default user to the system, so the referential integrity of licence user_id 0 works
func AddDefaultUser() error {
	// Add the new user to the database
	dbQuery := `
		INSERT INTO users (auth0_id, user_name, email, password_hash, client_cert, display_name)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_name)
			DO NOTHING`
	_, err := pdb.Exec(context.Background(), dbQuery, RandomString(16), "default", "default@dbhub.io",
		RandomString(16), "", "Default system user")
	if err != nil {
		log.Printf("Error when adding the default user to the database: %v", err)
		// For now, don't bother logging a failure here.  This *might* need changing later on
		return err
	}

	// Log addition of the default user
	log.Println("Default user added")
	return nil
}

// AddUser adds a user to the system
func AddUser(auth0ID, userName, password, email, displayName, avatarURL string) error {
	// Hash the user's password
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Failed to hash user password. User: '%v', error: %v.", SanitiseLogString(userName), err)
		return err
	}

	// Generate a new HTTPS client certificate for the user
	cert := []byte("no cert")
	if Conf.Sign.Enabled {
		cert, err = GenerateClientCert(userName)
		if err != nil {
			log.Printf("Error when generating client certificate for '%s': %v", SanitiseLogString(userName), err)
			return err
		}
	}

	// If the display name or avatar URL are an empty string, we insert a NULL instead
	var av, dn pgtype.Text
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
	commandTag, err := pdb.Exec(context.Background(), insertQuery, auth0ID, userName, email, hash, cert, dn, av)
	if err != nil {
		log.Printf("Adding user to database failed: %v", err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows affected when creating user: %v, username: %v", numRows, SanitiseLogString(userName))
	}

	// Log the user registration
	log.Printf("User registered: '%s' Email: '%s'", SanitiseLogString(userName), email)
	return nil
}

// AnalysisRecordUserStorage adds a record to the backend database containing the amount of storage space used by a user
func AnalysisRecordUserStorage(userName string, recordDate time.Time, spaceUsedStandard, spaceUsedLive int64) (err error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		INSERT INTO analysis_space_used (user_id, analysis_date, standard_databases_bytes, live_databases_bytes)
		VALUES ((SELECT user_id FROM u), $2, $3, $4)`
	commandTag, err := pdb.Exec(context.Background(), dbQuery, userName, recordDate, spaceUsedStandard, spaceUsedLive)
	if err != nil {
		log.Printf("Adding record of storage space used by '%s' failed: %s", userName, err)
		return
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when recording the storage space used by '%s'", numRows, userName)
	}
	return
}

// AnalysisUsersWithDBs returns the list of users with at least one database
func AnalysisUsersWithDBs() (userList map[string]int, err error) {
	dbQuery := `
		SELECT u.user_name, count(*)
		FROM users u, sqlite_databases db
		WHERE u.user_id = db.user_id
		GROUP BY u.user_name`
	rows, err := pdb.Query(context.Background(), dbQuery)
	if err != nil {
		log.Printf("Database query failed in %s: %v", GetCurrentFunctionName(), err)
		return
	}
	defer rows.Close()
	userList = make(map[string]int)
	for rows.Next() {
		var user string
		var numDBs int
		err = rows.Scan(&user, &numDBs)
		if err != nil {
			log.Printf("Error in %s when getting the list of users with at least one database: %v", GetCurrentFunctionName(), err)
			return nil, err
		}
		userList[user] = numDBs
	}
	return
}

// ApiCallLog records an API call operation.  Database name is optional, as not all API calls operate on a
// database.  If a database name is provided however, then the database owner name *must* also be provided
func ApiCallLog(loggedInUser, dbOwner, dbName, operation, callerSw string) {
	var dbQuery string
	var err error
	var commandTag pgconn.CommandTag
	if dbName != "" {
		dbQuery = `
		WITH loggedIn AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), owner AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($2)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, owner
			WHERE db.user_id = owner.user_id
				AND db.db_name = $3)
		INSERT INTO api_call_log (caller_id, db_owner_id, db_id, api_operation, api_caller_sw)
		VALUES ((SELECT user_id FROM loggedIn), (SELECT user_id FROM owner), (SELECT db_id FROM d), $4, $5)`
		commandTag, err = pdb.Exec(context.Background(), dbQuery, loggedInUser, dbOwner, dbName, operation, callerSw)
	} else {
		dbQuery = `
		WITH loggedIn AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		INSERT INTO api_call_log (caller_id, api_operation, api_caller_sw)
		VALUES ((SELECT user_id FROM loggedIn), $2, $3)`
		commandTag, err = pdb.Exec(context.Background(), dbQuery, loggedInUser, operation, callerSw)
	}
	if err != nil {
		log.Printf("Adding api call log entry failed: %s", err)
		return
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when adding api call entry for user '%s'", numRows, SanitiseLogString(loggedInUser))
	}
}

// APIKeySave saves a new API key to the PostgreSQL database
func APIKeySave(key, loggedInUser string, dateCreated time.Time) error {
	// Make sure the API key isn't already in the database
	dbQuery := `
		SELECT count(key)
		FROM api_keys
		WHERE key = $1`
	var keyCount int
	err := pdb.QueryRow(context.Background(), dbQuery, key).Scan(&keyCount)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("Checking if an API key exists failed: %s", err)
		return err
	}
	if keyCount != 0 {
		// API key is already in our system
		log.Printf("Duplicate API key (%s) generated for user '%s'", key, loggedInUser)
		return fmt.Errorf("API generator created duplicate key.  Try again, just in case...")
	}

	// Add the new API key to the database
	dbQuery = `
		INSERT INTO api_keys (user_id, key, date_created)
		SELECT (SELECT user_id FROM users WHERE lower(user_name) = lower($1)), $2, $3`
	commandTag, err := pdb.Exec(context.Background(), dbQuery, loggedInUser, key, dateCreated)
	if err != nil {
		log.Printf("Adding API key to database failed: %v", err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when adding API key: %v, username: %v", numRows, key,
			loggedInUser)
	}
	return nil
}

// CheckDBExists checks if a database exists. It does NOT perform any permission checks.
// If an error occurred, the true/false value should be ignored, as only the error value is valid
func CheckDBExists(dbOwner, dbName string) (bool, error) {
	// Query matching databases
	dbQuery := `
		SELECT COUNT(db_id)
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db_name = $2
			AND is_deleted = false
		LIMIT 1`
	var dbCount int
	err := pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&dbCount)
	if err != nil {
		return false, err
	}

	// Return true if the database count is not zero
	return dbCount != 0, nil
}

// CheckDBLive checks if the given database is a live database
func CheckDBLive(dbOwner, dbName string) (isLive bool, liveNode string, err error) {
	// Query matching databases
	dbQuery := `
		SELECT live_db, coalesce(live_node, '')
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db_name = $2
			AND is_deleted = false
		LIMIT 1`
	err = pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&isLive, &liveNode)
	if err != nil {
		return false, "", err
	}
	return
}

// CheckDBPermissions checks if a database exists and can be accessed by the given user.
// If an error occurred, the true/false value should be ignored, as only the error value is valid
func CheckDBPermissions(loggedInUser, dbOwner, dbName string, writeAccess bool) (bool, error) {
	// Query id and public flag of the database
	dbQuery := `
		SELECT db_id, public
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db_name = $2
			AND is_deleted = false
		LIMIT 1`
	var dbId int
	var dbPublic bool
	err := pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&dbId, &dbPublic)

	// There are two possible error cases: no rows returned or another error.
	// If no rows were returned the database simply does not exist and no error is returned to the caller.
	// If there was another, actual error this error is returned to the caller.
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	// If we get here this means that the database does exist. The next step is to check the permissions.

	if strings.ToLower(loggedInUser) == strings.ToLower(dbOwner) {
		// If the request is from the owner of the database, always allow access to the database
		return true, nil
	} else if writeAccess == false && dbPublic {
		// Read access to public databases is always permitted
		return true, nil
	} else if loggedInUser == "" {
		// If the user is not logged in and we reach this point, access is not permitted
		return false, nil
	}

	// If the request is from someone who is logged in but not the owner of the database, check
	// if the database is shared with the logged in user.

	// Query shares
	dbQuery = `
		SELECT access
		FROM database_shares
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db_id = $2
		LIMIT 1`
	var dbAccess ShareDatabasePermissions
	err = pdb.QueryRow(context.Background(), dbQuery, loggedInUser, dbId).Scan(&dbAccess)

	// Check if there are any shares. If not, don't allow access.
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	// If there are shares, check the permissions
	if writeAccess {
		// If write access is required, only return true if writing is allowed
		return dbAccess == MayReadAndWrite, nil
	}

	// If no write access is required, always return true if there is a share for this database and user
	return true, nil
}

// CheckDBID checks if a given database ID is available, and returns its name so the caller can determine if it
// has been renamed.  If an error occurs, the true/false value should be ignored, as only the error value is valid
func CheckDBID(dbOwner string, dbID int64) (avail bool, dbName string, err error) {
	dbQuery := `
		SELECT db_name
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db_id = $2
			AND is_deleted = false`
	err = pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbID).Scan(&dbName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			avail = false
		} else {
			log.Printf("Checking if a database exists failed: %v", err)
		}
		return
	}

	// Database exists
	avail = true
	return
}

// CheckDBStarred check if a database has been starred by a given user.  The boolean return value is only valid when
// err is nil
func CheckDBStarred(loggedInUser, dbOwner, dbName string) (bool, error) {
	dbQuery := `
		SELECT count(db_id)
		FROM database_stars
		WHERE database_stars.user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($3)
			)
			AND database_stars.db_id = (
					SELECT db_id
					FROM sqlite_databases
					WHERE user_id = (
							SELECT user_id
							FROM users
							WHERE lower(user_name) = lower($1)
						)
						AND db_name = $2
						AND is_deleted = false)`
	var starCount int
	err := pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbName, loggedInUser).Scan(&starCount)
	if err != nil {
		log.Printf("Error looking up star count for database. User: '%s' DB: '%s/%s'. Error: %v",
			loggedInUser, SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
		return true, err
	}
	if starCount == 0 {
		// Database hasn't been starred by the user
		return false, nil
	}

	// Database HAS been starred by the user
	return true, nil
}

// CheckDBWatched checks if a database is being watched by a given user.  The boolean return value is only valid when
// err is nil
func CheckDBWatched(loggedInUser, dbOwner, dbName string) (bool, error) {
	dbQuery := `
		SELECT count(db_id)
		FROM watchers
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($3)
			)
			AND db_id = (
					SELECT db_id
					FROM sqlite_databases
					WHERE user_id = (
							SELECT user_id
							FROM users
							WHERE lower(user_name) = lower($1)
						)
						AND db_name = $2
						AND is_deleted = false)`
	var watchCount int
	err := pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbName, loggedInUser).Scan(&watchCount)
	if err != nil {
		log.Printf("Error looking up watchers count for database. User: '%s' DB: '%s/%s'. Error: %v",
			loggedInUser, SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
		return true, err
	}
	if watchCount == 0 {
		// Database isn't being watched by the user
		return false, nil
	}

	// Database IS being watched by the user
	return true, nil
}

// CheckEmailExists checks if an email address already exists in our system. Returns true if the email is already in
// the system, false if not.  If an error occurred, the true/false value should be ignored, as only the error value
// is valid
func CheckEmailExists(email string) (bool, error) {
	// Check if the email address is already in our system
	dbQuery := `
		SELECT count(user_name)
		FROM users
		WHERE email = $1`
	var emailCount int
	err := pdb.QueryRow(context.Background(), dbQuery, email).Scan(&emailCount)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return true, err
	}
	if emailCount == 0 {
		// Email address isn't yet in our system
		return false, nil
	}

	// Email address IS already in our system
	return true, nil

}

// CheckLicenceExists checks if a given licence exists in our system
func CheckLicenceExists(userName, licenceName string) (exists bool, err error) {
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
	err = pdb.QueryRow(context.Background(), dbQuery, userName, licenceName).Scan(&count)
	if err != nil {
		log.Printf("Error checking if licence '%s' exists for user '%s' in database: %v",
			SanitiseLogString(licenceName), userName, err)
		return false, err
	}
	if count == 0 {
		// The requested licence wasn't found
		return false, nil
	}
	return true, nil
}

// CheckUserExists checks if a username already exists in our system.  Returns true if the username is already taken,
// false if not.  If an error occurred, the true/false value should be ignored, and only the error return code used
func CheckUserExists(userName string) (bool, error) {
	dbQuery := `
		SELECT count(user_id)
		FROM users
		WHERE lower(user_name) = lower($1)`
	var userCount int
	err := pdb.QueryRow(context.Background(), dbQuery, userName).Scan(&userCount)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return true, err
	}
	if userCount == 0 {
		// Username isn't in system
		return false, nil
	}
	// Username IS in system
	return true, nil
}

// ConnectPostgreSQL creates a connection pool to the PostgreSQL server
func ConnectPostgreSQL() (err error) {
	pdb, err = pgpool.New(context.Background(), pgConfig.ConnString())
	if err != nil {
		return fmt.Errorf("Couldn't connect to PostgreSQL server: %v", err)
	}

	// migrate doesn't handle pgx connection strings, so we need to manually create something it can use
	var mConnStr string
	if Conf.Environment.Environment == "production" {
		mConnStr = fmt.Sprintf("pgx5://%s@%s:%d/%s?password=%s&connect_timeout=10", Conf.Pg.Username, Conf.Pg.Server,
			uint16(Conf.Pg.Port), Conf.Pg.Database, url.PathEscape(Conf.Pg.Password))
	} else {
		// Non-production, so probably our Docker test container
		mConnStr = "pgx5://dbhub@localhost:5432/dbhub"
	}
	if Conf.Pg.SSL {
		mConnStr += "&sslmode=require"
	}
	m, err := migrate.New(fmt.Sprintf("file://%s/database/migrations", Conf.Web.BaseDir), mConnStr)
	if err != nil {
		return
	}

	// Bizarrely, migrate throws a "no change" error when there are no migrations to apply.  So, we work around it:
	// https://github.com/golang-migrate/migrate/issues/485
	err = m.Up()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return
	}

	// Log successful connection
	log.Printf("Connected to PostgreSQL server: %v:%v", Conf.Pg.Server, uint16(Conf.Pg.Port))
	return nil
}

// databaseID returns the ID number for a given user's database
func databaseID(dbOwner, dbName string) (dbID int, err error) {
	// Retrieve the database id
	dbQuery := `
		SELECT db_id
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1))
			AND db_name = $2
			AND is_deleted = false`
	err = pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&dbID)
	if err != nil {
		log.Printf("Error looking up database id. Owner: '%s', Database: '%s'. Error: %v",
			SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
	}
	return
}

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
	rows, err := pdb.Query(context.Background(), dbQuery, loggedInUser)
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
	rows, err = pdb.Query(context.Background(), dbQuery, loggedInUser)
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

// DBDetails returns the details for a specific database
func DBDetails(DB *SQLiteDBinfo, loggedInUser, dbOwner, dbName, commitID string) (err error) {
	// Check permissions first
	allowed, err := CheckDBPermissions(loggedInUser, dbOwner, dbName, false)
	if err != nil {
		return err
	}
	if allowed == false {
		return fmt.Errorf("The requested database doesn't exist")
	}

	// First, we check if the database is a live one.  If it is, we need to do things a bit differently
	isLive, _, err := CheckDBLive(dbOwner, dbName)
	if err != nil {
		return
	}
	if !isLive {
		// * This is a standard database *

		// If no commit ID was supplied, we retrieve the latest one from the default branch
		if commitID == "" {
			commitID, err = DefaultCommit(dbOwner, dbName)
			if err != nil {
				return err
			}
		}

		// Retrieve the database details
		dbQuery := `
			SELECT db.date_created, db.last_modified, db.watchers, db.stars, db.discussions, db.merge_requests,
				$3::text AS commit_id, db.commit_list->$3::text->'tree'->'entries'->0 AS db_entry, db.branches,
				db.release_count, db.contributors, coalesce(db.one_line_description, ''),
				coalesce(db.full_description, 'No full description'), coalesce(db.default_table, ''), db.public,
				coalesce(db.source_url, ''), db.tags, coalesce(db.default_branch, ''), db.live_db,
				coalesce(db.live_node, ''), coalesce(db.live_minio_object_id, '')
			FROM sqlite_databases AS db
			WHERE db.user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND db.db_name = $2
				AND db.is_deleted = false`

		// Retrieve the requested database details
		err = pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbName, commitID).Scan(&DB.Info.DateCreated, &DB.Info.RepoModified,
			&DB.Info.Watchers, &DB.Info.Stars, &DB.Info.Discussions, &DB.Info.MRs, &DB.Info.CommitID, &DB.Info.DBEntry,
			&DB.Info.Branches, &DB.Info.Releases, &DB.Info.Contributors, &DB.Info.OneLineDesc, &DB.Info.FullDesc,
			&DB.Info.DefaultTable, &DB.Info.Public, &DB.Info.SourceURL, &DB.Info.Tags, &DB.Info.DefaultBranch,
			&DB.Info.IsLive, &DB.Info.LiveNode, &DB.MinioId)
		if err != nil {
			log.Printf("Error when retrieving database details: %v", err.Error())
			return errors.New("The requested database doesn't exist")
		}
	} else {
		// This is a live database
		dbQuery := `
			SELECT db.date_created, db.last_modified, db.watchers, db.stars, db.discussions, coalesce(db.one_line_description, ''),
				coalesce(db.full_description, 'No full description'), coalesce(db.default_table, ''), db.public,
				coalesce(db.source_url, ''), coalesce(db.default_branch, ''), coalesce(db.live_node, ''),
				coalesce(db.live_minio_object_id, '')
			FROM sqlite_databases AS db
			WHERE db.user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND db.db_name = $2
				AND db.is_deleted = false`

		// Retrieve the requested database details
		err = pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&DB.Info.DateCreated,
			&DB.Info.RepoModified, &DB.Info.Watchers, &DB.Info.Stars, &DB.Info.Discussions, &DB.Info.OneLineDesc,
			&DB.Info.FullDesc, &DB.Info.DefaultTable, &DB.Info.Public, &DB.Info.SourceURL, &DB.Info.DefaultBranch,
			&DB.Info.LiveNode, &DB.MinioId)
		if err != nil {
			log.Printf("Error when retrieving database details: %v", err.Error())
			return errors.New("The requested database doesn't exist")
		}
		DB.Info.IsLive = true
	}

	// If an sha256 was in the licence field, retrieve its friendly name and url for displaying
	licSHA := DB.Info.DBEntry.LicenceSHA
	if licSHA != "" {
		DB.Info.Licence, DB.Info.LicenceURL, err = GetLicenceInfoFromSha256(dbOwner, licSHA)
		if err != nil {
			return err
		}
	} else {
		DB.Info.Licence = "Not specified"
	}

	// Retrieve correctly capitalised username for the database owner
	usrOwner, err := User(dbOwner)
	if err != nil {
		return err
	}

	// Fill out the fields we already have data for
	DB.Info.Database = dbName
	DB.Info.Owner = usrOwner.Username

	// The social stats are always updated because they could change without the cache being updated
	DB.Info.Watchers, DB.Info.Stars, DB.Info.Forks, err = SocialStats(dbOwner, dbName)
	if err != nil {
		return err
	}

	// Retrieve the latest discussion and MR counts
	DB.Info.Discussions, DB.Info.MRs, err = GetDiscussionAndMRCount(dbOwner, dbName)
	if err != nil {
		return err
	}

	// Retrieve the "forked from" information
	DB.Info.ForkOwner, DB.Info.ForkDatabase, DB.Info.ForkDeleted, err = ForkedFrom(dbOwner, dbName)
	if err != nil {
		return err
	}

	// Check if the database was starred by the logged in user
	DB.Info.MyStar, err = CheckDBStarred(loggedInUser, dbOwner, dbName)
	if err != nil {
		return err
	}

	// Check if the database is being watched by the logged in user
	DB.Info.MyWatch, err = CheckDBWatched(loggedInUser, dbOwner, dbName)
	if err != nil {
		return err
	}
	return nil
}

// DBStars returns the star count for a given database
func DBStars(dbOwner, dbName string) (starCount int, err error) {
	// Retrieve the updated star count
	dbQuery := `
		SELECT stars
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db_name = $2
			AND is_deleted = false`
	err = pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&starCount)
	if err != nil {
		log.Printf("Error looking up star count for database '%s/%s'. Error: %v", SanitiseLogString(dbOwner),
			SanitiseLogString(dbName), err)
		return -1, err
	}
	return starCount, nil
}

// DBWatchers returns the watchers count for a given database
func DBWatchers(dbOwner, dbName string) (watcherCount int, err error) {
	// Retrieve the updated watchers count
	dbQuery := `
		SELECT watchers
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db_name = $2
			AND is_deleted = false`
	err = pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&watcherCount)
	if err != nil {
		log.Printf("Error looking up watcher count for database '%s/%s'. Error: %v",
			SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
		return -1, err
	}
	return watcherCount, nil
}

// DefaultCommit returns the default commit ID for a specific database
func DefaultCommit(dbOwner, dbName string) (commitID string, err error) {
	// If no commit ID was supplied, we retrieve the latest commit ID from the default branch
	dbQuery := `
		SELECT branch_heads->default_branch->>'commit'::text AS commit_id
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
			)
			AND db_name = $2
			AND is_deleted = false`
	var c pgtype.Text
	err = pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&c)
	if err != nil {
		log.Printf("Error when retrieving head commit ID of default branch: %v", err.Error())
		return "", errors.New("Internal error when looking up database details")
	}
	if c.Valid {
		commitID = c.String
	}
	return commitID, nil
}

// DeleteComment deletes a specific comment from a discussion
func DeleteComment(dbOwner, dbName string, discID, comID int) error {
	// Begin a transaction
	tx, err := pdb.Begin(context.Background())
	if err != nil {
		return err
	}
	// Set up an automatic transaction roll back if the function exits without committing
	defer tx.Rollback(context.Background())

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
				AND db_name = $2
		), int AS (
			SELECT internal_id AS int_id
			FROM discussions
			WHERE db_id = (SELECT db_id FROM d)
			AND disc_id = $3
		)
		DELETE FROM discussion_comments
		WHERE db_id = (SELECT db_id FROM d)
			AND disc_id = (SELECT int_id FROM int)
			AND com_id = $4`
	commandTag, err := tx.Exec(context.Background(), dbQuery, dbOwner, dbName, discID, comID)
	if err != nil {
		log.Printf("Deleting comment '%d' from '%s/%s', discussion '%d' failed: %v", comID,
			SanitiseLogString(dbOwner), SanitiseLogString(dbName), discID, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when deleting comment '%d' from database '%s/%s, discussion '%d''",
			numRows, comID, SanitiseLogString(dbOwner), SanitiseLogString(dbName), discID)
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
				AND db_name = $2
		), int AS (
			SELECT internal_id AS int_id
			FROM discussions
			WHERE db_id = (SELECT db_id FROM d)
			AND disc_id = $3
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
	commandTag, err = tx.Exec(context.Background(), dbQuery, dbOwner, dbName, discID)
	if err != nil {
		log.Printf("Updating comment count for discussion '%v' of '%s/%s' in PostgreSQL failed: %v",
			discID, SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when updating comment count for discussion '%v' in "+
			"'%s/%s'", numRows, discID, SanitiseLogString(dbOwner), SanitiseLogString(dbName))
	}

	// Commit the transaction
	err = tx.Commit(context.Background())
	if err != nil {
		return err
	}

	return nil
}

// DeleteDatabase deletes a database from PostgreSQL
// Note that we leave a stub/placeholder entry for all uploaded databases in PG, so our stats don't miss data over time
// and so the dependant table data doesn't go weird.  We also set the "is_deleted" boolean to true for its entry, so
// our database query functions know to skip it
func DeleteDatabase(dbOwner, dbName string) error {
	// Is this a live database
	isLive, _, err := CheckDBLive(dbOwner, dbName)
	if err != nil {
		return err
	}

	// Begin a transaction
	tx, err := pdb.Begin(context.Background())
	if err != nil {
		return err
	}
	// Set up an automatic transaction roll back if the function exits without committing
	defer tx.Rollback(context.Background())

	// Remove all watchers for this database
	dbQuery := `
			DELETE FROM watchers
			WHERE db_id = (
					SELECT db_id
					FROM sqlite_databases
					WHERE user_id = (
							SELECT user_id
							FROM users
							WHERE lower(user_name) = lower($1)
						)
						AND db_name = $2
				)`
	commandTag, err := tx.Exec(context.Background(), dbQuery, dbOwner, dbName)
	if err != nil {
		log.Printf("Removing all watchers for database '%s/%s' failed: Error '%s'", SanitiseLogString(dbOwner),
			SanitiseLogString(dbName), err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong # of rows affected (%v) when removing all watchers for database '%s/%s'", numRows,
			SanitiseLogString(dbOwner), SanitiseLogString(dbName))
	}

	// Check if there are any forks of this database
	dbQuery = `
		WITH this_db AS (
			SELECT db_id
			FROM sqlite_databases
			WHERE user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND db_name = $2
		)
		SELECT count(*)
		FROM sqlite_databases AS db, this_db
		WHERE db.forked_from = this_db.db_id`
	var numForks int
	err = tx.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&numForks)
	if err != nil {
		log.Printf("Retrieving fork list failed for database '%s/%s': %s", SanitiseLogString(dbOwner),
			SanitiseLogString(dbName), err)
		return err
	}
	if numForks == 0 {
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
					AND db_name = $2
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
		commandTag, err := tx.Exec(context.Background(), dbQuery, dbOwner, dbName)
		if err != nil {
			log.Printf("Updating fork count for '%s/%s' in PostgreSQL failed: %s", SanitiseLogString(dbOwner),
				SanitiseLogString(dbName), err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 && !isLive { // Skip this check when deleting live databases
			log.Printf("Wrong number of rows (%d) affected (spot 1) when updating fork count for database '%s/%s'",
				numRows, SanitiseLogString(dbOwner), SanitiseLogString(dbName))
		}

		// Generate a random string to be used in the deleted database's name field, so if the user adds a database with
		// the deleted one's name then the unique constraint on the database won't reject it
		newName := "deleted-database-" + RandomString(20)

		// Mark the database as deleted in PostgreSQL, replacing the entry with the ~randomly generated name
		dbQuery = `
			UPDATE sqlite_databases AS db
			SET is_deleted = true, public = false, db_name = $3, last_modified = now()
			WHERE user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND db_name = $2`
		commandTag, err = tx.Exec(context.Background(), dbQuery, dbOwner, dbName, newName)
		if err != nil {
			log.Printf("Deleting (forked) database entry failed for database '%s/%s': %v",
				SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf(
				"Wrong number of rows (%d) affected when deleting (forked) database '%s/%s'", numRows,
				SanitiseLogString(dbOwner), SanitiseLogString(dbName))
		}

		// Commit the transaction
		err = tx.Commit(context.Background())
		if err != nil {
			return err
		}

		// Log the database deletion
		log.Printf("Database '%s/%s' deleted", SanitiseLogString(dbOwner), SanitiseLogString(dbName))
		return nil
	}

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
				AND db_name = $2
			)`
	commandTag, err = tx.Exec(context.Background(), dbQuery, dbOwner, dbName)
	if err != nil {
		log.Printf("Deleting (forked) database stars failed for database '%s/%s': %v",
			SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
		return err
	}

	// Generate a random string to be used in the deleted database's name field, so if the user adds a database with
	// the deleted one's name then the unique constraint on the database won't reject it
	newName := "deleted-database-" + RandomString(20)

	// Replace the database entry in sqlite_databases with a stub
	dbQuery = `
		UPDATE sqlite_databases AS db
		SET is_deleted = true, public = false, db_name = $3, last_modified = now()
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db_name = $2`
	commandTag, err = tx.Exec(context.Background(), dbQuery, dbOwner, dbName, newName)
	if err != nil {
		log.Printf("Deleting (forked) database entry failed for database '%s/%s': %v",
			SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf(
			"Wrong number of rows (%d) affected when deleting (forked) database '%s/%s'", numRows,
			SanitiseLogString(dbOwner), SanitiseLogString(dbName))
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
				AND db_name = $2
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
	commandTag, err = tx.Exec(context.Background(), dbQuery, dbOwner, newName)
	if err != nil {
		log.Printf("Updating fork count for '%s/%s' in PostgreSQL failed: %v", SanitiseLogString(dbOwner),
			SanitiseLogString(dbName), err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected (spot 2) when updating fork count for database '%s/%s'",
			numRows, SanitiseLogString(dbOwner), SanitiseLogString(dbName))
	}

	// Commit the transaction
	err = tx.Commit(context.Background())
	if err != nil {
		return err
	}

	// Log the database deletion
	log.Printf("(Forked) database '%s/%s' deleted", SanitiseLogString(dbOwner), SanitiseLogString(dbName))
	return nil
}

// DeleteLicence removes a (user supplied) database licence from the system
func DeleteLicence(userName, licenceName string) (err error) {
	// Begin a transaction
	tx, err := pdb.Begin(context.Background())
	if err != nil {
		return err
	}
	// Set up an automatic transaction roll back if the function exits without committing
	defer tx.Rollback(context.Background())

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
	//        using straight PG SQL
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
	err = pdb.QueryRow(context.Background(), dbQuery, userName).Scan(&DBCount)
	if err != nil {
		log.Printf("Checking if the licence is in use failed: %v", err)
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
	commandTag, err := tx.Exec(context.Background(), dbQuery, userName, licSHA, licenceName)
	if err != nil {
		log.Printf("Error when retrieving sha256 for licence '%s', user '%s' from database: %v",
			SanitiseLogString(licenceName), userName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when deleting licence '%s' for user '%s'",
			numRows, SanitiseLogString(licenceName), userName)
	}

	// Commit the transaction
	err = tx.Commit(context.Background())
	if err != nil {
		return err
	}

	return nil
}

// DisconnectPostgreSQL disconnects the PostgreSQL database connection
func DisconnectPostgreSQL() {
	pdb.Close()

	// Log successful disconnection
	log.Printf("Disconnected from PostgreSQL server: %v:%v", Conf.Pg.Server, uint16(Conf.Pg.Port))
}

// Discussions returns the list of discussions or MRs for a given database
// If a non-0 discID value is passed, it will only return the details for that specific discussion/MR.  Otherwise, it
// will return a list of all discussions or MRs for a given database
// Note - This returns a slice of DiscussionEntry, instead of a map.  We use a slice because it lets us use an ORDER
//
//	BY clause in the SQL and preserve the returned order (maps don't preserve order).  If in future we no longer
//	need to preserve the order, it might be useful to switch to using a map instead since they're often simpler
//	to work with.
func Discussions(dbOwner, dbName string, discType DiscussionType, discID int) (list []DiscussionEntry, err error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
				AND db.db_name = $2)
		SELECT disc.disc_id, disc.title, disc.open, disc.date_created, users.user_name, users.email, users.avatar_url,
			disc.description, last_modified, comment_count, mr_source_db_id, mr_source_db_branch,
			mr_destination_branch, mr_state, mr_commits
		FROM discussions AS disc, d, users
		WHERE disc.db_id = d.db_id
			AND disc.discussion_type = $3
			AND disc.creator = users.user_id`
	if discID != 0 {
		dbQuery += fmt.Sprintf(`
			AND disc_id = %d`, discID)
	}
	dbQuery += `
		ORDER BY last_modified DESC`
	var rows pgx.Rows
	rows, err = pdb.Query(context.Background(), dbQuery, dbOwner, dbName, discType)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return
	}
	for rows.Next() {
		var av, em, sb, db pgtype.Text
		var sdb pgtype.Int8
		var oneRow DiscussionEntry
		err = rows.Scan(&oneRow.ID, &oneRow.Title, &oneRow.Open, &oneRow.DateCreated, &oneRow.Creator, &em, &av,
			&oneRow.Body, &oneRow.LastModified, &oneRow.CommentCount, &sdb, &sb, &db, &oneRow.MRDetails.State,
			&oneRow.MRDetails.Commits)
		if err != nil {
			log.Printf("Error retrieving discussion/MR list for database '%s/%s': %v",
				SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
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

	// For merge requests, turn the source database ID's into full owner/name strings
	if discType == MERGE_REQUEST {
		for i, j := range list {
			// Retrieve the owner/name for a database id
			dbQuery = `
				SELECT users.user_name, db.db_name
				FROM sqlite_databases AS db, users
				WHERE db.db_id = $1
					AND db.user_id = users.user_id`
			var o, n pgtype.Text
			err2 := pdb.QueryRow(context.Background(), dbQuery, j.MRDetails.SourceDBID).Scan(&o, &n)
			if err2 != nil && !errors.Is(err2, pgx.ErrNoRows) {
				log.Printf("Retrieving source database owner/name failed: %v", err)
				return
			}
			if o.Valid {
				list[i].MRDetails.SourceOwner = o.String
			}
			if n.Valid {
				list[i].MRDetails.SourceDBName = n.String
			}
		}
	}

	rows.Close()
	return
}

// DiscussionComments returns the list of comments for a given discussion
// If a non-0 comID value is passed, it will only return the details for that specific comment in the discussion.
// Otherwise it will return a list of all comments for a given discussion
// Note - This returns a slice instead of a map.  We use a slice because it lets us use an ORDER BY clause in the SQL
// and preserve the returned order (maps don't preserve order).  If in future we no longer need to preserve the
// order, it might be useful to switch to using a map instead since they're often simpler to work with.
func DiscussionComments(dbOwner, dbName string, discID, comID int) (list []DiscussionCommentEntry, err error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
				AND db.db_name = $2
		), int AS (
				SELECT internal_id AS int_id
				FROM discussions
				WHERE db_id = (SELECT db_id FROM d)
				AND disc_id = $3
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
	var rows pgx.Rows
	rows, err = pdb.Query(context.Background(), dbQuery, dbOwner, dbName, discID)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return
	}
	for rows.Next() {
		var av, em pgtype.Text
		var oneRow DiscussionCommentEntry
		err = rows.Scan(&oneRow.ID, &oneRow.Commenter, &em, &av, &oneRow.DateCreated, &oneRow.Body, &oneRow.EntryType)
		if err != nil {
			log.Printf("Error retrieving comment list for database '%s/%s', discussion '%d': %v",
				SanitiseLogString(dbOwner), SanitiseLogString(dbName), discID, err)
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

// FlushViewCount periodically flushes the database view count from Memcache to PostgreSQL
func FlushViewCount() {
	type dbEntry struct {
		Owner string
		Name  string
	}

	// Log the start of the loop
	log.Printf("Periodic view count flushing loop started.  %d second refresh.",
		Conf.Memcache.ViewCountFlushDelay)

	// Start the endless flush loop
	var rows pgx.Rows
	var err error
	for true {
		// Retrieve the list of all public databases
		dbQuery := `
			SELECT users.user_name, db.db_name
			FROM sqlite_databases AS db, users
			WHERE db.public = true
				AND db.is_deleted = false
				AND db.user_id = users.user_id`
		rows, err = pdb.Query(context.Background(), dbQuery)
		if err != nil {
			log.Printf("Database query failed: %v", err)
			return
		}
		var dbList []dbEntry
		for rows.Next() {
			var oneRow dbEntry
			err = rows.Scan(&oneRow.Owner, &oneRow.Name)
			if err != nil {
				log.Printf("Error retrieving database list for view count flush thread: %v", err)
				rows.Close()
				return
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
				commandTag, err := pdb.Exec(context.Background(), dbQuery, dbOwner, dbName, newValue)
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
		time.Sleep(Conf.Memcache.ViewCountFlushDelay * time.Second)
	}
	return
}

// ForkDatabase forks the PostgreSQL entry for a SQLite database from one user to another
func ForkDatabase(srcOwner, dbName, dstOwner string) (newForkCount int, err error) {
	// Copy the main database entry
	dbQuery := `
		WITH dst_u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		INSERT INTO sqlite_databases (user_id, db_name, public, forks, one_line_description, full_description,
			branches, contributors, root_database, default_table, source_url, commit_list, branch_heads, tags,
			default_branch, forked_from)
		SELECT dst_u.user_id, db_name, public, 0, one_line_description, full_description, branches,
			contributors, root_database, default_table, source_url, commit_list, branch_heads, tags, default_branch,
			db_id
		FROM sqlite_databases, dst_u
		WHERE sqlite_databases.user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($2)
			)
			AND db_name = $3`
	commandTag, err := pdb.Exec(context.Background(), dbQuery, dstOwner, srcOwner, dbName)
	if err != nil {
		log.Printf("Forking database '%s/%s' in PostgreSQL failed: %v", SanitiseLogString(srcOwner),
			SanitiseLogString(dbName), err)
		return 0, err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows affected (%d) when forking main database entry: "+
			"'%s/%s' to '%s/%s'", numRows, SanitiseLogString(srcOwner), SanitiseLogString(dbName),
			dstOwner, SanitiseLogString(dbName))
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
				AND db_name = $2
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
	err = pdb.QueryRow(context.Background(), dbQuery, dstOwner, dbName).Scan(&newForkCount)
	if err != nil {
		log.Printf("Updating fork count in PostgreSQL failed: %v", err)
		return 0, err
	}
	return newForkCount, nil
}

// ForkedFrom checks if the given database was forked from another, and if so returns that one's owner and
// database name
func ForkedFrom(dbOwner, dbName string) (forkOwn, forkDB string, forkDel bool, err error) {
	// Check if the database was forked from another
	var dbID, forkedFrom pgtype.Int8
	dbQuery := `
		SELECT db_id, forked_from
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1))
			AND db_name = $2`
	err = pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&dbID, &forkedFrom)
	if err != nil {
		log.Printf("Error checking if database was forked from another '%s/%s'. Error: %v",
			SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
		return "", "", false, err
	}
	if !forkedFrom.Valid {
		// The database wasn't forked, so return empty strings
		return "", "", false, nil
	}

	// Return the details of the database this one was forked from
	dbQuery = `
		SELECT u.user_name, db.db_name, db.is_deleted
		FROM users AS u, sqlite_databases AS db
		WHERE db.db_id = $1
			AND u.user_id = db.user_id`
	err = pdb.QueryRow(context.Background(), dbQuery, forkedFrom).Scan(&forkOwn, &forkDB, &forkDel)
	if err != nil {
		log.Printf("Error retrieving forked database information for '%s/%s'. Error: %v",
			SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
		return "", "", false, err
	}

	// If the database this one was forked from has been deleted, indicate that and clear the database name value
	if forkDel {
		forkDB = ""
	}
	return forkOwn, forkDB, forkDel, nil
}

// ForkParent returns the parent of a database, if there is one (and it's accessible to the logged in user).  If no
// parent was found, the returned Owner/DBName values will be empty strings
func ForkParent(loggedInUser, dbOwner, dbName string) (parentOwner, parentDBName string, err error) {
	dbQuery := `
		SELECT users.user_name, db.db_name, db.public, db.db_id, db.forked_from, db.is_deleted
		FROM sqlite_databases AS db, users
		WHERE db.root_database = (
				SELECT root_database
				FROM sqlite_databases
				WHERE user_id = (
						SELECT user_id
						FROM users
						WHERE lower(user_name) = lower($1)
					)
					AND db_name = $2
				)
			AND db.user_id = users.user_id
		ORDER BY db.forked_from NULLS FIRST`
	rows, err := pdb.Query(context.Background(), dbQuery, dbOwner, dbName)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return
	}
	defer rows.Close()
	dbList := make(map[int]ForkEntry)
	for rows.Next() {
		var frk pgtype.Int8
		var oneRow ForkEntry
		err = rows.Scan(&oneRow.Owner, &oneRow.DBName, &oneRow.Public, &oneRow.ID, &frk, &oneRow.Deleted)
		if err != nil {
			log.Printf("Error retrieving fork parent for '%s/%s': %v", dbOwner, dbName,
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
		err = fmt.Errorf("Empty list returned instead of fork tree.  This shouldn't happen.")
		return
	}

	// Get the ID of the database being called
	dbID, err := databaseID(dbOwner, dbName)
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
			parentDBName = dbEntry.DBName
			break
		}
	}
	return
}

// ForkTree returns the complete fork tree for a given database
func ForkTree(loggedInUser, dbOwner, dbName string) (outputList []ForkEntry, err error) {
	dbQuery := `
		SELECT users.user_name, db.db_name, db.public, db.db_id, db.forked_from, db.is_deleted
		FROM sqlite_databases AS db, users
		WHERE db.root_database = (
				SELECT root_database
				FROM sqlite_databases
				WHERE user_id = (
						SELECT user_id
						FROM users
						WHERE lower(user_name) = lower($1)
					)
					AND db_name = $2
				)
			AND db.user_id = users.user_id
		ORDER BY db.forked_from NULLS FIRST`
	rows, err := pdb.Query(context.Background(), dbQuery, dbOwner, dbName)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return nil, err
	}
	defer rows.Close()
	var dbList []ForkEntry
	for rows.Next() {
		var frk pgtype.Int8
		var oneRow ForkEntry
		err = rows.Scan(&oneRow.Owner, &oneRow.DBName, &oneRow.Public, &oneRow.ID, &frk, &oneRow.Deleted)
		if err != nil {
			log.Printf("Error retrieving fork list for '%s/%s': %v", dbOwner, dbName, err)
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
		return nil, errors.New("Empty list returned instead of fork tree.  This shouldn't happen.")
	}
	if dbList[0].ForkedFrom != 0 {
		// The first entry has a non-zero forked_from field, indicating it's not the root entry.  That
		// shouldn't happen, so return an error.
		return nil, errors.New("Incorrect root entry data in retrieved database list.")
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

// GetActivityStats returns the latest activity stats
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
	starRows, err := pdb.Query(context.Background(), dbQuery)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return
	}
	defer starRows.Close()
	for starRows.Next() {
		var oneRow ActivityRow
		err = starRows.Scan(&oneRow.Owner, &oneRow.DBName, &oneRow.Count)
		if err != nil {
			log.Printf("Error retrieving list of most starred databases: %v", err)
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
	forkRows, err := pdb.Query(context.Background(), dbQuery)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return
	}
	defer forkRows.Close()
	for forkRows.Next() {
		var oneRow ActivityRow
		err = forkRows.Scan(&oneRow.Owner, &oneRow.DBName, &oneRow.Count)
		if err != nil {
			log.Printf("Error retrieving list of most forked databases: %v", err)
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
	upRows, err := pdb.Query(context.Background(), dbQuery)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return
	}
	defer upRows.Close()
	for upRows.Next() {
		var oneRow UploadRow
		err = upRows.Scan(&oneRow.Owner, &oneRow.DBName, &oneRow.UploadDate)
		if err != nil {
			log.Printf("Error retrieving list of most recent uploads: %v", err)
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
	dlRows, err := pdb.Query(context.Background(), dbQuery)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return
	}
	defer dlRows.Close()
	for dlRows.Next() {
		var oneRow ActivityRow
		err = dlRows.Scan(&oneRow.Owner, &oneRow.DBName, &oneRow.Count)
		if err != nil {
			log.Printf("Error retrieving list of most downloaded databases: %v", err)
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
	viewRows, err := pdb.Query(context.Background(), dbQuery)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return
	}
	defer viewRows.Close()
	for viewRows.Next() {
		var oneRow ActivityRow
		err = viewRows.Scan(&oneRow.Owner, &oneRow.DBName, &oneRow.Count)
		if err != nil {
			log.Printf("Error retrieving list of most viewed databases: %v", err)
			return
		}
		stats.Viewed = append(stats.Viewed, oneRow)
	}
	return
}

// GetBranches load the branch heads for a database
// TODO: It might be better to have the default branch name be returned as part of this list, by indicating in the list
// TODO  which of the branches is the default.
func GetBranches(dbOwner, dbName string) (branches map[string]BranchEntry, err error) {
	dbQuery := `
		SELECT db.branch_heads
		FROM sqlite_databases AS db
		WHERE db.user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db.db_name = $2`
	err = pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&branches)
	if err != nil {
		log.Printf("Error when retrieving branch heads for database '%s/%s': %v", SanitiseLogString(dbOwner),
			SanitiseLogString(dbName), err)
		return nil, err
	}
	return branches, nil
}

// GetAPIKeys returns the list of API keys for a user
func GetAPIKeys(user string) ([]APIKey, error) {
	dbQuery := `
		SELECT key, date_created
		FROM api_keys
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)`
	rows, err := pdb.Query(context.Background(), dbQuery, user)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return nil, err
	}
	defer rows.Close()
	var keys []APIKey
	for rows.Next() {
		var key string
		var dateCreated time.Time
		err = rows.Scan(&key, &dateCreated)
		if err != nil {
			log.Printf("Error retrieving API key list: %v", err)
			return nil, err
		}
		keys = append(keys, APIKey{Key: key, DateCreated: dateCreated})
	}
	return keys, nil
}

// GetAPIKeyUser returns the owner of a given API key.  Returns an empty string if the key has no known owner
func GetAPIKeyUser(key string) (user string, err error) {
	dbQuery := `
		SELECT user_name
		FROM api_keys AS api, users
		WHERE api.key = $1
			AND api.user_id = users.user_id`
	err = pdb.QueryRow(context.Background(), dbQuery, key).Scan(&user)
	if err != nil {
		return
	}
	return
}

// GetCommitList returns the full commit list for a database
func GetCommitList(dbOwner, dbName string) (map[string]CommitEntry, error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		SELECT commit_list as commits
		FROM sqlite_databases AS db, u
		WHERE db.user_id = u.user_id
			AND db.db_name = $2
			AND db.is_deleted = false`
	var l map[string]CommitEntry
	err := pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&l)
	if err != nil {
		log.Printf("Retrieving commit list for '%s/%s' failed: %v", SanitiseLogString(dbOwner),
			SanitiseLogString(dbName), err)
		return map[string]CommitEntry{}, err
	}
	return l, nil
}

// GetDefaultBranchName returns the default branch name for a database
func GetDefaultBranchName(dbOwner, dbName string) (branchName string, err error) {
	dbQuery := `
		SELECT db.default_branch
		FROM sqlite_databases AS db
		WHERE db.user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db.db_name = $2
			AND db.is_deleted = false`
	var b pgtype.Text
	err = pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&b)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("Error when retrieving default branch name for database '%s/%s': %v",
				SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
		} else {
			log.Printf("No default branch name exists for database '%s/%s'. This shouldn't happen",
				SanitiseLogString(dbOwner), SanitiseLogString(dbName))
		}
		return
	}
	if b.Valid {
		branchName = b.String
	}
	return
}

// GetDefaultTableName returns the default table name for a database
func GetDefaultTableName(dbOwner, dbName string) (tableName string, err error) {
	dbQuery := `
		SELECT db.default_table
		FROM sqlite_databases AS db
		WHERE db.user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db.db_name = $2
			AND db.is_deleted = false`
	var t pgtype.Text
	err = pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&t)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("Error when retrieving default table name for database '%s/%s': %v",
				SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
			return
		}
	}
	if t.Valid {
		tableName = t.String
	}
	return
}

// GetDiscussionAndMRCount returns the discussion and merge request counts for a database
// TODO: The only reason this function exists atm, is because we're incorrectly caching the discussion and MR data in
// TODO  a way that makes invalidating it correctly hard/impossible.  We should redo our memcached approach to solve the
// TODO  issue properly
func GetDiscussionAndMRCount(dbOwner, dbName string) (discCount, mrCount int, err error) {
	dbQuery := `
		SELECT db.discussions, db.merge_requests
		FROM sqlite_databases AS db
		WHERE db.user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db.db_name = $2
			AND db.is_deleted = false`
	err = pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&discCount, &mrCount)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("Error when retrieving discussion and MR count for database '%s/%s': %v",
				SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
		} else {
			log.Printf("Database '%s/%s' not found when attempting to retrieve discussion and MR count. This"+
				"shouldn't happen", SanitiseLogString(dbOwner), SanitiseLogString(dbName))
		}
		return
	}
	return
}

// GetLicence returns the text for a given licence
func GetLicence(userName, licenceName string) (txt, format string, err error) {
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
	err = pdb.QueryRow(context.Background(), dbQuery, userName, licenceName).Scan(&txt, &format)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The requested licence text wasn't found
			return "", "", errors.New("unknown licence")
		}
		log.Printf("Error when retrieving licence '%s', user '%s': %v", SanitiseLogString(licenceName), userName, err)
		return "", "", err
	}
	return txt, format, nil
}

// GetLicences returns the list of licences available to a user
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
	rows, err := pdb.Query(context.Background(), dbQuery, user)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return nil, err
	}
	defer rows.Close()
	lics := make(map[string]LicenceEntry)
	for rows.Next() {
		var name string
		var oneRow LicenceEntry
		err = rows.Scan(&name, &oneRow.FullName, &oneRow.Sha256, &oneRow.URL, &oneRow.FileFormat, &oneRow.Order)
		if err != nil {
			log.Printf("Error retrieving licence list: %v", err)
			return nil, err
		}
		lics[name] = oneRow
	}
	return lics, nil
}

// GetLicenceInfoFromSha256 returns the friendly name + licence URL for the licence matching a given sha256
// Note - When user defined licence has the same sha256 as a default one we return the user defined licences' friendly
// name
func GetLicenceInfoFromSha256(userName, sha256 string) (lName, lURL string, err error) {
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
	rows, err := pdb.Query(context.Background(), dbQuery, userName, sha256)
	if err != nil {
		log.Printf("Error when retrieving friendly name for licence sha256 '%s', user '%s': %v", sha256,
			SanitiseLogString(userName), err)
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
			log.Printf("Error retrieving friendly name for licence sha256 '%s', user: %v", sha256, err)
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
		return "", "", fmt.Errorf("Multiple matching licences found, but belonging to user %s", userName)
	}

	// To get here we must have successfully picked a user defined licence out of several matches.  This seems like
	// an acceptable scenario
	return lName, lURL, nil
}

// GetLicenceSha256FromName returns the sha256 for a given licence
func GetLicenceSha256FromName(userName, licenceName string) (sha256 string, err error) {
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
	err = pdb.QueryRow(context.Background(), dbQuery, userName, licenceName).Scan(&sha256)
	if err != nil {
		log.Printf("Error when retrieving sha256 for licence '%s', user '%s' from database: %v",
			SanitiseLogString(licenceName), userName, err)
		return "", err
	}
	if sha256 == "" {
		// The requested licence wasn't found
		return "", errors.New("Licence not found")
	}
	return sha256, nil
}

// GetReleases returns the list of releases for a database
func GetReleases(dbOwner, dbName string) (releases map[string]ReleaseEntry, err error) {
	dbQuery := `
		SELECT release_list
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db_name = $2`
	err = pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&releases)
	if err != nil {
		log.Printf("Error when retrieving releases for database '%s/%s': %v", SanitiseLogString(dbOwner),
			SanitiseLogString(dbName), err)
		return nil, err
	}
	if releases == nil {
		// If there aren't any releases yet, return an empty set instead of nil
		releases = make(map[string]ReleaseEntry)
	}
	return releases, nil
}

// GetShares returns a map with all users for which the given database is shared as key and their
// permissions as value.
func GetShares(dbOwner, dbName string) (shares map[string]ShareDatabasePermissions, err error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
			AND db_name = $2
		)
		SELECT usr.user_name, share.access
		FROM database_shares AS share, d, users AS usr
		WHERE share.db_id = d.db_id AND usr.user_id = share.user_id
		ORDER BY usr.user_name`
	rows, e := pdb.Query(context.Background(), dbQuery, dbOwner, dbName)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return nil, e
	}
	defer rows.Close()

	shares = make(map[string]ShareDatabasePermissions)

	var name string
	var access ShareDatabasePermissions
	for rows.Next() {
		err = rows.Scan(&name, &access)
		if err != nil {
			return
		}
		shares[name] = access
	}
	return
}

// GetSharesForUser returns a list of all the databases shared with the given user, and their permissions.
func GetSharesForUser(userName string) (shares []ShareDatabasePermissionsUser, err error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		SELECT users.user_name, db.db_name, db.live_db, shares.access
		FROM database_shares AS shares, sqlite_databases AS db, u, users
		WHERE shares.user_id = u.user_id
			AND shares.db_id = db.db_id
			AND db.user_id = users.user_id
			AND db.is_deleted = false
		ORDER by users.user_name, db.db_name`
	rows, e := pdb.Query(context.Background(), dbQuery, userName)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return nil, e
	}
	defer rows.Close()
	var x ShareDatabasePermissionsUser
	for rows.Next() {
		err = rows.Scan(&x.OwnerName, &x.DBName, &x.IsLive, &x.Permission)
		if err != nil {
			return
		}
		shares = append(shares, x)
	}
	return
}

// GetTags returns the tags for a database
func GetTags(dbOwner, dbName string) (tags map[string]TagEntry, err error) {
	dbQuery := `
		SELECT tag_list
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db_name = $2`
	err = pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&tags)
	if err != nil {
		log.Printf("Error when retrieving tags for database '%s/%s': %v", SanitiseLogString(dbOwner),
			SanitiseLogString(dbName), err)
		return nil, err
	}
	if tags == nil {
		// If there aren't any tags yet, return an empty set instead of nil
		tags = make(map[string]TagEntry)
	}
	return tags, nil
}

// GetUsernameFromEmail returns the username associated with an email address
func GetUsernameFromEmail(email string) (userName, avatarURL string, err error) {
	dbQuery := `
		SELECT user_name, avatar_url
		FROM users
		WHERE email = $1`
	var av pgtype.Text
	err = pdb.QueryRow(context.Background(), dbQuery, email).Scan(&userName, &av)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No matching username of the email
			err = nil
			return
		}
		log.Printf("Looking up username for email address '%s' failed: %v", SanitiseLogString(email), err)
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

// GetVisualisations returns the saved visualisations for a given database
func GetVisualisations(dbOwner, dbName string) (visualisations map[string]VisParamsV2, err error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
				AND db_name = $2
		)
		SELECT name, parameters
		FROM vis_params as vis, u, d
		WHERE vis.db_id = d.db_id
			AND vis.user_id = u.user_id
		ORDER BY name`
	rows, e := pdb.Query(context.Background(), dbQuery, dbOwner, dbName)
	if e != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// There weren't any saved visualisations for this database
			return
		}

		// A real database error occurred
		err = e
		log.Printf("Retrieving visualisation list for '%s/%s' failed: %v", dbOwner, dbName, e)
		return
	}
	defer rows.Close()

	visualisations = make(map[string]VisParamsV2)
	for rows.Next() {
		var n string
		var p VisParamsV2
		err = rows.Scan(&n, &p)
		if err != nil {
			log.Printf("Error retrieving visualisation list: %v", err.Error())
			return
		}

		visualisations[n] = p
	}
	return
}

// IncrementDownloadCount increments the download count for a database
func IncrementDownloadCount(dbOwner, dbName string) error {
	dbQuery := `
		UPDATE sqlite_databases
		SET download_count = download_count + 1
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db_name = $2`
	commandTag, err := pdb.Exec(context.Background(), dbQuery, dbOwner, dbName)
	if err != nil {
		log.Printf("Increment download count for '%s/%s' failed: %v", SanitiseLogString(dbOwner),
			SanitiseLogString(dbName), err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		errMsg := fmt.Sprintf("Wrong number of rows affected (%v) when incrementing download count for '%s/%s'",
			numRows, dbOwner, dbName)
		log.Printf(SanitiseLogString(errMsg))
		return errors.New(errMsg)
	}
	return nil
}

// LiveAddDatabasePG adds the details for a live database to PostgreSQL
func LiveAddDatabasePG(dbOwner, dbName, bucketName, liveNode string, accessType SetAccessType) (err error) {
	// Figure out new public/private access setting
	var public bool
	switch accessType {
	case SetToPublic:
		public = true
	case SetToPrivate:
		public = false
	default:
		err = errors.New("Error: Unknown public/private setting requested for a new live database.  Aborting.")
		return
	}

	var commandTag pgconn.CommandTag
	dbQuery := `
		WITH root AS (
			SELECT nextval('sqlite_databases_db_id_seq') AS val
		)
		INSERT INTO sqlite_databases (user_id, db_id, db_name, public, live_db, live_node, live_minio_object_id)
		SELECT (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)), (SELECT val FROM root), $2, $3, true, $4, $5`
	commandTag, err = pdb.Exec(context.Background(), dbQuery, dbOwner, dbName, public, liveNode, bucketName)
	if err != nil {
		log.Printf("Storing LIVE database '%s/%s' failed: %s", SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected while storing LIVE database '%s/%s'", numRows,
			SanitiseLogString(dbOwner), SanitiseLogString(dbName))
	}
	return nil
}

// LiveGenerateMinioNames generates Minio bucket and object names for a live database
func LiveGenerateMinioNames(userName string) (bucketName, objectName string, err error) {
	// If the user already has a Minio bucket name assigned, then we use it
	z, err := User(userName)
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
		commandTag, err = pdb.Exec(context.Background(), dbQuery, userName, bucketName)
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
	usr, err := User(dbOwner)
	if err != nil {
		return
	}

	// Retrieve database details
	var db SQLiteDBinfo
	err = DBDetails(&db, loggedInUser, dbOwner, dbName, "")
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

// LiveSqlHistoryAdd adds a new record to the history of recently executed SQL statements
func LiveSqlHistoryAdd(loggedInUser, dbOwner, dbName, stmt string, state SqlHistoryItemStates, result interface{}) (err error) {
	// Delete old records. We want to keep 100 records, so delete all but 99 and add one new in the next step
	// TODO Make this number configurable or something
	err = LiveSqlHistoryDeleteOld(loggedInUser, dbOwner, dbName, 99)
	if err != nil {
		return err
	}

	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
				AND db.db_name = $2
		), l AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($3)
		)
		INSERT INTO sql_terminal_history (user_id, db_id, sql_stmt, state, result)
		VALUES ((SELECT user_id FROM l), (SELECT db_id FROM d), $4, $5,  $6)`
	commandTag, err := pdb.Exec(context.Background(), dbQuery, dbOwner, dbName, loggedInUser, stmt, state, result)
	if err != nil {
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected while saving SQL statement for user '%s'", numRows,
			SanitiseLogString(loggedInUser))
	}
	return
}

// LiveSqlHistoryDeleteOld deletes all saved SQL statements in the SQL history table, except for the most recent ones
func LiveSqlHistoryDeleteOld(loggedInUser, dbOwner, dbName string, keepRecords int) (err error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
				AND db.db_name = $2
		), l AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($3)
		)
		DELETE FROM sql_terminal_history
		WHERE history_id NOT IN (
			SELECT h.history_id FROM sql_terminal_history h, u, d, l WHERE h.user_id=u.user_id AND h.db_id=d.db_id
			ORDER BY h.history_id DESC LIMIT $4
		)`
	_, err = pdb.Exec(context.Background(), dbQuery, dbOwner, dbName, loggedInUser, keepRecords)
	if err != nil {
		return err
	}
	return
}

// LiveSqlHistoryGet returns the list of recently executed SQL statement for a user and database
func LiveSqlHistoryGet(loggedInUser, dbOwner, dbName string) (history []SqlHistoryItem, err error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
				AND db.db_name = $2
		), l AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($3)
		)
		SELECT h.sql_stmt, h.result, h.state
		FROM sql_terminal_history h, l, d, u
		WHERE h.user_id=l.user_id AND h.db_id=d.db_id
		ORDER BY history_id ASC`
	rows, err := pdb.Query(context.Background(), dbQuery, dbOwner, dbName, loggedInUser)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item SqlHistoryItem
		err = rows.Scan(&item.Statement, &item.Result, &item.State)
		if err != nil {
			return nil, err
		}

		history = append(history, item)
	}
	return
}

// LiveUserDBs returns the list of live databases owned by the user
func LiveUserDBs(dbOwner string, public AccessType) (list []DBInfo, err error) {
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
	case DB_PUBLIC:
		// Only public databases
		dbQuery += ` AND public = true`
	case DB_PRIVATE:
		// Only private databases
		dbQuery += ` AND public = false`
	case DB_BOTH:
		// Both public and private, so no need to add a query clause
	default:
		// This clause shouldn't ever be reached
		return nil, fmt.Errorf("Incorrect 'public' value '%v' passed to LiveUserDBs() function.", public)
	}
	dbQuery += " ORDER BY date_created DESC"

	rows, err := pdb.Query(context.Background(), dbQuery, dbOwner)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var oneRow DBInfo
		var liveNode string
		err = rows.Scan(&oneRow.Database, &oneRow.DateCreated, &oneRow.RepoModified, &oneRow.Public, &oneRow.IsLive, &liveNode,
			&oneRow.Watchers, &oneRow.Stars, &oneRow.Discussions, &oneRow.Contributors,
			&oneRow.OneLineDesc, &oneRow.SourceURL, &oneRow.Downloads, &oneRow.Views)
		if err != nil {
			log.Printf("Error when retrieving list of live databases for user '%s': %v", dbOwner, err)
			return nil, err
		}

		// Ask the AMQP backend for the database file size
		oneRow.Size, err = LiveSize(liveNode, dbOwner, dbOwner, oneRow.Database)
		if err != nil {
			log.Printf("Error when retrieving size of live databases for user '%s': %v", dbOwner, err)
			return nil, err
		}

		list = append(list, oneRow)
	}
	return
}

// LogDB4SConnect creates a DB4S default browse list entry
func LogDB4SConnect(userAcc, ipAddr, userAgent string, downloadDate time.Time) error {
	if Conf.DB4S.Debug {
		log.Printf("User '%s' just connected with '%s' and generated the default browse list", userAcc, SanitiseLogString(userAgent))
	}

	// If the user account isn't "public", then we look up the user id and store the info with the request
	userID := 0
	if userAcc != "public" {
		dbQuery := `
			SELECT user_id
			FROM users
			WHERE user_name = $1`

		err := pdb.QueryRow(context.Background(), dbQuery, userAcc).Scan(&userID)
		if err != nil {
			log.Printf("Looking up the user ID failed: %v", err)
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
	commandTag, err := pdb.Exec(context.Background(), dbQuery, userID, ipAddr, userAgent, downloadDate)
	if err != nil {
		log.Printf("Storing record of DB4S connection failed: %v", err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected while storing DB4S connection record for user '%s'", numRows, userAcc)
	}
	return nil
}

// LogDownload creates a download log entry
func LogDownload(dbOwner, dbName, loggedInUser, ipAddr, serverSw, userAgent string, downloadDate time.Time, sha string) error {
	// If the downloader isn't a logged in user, use a NULL value for that column
	var downloader pgtype.Text
	if loggedInUser != "" {
		downloader.String = loggedInUser
		downloader.Valid = true
	}

	// Store the download details
	dbQuery := `
		WITH d AS (
			SELECT db.db_id, db.db_name
			FROM sqlite_databases AS db
			WHERE user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND db.db_name = $2
		)
		INSERT INTO database_downloads (db_id, user_id, ip_addr, server_sw, user_agent, download_date, db_sha256)
		SELECT (SELECT db_id FROM d), (SELECT user_id FROM users WHERE lower(user_name) = lower($3)), $4, $5, $6, $7, $8`
	commandTag, err := pdb.Exec(context.Background(), dbQuery, dbOwner, dbName, downloader, ipAddr, serverSw, userAgent,
		downloadDate, sha)
	if err != nil {
		log.Printf("Storing record of download '%s/%s', sha '%s' by '%v' failed: %v", SanitiseLogString(dbOwner),
			SanitiseLogString(dbName), sha, downloader, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected while storing download record for '%s/%s'", numRows,
			SanitiseLogString(dbOwner), SanitiseLogString(dbName))
	}
	return nil
}

// LogSQLiteQueryAfter adds memory allocation stats for the execution run of a user supplied SQLite query
func LogSQLiteQueryAfter(insertID, memUsed, memHighWater int64) (err error) {
	dbQuery := `
		UPDATE vis_query_runs
		SET memory_used = $2, memory_high_water = $3
		WHERE query_run_id = $1`
	commandTag, err := pdb.Exec(context.Background(), dbQuery, insertID, memUsed, memHighWater)
	if err != nil {
		log.Printf("Adding memory stats for SQLite query run '%d' failed: %v", insertID, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected while adding memory stats for SQLite query run '%d'",
			numRows, insertID)
	}
	return nil
}

// LogSQLiteQueryBefore logs the basic info for a user supplied SQLite query
func LogSQLiteQueryBefore(source, dbOwner, dbName, loggedInUser, ipAddr, userAgent, query string) (int64, error) {
	// If the user isn't logged in, use a NULL value for that column
	var queryUser pgtype.Text
	if loggedInUser != "" {
		queryUser.String = loggedInUser
		queryUser.Valid = true
	}

	// Base64 encode the SQLite query string, just to be as safe as possible
	encodedQuery := base64.StdEncoding.EncodeToString([]byte(query))

	// Store the query details
	dbQuery := `
		WITH d AS (
			SELECT db.db_id, db.db_name
			FROM sqlite_databases AS db
			WHERE user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND db.db_name = $2
		)
		INSERT INTO vis_query_runs (db_id, user_id, ip_addr, user_agent, query_string, source)
		SELECT (SELECT db_id FROM d), (SELECT user_id FROM users WHERE lower(user_name) = lower($3)), $4, $5, $6, $7
		RETURNING query_run_id`
	var insertID int64
	err := pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbName, queryUser, ipAddr, userAgent, encodedQuery, source).Scan(&insertID)
	if err != nil {
		log.Printf("Storing record of user SQLite query '%v' on '%s/%s' failed: %v", encodedQuery,
			SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
		return 0, err
	}
	return insertID, nil
}

// LogUpload creates an upload log entry
func LogUpload(dbOwner, dbName, loggedInUser, ipAddr, serverSw, userAgent string, uploadDate time.Time, sha string) error {
	// If the uploader isn't a logged in user, use a NULL value for that column
	var uploader pgtype.Text
	if loggedInUser != "" {
		uploader.String = loggedInUser
		uploader.Valid = true
	}

	// Store the upload details
	dbQuery := `
		WITH d AS (
			SELECT db.db_id, db.db_name
			FROM sqlite_databases AS db
			WHERE user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND db.db_name = $2
		)
		INSERT INTO database_uploads (db_id, user_id, ip_addr, server_sw, user_agent, upload_date, db_sha256)
		SELECT (SELECT db_id FROM d), (SELECT user_id FROM users WHERE lower(user_name) = lower($3)), $4, $5, $6, $7, $8`
	commandTag, err := pdb.Exec(context.Background(), dbQuery, dbOwner, dbName, uploader, ipAddr, serverSw, userAgent,
		uploadDate, sha)
	if err != nil {
		log.Printf("Storing record of upload '%s/%s', sha '%s' by '%v' failed: %v", SanitiseLogString(dbOwner),
			SanitiseLogString(dbName), sha, uploader, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected while storing upload record for '%s/%s'", numRows,
			SanitiseLogString(dbOwner), SanitiseLogString(dbName))
	}
	return nil
}

// MinioLocation returns the Minio bucket and ID for a given database. dbOwner & dbName are from
// owner/database URL fragment, loggedInUser is the name for the currently logged in user, for access permission
// check.  Use an empty string ("") as the loggedInUser parameter if the true value isn't set or known.
// If the requested database doesn't exist, or the loggedInUser doesn't have access to it, then an error will be
// returned
func MinioLocation(dbOwner, dbName, commitID, loggedInUser string) (minioBucket, minioID string, lastModified time.Time, err error) {
	// Check permissions
	allowed, err := CheckDBPermissions(loggedInUser, dbOwner, dbName, false)
	if err != nil {
		return
	}
	if !allowed {
		err = errors.New("Database not found")
		return
	}

	// If no commit was provided, we grab the default one
	if commitID == "" {
		commitID, err = DefaultCommit(dbOwner, dbName)
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
	err = pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbName, commitID).Scan(&sha, &mod)
	if err != nil {
		log.Printf("Error retrieving MinioID for '%s/%s' version '%v' by logged in user '%v': %v",
			SanitiseLogString(dbOwner), SanitiseLogString(dbName), SanitiseLogString(commitID), loggedInUser, err)
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

// NewEvent adds an event entry to PostgreSQL
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
				AND db_name = $2
				AND is_deleted = false
		)
		INSERT INTO events (db_id, event_type, event_data)
		VALUES ((SELECT db_id FROM d), $3, $4)`
	_, err = pdb.Exec(context.Background(), dbQuery, details.Owner, details.DBName, details.Type, details)
	if err != nil {
		return err
	}
	return
}

// PrefUserMaxRows returns the user's preference for maximum number of SQLite rows to display.
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
	err := pdb.QueryRow(context.Background(), dbQuery, loggedInUser).Scan(&maxRows)
	if err != nil {
		log.Printf("Error retrieving user '%s' preference data: %v", loggedInUser, err)
		return DefaultNumDisplayRows // Use the default value
	}
	return maxRows
}

// RenameDatabase renames a SQLite database
func RenameDatabase(userName, dbName, newName string) error {
	// Save the database settings
	dbQuery := `
		UPDATE sqlite_databases
		SET db_name = $3
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db_name = $2`
	commandTag, err := pdb.Exec(context.Background(), dbQuery, userName, dbName, newName)
	if err != nil {
		log.Printf("Renaming database '%s/%s' failed: %v", SanitiseLogString(userName),
			SanitiseLogString(dbName), err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		errMsg := fmt.Sprintf("Wrong number of rows affected (%d) when renaming '%s/%s' to '%s/%s'",
			numRows, userName, dbName, userName, newName)
		log.Printf(SanitiseLogString(errMsg))
		return errors.New(errMsg)
	}

	// Log the rename
	log.Printf("Database renamed from '%s/%s' to '%s/%s'", SanitiseLogString(userName), SanitiseLogString(dbName),
		SanitiseLogString(userName), SanitiseLogString(newName))
	return nil
}

// ResetDB resets the database to its default state. eg for testing purposes
func ResetDB() error {
	// We probably don't want to drop the database itself, as that'd screw up the current database
	// connection.  Instead, lets truncate all the tables then load their default values
	tableNames := []string{
		"api_call_log",
		"api_keys",
		"database_downloads",
		"database_files",
		"database_licences",
		"database_shares",
		"database_stars",
		"database_uploads",
		"db4s_connects",
		"discussion_comments",
		"discussions",
		"email_queue",
		"events",
		"sql_terminal_history",
		"sqlite_databases",
		"users",
		"vis_params",
		"vis_query_runs",
		"vis_result_cache",
		"watchers",
	}

	sequenceNames := []string{
		"api_keys_key_id_seq",
		"api_log_log_id_seq",
		"database_downloads_dl_id_seq",
		"database_licences_lic_id_seq",
		"database_uploads_up_id_seq",
		"db4s_connects_connect_id_seq",
		"discussion_comments_com_id_seq",
		"discussions_disc_id_seq",
		"email_queue_email_id_seq",
		"events_event_id_seq",
		"sql_terminal_history_history_id_seq",
		"sqlite_databases_db_id_seq",
		"users_user_id_seq",
		"vis_query_runs_query_run_id_seq",
	}

	// Begin a transaction
	tx, err := pdb.Begin(context.Background())
	if err != nil {
		return err
	}
	// Set up an automatic transaction roll back if the function exits without committing
	defer tx.Rollback(context.Background())

	// Truncate the database tables
	for _, tbl := range tableNames {
		// Ugh, string smashing just feels so wrong when working with SQL
		dbQuery := fmt.Sprintf("TRUNCATE TABLE %s CASCADE", tbl)
		_, err := pdb.Exec(context.Background(), dbQuery)
		if err != nil {
			log.Printf("Error truncating table while resetting database: %s", err)
			return err
		}
	}

	// Reset the sequences
	for _, seq := range sequenceNames {
		dbQuery := fmt.Sprintf("ALTER SEQUENCE %v RESTART", seq)
		_, err := pdb.Exec(context.Background(), dbQuery)
		if err != nil {
			log.Printf("Error restarting sequence while resetting database: %v", err)
			return err
		}
	}

	// Add the default user to the system
	err = AddDefaultUser()
	if err != nil {
		log.Fatal(err)
	}

	// Add the default licences
	err = AddDefaultLicences()
	if err != nil {
		log.Fatal(err)
	}

	// Commit the transaction
	err = tx.Commit(context.Background())
	if err != nil {
		return err
	}

	// Log the database reset
	log.Println("Database reset")
	return nil
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
	commandTag, err := pdb.Exec(context.Background(), SQLQuery, userName, dbName, nullable1LineDesc, nullableFullDesc, defaultTable,
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
	if Conf.Event.Smtp2GoKey == "" && os.Getenv("SMTP2GO_API_KEY") == "" {
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
		rows, err := pdb.Query(context.Background(), dbQuery)
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
			commandTag, err := pdb.Exec(context.Background(), dbQuery, j.ID)
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

// SetClientCert stores a certificate for a given client
func SetClientCert(newCert []byte, userName string) error {
	SQLQuery := `
		UPDATE users
		SET client_cert = $1
		WHERE lower(user_name) = lower($2)`
	commandTag, err := pdb.Exec(context.Background(), SQLQuery, newCert, userName)
	if err != nil {
		log.Printf("Updating client certificate for '%s' failed: %v", userName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		errMsg := fmt.Sprintf("Wrong number of rows affected (%d) when storing client cert for '%s'", numRows, userName)
		log.Printf(errMsg)
		return errors.New(errMsg)
	}
	return nil
}

// SetUserPreferences sets the user's preference for maximum number of SQLite rows to display
func SetUserPreferences(userName string, maxRows int, displayName, email string) error {
	dbQuery := `
		UPDATE users
		SET pref_max_rows = $2, display_name = $3, email = $4
		WHERE lower(user_name) = lower($1)`
	commandTag, err := pdb.Exec(context.Background(), dbQuery, userName, maxRows, displayName, email)
	if err != nil {
		log.Printf("Updating user preferences failed for user '%s'. Error: '%v'", userName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong # of rows (%v) affected when updating user preferences. User: '%s'", numRows,
			userName)
	}
	return nil
}

// SocialStats returns the latest social stats for a given database
func SocialStats(dbOwner, dbName string) (wa, st, fo int, err error) {

	// TODO: Implement caching of these stats

	// Retrieve latest star, fork, and watcher count
	dbQuery := `
		SELECT stars, forks, watchers
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db_name = $2`
	err = pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&st, &fo, &wa)
	if err != nil {
		log.Printf("Error retrieving social stats count for '%s/%s': %v", SanitiseLogString(dbOwner),
			SanitiseLogString(dbName), err)
		return -1, -1, -1, err
	}
	return
}

// StatusUpdates returns the list of outstanding status updates for a user
func StatusUpdates(loggedInUser string) (statusUpdates map[string][]StatusUpdateEntry, err error) {
	dbQuery := `
		SELECT status_updates
		FROM users
		WHERE user_name = $1`
	err = pdb.QueryRow(context.Background(), dbQuery, loggedInUser).Scan(&statusUpdates)
	if err != nil {
		log.Printf("Error retrieving status updates list for user '%s': %v", loggedInUser, err)
		return
	}
	return
}

// StatusUpdatesLoop periodically generates status updates (alert emails TBD) from the event queue
func StatusUpdatesLoop() {
	// Ensure a warning message is displayed on the console if the status update loop exits
	defer func() {
		log.Printf("WARN: Status update loop exited")
	}()

	// Log the start of the loop
	log.Printf("Status update processing loop started.  %d second refresh.", Conf.Event.Delay)

	// Start the endless status update processing loop
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
		var tx pgx.Tx
		tx, err = pdb.Begin(context.Background())
		if err != nil {
			log.Printf("Couldn't begin database transaction for status update processing loop: %s", err.Error())
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
				dbQuery = `
					SELECT user_name, email, status_updates
					FROM users
					WHERE user_id = $1`
				userEvents := make(map[string][]StatusUpdateEntry)
				var userName string
				err = tx.QueryRow(context.Background(), dbQuery, u).Scan(&userName, &eml, &userEvents)
				if err != nil {
					if !errors.Is(err, pgx.ErrNoRows) {
						// A real error occurred
						log.Printf("Database query failed: %s", err)
						tx.Rollback(context.Background())
					}
					continue
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
				case EVENT_NEW_DISCUSSION:
					msg = fmt.Sprintf("A new discussion has been created for %s/%s.\n\nVisit https://%s%s "+
						"for the details", ev.details.Owner, ev.details.DBName, Conf.Web.ServerName,
						ev.details.URL)
					subj = fmt.Sprintf("DBHub.io: New discussion created on %s/%s", ev.details.Owner,
						ev.details.DBName)
				case EVENT_NEW_MERGE_REQUEST:
					msg = fmt.Sprintf("A new merge request has been created for %s/%s.\n\nVisit https://%s%s "+
						"for the details", ev.details.Owner, ev.details.DBName, Conf.Web.ServerName,
						ev.details.URL)
					subj = fmt.Sprintf("DBHub.io: New merge request created on %s/%s", ev.details.Owner,
						ev.details.DBName)
				case EVENT_NEW_COMMENT:
					msg = fmt.Sprintf("A new comment has been created for %s/%s.\n\nVisit https://%s%s for "+
						"the details", ev.details.Owner, ev.details.DBName, Conf.Web.ServerName,
						ev.details.URL)
					subj = fmt.Sprintf("DBHub.io: New comment on %s/%s", ev.details.Owner,
						ev.details.DBName)
				default:
					log.Printf("Unknown message type when creating email message")
				}
				if eml.Valid {
					// If the email address is of the form username@this_server (which indicates a non-functional email address), then skip it
					serverName := strings.Split(Conf.Web.ServerName, ":")
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

		// Wait before running the loop again
		time.Sleep(Conf.Event.Delay * time.Second)
	}
	return
}

// StoreBranches updates the branches list for a database
func StoreBranches(dbOwner, dbName string, branches map[string]BranchEntry) error {
	dbQuery := `
		UPDATE sqlite_databases
		SET branch_heads = $3, branches = $4
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
				)
			AND db_name = $2`
	commandTag, err := pdb.Exec(context.Background(), dbQuery, dbOwner, dbName, branches, len(branches))
	if err != nil {
		log.Printf("Updating branch heads for database '%s/%s' to '%v' failed: %v",
			SanitiseLogString(dbOwner), SanitiseLogString(dbName), branches, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf(
			"Wrong number of rows (%d) affected when updating branch heads for database '%s/%s' to '%v'",
			numRows, SanitiseLogString(dbOwner), SanitiseLogString(dbName), branches)
	}
	return nil
}

// StoreComment adds a comment to a discussion
func StoreComment(dbOwner, dbName, commenter string, discID int, comText string, discClose bool, mrState MergeRequestState) error {
	// Begin a transaction
	tx, err := pdb.Begin(context.Background())
	if err != nil {
		return err
	}
	// Set up an automatic transaction roll back if the function exits without committing
	defer tx.Rollback(context.Background())

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
					AND db_name = $2
			)
			AND disc.disc_id = $3
			AND disc.creator = u.user_id`
	err = tx.QueryRow(context.Background(), dbQuery, dbOwner, dbName, discID).Scan(&discState, &discCreator, &discType, &discTitle)
	if err != nil {
		log.Printf("Error retrieving current open state for '%s/%s', discussion '%d': %v",
			SanitiseLogString(dbOwner), SanitiseLogString(dbName), discID, err)
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
	var commandTag pgconn.CommandTag
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
					AND db_name = $2
			), int AS (
				SELECT internal_id AS int_id
				FROM discussions
				WHERE db_id = (SELECT db_id FROM d)
				AND disc_id = $4
			)
			INSERT INTO discussion_comments (db_id, disc_id, commenter, body, entry_type)
			SELECT (SELECT db_id FROM d), (SELECT int_id FROM int), (SELECT user_id FROM users WHERE lower(user_name) = lower($3)), $5, 'txt'
			RETURNING com_id`
		err = tx.QueryRow(context.Background(), dbQuery, dbOwner, dbName, commenter, discID, comText).Scan(&comID)
		if err != nil {
			log.Printf("Adding comment for database '%s/%s', discussion '%d' failed: %v",
				SanitiseLogString(dbOwner), SanitiseLogString(dbName), discID, err)
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
					AND db_name = $2
			), int AS (
				SELECT internal_id AS int_id
				FROM discussions
				WHERE db_id = (SELECT db_id FROM d)
				AND disc_id = $4
			)
			INSERT INTO discussion_comments (db_id, disc_id, commenter, body, entry_type)
			SELECT (SELECT db_id FROM d), (SELECT int_id FROM int), (SELECT user_id FROM users WHERE lower(user_name) = lower($3)), $5, $6`
		commandTag, err = tx.Exec(context.Background(), dbQuery, dbOwner, dbName, commenter, discID, eventTxt, eventType)
		if err != nil {
			log.Printf("Adding comment for database '%s/%s', discussion '%d' failed: %v",
				SanitiseLogString(dbOwner), SanitiseLogString(dbName), discID, err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf(
				"Wrong number of rows (%d) affected when adding a comment to database '%s/%s', discussion '%d'",
				numRows, SanitiseLogString(dbOwner), SanitiseLogString(dbName), discID)
		}
	}

	// Update the merge request state for MR's being closed
	if discClose == true && discType == MERGE_REQUEST {
		dbQuery = `
			UPDATE discussions
			SET mr_state = $4
			WHERE db_id = (
					SELECT db.db_id
					FROM sqlite_databases AS db
					WHERE db.user_id = (
							SELECT user_id
							FROM users
							WHERE lower(user_name) = lower($1)
						)
						AND db_name = $2
				)
				AND disc_id = $3`
		commandTag, err = tx.Exec(context.Background(), dbQuery, dbOwner, dbName, discID, mrState)
		if err != nil {
			log.Printf("Updating MR state for database '%s/%s', discussion '%d' failed: %v",
				SanitiseLogString(dbOwner), SanitiseLogString(dbName), discID, err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf(
				"Wrong number of rows (%d) affected when updating MR state for database '%s/%s', discussion '%d'",
				numRows, SanitiseLogString(dbOwner), SanitiseLogString(dbName), discID)
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
					AND db_name = $2
			)
			AND disc_id = $3`
	commandTag, err = tx.Exec(context.Background(), dbQuery, dbOwner, dbName, discID)
	if err != nil {
		log.Printf("Updating last modified date for database '%s/%s', discussion '%d' failed: %v",
			SanitiseLogString(dbOwner), SanitiseLogString(dbName), discID, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf(
			"Wrong number of rows (%d) affected when updating last_modified date for database '%s/%s', discussion '%d'",
			numRows, SanitiseLogString(dbOwner), SanitiseLogString(dbName), discID)
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
				AND db_name = $2
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
	commandTag, err = tx.Exec(context.Background(), dbQuery, dbOwner, dbName)
	if err != nil {
		log.Printf("Updating discussion count for database '%s/%s' failed: %v", SanitiseLogString(dbOwner),
			SanitiseLogString(dbName), err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf(
			"Wrong number of rows (%d) affected when updating discussion count for database '%s/%s'",
			numRows, SanitiseLogString(dbOwner), SanitiseLogString(dbName))
	}

	// If comment text was provided, generate an event about the new comment
	if comText != "" {
		var commentURL string
		if discType == MERGE_REQUEST {
			commentURL = fmt.Sprintf("/merge/%s/%s?id=%d#c%d", url.PathEscape(dbOwner),
				url.PathEscape(dbName), discID, comID)
		} else {
			commentURL = fmt.Sprintf("/discuss/%s/%s?id=%d#c%d", url.PathEscape(dbOwner),
				url.PathEscape(dbName), discID, comID)
		}
		details := EventDetails{
			DBName:   dbName,
			DiscID:   discID,
			Owner:    dbOwner,
			Type:     EVENT_NEW_COMMENT,
			Title:    discTitle,
			URL:      commentURL,
			UserName: commenter,
		}
		err = NewEvent(details)
		if err != nil {
			log.Printf("Error when creating a new event: %s", err.Error())
			return err
		}
	}

	// Commit the transaction
	err = tx.Commit(context.Background())
	if err != nil {
		return err
	}
	return nil
}

// StoreCommits updates the commit list for a database
func StoreCommits(dbOwner, dbName string, commitList map[string]CommitEntry) error {
	dbQuery := `
		UPDATE sqlite_databases
		SET commit_list = $3, last_modified = now()
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
				)
			AND db_name = $2`
	commandTag, err := pdb.Exec(context.Background(), dbQuery, dbOwner, dbName, commitList)
	if err != nil {
		log.Printf("Updating commit list for database '%s/%s' failed: %v", SanitiseLogString(dbOwner),
			SanitiseLogString(dbName), err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when updating commit list for database '%s/%s'", numRows,
			SanitiseLogString(dbOwner), SanitiseLogString(dbName))
	}
	return nil
}

// StoreDatabase stores database details in PostgreSQL, and the database data itself in Minio
func StoreDatabase(dbOwner, dbName string, branches map[string]BranchEntry, c CommitEntry, pub bool,
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
	cMap := map[string]CommitEntry{c.ID: c}
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
		commandTag, err = pdb.Exec(context.Background(), dbQuery, dbOwner, dbName, pub, nullable1LineDesc, nullableFullDesc,
			cMap, branches, sourceURL)
	} else {
		commandTag, err = pdb.Exec(context.Background(), dbQuery, dbOwner, dbName, pub, nullable1LineDesc, nullableFullDesc,
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
		err = StoreDefaultBranchName(dbOwner, dbName, branchName)
		if err != nil {
			log.Printf("Storing default branch '%s' name for '%s/%s' failed: %v", SanitiseLogString(branchName),
				SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
			return err
		}
	}
	return nil
}

// StoreDefaultBranchName stores the default branch name for a database
func StoreDefaultBranchName(dbOwner, dbName, branchName string) error {
	dbQuery := `
		UPDATE sqlite_databases
		SET default_branch = $3
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
				)
			AND db_name = $2`
	commandTag, err := pdb.Exec(context.Background(), dbQuery, dbOwner, dbName, branchName)
	if err != nil {
		log.Printf("Changing default branch for database '%v' to '%v' failed: %v", SanitiseLogString(dbName),
			SanitiseLogString(branchName), err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected during update: database: %v, new branch name: '%v'",
			numRows, SanitiseLogString(dbName), SanitiseLogString(branchName))
	}
	return nil
}

// StoreDefaultTableName stores the default table name for a database
func StoreDefaultTableName(dbOwner, dbName, tableName string) error {
	var t pgtype.Text
	if tableName != "" {
		t.String = tableName
		t.Valid = true
	}
	dbQuery := `
		UPDATE sqlite_databases
		SET default_table = $3
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
				)
			AND db_name = $2`
	commandTag, err := pdb.Exec(context.Background(), dbQuery, dbOwner, dbName, t)
	if err != nil {
		log.Printf("Changing default table for database '%v' to '%v' failed: %v", SanitiseLogString(dbName),
			tableName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected during update: database: %v, new table name: '%v'",
			numRows, SanitiseLogString(dbName), tableName)
	}
	return nil
}

// StoreDiscussion stores a new discussion for a database
func StoreDiscussion(dbOwner, dbName, loggedInUser, title, text string, discType DiscussionType,
	mr MergeRequestEntry) (newID int, err error) {

	// Begin a transaction
	tx, err := pdb.Begin(context.Background())
	if err != nil {
		return
	}
	// Set up an automatic transaction roll back if the function exits without committing
	defer tx.Rollback(context.Background())

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
				AND db.db_name = $2
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
			(SELECT user_id FROM users WHERE lower(user_name) = lower($3)),
			$4,
			$5,
			true,
			$6`
	if discType == MERGE_REQUEST {
		dbQuery += `,(
			SELECT db_id
			FROM sqlite_databases
			WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($7))
			AND db_name = $8
			AND is_deleted = false
		), $9, $10, $11`
	}
	dbQuery += `
		RETURNING (SELECT id FROM next_id)`
	if discType == MERGE_REQUEST {
		err = tx.QueryRow(context.Background(), dbQuery, dbOwner, dbName, loggedInUser, title, text, discType, mr.SourceOwner,
			mr.SourceDBName, mr.SourceBranch, mr.DestBranch, mr.Commits).Scan(&newID)
	} else {
		err = tx.QueryRow(context.Background(), dbQuery, dbOwner, dbName, loggedInUser, title, text, discType).Scan(&newID)
	}
	if err != nil {
		log.Printf("Adding new discussion or merge request '%s' for '%s/%s' failed: %v", SanitiseLogString(title),
			SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
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
			AND db_name = $2`
	commandTag, err := tx.Exec(context.Background(), dbQuery, dbOwner, dbName)
	if err != nil {
		log.Printf("Updating discussion counter for '%s/%s' failed: %v", SanitiseLogString(dbOwner),
			SanitiseLogString(dbName), err)
		return
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when updating discussion counter for '%s/%s'",
			numRows, SanitiseLogString(dbOwner), SanitiseLogString(dbName))
	}

	// Commit the transaction
	err = tx.Commit(context.Background())
	if err != nil {
		return
	}
	return
}

// StoreLicence stores a licence
func StoreLicence(userName, licenceName string, txt []byte, url string, orderNum int, fullName, fileFormat string) error {
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
	commandTag, err := pdb.Exec(context.Background(), dbQuery, userName, licenceName, hex.EncodeToString(sha[:]), txt, url, orderNum,
		fullName, fileFormat)
	if err != nil {
		log.Printf("Inserting licence '%v' in database failed: %v", SanitiseLogString(licenceName), err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when storing licence '%v'", numRows, SanitiseLogString(licenceName))
	}
	return nil
}

// StoreReleases stores the releases for a database
func StoreReleases(dbOwner, dbName string, releases map[string]ReleaseEntry) error {
	dbQuery := `
		UPDATE sqlite_databases
		SET release_list = $3, release_count = $4
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db_name = $2`
	commandTag, err := pdb.Exec(context.Background(), dbQuery, dbOwner, dbName, releases, len(releases))
	if err != nil {
		log.Printf("Storing releases for database '%s/%s' failed: %v", SanitiseLogString(dbOwner),
			SanitiseLogString(dbName), err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when storing releases for database: '%s/%s'", numRows,
			SanitiseLogString(dbOwner), SanitiseLogString(dbName))
	}
	return nil
}

// StoreShares stores the shares of a database
func StoreShares(dbOwner, dbName string, shares map[string]ShareDatabasePermissions) (err error) {
	// Begin a transaction
	tx, err := pdb.Begin(context.Background())
	if err != nil {
		return err
	}

	// Set up an automatic transaction roll back if the function exits without committing
	defer tx.Rollback(context.Background())

	// Delete all current shares for this database
	deleteQuery := `
		DELETE FROM database_shares
		WHERE db_id = (
			SELECT db_id
			FROM sqlite_databases
			WHERE user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND db_name = $2
				AND is_deleted = false
		)`
	_, err = tx.Exec(context.Background(), deleteQuery, dbOwner, dbName)
	if err != nil {
		return
	}

	// Insert new shares
	for name, access := range shares {
		insertQuery := `
			WITH o AS (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			), u AS (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($3)
			), d AS (
				SELECT db.db_id
				FROM sqlite_databases AS db, o
				WHERE db.user_id = o.user_id
				AND db_name = $2
			)
			INSERT INTO database_shares (db_id, user_id, access)
			SELECT d.db_id, u.user_id, $4 FROM d, u`
		_, err := tx.Exec(context.Background(), insertQuery, dbOwner, dbName, name, access)
		if err != nil {
			return err
		}
	}

	// Commit the transaction
	err = tx.Commit(context.Background())
	if err != nil {
		return
	}
	return
}

// StoreStatusUpdates stores the status updates list for a user
func StoreStatusUpdates(userName string, statusUpdates map[string][]StatusUpdateEntry) error {
	dbQuery := `
		UPDATE users
		SET status_updates = $2
		WHERE user_name = $1`
	commandTag, err := pdb.Exec(context.Background(), dbQuery, userName, statusUpdates)
	if err != nil {
		log.Printf("Adding status update for user '%s' failed: %v", userName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows affected (%d) when storing status update for user '%s'", numRows,
			userName)
		return err
	}
	return nil
}

// StoreTags stores the tags for a database
func StoreTags(dbOwner, dbName string, tags map[string]TagEntry) error {
	dbQuery := `
		UPDATE sqlite_databases
		SET tag_list = $3, tags = $4
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db_name = $2`
	commandTag, err := pdb.Exec(context.Background(), dbQuery, dbOwner, dbName, tags, len(tags))
	if err != nil {
		log.Printf("Storing tags for database '%s/%s' failed: %v", SanitiseLogString(dbOwner),
			SanitiseLogString(dbName), err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when storing tags for database: '%s/%s'", numRows,
			SanitiseLogString(dbOwner), SanitiseLogString(dbName))
	}
	return nil
}

// ToggleDBStar toggles the starring of a database by a user
func ToggleDBStar(loggedInUser, dbOwner, dbName string) error {
	// Check if the database is already starred
	starred, err := CheckDBStarred(loggedInUser, dbOwner, dbName)
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
			WITH u AS (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($2)
			)
			INSERT INTO database_stars (db_id, user_id)
			SELECT $1, u.user_id
			FROM u`
		commandTag, err := pdb.Exec(context.Background(), insertQuery, dbID, loggedInUser)
		if err != nil {
			log.Printf("Adding star to database failed. Database ID: '%v' Username: '%s' Error '%v'",
				dbID, loggedInUser, err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf("Wrong # of rows affected (%v) when starring database ID: '%v' Username: '%s'",
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
		commandTag, err := pdb.Exec(context.Background(), deleteQuery, dbID, loggedInUser)
		if err != nil {
			log.Printf("Removing star from database failed. Database ID: '%v' Username: '%s' Error: '%v'",
				dbID, loggedInUser, err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf("Wrong # of rows (%v) affected when unstarring database ID: '%v' Username: '%s'",
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
	commandTag, err := pdb.Exec(context.Background(), updateQuery, dbID)
	if err != nil {
		log.Printf("Updating star count in database failed: %v", err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong # of rows affected (%v) when updating star count. Database ID: '%v'", numRows, dbID)
	}
	return nil
}

// ToggleDBWatch toggles the watch status of a database by a user
func ToggleDBWatch(loggedInUser, dbOwner, dbName string) error {
	// Check if the database is already being watched
	watched, err := CheckDBWatched(loggedInUser, dbOwner, dbName)
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
				WHERE lower(user_name) = lower($3)
			), d AS (
				SELECT db_id
				FROM sqlite_databases
				WHERE user_id = (
						SELECT user_id
						FROM users
						WHERE lower(user_name) = lower($1)
					)
					AND db_name = $2
					AND is_deleted = false
			)
			INSERT INTO watchers (db_id, user_id)
			SELECT d.db_id, u.user_id
			FROM d, u`
		commandTag, err := pdb.Exec(context.Background(), insertQuery, dbOwner, dbName, loggedInUser)
		if err != nil {
			log.Printf("Adding '%s' to watchers list for database '%s/%s' failed: Error '%v'", loggedInUser,
				SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf("Wrong # of rows affected (%v) when adding '%s' to watchers list for database '%s/%s'",
				numRows, loggedInUser, SanitiseLogString(dbOwner), SanitiseLogString(dbName))
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
					AND db_name = $2
			)
			AND user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($3)
			)`
		commandTag, err := pdb.Exec(context.Background(), deleteQuery, dbOwner, dbName, loggedInUser)
		if err != nil {
			log.Printf("Removing '%s' from watchers list for database '%s/%s' failed: Error '%v'",
				loggedInUser, SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf("Wrong # of rows affected (%v) when removing '%s' from watchers list for database '%s/%s'",
				numRows, loggedInUser, SanitiseLogString(dbOwner), SanitiseLogString(dbName))
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
					AND db_name = $2
					AND is_deleted = false
		)
		UPDATE sqlite_databases
		SET watchers = (
			SELECT count(db_id)
			FROM watchers
			WHERE db_id = (SELECT db_id FROM d)
		) WHERE db_id = (SELECT db_id FROM d)`
	commandTag, err := pdb.Exec(context.Background(), updateQuery, dbOwner, dbName)
	if err != nil {
		log.Printf("Updating watchers count for '%s/%s' failed: %v", SanitiseLogString(dbOwner),
			SanitiseLogString(dbName), err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong # of rows affected (%v) when updating watchers count for '%s/%s'", numRows,
			SanitiseLogString(dbOwner), SanitiseLogString(dbName))
	}
	return nil
}

// UpdateAvatarURL updates the Avatar URL for a user
func UpdateAvatarURL(userName, avatarURL string) error {
	dbQuery := `
		UPDATE users
		SET avatar_url = $2
		WHERE lower(user_name) = lower($1)`
	commandTag, err := pdb.Exec(context.Background(), dbQuery, userName, avatarURL)
	if err != nil {
		log.Printf("Updating avatar URL failed for user '%s'. Error: '%v'", userName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong # of rows (%v) affected when updating avatar URL. User: '%s'", numRows,
			userName)
	}
	return nil
}

// UpdateContributorsCount updates the contributors count for a database
func UpdateContributorsCount(dbOwner, dbName string) error {
	// Get the commit list for the database
	commitList, err := GetCommitList(dbOwner, dbName)
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
		SET contributors = $3
			WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
				AND db_name = $2`
	commandTag, err := pdb.Exec(context.Background(), dbQuery, dbOwner, dbName, n)
	if err != nil {
		log.Printf("Updating contributor count in database '%s/%s' failed: %v", SanitiseLogString(dbOwner),
			SanitiseLogString(dbName), err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong # of rows affected (%v) when updating contributor count for database '%s/%s'",
			numRows, SanitiseLogString(dbOwner), SanitiseLogString(dbName))
	}
	return nil
}

// UpdateComment updates the text for a comment
func UpdateComment(dbOwner, dbName, loggedInUser string, discID, comID int, newText string) error {
	// Begin a transaction
	tx, err := pdb.Begin(context.Background())
	if err != nil {
		return err
	}
	// Set up an automatic transaction roll back if the function exits without committing
	defer tx.Rollback(context.Background())

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
				AND db_name = $2
		), int AS (
			SELECT internal_id AS int_id
			FROM discussions
			WHERE db_id = (SELECT db_id FROM d)
			AND disc_id = $3
		)
		SELECT u.user_name
		FROM discussion_comments AS com, users AS u
		WHERE com.db_id = (SELECT db_id FROM d)
			AND com.disc_id = (SELECT int_id FROM int)
			AND com.com_id = $4
			AND com.commenter = u.user_id`
	err = tx.QueryRow(context.Background(), dbQuery, dbOwner, dbName, discID, comID).Scan(&comCreator)
	if err != nil {
		log.Printf("Error retrieving name of comment creator for '%s/%s', discussion '%d', comment '%d': %v",
			SanitiseLogString(dbOwner), SanitiseLogString(dbName), discID, comID, err)
		return err
	}

	// Ensure only users with write access or the comment creator can update the comment
	allowed := strings.ToLower(loggedInUser) != strings.ToLower(comCreator)
	if !allowed {
		allowed, err = CheckDBPermissions(loggedInUser, dbOwner, dbName, true)
		if err != nil {
			return err
		}
	}
	if !allowed {
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
				AND db_name = $2
		), int AS (
			SELECT internal_id AS int_id
			FROM discussions
			WHERE db_id = (SELECT db_id FROM d)
			AND disc_id = $3
		)
		UPDATE discussion_comments AS com
		SET body = $5
		WHERE com.db_id = (SELECT db_id FROM d)
			AND com.disc_id = (SELECT int_id FROM int)
			AND com.com_id = $4`
	commandTag, err := tx.Exec(context.Background(), dbQuery, dbOwner, dbName, discID, comID, newText)
	if err != nil {
		log.Printf("Updating comment for database '%s/%s', discussion '%d', comment '%d' failed: %v",
			SanitiseLogString(dbOwner), SanitiseLogString(dbName), discID, comID, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf(
			"Wrong number of rows (%d) affected when updating comment for database '%s/%s', discussion '%d', comment '%d'",
			numRows, SanitiseLogString(dbOwner), SanitiseLogString(dbName), discID, comID)
	}

	// Commit the transaction
	err = tx.Commit(context.Background())
	if err != nil {
		return err
	}
	return nil
}

// UpdateDiscussion updates the text for a discussion
func UpdateDiscussion(dbOwner, dbName, loggedInUser string, discID int, newTitle, newText string) error {
	// Begin a transaction
	tx, err := pdb.Begin(context.Background())
	if err != nil {
		return err
	}
	// Set up an automatic transaction roll back if the function exits without committing
	defer tx.Rollback(context.Background())

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
					AND db_name = $2
			)
			AND disc.disc_id = $3
			AND disc.creator = u.user_id`
	err = tx.QueryRow(context.Background(), dbQuery, dbOwner, dbName, discID).Scan(&discCreator)
	if err != nil {
		log.Printf("Error retrieving name of discussion creator for '%s/%s', discussion '%d': %v",
			SanitiseLogString(dbOwner), SanitiseLogString(dbName), discID, err)
		return err
	}

	// Ensure only users with write access or the discussion starter can update the discussion
	allowed := strings.ToLower(loggedInUser) != strings.ToLower(discCreator)
	if !allowed {
		allowed, err = CheckDBPermissions(loggedInUser, dbOwner, dbName, true)
		if err != nil {
			return err
		}
	}
	if !allowed {
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
				AND db_name = $2
		)
		UPDATE discussions AS disc
		SET title = $4, description = $5, last_modified = now()
		WHERE disc.db_id = (SELECT db_id FROM d)
			AND disc.disc_id = $3`
	commandTag, err := tx.Exec(context.Background(), dbQuery, dbOwner, dbName, discID, newTitle, newText)
	if err != nil {
		log.Printf("Updating discussion for database '%s/%s', discussion '%d' failed: %v", SanitiseLogString(dbOwner),
			SanitiseLogString(dbName), discID, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf(
			"Wrong number of rows (%d) affected when updating discussion for database '%s/%s', discussion '%d'",
			numRows, SanitiseLogString(dbOwner), SanitiseLogString(dbName), discID)
	}

	// Commit the transaction
	err = tx.Commit(context.Background())
	if err != nil {
		return err
	}
	return nil
}

// UpdateMergeRequestCommits updates the commit list for a Merge Request
func UpdateMergeRequestCommits(dbOwner, dbName string, discID int, mrCommits []CommitEntry) (err error) {
	dbQuery := `
		WITH d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db
			WHERE db.user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				)
				AND db_name = $2
		)
		UPDATE discussions AS disc
		SET mr_commits = $4
		WHERE disc.db_id = (SELECT db_id FROM d)
			AND disc.disc_id = $3`
	commandTag, err := pdb.Exec(context.Background(), dbQuery, dbOwner, dbName, discID, mrCommits)
	if err != nil {
		log.Printf("Updating commit list for database '%s/%s', MR '%d' failed: %v", dbOwner,
			dbName, discID, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when updating commit list for database '%s/%s', MR '%d'",
			numRows, dbOwner, dbName, discID)
	}
	return nil
}

// User returns details for a user
func User(userName string) (user UserDetails, err error) {
	dbQuery := `
		SELECT user_name, coalesce(display_name, ''), coalesce(email, ''), coalesce(avatar_url, ''), password_hash,
		       date_joined, client_cert, coalesce(live_minio_bucket_name, '')
		FROM users
		WHERE lower(user_name) = lower($1)`
	err = pdb.QueryRow(context.Background(), dbQuery, userName).Scan(&user.Username, &user.DisplayName, &user.Email, &user.AvatarURL,
		&user.PHash, &user.DateJoined, &user.ClientCert, &user.MinioBucket)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The error was just "no such user found"
			return user, nil
		}

		// A real occurred
		log.Printf("Error retrieving details for user '%s' from database: %v", SanitiseLogString(userName), err)
		return user, nil
	}

	// Determine an appropriate URL for the users' profile pic
	if user.AvatarURL == "" {
		// No avatar URL is presently stored, so default to a gravatar based on users email (if known)
		if user.Email != "" {
			picHash := md5.Sum([]byte(user.Email))
			user.AvatarURL = fmt.Sprintf("https://www.gravatar.com/avatar/%x?d=identicon", picHash)
		}
	}
	return user, nil
}

// UserDBs returns the list of databases for a user
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
			SELECT DISTINCT ON (db.db_name) db.db_name, db.date_created, db.last_modified, db.public,
				db.watchers, db.stars, db.discussions, db.merge_requests, db.branches, db.release_count, db.tags,
				db.contributors, db.one_line_description, default_commits.id,
				db.commit_list->default_commits.id->'tree'->'entries'->0, db.source_url, db.default_branch,
				db.download_count, db.page_views
			FROM sqlite_databases AS db, default_commits
			WHERE db.db_id = default_commits.db_id
				AND db.is_deleted = false
				AND db.live_db = false`
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
	rows, err := pdb.Query(context.Background(), dbQuery, userName)
	if err != nil {
		log.Printf("Getting list of databases for user failed: %s", err)
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var defBranch, desc, source pgtype.Text
		var oneRow DBInfo
		err = rows.Scan(&oneRow.Database, &oneRow.DateCreated, &oneRow.RepoModified, &oneRow.Public,
			&oneRow.Watchers, &oneRow.Stars, &oneRow.Discussions, &oneRow.MRs, &oneRow.Branches,
			&oneRow.Releases, &oneRow.Tags, &oneRow.Contributors, &desc, &oneRow.CommitID, &oneRow.DBEntry, &source,
			&defBranch, &oneRow.Downloads, &oneRow.Views)
		if err != nil {
			log.Printf("Error retrieving database list for user: %v", err)
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
		// Retrieve the latest fork count
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
					AND db_name = $2)`
		err = pdb.QueryRow(context.Background(), dbQuery, userName, j.Database).Scan(&list[i].Forks)
		if err != nil {
			log.Printf("Error retrieving fork count for '%s/%s': %v", SanitiseLogString(userName),
				j.Database, err)
			return nil, err
		}
	}
	return list, nil
}

// UserNameFromAuth0ID returns the username for a given Auth0 ID
func UserNameFromAuth0ID(auth0id string) (string, error) {
	// Query the database for a username matching the given Auth0 ID
	dbQuery := `
		SELECT user_name
		FROM users
		WHERE auth0_id = $1`
	var userName string
	err := pdb.QueryRow(context.Background(), dbQuery, auth0id).Scan(&userName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No matching user for the given Auth0 ID
			return "", nil
		}

		// A real occurred
		log.Printf("Error looking up username in database: %v", err)
		return "", nil
	}

	return userName, nil
}

// UserStarredDBs returns the list of databases starred by a user
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
			SELECT db.user_id, db.db_name, stars.date_starred
			FROM sqlite_databases AS db, stars
			WHERE db.db_id = stars.db_id
			AND db.is_deleted = false
		)
		SELECT users.user_name, db_users.db_name, db_users.date_starred
		FROM users, db_users
		WHERE users.user_id = db_users.user_id
		ORDER BY date_starred DESC`
	rows, err := pdb.Query(context.Background(), dbQuery, userName)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var oneRow DBEntry
		err = rows.Scan(&oneRow.Owner, &oneRow.DBName, &oneRow.DateEntry)
		if err != nil {
			log.Printf("Error retrieving stars list for user: %v", err)
			return nil, err
		}
		list = append(list, oneRow)
	}

	return list, nil
}

// UsersStarredDB returns the list of users who starred a database
func UsersStarredDB(dbOwner, dbName string) (list []DBEntry, err error) {
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
					AND db_name = $2
					AND is_deleted = false
				)
		)
		SELECT users.user_name, users.display_name, star_users.date_starred
		FROM users, star_users
		WHERE users.user_id = star_users.user_id
		ORDER BY star_users.date_starred DESC`
	rows, err := pdb.Query(context.Background(), dbQuery, dbOwner, dbName)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var oneRow DBEntry
		var dn pgtype.Text
		err = rows.Scan(&oneRow.Owner, &dn, &oneRow.DateEntry)
		if err != nil {
			log.Printf("Error retrieving list of stars for %s/%s: %v", dbOwner, dbName, err)
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

// UsersWatchingDB returns the list of users watching a database
func UsersWatchingDB(dbOwner, dbName string) (list []DBEntry, err error) {
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
					AND db_name = $2
					AND is_deleted = false
				)
		)
		SELECT users.user_name, users.display_name, lst.date_watched
		FROM users, lst
		WHERE users.user_id = lst.user_id
		ORDER BY lst.date_watched DESC`
	rows, err := pdb.Query(context.Background(), dbQuery, dbOwner, dbName)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var oneRow DBEntry
		var dn pgtype.Text
		err = rows.Scan(&oneRow.Owner, &dn, &oneRow.DateEntry)
		if err != nil {
			log.Printf("Error retrieving list of watchers for %s/%s: %v", dbOwner, dbName, err)
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

// UserWatchingDBs returns the list of databases watched by a user
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
			SELECT db.user_id, db.db_name, watching.date_watched
			FROM sqlite_databases AS db, watching
			WHERE db.db_id = watching.db_id
			AND db.is_deleted = false
		)
		SELECT users.user_name, db_users.db_name, db_users.date_watched
		FROM users, db_users
		WHERE users.user_id = db_users.user_id
		ORDER BY date_watched DESC`
	rows, err := pdb.Query(context.Background(), dbQuery, userName)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var oneRow DBEntry
		err = rows.Scan(&oneRow.Owner, &oneRow.DBName, &oneRow.DateEntry)
		if err != nil {
			log.Printf("Error retrieving database watch list for user: %v", err)
			return nil, err
		}
		list = append(list, oneRow)
	}

	return list, nil
}

// ViewCount returns the view counter for a specific database
func ViewCount(dbOwner, dbName string) (viewCount int, err error) {
	dbQuery := `
		SELECT page_views
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db_name = $2`
	err = pdb.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&viewCount)
	if err != nil {
		log.Printf("Retrieving view count for '%s/%s' failed: %v", SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
		return 0, err
	}
	return
}

// VisualisationDeleteParams deletes a set of visualisation parameters
func VisualisationDeleteParams(dbOwner, dbName, visName string) (err error) {
	var commandTag pgconn.CommandTag
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
				AND db_name = $2
		)
		DELETE FROM vis_params WHERE user_id = (SELECT user_id FROM u) AND db_id = (SELECT db_id FROM d) AND name = $3`
	commandTag, err = pdb.Exec(context.Background(), dbQuery, dbOwner, dbName, visName)
	if err != nil {
		log.Printf("Deleting visualisation '%s' for database '%s/%s' failed: %v", SanitiseLogString(visName),
			SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected while deleting visualisation '%s' for database '%s/%s'",
			numRows, SanitiseLogString(visName), SanitiseLogString(dbOwner), SanitiseLogString(dbName))
	}
	return
}

// VisualisationRename renames an existing saved visualisation
func VisualisationRename(dbOwner, dbName, visName, visNewName string) (err error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
				AND db_name = $2
		)
		UPDATE vis_params SET name = $4 WHERE user_id = (SELECT user_id FROM u) AND db_id = (SELECT db_id FROM d) AND name = $3`
	commandTag, err := pdb.Exec(context.Background(), dbQuery, dbOwner, dbName, visName, visNewName)
	if err != nil {
		log.Printf("Renaming visualisation '%s' for database '%s/%s' failed: %v", SanitiseLogString(visName),
			SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected while renaming visualisation '%s' for database '%s/%s'",
			numRows, SanitiseLogString(visName), SanitiseLogString(dbOwner), SanitiseLogString(dbName))
	}
	return
}

// VisualisationSaveData saves visualisation result data for later retrieval
func VisualisationSaveData(dbOwner, dbName, commitID, hash string, visData []VisRowV1) (err error) {
	var commandTag pgconn.CommandTag
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
				AND db_name = $2
		)
		INSERT INTO vis_result_cache (user_id, db_id, commit_id, hash, results)
		SELECT (SELECT user_id FROM u), (SELECT db_id FROM d), $3, $4, $5
		ON CONFLICT (db_id, user_id, commit_id, hash)
			DO UPDATE
			SET results = $5`
	commandTag, err = pdb.Exec(context.Background(), dbQuery, dbOwner, dbName, commitID, hash, visData)
	if err != nil {
		log.Printf("Saving visualisation data for database '%s/%s', commit '%s', hash '%s' failed: %v", dbOwner,
			dbName, commitID, hash, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected while saving visualisation data for database '%s/%s', commit '%s', hash '%s'",
			numRows, dbOwner, dbName, commitID, hash)
	}
	return
}

// VisualisationSaveParams saves a set of visualisation parameters for later retrieval
func VisualisationSaveParams(dbOwner, dbName, visName string, visParams VisParamsV2) (err error) {
	var commandTag pgconn.CommandTag
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
				AND db_name = $2
		)
		INSERT INTO vis_params (user_id, db_id, name, parameters)
		SELECT (SELECT user_id FROM u), (SELECT db_id FROM d), $3, $4
		ON CONFLICT (db_id, user_id, name)
			DO UPDATE
			SET parameters = $4`
	commandTag, err = pdb.Exec(context.Background(), dbQuery, dbOwner, dbName, visName, visParams)
	if err != nil {
		log.Printf("Saving visualisation '%s' for database '%s/%s' failed: %v", SanitiseLogString(visName),
			SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected while saving visualisation '%s' for database '%s/%s'",
			numRows, SanitiseLogString(visName), SanitiseLogString(dbOwner), SanitiseLogString(dbName))
	}
	return
}

// RecordWebLogin records the start time of a user login session, for stats purposes
func RecordWebLogin(userName string) (err error) {
	// Add the new user to the database
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		INSERT INTO webui_logins (user_id)
		SELECT (SELECT user_id FROM u)`
	commandTag, err := pdb.Exec(context.Background(), dbQuery, userName)
	if err != nil {
		return
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		err = errors.New(fmt.Sprintf("Wrong number of rows (%d) affected while adding a webUI login record for '%s' to the database",
			numRows, SanitiseLogString(userName)))
	}
	return
}
