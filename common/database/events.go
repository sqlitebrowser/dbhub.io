package database

import (
	"context"
)

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
	_, err = DB.Exec(context.Background(), dbQuery, details.Owner, details.DBName, details.Type, details)
	if err != nil {
		return err
	}
	return
}
