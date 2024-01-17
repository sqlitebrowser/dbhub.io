package database

import (
	"context"
	"fmt"
)

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
	commandTag, err := DB.Exec(context.Background(), dbQuery, userName)
	if err != nil {
		return
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		err = fmt.Errorf("Wrong number of rows (%d) affected while adding a webUI login record for '%s' to the database",
			numRows, userName)
	}
	return
}
