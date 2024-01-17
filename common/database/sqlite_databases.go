package database

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/sqlitebrowser/dbhub.io/common/config"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// AccessType is whether a database is private, or public, or both
type AccessType int

const (
	DB_BOTH AccessType = iota
	DB_PRIVATE
	DB_PUBLIC
)

type DBTreeEntryType string

const (
	TREE     DBTreeEntryType = "tree"
	DATABASE                 = "db"
	LICENCE                  = "licence"
)

type ForkType int

const (
	SPACE ForkType = iota
	ROOT
	STEM
	BRANCH
	END
)

// SetAccessType is used for setting the public flag of a database
type SetAccessType int

const (
	SetToPublic SetAccessType = iota
	SetToPrivate
	KeepCurrentAccessType
)

type ActivityRow struct {
	Count  int    `json:"count"`
	DBName string `json:"dbname"`
	Owner  string `json:"owner"`
}

type ActivityStats struct {
	Downloads []ActivityRow
	Forked    []ActivityRow
	Starred   []ActivityRow
	Uploads   []UploadRow
	Viewed    []ActivityRow
}

type BranchEntry struct {
	Commit      string `json:"commit"`
	CommitCount int    `json:"commit_count"`
	Description string `json:"description"`
}

type CommitEntry struct {
	AuthorEmail    string    `json:"author_email"`
	AuthorName     string    `json:"author_name"`
	CommitterEmail string    `json:"committer_email"`
	CommitterName  string    `json:"committer_name"`
	ID             string    `json:"id"`
	Message        string    `json:"message"`
	OtherParents   []string  `json:"other_parents"`
	Parent         string    `json:"parent"`
	Timestamp      time.Time `json:"timestamp"`
	Tree           DBTree    `json:"tree"`
}

type DBEntry struct {
	DateEntry        time.Time
	DBName           string
	Owner            string
	OwnerDisplayName string `json:"display_name"`
}

type DBInfo struct {
	Branch        string
	Branches      int
	BranchList    []string
	Commits       int
	CommitID      string
	Contributors  int
	Database      string
	DateCreated   time.Time
	DBEntry       DBTreeEntry
	DefaultBranch string
	DefaultTable  string
	Discussions   int
	Downloads     int
	ForkDatabase  string
	ForkDeleted   bool
	ForkOwner     string
	Forks         int
	FullDesc      string
	IsLive        bool
	LastModified  time.Time
	Licence       string
	LicenceURL    string
	LiveNode      string
	MRs           int
	MyStar        bool
	MyWatch       bool
	OneLineDesc   string
	Owner         string
	Public        bool
	RepoModified  time.Time
	Releases      int
	SHA256        string
	Size          int64
	SourceURL     string
	Stars         int
	Tables        []string
	Tags          int
	Views         int
	Watchers      int
}

type DBTree struct {
	ID      string        `json:"id"`
	Entries []DBTreeEntry `json:"entries"`
}

type DBTreeEntry struct {
	EntryType    DBTreeEntryType `json:"entry_type"`
	LastModified time.Time       `json:"last_modified"`
	LicenceSHA   string          `json:"licence"`
	Name         string          `json:"name"`
	Sha256       string          `json:"sha256"`
	Size         int64           `json:"size"`
}

type ForkEntry struct {
	DBName     string     `json:"database_name"`
	ForkedFrom int        `json:"forked_from"`
	IconList   []ForkType `json:"icon_list"`
	ID         int        `json:"id"`
	Owner      string     `json:"database_owner"`
	Processed  bool       `json:"processed"`
	Public     bool       `json:"public"`
	Deleted    bool       `json:"deleted"`
}

type ReleaseEntry struct {
	Commit        string    `json:"commit"`
	Date          time.Time `json:"date"`
	Description   string    `json:"description"`
	ReleaserEmail string    `json:"email"`
	ReleaserName  string    `json:"name"`
	Size          int64     `json:"size"`
}

