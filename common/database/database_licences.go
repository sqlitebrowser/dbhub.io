package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/sqlitebrowser/dbhub.io/common/config"

	pgx "github.com/jackc/pgx/v5"
)

type LicenceEntry struct {
	FileFormat string `json:"file_format"`
	FullName   string `json:"full_name"`
	Order      int    `json:"order"`
	Sha256     string `json:"sha256"`
	URL        string `json:"url"`
}

// AddDefaultLicences adds the default licences to the PostgreSQL database.  Generally useful for populating a new
// database, or adding new entries to an existing one
func AddDefaultLicences() (err error) {
	// The default licences to load into the system
	type licenceInfo struct {
		DisplayOrder int
		FileFormat   string
		FullName     string
		Path         string
		URL          string
	}
	licences := map[string]licenceInfo{
		"Not specified": {
			DisplayOrder: 100,
			FileFormat:   "text",
			FullName:     "No licence specified",
			Path:         "",
			URL:          ""},
		"CC0": {
			DisplayOrder: 200,
			FileFormat:   "text",
			FullName:     "Creative Commons Zero 1.0",
			Path:         "CC0-1.0.txt",
			URL:          "https://creativecommons.org/publicdomain/zero/1.0/"},
		"CC-BY-4.0": {
			DisplayOrder: 300,
			FileFormat:   "text",
			FullName:     "Creative Commons Attribution 4.0 International",
			Path:         "CC-BY-4.0.txt",
			URL:          "https://creativecommons.org/licenses/by/4.0/"},
		"CC-BY-SA-4.0": {
			DisplayOrder: 400,
			FileFormat:   "text",
			FullName:     "Creative Commons Attribution-ShareAlike 4.0 International",
			Path:         "CC-BY-SA-4.0.txt",
			URL:          "https://creativecommons.org/licenses/by-sa/4.0/"},
		"CC-BY-NC-4.0": {
			DisplayOrder: 500,
			FileFormat:   "text",
			FullName:     "Creative Commons Attribution-NonCommercial 4.0 International",
			Path:         "CC-BY-NC-4.0.txt",
			URL:          "https://creativecommons.org/licenses/by-nc/4.0/"},
		"CC-BY-IGO-3.0": {
			DisplayOrder: 600,
			FileFormat:   "html",
			FullName:     "Creative Commons Attribution 3.0 IGO",
			Path:         "CC-BY-IGO-3.0.html",
			URL:          "https://creativecommons.org/licenses/by/3.0/igo/"},
		"ODbL-1.0": {
			DisplayOrder: 700,
			FileFormat:   "text",
			FullName:     "Open Data Commons Open Database License 1.0",
			Path:         "ODbL-1.0.txt",
			URL:          "https://opendatacommons.org/licenses/odbl/1.0/"},
		"UK-OGL-3": {
			DisplayOrder: 800,
			FileFormat:   "html",
			FullName:     "United Kingdom Open Government Licence 3",
			Path:         "UK-OGL3.html",
			URL:          "https://www.nationalarchives.gov.uk/doc/open-government-licence/version/3/"},
	}

	// Add the default licences to PostgreSQL
	for lName, l := range licences {
		txt := []byte{}
		if l.Path != "" {
			// Read the file contents
			txt, err = os.ReadFile(filepath.Join(config.Conf.Licence.LicenceDir, l.Path))
			if err != nil {
				return err
			}
		}

		// Save the licence text, sha256, and friendly name in the database
		err = StoreLicence("default", lName, txt, l.URL, l.DisplayOrder, l.FullName, l.FileFormat)
		if err != nil {
			return err
		}
	}
	log.Printf("%s: default licences added", config.Conf.Live.Nodename)
	return nil
}

// CheckLicenceExists checks if a given licence exists in our system
func CheckLicenceExists(userName, licenceName string) (exists bool, err error) {
	dbQuery := `
		SELECT count(*)
		FROM database_licences
		WHERE friendly_name = $2
			AND (user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = 'default'
			)
			OR user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			))`
	var count int
	err = DB.QueryRow(context.Background(), dbQuery, userName, licenceName).Scan(&count)
	if err != nil {
		log.Printf("Error checking if licence '%s' exists for user '%s' in database: %v",
			licenceName, userName, err)
		return false, err
	}
	if count == 0 {
		// The requested licence wasn't found
		return false, nil
	}
	return true, nil
}

