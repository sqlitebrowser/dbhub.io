// These functions extract (and validate) user provided form data.
package common

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Return the requested branch name, from get or post data.
func GetFormBranch(r *http.Request) (string, error) {
	// If no branch was given in the input, returns an empty string
	c := r.FormValue("branch")
	if c == "" {
		return "", nil
	}
	err := Validate.Var(c, "branchortagname,min=1,max=32") // 32 seems a reasonable first guess.
	if err != nil {
		return "", errors.New(fmt.Sprintf("Invalid branch name: '%v'", c))
	}
	return c, nil
}

// Return the requested database commit, from form data.
func GetFormCommit(r *http.Request) (string, error) {
	// If no commit was given in the input, returns an empty string
	c := r.FormValue("commit")
	if c == "" {
		return "", nil
	}
	err := Validate.Var(c, "hexadecimal,min=1,max=64")
	if err != nil {
		return "", errors.New(fmt.Sprintf("Invalid database commit: '%v'", c))
	}
	return c, nil
}

// Extracts a database name from form data
func GetFormDatabase(r *http.Request) (string, error) {
	dbName := r.PostFormValue("dbname")
	err := ValidateDB(dbName)
	if err != nil {
		log.Printf("Validation failed for database name '%s': %s", dbName, err)
		return "", errors.New("Invalid database name")
	}
	return dbName, nil
}

// Returns the folder name (if any) present in the form data
func GetFormFolder(r *http.Request) (string, error) {
	// Gather submitted form data (if any)
	err := r.ParseForm()
	if err != nil {
		log.Printf("Error when parsing form data: %s\n", err)
		return "", err
	}
	folder := r.PostFormValue("folder")

	// If no folder given, return
	if folder == "" {
		return "", nil
	}

	// Validate the username
	err = ValidateFolder(folder)
	if err != nil {
		log.Printf("Validation failed for folder: '%s': %s", folder, err)
		return "", err
	}

	return folder, nil
}

// Return the requested tag name, from get or post data.
func GetFormTag(r *http.Request) (string, error) {
	// If no tag was given in the input, returns an empty string
	c := r.FormValue("tag")
	if c == "" {
		return "", nil
	}
	err := Validate.Var(c, "branchortagname,min=1,max=32") // 32 seems a reasonable first guess.
	if err != nil {
		return "", errors.New(fmt.Sprintf("Invalid tag name: '%v'", c))
	}
	return c, nil
}

// Return the username, database, and commit (if any) present in the form data.
func GetFormUDC(r *http.Request) (string, string, string, error) {
	// Extract the username
	userName, err := GetFormUsername(r)
	if err != nil {
		return "", "", "", err
	}

	// Extract the database name
	dbName, err := GetFormDatabase(r)
	if err != nil {
		return "", "", "", err
	}

	// Extract the commit string
	commitID, err := GetFormCommit(r)
	if err != nil {
		return "", "", "", err
	}

	return userName, dbName, commitID, nil
}

// Return the username, folder, and database name (if any) present in the form data.
func GetFormUFD(r *http.Request) (string, string, string, error) {
	// Extract the username
	userName, err := GetFormUsername(r)
	if err != nil {
		return "", "", "", err
	}

	// Extract the folder
	dbFolder, err := GetFormFolder(r)
	if err != nil {
		return "", "", "", err
	}

	// Extract the database name
	dbName, err := GetFormDatabase(r)
	if err != nil {
		return "", "", "", err
	}

	return userName, dbFolder, dbName, nil
}

// Return the username, password, and source URL from the form data.
func GetFormUPS(r *http.Request) (string, string, string, error) {
	// Get username
	userName, err := GetFormUsername(r)
	if err != nil {
		return "", "", "", err
	}

	// Get password and Source URL
	password := r.PostFormValue("pass")
	sourceURL := r.PostFormValue("sourceurl")

	// If no username/password was given, return
	if userName == "" && password == "" {
		return "", "", "", err
	}

	// Check the password isn't blank
	if len(password) < 1 {
		log.Print("Password missing")
		return "", "", "", err
	}

	// Validate the source referrer (if present)
	var bounceURL string
	if sourceURL != "" {
		ref, err := url.Parse(sourceURL)
		if err != nil {
			log.Printf("Error when parsing referrer URL for login form: %s\n", err)
		} else {
			// Only use the referrer path if no hostname is set (eg check if someone is screwing around)
			if ref.Host == "" {
				bounceURL = ref.Path
			}
		}
	}

	return userName, password, bounceURL, nil
}

