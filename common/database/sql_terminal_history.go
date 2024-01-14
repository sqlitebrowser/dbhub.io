package database

import (
	"context"
	"log"
)

type SqlHistoryItemStates string

const (
	Executed SqlHistoryItemStates = "executed"
	Queried  SqlHistoryItemStates = "queried"
	Error    SqlHistoryItemStates = "error"
)

type SqlHistoryItem struct {
	Statement string               `json:"input"`
	Result    interface{}          `json:"output"`
	State     SqlHistoryItemStates `json:"state"`
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
	commandTag, err := DB.Exec(context.Background(), dbQuery, dbOwner, dbName, loggedInUser, stmt, state, result)
	if err != nil {
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected while saving SQL statement for user '%s'", numRows,
			loggedInUser)
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
	_, err = DB.Exec(context.Background(), dbQuery, dbOwner, dbName, loggedInUser, keepRecords)
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
	rows, err := DB.Query(context.Background(), dbQuery, dbOwner, dbName, loggedInUser)
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
