package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	mathrand "math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	sqlite "github.com/gwenn/gosqlite"
	"github.com/icza/session"
	"github.com/jackc/pgx"
	"github.com/minio/go-homedir"
	"github.com/minio/minio-go"
	"golang.org/x/crypto/bcrypt"
	valid "gopkg.in/go-playground/validator.v9"
)

type ValType int

const (
	Binary ValType = iota
	Image
	Null
	Text
	Integer
	Float
)

type dataValue struct {
	Name  string
	Type  ValType
	Value interface{}
}
type dataRow []dataValue
type dbInfo struct {
	Database     string
	TableHeaders []string
	Records      []dataRow
	Tables       []string
	Tablename    string
	Watchers     int
	Stars        int
	Forks        int
	Discussions  int
	PRs          int
	Description  string
	Updates      int
	Branches     int
	Releases     int
	Contributors int
	Readme       string
	DateCreated  time.Time
	LastModified time.Time
	Public       bool
	Size         int
	Version      int
}

type metaInfo struct {
	Protocol     string
	Server       string
	Title        string
	Username     string
	Database     string
	LoggedInUser string
}

// Configuration file
type tomlConfig struct {
	Minio minioInfo
	Pg    pgInfo
	Web   webInfo
}

// Minio connection parameters
type minioInfo struct {
	Server    string
	AccessKey string `toml:"access_key"`
	Secret    string
	HTTPS     bool
}

// PostgreSQL connection parameters
type pgInfo struct {
	Server   string
	Port     int
	Username string
	Password string
	Database string
}

type webInfo struct {
	Server         string
	Certificate    string
	CertificateKey string `toml:"certificate_key"`
	RequestLog     string `toml:"request_log"`
}

var (
	// Our configuration info
	conf tomlConfig

	// Connection handles
	db          *pgx.Conn
	minioClient *minio.Client

	// PostgreSQL configuration info
	pgConfig = new(pgx.ConnConfig)

	// Log file for incoming HTTPS requests
	reqLog *os.File

	// Our parsed HTML templates
	tmpl *template.Template

	// For input validation
	validate *valid.Validate
)

