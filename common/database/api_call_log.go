package database

import (
	"context"
	"errors"
	"log"
	"time"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type ApiUsage struct {
	Date         string `json:"date"`
	NumCalls     int64  `json:"num_calls"`
	Runtime      int64  `json:"runtime"`
	RequestSize  int64  `json:"request_size"`
	ResponseSize int64  `json:"response_size"`
}

// ApiCallLog records an API call operation.  Database name is optional, as not all API calls operate on a
// database.  If a database name is provided however, then the database owner name *must* also be provided
func ApiCallLog(key APIKey, loggedInUser, dbOwner, dbName, operation, callerSw, method string, statusCode int, runtime time.Duration, requestSize int64, responseSize int) {
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
		INSERT INTO api_call_log (caller_id, db_owner_id, db_id, api_operation, api_caller_sw, key_id, method, status_code, runtime, request_size, response_size)
		VALUES ((SELECT user_id FROM loggedIn), (SELECT user_id FROM owner), (SELECT db_id FROM d), $4, $5, $6, $7, $8, $9, $10, $11)`
		commandTag, err = DB.Exec(context.Background(), dbQuery, loggedInUser, dbOwner, dbName, operation, callerSw, key.ID, method, statusCode, runtime, requestSize, responseSize)
	} else {
		dbQuery = `
		WITH loggedIn AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		INSERT INTO api_call_log (caller_id, api_operation, api_caller_sw, key_id, method, status_code, runtime, request_size, response_size)
		VALUES ((SELECT user_id FROM loggedIn), $2, $3, $4, $5, $6, $7, $8, $9)`
		commandTag, err = DB.Exec(context.Background(), dbQuery, loggedInUser, operation, callerSw, key.ID, method, statusCode, runtime, requestSize, responseSize)
	}
	if err != nil {
		log.Printf("Adding api call log entry failed: %s", err)
		return
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when adding api call entry for user '%s'", numRows, loggedInUser)
	}
}

func ApiUsageData(user string, from, to time.Time) (usage []ApiUsage, err error) {
	query := `
		WITH userData AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		SELECT to_char(api_call_date, 'YYYY-MM-DD') AS dt,
			count(*) AS num_calls,
			coalesce((sum(runtime) / 1000)::bigint, 0) AS runtime,
			coalesce(sum(request_size), 0) AS request_size,
			coalesce(sum(response_size), 0) AS response_size
		FROM api_call_log
		WHERE caller_id=(SELECT user_id FROM userData) AND api_call_date>=$2 AND api_call_date<=$3
		GROUP BY dt ORDER BY dt`
	rows, err := DB.Query(context.Background(), query, user, from, to)
	if err != nil {
		log.Printf("Querying API usage data failed: %v", err)
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var day ApiUsage
		err = rows.Scan(&day.Date, &day.NumCalls, &day.Runtime, &day.RequestSize, &day.ResponseSize)
		if err != nil {
			log.Printf("Error retrieving API usage data: %v", err)
			return nil, err
		}
		usage = append(usage, day)
	}

	return
}

// ApiUsageStatsLastPeriod returns the number of API calls and the timestamp of the last API call for a given user and
// period. The period is between now and `period` time ago.
func ApiUsageStatsLastPeriod(user string, period time.Duration) (count int, lastCall time.Time, err error) {
	query := `
		WITH userData AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		SELECT count(*) AS num_calls, max(api_call_date) AS last_call
		FROM api_call_log
		WHERE caller_id=(SELECT user_id FROM userData) AND api_call_date>=$2
		GROUP BY caller_id`
	err = DB.QueryRow(context.Background(), query, user, time.Now().Add(-period)).Scan(&count, &lastCall)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("Querying API usage stats failed for user '%s': %v", user, err)
		return
	}

	return
}
