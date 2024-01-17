package database

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

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
	commandTag, err := DB.Exec(context.Background(), dbQuery, dbOwner, dbName, uploader, ipAddr, serverSw, userAgent,
		uploadDate, sha)
	if err != nil {
		log.Printf("Storing record of upload '%s/%s', sha '%s' by '%v' failed: %v", dbOwner,
			dbName, sha, uploader, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected while storing upload record for '%s/%s'", numRows,
			dbOwner, dbName)
	}
	return nil
}
