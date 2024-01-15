package common

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	valid "github.com/go-playground/validator/v10"
)

var (
	regexBraTagName      = regexp.MustCompile(`^[a-z,A-Z,0-9,\^,\.,\-,\_,\/,\(,\),\:,\&,\ )]+$`)
	regexDBName          = regexp.MustCompile(`^[a-z,A-Z,0-9,\.,\-,\_,\(,\),\+,\ ]+$`)
	regexDiscussTitle    = regexp.MustCompile(`^[a-z,A-Z,0-9,\^,\.,\-,\_,\/,\(,\),\',\!,\@,\#,\&,\$,\+,\:,\;,\?,\ )]+$`)
	regexFieldName       = regexp.MustCompile(`^[a-z,A-Z,0-9,\^,\.,\-,\_,\/,\(,\),\ )]+$`)
	regexLicence         = regexp.MustCompile(`^[a-z,A-Z,0-9,\.,\-,\_,\(,\),\ ]+$`)
	regexLicenceFullName = regexp.MustCompile(`^[a-z,A-Z,0-9,\.,\-,\_,\(,\),\ ]+$`)
	regexMarkDownSource  = regexp.MustCompile(`^[a-z,A-Z,0-9` + ",`," + `‘,’,“,”,\.,\-,\_,\/,\(,\),\[,\],\\,\!,\#,\',\",\@,\$,\*,\%,\^,\&,\+,\=,\:,\;,\<,\>,\,,\?,\~,\|,\ ,\012,\015]+$`)
	regexPGTable         = regexp.MustCompile(`^[a-z,A-Z,0-9,\.,\-,\_,\(,\),\ ]+$`)
	regexUsername        = regexp.MustCompile(`^[a-z,A-Z,0-9,\.,\-,\_]+$`)
	regexUuid            = regexp.MustCompile(`^[0-9a-fA-F]{8}\b-[0-9a-fA-F]{4}\b-[0-9a-fA-F]{4}\b-[0-9a-fA-F]{4}\b-[0-9a-fA-F]{12}$`)

	// Validate is used for input validation
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
	Validate.RegisterValidation("licence", checkLicence)
	Validate.RegisterValidation("licencefullname", checkLicenceFullName)
	Validate.RegisterValidation("markdownsource", checkMarkDownSource)
	Validate.RegisterValidation("pgtable", checkPGTableName)
	Validate.RegisterValidation("username", checkUsername)
	Validate.RegisterValidation("uuid", checkUuid)

	// Custom validation functions
	Validate.RegisterValidation("visname", checkVisName)
}

// checkBranchOrTagName is a custom validation function for branch and tag names
// At the moment it just allows alphanumeric and "^.-_/():& " chars, though it should probably be extended to cover any
// valid file name
func checkBranchOrTagName(fl valid.FieldLevel) bool {
	// TODO: Replace this regex with something that allow for all valid unicode characters, minus:
	//         * the Unicode control ones
	//         * the ascii control ones
	//         * special characters recognised by either SQLite or PostgreSQL
	return regexBraTagName.MatchString(fl.Field().String())
}

// checkDBName is a custom validation function for SQLite database names
// At the moment it just allows alphanumeric and ".-_()+ " chars, though it should probably be extended to cover any
// valid file name
func checkDBName(fl valid.FieldLevel) bool {
	// TODO: Replace this regex with something that allow for all valid unicode characters, minus:
	//         * the Unicode control ones
	//         * the ascii control ones
	//         * special characters recognised by either SQLite or PostgreSQL
	return regexDBName.MatchString(fl.Field().String())
}

// checkDiscussTitle is a custom validation function for discussion titles
// At the moment it just allows alpha and "^.-_/()'!@#&$+:;? " chars
func checkDiscussTitle(fl valid.FieldLevel) bool {
	// TODO: Replace this regex with something that allow for all valid unicode characters, minus:
	//         * the Unicode control ones
	//         * the ascii control ones
	//         * special characters recognised by either SQLite or PostgreSQL
	return regexDiscussTitle.MatchString(fl.Field().String())
}

