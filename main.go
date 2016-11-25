package main

import (
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	sqlite "github.com/gwenn/gosqlite"
	"github.com/jackc/pgx"
	"github.com/minio/go-homedir"
	"github.com/minio/minio-go"
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
	TableHeaders []string
	Records      []dataRow
	Tables       []string
	Username     string
	Database     string
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
	MinioID      string
	Size         int
	Version      int
	Protocol     string
	Server       string
}

// Configuration file
type tomlConfig struct {
	Minio minioInfo
	Pg    pgInfo
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

var (
	// Our configuration info
	conf tomlConfig

	// PostgreSQL configuration info
	pgConfig = new(pgx.ConnConfig)

	// Connection handles
	db          *pgx.Conn
	minioClient *minio.Client

	// Address to listen on
	listenProtocol = "http"
	listenAddr     = "localhost"
	listenPort     = 8080
)

func downloadHandler(w http.ResponseWriter, req *http.Request) {
	pageName := "Download Handler"

	// Split the request URL into path components
	pathStrings := strings.Split(req.URL.Path, "/")

	// Basic sanity check
	numPieces := len(pathStrings)
	if numPieces < 4 {
		http.Error(w, "Invalid database requested", http.StatusBadRequest)
		return
	}

	// Extract the username, database, and version requested
	// TODO: Validate the user supplied data better, or at least verify that net/http does so itself sufficiently
	var dbVersion int64
	userName := pathStrings[2]
	dbName := pathStrings[3]
	queryValues := req.URL.Query()
	dbVersion, err := strconv.ParseInt(queryValues["version"][0], 10, 0)
	if err != nil {
		log.Printf("%s: Invalid version number: \n%v", pageName, err)
		http.Error(w, fmt.Sprintf("Invalid version number"), http.StatusBadRequest)
		return
	}

	// Verify the given database exists and is ok to be downloaded (and get the MinioID while at it)
	rows, err := db.Query("SELECT minioid FROM public.sqlite_databases "+
		"WHERE dbname = $1 "+
		"AND version = $2 "+
		"AND username = $3 " +
		"AND public = true", dbName, dbVersion, userName)
	if err != nil {
		log.Printf("%s: Database query failed: \n%v", pageName, err)
		http.Error(w, fmt.Sprintf("Database query failed"), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var minioID string
	for rows.Next() {
		err = rows.Scan(&minioID)
		if err != nil {
			log.Printf("%s: Error retrieving MinioID: %v\n", pageName, err)
			http.Error(w, fmt.Sprintf("Database query failed"), http.StatusInternalServerError)
			return
		}
	}
	if minioID == "" {
		log.Printf("%s: Couldn't retrieve required MinioID\n", pageName)
		http.Error(w, fmt.Sprintf("Database query failed"), http.StatusInternalServerError)
		return
	}

	// Get a handle from Minio for the database object
	userDB, err := minioClient.GetObject(userName, minioID)
	if err != nil {
		log.Printf("%s: Error retrieving DB from Minio: %v\n", pageName, err)
		http.Error(w, fmt.Sprintf("Database query failed"), http.StatusInternalServerError)
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
	log.Printf("%s: '%v' downloaded by user '%v', %v bytes", pageName, dbName, userName, bytesWritten)
}

func main() {
	// Read server configuration
	var err error
	if err = readConfig(); err != nil {
		log.Fatalf("Configuration file problem\n\n%v", err)
	}

	// Connect to Minio server
	minioClient, err = minio.New(conf.Minio.Server, conf.Minio.AccessKey, conf.Minio.Secret, conf.Minio.HTTPS)
	if err != nil {
		log.Fatalf("Problem with Minio server configuration: \n\n%v", err)
	}

	// Log Minio server end point
	log.Printf("Minio server config ok: %v\n", conf.Minio.Server)

	// Connect to PostgreSQL server
	db, err = pgx.Connect(*pgConfig)
	defer db.Close()
	if err != nil {
		log.Fatalf("Couldn't connect to database\n\n%v", err)
	}

	// Log successful connection message
	log.Printf("Connected to PostgreSQL server: %v:%v\n", conf.Pg.Server, uint16(conf.Pg.Port))

	log.Println("Running...")
	http.HandleFunc("/", mainHandler)
	http.HandleFunc("/download/", downloadHandler)
	log.Fatal(http.ListenAndServe(listenAddr+":"+strconv.Itoa(listenPort), nil))
}

func mainHandler(w http.ResponseWriter, req *http.Request) {
	//pageName := "mainHandler()"

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
			renderRootPage(w)
			return
		}

		// The request was for a user page
		renderUserPage(w, userName)
		return
	}

	userName := pathStrings[1]
	databaseName := pathStrings[2]

	// This catches the case where a "/" is on the end of a user page URL
	// TODO: Refactor this and the above identical code.  Doing it this way is non-optimal
	if pathStrings[2] == "" {
		// The request was for a user page
		renderUserPage(w, userName)
		return
	}

	// A specific database was requested
	// TODO: Add support for folders and sub-folders in request paths
	renderDatabasePage(w, userName, databaseName)
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

func renderDatabasePage(w http.ResponseWriter, userName string, databaseName string) {
	pageName := "Render Database Page"

	// Retrieve the MinioID, and the user visible info for the requested database
	rows, err := db.Query(
		"SELECT minioid, date_created, last_modified, size, version, public, watchers, stars, forks, "+
			"discussions, pull_requests, updates, branches, releases, contributors, description, readme "+
			"FROM public.sqlite_databases "+
			"WHERE username = $1 "+
			"AND dbname = $2 "+
			"ORDER BY version DESC "+
			"LIMIT 1",
		userName, databaseName)
	if err != nil {
		log.Printf("%s: Database query failed: \n%v", pageName, err)
		http.Error(w, "Database query failed", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var dataRows dbInfo
	for rows.Next() {
		var Desc pgx.NullString
		var Readme pgx.NullString
		err = rows.Scan(&dataRows.MinioID, &dataRows.DateCreated, &dataRows.LastModified, &dataRows.Size,
			&dataRows.Version, &dataRows.Public, &dataRows.Watchers, &dataRows.Stars, &dataRows.Forks,
			&dataRows.Discussions, &dataRows.PRs, &dataRows.Updates, &dataRows.Branches, &dataRows.Releases,
			&dataRows.Contributors, &Desc, &Readme)
		if err != nil {
			log.Printf("%s: Error retrieving MinioID from database: %v\n", pageName, err)
			http.Error(w, "Error retrieving MinioID from database", http.StatusInternalServerError)
			return
		}
		if !Desc.Valid {
			dataRows.Description = "No description"
		} else {
			dataRows.Description = Desc.String
		}
		if !Readme.Valid {
			dataRows.Readme = "No readme"
		} else {
			dataRows.Readme = Readme.String
		}
	}
	if dataRows.MinioID == "" {
		log.Printf("%s: Requested database not found: %v for user: %v \n", pageName, databaseName, userName)
		http.Error(w, "The requested database doesn't exist", http.StatusNotFound)
		return
	}

	// Get a handle from Minio for the database object
	userDB, err := minioClient.GetObject(userName, dataRows.MinioID)
	if err != nil {
		log.Printf("%s: Error retrieving DB from Minio: %v\n", pageName, err)
		http.Error(w, "Error retrieving DB from Minio", http.StatusInternalServerError)
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
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	tempfile := tempfileHandle.Name()
	bytesWritten, err := io.Copy(tempfileHandle, userDB)
	if err != nil {
		log.Printf("%s: Error writing database to temporary file: %v\n", pageName, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if bytesWritten == 0 {
		log.Printf("%s: 0 bytes written to the temporary file: %v\n", pageName, databaseName)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
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
		// TODO: Add proper error handing here.  Maybe display the page, but show the error where
		// TODO  the table data would otherwise be?
		http.Error(w, fmt.Sprintf("Error reading from '%s'.  Possibly encrypted or not a database?",
			databaseName), http.StatusInternalServerError)
		return
	}
	if len(tables) == 0 {
		// No table names were returned, so abort
		log.Printf("The database '%s' doesn't seem to have any tables. Aborting.", databaseName)
		return
	}
	dataRows.Tables = tables

	// Select the first table
	selectedTable := dataRows.Tables[0]

	// Retrieve (up to) x rows from the selected database
	// Ugh, have to use string smashing for this, even though the SQL spec doesn't seem to say table names
	// shouldn't be parameterised.  Limitation from SQLite's implementation? :(
	stmt, err := db.Prepare("SELECT * FROM " + selectedTable + " LIMIT 10")
	if err != nil {
		log.Printf("Error when preparing statement for database: %s\v", err)
		return
	}

	// Retrieve the field names
	dataRows.TableHeaders = stmt.ColumnNames()

	// Process each row
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

		return nil
	})
	if err != nil {
		log.Printf("Error when retrieving select data from database: %s\v", err)
		http.Error(w, fmt.Sprintf("Error reading data from '%s'.  Possibly malformed?", databaseName),
			http.StatusInternalServerError)
		return
	}
	defer stmt.Finalize()

	dataRows.Username = userName
	dataRows.Database = databaseName
	dataRows.Tablename = selectedTable
	dataRows.Protocol = listenProtocol
	dataRows.Server = listenAddr + ":9080"

	// Parse the template, but use "[[" and "]]" as delimiters.  This is because both Go and AngularJS use
	// "{{" "}}" by default, so one needs to be changed ;)
	t := template.New("database.html")
	t.Delims("[[", "]]")
	t, err = t.ParseFiles("templates/database.html")
	if err != nil {
		log.Printf("Error: %s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = t.Execute(w, dataRows)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func renderRootPage(w http.ResponseWriter) {
	pageName := "User Page"

	// Structure to hold user list
	type userInfo struct {
		Username string
		LastModified time.Time
	}
	var userList []userInfo

	// Retrieve list of users with public databases
	dbQuery := "WITH user_list AS ( " +
		"SELECT DISTINCT ON (username) username, last_modified " +
		"FROM public.sqlite_databases " +
		"WHERE public = true " +
		"ORDER BY username, last_modified DESC " +
		") SELECT username, last_modified " +
		"FROM user_list " +
		"ORDER BY last_modified DESC"
	rows, err := db.Query(dbQuery)
	if err != nil {
		log.Printf("%s: Database query failed: \n%v", pageName, err)
		http.Error(w, "Database query failed", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var oneRow userInfo
		err = rows.Scan(&oneRow.Username, &oneRow.LastModified)
		if err != nil {
			log.Printf("%s: Error retrieving database list for user: %v\n", pageName, err)
			http.Error(w, "Error retrieving database list for user", http.StatusInternalServerError)
			return
		}
		userList = append(userList, oneRow)
	}

	// Parse and execute the template
	t := template.New("root.html")
	t.Delims("[[", "]]")
	t, err = t.ParseFiles("templates/root.html")
	if err != nil {
		log.Printf("Error: %s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = t.Execute(w, userList)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func renderUserPage(w http.ResponseWriter, userName string) {
	pageName := "User Page"

	// Structure to hold user data
	var userData struct {
		Username string
		DataRows []dbInfo
	}
	userData.Username = userName

	// Retrieve list of public databases for the user
	dbQuery := "WITH user_public_databases AS (" +
		"SELECT DISTINCT ON (dbname) dbname, version " +
		"FROM public.sqlite_databases " +
		"WHERE username = $1 " +
		"AND public = TRUE " +
		"ORDER BY dbname, version DESC" +
		") " +
		"SELECT i.dbname, last_modified, size, i.version, watchers, stars, forks, " +
		"discussions, pull_requests, updates, branches, releases, contributors, description " +
		"FROM user_public_databases AS l, public.sqlite_databases AS i " +
		"WHERE username = $1 " +
		"AND l.dbname = i.dbname " +
		"AND l.version = i.version " +
		"AND public = TRUE " +
		"ORDER BY last_modified DESC"
	rows, err := db.Query(dbQuery, userName)
	if err != nil {
		log.Printf("%s: Database query failed: \n%v", pageName, err)
		http.Error(w, "Database query failed", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var oneRow dbInfo
		var Desc pgx.NullString
		err = rows.Scan(&oneRow.Database, &oneRow.LastModified, &oneRow.Size, &oneRow.Version,
			&oneRow.Watchers, &oneRow.Stars, &oneRow.Forks, &oneRow.Discussions, &oneRow.PRs,
			&oneRow.Updates, &oneRow.Branches, &oneRow.Releases, &oneRow.Contributors, &Desc)
		if err != nil {
			log.Printf("%s: Error retrieving database list for user: %v\n", pageName, err)
			http.Error(w, "Error retrieving database list for user", http.StatusInternalServerError)
			return
		}
		if !Desc.Valid {
			oneRow.Description = ""
		} else {
			oneRow.Description = fmt.Sprintf(": %s", Desc.String)
		}
		userData.DataRows = append(userData.DataRows, oneRow)
	}

	// Parse and execute the template
	t := template.New("user.html")
	t.Delims("[[", "]]")
	t, err = t.ParseFiles("templates/user.html")
	if err != nil {
		log.Printf("Error: %s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = t.Execute(w, userData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}
