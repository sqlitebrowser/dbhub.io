package main

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strconv"

	sqlite "github.com/gwenn/gosqlite"
	"github.com/icza/session"
	com "github.com/sqlitebrowser/dbhub.io/common"
)

func databasePage(w http.ResponseWriter, r *http.Request, dbOwner string, dbName string, dbTable string) {
	pageName := "Render database page"

	var pageData struct {
		Auth0  com.Auth0Set
		Data   com.SQLiteRecordSet
		DB     com.SQLiteDBinfo
		Meta   com.MetaInfo
		MyStar bool
	}

	// Retrieve session data (if any)
	var loggedInUser string
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			pageData.Meta.LoggedInUser = loggedInUser
		} else {
			session.Remove(sess, w)
		}
	}

	// Check if the user has access to the requested database
	err := com.CheckUserDBAccess(&pageData.DB, loggedInUser, dbOwner, dbName)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// * Execution can only get here if the user has access to the requested database *

	// Check if the database was starred by the logged in user
	myStar, err := com.CheckDBStarred(loggedInUser, dbOwner, "/", dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve latest social stats")
		return
	}

	// Generate a predictable cache key for the whole page data
	var pageCacheKey string
	if loggedInUser != dbOwner {
		tempArr := md5.Sum([]byte(dbOwner + "/" + dbName + "/" + dbTable))
		pageCacheKey = "dwndb-pub-" + hex.EncodeToString(tempArr[:])
	} else {
		tempArr := md5.Sum([]byte(loggedInUser + "-" + dbOwner + "/" + dbName + "/" + dbTable))
		pageCacheKey = "dwndb-" + hex.EncodeToString(tempArr[:])
	}

	// Determine the number of rows to display
	if loggedInUser != "" {
		pageData.DB.MaxRows = com.PrefUserMaxRows(loggedInUser)
	} else {
		// Not logged in, so default to 10 rows
		pageData.DB.MaxRows = 10
	}

	// If a cached version of the page data exists, use it
	pageCacheKey += "/" + strconv.Itoa(pageData.DB.MaxRows)
	ok, err := com.GetCachedData(pageCacheKey, &pageData)
	if err != nil {
		log.Printf("%s: Error retrieving page data from cache: %v\n", pageName, err)
	}
	if ok {
		// Retrieve up to date counts of watchers, stars, and forks stats
		wa, st, fo, err := com.SocialStats(dbOwner, "/", dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve latest social stats")
			return
		}
		if wa != -1 {
			pageData.DB.Info.Watchers = wa
		}
		if st != -1 {
			pageData.DB.Info.Stars = st
		}
		if fo != -1 {
			pageData.DB.Info.Forks = fo
		}

		// Update database star status for the logged in user
		pageData.MyStar = myStar

		// Render the page from cache
		t := tmpl.Lookup("databasePage")
		err = t.Execute(w, pageData)
		if err != nil {
			log.Printf("Error: %s", err)
		}
		return
	}

	// Get a handle from Minio for the database object
	db, err := com.OpenMinioObject(pageData.DB.MinioBkt, pageData.DB.MinioId)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	defer db.Close()

	// Retrieve the list of tables in the database
	tables, err := db.Tables("")
	if err != nil {
		log.Printf("Error retrieving table names: %s", err)
		// TODO: Add proper error handing here.  Maybe display the page, but show the error where
		// TODO  the table data would otherwise be?
		errorPage(w, r, http.StatusInternalServerError,
			fmt.Sprintf("Error reading from '%s'.  Possibly encrypted or not a database?", dbName))
		return
	}
	if len(tables) == 0 {
		// No table names were returned, so abort
		log.Printf("The database '%s' doesn't seem to have any tables. Aborting.", dbName)
		errorPage(w, r, http.StatusInternalServerError, "Database has no tables?")
		return
	}
	pageData.DB.Info.Tables = tables

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
			log.Printf("%s: Requested table not present in database. DB: '%s/%s', Table: '%s'\n",
				pageName, dbOwner, dbName, dbTable)
			errorPage(w, r, http.StatusBadRequest, "Requested table not present")
			return
		}
	}

	// If a specific table wasn't requested, use the first table in the database
	if dbTable == "" {
		dbTable = pageData.DB.Info.Tables[0]
	}

	// Retrieve (up to) x rows from the selected database
	// Ugh, have to use string smashing for this, even though the SQL spec doesn't seem to say table names
	// shouldn't be parameterised.  Limitation from SQLite's implementation? :(
	stmt, err := db.Prepare("SELECT * FROM "+dbTable+" LIMIT ?", pageData.DB.MaxRows)
	if err != nil {
		log.Printf("Error when preparing statement for database: %s\v", err)
		errorPage(w, r, http.StatusInternalServerError, "Internal error")
		return
	}

	// Retrieve the field names
	pageData.Data.ColNames = stmt.ColumnNames()
	pageData.Data.ColCount = len(pageData.Data.ColNames)

	// Process each row
	fieldCount := -1
	err = stmt.Select(func(s *sqlite.Stmt) error {

		// Get the number of fields in the result
		if fieldCount == -1 {
			fieldCount = stmt.DataCount()
		}

		// Retrieve the data for each row
		var row []com.DataValue
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
					row = append(row, com.DataValue{Name: pageData.Data.ColNames[i],
						Type: com.Integer, Value: stringVal})
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
					row = append(row, com.DataValue{Name: pageData.Data.ColNames[i],
						Type: com.Float, Value: stringVal})
				}
			case sqlite.Text:
				var val string
				val, isNull = s.ScanText(i)
				if !isNull {
					row = append(row, com.DataValue{Name: pageData.Data.ColNames[i],
						Type: com.Text, Value: val})
				}
			case sqlite.Blob:
				_, isNull = s.ScanBlob(i)
				if !isNull {
					row = append(row, com.DataValue{Name: pageData.Data.ColNames[i],
						Type: com.Binary, Value: "<i>BINARY DATA</i>"})
				}
			case sqlite.Null:
				isNull = true
			}
			if isNull {
				row = append(row, com.DataValue{Name: pageData.Data.ColNames[i], Type: com.Null,
					Value: "<i>NULL</i>"})
			}
		}
		pageData.Data.Records = append(pageData.Data.Records, row)

		return nil
	})
	if err != nil {
		log.Printf("Error when retrieving select data from database: %s\v", err)
		errorPage(w, r, http.StatusInternalServerError,
			fmt.Sprintf("Error reading data from '%s'.  Possibly malformed?", dbName))
		return
	}
	defer stmt.Finalize()

	// Count the total number of rows in the selected table
	dbQuery := "SELECT count(*) FROM " + dbTable
	err = db.OneValue(dbQuery, &pageData.Data.RowCount)
	if err != nil {
		log.Printf("%s: Error occurred when counting total table rows: %s\n", pageName, err)
		errorPage(w, r, http.StatusInternalServerError, "Database query failure")
		return
	}

	pageData.Data.Tablename = dbTable
	pageData.Meta.Owner = dbOwner
	pageData.Meta.Database = dbName
	pageData.Meta.Server = com.WebServer()
	pageData.Meta.Title = fmt.Sprintf("%s / %s", dbOwner, dbName)

	// Retrieve the "forked from" information
	frkOwn, frkFol, frkDB, err := com.ForkedFrom(dbOwner, "/", dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failure")
		return
	}
	pageData.Meta.ForkOwner = frkOwn
	pageData.Meta.ForkFolder = frkFol
	pageData.Meta.ForkDatabase = frkDB

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.WebServer() + "/x/callback"
	pageData.Auth0.ClientID = com.Auth0ClientID()
	pageData.Auth0.Domain = com.Auth0Domain()

	// Update database star status for the logged in user
	pageData.MyStar = myStar

	// Cache the page data
	err = com.CacheData(pageCacheKey, pageData, com.CacheTime)
	if err != nil {
		log.Printf("%s: Error when caching page data: %v\n", pageName, err)
	}

	// Render the page
	t := tmpl.Lookup("databasePage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// General error display page.
func errorPage(w http.ResponseWriter, r *http.Request, httpcode int, msg string) {
	var pageData struct {
		Auth0   com.Auth0Set
		Message string
		Meta    com.MetaInfo
	}
	pageData.Message = msg

	// Retrieve session data (if any)
	var loggedInUser string
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			pageData.Meta.LoggedInUser = loggedInUser
		} else {
			session.Remove(sess, w)
		}
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.WebServer() + "/x/callback"
	pageData.Auth0.ClientID = com.Auth0ClientID()
	pageData.Auth0.Domain = com.Auth0Domain()

	// Render the page
	w.WriteHeader(httpcode)
	t := tmpl.Lookup("errorPage")
	err := t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Render the page showing forks of the given database
func forksPage(w http.ResponseWriter, r *http.Request, dbOwner string, dbFolder string, dbName string) {
	var pageData struct {
		Auth0 com.Auth0Set
		Meta  com.MetaInfo
		Forks []com.ForkEntry
	}
	pageData.Meta.Title = "Forks"
	pageData.Meta.Owner = dbOwner
	pageData.Meta.Database = dbName

	// Retrieve session data (if any)
	var loggedInUser string
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			pageData.Meta.LoggedInUser = loggedInUser
		} else {
			session.Remove(sess, w)
		}
	}

	// Retrieve list of forks for the database
	var err error
	pageData.Forks, err = com.ForkTree(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError,
			fmt.Sprintf("Error retrieving fork list for '%s%s%s': %v\n", dbOwner, dbFolder,
				dbName, err.Error()))
		return
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.WebServer() + "/x/callback"
	pageData.Auth0.ClientID = com.Auth0ClientID()
	pageData.Auth0.Domain = com.Auth0Domain()

	// Render the page
	t := tmpl.Lookup("forksPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}

}

