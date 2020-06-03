package common

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	valid "github.com/go-playground/validator/v10"
)

var (
	// Regex based validation functions
	regexLicence         = regexp.MustCompile(`^[a-z,A-Z,0-9,\.,\-,\_,\(,\),\ ]+$`)
	regexLicenceFullName = regexp.MustCompile(`^[a-z,A-Z,0-9,\.,\-,\_,\(,\),\ ]+$`)
	regexMarkDownSource  = regexp.MustCompile(`^[a-z,A-Z,0-9` + ",`," + `‘,’,“,”,\.,\-,\_,\/,\(,\),\[,\],\\,\!,\#,\',\",\@,\$,\*,\%,\^,\&,\+,\=,\:,\;,\<,\>,\,,\?,\~,\|,\ ,\012,\015]+$`)

	// For input validation
	Validate *valid.Validate
)

type VisGetFields struct {
	VisName string `validate:"required,visname,min=1,max=63"` // 63 char limit seems reasonable
}

type VisSaveFields struct {
	ChartType      string `validate:"required,charttype"`
	ShowXAxisLabel string `validate:"bool"`
	ShowYAxisLabel string `validate:"bool"`
	SQL            string `validate:"required,base64sql"`
	VisName        string `validate:"required,visname,min=1,max=63"` // 63 char limit seems reasonable
	XAxis          string `validate:"required,axisname,min=1,max=63"`
	YAxis          string `validate:"required,axisname,min=1,max=63"`
}

func init() {
	// Load validation code
	Validate = valid.New()
	Validate.RegisterValidation("branchortagname", checkGenericTitle)
	Validate.RegisterValidation("dbname", checkDBName)
	Validate.RegisterValidation("discussiontitle", checkGenericTitle)
	Validate.RegisterValidation("displayname", checkDisplayName)
	Validate.RegisterValidation("fieldname", checkVisName) // visName is our most restrictive unicode aware checker (atm), which is probably the correct choice as we have to do potentially unsafe string smashing with this value
	Validate.RegisterValidation("folder", checkFolder)
	Validate.RegisterValidation("licence", checkLicence)
	Validate.RegisterValidation("licencefullname", checkLicenceFullName)
	Validate.RegisterValidation("markdownsource", checkMarkDownSource)
	Validate.RegisterValidation("table", checkVisName) // visName is our most restrictive unicode aware checker (atm), which is probably the correct choice as we have to do potentially unsafe string smashing with this value
	Validate.RegisterValidation("username", checkVisName)

	// Custom validation functions
	Validate.RegisterValidation("axisname", checkVisAxisName)
	Validate.RegisterValidation("base64sql", checkVisBase64SQL)
	Validate.RegisterValidation("charttype", checkVisChartType)
	Validate.RegisterValidation("visname", checkVisName)
}

// Custom validation function for SQLite database names.
func checkDBName(fl valid.FieldLevel) bool {
	input := fl.Field().String()

	// Check for the presence of unicode control characters and similar in the decoded string
	invalidChar := false
	for _, j := range input {
		if unicode.IsControl(j) || unicode.Is(unicode.C, j) {
			invalidChar = true
		}

		switch j {
		// Check for any of the characters which have special meaning in SQLite, except (, ), ., and +, which we allow.
		// https://github.com/sqlite/sqlite/blob/d31fcd4751745b1fe2e263cd31792debb2e21b52/src/tokenize.c
		case '$', '@', '#', ':', '?', '"', '`', '[', ']', '|', '<', '>', '=', '!', '/', ';', '%', '&', '~', '\'', ',':
			invalidChar = true

		// Other characters that probably don't belong in visualisation names
		case '*', '^', '\\', '{', '}':
			invalidChar = true
		}
	}
	return !invalidChar
}

// Custom validation function for generic titles.
func checkGenericTitle(fl valid.FieldLevel) bool {
	input := fl.Field().String()

	// Check for the presence of unicode control characters and similar in the decoded string
	invalidChar := false
	for _, j := range input {
		if unicode.IsControl(j) || unicode.Is(unicode.C, j) {
			invalidChar = true
		}

		switch j {
		// Check for any of the characters which might (potentially) be dangerous:
		// https://github.com/sqlite/sqlite/blob/d31fcd4751745b1fe2e263cd31792debb2e21b52/src/tokenize.c
		case '#', '"', '`', '[', ']', '|', '<', '>', '=', '%', '~', '*', '\\', '{', '}':
			invalidChar = true
		}
	}
	return !invalidChar
}

