package main

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	sqlite "github.com/gwenn/gosqlite"
	"github.com/jackc/pgx"
)

// Check if the user has access to the requested database
func checkUserDBAccess(DB *sqliteDBinfo, loggedInUser string, dbUser string, dbName string) error {
	var queryCacheKey, dbQuery string
	if loggedInUser != dbUser {
		// * The request is for another users database, so it needs to be a public one *
		dbQuery = `
			SELECT ver.minioid, db.date_created, db.last_modified, ver.size, ver.version, db.watchers,
				db.stars, db.forks, db.discussions, db.pull_requests, db.updates, db.branches,
				db.releases, db.contributors, db.description, db.readme, db.minio_bucket
			FROM sqlite_databases AS db, database_versions AS ver
			WHERE db.username = $1
				AND db.dbname = $2
				AND db.idnum = ver.db
				AND ver.public = true
			ORDER BY version DESC
			LIMIT 1`
		tempArr := md5.Sum([]byte(fmt.Sprintf(dbQuery, dbUser, dbName)))
		queryCacheKey = "pub/" + hex.EncodeToString(tempArr[:])
	} else {
		dbQuery = `
			SELECT ver.minioid, db.date_created, db.last_modified, ver.size, ver.version, db.watchers,
				db.stars, db.forks, db.discussions, db.pull_requests, db.updates, db.branches,
				db.releases, db.contributors, db.description, db.readme, db.minio_bucket
			FROM sqlite_databases AS db, database_versions AS ver
			WHERE db.username = $1
				AND db.dbname = $2
				AND db.idnum = ver.db
			ORDER BY version DESC
			LIMIT 1`
		tempArr := md5.Sum([]byte(fmt.Sprintf(dbQuery, dbUser, dbName)))
		queryCacheKey = loggedInUser + "/" + hex.EncodeToString(tempArr[:])
	}

	// Use a cached version of the query response if it exists
	ok, err := getCachedData(queryCacheKey, &DB)
	if err != nil {
		log.Printf("Error retrieving data from cache: %v\n", err)
	}
	if !ok {
		// Retrieve the requested database details
		var Desc, Readme pgx.NullString
		err := db.QueryRow(dbQuery, dbUser, dbName).Scan(&DB.MinioId, &DB.Info.DateCreated,
			&DB.Info.LastModified, &DB.Info.Size, &DB.Info.Version, &DB.Info.Watchers,
			&DB.Info.Stars, &DB.Info.Forks, &DB.Info.Discussions, &DB.Info.MRs,
			&DB.Info.Updates, &DB.Info.Branches, &DB.Info.Releases, &DB.Info.Contributors,
			&Desc, &Readme, &DB.MinioBkt)
		if err != nil {
			log.Printf("Requested database '%s/%s' not found or not available for user\n", dbUser, dbName)
			return errors.New("The requested database doesn't exist")
		}
		if !Desc.Valid {
			DB.Info.Description = "No description"
		} else {
			DB.Info.Description = Desc.String
		}
		if !Readme.Valid {
			DB.Info.Readme = "No readme"
		} else {
			DB.Info.Readme = Readme.String
		}

		// Cache the database details
		err = cacheData(queryCacheKey, DB, 120)
		if err != nil {
			log.Printf("Error when caching page data: %v\n", err)
		}
	}

	return nil
}

// Returns the number of rows in a SQLite table
func getSQLiteRowCount(db *sqlite.Conn, dbTable string) (int, error) {
	dbQuery := "SELECT count(*) FROM " + dbTable
	var rowCount int
	err := db.OneValue(dbQuery, &rowCount)
	if err != nil {
		log.Printf("Error occurred when counting total table rows: %s\n", err)
		return 0, errors.New("Database query failure")
	}
	return rowCount, nil
}

// Extracts and returns the requested table name (if any)
func getTable(r *http.Request) (string, error) {
	var requestedTable string
	requestedTable = r.FormValue("table")

	// If a table name was supplied, validate it
	// FIXME: We should probably create a validation function for SQLite table names, not use our one for PG
	if requestedTable != "" {
		err := validatePGTable(requestedTable)
		if err != nil {
			log.Printf("Validation failed for table name: %s", err)
			return "", errors.New("Invalid table name")
		}
	}

	// Everything seems ok
	return requestedTable, nil
}

