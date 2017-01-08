package main

import (
	"fmt"
	"regexp"

	"gopkg.in/go-playground/validator.v9"
)

var regexDBName = regexp.MustCompile(`^[a-z,A-Z,0-9,\.,\-,\_,\ ]+$`)
var regexPGTable = regexp.MustCompile(`^[a-z,A-Z,0-9,\.,\-,\_]+$`)

// Custom validation function for SQLite database names
// At the moment it just allows alphanumeric and ".-_ " chars, though it should probably be extended to cover any
// valid file name
func checkDBName(fl validator.FieldLevel) bool {
	return regexDBName.MatchString(fl.Field().String())
}

// Custom validation function for PostgreSQL table names
// At the moment it just allows alphanumeric and ".-_" chars (may need to be expanded out at some point)
func checkPGTableName(fl validator.FieldLevel) bool {
	return regexPGTable.MatchString(fl.Field().String())
}

// Checks a username against the list of reserved ones
func reservedUsernamesCheck(userName string) error {
	reserved := []string{"about", "admin", "blog", "download", "downloadcsv", "legal", "login", "logout", "mail",
		"news", "pref", "printer", "public", "reference", "register", "root", "star", "stars", "system",
		"table", "upload", "uploaddata", "vis"}
	for _, word := range reserved {
		if userName == word {
			return fmt.Errorf("That username is not available: %s\n", userName)
		}
	}

	return nil
}

// Validate the database name
func validateDB(dbName string) error {
	errs := validate.Var(dbName, "required,dbname,min=1,max=256") // 256 char limit seems reasonable
	if errs != nil {
		return errs
	}

	return nil
}

// Validate the provided email address
func validateEmail(email string) error {
	errs := validate.Var(email, "required,email")
	if errs != nil {
		return errs
	}

	return nil
}

// Validate the provided PostgreSQL table name
func validatePGTable(table string) error {
	// TODO: Improve this to work with all valid SQLite identifiers
	// TODO  Not seeing a definitive reference page for SQLite yet, so using the PostgreSQL one is
	// TODO  probably ok as a fallback:
	// TODO      https://www.postgresql.org/docs/current/static/sql-syntax-lexical.html#SQL-SYNTAX-IDENTIFIERS
	// TODO: Should we exclude SQLite internal tables too? (eg "sqlite_*" https://sqlite.org/lang_createtable.html)
	errs := validate.Var(table, "required,pgtable,max=63")
	if errs != nil {
		return errs
	}

	return nil
}

// Validate a user provided SQLite expression
func validateSQLiteexpr(user_expr string) error {
	errs := validate.Var(user_expr, "sqliteexpr,max=1024")
	if errs != nil {
		return errs
	}

	return nil
}

// Validate the provided username
func validateUser(user string) error {
	errs := validate.Var(user, "required,alphanum,min=3,max=63")
	if errs != nil {
		return errs
	}

	return nil
}

// Validate the provided user and database name
func validateUserDB(user string, db string) error {
	errs := validateUser(user)
	if errs != nil {
		return errs
	}

	errs = validateDB(db)
	if errs != nil {
		return errs
	}

	return nil
}

// Validate the provided username and email address
func validateUserEmail(user string, email string) error {
	errs := validateUser(user)
	if errs != nil {
		return errs
	}

	errs = validateEmail(email)
	if errs != nil {
		return errs
	}

	return nil
}

// Validate the provided user, database, and table name
func validateUserDBTable(user string, db string, table string) error {
	errs := validateUserDB(user, db)
	if errs != nil {
		return errs
	}

	errs = validatePGTable(table)
	if errs != nil {
		return errs
	}

	return nil
}