// Renders the front page of the website.
func frontPage(w http.ResponseWriter, r *http.Request) {
	// Structure to hold page data
	var pageData struct {
		Auth0 com.Auth0Set
		List  []com.UserInfo
		Meta  com.MetaInfo
	}

	// Retrieve session data (if any)
	var loggedInUser string
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			pageData.Meta.LoggedInUser = loggedInUser
		} else {
			session.Remove(sess, w)
		}
	}

	// Retrieve list of users with public databases
	var err error
	pageData.List, err = com.PublicUserDBs()
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}
	pageData.Meta.Title = `SQLite storage "in the cloud"`

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.WebServer() + "/x/callback"
	pageData.Auth0.ClientID = com.Auth0ClientID()
	pageData.Auth0.Domain = com.Auth0Domain()

	// Render the page
	t := tmpl.Lookup("rootPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Renders the user Preferences page.
func prefPage(w http.ResponseWriter, r *http.Request, loggedInUser string) {
	var pageData struct {
		Auth0   com.Auth0Set
		MaxRows int
		Meta    com.MetaInfo
	}
	pageData.Meta.Title = "Preferences"
	pageData.Meta.LoggedInUser = loggedInUser

	// Retrieve the user preference data
	pageData.MaxRows = com.PrefUserMaxRows(loggedInUser)

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.WebServer() + "/x/callback"
	pageData.Auth0.ClientID = com.Auth0ClientID()
	pageData.Auth0.Domain = com.Auth0Domain()

	// Render the page
	t := tmpl.Lookup("prefPage")
	err := t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func profilePage(w http.ResponseWriter, r *http.Request, userName string) {
	var pageData struct {
		Auth0      com.Auth0Set
		Meta       com.MetaInfo
		PrivateDBs []com.DBInfo
		PublicDBs  []com.DBInfo
		Stars      []com.DBEntry
	}
	pageData.Meta.Owner = userName
	pageData.Meta.Title = userName
	pageData.Meta.Server = com.WebServer()
	pageData.Meta.LoggedInUser = userName

	// Check if the desired user exists
	userExists, err := com.CheckUserExists(userName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// If the user doesn't exist, indicate that
	if !userExists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Unknown user: %s", userName))
		return
	}

	// Retrieve list of public databases for the user
	pageData.PublicDBs, err = com.UserDBs(userName, com.DB_PUBLIC)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Retrieve list of private databases for the user
	pageData.PrivateDBs, err = com.UserDBs(userName, com.DB_PRIVATE)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Retrieve the list of starred databases for the user
	pageData.Stars, err = com.UserStarredDBs(userName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.WebServer() + "/x/callback"
	pageData.Auth0.ClientID = com.Auth0ClientID()
	pageData.Auth0.Domain = com.Auth0Domain()

	// Render the page
	t := tmpl.Lookup("profilePage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func selectUsernamePage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		Auth0 com.Auth0Set
		Meta  com.MetaInfo
		Nick  string
	}
	pageData.Meta.Title = "Select your username"

	// Retrieve session data (if any)
	sess := session.Get(r)
	if sess != nil {
		validRegSession := false
		va := sess.CAttr("registrationinprogress")
		if va == nil {
			// This isn't a valid username selection session, so abort
			errorPage(w, r, http.StatusBadRequest, "Invalid user creation session")
			return
		}
		validRegSession = va.(bool)

		if validRegSession != true {
			// For some reason this isn't a valid username selection session, so abort
			errorPage(w, r, http.StatusBadRequest, "Invalid user creation session")
			return
		}
	} else {
		errorPage(w, r, http.StatusBadRequest, "Invalid user creation session")
		return
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.WebServer() + "/x/callback"
	pageData.Auth0.ClientID = com.Auth0ClientID()
	pageData.Auth0.Domain = com.Auth0Domain()

	// If the Auth0 profile included a nickname, we use that to prefill the input field
	ni := sess.CAttr("nickname")
	if ni != nil {
		pageData.Nick = ni.(string)
	}

	// Render the page
	t := tmpl.Lookup("selectUsernamePage")
	err := t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Render the stars page.
func starsPage(w http.ResponseWriter, r *http.Request, dbOwner string, dbName string) {
	var pageData struct {
		Auth0 com.Auth0Set
		Meta  com.MetaInfo
		Stars []com.DBEntry
	}
	pageData.Meta.Title = "Stars"
	pageData.Meta.Owner = dbOwner
	pageData.Meta.Database = dbName

	// Retrieve session data (if any)
	var loggedInUser string
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			pageData.Meta.LoggedInUser = loggedInUser
		} else {
			session.Remove(sess, w)
		}
	}

	// Retrieve list of users who starred the database
	var err error
	pageData.Stars, err = com.UsersStarredDB(dbOwner, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.WebServer() + "/x/callback"
	pageData.Auth0.ClientID = com.Auth0ClientID()
	pageData.Auth0.Domain = com.Auth0Domain()

	// Render the page
	t := tmpl.Lookup("starsPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func uploadPage(w http.ResponseWriter, r *http.Request, userName string) {
	var pageData struct {
		Auth0 com.Auth0Set
		Meta  com.MetaInfo
	}
	pageData.Meta.Title = "Upload database"
	pageData.Meta.LoggedInUser = userName

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.WebServer() + "/x/callback"
	pageData.Auth0.ClientID = com.Auth0ClientID()
	pageData.Auth0.Domain = com.Auth0Domain()

	// Render the page
	t := tmpl.Lookup("uploadPage")
	err := t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func userPage(w http.ResponseWriter, r *http.Request, userName string) {
	// Structure to hold page data
	var pageData struct {
		Auth0  com.Auth0Set
		DBRows []com.DBInfo
		Meta   com.MetaInfo
	}
	pageData.Meta.Owner = userName
	pageData.Meta.Title = userName
	pageData.Meta.Server = com.WebServer()

	// Retrieve session data (if any)
	var loggedInUser string
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			if loggedInUser == userName {
				// The logged in user is looking at their own user page
				profilePage(w, r, loggedInUser)
				return
			}
			pageData.Meta.LoggedInUser = loggedInUser
		} else {
			session.Remove(sess, w)
		}
	}

	// Check if the desired user exists
	userExists, err := com.CheckUserExists(userName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// If the user doesn't exist, indicate that
	if !userExists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Unknown user: %s", userName))
		return
	}

	// Retrieve list of public databases for the user
	pageData.DBRows, err = com.UserDBs(userName, com.DB_PUBLIC)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.WebServer() + "/x/callback"
	pageData.Auth0.ClientID = com.Auth0ClientID()
	pageData.Auth0.Domain = com.Auth0Domain()

	// Render the page
	t := tmpl.Lookup("userPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func visualisePage(w http.ResponseWriter, r *http.Request) {
	// Structure to hold page data
	var pageData struct {
		Auth0    com.Auth0Set
		ColNames []string
		Data     com.SQLiteRecordSet
		DB       com.SQLiteDBinfo
		Meta     com.MetaInfo
	}
	pageData.Meta.Title = "Visualise data"

	// Retrieve user and database name
	dbOwner, dbName, requestedTable, err := com.GetODT(1, r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	pageData.Meta.Owner = dbOwner
	pageData.Meta.Database = dbName

	// Retrieve session data (if any)
	var loggedInUser string
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			pageData.Meta.LoggedInUser = loggedInUser
		} else {
			session.Remove(sess, w)
		}
	}

	// Check if the user has access to the requested database
	err = com.CheckUserDBAccess(&pageData.DB, loggedInUser, dbOwner, dbName)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Get a handle from Minio for the database object
	db, err := com.OpenMinioObject(pageData.DB.MinioBkt, pageData.DB.MinioId)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	defer db.Close()

	// Retrieve the list of tables in the database
	tables, err := db.Tables("")
	if err != nil {
		log.Printf("Error retrieving table names: %s", err)
		// TODO: Add proper error handing here.  Maybe display the page, but show the error where
		// TODO  the table data would otherwise be?
		errorPage(w, r, http.StatusInternalServerError,
			fmt.Sprintf("Error reading from '%s'.  Possibly encrypted or not a database?", dbName))
		return
	}
	if len(tables) == 0 {
		// No table names were returned, so abort
		log.Printf("The database '%s' doesn't seem to have any tables. Aborting.", dbName)
		errorPage(w, r, http.StatusInternalServerError, "Database has no tables?")
		return
	}
	pageData.DB.Info.Tables = tables

	// If a specific table was requested, check it exists
	if requestedTable != "" {
		tablePresent := false
		for _, tableName := range tables {
			if requestedTable == tableName {
				tablePresent = true
			}
		}
		if tablePresent == false {
			// The requested table doesn't exist
			errorPage(w, r, http.StatusBadRequest, "Requested table does not exist")
			return
		}
	}

	// If no specific table was requested, just choose the first one given to us in the list from the database
	if requestedTable == "" {
		requestedTable = tables[0]
	}
	pageData.Data.Tablename = requestedTable

	// Retrieve a list of all column names in the specified table
	var tempStruct com.SQLiteRecordSet
	tempStruct, err = com.ReadSQLiteDB(db, requestedTable, 1)
	if err != nil {
		// Some kind of error when reading the database data
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	pageData.ColNames = tempStruct.ColNames

	// TODO: If a full visualisation profile was specified, we should gather the data for it and provide it to the
	// TODO  render function

	// Read all of the data from the requested (or default) table, add it to the page data
	pageData.Data, err = com.ReadSQLiteDB(db, requestedTable, 1000) // 1000 row maximum for now
	if err != nil {
		// Some kind of error when reading the database data
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.WebServer() + "/x/callback"
	pageData.Auth0.ClientID = com.Auth0ClientID()
	pageData.Auth0.Domain = com.Auth0Domain()

	// Render the page
	t := tmpl.Lookup("visualisePage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}
