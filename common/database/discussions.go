package database

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	gfm "github.com/sqlitebrowser/github_flavored_markdown"
)

type DiscussionType int

const (
	DISCUSSION    DiscussionType = 0 // These are not iota, as it would be seriously bad for these numbers to change
	MERGE_REQUEST                = 1
)

type MergeRequestState int

const (
	OPEN                 MergeRequestState = 0 // These are not iota, as it would be seriously bad for these numbers to change
	CLOSED_WITH_MERGE                      = 1
	CLOSED_WITHOUT_MERGE                   = 2
)

type DiscussionEntry struct {
	AvatarURL    string            `json:"avatar_url"`
	Body         string            `json:"body"`
	BodyRendered string            `json:"body_rendered"`
	CommentCount int               `json:"comment_count"`
	Creator      string            `json:"creator"`
	DateCreated  time.Time         `json:"creation_date"`
	ID           int               `json:"disc_id"`
	LastModified time.Time         `json:"last_modified"`
	MRDetails    MergeRequestEntry `json:"mr_details"`
	Open         bool              `json:"open"`
	Title        string            `json:"title"`
	Type         DiscussionType    `json:"discussion_type"`
}

type MergeRequestEntry struct {
	Commits      []CommitEntry     `json:"commits"`
	DestBranch   string            `json:"destination_branch"`
	SourceBranch string            `json:"source_branch"`
	SourceDBID   int64             `json:"source_database_id"`
	SourceDBName string            `json:"source_database_name"`
	SourceOwner  string            `json:"source_owner"`
	State        MergeRequestState `json:"state"`
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
	rows, err = DB.Query(context.Background(), dbQuery, dbOwner, dbName, discType)
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
				dbOwner, dbName, err)
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
			err2 := DB.QueryRow(context.Background(), dbQuery, j.MRDetails.SourceDBID).Scan(&o, &n)
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

// StoreDiscussion stores a new discussion for a database
func StoreDiscussion(dbOwner, dbName, loggedInUser, title, text string, discType DiscussionType,
	mr MergeRequestEntry) (newID int, err error) {

	// Begin a transaction
	tx, err := DB.Begin(context.Background())
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
		log.Printf("Adding new discussion or merge request '%s' for '%s/%s' failed: %v", title,
			dbOwner, dbName, err)
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
		log.Printf("Updating discussion counter for '%s/%s' failed: %v", dbOwner,
			dbName, err)
		return
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when updating discussion counter for '%s/%s'",
			numRows, dbOwner, dbName)
	}

	// Commit the transaction
	err = tx.Commit(context.Background())
	if err != nil {
		return
	}
	return
}

// UpdateDiscussion updates the text for a discussion
func UpdateDiscussion(dbOwner, dbName, loggedInUser string, discID int, newTitle, newText string) error {
	// Begin a transaction
	tx, err := DB.Begin(context.Background())
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
			dbOwner, dbName, discID, err)
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
		log.Printf("Updating discussion for database '%s/%s', discussion '%d' failed: %v", dbOwner,
			dbName, discID, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf(
			"Wrong number of rows (%d) affected when updating discussion for database '%s/%s', discussion '%d'",
			numRows, dbOwner, dbName, discID)
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
	commandTag, err := DB.Exec(context.Background(), dbQuery, dbOwner, dbName, discID, mrCommits)
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