// Extracts and returns the requested user and database name
func getUD(ignore_leading int, r *http.Request) (string, string, error) {
	// Split the request URL into path components
	pathStrings := strings.Split(r.URL.Path, "/")

	// Check that at least a username/database combination was requested
	if len(pathStrings) < (3 + ignore_leading) {
		log.Printf("Something wrong with the requested URL: %v\n", r.URL.Path)
		return "", "", errors.New("Invalid URL")
	}
	userName := pathStrings[1+ignore_leading]
	dbName := pathStrings[2+ignore_leading]

	// Validate the user supplied user and database name
	err := validateUserDB(userName, dbName)
	if err != nil {
		log.Printf("Validation failed for user or database name: %s", err)
		return "", "", errors.New("Invalid user or database name")
	}

	// Everything seems ok
	return userName, dbName, nil
}

// Extracts and returns the requested username, database, and table name
func getUDT(ignore_leading int, r *http.Request) (string, string, string, error) {
	// Grab user and database name
	userName, dbName, err := getUD(ignore_leading, r)
	if err != nil {
		return "", "", "", err
	}

	// If a specific table was requested, get that info too
	requestedTable, err := getTable(r)
	if err != nil {
		return "", "", "", err
	}

	// Everything seems ok
	return userName, dbName, requestedTable, nil
}

// Extracts and returns the requested username, database, table name, and version number
func getUDTV(ignore_leading int, r *http.Request) (string, string, string, int64, error) {
	// Grab user and database name
	userName, dbName, err := getUD(ignore_leading, r)
	if err != nil {
		return "", "", "", 0, err
	}

	// If a specific table was requested, get that info too
	requestedTable, err := getTable(r)
	if err != nil {
		return "", "", "", 0, err
	}

	// Extract the version number
	dbVersion, err := getVersion(r)
	if err != nil {
		return "", "", "", 0, err
	}

	// Everything seems ok
	return userName, dbName, requestedTable, dbVersion, nil
}

// Extracts and returns the requested username, database, and database version
func getUDV(ignore_leading int, r *http.Request) (string, string, int64, error) {
	// Grab user and database name
	userName, dbName, err := getUD(ignore_leading, r)
	if err != nil {
		return "", "", 0, err
	}

	// Extract the version number
	dbVersion, err := getVersion(r)
	if err != nil {
		return "", "", 0, err
	}

	// Everything seems ok
	return userName, dbName, dbVersion, nil
}

// Retrieve the user's preference for maximum number of SQLite rows to display
func getUserMaxRowsPref(loggedInUser string) int {
	// Retrieve the user preference data
	dbQuery := `
		SELECT pref_max_rows
		FROM users
		WHERE username = $1`
	var maxRows int
	err := db.QueryRow(dbQuery, loggedInUser).Scan(&maxRows)
	if err != nil {
		log.Printf("Error retrieving user '%s' preference data: %v\n", loggedInUser, err)
		return 10 // Use the default value
	}

	return maxRows
}

// Extract and return the requested version number
func getVersion(r *http.Request) (int64, error) {
	dbVersion, err := strconv.ParseInt(r.FormValue("version"), 10, 0) // This also validates the version input
	if err != nil {
		log.Printf("Invalid database version number: %v\n", err)
		return 0, errors.New("Invalid database version number")
	}
	return dbVersion, nil
}

// Retrieves a SQLite database from Minio, then opens it
func openMinioObject(bucket string, id string) (*sqlite.Conn, error) {
	// Get a handle from Minio for the database object
	userDB, err := minioClient.GetObject(bucket, id)
	if err != nil {
		log.Printf("Error retrieving DB from Minio: %v\n", err)
		return nil, errors.New("Internal retrieving database from object store")
	}

	// Close the object handle when this function finishes
	defer func() {
		err := userDB.Close()
		if err != nil {
			log.Printf("Error closing object handle: %v\n", err)
		}
	}()

	// Save the database locally to a temporary file
	tempfileHandle, err := ioutil.TempFile("", "databaseViewHandler-")
	if err != nil {
		log.Printf("Error creating tempfile: %v\n", err)
		return nil, errors.New("Internal server error")
	}
	tempfile := tempfileHandle.Name()
	bytesWritten, err := io.Copy(tempfileHandle, userDB)
	if err != nil {
		log.Printf("Error writing database to temporary file: %v\n", err)
		return nil, errors.New("Internal server error")
	}
	if bytesWritten == 0 {
		log.Printf("0 bytes written to the SQLite temporary file. Minio object: %s/%s\n", bucket, id)
		return nil, errors.New("Internal server error")
	}
	tempfileHandle.Close()
	defer os.Remove(tempfile) // Delete the temporary file when this function finishes

	// Open database
	db, err := sqlite.Open(tempfile, sqlite.OpenReadOnly)
	if err != nil {
		log.Printf("Couldn't open database: %s", err)
		return nil, errors.New("Internal server error")
	}

	return db, nil
}

