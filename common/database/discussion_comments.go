package database

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	gfm "github.com/sqlitebrowser/github_flavored_markdown"
)

type DiscussionCommentType string

const (
	TEXT   DiscussionCommentType = "txt"
	CLOSE                        = "cls"
	REOPEN                       = "rop"
)

type DiscussionCommentEntry struct {
	AvatarURL    string                `json:"avatar_url"`
	Body         string                `json:"body"`
	BodyRendered string                `json:"body_rendered"`
	Commenter    string                `json:"commenter"`
	DateCreated  time.Time             `json:"creation_date"`
	EntryType    DiscussionCommentType `json:"entry_type"`
	ID           int                   `json:"com_id"`
}

// DeleteComment deletes a specific comment from a discussion
func DeleteComment(dbOwner, dbName string, discID, comID int) error {
	// Begin a transaction
	tx, err := DB.Begin(context.Background())
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
			dbOwner, dbName, discID, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when deleting comment '%d' from database '%s/%s, discussion '%d''",
			numRows, comID, dbOwner, dbName, discID)
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
			discID, dbOwner, dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when updating comment count for discussion '%v' in "+
			"'%s/%s'", numRows, discID, dbOwner, dbName)
	}

	// Commit the transaction
	err = tx.Commit(context.Background())
	if err != nil {
		return err
	}

	return nil
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
	rows, err = DB.Query(context.Background(), dbQuery, dbOwner, dbName, discID)
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
				dbOwner, dbName, discID, err)
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

// StoreComment adds a comment to a discussion
func StoreComment(dbOwner, dbName, commenter string, discID int, comText string, discClose bool, mrState MergeRequestState) error {
	// Begin a transaction
	tx, err := DB.Begin(context.Background())
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
			dbOwner, dbName, discID, err)
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
				dbOwner, dbName, discID, err)
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
				dbOwner, dbName, discID, err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf(
				"Wrong number of rows (%d) affected when adding a comment to database '%s/%s', discussion '%d'",
				numRows, dbOwner, dbName, discID)
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
				dbOwner, dbName, discID, err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf(
				"Wrong number of rows (%d) affected when updating MR state for database '%s/%s', discussion '%d'",
				numRows, dbOwner, dbName, discID)
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
			dbOwner, dbName, discID, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf(
			"Wrong number of rows (%d) affected when updating last_modified date for database '%s/%s', discussion '%d'",
			numRows, dbOwner, dbName, discID)
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
		log.Printf("Updating discussion count for database '%s/%s' failed: %v", dbOwner,
			dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf(
			"Wrong number of rows (%d) affected when updating discussion count for database '%s/%s'",
			numRows, dbOwner, dbName)
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

// UpdateComment updates the text for a comment
func UpdateComment(dbOwner, dbName, loggedInUser string, discID, comID int, newText string) error {
	// Begin a transaction
	tx, err := DB.Begin(context.Background())
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
			dbOwner, dbName, discID, comID, err)
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
			dbOwner, dbName, discID, comID, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf(
			"Wrong number of rows (%d) affected when updating comment for database '%s/%s', discussion '%d', comment '%d'",
			numRows, dbOwner, dbName, discID, comID)
	}

	// Commit the transaction
	err = tx.Commit(context.Background())
	if err != nil {
		return err
	}
	return nil
}
