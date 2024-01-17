package database

import (
	"context"
	"encoding/base64"
	"log"

	"github.com/jackc/pgx/v5/pgtype"
)

// LogSQLiteQueryAfter adds memory allocation stats for the execution run of a user supplied SQLite query
func LogSQLiteQueryAfter(insertID, memUsed, memHighWater int64) (err error) {
	dbQuery := `
		UPDATE vis_query_runs
		SET memory_used = $2, memory_high_water = $3
		WHERE query_run_id = $1`
	commandTag, err := DB.Exec(context.Background(), dbQuery, insertID, memUsed, memHighWater)
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
	err := DB.QueryRow(context.Background(), dbQuery, dbOwner, dbName, queryUser, ipAddr, userAgent, encodedQuery, source).Scan(&insertID)
	if err != nil {
		log.Printf("Storing record of user SQLite query '%v' on '%s/%s' failed: %v", encodedQuery,
			dbOwner, dbName, err)
		return 0, err
	}
	return insertID, nil
}
