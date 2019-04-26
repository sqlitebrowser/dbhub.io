package common

import (
	"fmt"
	"regexp"
	"strings"

	valid "gopkg.in/go-playground/validator.v9"
)

var (
	regexBraTagName      = regexp.MustCompile(`^[a-z,A-Z,0-9,\^,\.,\-,\_,\/,\(,\),\:,\&,\ )]+$`)
	regexDBName          = regexp.MustCompile(`^[a-z,A-Z,0-9,\.,\-,\_,\(,\),\+,\ ]+$`)
	regexDiscussTitle    = regexp.MustCompile(`^[a-z,A-Z,0-9,\^,\.,\-,\_,\/,\(,\),\',\!,\@,\#,\&,\$,\+,\:,\;,\?,\ )]+$`)
	regexDisplayName     = regexp.MustCompile(`^[a-z,A-Z,0-9,\.,\-,\,,\',\ ]+$`)
	regexFieldName       = regexp.MustCompile(`^[a-z,A-Z,0-9,\^,\.,\-,\_,\/,\(,\),\ )]+$`)
	regexFolder          = regexp.MustCompile(`^[a-z,A-Z,0-9,\.,\-,\_,\/]+$`)
	regexLicence         = regexp.MustCompile(`^[a-z,A-Z,0-9,\.,\-,\_,\(,\),\ ]+$`)
	regexLicenceFullName = regexp.MustCompile(`^[a-z,A-Z,0-9,\.,\-,\_,\(,\),\ ]+$`)
	regexMarkDownSource  = regexp.MustCompile(`^[a-z,A-Z,0-9` + ",`," + `‘,’,“,”,\.,\-,\_,\/,\(,\),\[,\],\\,\!,\#,\',\",\@,\$,\*,\%,\^,\&,\+,\=,\:,\;,\<,\>,\,,\?,\~,\|,\ ,\012,\015]+$`)
	regexPGTable         = regexp.MustCompile(`^[a-z,A-Z,0-9,\.,\-,\_,\(,\),\ ]+$`)
	regexUsername        = regexp.MustCompile(`^[a-z,A-Z,0-9,\.,\-,\_]+$`)

	// For input validation
	Validate *valid.Validate
)

func init() {
	// Load validation code
	Validate = valid.New()
	Validate.RegisterValidation("branchortagname", checkBranchOrTagName)
	Validate.RegisterValidation("dbname", checkDBName)
	Validate.RegisterValidation("discussiontitle", checkDiscussTitle)
	Validate.RegisterValidation("displayname", checkDisplayName)
	Validate.RegisterValidation("fieldname", checkFieldName)
	Validate.RegisterValidation("folder", checkFolder)
	Validate.RegisterValidation("licence", checkLicence)
	Validate.RegisterValidation("licencefullname", checkLicenceFullName)
	Validate.RegisterValidation("markdownsource", checkMarkDownSource)
	Validate.RegisterValidation("pgtable", checkPGTableName)
	Validate.RegisterValidation("username", checkUsername)
}

// Custom validation function for branch and tag names.
// At the moment it just allows alphanumeric and "^.-_/():& " chars, though it should probably be extended to cover any
// valid file name
func checkBranchOrTagName(fl valid.FieldLevel) bool {
	return regexBraTagName.MatchString(fl.Field().String())
}

// Custom validation function for SQLite database names.
// At the moment it just allows alphanumeric and ".-_()+ " chars, though it should probably be extended to cover any
// valid file name
func checkDBName(fl valid.FieldLevel) bool {
	return regexDBName.MatchString(fl.Field().String())
}

// Custom validation function for discussion titles.
// At the moment it just allows alpha and "^.-_/()'!@#&$+:;? " chars
func checkDiscussTitle(fl valid.FieldLevel) bool {
	return regexDiscussTitle.MatchString(fl.Field().String())
}

// Custom validation function for display names.
// At the moment it just allows alpha and ".,-' " chars
func checkDisplayName(fl valid.FieldLevel) bool {
	return regexDisplayName.MatchString(fl.Field().String())
}

