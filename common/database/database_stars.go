package database

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgtype"
)

// CheckDBStarred check if a database has been starred by a given user.  The boolean return value is only valid when
// err is nil
func CheckDBStarred(loggedInUser, dbOwner, dbName string) (bool, error) {
	dbQuery := `
		SELECT count(db_id)
		FROM database_stars
		WHERE database_stars.user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($3)
			)
			AND database_stars.db_id = (
					SELECT db_id
					FROM sqlite_databases
					WHERE user_id = (
							SELECT user_id
							FROM users
							WHERE lower(user_name) = lower($1)
						)
						AND db_name = $2
						AND is_deleted = false)`
	var starCount int
	err := DB.QueryRow(context.Background(), dbQuery, dbOwner, dbName, loggedInUser).Scan(&starCount)
	if err != nil {
		log.Printf("Error looking up star count for database. User: '%s' DB: '%s/%s'. Error: %v",
			loggedInUser, dbOwner, dbName, err)
		return true, err
	}
	if starCount == 0 {
		// Database hasn't been starred by the user
		return false, nil
	}

	// Database HAS been starred by the user
	return true, nil
}

// ToggleDBStar toggles the starring of a database by a user
func ToggleDBStar(loggedInUser, dbOwner, dbName string) error {
	// Check if the database is already starred
	starred, err := CheckDBStarred(loggedInUser, dbOwner, dbName)
	if err != nil {
		return err
	}

	// Add or remove the star
	if !starred {
		// Star the database
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
			INSERT INTO database_stars (db_id, user_id)
			SELECT d.db_id, u.user_id
			FROM d, u`
		commandTag, err := DB.Exec(context.Background(), insertQuery, dbOwner, dbName, loggedInUser)
		if err != nil {
			log.Printf("Adding star by '%s' to database '%s/s' failed: Error '%v'", loggedInUser,
				dbOwner, dbName, err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf("Wrong # of rows affected (%v) when starring '%s' database '%s/%s'",
				numRows, loggedInUser, dbOwner, dbName)
		}
	} else {
		// Unstar the database
		deleteQuery := `
			DELETE FROM database_stars
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
			log.Printf("Removing star by user '%s' from database '%s/%s' failed: Error '%v'",
				loggedInUser, dbOwner, dbName, err)
			return err
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf("Wrong # of rows affected (%v) when removing star by '%s' from database '%s/%s'",
				numRows, loggedInUser, dbOwner, dbName)
		}
	}

	// Refresh the main database table with the updated star count
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
		SET stars = (
			SELECT count(db_id)
			FROM database_stars
			WHERE db_id = (SELECT db_id FROM d)
		) WHERE db_id = (SELECT db_id FROM d)`
	commandTag, err := DB.Exec(context.Background(), updateQuery, dbOwner, dbName)
	if err != nil {
		log.Printf("Updating star count in database failed: %v", err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong # of rows affected (%v) when updating star count. Database: '%s/%s'", numRows, dbOwner, dbName)
	}
	return nil
}

// UsersStarredDB returns the list of users who starred a database
func UsersStarredDB(dbOwner, dbName string) (list []DBEntry, err error) {
	dbQuery := `
		WITH star_users AS (
			SELECT user_id, date_starred
			FROM database_stars
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
		SELECT users.user_name, users.display_name, star_users.date_starred
		FROM users, star_users
		WHERE users.user_id = star_users.user_id
		ORDER BY star_users.date_starred DESC`
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
			log.Printf("Error retrieving list of stars for %s/%s: %v", dbOwner, dbName, err)
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
