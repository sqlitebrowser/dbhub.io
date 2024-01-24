package database

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/sqlitebrowser/dbhub.io/common/config"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type EventDetails struct {
	DBName    string    `json:"database_name"`
	DiscID    int       `json:"discussion_id"`
	ID        string    `json:"event_id"`
	Message   string    `json:"message"`
	Owner     string    `json:"database_owner"`
	Timestamp time.Time `json:"event_timestamp"`
	Title     string    `json:"title"`
	Type      EventType `json:"event_type"`
	URL       string    `json:"event_url"`
	UserName  string    `json:"username"`
}

type EventType int

const (
	EVENT_NEW_DISCUSSION    EventType = 0 // These are not iota, as it would be seriously bad for these numbers to change
	EVENT_NEW_MERGE_REQUEST           = 1
	EVENT_NEW_COMMENT                 = 2
	EVENT_NEW_RELEASE                 = 3
)

type StatusUpdateEntry struct {
	DiscID int    `json:"discussion_id"`
	Title  string `json:"title"`
	URL    string `json:"event_url"`
}

type UserDetails struct {
	AvatarURL   string
	DateJoined  time.Time
	DisplayName string
	Email       string
	MinioBucket string
	Password    string
	PVerify     string
	Username    string
}

// DefaultNumDisplayRows is the number of rows to display by default on the database page
const DefaultNumDisplayRows = 25

// AddDefaultUser adds the default user to the system, so the referential integrity of licence user_id 0 works
func AddDefaultUser() error {
	// Make sure the default user doesn't exist already
	existsAlready, err := CheckUserExists("default")
	if err != nil {
		return err
	}
	if existsAlready {
		return nil
	}

	// Add the new user to the database
	dbQuery := `
		INSERT INTO users (auth0_id, user_name, email, display_name)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_name)
			DO NOTHING`
	_, err = DB.Exec(context.Background(), dbQuery, "", "default", "default@dbhub.io",
		"Default system user")
	if err != nil {
		log.Printf("Error when adding the default user to the database: %v", err)
		// For now, don't bother logging a failure here.  This *might* need changing later on
		return err
	}

	// Log addition of the default user
	log.Printf("%v: default user added", config.Conf.Live.Nodename)
	return nil
}

// AddUser adds a user to the system
func AddUser(auth0ID, userName, email, displayName, avatarURL string) (err error) {
	// If the display name or avatar URL are an empty string, we insert a NULL instead
	var av, dn pgtype.Text
	if displayName != "" {
		dn.String = displayName
		dn.Valid = true
	}
	if avatarURL != "" {
		av.String = avatarURL
		av.Valid = true
	}

	// Add the new user to the database
	insertQuery := `
		INSERT INTO users (auth0_id, user_name, email, display_name, avatar_url)
		VALUES ($1, $2, $3, $4, $5)`
	commandTag, err := DB.Exec(context.Background(), insertQuery, auth0ID, userName, email, dn, av)
	if err != nil {
		log.Printf("Adding user to database failed: %v", err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows affected when creating user: %v, username: %v", numRows, userName)
	}

	// Log the user registration
	log.Printf("User registered: '%s' Email: '%s'", userName, email)
	return nil
}

// CheckEmailExists checks if an email address already exists in our system. Returns true if the email is already in
// the system, false if not.  If an error occurred, the true/false value should be ignored, as only the error value
// is valid
func CheckEmailExists(email string) (bool, error) {
	// Check if the email address is already in our system
	dbQuery := `
		SELECT count(user_name)
		FROM users
		WHERE email = $1`
	var emailCount int
	err := DB.QueryRow(context.Background(), dbQuery, email).Scan(&emailCount)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return true, err
	}
	if emailCount == 0 {
		// Email address isn't yet in our system
		return false, nil
	}

	// Email address IS already in our system
	return true, nil
}

// CheckUserExists checks if a username already exists in our system.  Returns true if the username is already taken,
// false if not.  If an error occurred, the true/false value should be ignored, and only the error return code used
func CheckUserExists(userName string) (bool, error) {
	dbQuery := `
		SELECT count(user_id)
		FROM users
		WHERE lower(user_name) = lower($1)`
	var userCount int
	err := DB.QueryRow(context.Background(), dbQuery, userName).Scan(&userCount)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return true, err
	}
	if userCount == 0 {
		// Username isn't in system
		return false, nil
	}
	// Username IS in system
	return true, nil
}

// GetUsernameFromEmail returns the username associated with an email address
func GetUsernameFromEmail(email string) (userName, avatarURL string, err error) {
	dbQuery := `
		SELECT user_name, avatar_url
		FROM users
		WHERE email = $1`
	var av pgtype.Text
	err = DB.QueryRow(context.Background(), dbQuery, email).Scan(&userName, &av)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No matching username of the email
			err = nil
			return
		}
		log.Printf("Looking up username for email address '%s' failed: %v", email, err)
		return
	}

	// If no avatar URL is presently stored, default to a gravatar based on the users email (if known)
	if !av.Valid {
		picHash := md5.Sum([]byte(email))
		avatarURL = fmt.Sprintf("https://www.gravatar.com/avatar/%x?d=identicon", picHash)
	} else {
		avatarURL = av.String
	}
	return
}

