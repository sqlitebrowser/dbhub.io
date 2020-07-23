package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	sqlite "github.com/gwenn/gosqlite"
	com "github.com/sqlitebrowser/dbhub.io/common"
)

// getTablesHandler returns the list of tables present in a SQLite database
// This can be run from the command line using curl, like this:
//   $ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/get_tables
//   * "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//   * "dbowner" is the owner of the database being queried
//   * "dbname" is the name of the database being queried
func getTablesHandler(w http.ResponseWriter, r *http.Request) {
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
	dbFolder := "/"

	// Check if the user has access to the requested database
	var bucket, id string
	bucket, id, _, err = com.MinioLocation(dbOwner, dbFolder, dbName, commitID, loggedInUser)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Sanity check
	if id == "" {
		// The requested database wasn't found, or the user doesn't have permission to access it
		err = fmt.Errorf("Requested database not found")
		log.Printf("Requested database not found. Owner: '%s%s%s'", dbOwner, dbFolder, dbName)
		jsonErr(w, err.Error(), http.StatusNotFound)
		return
	}

	// Retrieve database file from Minio, using locally cached version if it's already there
	newDB, err := com.RetrieveDatabaseFile(bucket, id)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusNotFound)
		return
	}

	// Open the SQLite database in read only mode
	var sdb *sqlite.Conn
	sdb, err = sqlite.Open(newDB, sqlite.OpenReadOnly)
	if err != nil {
		log.Printf("Couldn't open database in getTablesHandler(): %s", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err = sdb.EnableExtendedResultCodes(true); err != nil {
		log.Printf("Couldn't enable extended result codes in getTablesHandler(): %v\n", err.Error())
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Retrieve the list of tables
	var returnData struct {
		Tables []string `json:"tables"`
	}
	returnData.Tables, err = com.Tables(sdb, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the results
	jsonData, err := json.Marshal(returnData.Tables)
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in getTablesHandler(): %v\n", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// queryHandler executes a SQL query on a SQLite database, returning the results to the caller
// This can be run from the command line using curl, like this:
//   $ curl -kD headers.out -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" \
//       -F sql="U0VMRUNUIHRhYmxlMS5OYW1lLCB0YWJsZTIudmFsdWUKRlJPTSB0YWJsZTEgSk9JTiB0YWJsZTIKVVNJTkcgKGlkKQpPUkRFUiBCWSB0YWJsZTEuaWQ7" \
//       https://api.dbhub.io/v1/query
//   * "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//   * "dbowner" is the owner of the database being queried
//   * "dbname" is the name of the database being queried
//   * "sql" is the SQL query to run, base64 encoded
func queryHandler(w http.ResponseWriter, r *http.Request) {
	loggedInUser, err := checkAuth(w, r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Extract the database owner name, database name, and (optional) commit ID for the database from the request
	dbOwner, dbName, commitID, err := com.GetFormODC(r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	dbFolder := "/"

	// Grab the incoming SQLite query
	rawInput := r.FormValue("sql")
	decodedStr, err := com.CheckUnicode(rawInput)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err)
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Database '%s%s%s' doesn't exist", dbOwner, dbFolder, dbName)
		return
	}

	// Run the query
	var data com.SQLiteRecordSet
	data, err = com.SQLiteRunQueryDefensive(w, r, com.API, dbOwner, dbFolder, dbName, commitID, loggedInUser, decodedStr)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}

	// Return the results
	jsonData, err := json.Marshal(data.Records)
	if err != nil {
		errMsg := fmt.Sprintf("Error when JSON marshalling the returned data: %v\n", err)
		log.Print(errMsg)
		http.Error(w, errMsg, http.StatusBadRequest)
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
