package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	sqlite "github.com/gwenn/gosqlite"
	"github.com/icza/session"
	"github.com/jackc/pgx"
)

func databasePage(w http.ResponseWriter, req *http.Request, userName string, dbName string, dbTable string) {
	pageName := "Render Database Page"

	// Retrieve the MinioID, and the user visible info for the requested database
	rows, err := db.Query(
		`SELECT ver.minioid, db.date_created, db.last_modified, ver.size, ver.version, db.watchers, db.stars, db.forks,
	db.discussions, db.pull_requests, db.updates, db.branches, db.releases, db.contributors, db.description, db.readme
	FROM sqlite_databases AS db, database_versions AS ver
	WHERE db.username = $1 AND db.dbname = $2 AND db.idnum = ver.db
	ORDER BY version DESC
	LIMIT 1`,
		userName, dbName)
	if err != nil {
		log.Printf("%s: Database query failed: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
		return
	}
	defer rows.Close()

	var pageData struct {
		Meta metaInfo
		DB   dbInfo
	}

	// Retrieve session data (if any)
	sess := session.Get(req)
	if sess != nil {
		loggedInUser := sess.CAttr("UserName")
		pageData.Meta.LoggedInUser = fmt.Sprintf("%s", loggedInUser)
	}

	var minioID string
	for rows.Next() {
		var Desc pgx.NullString
		var Readme pgx.NullString
		err = rows.Scan(&minioID, &pageData.DB.DateCreated, &pageData.DB.LastModified, &pageData.DB.Size,
			&pageData.DB.Version, &pageData.DB.Watchers, &pageData.DB.Stars, &pageData.DB.Forks,
			&pageData.DB.Discussions, &pageData.DB.PRs, &pageData.DB.Updates, &pageData.DB.Branches,
			&pageData.DB.Releases, &pageData.DB.Contributors, &Desc, &Readme)
		if err != nil {
			log.Printf("%s: Error retrieving metadata from database: %v\n", pageName, err)
			errorPage(w, req, http.StatusInternalServerError, "Error retrieving metadata from database")
			return
		}
		if !Desc.Valid {
			pageData.DB.Description = "No description"
		} else {
			pageData.DB.Description = Desc.String
		}
		if !Readme.Valid {
			pageData.DB.Readme = "No readme"
		} else {
			pageData.DB.Readme = Readme.String
		}
	}
	if minioID == "" {
		log.Printf("%s: Requested database not found: %v for user: %v \n", pageName, dbName, userName)
		errorPage(w, req, http.StatusInternalServerError, "The requested database doesn't exist")
		return
	}

	// Get a handle from Minio for the database object
	userDB, err := minioClient.GetObject(userName, minioID)
	if err != nil {
		log.Printf("%s: Error retrieving DB from Minio: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Internal retrieving database from object store")
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

	// Retrieve the list of tables in the database
	tables, err := db.Tables("")
	if err != nil {
		log.Printf("Error retrieving table names: %s", err)
		// TODO: Add proper error handing here.  Maybe display the page, but show the error where
		// TODO  the table data would otherwise be?
		errorPage(w, req, http.StatusInternalServerError,
			fmt.Sprintf("Error reading from '%s'.  Possibly encrypted or not a database?", dbName))
		return
	}
	if len(tables) == 0 {
		// No table names were returned, so abort
		log.Printf("The database '%s' doesn't seem to have any tables. Aborting.", dbName)
		errorPage(w, req, http.StatusInternalServerError, "Database has no tables?")
		return
	}
	pageData.DB.Tables = tables

	// If a specific table was requested, check that it's present
	if dbTable != "" {
		// Check the requested table is present
		tablePresent := false
		for _, tbl := range tables {
			if tbl == dbTable {
				tablePresent = true
			}
		}
		if tablePresent == false {
			// The requested table doesn't exist in the database
			dbTable = ""
		}
	}
	// If a specific table wasn't requested, or the one requested isn't present, use the first table in the database
	if dbTable == "" {
		dbTable = pageData.DB.Tables[0]
	}

	// Retrieve (up to) x rows from the selected database
	// Ugh, have to use string smashing for this, even though the SQL spec doesn't seem to say table names
	// shouldn't be parameterised.  Limitation from SQLite's implementation? :(
	stmt, err := db.Prepare("SELECT * FROM " + dbTable + " LIMIT 10")
	if err != nil {
		log.Printf("Error when preparing statement for database: %s\v", err)
		errorPage(w, req, http.StatusInternalServerError, "Internal error")
		return
	}

	// Retrieve the field names
	pageData.DB.TableHeaders = stmt.ColumnNames()

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
					row = append(row, dataValue{Name: pageData.DB.TableHeaders[i], Type: Integer,
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
					row = append(row, dataValue{Name: pageData.DB.TableHeaders[i], Type: Float,
						Value: stringVal})
				}
			case sqlite.Text:
				var val string
				val, isNull = s.ScanText(i)
				if !isNull {
					row = append(row, dataValue{Name: pageData.DB.TableHeaders[i], Type: Text,
						Value: val})
				}
			case sqlite.Blob:
				_, isNull = s.ScanBlob(i)
				if !isNull {
					row = append(row, dataValue{Name: pageData.DB.TableHeaders[i], Type: Binary,
						Value: "<i>BINARY DATA</i>"})
				}
			case sqlite.Null:
				isNull = true
			}
			if isNull {
				row = append(row, dataValue{Name: pageData.DB.TableHeaders[i], Type: Null,
					Value: "<i>NULL</i>"})
			}
		}
		pageData.DB.Records = append(pageData.DB.Records, row)

		return nil
	})
	if err != nil {
		log.Printf("Error when retrieving select data from database: %s\v", err)
		errorPage(w, req, http.StatusInternalServerError,
			fmt.Sprintf("Error reading data from '%s'.  Possibly malformed?", dbName))
		return
	}
	defer stmt.Finalize()

	pageData.DB.Tablename = dbTable
	pageData.Meta.Username = userName
	pageData.Meta.Database = dbName
	pageData.Meta.Server = conf.Web.Server
	pageData.Meta.Title = fmt.Sprintf("%s / %s", userName, dbName)

	// Render the page
	t := tmpl.Lookup("databasePage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// General error display page
func errorPage(w http.ResponseWriter, req *http.Request, httpcode int, msg string) {
	var pageData struct {
		Meta    metaInfo
		Message string
	}
	pageData.Message = msg

	// Retrieve session data (if any)
	sess := session.Get(req)
	if sess != nil {
		loggedInUser := sess.CAttr("UserName")
		pageData.Meta.LoggedInUser = fmt.Sprintf("%s", loggedInUser)
	}

	// Render the page
	w.WriteHeader(httpcode)
	t := tmpl.Lookup("errorPage")
	err := t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Renders the front page of the website
func frontPage(w http.ResponseWriter, req *http.Request) {
	pageName := "User Page"

	// Structure to hold page data
	type userInfo struct {
		Username     string
		LastModified time.Time
	}
	var pageData struct {
		Meta metaInfo
		List []userInfo
	}

	// Retrieve session data (if any)
	sess := session.Get(req)
	if sess != nil {
		loggedInUser := sess.CAttr("UserName")
		pageData.Meta.LoggedInUser = fmt.Sprintf("%s", loggedInUser)
	}

	// Retrieve list of users with public databases
	dbQuery := `
		WITH public_dbs AS (
			SELECT DISTINCT ON (ver.db) ver.db, ver.version, ver.last_modified
			FROM database_versions AS ver
			WHERE ver.public = true
			ORDER BY ver.db DESC, ver.version DESC
		), public_users AS (
			SELECT DISTINCT ON (db.username) db.username, pub.db, pub.version, pub.last_modified
			FROM public_dbs as pub, sqlite_databases AS db
			WHERE db.idnum = pub.db
			ORDER BY db.username, last_modified DESC
		)
		SELECT username, last_modified FROM public_users
		ORDER BY last_modified DESC`
	rows, err := db.Query(dbQuery)
	if err != nil {
		log.Printf("%s: Database query failed: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var oneRow userInfo
		err = rows.Scan(&oneRow.Username, &oneRow.LastModified)
		if err != nil {
			log.Printf("%s: Error retrieving database list for user: %v\n", pageName, err)
			errorPage(w, req, http.StatusInternalServerError, "Error retrieving database list for user")
			return
		}
		pageData.List = append(pageData.List, oneRow)
	}
	pageData.Meta.Title = `SQLite storage "in the cloud"`

	// Render the page
	t := tmpl.Lookup("rootPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func loginPage(w http.ResponseWriter, req *http.Request) {
	var pageData struct {
		Meta      metaInfo
		SourceRef string
	}
	pageData.Meta.Title = "Login"

	// Retrieve session data (if any)
	sess := session.Get(req)
	if sess != nil {
		loggedInUser := sess.CAttr("UserName")
		pageData.Meta.LoggedInUser = fmt.Sprintf("%s", loggedInUser)
	}

	// If the referrer is a page from our website, pass that to the login page
	referrer := req.Referer()
	if referrer != "" {
		ref, err := url.Parse(referrer)
		if err != nil {
			log.Printf("Error when parsing referrer URL for login page: %s\n", err)
		} else {
			// localhost:8080 means the server is running on a local (development) box ;)
			if ref.Host == "localhost:8080" || strings.HasSuffix(ref.Host, "dbhub.io") {
				pageData.SourceRef = ref.Path
			}
		}
	}

	// Render the page
	t := tmpl.Lookup("loginPage")
	err := t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func profilePage(w http.ResponseWriter, req *http.Request, userName string) {
	pageName := "User Page"

	// Structure to hold page data
	type starRow struct {
		Username    string
		Database    string
		DateStarred time.Time
	}
	var pageData struct {
		Meta       metaInfo
		PublicDBS  []dbInfo
		PrivateDBS []dbInfo
		Stars      []starRow
	}
	pageData.Meta.Username = userName
	pageData.Meta.Title = userName
	pageData.Meta.Server = conf.Web.Server
	pageData.Meta.LoggedInUser = userName

	// Check if the desired user exists
	row := db.QueryRow("SELECT count(username) FROM public.users WHERE username = $1", userName)
	var userCount int
	err := row.Scan(&userCount)
	if err != nil {
		log.Printf("%s: Error looking up user details failed. User: '%s' Error: %v\n", pageName, userName, err)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
		return
	}

	// If the user doesn't exist, display an error page
	if userCount == 0 {
		errorPage(w, req, http.StatusNotFound, fmt.Sprintf("Unknown user: %s", userName))
		return
	}

	var dbQuery string
	// Retrieve list of public databases for the user
	dbQuery = `
		WITH public_dbs AS (
			SELECT db.dbname, db.last_modified, ver.size, ver.version, db.watchers, db.stars,
				db.forks, db.discussions, db.pull_requests, db.updates, db.branches,
				db.releases, db.contributors, db.description
			FROM sqlite_databases AS db, database_versions AS ver
			WHERE db.idnum = ver.db
				AND db.username = $1
				AND ver.public = true
			ORDER BY dbname, version DESC
		), unique_dbs AS (
			SELECT DISTINCT ON (dbname) * FROM public_dbs ORDER BY dbname
		)
		SELECT * FROM unique_dbs ORDER BY last_modified DESC`
	rows, err := db.Query(dbQuery, userName)
	if err != nil {
		log.Printf("%s: Database query failed: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
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
			log.Printf("%s: Error retrieving public database list for user: %v\n", pageName, err)
			errorPage(w, req, http.StatusInternalServerError, "Error retrieving database list")
			return
		}
		if !Desc.Valid {
			oneRow.Description = ""
		} else {
			oneRow.Description = fmt.Sprintf(": %s", Desc.String)
		}
		pageData.PublicDBS = append(pageData.PublicDBS, oneRow)
	}

	// Retrieve list of private databases for the user
	dbQuery = `
		WITH public_dbs AS (
			SELECT db.dbname, db.last_modified, ver.size, ver.version, db.watchers, db.stars,
				db.forks, db.discussions, db.pull_requests, db.updates, db.branches,
				db.releases, db.contributors, db.description
			FROM sqlite_databases AS db, database_versions AS ver
			WHERE db.idnum = ver.db
				AND db.username = $1
				AND ver.public = false
			ORDER BY dbname, version DESC
		), unique_dbs AS (
			SELECT DISTINCT ON (dbname) * FROM public_dbs ORDER BY dbname
		)
		SELECT * FROM unique_dbs ORDER BY last_modified DESC`
	rows2, err := db.Query(dbQuery, userName)
	if err != nil {
		log.Printf("%s: Database query failed: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
		return
	}
	defer rows2.Close()
	for rows2.Next() {
		var oneRow dbInfo
		var Desc pgx.NullString
		err = rows2.Scan(&oneRow.Database, &oneRow.LastModified, &oneRow.Size, &oneRow.Version,
			&oneRow.Watchers, &oneRow.Stars, &oneRow.Forks, &oneRow.Discussions, &oneRow.PRs,
			&oneRow.Updates, &oneRow.Branches, &oneRow.Releases, &oneRow.Contributors, &Desc)
		if err != nil {
			log.Printf("%s: Error retrieving private database list for user: %v\n", pageName, err)
			errorPage(w, req, http.StatusInternalServerError, "Error retrieving database list")
			return
		}
		if !Desc.Valid {
			oneRow.Description = ""
		} else {
			oneRow.Description = fmt.Sprintf(": %s", Desc.String)
		}
		pageData.PrivateDBS = append(pageData.PrivateDBS, oneRow)
	}

	// Retrieve the list of starred databases for the user
	dbQuery = `
		WITH stars AS (
			SELECT db, date_starred
			FROM database_stars
			WHERE username = $1
		)
		SELECT dbs.username, dbs.dbname, stars.date_starred
		FROM sqlite_databases AS dbs, stars
		WHERE dbs.idnum = stars.db
		ORDER BY date_starred DESC`
	rows3, err := db.Query(dbQuery, userName)
	if err != nil {
		log.Printf("%s: Database query failed: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
		return
	}
	defer rows3.Close()
	for rows3.Next() {
		var oneRow starRow
		err = rows3.Scan(&oneRow.Username, &oneRow.Database, &oneRow.DateStarred)
		if err != nil {
			log.Printf("%s: Error retrieving stars list for user: %v\n", pageName, err)
			errorPage(w, req, http.StatusInternalServerError, "Error retrieving stars list")
			return
		}
		pageData.Stars = append(pageData.Stars, oneRow)
	}

	// Render the page
	t := tmpl.Lookup("profilePage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func registerPage(w http.ResponseWriter, req *http.Request) {
	var pageData struct {
		Meta metaInfo
	}
	pageData.Meta.Title = "Register"

	// Retrieve session data (if any)
	sess := session.Get(req)
	if sess != nil {
		loggedInUser := sess.CAttr("UserName")
		pageData.Meta.LoggedInUser = fmt.Sprintf("%s", loggedInUser)
	}

	// Render the page
	t := tmpl.Lookup("registerPage")
	err := t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func settingsPage(w http.ResponseWriter, req *http.Request, userName string) {
	var pageData struct {
		Meta metaInfo
	}
	pageData.Meta.Title = "Settings"
	pageData.Meta.LoggedInUser = userName

	// Render the page
	t := tmpl.Lookup("settingsPage")
	err := t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func starsPage(w http.ResponseWriter, req *http.Request, userName string, dbName string) {
	pageName := "Stars page"

	type userInfo struct {
		Username    string
		DateStarred time.Time
	}
	var pageData struct {
		Meta  metaInfo
		Stars []userInfo
	}
	pageData.Meta.Title = "Stars"
	pageData.Meta.Username = userName
	pageData.Meta.Database = dbName

	// Retrieve session data (if any)
	sess := session.Get(req)
	if sess != nil {
		loggedInUser := sess.CAttr("UserName")
		pageData.Meta.LoggedInUser = fmt.Sprintf("%s", loggedInUser)
	}

	// Retrieve list of users who starred the database
	dbQuery := `
		SELECT username, date_starred
		FROM database_stars
		WHERE db = (
			SELECT idnum
			FROM sqlite_databases
			WHERE username = $1
				AND dbname = $2
			)
		ORDER BY date_starred DESC`
	rows, err := db.Query(dbQuery, userName, dbName)
	if err != nil {
		log.Printf("%s: Database query failed: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
		return
	}
	defer rows.Close()
	for rows.Next() {
		var oneRow userInfo
		err = rows.Scan(&oneRow.Username, &oneRow.DateStarred)
		if err != nil {
			log.Printf("%s: Error retrieving list of stars for %s/%s: %v\n", pageName, userName, dbName,
				err)
			errorPage(w, req, http.StatusInternalServerError, "Database query failed")
			return
		}
		pageData.Stars = append(pageData.Stars, oneRow)
	}

	// Render the page
	t := tmpl.Lookup("starsPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func uploadPage(w http.ResponseWriter, req *http.Request, userName string) {
	var pageData struct {
		Meta metaInfo
	}
	pageData.Meta.Title = "Upload database"
	pageData.Meta.LoggedInUser = userName

	// Render the page
	t := tmpl.Lookup("uploadPage")
	err := t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func userPage(w http.ResponseWriter, req *http.Request, userName string) {
	pageName := "User Page"

	// Structure to hold page data
	var pageData struct {
		Meta     metaInfo
		DataRows []dbInfo
	}
	pageData.Meta.Username = userName
	pageData.Meta.Title = userName
	pageData.Meta.Server = conf.Web.Server

	// Retrieve session data (if any)
	var loggedInUser string
	sess := session.Get(req)
	if sess != nil {
		loggedInUser = fmt.Sprintf("%s", sess.CAttr("UserName"))
		if loggedInUser == userName {
			// The logged in user is looking at their own user page
			profilePage(w, req, loggedInUser)
			return
		}
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Check if the desired user exists
	row := db.QueryRow("SELECT count(username) FROM public.users WHERE username = $1", userName)
	var userCount int
	err := row.Scan(&userCount)
	if err != nil {
		log.Printf("%s: Error looking up user details failed. User: '%s' Error: %v\n", pageName, userName, err)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
		return
	}

	// If the user doesn't exist, display an error page
	if userCount == 0 {
		errorPage(w, req, http.StatusNotFound, fmt.Sprintf("Unknown user: %s", userName))
		return
	}

	var dbQuery string
	// Retrieve list of public databases for the user
	dbQuery = `
		WITH public_dbs AS (
			SELECT db.dbname, db.last_modified, ver.size, ver.version, db.watchers, db.stars, db.forks,
				db.discussions, db.pull_requests, db.updates, db.branches, db.releases,
				db.contributors, db.description
			FROM sqlite_databases AS db, database_versions AS ver
			WHERE db.idnum = ver.db
				AND db.username = $1
				AND ver.public = true
			ORDER BY dbname, version DESC
		), unique_dbs AS (
			SELECT DISTINCT ON (dbname) * FROM public_dbs ORDER BY dbname
		)
		SELECT * FROM unique_dbs ORDER BY last_modified DESC`
	rows, err := db.Query(dbQuery, userName)
	if err != nil {
		log.Printf("%s: Database query failed: %v\n", pageName, err)
		errorPage(w, req, http.StatusInternalServerError, "Database query failed")
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
			errorPage(w, req, http.StatusInternalServerError, "Error retrieving database list for user")
			return
		}
		if !Desc.Valid {
			oneRow.Description = ""
		} else {
			oneRow.Description = fmt.Sprintf(": %s", Desc.String)
		}
		pageData.DataRows = append(pageData.DataRows, oneRow)
	}

	// Render the page
	t := tmpl.Lookup("userPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}