func downloadCSVHandler(w http.ResponseWriter, req *http.Request) {
	pageName := "Download CSV"

	// Split the request URL into path components
	pathStrings := strings.Split(req.URL.Path, "/")

	// Basic sanity check
	numPieces := len(pathStrings)
	if numPieces < 4 {
		errorPage(w, req, http.StatusBadRequest, "Invalid database requested")
		return
	}

	// Extract the username, database, table, and version requested
	var dbVersion int64
	userName := pathStrings[2]
	dbName := pathStrings[3]
	queryValues := req.URL.Query()
	dbTable := queryValues["table"][0]
	dbVersion, err := strconv.ParseInt(queryValues["version"][0], 10, 0) // This also validates the version input
	if err != nil {
		log.Printf("%s: Invalid version number: %v\n", pageName, err)
		errorPage(w, req, http.StatusBadRequest, "Invalid version number")
		return
	}

	// Validate the user supplied user, database, and table name
	err = validateUserDBTable(userName, dbName, dbTable)
	if err != nil {
		log.Printf("Validation failed for user, database, or table name: %s", err)
		errorPage(w, req, http.StatusBadRequest, "Invalid user, database, or table name")
		return
	}

	// Abort if no table name was given
	if dbTable == "" {
		log.Printf("%s: No table name given\n", pageName)
		errorPage(w, req, http.StatusBadRequest, "No table name given")
		return
	}

	// Verify the given database exists and is ok to be downloaded (and get the MinioID while at it)
	rows, err := db.Query(`
		SELECT minioid
		FROM database_versions
		WHERE db = (SELECT idnum
			FROM sqlite_databases
			WHERE username = $1
				AND dbname = $2
				AND version = $3)`,
		userName, dbName, dbVersion)
	if err != nil {
		log.Printf("%s: Database query failed: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
		return
	}
	defer rows.Close()
	var minioID string
	for rows.Next() {
		err = rows.Scan(&minioID)
		if err != nil {
			log.Printf("%s: Error retrieving MinioID: %v\n", pageName, err)
			errorPage(w, req, http.StatusInternalServerError, "Database query failed")
			return
		}
	}
	if minioID == "" {
		log.Printf("%s: Couldn't retrieve required MinioID\n", pageName)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Get a handle from Minio for the database object
	userDB, err := minioClient.GetObject(userName, minioID)
	if err != nil {
		log.Printf("%s: Error retrieving DB from Minio: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Close the object handle when this function finishes
	defer func() {
		err := userDB.Close()
		if err != nil {
			log.Printf("%s: Error closing object handle: %v\n", pageName, err)
		}
	}()

	// Save the database locally to a temporary file
	tempfileHandle, err := ioutil.TempFile("", "databaseViewHandler-")
	if err != nil {
		log.Printf("%s: Error creating tempfile: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Internal server error")
		return
	}
	tempfile := tempfileHandle.Name()
	bytesWritten, err := io.Copy(tempfileHandle, userDB)
	if err != nil {
		log.Printf("%s: Error writing database to temporary file: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Internal server error")
		return
	}
	if bytesWritten == 0 {
		log.Printf("%s: 0 bytes written to the temporary file: %v\n", pageName, dbName)
		errorPage(w, req, http.StatusInternalServerError, "Internal server error")
		return
	}
	tempfileHandle.Close()
	defer os.Remove(tempfile) // Delete the temporary file when this function finishes

	// Open database
	db, err := sqlite.Open(tempfile, sqlite.OpenReadOnly)
	if err != nil {
		log.Printf("Couldn't open database: %s", err)
		errorPage(w, req, http.StatusInternalServerError, "Internal error")
		return
	}
	defer db.Close()

	// Retrieve all of the data from the selected database table
	stmt, err := db.Prepare("SELECT * FROM " + dbTable)
	if err != nil {
		log.Printf("Error when preparing statement for database: %s\v", err)
		errorPage(w, req, http.StatusInternalServerError, "Internal error")
		return
	}

	// Process each row
	fieldCount := -1
	var resultSet [][]string
	err = stmt.Select(func(s *sqlite.Stmt) error {

		// Get the number of fields in the result
		if fieldCount == -1 {
			fieldCount = stmt.DataCount()
		}

		// Retrieve the data for each row
		var row []string
		for i := 0; i < fieldCount; i++ {
			// Retrieve the data type for the field
			fieldType := stmt.ColumnType(i)

			isNull := false
			switch fieldType {
			case sqlite.Integer:
				var val int
				val, isNull, err = s.ScanInt(i)
				if err != nil {
					log.Printf("Something went wrong with ScanInt(): %v\n", err)
					break
				}
				if !isNull {
					row = append(row, fmt.Sprintf("%d", val))
				}
			case sqlite.Float:
				var val float64
				val, isNull, err = s.ScanDouble(i)
				if err != nil {
					log.Printf("Something went wrong with ScanDouble(): %v\n", err)
					break
				}
				if !isNull {
					row = append(row, strconv.FormatFloat(val, 'f', 4, 64))
				}
			case sqlite.Text:
				var val string
				val, isNull = s.ScanText(i)
				if !isNull {
					row = append(row, val)
				}
			case sqlite.Blob:
				var val []byte
				val, isNull = s.ScanBlob(i)
				if !isNull {
					// Base64 encode the value
					row = append(row, base64.StdEncoding.EncodeToString(val))
				}
			case sqlite.Null:
				isNull = true
			}
			if isNull {
				row = append(row, "NULL")
			}
		}
		resultSet = append(resultSet, row)

		return nil
	})
	if err != nil {
		log.Printf("Error when reading data from database: %s\v", err)
		errorPage(w, req, http.StatusInternalServerError,
			fmt.Sprintf("Error reading data from '%s'.  Possibly malformed?", dbName))
		return
	}
	defer stmt.Finalize()

	// Convert resultSet into CSV and send to the user
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.csv", dbTable))
	w.Header().Set("Content-Type", "text/csv")
	csvFile := csv.NewWriter(w)
	err = csvFile.WriteAll(resultSet)
	if err != nil {
		log.Printf("%s: Error when generating CSV: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Error when generating CSV")
		return
	}
}

func downloadHandler(w http.ResponseWriter, req *http.Request) {
	pageName := "Download Handler"

	// Split the request URL into path components
	pathStrings := strings.Split(req.URL.Path, "/")

	// Basic sanity check
	numPieces := len(pathStrings)
	if numPieces < 4 {
		errorPage(w, req, http.StatusBadRequest, "Invalid database requested")
		return
	}

	// Extract the username, database, and version requested
	var dbVersion int64
	userName := pathStrings[2]
	dbName := pathStrings[3]
	queryValues := req.URL.Query()
	dbVersion, err := strconv.ParseInt(queryValues["version"][0], 10, 0) // This also validates the version input
	if err != nil {
		log.Printf("%s: Invalid version number: %v\n", pageName, err)
		errorPage(w, req, http.StatusBadRequest, "Invalid version number")
		return
	}

	// Validate the user supplied user and database name
	err = validateUserDB(userName, dbName)
	if err != nil {
		log.Printf("Validation failed for user or database name: %s", err)
		errorPage(w, req, http.StatusBadRequest, "Invalid user or database name")
		return
	}

	// Verify the given database exists and is ok to be downloaded (and get the MinioID while at it)
	rows, err := db.Query(`
		SELECT minioid
		FROM database_versions
		WHERE db = (SELECT idnum
			FROM sqlite_databases
			WHERE username = $1
				AND dbname = $2
				AND version = $3)`,
		userName, dbName, dbVersion)
	if err != nil {
		log.Printf("%s: Database query failed: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
		return
	}
	defer rows.Close()
	var minioID string
	for rows.Next() {
		err = rows.Scan(&minioID)
		if err != nil {
			log.Printf("%s: Error retrieving MinioID: %v\n", pageName, err)
			errorPage(w, req, http.StatusInternalServerError, "Database query failed")
			return
		}
	}
	if minioID == "" {
		log.Printf("%s: Couldn't retrieve required MinioID\n", pageName)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Get a handle from Minio for the database object
	userDB, err := minioClient.GetObject(userName, minioID)
	if err != nil {
		log.Printf("%s: Error retrieving DB from Minio: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Close the object handle when this function finishes
	defer func() {
		err := userDB.Close()
		if err != nil {
			log.Printf("%s: Error closing object handle: %v\n", pageName, err)
		}
	}()

	// Send the database to the user
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", dbName))
	w.Header().Set("Content-Type", "application/x-sqlite3")
	bytesWritten, err := io.Copy(w, userDB)
	if err != nil {
		log.Printf("%s: Error returning DB file: %v\n", pageName, err)
		fmt.Fprintf(w, "%s: Error returning DB file: %v\n", pageName, err)
		return
	}

	// Log the number of bytes written
	log.Printf("%s: '%s/%s' downloaded. %d bytes", pageName, userName, dbName, bytesWritten)
}

func loginHandler(w http.ResponseWriter, req *http.Request) {
	pageName := "Login page"

	// TODO: Add browser side validation of the form data too (using AngularJS?) to save a trip to the server
	// TODO  and make for a nicer user experience for sign up

	// Gather submitted form data (if any)
	err := req.ParseForm()
	if err != nil {
		log.Printf("%s: Error when parsing login data: %s\n", pageName, err)
		errorPage(w, req, http.StatusBadRequest, "Error when parsing login data")
		return
	}
	userName := req.PostFormValue("username")
	password := req.PostFormValue("pass")

	// Check if any (relevant) form data was submitted
	if userName == "" && password == "" {
		// No, so render the login page
		loginPage(w, req)
		return
	}

	// Check the password isn't blank
	if len(password) < 1 {
		log.Printf("%s: Password missing", pageName)
		errorPage(w, req, http.StatusBadRequest, "Password missing")
		return
	}

	// Validate the username
	err = validateUser(userName)
	if err != nil {
		log.Printf("%s: Validation failed for username: %s", pageName, err)
		errorPage(w, req, http.StatusBadRequest, "Invalid username")
		return
	}

	// Retrieve the password hash for the user, if they exist in the database
	row := db.QueryRow("SELECT password_hash FROM public.users WHERE username = $1", userName)
	var passHash []byte
	err = row.Scan(&passHash)
	if err != nil {
		log.Printf("%s: Error looking up password hash for login. User: '%s' Error: %v\n", pageName, userName,
			err)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Hash the user's password
	err = bcrypt.CompareHashAndPassword(passHash, []byte(password))
	if err != nil {
		log.Printf("%s: Login failure, username/password not correct. User: '%s'\n", pageName, userName)
		errorPage(w, req, http.StatusBadRequest, fmt.Sprint("Login failed. Username/password not correct"))
		return
	}

	// Create session cookie
	sess := session.NewSessionOptions(&session.SessOptions{
		CAttrs: map[string]interface{}{"UserName": userName},
	})
	session.Add(sess, w)

	// Bounce to the user page
	http.Redirect(w, req, "/"+userName, http.StatusTemporaryRedirect)
}

func logoutHandler(w http.ResponseWriter, req *http.Request) {
	//pageName := "Log out page"

	// Remove session info
	sess := session.Get(req)
	if sess != nil {
		// Session data was present, so remove it
		session.Remove(sess, w)
	}

	// Bounce to the front page
	// TODO: This should probably reload the existing page instead
	http.Redirect(w, req, "/", http.StatusTemporaryRedirect)
}

// Wrapper function to log incoming https requests
func logReq(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		// Check if user is logged in
		var loggedInUser string
		sess := session.Get(req)
		if sess == nil {
			loggedInUser = "-"
		} else {
			loggedInUser = fmt.Sprintf("%s", sess.CAttr("UserName"))
		}

		// Write request details to the request log
		fmt.Fprintf(reqLog, "%v - %s [%s] \"%s %s %s\" \"-\" \"-\" \"%s\" \"%s\"\n", req.RemoteAddr,
			loggedInUser, time.Now().Format(time.RFC3339Nano), req.Method, req.URL, req.Proto,
			req.Referer(), req.Header.Get("User-Agent"))

		// Call the original function
		fn(w, req)
	}
}

func main() {
	// Load validation code
	validate = valid.New()

	// Read server configuration
	var err error
	if err = readConfig(); err != nil {
		log.Fatalf("Configuration file problem\n\n%v", err)
	}

	// Open the request log for writing
	reqLog, err = os.OpenFile(conf.Web.RequestLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY|os.O_SYNC, 0750)
	if err != nil {
		log.Fatalf("Error when opening request log: %s\n", err)
	}
	defer reqLog.Close()
	log.Printf("Request log opened: %s\n", conf.Web.RequestLog)

	// Setup session storage
	session.Global.Close()
	session.Global = session.NewCookieManagerOptions(session.NewInMemStore(),
		&session.CookieMngrOptions{AllowHTTP: false})

	// Parse our template files
	tmpl = template.Must(template.New("templates").Delims("[[", "]]").ParseGlob("templates/*.html"))

	// Connect to Minio server
	minioClient, err = minio.New(conf.Minio.Server, conf.Minio.AccessKey, conf.Minio.Secret, conf.Minio.HTTPS)
	if err != nil {
		log.Fatalf("Problem with Minio server configuration: \n\n%v", err)
	}

	// Log Minio server end point
	log.Printf("Minio server config ok. Address: %v\n", conf.Minio.Server)

	// Connect to PostgreSQL server
	db, err = pgx.Connect(*pgConfig)
	defer db.Close()
	if err != nil {
		log.Fatalf("Couldn't connect to database\n\n%v", err)
	}

	// Log successful connection message
	log.Printf("Connected to PostgreSQL server: %v:%v\n", conf.Pg.Server, uint16(conf.Pg.Port))

	// Our pages
	http.HandleFunc("/", logReq(mainHandler))
	http.HandleFunc("/download/", logReq(downloadHandler))
	http.HandleFunc("/downloadcsv/", logReq(downloadCSVHandler))
	http.HandleFunc("/login", logReq(loginHandler))
	http.HandleFunc("/logout", logReq(logoutHandler))
	http.HandleFunc("/register", logReq(registerHandler))
	http.HandleFunc("/settings", logReq(settingsHandler))
	http.HandleFunc("/star/", logReq(starHandler))
	http.HandleFunc("/table/", logReq(tableViewHandler))
	http.HandleFunc("/upload/", logReq(uploadFormHandler))
	http.HandleFunc("/uploaddata/", logReq(uploadDataHandler))

	// Static files
	http.HandleFunc("/images/sqlitebrowser.svg", logReq(func(w http.ResponseWriter, req *http.Request) {
		http.ServeFile(w, req, "images/sqlitebrowser.svg")
	}))
	http.HandleFunc("/favicon.ico", logReq(func(w http.ResponseWriter, req *http.Request) {
		http.ServeFile(w, req, "favicon.ico")
	}))
	http.HandleFunc("/robots.txt", logReq(func(w http.ResponseWriter, req *http.Request) {
		http.ServeFile(w, req, "robots.txt")
	}))

	// Start server
	log.Printf("DBHub server starting on https://%s\n", conf.Web.Server)
	log.Fatal(http.ListenAndServeTLS(conf.Web.Server, conf.Web.Certificate, conf.Web.CertificateKey, nil))
}

func mainHandler(w http.ResponseWriter, req *http.Request) {
	pageName := "Main handler"

	// Split the request URL into path components
	pathStrings := strings.Split(req.URL.Path, "/")

	// numPieces will be 2 if the request was for the root directory (https://server/), or if
	// the request included only a single path component (https://server/someuser/)
	numPieces := len(pathStrings)
	if numPieces == 2 {
		userName := pathStrings[1]
		// Check if the request was for the root directory
		if pathStrings[1] == "" {
			// Yep, root directory request
			frontPage(w, req)
			return
		}

		// The request was for a user page
		userPage(w, req, userName)
		return
	}

	userName := pathStrings[1]
	dbName := pathStrings[2]

	// Validate the user supplied user and database name
	err := validateUserDB(userName, dbName)
	if err != nil {
		log.Printf("Validation failed of user or database name: %s", err)
		errorPage(w, req, http.StatusBadRequest, "Invalid user or database name")
		return
	}

	// This catches the case where a "/" is on the end of a user page URL
	// TODO: Refactor this and the above identical code.  Doing it this way is non-optimal
	if pathStrings[2] == "" {
		// The request was for a user page
		userPage(w, req, userName)
		return
	}

	// * A specific database was requested *

	// Check if a table name was also requested
	err = req.ParseForm()
	if err != nil {
		log.Printf("%s: Error with ParseForm() in main handler: %s\n", pageName, err)
	}
	dbTable := req.FormValue("table")

	// If a table name was supplied, validate it
	if dbTable != "" {
		err = validatePGTable(dbTable)
		if err != nil {
			// Validation failed, so don't pass on the table name
			log.Printf("%s: Validation failed for table name: %s", pageName, err)
			dbTable = ""
		}
	}

	// TODO: Add support for folders and sub-folders in request paths
	databasePage(w, req, userName, dbName, dbTable)
}

// Read the server configuration file
func readConfig() error {
	// Reads the server configuration from disk
	// TODO: Might be a good idea to add permission checks of the dir & conf file, to ensure they're not
	// TODO: world readable
	userHome, err := homedir.Dir()
	if err != nil {
		return fmt.Errorf("User home directory couldn't be determined: %s", "\n")
	}
	configFile := filepath.Join(userHome, ".dbhub", "config.toml")
	if _, err := toml.DecodeFile(configFile, &conf); err != nil {
		return fmt.Errorf("Config file couldn't be parsed: %v\n", err)
	}

	// Override config file via environment variables
	tempString := os.Getenv("MINIO_SERVER")
	if tempString != "" {
		conf.Minio.Server = tempString
	}
	tempString = os.Getenv("MINIO_ACCESS_KEY")
	if tempString != "" {
		conf.Minio.AccessKey = tempString
	}
	tempString = os.Getenv("MINIO_SECRET")
	if tempString != "" {
		conf.Minio.Secret = tempString
	}
	tempString = os.Getenv("MINIO_HTTPS")
	if tempString != "" {
		conf.Minio.HTTPS, err = strconv.ParseBool(tempString)
		if err != nil {
			return fmt.Errorf("Failed to parse MINIO_HTTPS: %v\n", err)
		}
	}
	tempString = os.Getenv("PG_SERVER")
	if tempString != "" {
		conf.Pg.Server = tempString
	}
	tempString = os.Getenv("PG_PORT")
	if tempString != "" {
		tempInt, err := strconv.ParseInt(tempString, 10, 0)
		if err != nil {
			return fmt.Errorf("Failed to parse PG_PORT: %v\n", err)
		}
		conf.Pg.Port = int(tempInt)
	}
	tempString = os.Getenv("PG_USER")
	if tempString != "" {
		conf.Pg.Username = tempString
	}
	tempString = os.Getenv("PG_PASS")
	if tempString != "" {
		conf.Pg.Password = tempString
	}
	tempString = os.Getenv("PG_DBNAME")
	if tempString != "" {
		conf.Pg.Database = tempString
	}

	// Verify we have the needed configuration information
	// Note - We don't check for a valid conf.Pg.Password here, as the PostgreSQL password can also be kept
	// in a .pgpass file as per https://www.postgresql.org/docs/current/static/libpq-pgpass.html
	var missingConfig []string
	if conf.Minio.Server == "" {
		missingConfig = append(missingConfig, "Minio server:port string")
	}
	if conf.Minio.AccessKey == "" {
		missingConfig = append(missingConfig, "Minio access key string")
	}
	if conf.Minio.Secret == "" {
		missingConfig = append(missingConfig, "Minio secret string")
	}
	if conf.Pg.Server == "" {
		missingConfig = append(missingConfig, "PostgreSQL server string")
	}
	if conf.Pg.Port == 0 {
		missingConfig = append(missingConfig, "PostgreSQL port number")
	}
	if conf.Pg.Username == "" {
		missingConfig = append(missingConfig, "PostgreSQL username string")
	}
	if conf.Pg.Password == "" {
		missingConfig = append(missingConfig, "PostgreSQL password string")
	}
	if conf.Pg.Database == "" {
		missingConfig = append(missingConfig, "PostgreSQL database string")
	}
	if len(missingConfig) > 0 {
		// Some config is missing
		returnMessage := fmt.Sprint("Missing or incomplete value(s):\n")
		for _, value := range missingConfig {
			returnMessage += fmt.Sprintf("\n \t→ %v", value)
		}
		return fmt.Errorf(returnMessage)
	}

	// Set the PostgreSQL configuration values
	pgConfig.Host = conf.Pg.Server
	pgConfig.Port = uint16(conf.Pg.Port)
	pgConfig.User = conf.Pg.Username
	pgConfig.Password = conf.Pg.Password
	pgConfig.Database = conf.Pg.Database
	pgConfig.TLSConfig = nil

	// The configuration file seems good
	return nil
}

func registerHandler(w http.ResponseWriter, req *http.Request) {
	pageName := "Registration page"

	// TODO: Add browser side validation of the form data too (using AngularJS?) to save a trip to the server
	// TODO  and make for a nicer user experience for sign up

	// Gather submitted form data (if any)
	err := req.ParseForm()
	if err != nil {
		log.Printf("Error when parsing registration data: %s\n", err)
		errorPage(w, req, http.StatusBadRequest, "Error when parsing registration data")
		return
	}
	userName := req.PostFormValue("username")
	password := req.PostFormValue("pass")
	passConfirm := req.PostFormValue("pconfirm")
	email := req.PostFormValue("email")
	agree := req.PostFormValue("agree")

	// Check if any (relevant) form data was submitted
	if userName == "" && password == "" && passConfirm == "" && email == "" && agree == "" {
		// No, so render the registration page
		registerPage(w, req)
		return
	}

	// Validate the user supplied username and email address
	err = validateUserEmail(userName, email)
	if err != nil {
		log.Printf("Validation failed of username or email: %s", err)
		errorPage(w, req, http.StatusBadRequest, "Invalid username or email")
		return
	}

	// Check the password and confirmation match
	if len(password) != len(passConfirm) || password != passConfirm {
		log.Println("Password and confirmation do not match")
		errorPage(w, req, http.StatusBadRequest, "Password and confirmation do not match")
		return
	}

	// Check the password isn't blank
	if len(password) < 6 {
		log.Println("Password must be 6 characters or greater")
		errorPage(w, req, http.StatusBadRequest, "Password must be 6 characters or greater")
		return
	}

	// Check the Terms and Conditions was agreed to
	if agree != "on" {
		log.Println("Terms and Conditions wasn't agreed to")
		errorPage(w, req, http.StatusBadRequest, "Terms and Conditions weren't agreed to")
		return
	}

	// Ensure the username isn't a reserved one
	err = reservedUsernamesCheck(userName)
	if err != nil {
		log.Println(err)
		errorPage(w, req, http.StatusBadRequest, err.Error())
		return
	}

	log.Printf("Username: '%s' Password: '%s' Password confirm: '%s' Email: '%s' Agree: '%v'", userName, password,
		passConfirm, email, agree)

	// Check if the username is already in our system
	rows, err := db.Query("SELECT count(username) FROM public.users WHERE username = $1", userName)
	if err != nil {
		log.Printf("%s: Database query failed: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
		return
	}
	defer rows.Close()
	var userCount int
	for rows.Next() {
		err = rows.Scan(&userCount)
		if err != nil {
			log.Printf("%s: Error checking if user '%s' already exists: %v\n", pageName, userName, err)
			errorPage(w, req, http.StatusInternalServerError, "Database query failed")
			return
		}
	}
	if userCount > 0 {
		log.Println("That username is already taken")
		errorPage(w, req, http.StatusConflict, "That username is already taken")
		return
	}

	// Check if the email address is already in our system
	rows, err = db.Query("SELECT count(username) FROM public.users WHERE email = $1", email)
	if err != nil {
		log.Printf("%s: Database query failed: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
		return
	}
	defer rows.Close()
	var emailCount int
	for rows.Next() {
		err = rows.Scan(&emailCount)
		if err != nil {
			log.Printf("%s: Error checking if email '%s' already exists: %v\n", pageName, email, err)
			errorPage(w, req, http.StatusInternalServerError, "Database query failed")
			return
		}
	}
	if emailCount > 0 {
		log.Println("That email address is already associated with an account in our system")
		errorPage(w, req, http.StatusConflict,
			"That email address is already associated with an account in our system")
		return
	}

	// Hash the user's password
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("%s: Failed to hash user password. User: '%v', error: %v.\n", pageName, userName, err)
		errorPage(w, req, http.StatusInternalServerError, "Something went wrong during user creation")
		return
	}

	// TODO: Create the users certificate

	// Add the new user to the database
	insertQuery := "INSERT INTO public.users (username, email, password_hash, client_certificate) " +
		"VALUES ($1, $2, $3, $4)"
	commandTag, err := db.Exec(insertQuery, userName, email, hash, "") // TODO: Real certificate string should go here
	if err != nil {
		log.Printf("%s: Adding user to database failed: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Something went wrong during user creation")
		return
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("%s: Wrong number of rows affected: %v, username: %v\n", pageName, numRows, userName)
		return
	}

	// Create a new bucket for the user in Minio
	err = minioClient.MakeBucket(userName, "us-east-1")
	if err != nil {
		log.Printf("%s: Error creating new bucket: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Something went wrong during user creation")
		return
	}

	// TODO: Send a confirmation email, with verification link

	// TODO: Display a proper success page
	// TODO: This should probably bounce the user to their logged in profile page
	w.WriteHeader(http.StatusCreated)
	fmt.Fprint(w, `<html><body>Account created successfully, please login: <a href="/login">Login</a></body></html>`)
}

// This handles incoming requests for the settings page by logged in users
func settingsHandler(w http.ResponseWriter, req *http.Request) {
	//pageName := "Settings handler"

	// Ensure user is logged in
	var loggedInUser interface{}
	sess := session.Get(req)
	if sess != nil {
		loggedInUser = sess.CAttr("UserName")
	} else {
		errorPage(w, req, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Render the settings page
	settingsPage(w, req, fmt.Sprintf("%s", loggedInUser))
}

func starHandler(w http.ResponseWriter, req *http.Request) {
	pageName := "Star toggle Handler"

	// Split the request URL into path components
	pathStrings := strings.Split(req.URL.Path, "/")

	// Basic sanity check
	numPieces := len(pathStrings)
	if numPieces != 4 {
		return
	}

	// Extract the username, database, and version requested
	userName := pathStrings[2]
	dbName := pathStrings[3]

	// Validate the user supplied user and database name
	err := validateUserDB(userName, dbName)
	if err != nil {
		log.Printf("%s: Validation failed for user or database name: %s", pageName, err)
		return
	}

	// Retrieve session data (if any)
	var loggedInUser interface{}
	sess := session.Get(req)
	if sess != nil {
		loggedInUser = sess.CAttr("UserName")
	} else {
		// No logged in username, so nothing to update
		fmt.Fprint(w, "-1") // -1 tells the front end not to update the displayed star count
		return
	}

	// Retrieve the database id
	row := db.QueryRow(`SELECT idnum FROM sqlite_databases WHERE username = $1 AND dbname = $2`, userName, dbName)
	var dbId int
	err = row.Scan(&dbId)
	if err != nil {
		log.Printf("%s: Error looking up database id. User: '%s' Error: %v\n", pageName, loggedInUser, err)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Check if this user has already starred this username/database
	row = db.QueryRow(`
		SELECT count(db)
		FROM database_stars
		WHERE database_stars.db = $1
			AND database_stars.username = $2`, dbId, loggedInUser)
	var starCount int
	err = row.Scan(&starCount)
	if err != nil {
		log.Printf("%s: Error looking up star count for database. User: '%s' Error: %v\n", pageName,
			loggedInUser, err)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
		return

	}

	// Add or remove the star
	if starCount != 0 {
		// Unstar the database
		deleteQuery := `DELETE FROM database_stars WHERE db = $1 AND username = $2`
		commandTag, err := db.Exec(deleteQuery, dbId, loggedInUser)
		if err != nil {
			log.Printf("%s: Removing star from database failed: %v\n", pageName, err)
			errorPage(w, req, http.StatusInternalServerError, "Database query failed")
			return
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf("%s: Wrong number of rows affected: %v, username: %v\n", pageName, numRows, userName)
			return
		}

	} else {
		// Add a star for the database
		insertQuery := `INSERT INTO database_stars (db, username) VALUES ($1, $2)`
		commandTag, err := db.Exec(insertQuery, dbId, loggedInUser)
		if err != nil {
			log.Printf("%s: Adding star to database failed: %v\n", pageName, err)
			errorPage(w, req, http.StatusInternalServerError, "Database query failed")
			return
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf("%s: Wrong number of rows affected: %v, username: %v\n", pageName, numRows, userName)
			return
		}
	}

	// Refresh the main database table with the updated star count
	updateQuery := `
		UPDATE sqlite_databases
		SET stars = (
			SELECT count(db)
			FROM database_stars
			WHERE db = $1
		) WHERE idnum = $1`
	commandTag, err := db.Exec(updateQuery, dbId)
	if err != nil {
		log.Printf("%s: Updating star count in database failed: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
		return
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("%s: Wrong number of rows affected: %v, username: %v\n", pageName, numRows, userName)
		return
	}

	// Return the updated star count to the user
	row = db.QueryRow(`
		SELECT stars
		FROM sqlite_databases
		WHERE idnum = $1`, dbId)
	var newStarCount int
	err = row.Scan(&newStarCount)
	if err != nil {
		log.Printf("%s: Error looking up new star count for database. User: '%s' Error: %v\n", pageName,
			loggedInUser, err)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
		return
	}
	fmt.Fprint(w, newStarCount)
}

func tableViewHandler(w http.ResponseWriter, req *http.Request) {
	pageName := "databaseViewHandler()"

	// Split the request URL into path components
	pathStrings := strings.Split(req.URL.Path, "/")

	// Check that at least a username/database combination was requested
	if len(pathStrings) < 3 {
		log.Printf("Something wrong with the requested URL: %v\n", req.URL.Path)
		return
	}
	userName := pathStrings[2]
	dbName := pathStrings[3]

	// If a specific table was requested, get that info too
	var requestedTable string
	requestedTable = req.URL.RawQuery

	// TODO: Add support for database versions, instead of always using the latest

	// Validate the user supplied user and database name
	err := validateUserDB(userName, dbName)
	if err != nil {
		log.Printf("%s: Validation failed for user or database name: %s", pageName, err)
		return
	}

	// If a table name was supplied, validate it
	if requestedTable != "" {
		err = validatePGTable(requestedTable)
		if err != nil {
			log.Printf("%s: Validation failed for table name: %s", pageName, err)
			return
		}
	}

	// Retrieve session data (if any)
	var loggedInUser interface{}
	sess := session.Get(req)
	if sess != nil {
		loggedInUser = sess.CAttr("UserName")
	}

	// TODO: Implement caching

	// Check if the user has access to the requested database
	var minioId string
	if loggedInUser != userName {
		// * The request is for another users database, so it needs to be a public one *

		// Retrieve the MinioID of a public database with the given username/database combination
		row := db.QueryRow(`
			WITH requested_db AS (
				SELECT idnum
				FROM sqlite_databases
				WHERE username = $1
					AND dbname = $2
			)
			SELECT ver.minioid
			FROM database_versions AS ver, requested_db AS db
			WHERE ver.db = db.idnum
				AND ver.public = true
			ORDER BY version DESC
			LIMIT 1`, userName, dbName)
		err = row.Scan(&minioId)
		if err != nil {
			log.Printf("%s: Error looking up MinioID. User: '%s' Database: %v Error: %v\n", pageName,
				userName, dbName, err)
			return
		}
	} else {
		// Retrieve the MinioID of a database with the given username/database combination
		row := db.QueryRow(`
			WITH requested_db AS (
				SELECT idnum
				FROM sqlite_databases
				WHERE username = $1
					AND dbname = $2
			)
			SELECT ver.minioid
			FROM database_versions AS ver, requested_db AS db
			WHERE ver.db = db.idnum
			ORDER BY version DESC
			LIMIT 1`, userName, dbName)
		err = row.Scan(&minioId)
		if err != nil {
			log.Printf("%s: Error looking up database id. User: '%s' Error: %v\n", pageName, loggedInUser,
				err)
			return
		}
	}

	// Sanity check
	if minioId == "" {
		// The requested database wasn't found
		log.Printf("%s: Requested database not found. Username: '%s' Database: '%s'", pageName, userName,
			dbName)
		return
	}

	// Get a handle from Minio for the database object
	userDB, err := minioClient.GetObject(userName, minioId)
	if err != nil {
		log.Printf("%s: Error retrieving DB from Minio: %v\n", pageName, err)
		return
	}

	// Close the object handle when this function finishes
	defer func() {
		err := userDB.Close()
		if err != nil {
			log.Printf("%s: Error closing object handle: %v\n", pageName, err)
		}
	}()

	// Save the database locally to a temporary file
	tempfileHandle, err := ioutil.TempFile("", "databaseViewHandler-")
	if err != nil {
		log.Printf("%s: Error creating tempfile: %v\n", pageName, err)
		return
	}
	tempfile := tempfileHandle.Name()
	bytesWritten, err := io.Copy(tempfileHandle, userDB)
	if err != nil {
		log.Printf("%s: Error writing database to temporary file: %v\n", pageName, err)
		return
	}
	if bytesWritten == 0 {
		log.Printf("%s: 0 bytes written to the temporary file: %v\n", pageName, dbName)
		return
	}
	tempfileHandle.Close()
	defer os.Remove(tempfile) // Delete the temporary file when this function finishes

	// Open database
	db, err := sqlite.Open(tempfile, sqlite.OpenReadOnly)
	if err != nil {
		log.Printf("Couldn't open database: %s", err)
		return
	}
	defer db.Close()

	// Retrieve the list of tables in the database
	tables, err := db.Tables("")
	if err != nil {
		log.Printf("Error retrieving table names: %s", err)
		return
	}
	if len(tables) == 0 {
		// No table names were returned, so abort
		log.Printf("The database '%s' doesn't seem to have any tables. Aborting.", dbName)
		return
	}

	// If no specific table was requested, use the first one
	var dataRows dbInfo
	if requestedTable == "" {
		requestedTable = tables[0]
	}
	dataRows.Tablename = requestedTable

	// Retrieve (up to) x rows from the selected database
	// Ugh, have to use string smashing for this, even though the SQL spec doesn't seem to say table names
	// shouldn't be parameterised.  Limitation from SQLite's implementation? :(
	stmt, err := db.Prepare("SELECT * FROM " + requestedTable + " LIMIT 10")
	if err != nil {
		log.Printf("Error when preparing statement for database: %s\v", err)
		return
	}

	// Retrieve the field names
	dataRows.TableHeaders = stmt.ColumnNames()

	// Process each row
	var rowCount int
	fieldCount := -1
	err = stmt.Select(func(s *sqlite.Stmt) error {

		// Get the number of fields in the result
		if fieldCount == -1 {
			fieldCount = stmt.DataCount()
		}

		// Retrieve the data for each row
		var row []dataValue
		for i := 0; i < fieldCount; i++ {
			// Retrieve the data type for the field
			fieldType := stmt.ColumnType(i)

			isNull := false
			switch fieldType {
			case sqlite.Integer:
				var val int
				val, isNull, err = s.ScanInt(i)
				if err != nil {
					log.Printf("Something went wrong with ScanInt(): %v\n", err)
					break
				}
				if !isNull {
					stringVal := fmt.Sprintf("%d", val)
					row = append(row, dataValue{Name: dataRows.TableHeaders[i], Type: Integer,
						Value: stringVal})
				}
			case sqlite.Float:
				var val float64
				val, isNull, err = s.ScanDouble(i)
				if err != nil {
					log.Printf("Something went wrong with ScanDouble(): %v\n", err)
					break
				}
				if !isNull {
					stringVal := strconv.FormatFloat(val, 'f', 4, 64)
					row = append(row, dataValue{Name: dataRows.TableHeaders[i], Type: Float,
						Value: stringVal})
				}
			case sqlite.Text:
				var val string
				val, isNull = s.ScanText(i)
				if !isNull {
					row = append(row, dataValue{Name: dataRows.TableHeaders[i], Type: Text,
						Value: val})
				}
			case sqlite.Blob:
				_, isNull = s.ScanBlob(i)
				if !isNull {
					row = append(row, dataValue{Name: dataRows.TableHeaders[i], Type: Binary,
						Value: "<i>BINARY DATA</i>"})
				}
			case sqlite.Null:
				isNull = true
			}
			if isNull {
				row = append(row, dataValue{Name: dataRows.TableHeaders[i], Type: Null,
					Value: "<i>NULL</i>"})
			}
		}
		dataRows.Records = append(dataRows.Records, row)
		rowCount += 1

		return nil
	})
	if err != nil {
		log.Printf("Error when retrieving select data from database: %s\v", err)
		return
	}
	defer stmt.Finalize()

	var jsonResponse []byte
	if rowCount > 0 {
		// Use json.MarshalIndent() for nicer looking output
		jsonResponse, err = json.MarshalIndent(dataRows, "", " ")
		if err != nil {
			log.Println(err)
			return
		}
	} else {
		// Return an empty set indicator, instead of "null"
		jsonResponse = []byte{'{', ']'}
	}

	// TODO: Cache the response

	// TODO: Send the response from cache
	//w.Header().Set("Access-Control-Allow-Origin", "*")
	fmt.Fprintf(w, "%s", jsonResponse)
}

// This function presents the database upload form to logged in users
func uploadFormHandler(w http.ResponseWriter, req *http.Request) {
	//pageName := "Upload handler"

	// Ensure user is logged in
	var loggedInUser interface{}
	sess := session.Get(req)
	if sess != nil {
		loggedInUser = sess.CAttr("UserName")
	} else {
		errorPage(w, req, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// TODO: If uploaded file + form data is present, process, it, otherwise
	// TODO  render the upload page

	// Render the upload page
	uploadPage(w, req, fmt.Sprintf("%s", loggedInUser))
}

// This function processes new database data submitted through the upload form
func uploadDataHandler(w http.ResponseWriter, req *http.Request) {
	pageName := "Upload DB handler"

	// Ensure user is logged in
	var loggedInUser string
	sess := session.Get(req)
	if sess == nil {
		errorPage(w, req, http.StatusUnauthorized, "You need to be logged in")
		return
	}
	loggedInUser = fmt.Sprintf("%s", sess.CAttr("UserName"))

	// Prepare the form data
	req.ParseMultipartForm(32 << 20) // 64MB of ram max
	if err := req.ParseForm(); err != nil {
		fmt.Errorf("%s: ParseForm() error: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, err.Error())
		return
	}

	// Grab and validate the supplied "public" form field
	userPublic := req.PostFormValue("public")
	public, err := strconv.ParseBool(userPublic)
	if err != nil {
		log.Printf("%s: Error when converting public value to boolean: %v\n", pageName, err)
		errorPage(w, req, http.StatusBadRequest, "Public value incorrect")
		return
	}

	// TODO: Add support for folders and subfolders
	folder := "/"

	tempFile, handler, err := req.FormFile("database")
	if err != nil {
		log.Printf("%s: Uploading file failed: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Database file missing from upload data?")
		return
	}
	dbName := handler.Filename
	defer tempFile.Close()

	// Validate the database name
	err = validateDB(dbName)
	if err != nil {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		errorPage(w, req, http.StatusBadRequest, "Invalid database name")
		return
	}

	// Write the temporary file locally, so we can try opening it with SQLite to verify it's ok
	var tempBuf bytes.Buffer
	bytesWritten, err := io.Copy(&tempBuf, tempFile)
	if err != nil {
		log.Printf("%s: Error: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Internal error")
		return
	}
	if bytesWritten == 0 {
		log.Printf("%s: Database seems to be 0 bytes in length. Username: %s, Database: %s\n", pageName,
			loggedInUser, dbName)
		errorPage(w, req, http.StatusBadRequest, "Database file is 0 length?")
		return
	}
	tempDB, err := ioutil.TempFile("", "dbhub-upload-")
	if err != nil {
		log.Printf("%s: Error creating temporary file. User: %s, Database: %s, Filename: %s, Error: %v\n",
			pageName, loggedInUser, dbName, tempDB.Name(), err)
		errorPage(w, req, http.StatusInternalServerError, "Internal error")
		return
	}
	_, err = tempDB.Write(tempBuf.Bytes())
	if err != nil {
		log.Printf("%s: Error when writing the uploaded db to a temp file. User: %s, Database: %s"+
			"Error: %v\n", pageName, loggedInUser, dbName, err)
		errorPage(w, req, http.StatusInternalServerError, "Internal error")
		return
	}
	tempDBName := tempDB.Name()

	// Delete the temporary file when this function finishes
	defer os.Remove(tempDBName)

	// Perform a read on the database, as a basic sanity check to ensure it's really a SQLite database
	sqliteDB, err := sqlite.Open(tempDBName, sqlite.OpenReadOnly)
	if err != nil {
		log.Printf("Couldn't open database when sanity checking upload: %s", err)
		errorPage(w, req, http.StatusInternalServerError, "Internal error")
		return
	}
	defer sqliteDB.Close()
	tables, err := sqliteDB.Tables("")
	if err != nil {
		log.Printf("Error retrieving table names when sanity checking upload: %s", err)
		errorPage(w, req, http.StatusInternalServerError,
			"Error when sanity checking file.  Possibly encrypted or not a database?")
		return
	}
	if len(tables) == 0 {
		// No table names were returned, so abort
		log.Printf("The attemped upload for '%s' failed, as it doesn't seem to have any tables.", dbName)
		errorPage(w, req, http.StatusInternalServerError, "Database has no tables?")
		return
	}

	// Generate sha256 of the uploaded file
	shaSum := sha256.Sum256(tempBuf.Bytes())

	// Check if the database already exists
	var highestVersion int
	err = db.QueryRow(`
		SELECT version
		FROM database_versions
		WHERE db = (SELECT idnum
			FROM sqlite_databases
			WHERE username = $1
			AND dbname = $2)
		ORDER BY version DESC
		LIMIT 1`, loggedInUser, dbName).Scan(&highestVersion)
	if err != nil && err != pgx.ErrNoRows {
		log.Printf("%s: Error when querying database: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Database query failure")
		return
	}
	var newVersion int
	if highestVersion > 0 {
		// The database already exists
		newVersion = highestVersion + 1
	} else {
		newVersion = 1
	}

	// Generate random filename to store the database as
	mathrand.Seed(time.Now().UnixNano())
	const alphaNum = "abcdefghijklmnopqrstuvwxyz0123456789"
	randomString := make([]byte, 8)
	for i := range randomString {
		randomString[i] = alphaNum[mathrand.Intn(len(alphaNum))]
	}
	minioId := string(randomString) + ".db"

	// TODO: We should probably check if the randomly generated filename is already used for the user, just in case

	// Store the database file in Minio
	dbSize, err := minioClient.PutObject(loggedInUser, minioId, &tempBuf, handler.Header["Content-Type"][0])
	if err != nil {
		log.Printf("%s: Storing file in Minio failed: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Storing in object store failed")
		return
	}

	// TODO: Put these queries inside a single transaction

	// Add the new database details to the PG database
	var dbQuery string
	if newVersion == 1 {
		dbQuery = `
			INSERT INTO sqlite_databases (username, folder, dbname)
			VALUES ($1, $2, $3)`
		commandTag, err := db.Exec(dbQuery, loggedInUser, folder, dbName)
		if err != nil {
			log.Printf("%s: Adding database to PostgreSQL failed: %v\n", pageName, err)
			errorPage(w, req, http.StatusInternalServerError, "Database query failed")
			return
		}
		if numRows := commandTag.RowsAffected(); numRows != 1 {
			log.Printf("%s: Wrong number of rows affected: %v, user: %s, database: %v\n", pageName,
				numRows, loggedInUser, dbName)
			return
		}
	}

	// Add the database to database_versions
	dbQuery = `
		WITH databaseid AS (
			SELECT idnum
			FROM sqlite_databases
			WHERE username = $1
				AND dbname = $2)
		INSERT INTO database_versions (db, size, version, sha256, public, minioid)
		SELECT idnum, $3, $4, $5, $6, $7 FROM databaseid`
	commandTag, err := db.Exec(dbQuery, loggedInUser, dbName, dbSize, newVersion, hex.EncodeToString(shaSum[:]),
		public, minioId)
	if err != nil {
		log.Printf("%s: Adding version info to PostgreSQL failed: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Update the last_modified date for the database in sqlite_databases
	dbQuery = `
		UPDATE sqlite_databases
		SET last_modified = (
			SELECT last_modified
			FROM database_versions
			WHERE db = (
				SELECT idnum
				FROM sqlite_databases
				WHERE username = $1
					AND dbname = $2)
				AND version = $3)
		WHERE username = $1
			AND dbname = $2`
	commandTag, err = db.Exec(dbQuery, loggedInUser, dbName, newVersion)
	if err != nil {
		log.Printf("%s: Updating last_modified date in PostgreSQL failed: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
		return
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("%s: Wrong number of rows affected: %v, user: %s, database: %v\n", pageName, numRows,
			loggedInUser, dbName)
		return
	}

	// Log the successful database upload
	log.Printf("%s: Username: %v, database '%v' uploaded as '%v', bytes: %v\n", pageName, loggedInUser, dbName,
		minioId, dbSize)

	// Database upload succeeded.  Tell the user then bounce back to their profile page
	fmt.Fprintf(w, `
	<html><head><script type="text/javascript"><!--
		function delayer(){
			window.location = "/%s"
		}//-->
	</script></head>
	<body onLoad="setTimeout('delayer()', 5000)">
	<body>Upload succeeded<br /><br /><a href="/%s">Continuing to profile page...</a></body></html>`,
		loggedInUser, loggedInUser)
}