type SQLiteDBinfo struct {
	Info     DBInfo
	MaxRows  int
	MinioBkt string
	MinioId  string
}

type TagEntry struct {
	Commit      string    `json:"commit"`
	Date        time.Time `json:"date"`
	Description string    `json:"description"`
	TaggerEmail string    `json:"email"`
	TaggerName  string    `json:"name"`
}

type UploadRow struct {
	DBName     string    `json:"dbname"`
	Owner      string    `json:"owner"`
	UploadDate time.Time `json:"upload_date"`
}

// AnalysisUsersWithDBs returns the list of users with at least one database
func AnalysisUsersWithDBs() (userList map[string]int, err error) {
	dbQuery := `
		SELECT u.user_name, count(*)
		FROM users u, sqlite_databases db
		WHERE u.user_id = db.user_id
		GROUP BY u.user_name`
	rows, err := DB.Query(context.Background(), dbQuery)
	if err != nil {
		log.Printf("Database query failed in AnalysisUsersWithDBs: %v", err)
		return
	}
	defer rows.Close()
	userList = make(map[string]int)
	for rows.Next() {
		var user string
		var numDBs int
		err = rows.Scan(&user, &numDBs)
		if err != nil {
			log.Printf("Error in AnalysisUsersWithDBs when getting the list of users with at least one database: %v", err)
			return nil, err
		}
		userList[user] = numDBs
	}
	return
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
	err := DB.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&dbCount)
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
	err = DB.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&isLive, &liveNode)
	if err != nil {
		return false, "", err
	}
	return
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
	err = DB.QueryRow(context.Background(), dbQuery, dbOwner, dbID).Scan(&dbName)
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

