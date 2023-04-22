package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"sort"

	sqlite "github.com/gwenn/gosqlite"
	com "github.com/sqlitebrowser/dbhub.io/common"
)

// branchesHandler returns the list of branches for a database
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" \
//	    -F dbowner="justinclift" \
//	    -F dbname="Join Testing.sqlite" \
//	    https://api.dbhub.io/v1/branches
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbowner" is the owner of the database
//	* "dbname" is the name of the database
func branchesHandler(w http.ResponseWriter, r *http.Request) {
	// Do auth check, grab request info
	_, dbOwner, dbName, _, httpStatus, err := collectInfo(w, r)
	if err != nil {
		jsonErr(w, err.Error(), httpStatus)
		return
	}

	// If the database is a live database, we return an error message
	isLive, _, err := com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if isLive {
		jsonErr(w, "That database is a live database.  It doesn't have branches.", http.StatusBadRequest)
		return
	}

	// Retrieve the branch list for the database
	brList, err := com.BranchListResponse(dbOwner, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the list as JSON
	jsonList, err := json.MarshalIndent(brList, "", "  ")
	if err != nil {
		errMsg := fmt.Sprintf("Error when JSON marshalling the branch list: %v", err)
		log.Print(errMsg)
		jsonErr(w, errMsg, http.StatusBadRequest)
		return
	}
	fmt.Fprintf(w, string(jsonList))
	return
}

// changeLogHandler handles requests for the Changelog (a html page)
func changeLogHandler(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		ServerName string
	}

	// Log the incoming request
	logReq(r, "-")

	// Pass through some variables, useful for the generated docs
	pageData.ServerName = com.Conf.Web.ServerName

	// Display our API documentation
	t := tmpl.Lookup("changelog")
	err := t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// columnsHandler returns the list of columns in a table or view
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" \
//	    -F dbowner="justinclift" \
//	    -F dbname="Join Testing.sqlite" \
//	    -F table="table1" https://api.dbhub.io/v1/columns
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbowner" is the owner of the database
//	* "dbname" is the name of the database
//	* "table" is the name of the table or view
func columnsHandler(w http.ResponseWriter, r *http.Request) {
	// Do auth check, grab request info
	loggedInUser, dbOwner, dbName, _, httpStatus, err := collectInfo(w, r)
	if err != nil {
		jsonErr(w, err.Error(), httpStatus)
		return
	}

	// Extract the table name
	table, err := com.GetFormTable(r, false)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Make sure a table name was provided
	if table == "" {
		jsonErr(w, "Missing table name", http.StatusBadRequest)
		return
	}

	// Check if the database is a live database, and get the node/queue to send the request to
	isLive, liveNode, err := com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If a live database has been uploaded but doesn't have a live node handling its requests, then error out as this
	// should never happen
	if isLive && liveNode == "" {
		jsonErr(w, "No AMQP node available for request", http.StatusInternalServerError)
		return
	}

	// If it's a standard database, process it locally.  Else send the query to our AMQP backend
	var cols []sqlite.Column
	if !isLive {
		// Get Minio bucket and object id for the SQLite file
		bucket, id, _, err := com.MinioLocation(dbOwner, dbName, "", loggedInUser)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Sanity check
		if id == "" {
			// The requested database wasn't found, or the user doesn't have permission to access it
			jsonErr(w, "Requested database not found", http.StatusNotFound)
			return
		}

		// Retrieve the database from Minio, then open it
		var sdb *sqlite.Conn
		sdb, err = com.OpenSQLiteDatabase(bucket, id)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer sdb.Close()

		// Verify the requested table or view we're about to query does exist
		var tablesViews []string
		tablesViews, err = com.TablesAndViews(sdb, dbName)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tableOrViewFound := false
		for _, t := range tablesViews {
			if t == table {
				tableOrViewFound = true
			}
		}
		if !tableOrViewFound {
			jsonErr(w, "Provided table or view name doesn't exist in this database", http.StatusBadRequest)
			return
		}

		// Retrieve the list of columns for the table
		cols, err = sdb.Columns("", table)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		// Send the columns request to our AMQP backend
		cols, _, err = com.LiveColumns(liveNode, loggedInUser, dbOwner, dbName, table)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusBadRequest)
			log.Println(err)
			return
		}
	}

	// Transfer the column info into our own structure, for better json formatting
	var jsonCols []com.APIJSONColumn
	for _, j := range cols {
		jsonCols = append(jsonCols, com.APIJSONColumn{
			Cid:       j.Cid,
			Name:      j.Name,
			DataType:  j.DataType,
			NotNull:   j.NotNull,
			DfltValue: j.DfltValue,
			Pk:        j.Pk,
		})
	}

	// Return the results
	jsonData, err := json.MarshalIndent(jsonCols, "", "  ")
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in columnsHandler(): %v", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// commitsHandler returns the details of all commits for a database
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/commits
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbowner" is the owner of the database
//	* "dbname" is the name of the database
func commitsHandler(w http.ResponseWriter, r *http.Request) {
	// Do auth check, grab request info
	_, dbOwner, dbName, _, httpStatus, err := collectInfo(w, r)
	if err != nil {
		jsonErr(w, err.Error(), httpStatus)
		return
	}

	// If the database is a live database, we return an error message
	isLive, _, err := com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if isLive {
		jsonErr(w, "That database is a live database.  It doesn't have commits.", http.StatusBadRequest)
		return
	}

	// Retrieve the commits
	commits, err := com.GetCommitList(dbOwner, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the tags as JSON
	jsonData, err := json.MarshalIndent(commits, "", "  ")
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in commitsHandler(): %v", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// databasesHandler returns the list of databases in the requesting users account.
// If the new (optional) "live" boolean text field is set to true, then it will return the list of live
// databases.  Otherwise, it will return the list of standard databases.
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F live="true" https://api.dbhub.io/v1/databases
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "live" is whether to show Live databases, or standard ones
func databasesHandler(w http.ResponseWriter, r *http.Request) {
	// Authenticate the request
	loggedInUser, err := checkAuth(w, r)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Get "live" boolean value, if provided by the caller
	var live bool
	live, err = com.GetFormLive(r)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Retrieve the list of databases in the user account
	var databases []com.DBInfo
	if !live {
		// Get the list of standard databases
		databases, err = com.UserDBs(loggedInUser, com.DB_BOTH)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		// Get the list of live databases
		databases, err = com.LiveUserDBs(loggedInUser)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Extract just the database names
	var list []string
	for _, j := range databases {
		list = append(list, j.Database)
	}

	// Return the results
	jsonData, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in databasesHandler(): %v", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// deleteHandler deletes a database from the requesting users account
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/delete
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbname" is the name of the database
func deleteHandler(w http.ResponseWriter, r *http.Request) {
	// Authenticate the request
	loggedInUser, err := checkAuth(w, r)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Validate the database name
	var dbName string
	dbName, err = com.GetDatabase(r, false)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	dbOwner := loggedInUser

	// Check if the database exists
	exists, err := com.CheckDBPermissions(loggedInUser, dbOwner, dbName, false)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !exists {
		jsonErr(w, "Database does not exist, or user isn't authorised to access it", http.StatusNotFound)
		return
	}

	// For a standard database, invalidate its memcache data
	var isLive bool
	var liveNode string
	isLive, liveNode, err = com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusNotFound)
		return
	}
	if !isLive {
		err = com.InvalidateCacheEntry(loggedInUser, dbOwner, dbName, "") // Empty string indicates "for all versions"
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// For a live database, delete it from both Minio and our AMQP backend
	var bucket, id string
	if isLive {
		// Get the Minio bucket and object names for this database
		bucket, id, err = com.LiveGetMinioNames(loggedInUser, dbOwner, dbName)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Delete the database from Minio
		err = com.MinioDeleteDatabase("API server", dbOwner, dbName, bucket, id)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Delete the database from our AMQP backend
		err = com.LiveDelete(liveNode, loggedInUser, dbOwner, dbName)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Delete the database in PostgreSQL
	err = com.DeleteDatabase(dbOwner, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return a "success" message
	z := com.StatusResponseContainer{Status: "OK"}
	jsonData, err := json.MarshalIndent(z, "", "  ")
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in deleteHandler(): %v", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// diffHandler generates a diff between two databases or two versions of a database
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner_a="justinclift" -F dbname_a="Join Testing.sqlite" -F commit_a="ea12..." -F commit_b="5a7c..." https://api.dbhub.io/v1/diff
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbowner_a" is the owner of the first database being diffed
//	* "dbname_a" is the name of the first database being diffed
//	* "dbowner_b" is the owner of the second database being diffed (optional, if not provided same as first owner)
//	* "dbname_b" is the name of the second database being diffed (optional, if not provided same as first name)
//	* "commit_a" is the first commit for diffing
//	* "commit_b" is the second commit for diffing
//	* "merge" specifies the merge strategy (possible values: "none", "preserve_pk", "new_pk"; optional, defaults to "none")
//	* "include_data" can be set to "1" to include the full data of all changed rows instead of just the primary keys (optional, defaults to 0)
func diffHandler(w http.ResponseWriter, r *http.Request) {
	loggedInUser, err := checkAuth(w, r)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Get merge strategy and parse value. Default to "none"
	merge := r.PostFormValue("merge")
	mergeStrategy := com.NoMerge
	if merge == "preserve_pk" {
		mergeStrategy = com.PreservePkMerge
	} else if merge == "new_pk" {
		mergeStrategy = com.NewPkMerge
	} else if merge != "" && merge != "none" {
		jsonErr(w, "Invalid merge strategy", http.StatusBadRequest)
		return
	}

	// Get include data parameter
	includeDataValue := r.PostFormValue("include_data")
	includeData := false
	if includeDataValue == "1" {
		includeData = true
	}

	// Retrieve owner, name, and commit ids
	oa := r.PostFormValue("dbowner_a")
	na := r.PostFormValue("dbname_a")
	ca := r.PostFormValue("commit_a")
	ob := r.PostFormValue("dbowner_b")
	nb := r.PostFormValue("dbname_b")
	cb := r.PostFormValue("commit_b")

	// If no primary database owner and name are given or if no commit ids are given, return
	if oa == "" || na == "" || ca == "" || cb == "" {
		jsonErr(w, "Incomplete database details provided", http.StatusBadRequest)
		return
	}

	// If no secondary database owner and name are provided, use the ones of the first database
	if ob == "" || nb == "" {
		ob = oa
		nb = na
	}

	// Unescape, then validate the owner and database names and commit ids
	dbOwnerA, err := url.QueryUnescape(oa)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	dbOwnerB, err := url.QueryUnescape(ob)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	dbNameA, err := url.QueryUnescape(na)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	dbNameB, err := url.QueryUnescape(nb)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = com.ValidateUser(dbOwnerA)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = com.ValidateUser(dbOwnerB)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = com.ValidateDB(dbNameA)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = com.ValidateDB(dbNameB)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = com.ValidateCommitID(ca)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = com.ValidateCommitID(cb)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check permissions of the first database
	var allowed bool
	allowed, err = com.CheckDBPermissions(loggedInUser, dbOwnerA, dbNameA, false)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		jsonErr(w, "Database not found", http.StatusNotFound)
		return
	}

	// Check permissions of the second database
	allowed, err = com.CheckDBPermissions(loggedInUser, dbOwnerB, dbNameB, false)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		jsonErr(w, "Database not found", http.StatusNotFound)
		return
	}

	// If either database is a live database, we return an error message
	var isLive bool
	isLive, _, err = com.CheckDBLive(dbOwnerA, dbNameA)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if isLive {
		jsonErr(w, fmt.Sprintf("'%s/%s' is a live database.  It doesn't support diffs.", dbOwnerA, dbNameA), http.StatusBadRequest)
		return
	}
	isLive, _, err = com.CheckDBLive(dbOwnerB, dbNameB)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if isLive {
		jsonErr(w, fmt.Sprintf("'%s/%s' is a live database.  It doesn't support diffs.", dbOwnerB, dbNameB), http.StatusBadRequest)
		return
	}

	// Perform diff
	diffs, err := com.Diff(dbOwnerA, dbNameA, ca, dbOwnerB, dbNameB, cb, loggedInUser, mergeStrategy, includeData)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the results
	jsonData, err := json.MarshalIndent(diffs, "", "  ")
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in diffHandler(): %v", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// downloadHandler returns the requested SQLite database file.
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" -OJ https://api.dbhub.io/v1/download
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbowner" is the owner of the database
//	* "dbname" is the name of the database
func downloadHandler(w http.ResponseWriter, r *http.Request) {
	// Authenticate user and collect requested database details
	loggedInUser, dbOwner, dbName, commitID, httpStatus, err := collectInfo(w, r)
	if err != nil {
		jsonErr(w, err.Error(), httpStatus)
		return
	}

	// Return the requested database to the user
	_, err = com.DownloadDatabase(w, r, dbOwner, dbName, commitID, loggedInUser, "api")
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	return

}

// executeHandler executes a SQL query on a SQLite database.  It's used for running SQL queries which don't
// return a result set, like `INSERT`, `UPDATE`, `DELETE`, and so forth.
// This can be run from the command line using curl, like this:
//
//	$ curl -kD headers.out -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" \
//	    -F sql="VVBEQVRFIHRhYmxlMSBTRVQgTmFtZSA9ICdUZXN0aW5nIDEnIFdIRVJFIGlkID0gMQ==" \
//	    https://api.dbhub.io/v1/execute
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbowner" is the owner of the database
//	* "dbname" is the name of the database
//	* "sql" is the SQL query to execute, base64 encoded
//	NOTE that the above example (base64) encoded sql is: "UPDATE table1 SET Name = 'Testing 1' WHERE id = 1"
func executeHandler(w http.ResponseWriter, r *http.Request) {
	loggedInUser, err := checkAuth(w, r)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Extract the database owner name, database name, and (optional) commit ID for the database from the request
	var dbOwner, dbName string
	dbOwner, dbName, _, err = com.GetFormODC(r)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Grab the incoming SQLite query
	rawInput := r.FormValue("sql")
	var sql string
	sql, err = com.CheckUnicode(rawInput)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check if the requested database exists
	var exists bool
	exists, err = com.CheckDBPermissions(loggedInUser, dbOwner, dbName, false)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !exists {
		jsonErr(w, fmt.Sprintf("Database '%s/%s' doesn't exist", dbOwner, dbName),
			http.StatusNotFound)
		return
	}

	// Check if the database is a live database, and get the node/queue to send the request to
	var isLive bool
	var liveNode string
	isLive, liveNode, err = com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Reject attempts to run Execute() on non-live databases
	if !isLive {
		jsonErr(w, "Execute() only runs on Live databases.  This is not a live database.", http.StatusInternalServerError)
		return
	}

	// If a live database has been uploaded but doesn't have a live node handling its requests, then error out as this
	// should never happen
	if isLive && liveNode == "" {
		jsonErr(w, "No AMQP node available for request", http.StatusInternalServerError)
		return
	}

	// Send the SQL execution request to our AMQP backend
	var rowsChanged int
	rowsChanged, err = com.LiveExecute(liveNode, loggedInUser, dbOwner, dbName, sql)
	if err != nil {
		log.Println(err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// The Execute() succeeded, so pass along the # of rows changed
	z := com.ExecuteResponseContainer{RowsChanged: rowsChanged, Status: "OK"}
	jsonData, err := json.MarshalIndent(z, "", "  ")
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in executeHandler(): %v", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// indexesHandler returns the details of all indexes in a SQLite database
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/indexes
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbowner" is the owner of the database
//	* "dbname" is the name of the database
func indexesHandler(w http.ResponseWriter, r *http.Request) {
	// Do auth check, grab request info
	loggedInUser, dbOwner, dbName, _, httpStatus, err := collectInfo(w, r)
	if err != nil {
		jsonErr(w, err.Error(), httpStatus)
		return
	}

	// Check if the database is a live database, and get the node/queue to send the request to
	isLive, liveNode, err := com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If a live database has been uploaded but doesn't have a live node handling its requests, then error out as this
	// should never happen
	if isLive && liveNode == "" {
		jsonErr(w, "No AMQP node available for request", http.StatusInternalServerError)
		return
	}

	// If it's a standard database, process it locally.  Else send the query to our AMQP backend
	var indexes []com.APIJSONIndex
	if !isLive {
		// Get Minio bucket and object id for the SQLite file
		bucket, id, _, err := com.MinioLocation(dbOwner, dbName, "", loggedInUser)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Sanity check
		if id == "" {
			// The requested database wasn't found, or the user doesn't have permission to access it
			jsonErr(w, "Requested database not found", http.StatusNotFound)
			return
		}

		// Retrieve the database from Minio, then open it
		var sdb *sqlite.Conn
		sdb, err = com.OpenSQLiteDatabase(bucket, id)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer sdb.Close()

		// Retrieve the list of indexes
		var idx map[string]string
		idx, err = sdb.Indexes("")
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Retrieve the details for each index
		for nam, tab := range idx {
			oneIndex := com.APIJSONIndex{
				Name:    nam,
				Table:   tab,
				Columns: []com.APIJSONIndexColumn{},
			}
			cols, err := sdb.IndexColumns("", nam)
			if err != nil {
				jsonErr(w, err.Error(), http.StatusInternalServerError)
				return
			}
			for _, k := range cols {
				oneIndex.Columns = append(oneIndex.Columns, com.APIJSONIndexColumn{
					CID:  k.Cid,
					Name: k.Name,
				})
			}
			indexes = append(indexes, oneIndex)
		}
	} else {
		// Send the indexes request to our AMQP backend
		var rawResponse []byte
		rawResponse, err = com.MQRequest(com.AmqpChan, liveNode, "indexes", loggedInUser, dbOwner, dbName, "")
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			log.Println(err)
			return
		}

		// Decode the response
		var resp com.LiveDBIndexesResponse
		err = json.Unmarshal(rawResponse, &resp)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			log.Println(err)
			return
		}
		if resp.Error != "" {
			err = errors.New(resp.Error)
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if resp.Node == "" {
			log.Printf("In API (Live) indexesHandler().  A node responded, but didn't identify itself.")
			return
		}
		indexes = resp.Indexes
	}

	// Return the results
	jsonData, err := json.MarshalIndent(indexes, "", "  ")
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in indexesHandler(): %v", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// metadataHandler returns the commit, branch, release, tag and web page information for a database
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/metadata
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbowner" is the owner of the database
//	* "dbname" is the name of the database
func metadataHandler(w http.ResponseWriter, r *http.Request) {
	// Do auth check, grab request info
	_, dbOwner, dbName, _, httpStatus, err := collectInfo(w, r)
	if err != nil {
		jsonErr(w, err.Error(), httpStatus)
		return
	}

	// If the database is a live database, we return an error message
	isLive, _, err := com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if isLive {
		jsonErr(w, "That database is a live database.  It doesn't support metadata.", http.StatusBadRequest)
		return
	}

	// Retrieve the metadata for the database
	meta, err := com.MetadataResponse(dbOwner, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the list as JSON
	jsonList, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		errMsg := fmt.Sprintf("Error when JSON marshalling the metadata: %v", err)
		log.Print(errMsg)
		jsonErr(w, errMsg, http.StatusBadRequest)
		return
	}
	fmt.Fprintf(w, string(jsonList))
	return
}

// queryHandler executes a SQL query on a SQLite database, returning the results to the caller
// This can be run from the command line using curl, like this:
//
//	$ curl -kD headers.out -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" \
//	    -F sql="U0VMRUNUIHRhYmxlMS5OYW1lLCB0YWJsZTIudmFsdWUKRlJPTSB0YWJsZTEgSk9JTiB0YWJsZTIKVVNJTkcgKGlkKQpPUkRFUiBCWSB0YWJsZTEuaWQ7" \
//	    https://api.dbhub.io/v1/query
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbowner" is the owner of the database
//	* "dbname" is the name of the database
//	* "sql" is the SQL query to run, base64 encoded
func queryHandler(w http.ResponseWriter, r *http.Request) {
	loggedInUser, err := checkAuth(w, r)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Extract the database owner name, database name, and (optional) commit ID for the database from the request
	dbOwner, dbName, commitID, err := com.GetFormODC(r)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Grab the incoming SQLite query
	rawInput := r.FormValue("sql")
	query, err := com.CheckUnicode(rawInput)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckDBPermissions(loggedInUser, dbOwner, dbName, false)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !exists {
		jsonErr(w, fmt.Sprintf("Database '%s/%s' doesn't exist", dbOwner, dbName),
			http.StatusNotFound)
		return
	}

	// Check if the database is a live database, and get the node/queue to send the request to
	isLive, liveNode, err := com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If a live database has been uploaded but doesn't have a live node handling its requests, then error out as this
	// should never happen
	if isLive && liveNode == "" {
		jsonErr(w, "No AMQP node available for request", http.StatusInternalServerError)
		return
	}

	// Run the query
	var data com.SQLiteRecordSet
	if !isLive {
		// Standard database
		data, err = com.SQLiteRunQueryDefensive(w, r, com.QuerySourceAPI, dbOwner, dbName, commitID, loggedInUser, query)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		// Send the query to the appropriate backend live node
		data, err = com.LiveQuery(liveNode, loggedInUser, dbOwner, dbName, query)
		if err != nil {
			log.Println(err)
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Return the results
	jsonData, err := json.MarshalIndent(data.Records, "", "  ")
	if err != nil {
		jsonErr(w, fmt.Sprintf("Error when JSON marshalling the returned data: %v", err),
			http.StatusBadRequest)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// releasesHandler returns the details of all releases for a database
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/releases
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbowner" is the owner of the database
//	* "dbname" is the name of the database
func releasesHandler(w http.ResponseWriter, r *http.Request) {
	// Do auth check, grab request info
	_, dbOwner, dbName, _, httpStatus, err := collectInfo(w, r)
	if err != nil {
		jsonErr(w, err.Error(), httpStatus)
		return
	}

	// If the database is a live database, we return an error message
	isLive, _, err := com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if isLive {
		jsonErr(w, "That database is a live database.  It doesn't support releases.", http.StatusBadRequest)
		return
	}

	// Retrieve the list of releases
	rels, err := com.GetReleases(dbOwner, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the list as JSON
	jsonData, err := json.MarshalIndent(rels, "", "  ")
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in releasesHandler(): %v", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// rootHandler handles requests for "/" and all unknown paths
func rootHandler(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		ServerName string
	}

	// Log the incoming request
	logReq(r, "-")

	// If the incoming request is for anything other than the index page, return a 404
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Pass through some variables, useful for the generated docs
	pageData.ServerName = com.Conf.Web.ServerName

	// Display our API documentation
	t := tmpl.Lookup("docs")
	err := t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// tablesHandler returns the list of tables in a SQLite database
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/tables
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbowner" is the owner of the database
//	* "dbname" is the name of the database
func tablesHandler(w http.ResponseWriter, r *http.Request) {
	// Do auth check, grab request info
	loggedInUser, dbOwner, dbName, _, httpStatus, err := collectInfo(w, r)
	if err != nil {
		jsonErr(w, err.Error(), httpStatus)
		return
	}

	// Check if the database is a live database, and get the node/queue to send the request to
	isLive, liveNode, err := com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If a live database has been uploaded but doesn't have a live node handling its requests, then error out as this
	// should never happen
	if isLive && liveNode == "" {
		jsonErr(w, "No AMQP node available for request", http.StatusInternalServerError)
		return
	}

	// If it's a standard database, process it locally.  Else send the query to our AMQP backend
	var tables []string
	if !isLive {
		// Get Minio bucket and object id for the SQLite file
		bucket, id, _, err := com.MinioLocation(dbOwner, dbName, "", loggedInUser)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Sanity check
		if id == "" {
			// The requested database wasn't found, or the user doesn't have permission to access it
			jsonErr(w, "Requested database not found", http.StatusNotFound)
			return
		}

		// Retrieve the database from Minio, then open it
		var sdb *sqlite.Conn
		sdb, err = com.OpenSQLiteDatabase(bucket, id)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer sdb.Close()

		// Retrieve the list of tables
		tables, err = com.Tables(sdb)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		// Send the tables request to our AMQP backend
		tables, err = com.LiveTables(liveNode, loggedInUser, dbOwner, dbName)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Return the results
	sort.Strings(tables)
	jsonData, err := json.MarshalIndent(tables, "", "  ")
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in tablesHandler(): %v", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// tagsHandler returns the details of all tags for a database
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/tags
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbowner" is the owner of the database
//	* "dbname" is the name of the database
func tagsHandler(w http.ResponseWriter, r *http.Request) {
	// Do auth check, grab request info
	_, dbOwner, dbName, _, httpStatus, err := collectInfo(w, r)
	if err != nil {
		jsonErr(w, err.Error(), httpStatus)
		return
	}

	// If the database is a live database, we return an error message
	isLive, _, err := com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if isLive {
		jsonErr(w, "That database is a live database.  It doesn't support tags.", http.StatusBadRequest)
		return
	}

	// Retrieve the tags
	tags, err := com.GetTags(dbOwner, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the tags as JSON
	jsonData, err := json.MarshalIndent(tags, "", "  ")
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in tagsHandler(): %v", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// uploadHandler creates a new database in your account, or adds a new commit to an existing database
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F dbname="Join Testing.sqlite" -F file=@someupload.sqlite \
//	    -F "branch=main" -F "commitmsg=stuff" -F "sourceurl=https://example.org" \
//	    -F "lastmodified=2017-01-02T03:04:05Z"  -F "licence=CC0"  -F "public=true" \
//	    -F "commit=51d494f2c5eb6734ddaa204eccb9597b426091c79c951924ac83c72038f22b55" https://api.dbhub.io/v1/upload
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbname" is the name of the database being created
//	* "file" is the database file to upload
//	* "branch" (optional) is the database branch this commit is for.  Uses the default database branch if not specified
//	* "commitmsg" (optional) is a message to include with the commit.  Often a description of the changes in the new data
//	* "sourceurl" (optional) is the URL to the reference source of the data
//	* "lastmodified" (optional) is a datestamp in RFC3339 format
//	* "licence" (optional) is an identifier for a license that's "in the system"
//	* "live" (optional) is a boolean string ("true", "false") indicating whether this upload is a live database
//	* "public" (optional) is whether the database should be public.  True means "public", false means "not public"
//	* "commit" (ignored for new databases, required for existing ones) is the commit ID this new database revision
//	   should be appended to.  For new databases it's not needed, but for existing databases it's required (it's used to
//	   detect out of date / conflicting uploads)
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	// Authenticate the request
	loggedInUser, err := checkAuth(w, r)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Set the maximum accepted database size for uploading
	oversizeAllowed := false
	for _, user := range com.Conf.UserMgmt.SizeOverrideUsers {
		if loggedInUser == user {
			oversizeAllowed = true
		}
	}
	if !oversizeAllowed {
		r.Body = http.MaxBytesReader(w, r.Body, com.MaxDatabaseSize*1024*1024)
	}

	// Extract the database name and (optional) commit ID for the database from the request
	_, dbName, commitID, err := com.GetFormODC(r)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// The "public" user isn't allowed to make changes
	if loggedInUser == "public" {
		log.Printf("User from '%s' attempted to add a database using the public certificate", r.RemoteAddr)
		jsonErr(w, "You're using the 'public' certificate, which isn't allowed to make changes on the server",
			http.StatusUnauthorized)
		return
	}

	// Check whether the uploaded database is too large
	if !oversizeAllowed {
		if r.ContentLength > (com.MaxDatabaseSize * 1024 * 1024) {
			jsonErr(w, fmt.Sprintf("Database is too large. Maximum database upload size is %d MB, yours is %d MB",
				com.MaxDatabaseSize, r.ContentLength/1024/1024), http.StatusBadRequest)
			log.Printf("'%s' attempted to upload an oversized database %d MB in size.  Limit is %d MB",
				loggedInUser, r.ContentLength/1024/1024, com.MaxDatabaseSize)
			return
		}
	}

	// Get "live" boolean value, if provided by the caller
	var live bool
	live, err = com.GetFormLive(r)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Process the upload
	var httpStatus int
	var x map[string]string
	dbOwner := loggedInUser // We always use the API key / cert owner as the database owner for uploads
	if !live {
		x, httpStatus, err = com.UploadResponse(w, r, loggedInUser, dbOwner, dbName, commitID, "api")
		if err != nil {
			jsonErr(w, err.Error(), httpStatus)
			return
		}
	} else {
		// FIXME: The code below is grabbed from com.UploadResponse(), and is also very similar to the code in the
		//        webui uploadDataHandler().  May be able to refactor them.

		// Grab the uploaded file and form variables
		var tempFile multipart.File
		var handler *multipart.FileHeader
		tempFile, handler, err = r.FormFile("file")
		if err != nil && err.Error() != "http: no such file" {
			log.Printf("Uploading file failed: %v", err)
			jsonErr(w, fmt.Sprintf("Something went wrong when grabbing the file data: '%s'", err.Error()), http.StatusBadRequest)
			return
		}
		if err != nil {
			if err.Error() == "http: no such file" {
				// Check for a 'file1' FormFile too, as some clients can't use 'file' (without a number) due to a design bug
				tempFile, handler, err = r.FormFile("file1")
				if err != nil {
					log.Printf("Uploading file failed: %v", err)
					jsonErr(w, fmt.Sprintf("Something went wrong when grabbing the file data: '%s'", err.Error()), http.StatusBadRequest)
					return
				}
			}
		}
		defer tempFile.Close()

		// If no database name was passed as a function argument, use the name given in the upload itself
		if dbName == "" {
			dbName = handler.Filename
		}

		// Validate the database name
		err = com.ValidateDB(dbName)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Check if the database exists already
		exists, err := com.CheckDBExists(loggedInUser, dbName)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// If the upload is a live database, but the database already exists, then abort the upload
		// TODO: Consider if we want the existing "force" flag to be useful here, to potentially allow overwriting a
		//       live database
		if exists && live {
			jsonErr(w, "You're uploading a live database, but the same database name already exists.  "+
				"Delete that one first if you really want to overwrite it", http.StatusConflict)
			return
		}

		// Write the incoming database to a temporary file on disk, and sanity check it
		var numBytes int64
		var tempDB *os.File
		numBytes, tempDB, _, _, err = com.WriteDBtoDisk(loggedInUser, dbOwner, dbName, tempFile)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer os.Remove(tempDB.Name())

		// Rewind the internal cursor in the temporary file back to the start again
		var newOffset int64
		newOffset, err = tempDB.Seek(0, 0)
		if err != nil {
			log.Printf("Seeking on the temporary file (2nd time) failed: %s", err)
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if newOffset != 0 {
			jsonErr(w, "Seeking to start of temporary database file didn't work", http.StatusInternalServerError)
			return
		}

		// Store the database in Minio
		objectID, err := com.LiveStoreDatabaseMinio(tempDB, dbOwner, dbName, numBytes)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Log the successful database upload
		log.Printf("API Server: Username '%s' uploaded LIVE database '%s/%s', bytes: %v", loggedInUser,
			com.SanitiseLogString(dbOwner), com.SanitiseLogString(dbName), numBytes)

		// Send a request to the AMQP backend to set up the database there, ready for querying
		err = com.LiveCreateDB(com.AmqpChan, dbOwner, dbName, objectID, com.SetToPrivate)
		if err != nil {
			log.Println(err)
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Upload was successful, so we construct a fake commit ID then return a success message to the user
		x = make(map[string]string)
		x["commit_id"] = ""
		x["url"] = fmt.Sprintf("/%s", dbOwner)
	}

	// Construct the response message
	var ok bool
	var newCommit, newURL string
	if newCommit, ok = x["commit_id"]; !ok {
		jsonErr(w, "Something went wrong when uploading the database, no commit ID was returned",
			http.StatusInternalServerError)
		return
	}
	if newURL, ok = x["url"]; !ok {
		jsonErr(w, "Something went wrong when uploading the database, no url was returned",
			http.StatusInternalServerError)
		return
	}
	z := com.UploadResponseContainer{CommitID: newCommit, URL: newURL}
	jsonData, err := json.MarshalIndent(z, "", "  ")
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in uploadHandler(): %v", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated) // Signal the successful database creation
	fmt.Fprintf(w, string(jsonData))
}

// viewsHandler returns the list of views in a SQLite database
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/views
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbowner" is the owner of the database being queried
//	* "dbname" is the name of the database being queried
func viewsHandler(w http.ResponseWriter, r *http.Request) {
	// Do auth check, grab request info
	loggedInUser, dbOwner, dbName, _, httpStatus, err := collectInfo(w, r)
	if err != nil {
		jsonErr(w, err.Error(), httpStatus)
		return
	}

	// Check if the database is a live database, and get the node/queue to send the request to
	isLive, liveNode, err := com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If a live database has been uploaded but doesn't have a live node handling its requests, then error out as this
	// should never happen
	if isLive && liveNode == "" {
		jsonErr(w, "No AMQP node available for request", http.StatusInternalServerError)
		return
	}

	// If it's a standard database, process it locally.  Else send the query to our AMQP backend
	var views []string
	if !isLive {
		// Get Minio bucket and object id for the SQLite file
		bucket, id, _, err := com.MinioLocation(dbOwner, dbName, "", loggedInUser)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Sanity check
		if id == "" {
			// The requested database wasn't found, or the user doesn't have permission to access it
			jsonErr(w, "Requested database not found", http.StatusNotFound)
			return
		}

		// Retrieve the database from Minio, then open it
		var sdb *sqlite.Conn
		sdb, err = com.OpenSQLiteDatabase(bucket, id)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer sdb.Close()

		// Retrieve the list of views
		views, err = com.Views(sdb)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		// Send the views request to our AMQP backend
		views, err = com.LiveViews(liveNode, loggedInUser, dbOwner, dbName)
		if err != nil {
			jsonErr(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Return the results
	sort.Strings(views)
	jsonData, err := json.MarshalIndent(views, "", "  ")
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in viewsHandler(): %v", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// webpageHandler returns the address of the database in the webUI.  eg. for web browsers
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/webpage
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbowner" is the owner of the database being queried
//	* "dbname" is the name of the database being queried
func webpageHandler(w http.ResponseWriter, r *http.Request) {
	// Authenticate user and collect requested database details
	_, dbOwner, dbName, _, httpStatus, err := collectInfo(w, r)
	if err != nil {
		jsonErr(w, err.Error(), httpStatus)
		return
	}

	// Return the database webUI URL to the user
	var z com.WebpageResponseContainer
	z.WebPage = "https://" + com.Conf.Web.ServerName + "/" + dbOwner + "/" + dbName
	jsonData, err := json.MarshalIndent(z, "", "  ")
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in webpageHandler(): %v", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}
