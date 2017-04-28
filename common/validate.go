package common

import (
	"fmt"
	"regexp"

	valid "gopkg.in/go-playground/validator.v9"
)

var (
	regexDBName    = regexp.MustCompile(`^[a-z,A-Z,0-9,\.,\-,\_,\ ]+$`)
	regexFieldName = regexp.MustCompile(`^[a-z,A-Z,0-9,\.,\-,\_]+$`)
	regexFolder    = regexp.MustCompile(`^[a-z,A-Z,0-9,\.,\-,\_,//]+$`)
	regexPGTable   = regexp.MustCompile(`^[a-z,A-Z,0-9,\.,\-,\_]+$`)
	regexUsername  = regexp.MustCompile(`^[a-z,A-Z,0-9,\.,\-,\_]+$`)

	// For input validation
	Validate *valid.Validate
)

func init() {
	// Load validation code
	Validate = valid.New()
	Validate.RegisterValidation("dbname", checkDBName)
	Validate.RegisterValidation("fieldname", checkFieldName)
	Validate.RegisterValidation("folder", checkFolder)
	Validate.RegisterValidation("pgtable", checkPGTableName)
	Validate.RegisterValidation("username", checkUsername)
}

// Custom validation function for SQLite database names.
// At the moment it just allows alphanumeric and ".-_ " chars, though it should probably be extended to cover any
// valid file name
func checkDBName(fl valid.FieldLevel) bool {
	return regexDBName.MatchString(fl.Field().String())
}

// Custom validation function for SQLite field names
// At the moment it just allows alphanumeric and ".-_" chars, though it should probably be extended to cover all valid
// SQLite field name characters
func checkFieldName(fl valid.FieldLevel) bool {
	return regexFieldName.MatchString(fl.Field().String())
}

// Custom validation function for folder names.
// At the moment it allows alphanumeric and ".-_/" chars.  Will probably need more characters added.
func checkFolder(fl valid.FieldLevel) bool {
	return regexFolder.MatchString(fl.Field().String())
}

// Custom validation function for PostgreSQL table names.
// At the moment it just allows alphanumeric and ".-_" chars (may need to be expanded out at some point).
func checkPGTableName(fl valid.FieldLevel) bool {
	return regexPGTable.MatchString(fl.Field().String())
}

// Custom validation function for Usernames.
// At the moment it just allows alphanumeric and ".-_" chars (may need to be expanded out at some point).
func checkUsername(fl valid.FieldLevel) bool {
	return regexUsername.MatchString(fl.Field().String())
}

// Checks a username against the list of reserved ones.
func ReservedUsernamesCheck(userName string) error {
	reserved := []string{"about", "admin", "blog", "dbhub", "download", "downloadcsv", "forks", "legal", "login",
		"logout", "mail", "news", "pref", "printer", "public", "reference", "register", "root", "star",
		"stars", "system", "table", "upload", "uploaddata", "vis"}
	for _, word := range reserved {
		if userName == word {
			return fmt.Errorf("That username is not available: %s\n", userName)
		}
	}

	return nil
}

// Validate the SQLite field name
func ValidateFieldName(fieldName string) error {
	err := Validate.Var(fieldName, "required,fieldname,min=1,max=63") // 63 char limit seems reasonable
	if err != nil {
		return err
	}

	return nil
}

// Validate the database name.
func ValidateDB(dbName string) error {
	err := Validate.Var(dbName, "required,dbname,min=1,max=256") // 256 char limit seems reasonable
	if err != nil {
		return err
	}

	return nil
}

// Validate the provided email address.
func ValidateEmail(email string) error {
	err := Validate.Var(email, "required,email")
	if err != nil {
		return err
	}

	return nil
}

// Validate the provided folder name.
func ValidateFolder(folder string) error {
	err := Validate.Var(folder, "folder,max=127")
	if err != nil {
		return err
	}

	return nil
}

// Validate the provided PostgreSQL table name.
func ValidatePGTable(table string) error {
	// TODO: Improve this to work with all valid SQLite identifiers
	// TODO  Not seeing a definitive reference page for SQLite yet, so using the PostgreSQL one is
	// TODO  probably ok as a fallback:
	// TODO      https://www.postgresql.org/docs/current/static/sql-syntax-lexical.html#SQL-SYNTAX-IDENTIFIERS
	// TODO: Should we exclude SQLite internal tables too? (eg "sqlite_*" https://sqlite.org/lang_createtable.html)
	err := Validate.Var(table, "required,pgtable,max=63")
	if err != nil {
		return err
	}

	return nil
}

// Validate the provided username.
func ValidateUser(user string) error {
	err := Validate.Var(user, "required,username,min=2,max=63")
	if err != nil {
		return err
	}

	return nil
}

// Validate the provided user and database name.
func ValidateUserDB(user string, db string) error {
	err := ValidateUser(user)
	if err != nil {
		return err
	}

	err = ValidateDB(db)
	if err != nil {
		return err
	}

	return nil
}

// Validate the provided user, database, and table name.
func ValidateUserDBTable(user string, db string, table string) error {
	err := ValidateUserDB(user, db)
	if err != nil {
		return err
	}

	err = ValidatePGTable(table)
	if err != nil {
		return err
	}

	return nil
}

// Validate the provided username and email address.
func ValidateUserEmail(user string, email string) error {
	err := ValidateUser(user)
	if err != nil {
		return err
	}

	err = ValidateEmail(email)
	if err != nil {
		return err
	}

	return nil
}