// DeleteLicence removes a (user supplied) database licence from the system
func DeleteLicence(userName, licenceName string) (err error) {
	// Begin a transaction
	tx, err := DB.Begin(context.Background())
	if err != nil {
		return err
	}

	// Set up an automatic transaction roll back if the function exits without committing
	defer tx.Rollback(context.Background())

	// Don't allow deletion of the default licences
	switch licenceName {
	case "Not specified":
	case "CC0":
	case "CC-BY-4.0":
	case "CC-BY-SA-4.0":
	case "CC-BY-NC-4.0":
	case "CC-BY-IGO-3.0":
	case "ODbL-1.0":
	case "UK-OGL-3":
		return errors.New("Default licences can't be removed")
	}

	// Retrieve the SHA256 for the licence
	licSHA, err := GetLicenceSha256FromName(userName, licenceName)
	if err != nil {
		return err
	}

	// Check if there are databases present which use this licence.  If there are, then abort.
	dbQuery := `
		WITH working_set AS (
			SELECT DISTINCT db.db_id
			FROM sqlite_databases AS db
				CROSS JOIN jsonb_each(db.commit_list) AS firstjoin
				CROSS JOIN jsonb_array_elements(firstjoin.value -> 'tree' -> 'entries') AS secondjoin
			WHERE secondjoin ->> 'licence' = $2
				AND (
					user_id = (
						SELECT user_id
						FROM users
						WHERE user_name = 'default'
					)
					OR user_id = (
						SELECT user_id
						FROM users
						WHERE lower(user_name) = lower($1)
					)
				)
		)
		SELECT count(*)
		FROM working_set`
	var DBCount int
	err = DB.QueryRow(context.Background(), dbQuery, userName, licSHA).Scan(&DBCount)
	if err != nil {
		log.Printf("Checking if the licence is in use failed: %v", err)
		return err
	}
	if DBCount != 0 {
		// Database isn't in our system
		return errors.New("Can't delete the licence, as it's already being used by databases")
	}

	// Delete the licence
	dbQuery = `
		DELETE FROM database_licences
		WHERE lic_sha256 = $2
			AND friendly_name = $3
			AND (user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = 'default'
			)
			OR user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			))`
	commandTag, err := tx.Exec(context.Background(), dbQuery, userName, licSHA, licenceName)
	if err != nil {
		log.Printf("Error when retrieving sha256 for licence '%s', user '%s' from database: %v",
			licenceName, userName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when deleting licence '%s' for user '%s'",
			numRows, licenceName, userName)
	}

	// Commit the transaction
	err = tx.Commit(context.Background())
	if err != nil {
		return err
	}

	return nil
}

// GetLicence returns the text for a given licence
func GetLicence(userName, licenceName string) (txt, format string, err error) {
	dbQuery := `
		SELECT licence_text, file_format
		FROM database_licences
		WHERE friendly_name ILIKE $2
		AND (
				user_id = (
					SELECT user_id
					FROM users
					WHERE lower(user_name) = lower($1)
				) OR
				user_id = (
					SELECT user_id
					FROM users
					WHERE user_name = 'default'
				)
			)`
	err = DB.QueryRow(context.Background(), dbQuery, userName, licenceName).Scan(&txt, &format)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The requested licence text wasn't found
			return "", "", errors.New("unknown licence")
		}
		log.Printf("Error when retrieving licence '%s', user '%s': %v", licenceName, userName, err)
		return "", "", err
	}
	return txt, format, nil
}