// PrefUserMaxRows returns the user's preference for maximum number of SQLite rows to display.
func PrefUserMaxRows(loggedInUser string) int {
	// Retrieve the user preference data
	dbQuery := `
		SELECT pref_max_rows
		FROM users
		WHERE user_id = (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1))`
	var maxRows int
	err := DB.QueryRow(context.Background(), dbQuery, loggedInUser).Scan(&maxRows)
	if err != nil {
		log.Printf("Error retrieving user '%s' preference data: %v", loggedInUser, err)
		return DefaultNumDisplayRows // Use the default value
	}
	return maxRows
}

// SetUserPreferences sets the user's preference for maximum number of SQLite rows to display
func SetUserPreferences(userName string, maxRows int, displayName, email string) error {
	dbQuery := `
		UPDATE users
		SET pref_max_rows = $2, display_name = $3, email = $4
		WHERE lower(user_name) = lower($1)`
	commandTag, err := DB.Exec(context.Background(), dbQuery, userName, maxRows, displayName, email)
	if err != nil {
		log.Printf("Updating user preferences failed for user '%s'. Error: '%v'", userName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong # of rows (%v) affected when updating user preferences. User: '%s'", numRows,
			userName)
	}
	return nil
}

// StatusUpdates returns the list of outstanding status updates for a user
func StatusUpdates(loggedInUser string) (statusUpdates map[string][]StatusUpdateEntry, err error) {
	dbQuery := `
		SELECT status_updates
		FROM users
		WHERE user_name = $1`
	err = DB.QueryRow(context.Background(), dbQuery, loggedInUser).Scan(&statusUpdates)
	if err != nil {
		log.Printf("Error retrieving status updates list for user '%s': %v", loggedInUser, err)
		return
	}
	return
}

// StoreStatusUpdates stores the status updates list for a user
func StoreStatusUpdates(userName string, statusUpdates map[string][]StatusUpdateEntry) error {
	dbQuery := `
		UPDATE users
		SET status_updates = $2
		WHERE user_name = $1`
	commandTag, err := DB.Exec(context.Background(), dbQuery, userName, statusUpdates)
	if err != nil {
		log.Printf("Adding status update for user '%s' failed: %v", userName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows affected (%d) when storing status update for user '%s'", numRows,
			userName)
		return err
	}
	return nil
}

// UpdateAvatarURL updates the Avatar URL for a user
func UpdateAvatarURL(userName, avatarURL string) error {
	dbQuery := `
		UPDATE users
		SET avatar_url = $2
		WHERE lower(user_name) = lower($1)`
	commandTag, err := DB.Exec(context.Background(), dbQuery, userName, avatarURL)
	if err != nil {
		log.Printf("Updating avatar URL failed for user '%s'. Error: '%v'", userName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong # of rows (%v) affected when updating avatar URL. User: '%s'", numRows,
			userName)
	}
	return nil
}

// User returns details for a user
func User(userName string) (user UserDetails, err error) {
	dbQuery := `
		SELECT user_name, coalesce(display_name, ''), coalesce(email, ''), coalesce(avatar_url, ''),
		       date_joined, coalesce(live_minio_bucket_name, '')
		FROM users
		WHERE lower(user_name) = lower($1)`
	err = DB.QueryRow(context.Background(), dbQuery, userName).Scan(&user.Username, &user.DisplayName, &user.Email, &user.AvatarURL,
		&user.DateJoined, &user.MinioBucket)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The error was just "no such user found"
			return user, nil
		}

		// A real occurred
		log.Printf("Error retrieving details for user '%s' from database: %v", userName, err)
		return user, nil
	}

	// Determine an appropriate URL for the users' profile pic
	if user.AvatarURL == "" {
		// No avatar URL is presently stored, so default to a gravatar based on users email (if known)
		if user.Email != "" {
			picHash := md5.Sum([]byte(user.Email))
			user.AvatarURL = fmt.Sprintf("https://www.gravatar.com/avatar/%x?d=identicon", picHash)
		}
	}
	return user, nil
}

// UserNameFromAuth0ID returns the username for a given Auth0 ID
func UserNameFromAuth0ID(auth0id string) (string, error) {
	// Query the database for a username matching the given Auth0 ID
	dbQuery := `
		SELECT user_name
		FROM users
		WHERE auth0_id = $1`
	var userName string
	err := DB.QueryRow(context.Background(), dbQuery, auth0id).Scan(&userName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No matching user for the given Auth0 ID
			return "", nil
		}

		// A real occurred
		log.Printf("Error looking up username in database: %v", err)
		return "", nil
	}

	return userName, nil
}