// Reads up to maxRows number of rows from a given SQLite database table.  If maxRows < 0 (eg -1), then read all rows.
func readSQLiteDB(db *sqlite.Conn, dbTable string, maxRows int) (sqliteRecordSet, error) {
	return readSQLiteDBCols(db, dbTable, false, false, maxRows, nil, "*")
}

// Reads up to maxRows # of rows from a SQLite database.  Only returns the requested columns
func readSQLiteDBCols(db *sqlite.Conn, dbTable string, ignoreBinary bool, ignoreNull bool, maxRows int,
	filters []whereClause, cols ...string) (sqliteRecordSet, error) {
	// Ugh, have to use string smashing for this, even though the SQL spec doesn't seem to say table names
	// shouldn't be parameterised.  Limitation from SQLite's implementation? :(
	var dataRows sqliteRecordSet
	var err error
	var stmt *sqlite.Stmt

	// Set the table name
	dataRows.Tablename = dbTable

	// Construct the main SQL query
	var colString string
	for i, d := range cols {
		if i != 0 {
			colString += ", "
		}
		colString += fmt.Sprintf("%s", d)
	}
	dbQuery := fmt.Sprintf("SELECT %s FROM %s", colString, dbTable)

	// If filters were given, add them
	var filterVals []interface{}
	if filters != nil {
		for i, d := range filters {
			if i != 0 {
				dbQuery += " AND "
			}
			dbQuery = fmt.Sprintf("%s WHERE %s %s ?", dbQuery, d.Column, d.Type)
			filterVals = append(filterVals, d.Value)
		}
	}

	// If a row limit was given, add it
	if maxRows >= 0 {

		dbQuery = fmt.Sprintf("%s LIMIT %d", dbQuery, maxRows)
	}

	// Use parameter binding for the WHERE clause values
	if filters != nil {
		// Use parameter binding for the user supplied WHERE expression (safety!)
		stmt, err = db.Prepare(dbQuery, filterVals...)
	} else {
		stmt, err = db.Prepare(dbQuery)
	}
	if err != nil {
		log.Printf("Error when preparing statement for database: %s\v", err)
		return dataRows, errors.New("Error when reading data from the SQLite database")
	}

	// Retrieve the field names
	dataRows.ColNames = stmt.ColumnNames()
	dataRows.ColCount = len(dataRows.ColNames)

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
					row = append(row, dataValue{Name: dataRows.ColNames[i], Type: Integer,
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
					row = append(row, dataValue{Name: dataRows.ColNames[i], Type: Float,
						Value: stringVal})
				}
			case sqlite.Text:
				var val string
				val, isNull = s.ScanText(i)
				if !isNull {
					row = append(row, dataValue{Name: dataRows.ColNames[i], Type: Text,
						Value: val})
				}
			case sqlite.Blob:
				// BLOBs can be ignored (via flag to this function) for situations like the vis data
				if !ignoreBinary {
					_, isNull = s.ScanBlob(i)
					if !isNull {
						row = append(row, dataValue{Name: dataRows.ColNames[i], Type: Binary,
							Value: "<i>BINARY DATA</i>"})
					}
				}
			case sqlite.Null:
				isNull = true
			}
			if isNull && !ignoreNull {
				// NULLS can be ignored (via flag to this function) for situations like the vis data
				row = append(row, dataValue{Name: dataRows.ColNames[i], Type: Null,
					Value: "<i>NULL</i>"})
			}
		}
		dataRows.Records = append(dataRows.Records, row)
		dataRows.RowCount++

		return nil
	})
	if err != nil {
		log.Printf("Error when retrieving select data from database: %s\v", err)
		return dataRows, errors.New("Error when reading data from the SQLite database")
	}
	defer stmt.Finalize()

	return dataRows, nil
}