// GetLicences returns the list of licences available to a user
func GetLicences(user string) (map[string]LicenceEntry, error) {
	dbQuery := `
		SELECT friendly_name, full_name, lic_sha256, licence_url, file_format, display_order
		FROM database_licences
		WHERE user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = 'default'
			)
			OR user_id = (
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
	lics := make(map[string]LicenceEntry)
	for rows.Next() {
		var name string
		var oneRow LicenceEntry
		err = rows.Scan(&name, &oneRow.FullName, &oneRow.Sha256, &oneRow.URL, &oneRow.FileFormat, &oneRow.Order)
		if err != nil {
			log.Printf("Error retrieving licence list: %v", err)
			return nil, err
		}
		lics[name] = oneRow
	}
	return lics, nil
}

// GetLicenceInfoFromSha256 returns the friendly name + licence URL for the licence matching a given sha256
// Note - When user defined licence has the same sha256 as a default one we return the user defined licences' friendly
// name
func GetLicenceInfoFromSha256(userName, sha256 string) (lName, lURL string, err error) {
	dbQuery := `
		SELECT u.user_name, dl.friendly_name, dl.licence_url
		FROM database_licences AS dl, users AS u
		WHERE dl.lic_sha256 = $2
			AND dl.user_id = u.user_id
			AND (dl.user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = 'default'
			)
			OR dl.user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			))`
	rows, err := DB.Query(context.Background(), dbQuery, userName, sha256)
	if err != nil {
		log.Printf("Error when retrieving friendly name for licence sha256 '%s', user '%s': %v", sha256,
			userName, err)
		return "", "", err
	}
	defer rows.Close()
	type lic struct {
		Licence string
		Name    string
		User    string
	}
	var list []lic
	for rows.Next() {
		var oneRow lic
		err = rows.Scan(&oneRow.User, &oneRow.Name, &oneRow.Licence)
		if err != nil {
			log.Printf("Error retrieving friendly name for licence sha256 '%s', user: %v", sha256, err)
			return "", "", err
		}
		list = append(list, oneRow)
	}

	// Decide what to return based upon the number of licence matches
	numLics := len(list)
	switch numLics {
	case 0:
		// If there are no matching sha256's, something has gone wrong
		return "", "", errors.New("No matching licence found, something has gone wrong!")
	case 1:
		// If there's only one matching sha256, we return the corresponding licence name + url
		lName = list[0].Name
		lURL = list[0].Licence
		return lName, lURL, nil
	default:
		// If more than one name was found for the matching sha256, that seems a bit trickier.  At least one of them
		// would have to be a user defined licence, so we'll return the first one of those instead of the default
		// licence name.  This seems to allow users to define their own friendly name's for the default licences which
		// is probably not a bad thing
		for _, j := range list {
			if j.User == userName {
				lName = j.Name
				lURL = j.Licence
				break
			}
		}
	}
	if lName == "" {
		// Multiple licence friendly names were returned, but none of them matched the requesting user.  Something has
		// gone wrong
		return "", "", fmt.Errorf("Multiple matching licences found, but belonging to user %s", userName)
	}

	// To get here we must have successfully picked a user defined licence out of several matches.  This seems like
	// an acceptable scenario
	return lName, lURL, nil
}

// GetLicenceSha256FromName returns the sha256 for a given licence
func GetLicenceSha256FromName(userName, licenceName string) (sha256 string, err error) {
	dbQuery := `
		SELECT lic_sha256
		FROM database_licences
		WHERE friendly_name = $2
			AND (user_id = (
				SELECT user_id
				FROM users
				WHERE user_name = 'default'
			)
			OR user_id = (
				SELECT user_id
				FROM users
				WHERE lower(user_name) = lower($1)
			))`
	err = DB.QueryRow(context.Background(), dbQuery, userName, licenceName).Scan(&sha256)
	if err != nil {
		log.Printf("Error when retrieving sha256 for licence '%s', user '%s' from database: %v",
			licenceName, userName, err)
		return "", err
	}
	if sha256 == "" {
		// The requested licence wasn't found
		return "", errors.New("Licence not found")
	}
	return sha256, nil
}

// StoreLicence stores a licence
func StoreLicence(userName, licenceName string, txt []byte, url string, orderNum int, fullName, fileFormat string) error {
	// Store the licence in PostgreSQL
	sha := sha256.Sum256(txt)
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		INSERT INTO database_licences (user_id, friendly_name, lic_sha256, licence_text, licence_url, display_order,
			full_name, file_format)
		SELECT (SELECT user_id FROM u), $2, $3, $4, $5, $6, $7, $8
		ON CONFLICT (user_id, friendly_name)
			DO UPDATE
			SET friendly_name = $2,
				lic_sha256 = $3,
				licence_text = $4,
				licence_url = $5,
				user_id = (SELECT user_id FROM u),
				display_order = $6,
				full_name = $7,
				file_format = $8`
	commandTag, err := DB.Exec(context.Background(), dbQuery, userName, licenceName, hex.EncodeToString(sha[:]), txt, url, orderNum,
		fullName, fileFormat)
	if err != nil {
		log.Printf("Inserting licence '%v' in database failed: %v", licenceName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when storing licence '%v'", numRows, licenceName)
	}
	return nil
}
