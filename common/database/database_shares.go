package database

import (
	"context"
	"errors"
	"strings"

	pgx "github.com/jackc/pgx/v5"
)

type ShareDatabasePermissions string

const (
	MayRead         ShareDatabasePermissions = "r"
	MayReadAndWrite ShareDatabasePermissions = "rw"
)

// ShareDatabasePermissionsUser contains a list of shared database permissions for a given user
type ShareDatabasePermissionsUser struct {
	OwnerName  string                   `json:"owner_name"`
	DBName     string                   `json:"database_name"`
	IsLive     bool                     `json:"is_live"`
	Permission ShareDatabasePermissions `json:"permission"`
}

// CheckDBPermissions checks if a database exists and can be accessed by the given user.
// If an error occurred, the true/false value should be ignored, as only the error value is valid
func CheckDBPermissions(loggedInUser, dbOwner, dbName string, writeAccess bool) (bool, error) {
	// Query id and public flag of the database
	dbQuery := `
		SELECT db_id, public
		FROM sqlite_databases
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db_name = $2
			AND is_deleted = false
		LIMIT 1`
	var dbId int
	var dbPublic bool
	err := DB.QueryRow(context.Background(), dbQuery, dbOwner, dbName).Scan(&dbId, &dbPublic)

	// There are two possible error cases: no rows returned or another error.
	// If no rows were returned the database simply does not exist and no error is returned to the caller.
	// If there was another, actual error this error is returned to the caller.
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	// If we get here this means that the database does exist. The next step is to check the permissions.

	if strings.ToLower(loggedInUser) == strings.ToLower(dbOwner) {
		// If the request is from the owner of the database, always allow access to the database
		return true, nil
	} else if writeAccess == false && dbPublic {
		// Read access to public databases is always permitted
		return true, nil
	} else if loggedInUser == "" {
		// If the user is not logged in and we reach this point, access is not permitted
		return false, nil
	}

	// If the request is from someone who is logged in but not the owner of the database, check
	// if the database is shared with the logged in user.

	// Query shares
	dbQuery = `
		SELECT access
		FROM database_shares
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)
			AND db_id = $2
		LIMIT 1`
	var dbAccess ShareDatabasePermissions
	err = DB.QueryRow(context.Background(), dbQuery, loggedInUser, dbId).Scan(&dbAccess)

	// Check if there are any shares. If not, don't allow access.
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	// If there are shares, check the permissions
	if writeAccess {
		// If write access is required, only return true if writing is allowed
		return dbAccess == MayReadAndWrite, nil
	}

	// If no write access is required, always return true if there is a share for this database and user
	return true, nil
}

// GetShares returns a map with all users for which the given database is shared as key and their
// permissions as value.
func GetShares(dbOwner, dbName string) (shares map[string]ShareDatabasePermissions, err error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
			AND db_name = $2
		)
		SELECT usr.user_name, share.access
		FROM database_shares AS share, d, users AS usr
		WHERE share.db_id = d.db_id AND usr.user_id = share.user_id
		ORDER BY usr.user_name`
	rows, e := DB.Query(context.Background(), dbQuery, dbOwner, dbName)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return nil, e
	}
	defer rows.Close()

	shares = make(map[string]ShareDatabasePermissions)

	var name string
	var access ShareDatabasePermissions
	for rows.Next() {
		err = rows.Scan(&name, &access)
		if err != nil {
			return
		}
		shares[name] = access
	}
	return
}

// GetSharesForUser returns a list of all the databases shared with the given user, and their permissions.
func GetSharesForUser(userName string) (shares []ShareDatabasePermissionsUser, err error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		SELECT users.user_name, db.db_name, db.live_db, shares.access
		FROM database_shares AS shares, sqlite_databases AS db, u, users
		WHERE shares.user_id = u.user_id
			AND shares.db_id = db.db_id
			AND db.user_id = users.user_id
			AND db.is_deleted = false
		ORDER by users.user_name, db.db_name`
	rows, e := DB.Query(context.Background(), dbQuery, userName)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return nil, e
	}
	defer rows.Close()
	var x ShareDatabasePermissionsUser
	for rows.Next() {
		err = rows.Scan(&x.OwnerName, &x.DBName, &x.IsLive, &x.Permission)
		if err != nil {
			return
		}
		shares = append(shares, x)
	}
	return
}

// StoreShares stores the shares of a database
func StoreShares(dbOwner, dbName string, shares map[string]ShareDatabasePermissions) (err error) {
	// Begin a transaction
	tx, err := DB.Begin(context.Background())
	if err != nil {
		return err
	}

	// Set up an automatic transaction roll back if the function exits without committing
	defer tx.Rollback(context.Background())

	// Delete all current shares for this database
	deleteQuery := `
		DELETE FROM database_shares
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
		)`
	_, err = tx.Exec(context.Background(), deleteQuery, dbOwner, dbName)
	if err != nil {
		return
	}

	// Insert new shares
	for name, access := range shares {
		insertQuery := `
			WITH o AS (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			), u AS (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($3)
			), d AS (
				SELECT db.db_id
				FROM sqlite_databases AS db, o
				WHERE db.user_id = o.user_id
				AND db_name = $2
			)
			INSERT INTO database_shares (db_id, user_id, access)
			SELECT d.db_id, u.user_id, $4 FROM d, u`
		_, err := tx.Exec(context.Background(), insertQuery, dbOwner, dbName, name, access)
		if err != nil {
			return err
		}
	}

	// Commit the transaction
	err = tx.Commit(context.Background())
	if err != nil {
		return
	}
	return
}