// DBDetails returns the details for a specific database
func DBDetails(dbInfo *SQLiteDBinfo, loggedInUser, dbOwner, dbName, commitID string) (err error) {
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
		err = DB.QueryRow(context.Background(), dbQuery, dbOwner, dbName, commitID).Scan(&dbInfo.Info.DateCreated, &dbInfo.Info.RepoModified,
			&dbInfo.Info.Watchers, &dbInfo.Info.Stars, &dbInfo.Info.Discussions, &dbInfo.Info.MRs, &dbInfo.Info.CommitID, &dbInfo.Info.DBEntry,
			&dbInfo.Info.Branches, &dbInfo.Info.Releases, &dbInfo.Info.Contributors, &dbInfo.Info.OneLineDesc, &dbInfo.Info.FullDesc,
			&dbInfo.Info.DefaultTable, &dbInfo.Info.Public, &dbInfo.Info.SourceURL, &dbInfo.Info.Tags, &dbInfo.Info.DefaultBranch,
			&dbInfo.Info.IsLive, &dbInfo.Info.LiveNode, &dbInfo.MinioId)
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
		err = DB.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&dbInfo.Info.DateCreated,
			&dbInfo.Info.RepoModified, &dbInfo.Info.Watchers, &dbInfo.Info.Stars, &dbInfo.Info.Discussions, &dbInfo.Info.OneLineDesc,
			&dbInfo.Info.FullDesc, &dbInfo.Info.DefaultTable, &dbInfo.Info.Public, &dbInfo.Info.SourceURL, &dbInfo.Info.DefaultBranch,
			&dbInfo.Info.LiveNode, &dbInfo.MinioId)
		if err != nil {
			log.Printf("Error when retrieving database details: %v", err.Error())
			return errors.New("The requested database doesn't exist")
		}
		dbInfo.Info.IsLive = true
	}

	// If an sha256 was in the licence field, retrieve its friendly name and url for displaying
	licSHA := dbInfo.Info.DBEntry.LicenceSHA
	if licSHA != "" {
		dbInfo.Info.Licence, dbInfo.Info.LicenceURL, err = GetLicenceInfoFromSha256(dbOwner, licSHA)
		if err != nil {
			return err
		}
	} else {
		dbInfo.Info.Licence = "Not specified"
	}

	// Retrieve correctly capitalised username for the database owner
	usrOwner, err := User(dbOwner)
	if err != nil {
		return err
	}

	// Fill out the fields we already have data for
	dbInfo.Info.Database = dbName
	dbInfo.Info.Owner = usrOwner.Username

	// The social stats are always updated because they could change without the cache being updated
	dbInfo.Info.Watchers, dbInfo.Info.Stars, dbInfo.Info.Forks, err = SocialStats(dbOwner, dbName)
	if err != nil {
		return err
	}

	// Retrieve the latest discussion and MR counts
	dbInfo.Info.Discussions, dbInfo.Info.MRs, err = GetDiscussionAndMRCount(dbOwner, dbName)
	if err != nil {
		return err
	}

	// Retrieve the "forked from" information
	dbInfo.Info.ForkOwner, dbInfo.Info.ForkDatabase, dbInfo.Info.ForkDeleted, err = ForkedFrom(dbOwner, dbName)
	if err != nil {
		return err
	}

	// Check if the database was starred by the logged in user
	dbInfo.Info.MyStar, err = CheckDBStarred(loggedInUser, dbOwner, dbName)
	if err != nil {
		return err
	}

	// Check if the database is being watched by the logged in user
	dbInfo.Info.MyWatch, err = CheckDBWatched(loggedInUser, dbOwner, dbName)
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
	err = DB.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&starCount)
	if err != nil {
		log.Printf("Error looking up star count for database '%s/%s'. Error: %v", dbOwner,
			dbName, err)
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
	err = DB.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&watcherCount)
	if err != nil {
		log.Printf("Error looking up watcher count for database '%s/%s'. Error: %v",
			dbOwner, dbName, err)
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
	err = DB.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&c)
	if err != nil {
		log.Printf("Error when retrieving head commit ID of default branch: %v", err.Error())
		return "", errors.New("Internal error when looking up database details")
	}
	if c.Valid {
		commitID = c.String
	}
	return commitID, nil
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
	tx, err := DB.Begin(context.Background())
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
		log.Printf("Removing all watchers for database '%s/%s' failed: Error '%s'", dbOwner,
			dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong # of rows affected (%v) when removing all watchers for database '%s/%s'", numRows,
			dbOwner, dbName)
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
		log.Printf("Retrieving fork list failed for database '%s/%s': %s", dbOwner,
			dbName, err)
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
			log.Printf("Updating fork count for '%s/%s' in PostgreSQL failed: %s", dbOwner,
				dbName, err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 && !isLive { // Skip this check when deleting live databases
			log.Printf("Wrong number of rows (%d) affected (spot 1) when updating fork count for database '%s/%s'",
				numRows, dbOwner, dbName)
		}

		// Generate a random string to be used in the deleted database's name field, so if the user adds a database with
		// the deleted one's name then the unique constraint on the database won't reject it
		newName := "deleted-database-" + randomString(20)

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
			log.Printf("%s: deleting (forked) database entry failed for database '%s/%s': %v",
				config.Conf.Live.Nodename, dbOwner, dbName, err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf(
				"%s: wrong number of rows (%d) affected when deleting (forked) database '%s/%s'",
				config.Conf.Live.Nodename, numRows, dbOwner, dbName)
		}

		// Commit the transaction
		err = tx.Commit(context.Background())
		if err != nil {
			return err
		}

		// Log the database deletion
		log.Printf("%s: database '%s/%s' deleted", config.Conf.Live.Nodename, dbOwner, dbName)
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
			dbOwner, dbName, err)
		return err
	}

	// Generate a random string to be used in the deleted database's name field, so if the user adds a database with
	// the deleted one's name then the unique constraint on the database won't reject it
	newName := "deleted-database-" + randomString(20)

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
			dbOwner, dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf(
			"Wrong number of rows (%d) affected when deleting (forked) database '%s/%s'", numRows,
			dbOwner, dbName)
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
		log.Printf("Updating fork count for '%s/%s' in PostgreSQL failed: %v", dbOwner,
			dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected (spot 2) when updating fork count for database '%s/%s'",
			numRows, dbOwner, dbName)
	}

	// Commit the transaction
	err = tx.Commit(context.Background())
	if err != nil {
		return err
	}

	// Log the database deletion
	log.Printf("%s: (forked) database '%s/%s' deleted", config.Conf.Live.Nodename, dbOwner,
		dbName)
	return nil
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
	commandTag, err := DB.Exec(context.Background(), dbQuery, dstOwner, srcOwner, dbName)
	if err != nil {
		log.Printf("Forking database '%s/%s' in PostgreSQL failed: %v", srcOwner,
			dbName, err)
		return 0, err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows affected (%d) when forking main database entry: "+
			"'%s/%s' to '%s/%s'", numRows, srcOwner, dbName,
			dstOwner, dbName)
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
	err = DB.QueryRow(context.Background(), dbQuery, dstOwner, dbName).Scan(&newForkCount)
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
	err = DB.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&dbID, &forkedFrom)
	if err != nil {
		log.Printf("Error checking if database was forked from another '%s/%s'. Error: %v",
			dbOwner, dbName, err)
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
	err = DB.QueryRow(context.Background(), dbQuery, forkedFrom).Scan(&forkOwn, &forkDB, &forkDel)
	if err != nil {
		log.Printf("Error retrieving forked database information for '%s/%s'. Error: %v",
			dbOwner, dbName, err)
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
	rows, err := DB.Query(context.Background(), dbQuery, dbOwner, dbName)
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
	rows, err := DB.Query(context.Background(), dbQuery, dbOwner, dbName)
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
	starRows, err := DB.Query(context.Background(), dbQuery)
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
	forkRows, err := DB.Query(context.Background(), dbQuery)
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
	upRows, err := DB.Query(context.Background(), dbQuery)
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
	dlRows, err := DB.Query(context.Background(), dbQuery)
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
	viewRows, err := DB.Query(context.Background(), dbQuery)
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
	err = DB.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&branches)
	if err != nil {
		log.Printf("Error when retrieving branch heads for database '%s/%s': %v", dbOwner,
			dbName, err)
		return nil, err
	}
	return branches, nil
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
	err := DB.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&l)
	if err != nil {
		log.Printf("Retrieving commit list for '%s/%s' failed: %v", dbOwner,
			dbName, err)
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
	err = DB.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&b)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("Error when retrieving default branch name for database '%s/%s': %v",
				dbOwner, dbName, err)
		} else {
			log.Printf("No default branch name exists for database '%s/%s'. This shouldn't happen",
				dbOwner, dbName)
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
	err = DB.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&t)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("Error when retrieving default table name for database '%s/%s': %v",
				dbOwner, dbName, err)
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
	err = DB.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&discCount, &mrCount)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("Error when retrieving discussion and MR count for database '%s/%s': %v",
				dbOwner, dbName, err)
		} else {
			log.Printf("Database '%s/%s' not found when attempting to retrieve discussion and MR count. This"+
				"shouldn't happen", dbOwner, dbName)
		}
		return
	}
	return
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
	err = DB.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&releases)
	if err != nil {
		log.Printf("Error when retrieving releases for database '%s/%s': %v", dbOwner,
			dbName, err)
		return nil, err
	}
	if releases == nil {
		// If there aren't any releases yet, return an empty set instead of nil
		releases = make(map[string]ReleaseEntry)
	}
	return releases, nil
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
	err = DB.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&tags)
	if err != nil {
		log.Printf("Error when retrieving tags for database '%s/%s': %v", dbOwner,
			dbName, err)
		return nil, err
	}
	if tags == nil {
		// If there aren't any tags yet, return an empty set instead of nil
		tags = make(map[string]TagEntry)
	}
	return tags, nil
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
	commandTag, err := DB.Exec(context.Background(), dbQuery, dbOwner, dbName)
	if err != nil {
		log.Printf("Increment download count for '%s/%s' failed: %v", dbOwner,
			dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		errMsg := fmt.Sprintf("Wrong number of rows affected (%v) when incrementing download count for '%s/%s'",
			numRows, dbOwner, dbName)
		log.Printf(errMsg)
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
		err = errors.New("Error: Unknown public/private setting requested for a new live   Aborting.")
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
	commandTag, err = DB.Exec(context.Background(), dbQuery, dbOwner, dbName, public, liveNode, bucketName)
	if err != nil {
		log.Printf("Storing LIVE database '%s/%s' failed: %s", dbOwner, dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected while storing LIVE database '%s/%s'", numRows,
			dbOwner, dbName)
	}
	return nil
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
	commandTag, err := DB.Exec(context.Background(), dbQuery, userName, dbName, newName)
	if err != nil {
		log.Printf("Renaming database '%s/%s' failed: %v", userName,
			dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		errMsg := fmt.Sprintf("Wrong number of rows affected (%d) when renaming '%s/%s' to '%s/%s'",
			numRows, userName, dbName, userName, newName)
		log.Printf(errMsg)
		return errors.New(errMsg)
	}

	// Log the rename
	log.Printf("Database renamed from '%s/%s' to '%s/%s'", userName, dbName,
		userName, newName)
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
	err = DB.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&st, &fo, &wa)
	if err != nil {
		log.Printf("Error retrieving social stats count for '%s/%s': %v", dbOwner,
			dbName, err)
		return -1, -1, -1, err
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
	commandTag, err := DB.Exec(context.Background(), dbQuery, dbOwner, dbName, branches, len(branches))
	if err != nil {
		log.Printf("Updating branch heads for database '%s/%s' to '%v' failed: %v",
			dbOwner, dbName, branches, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf(
			"Wrong number of rows (%d) affected when updating branch heads for database '%s/%s' to '%v'",
			numRows, dbOwner, dbName, branches)
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
	commandTag, err := DB.Exec(context.Background(), dbQuery, dbOwner, dbName, commitList)
	if err != nil {
		log.Printf("Updating commit list for database '%s/%s' failed: %v", dbOwner,
			dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when updating commit list for database '%s/%s'", numRows,
			dbOwner, dbName)
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
	commandTag, err := DB.Exec(context.Background(), dbQuery, dbOwner, dbName, branchName)
	if err != nil {
		log.Printf("Changing default branch for database '%v' to '%v' failed: %v", dbName,
			branchName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected during update: database: %v, new branch name: '%v'",
			numRows, dbName, branchName)
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
	commandTag, err := DB.Exec(context.Background(), dbQuery, dbOwner, dbName, t)
	if err != nil {
		log.Printf("Changing default table for database '%v' to '%v' failed: %v", dbName,
			tableName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected during update: database: %v, new table name: '%v'",
			numRows, dbName, tableName)
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
	commandTag, err := DB.Exec(context.Background(), dbQuery, dbOwner, dbName, releases, len(releases))
	if err != nil {
		log.Printf("Storing releases for database '%s/%s' failed: %v", dbOwner,
			dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when storing releases for database: '%s/%s'", numRows,
			dbOwner, dbName)
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
	commandTag, err := DB.Exec(context.Background(), dbQuery, dbOwner, dbName, tags, len(tags))
	if err != nil {
		log.Printf("Storing tags for database '%s/%s' failed: %v", dbOwner,
			dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when storing tags for database: '%s/%s'", numRows,
			dbOwner, dbName)
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
	commandTag, err := DB.Exec(context.Background(), dbQuery, dbOwner, dbName, n)
	if err != nil {
		log.Printf("Updating contributor count in database '%s/%s' failed: %v", dbOwner,
			dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong # of rows affected (%v) when updating contributor count for database '%s/%s'",
			numRows, dbOwner, dbName)
	}
	return nil
}

// UpdateModified is a simple function to change the 'last modified' timestamp for a database to now()
func UpdateModified(dbOwner, dbName string) (err error) {
	dbQuery := `
		UPDATE sqlite_databases AS db
		SET last_modified = now()
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db_name = $2`
	commandTag, err := DB.Exec(context.Background(), dbQuery, dbOwner, dbName)
	if err != nil {
		log.Printf("%s: updating last_modified for database '%s/%s' failed: %v", config.Conf.Live.Nodename, dbOwner,
			dbName, err)
		return
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("%s: wrong number of rows (%d) affected when updating last_modified for database '%s/%s'",
			config.Conf.Live.Nodename, numRows, dbOwner, dbName)
	}
	return
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
	rows, err := DB.Query(context.Background(), dbQuery, userName)
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
		err = DB.QueryRow(context.Background(), dbQuery, userName, j.Database).Scan(&list[i].Forks)
		if err != nil {
			log.Printf("Error retrieving fork count for '%s/%s': %v", userName,
				j.Database, err)
			return nil, err
		}
	}
	return list, nil
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
	rows, err := DB.Query(context.Background(), dbQuery, userName)
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
	rows, err := DB.Query(context.Background(), dbQuery, userName)
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
	err = DB.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&viewCount)
	if err != nil {
		log.Printf("Retrieving view count for '%s/%s' failed: %v", dbOwner, dbName, err)
		return 0, err
	}
	return
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
	err = DB.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&dbID)
	if err != nil {
		log.Printf("Error looking up database id. Owner: '%s', Database: '%s'. Error: %v",
			dbOwner, dbName, err)
	}
	return
}

// nextChild looks for the next child fork in a fork tree
func nextChild(loggedInUser string, rawListPtr *[]ForkEntry, outputListPtr *[]ForkEntry, forkTrailPtr *[]int, iconDepth int) ([]ForkEntry, []int, bool) {
	// TODO: This approach feels half arsed.  Maybe redo it as a recursive function instead?

	// Resolve the pointers
	rawList := *rawListPtr
	outputList := *outputListPtr
	forkTrail := *forkTrailPtr

	// Grab the last database ID from the fork trail
	parentID := forkTrail[len(forkTrail)-1:][0]

	// Scan unprocessed rows for the first child of parentID
	numResults := len(rawList)
	for j := 1; j < numResults; j++ {
		// Skip already processed entries
		if rawList[j].Processed == false {
			if rawList[j].ForkedFrom == parentID {
				// * Found a fork of the parent *

				// Set the icon list for display in the browser
				for k := 0; k < iconDepth; k++ {
					rawList[j].IconList = append(rawList[j].IconList, SPACE)
				}
				rawList[j].IconList = append(rawList[j].IconList, END)

				// If the database is no longer public, then use placeholder details instead
				if !rawList[j].Public && (strings.ToLower(rawList[j].Owner) != strings.ToLower(loggedInUser)) {
					rawList[j].DBName = "private database"
				}

				// If the database is deleted, use a placeholder indicating that instead
				if rawList[j].Deleted {
					rawList[j].DBName = "deleted database"
				}

				// Add this database to the output list
				outputList = append(outputList, rawList[j])

				// Append this database ID to the fork trail
				forkTrail = append(forkTrail, rawList[j].ID)

				// Mark this database entry as processed
				rawList[j].Processed = true

				// Indicate a child fork was found
				return outputList, forkTrail, true
			}
		}
	}

	// Indicate no child fork was found
	return outputList, forkTrail, false
}

// randomString generates a random alphanumeric string of the desired length
func randomString(length int) string {
	rand.Seed(time.Now().UnixNano())
	const alphaNum = "abcdefghijklmnopqrstuvwxyz0123456789"
	randomString := make([]byte, length)
	for i := range randomString {
		randomString[i] = alphaNum[rand.Intn(len(alphaNum))]
	}
	return string(randomString)
}
