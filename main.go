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
	log.Fatal(http.ListenAndServe(listenAddr+":"+strconv.Itoa(listenPort), nil))
}

func mainHandler(w http.ResponseWriter, req *http.Request) {
	pageName := "mainHandler()"

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
			// TODO: Create a template for the root directory
			fmt.Fprintf(w, "Root directory")
			return
		}

		// The request was for a user directory, so return that list
		// TODO: Create a template for the user page
		fmt.Fprintf(w, "Page for user: %s", userName)
		return
	}

	userName := pathStrings[1]
	databaseName := pathStrings[2]

	// This catches the case where a "/" is on the end of the URL
	// TODO: Refactor this and the above identical code.  Doing it this way is non-optimal
	if pathStrings[2] == "" {
		// The request was for a user directory, so return that list
		// TODO: Create a template for the user page
		fmt.Fprintf(w, "Page for user: %s", userName)
		return
	}

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
		}
		if !Readme.Valid {
			dataRows.Readme = "No readme"
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
	stmt, err := db.Prepare("SELECT * FROM " + selectedTable + " LIMIT 8")
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

			switch fieldType {
			case sqlite.Integer:
				val, isNull, err := s.ScanInt(i)
				if err != nil {
					log.Printf("Something went wrong with ScanInt(): %v\n", err)
					break
				}
				if !isNull {
					stringVal := fmt.Sprintf("%d", val)
					row = append(row, dataValue{Name: dataRows.TableHeaders[i], Type: Integer,
						Value: stringVal})
				} else {
					row = append(row, dataValue{Name: dataRows.TableHeaders[i], Type: Null,
						Value: "NULL"})
				}
			case sqlite.Float:
				val, isNull, err := s.ScanDouble(i)
				if err != nil {
					log.Printf("Something went wrong with ScanDouble(): %v\n", err)
					break
				}
				if !isNull {
					stringVal := strconv.FormatFloat(val, 'f', 4, 64)
					row = append(row, dataValue{Name: dataRows.TableHeaders[i], Type: Float,
						Value: stringVal})
				} else {
					row = append(row, dataValue{Name: dataRows.TableHeaders[i], Type: Null,
						Value: "NULL"})
				}
			case sqlite.Text:
				val, isNull := s.ScanText(i)
				if !isNull {
					row = append(row, dataValue{Name: dataRows.TableHeaders[i], Type: Text,
						Value: val})
				} else {
					row = append(row, dataValue{Name: dataRows.TableHeaders[i], Type: Null,
						Value: "NULL"})
				}
			case sqlite.Blob:
				val, isNull := s.ScanBlob(i)
				if !isNull {
					row = append(row, dataValue{Name: dataRows.TableHeaders[i], Type: Binary,
						Value: val})
				} else {
					row = append(row, dataValue{Name: dataRows.TableHeaders[i], Type: Null,
						Value: "NULL"})
				}
			case sqlite.Null:
				row = append(row, dataValue{Name: dataRows.TableHeaders[i], Type: Null,
					Value: "NULL"})
			}
		}
		dataRows.Records = append(dataRows.Records, row)

		return nil
	})
	if err != nil {
		log.Printf("Error when retrieving select data from database: %s\v", err)
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

	t.Execute(w, dataRows)
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