// Return the username (if any) present in the form data.
func GetFormUsername(r *http.Request) (string, error) {
	// Gather submitted form data (if any)
	err := r.ParseForm()
	if err != nil {
		log.Printf("Error when parsing form data: %s\n", err)
		return "", err
	}
	userName := r.PostFormValue("username")

	// If no username given, return
	if userName == "" {
		return "", nil
	}

	// Validate the username
	err = ValidateUser(userName)
	if err != nil {
		log.Printf("Validation failed for username: %s", err)
		return "", err
	}

	return userName, nil
}

// Returns the requested database owner and database name.
func GetOD(ignore_leading int, r *http.Request) (string, string, error) {
	// Split the request URL into path components
	pathStrings := strings.Split(r.URL.Path, "/")

	// Check that at least an owner/database combination was requested
	if len(pathStrings) < (3 + ignore_leading) {
		log.Printf("Something wrong with the requested URL: %v\n", r.URL.Path)
		return "", "", errors.New("Invalid URL")
	}
	dbOwner := pathStrings[1+ignore_leading]
	dbName := pathStrings[2+ignore_leading]

	// Validate the user supplied owner and database name
	err := ValidateUserDB(dbOwner, dbName)
	if err != nil {
		// Don't bother logging the fairly common case of a bot using an AngularJS phrase in a request
		if dbOwner == "{{ meta.Owner + '" && dbName == "' + row.Database }}" {
			return "", "", errors.New("Invalid owner or database name")
		}

		log.Printf("Validation failed for owner or database name. Owner '%s', DB name '%s': %s",
			dbOwner, dbName, err)
		return "", "", errors.New("Invalid owner or database name")
	}

	// Everything seems ok
	return dbOwner, dbName, nil
}

// Returns the requested database owner, database name, and commit revision.
func GetODC(ignore_leading int, r *http.Request) (string, string, string, error) {
	// Grab owner and database name
	dbOwner, dbName, err := GetOD(ignore_leading, r)
	if err != nil {
		return "", "", "", err
	}

	// Extract the commit revision
	commitID, err := GetFormCommit(r)
	if err != nil {
		return "", "", "", err
	}

	// Everything seems ok
	return dbOwner, dbName, commitID, nil
}

// Returns the requested database owner, database name, and table name.
func GetODT(ignore_leading int, r *http.Request) (string, string, string, error) {
	// Grab owner and database name
	dbOwner, dbName, err := GetOD(ignore_leading, r)
	if err != nil {
		return "", "", "", err
	}

	// If a specific table was requested, get that info too
	requestedTable, err := GetTable(r)
	if err != nil {
		return "", "", "", err
	}

	// Everything seems ok
	return dbOwner, dbName, requestedTable, nil
}

// Returns the requested database owner, database name, table name, and commit string.
func GetODTC(ignore_leading int, r *http.Request) (string, string, string, string, error) {
	// Grab owner and database name
	dbOwner, dbName, err := GetOD(ignore_leading, r)
	if err != nil {
		return "", "", "", "", err
	}

	// If a specific table was requested, get that info too
	requestedTable, err := GetTable(r)
	if err != nil {
		return "", "", "", "", err
	}

	// Extract the commit string
	commitID, err := GetFormCommit(r)
	if err != nil {
		return "", "", "", "", err
	}

	// Everything seems ok
	return dbOwner, dbName, requestedTable, commitID, nil
}

// Returns the requested "public" variable, if present in the form data.
// If something goes wrong, it defaults to "false".
func GetPub(r *http.Request) (bool, error) {
	// Gather submitted form data (if any)
	err := r.ParseForm()
	if err != nil {
		log.Printf("Error when parsing form data: %s\n", err)
		return false, err
	}
	val := r.PostFormValue("public")
	if val == "" {
		// No public/private variable found
		return false, errors.New("No public/private value present")
	}
	pub, err := strconv.ParseBool(val)
	if err != nil {
		log.Printf("Error when converting public value to boolean: %v\n", err)
		return false, err
	}

	return pub, nil
}

// Returns the requested table name (if any).
func GetTable(r *http.Request) (string, error) {
	var requestedTable string
	requestedTable = r.FormValue("table")

	// If a table name was supplied, validate it
	// FIXME: We should probably create a validation function for SQLite table names, not use our one for PG
	if requestedTable != "" {
		err := ValidatePGTable(requestedTable)
		if err != nil {
			// If the failed table name is "{{ db.Tablename }}", don't bother logging it.  It's just a
			// search bot picking up the AngularJS string then doing a request with it
			if requestedTable != "{{ db.Tablename }}" {
				log.Printf("Validation failed for table name: '%s': %s", requestedTable, err)
			}
			return "", errors.New("Invalid table name")
		}
	}

	// Everything seems ok
	return requestedTable, nil
}
