package database

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	pgx "github.com/jackc/pgx/v5"
)

// APIKey is the model type for the api_keys table
type APIKey struct {
	ID          int64
	Uuid        string
	Key         string
	DateCreated time.Time
	ExpiryDate  *time.Time
	Comment     string
	Permissions ShareDatabasePermissions
}

// APIKeyDelete deletes an existing API key from the PostgreSQL database
func APIKeyDelete(loggedInUser, uuid string) (err error) {
	// Delete the API key
	dbQuery := "DELETE FROM api_keys WHERE uuid=$1 AND user_id = (SELECT user_id FROM users WHERE lower(user_name) = lower($2))"
	commandTag, err := DB.Exec(context.Background(), dbQuery, uuid, loggedInUser)
	if err != nil {
		log.Printf("Deleting API key from database failed: %v", err)
		return
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when deleting api key with uuid '%s'", numRows, uuid)
	}
	return
}

// APIKeyGenerate generates a random API key and saves it in the database
func APIKeyGenerate(loggedInUser string, expiryDate *time.Time, permissions ShareDatabasePermissions, comment string) (key APIKey, err error) {
	// Generate key
	length := 40
	data := make([]byte, length)
	_, err = rand.Read(data)
	if err != nil {
		return
	}
	key.Key = strings.Trim(base64.URLEncoding.EncodeToString(data), "=")

	// Set creation date
	key.DateCreated = time.Now()

	// Set expiry date
	key.ExpiryDate = expiryDate

	// Set permissions
	key.Permissions = permissions

	// Set comment
	key.Comment = comment

	// Save new key
	key.Uuid, err = APIKeySave(key.Key, loggedInUser, key.DateCreated, key.ExpiryDate, key.Permissions, key.Comment)
	return
}

// APIKeySave saves a new API key to the PostgreSQL database
func APIKeySave(key, loggedInUser string, dateCreated time.Time, expiryDate *time.Time, permissions ShareDatabasePermissions, comment string) (uuid string, err error) {
	// Hash the key
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(key)))

	// Make sure the API key isn't already in the database
	dbQuery := `
		SELECT count(key)
		FROM api_keys
		WHERE key = $1`
	var keyCount int
	err = DB.QueryRow(context.Background(), dbQuery, hash).Scan(&keyCount)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("Checking if an API key exists failed: %s", err)
		return
	}
	if keyCount != 0 {
		// API key is already in our system
		log.Printf("Duplicate API key generated for user '%s'", loggedInUser)
		return "", fmt.Errorf("API generator created duplicate key.  Try again, just in case...")
	}

	// Add the new API key to the database
	dbQuery = `
		INSERT INTO api_keys (user_id, key, date_created, expiry_date, permissions, comment)
		SELECT (SELECT user_id FROM users WHERE lower(user_name) = lower($1)), $2, $3, $4, $5, $6
		RETURNING concat(uuid, '')`
	err = DB.QueryRow(context.Background(), dbQuery, loggedInUser, hash, dateCreated, expiryDate, permissions, comment).Scan(&uuid)
	if err != nil {
		log.Printf("Adding API key to database failed: %v", err)
		return
	}
	return
}

// GetAPIKeys returns the list of API keys for a user
func GetAPIKeys(user string) ([]APIKey, error) {
	dbQuery := `
		SELECT key_id, uuid, date_created, expiry_date, permissions, coalesce(comment, '')
		FROM api_keys
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			)`
	rows, err := DB.Query(context.Background(), dbQuery, user)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return nil, err
	}
	defer rows.Close()
	var keys []APIKey
	for rows.Next() {
		var key APIKey
		err = rows.Scan(&key.ID, &key.Uuid, &key.DateCreated, &key.ExpiryDate, &key.Permissions, &key.Comment)
		if err != nil {
			log.Printf("Error retrieving API key list: %v", err)
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// GetAPIKeyBySecret returns the details of the API key with the given secret
func GetAPIKeyBySecret(secret string) (user string, key APIKey, err error) {
	// Hash API key
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(secret)))

	dbQuery := `
		SELECT user_name, key_id, uuid, date_created, expiry_date, permissions, coalesce(comment, '')
		FROM api_keys AS api, users
		WHERE api.key = $1
			AND api.user_id = users.user_id
			AND (api.expiry_date is null OR api.expiry_date > now())`
	err = DB.QueryRow(context.Background(), dbQuery, hash).Scan(&user, &key.ID, &key.Uuid, &key.DateCreated, &key.ExpiryDate, &key.Permissions, &key.Comment)
	if err != nil {
		return
	}
	return
}