// checkDisplayName is a custom validation function for display names
func checkDisplayName(fl valid.FieldLevel) bool {
	input := fl.Field().String()

	// Check for the presence of unicode control characters and similar in the decoded string
	invalidChar := false
	for _, j := range input {
		if unicode.IsControl(j) || unicode.Is(unicode.C, j) {
			invalidChar = true
		}

		switch j {
		// Check for any of the characters which have special meaning in SQLite.  Two exceptions are (') and (,) which
		// seem to be reasonably common in names so we'll allow:
		// https://github.com/sqlite/sqlite/blob/d31fcd4751745b1fe2e263cd31792debb2e21b52/src/tokenize.c
		case '$', '@', '#', ':', '?', '"', '`', '[', ']', '|', '<', '>', '=', '!', '/', '(', ')', ';', '+', '%', '&',
			'~', '.':
			invalidChar = true

		// Other characters that probably don't belong in display names
		case '*', '^', '_', '\\', '{', '}':
			invalidChar = true
		}
	}
	return !invalidChar
}

// checkFieldName is a custom validation function for SQLite field names
// At the moment it just allows alphanumeric and "^.-_/() " chars, though it should probably be extended to cover all
// valid SQLite field name characters
func checkFieldName(fl valid.FieldLevel) bool {
	// TODO: Replace this regex with something that allow for all valid unicode characters, minus:
	//         * the Unicode control ones
	//         * the ascii control ones
	//         * special characters recognised by either SQLite or PostgreSQL
	return regexFieldName.MatchString(fl.Field().String())
}

// checkLicence is a custom validation function for licence (ID) names
// At the moment it allows alphanumeric and ".-_() " chars.  Will probably need more characters added.
func checkLicence(fl valid.FieldLevel) bool {
	return regexLicence.MatchString(fl.Field().String())
}

// checkLicenceFullName is a custom validation function for licence full names
// At the moment it allows alphanumeric and ".-_() " chars.
func checkLicenceFullName(fl valid.FieldLevel) bool {
	return regexLicenceFullName.MatchString(fl.Field().String())
}

// checkMarkDownSource is a custom validation function for Markdown source text
// At the moment it allows Unicode alphanumeric, "`‘’“”.-_/()[]\#\!'"@$*%^&+=:;<>,?~| ", and "\r\n" chars.  Will probably need more characters added.
func checkMarkDownSource(fl valid.FieldLevel) bool {
	// TODO: Replace this regex with something that allow for all valid unicode characters, minus:
	//         * the Unicode control ones
	//         * the ascii control ones
	//         * special characters recognised by either SQLite or PostgreSQL
	return regexMarkDownSource.MatchString(fl.Field().String())
}

// checkPGTableName is a custom validation function for PostgreSQL table names
// At the moment it just allows alphanumeric and ".-_ " chars (may need to be expanded out at some point).
func checkPGTableName(fl valid.FieldLevel) bool {
	// TODO: Replace this regex with something that allow for all valid unicode characters, minus:
	//         * the Unicode control ones
	//         * the ascii control ones
	//         * special characters recognised by PostgreSQL
	return regexPGTable.MatchString(fl.Field().String())
}

// checkUsername is a custom validation function for Usernames
// At the moment it just allows alphanumeric and ".-_" chars (may need to be expanded out at some point).
func checkUsername(fl valid.FieldLevel) bool {
	// TODO: Replace this regex with something that allow for all valid unicode characters, minus:
	//         * the Unicode control ones
	//         * the ascii control ones
	//         * special characters recognised by either SQLite or PostgreSQL
	return regexUsername.MatchString(fl.Field().String())
}

func checkUuid(fl valid.FieldLevel) bool {
	return regexUuid.MatchString(fl.Field().String())
}