// Custom validation function for display names.
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

// Custom validation function for folder names.
func checkFolder(fl valid.FieldLevel) bool {
	input := fl.Field().String()

	// Check for the presence of unicode control characters and similar in the decoded string
	invalidChar := false
	for _, j := range input {
		if unicode.IsControl(j) || unicode.Is(unicode.C, j) {
			invalidChar = true
		}

		switch j {
		// Check for any of the characters which have special meaning in SQLite.  We allow the full stop (.) character
		// too, as it's likely to be reasonably common
		// https://github.com/sqlite/sqlite/blob/d31fcd4751745b1fe2e263cd31792debb2e21b52/src/tokenize.c
		case '$', '@', '#', ':', '?', '"', '`', '[', ']', '|', '<', '>', '=', '!', '/', '(', ')', ';', '+', '%', '&',
			'~', '\'', ',':
			invalidChar = true

		// Other characters that probably don't belong in folder names
		case '*', '^', '\\', '{', '}':
			invalidChar = true
		}
	}
	return !invalidChar
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
	// TODO: Replace this regex with something that allow for all valid unicode characters, minus:
	//         * the Unicode control ones
	//         * the ascii control ones
	//         * special characters recognised by either SQLite or PostgreSQL
	return regexMarkDownSource.MatchString(fl.Field().String())
}

// Custom validation function for Visualisation axis names.
func checkVisAxisName(fl valid.FieldLevel) bool {
	input := fl.Field().String()

	// Check for the presence of unicode control characters and similar in the decoded string
	invalidChar := false
	for _, j := range input {
		if unicode.IsControl(j) || unicode.Is(unicode.C, j) {
			invalidChar = true
		}

		switch j {
		// Check for characters which have special meaning in SQLite and probably don't belong in axis names
		// https://github.com/sqlite/sqlite/blob/d31fcd4751745b1fe2e263cd31792debb2e21b52/src/tokenize.c
		case '$', '@', '#', '?', '"', '`', '[', ']', '|', '<', '>', '=', '!', '/', ';', '+', '%', '&', '~', '\'', ',':
			invalidChar = true

		// Other characters that probably don't belong in axis names
		case '*', '^', '\\', '{', '}':
			invalidChar = true
		}
	}
	return !invalidChar
}

// Custom validation function for Base64 encoded SQL queries.
func checkVisBase64SQL(fl valid.FieldLevel) bool {
	d, err := base64.StdEncoding.DecodeString(fl.Field().String())
	if err != nil {
		return false
	}
	decoded := string(d)

	// Ensure the decoded string is valid UTF-8
	if !utf8.ValidString(decoded) {
		return false
	}

	// Check for the presence of unicode control characters and similar in the decoded string
	invalidChar := false
	for _, j := range decoded {
		if unicode.IsControl(j) || unicode.Is(unicode.C, j) {
			if j != 10 { // 10 == new line, which is safe to allow.  Everything else should (probably) raise an error
				// TODO: Check if this works with Windows based browsers, as they may be sending through CR or similar as well
				invalidChar = true
			}
		}
	}
	if invalidChar {
		return false
	}

	// No errors
	return true
}

// Custom validation function for Visualisation chart types.
func checkVisChartType(fl valid.FieldLevel) bool {
	input := fl.Field().String()
	if input != "hbc" && input != "vbc" && input != "lc" && input != "pie" {
		return false
	}
	return true
}

// Custom validation function for Visualisation names.
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

// Validate the provided discussion or merge request title.
func ValidateDiscussionTitle(fieldName string) error {
	err := Validate.Var(fieldName, "discussiontitle,max=120") // 120 seems a reasonable first guess.
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

// Validate the provided SQLite table name.
func ValidateTableName(table string) error {
	err := Validate.Var(table, "required,table,max=63")
	if err != nil {
		return err
	}

	// Exclude SQLite internal tables too (eg "sqlite_*" https://sqlite.org/lang_createtable.html)
	// TODO: Would it be potentially useful to allow these?
	if strings.HasPrefix(table, "sqlite_") {
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
