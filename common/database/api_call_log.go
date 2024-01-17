package database

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgconn"
)

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
		commandTag, err = DB.Exec(context.Background(), dbQuery, loggedInUser, dbOwner, dbName, operation, callerSw)
	} else {
		dbQuery = `
		WITH loggedIn AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		INSERT INTO api_call_log (caller_id, api_operation, api_caller_sw)
		VALUES ((SELECT user_id FROM loggedIn), $2, $3)`
		commandTag, err = DB.Exec(context.Background(), dbQuery, loggedInUser, operation, callerSw)
	}
	if err != nil {
		log.Printf("Adding api call log entry failed: %s", err)
		return
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when adding api call entry for user '%s'", numRows, loggedInUser)
	}
}
