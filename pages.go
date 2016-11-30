package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	sqlite "github.com/gwenn/gosqlite"
	"github.com/jackc/pgx"
)

func databasePage(w http.ResponseWriter, userName string, databaseName string) {
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
		log.Printf("%s: Database query failed: %v\n", pageName, err)
		http.Error(w, "Database query failed", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var pageData struct {
		Meta metaInfo
		DB   dbInfo
	}

	var minioID string
	for rows.Next() {
		var Desc pgx.NullString
		var Readme pgx.NullString
		err = rows.Scan(&minioID, &pageData.DB.DateCreated, &pageData.DB.LastModified, &pageData.DB.Size,
			&pageData.DB.Version, &pageData.DB.Public, &pageData.DB.Watchers, &pageData.DB.Stars,
			&pageData.DB.Forks, &pageData.DB.Discussions, &pageData.DB.PRs, &pageData.DB.Updates,
			&pageData.DB.Branches, &pageData.DB.Releases, &pageData.DB.Contributors, &Desc, &Readme)
		if err != nil {
			log.Printf("%s: Error retrieving metadata from database: %v\n", pageName, err)
			http.Error(w, "Error retrieving metadata from database", http.StatusInternalServerError)
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
		log.Printf("%s: Requested database not found: %v for user: %v \n", pageName, databaseName, userName)
		http.Error(w, "The requested database doesn't exist", http.StatusNotFound)
		return
	}

	// Get a handle from Minio for the database object
	userDB, err := minioClient.GetObject(userName, minioID)
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
	pageData.DB.Tables = tables

	// Select the first table
	selectedTable := pageData.DB.Tables[0]

	// Retrieve (up to) x rows from the selected database
	// Ugh, have to use string smashing for this, even though the SQL spec doesn't seem to say table names
	// shouldn't be parameterised.  Limitation from SQLite's implementation? :(
	stmt, err := db.Prepare("SELECT * FROM " + selectedTable + " LIMIT 10")
	if err != nil {
		log.Printf("Error when preparing statement for database: %s\v", err)
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
		http.Error(w, fmt.Sprintf("Error reading data from '%s'.  Possibly malformed?", databaseName),
			http.StatusInternalServerError)
		return
	}
	defer stmt.Finalize()

	pageData.DB.Tablename = selectedTable
	pageData.Meta.Username = userName
	pageData.Meta.Database = databaseName
	pageData.Meta.Protocol = listenProtocol
	pageData.Meta.Server = listenAddr + ":9080"
	pageData.Meta.Title = fmt.Sprintf("%s / %s", userName, databaseName)

	// Render the page
	t := tmpl.Lookup("databasePage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func frontPage(w http.ResponseWriter) {
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
		log.Printf("%s: Database query failed: %v\n", pageName, err)
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

func registerPage(w http.ResponseWriter) {
	var pageData struct {
		Meta metaInfo
	}
	pageData.Meta.Title = "Register"

	// Render the page
	t := tmpl.Lookup("registerPage")
	err := t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func userPage(w http.ResponseWriter, userName string) {
	pageName := "User Page"

	// Structure to hold page data
	var pageData struct {
		Meta     metaInfo
		DataRows []dbInfo
	}
	pageData.Meta.Username = userName
	pageData.Meta.Title = userName

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
		log.Printf("%s: Database query failed: %v\n", pageName, err)
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
		pageData.DataRows = append(pageData.DataRows, oneRow)
	}

	// TODO: Check if the user exists, and display an error message if they don't, or if they have no public
	// TODO  databases.  This can probably be done by checking the row count from dbQuery above, and barfing if
	// TODO  it's 0

	// Render the page
	t := tmpl.Lookup("userPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}