// Custom validation function for SQLite field names
// At the moment it just allows alphanumeric and "^.-_/() " chars, though it should probably be extended to cover all
// valid SQLite field name characters
func checkFieldName(fl valid.FieldLevel) bool {
	return regexFieldName.MatchString(fl.Field().String())
}

// Custom validation function for folder names.
// At the moment it allows alphanumeric and ".-_/" chars.  Will probably need more characters added.
func checkFolder(fl valid.FieldLevel) bool {
	return regexFolder.MatchString(fl.Field().String())
}

// Custom validation function for licence (ID) names.
// At the moment it allows alphanumeric and ".-_() " chars.  Will probably need more characters added.
func checkLicence(fl valid.FieldLevel) bool {
	return regexLicence.MatchString(fl.Field().String())
}

// Custom validation function for licence full names.
// At the moment it allows alphanumeric and ".-_() " chars.
func checkLicenceFullName(fl valid.FieldLevel) bool {
	return regexLicenceFullName.MatchString(fl.Field().String())
}

// Custom validation function for Markdown source text.
// At the moment it allows Unicode alphanumeric, "`‘’“”.-_/()[]\#\!'"@$*%^&+=:;<>,?~| ", and "\r\n" chars.  Will probably need more characters added.
func checkMarkDownSource(fl valid.FieldLevel) bool {
	return regexMarkDownSource.MatchString(fl.Field().String())
}

// Custom validation function for PostgreSQL table names.
// At the moment it just allows alphanumeric and ".-_ " chars (may need to be expanded out at some point).
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
	reserved := []string{"about", "account", "accounts", "admin", "administrator", "blog", "ceo", "compare", "dbhub",
		"default", "demo", "download", "forks", "legal", "login", "logout", "mail", "news", "pref", "printer", "public",
		"reference", "register", "root", "sales", "star", "stars", "system", "table", "upload", "uploaddata", "vis",
		"watchers"}
	for _, word := range reserved {
		if strings.ToLower(userName) == strings.ToLower(word) {
			return fmt.Errorf("That username is not available: %s\n", userName)
		}
	}

	return nil
}

// Validate the provided branch, release, or tag name.
func ValidateBranchName(fieldName string) error {
	err := Validate.Var(fieldName, "branchortagname,min=1,max=32") // 32 seems a reasonable first guess
	if err != nil {
		return err
	}

	return nil
}

// Validate the provided commit ID.
func ValidateCommitID(fieldName string) error {
	err := Validate.Var(fieldName, "hexadecimal,min=64,max=64") // Always 64 alphanumeric characters
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

// Validate a provided full name.
func ValidateDisplayName(dbName string) error {
	err := Validate.Var(dbName, "required,displayname,min=1,max=80") // 80 char limit seems reasonable
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

// Validate the SQLite field name.
func ValidateFieldName(fieldName string) error {
	err := Validate.Var(fieldName, "required,fieldname,min=1,max=63") // 63 char limit seems reasonable
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

// Validate the provided licence name (ID).
func ValidateLicence(licence string) error {
	err := Validate.Var(licence, "licence,min=1,max=13") // 13 is the length of our longest licence name (thus far)
	if err != nil {
		return err
	}

	return nil
}

// Validate the provided licence full name.
func ValidateLicenceFullName(licence string) error {
	err := Validate.Var(licence, "licencefullname,min=1,max=70") // Our longest licence full name (thus far) is 61 chars, so 70 is a reasonable start
	if err != nil {
		return err
	}

	return nil
}

// Validate the provided markdown.
func ValidateMarkdown(fieldName string) error {
	err := Validate.Var(fieldName, "markdownsource,max=1024") // 1024 seems like a reasonable first guess
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

// Validate the provided discussion or merge request title.
func ValidateDiscussionTitle(fieldName string) error {
	err := Validate.Var(fieldName, "discussiontitle,max=120") // 120 seems a reasonable first guess.
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
