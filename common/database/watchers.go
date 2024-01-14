package database

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgtype"
)

// CheckDBWatched checks if a database is being watched by a given user.  The boolean return value is only valid when
// err is nil
func CheckDBWatched(loggedInUser, dbOwner, dbName string) (bool, error) {
	dbQuery := `
		SELECT count(db_id)
		FROM watchers
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($3)
			)
			AND db_id = (
					SELECT db_id
					FROM sqlite_databases
					WHERE user_id = (
							SELECT user_id
							FROM users
							WHERE lower(user_name) = lower($1)
						)
						AND db_name = $2
						AND is_deleted = false)`
	var watchCount int
	err := DB.QueryRow(context.Background(), dbQuery, dbOwner, dbName, loggedInUser).Scan(&watchCount)
	if err != nil {
		log.Printf("Error looking up watchers count for database. User: '%s' DB: '%s/%s'. Error: %v",
			loggedInUser, dbOwner, dbName, err)
		return true, err
	}
	if watchCount == 0 {
		// Database isn't being watched by the user
		return false, nil
	}

	// Database IS being watched by the user
	return true, nil
}

// ToggleDBWatch toggles the watch status of a database by a user
func ToggleDBWatch(loggedInUser, dbOwner, dbName string) error {
	// Check if the database is already being watched
	watched, err := CheckDBWatched(loggedInUser, dbOwner, dbName)
	if err != nil {
		return err
	}

	// Add or remove the user from the watchers list
	if !watched {
		// Watch the database
		insertQuery := `
			WITH u AS (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($3)
			), d AS (
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
			INSERT INTO watchers (db_id, user_id)
			SELECT d.db_id, u.user_id
			FROM d, u`
		commandTag, err := DB.Exec(context.Background(), insertQuery, dbOwner, dbName, loggedInUser)
		if err != nil {
			log.Printf("Adding '%s' to watchers list for database '%s/%s' failed: Error '%v'", loggedInUser,
				dbOwner, dbName, err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf("Wrong # of rows affected (%v) when adding '%s' to watchers list for database '%s/%s'",
				numRows, loggedInUser, dbOwner, dbName)
		}
	} else {
		// Unwatch the database
		deleteQuery := `
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
			)
			AND user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($3)
			)`
		commandTag, err := DB.Exec(context.Background(), deleteQuery, dbOwner, dbName, loggedInUser)
		if err != nil {
			log.Printf("Removing '%s' from watchers list for database '%s/%s' failed: Error '%v'",
				loggedInUser, dbOwner, dbName, err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf("Wrong # of rows affected (%v) when removing '%s' from watchers list for database '%s/%s'",
				numRows, loggedInUser, dbOwner, dbName)
		}
	}

	// Refresh the main database table with the updated watchers count
	updateQuery := `
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
		UPDATE sqlite_databases
		SET watchers = (
			SELECT count(db_id)
			FROM watchers
			WHERE db_id = (SELECT db_id FROM d)
		) WHERE db_id = (SELECT db_id FROM d)`
	commandTag, err := DB.Exec(context.Background(), updateQuery, dbOwner, dbName)
	if err != nil {
		log.Printf("Updating watchers count for '%s/%s' failed: %v", dbOwner,
			dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong # of rows affected (%v) when updating watchers count for '%s/%s'", numRows,
			dbOwner, dbName)
	}
	return nil
}

// UsersWatchingDB returns the list of users watching a database
func UsersWatchingDB(dbOwner, dbName string) (list []DBEntry, err error) {
	dbQuery := `
		WITH lst AS (
			SELECT user_id, date_watched
			FROM watchers
			WHERE db_id = (
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
		)
		SELECT users.user_name, users.display_name, lst.date_watched
		FROM users, lst
		WHERE users.user_id = lst.user_id
		ORDER BY lst.date_watched DESC`
	rows, err := DB.Query(context.Background(), dbQuery, dbOwner, dbName)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var oneRow DBEntry
		var dn pgtype.Text
		err = rows.Scan(&oneRow.Owner, &dn, &oneRow.DateEntry)
		if err != nil {
			log.Printf("Error retrieving list of watchers for %s/%s: %v", dbOwner, dbName, err)
			return nil, err
		}

		// If the user hasn't filled out their display name, use their username instead
		if dn.Valid {
			oneRow.OwnerDisplayName = dn.String
		} else {
			oneRow.OwnerDisplayName = oneRow.Owner
		}
		list = append(list, oneRow)
	}
	return list, nil
}