// checkVisName is a custom validation function for Visualisation names
func checkVisName(fl valid.FieldLevel) bool {
	input := fl.Field().String()

	// Check for the presence of unicode control characters and similar in the decoded string
	invalidChar := false
	for _, j := range input {
		if unicode.IsControl(j) || unicode.Is(unicode.C, j) {
			invalidChar = true
		}

		switch j {
		// Check for any of the characters which have special meaning in SQLite
		// https://github.com/sqlite/sqlite/blob/d31fcd4751745b1fe2e263cd31792debb2e21b52/src/tokenize.c
		case '$', '@', '#', ':', '?', '"', '`', '[', ']', '|', '<', '>', '=', '!', '/', '(', ')', ';', '+', '%', '&',
			'~', '.', '\'', ',':
			invalidChar = true

		// Other characters that probably don't belong in visualisation names
		case '*', '^', '\\', '{', '}':
			invalidChar = true
		}
	}
	return !invalidChar
}

// ReservedUsernamesCheck checks a username against the list of reserved ones
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

// ValidateBranchName validates the provided branch, release, or tag name
func ValidateBranchName(fieldName string) error {
	err := Validate.Var(fieldName, "branchortagname,min=1,max=32") // 32 seems a reasonable first guess
	if err != nil {
		return err
	}
	return nil
}

// ValidateCommitID validates the provided commit ID
func ValidateCommitID(fieldName string) error {
	err := Validate.Var(fieldName, "hexadecimal,min=64,max=64") // Always 64 alphanumeric characters
	if err != nil {
		return err
	}
	return nil
}

// ValidateDB validates the database name
func ValidateDB(dbName string) error {
	err := Validate.Var(dbName, "required,dbname,min=1,max=256") // 256 char limit seems reasonable
	if err != nil {
		return err
	}
	return nil
}

// ValidateDisplayName validates a provided full name
func ValidateDisplayName(dbName string) error {
	err := Validate.Var(dbName, "required,displayname,min=1,max=80") // 80 char limit seems reasonable
	if err != nil {
		return err
	}
	return nil
}

// ValidateEmail validates the provided email address
func ValidateEmail(email string) error {
	err := Validate.Var(email, "required,email")
	if err != nil {
		return err
	}
	return nil
}

// ValidateFieldName validates the SQLite field name
func ValidateFieldName(fieldName string) error {
	err := Validate.Var(fieldName, "required,fieldname,min=1,max=63") // 63 char limit seems reasonable
	if err != nil {
		return err
	}
	return nil
}

// ValidateLicence validates the provided licence name (ID)
func ValidateLicence(licence string) error {
	err := Validate.Var(licence, "licence,min=1,max=13") // 13 is the length of our longest licence name (thus far)
	if err != nil {
		return err
	}
	return nil
}

// ValidateLicenceFullName validate the provided licence full name
func ValidateLicenceFullName(licence string) error {
	err := Validate.Var(licence, "licencefullname,min=1,max=70") // Our longest licence full name (thus far) is 61 chars, so 70 is a reasonable start
	if err != nil {
		return err
	}
	return nil
}

// ValidateMarkdown validates the provided markdown
func ValidateMarkdown(fieldName string) error {
	err := Validate.Var(fieldName, "markdownsource,max=1024") // 1024 seems like a reasonable first guess
	if err != nil {
		return err
	}
	return nil
}

// ValidatePGTable validates the provided PostgreSQL table name
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

// ValidateDiscussionTitle validates the provided discussion or merge request title
func ValidateDiscussionTitle(fieldName string) error {
	err := Validate.Var(fieldName, "discussiontitle,max=120") // 120 seems a reasonable first guess.
	if err != nil {
		return err
	}
	return nil
}

// ValidateUser validates the provided username
func ValidateUser(user string) error {
	err := Validate.Var(user, "required,username,min=2,max=63")
	if err != nil {
		return err
	}
	return nil
}

// ValidateUserDB validates the provided user and database name
func ValidateUserDB(user, db string) error {
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

// ValidateUuid validates a uuid
func ValidateUuid(uuid string) error {
	err := Validate.Var(uuid, "required,uuid")
	if err != nil {
		return err
	}
	return nil
}

// ValidateVisualisationName validates the provided name of a saved visualisation query
func ValidateVisualisationName(name string) error {
	err := Validate.Var(name, "required,visname,min=1,max=63")
	if err != nil {
		return err
	}
	return nil
}
