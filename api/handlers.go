package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"

	com "github.com/sqlitebrowser/dbhub.io/common"
)

// branchesHandler returns the list of branches for a database
// This can be run from the command line using curl, like this:
//   $ curl -F apikey="YOUR_API_KEY_HERE" \
//       -F dbowner="justinclift" \
//       -F dbname="Join Testing.sqlite" \
//       https://api.dbhub.io/v1/branches
//   * "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//   * "dbowner" is the owner of the database
//   * "dbname" is the name of the database
func branchesHandler(w http.ResponseWriter, r *http.Request) {
	// Do auth check, grab request info
	loggedInUser, dbOwner, dbName, apiKey, _, httpStatus, err := collectInfo(w, r)
	if err != nil {
		jsonErr(w, err.Error(), httpStatus)
		return
	}
	dbFolder := "/"

	// Make sure the API key has permission to run this function on the requested database
	err = permissionCheck(loggedInUser, apiKey, dbName, "branches")
	if err != nil {
		jsonErr(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Retrieve the branch list for the database
	brList, err := com.BranchListResponse(dbOwner, dbFolder, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the list as JSON
	jsonList, err := json.MarshalIndent(brList, "", "  ")
	if err != nil {
		errMsg := fmt.Sprintf("Error when JSON marshalling the branch list: %v\n", err)
		log.Print(errMsg)
		jsonErr(w, errMsg, http.StatusBadRequest)
		return
	}
	fmt.Fprintf(w, string(jsonList))
	return
}

// columnsHandler returns the list of columns in a table or view
// This can be run from the command line using curl, like this:
//   $ curl -F apikey="YOUR_API_KEY_HERE" \
//       -F dbowner="justinclift" \
//       -F dbname="Join Testing.sqlite" \
//       -F table="tablename" https://api.dbhub.io/v1/columns
//   * "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//   * "dbowner" is the owner of the database
//   * "dbname" is the name of the database
//   * "table" is the name of the table or view
func columnsHandler(w http.ResponseWriter, r *http.Request) {
	// Do auth check, grab request info, open the database
	sdb, httpStatus, err := collectInfoAndOpen(w, r, "columns")
	if err != nil {
		jsonErr(w, err.Error(), httpStatus)
		return
	}
	defer sdb.Close()

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

	// Retrieve the list of columns for the table
	cols, err := sdb.Columns("", table)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
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
		log.Printf("Error when JSON marshalling returned data in columnsHandler(): %v\n", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// commitsHandler returns the details of all commits for a database
// This can be run from the command line using curl, like this:
//   $ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/commits
//   * "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//   * "dbowner" is the owner of the database
//   * "dbname" is the name of the database
func commitsHandler(w http.ResponseWriter, r *http.Request) {
	// Do auth check, grab request info
	loggedInUser, dbOwner, dbName, apiKey, _, httpStatus, err := collectInfo(w, r)
	if err != nil {
		jsonErr(w, err.Error(), httpStatus)
		return
	}
	dbFolder := "/"

	// Make sure the API key has permission to run this function on the requested database
	err = permissionCheck(loggedInUser, apiKey, dbName, "commits")
	if err != nil {
		jsonErr(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Retrieve the commits
	commits, err := com.GetCommitList(dbOwner, dbFolder, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the tags as JSON
	jsonData, err := json.MarshalIndent(commits, "", "  ")
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in commitsHandler(): %v\n", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// databasesHandler returns the list of databases in the requesting users account
// This can be run from the command line using curl, like this:
//   $ curl -F apikey="YOUR_API_KEY_HERE" https://api.dbhub.io/v1/databases
//   * "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
func databasesHandler(w http.ResponseWriter, r *http.Request) {
	// Authenticate the request
	loggedInUser, apiKey, err := checkAuth(w, r)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Make sure the API key has permission to run this function on the requested database
	// TODO: We probably need a special case for handling the Databases(), Releases(), and Tags() functions.
	//       Maybe set the dbName value here to a magic value, which permissionCheck() looks for?
	err = permissionCheck(loggedInUser, apiKey, "what should we do here", "databases")
	if err != nil {
		jsonErr(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Retrieve the list of databases in the user account
	var databases []com.DBInfo
	databases, err = com.UserDBs(loggedInUser, com.DB_BOTH)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Extract just the database names
	var list []string
	for _, j := range databases {
		list = append(list, j.Database)
	}

	// Return the results
	jsonData, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in databasesHandler(): %v\n", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// deleteHandler deletes a database from the requesting users account
// This can be run from the command line using curl, like this:
//   $ curl -F apikey="YOUR_API_KEY_HERE" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/delete
//   * "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//   * "dbname" is the name of the database
func deleteHandler(w http.ResponseWriter, r *http.Request) {
	// Authenticate the request
	loggedInUser, apiKey, err := checkAuth(w, r)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Validate the database name
	dbName, err := com.GetDatabase(r, false)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	dbOwner := loggedInUser
	dbFolder := "/"

	// Make sure the API key has permission to run this function on the requested database
	err = permissionCheck(loggedInUser, apiKey, dbName, "delete")
	if err != nil {
		jsonErr(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Invalidate the memcache data for the database
	err = com.InvalidateCacheEntry(loggedInUser, dbOwner, dbFolder, dbName, "") // Empty string indicates "for all versions"
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Delete the database
	err = com.DeleteDatabase(dbOwner, dbFolder, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return a "success" message
	z := com.StatusResponseContainer{Status: "OK"}
	jsonData, err := json.MarshalIndent(z, "", "  ")
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in deleteHandler(): %v\n", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// diffHandler generates a diff between two databases or two versions of a database
// This can be run from the command line using curl, like this:
//   $ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner_a="justinclift" -F dbname_a="Join Testing.sqlite" -F commit_a="ea12..." -F commit_b="5a7c..." https://api.dbhub.io/v1/diff
//   * "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//   * "dbowner_a" is the owner of the first database being diffed
//   * "dbname_a" is the name of the first database being diffed
//   * "dbowner_b" is the owner of the second database being diffed (optional, if not provided same as first owner)
//   * "dbname_b" is the name of the second database being diffed (optional, if not provided same as first name)
//   * "commit_a" is the first commit for diffing
//   * "commit_b" is the second commit for diffing
//   * "merge" specifies the merge strategy (possible values: "none", "preserve_pk", "new_pk"; optional, defaults to "none")
//   * "include_data" can be set to "1" to include the full data of all changed rows instead of just the primary keys (optional, defaults to 0)
func diffHandler(w http.ResponseWriter, r *http.Request) {
	loggedInUser, apiKey, err := checkAuth(w, r)
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

	// Make sure the API key has permission to run this function on the requested databases
	err = permissionCheck(loggedInUser, apiKey, dbNameA, "diff")
	if err != nil {
		jsonErr(w, err.Error(), http.StatusUnauthorized)
		return
	}
	err = permissionCheck(loggedInUser, apiKey, dbNameB, "diff")
	if err != nil {
		jsonErr(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Perform diff
	diffs, err := com.Diff(dbOwnerA, "/", dbNameA, ca, dbOwnerB, "/", dbNameB, cb, loggedInUser, mergeStrategy, includeData)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the results
	jsonData, err := json.MarshalIndent(diffs, "", "  ")
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in diffHandler(): %v\n", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// downloadHandler returns the requested SQLite database file
// This can be run from the command line using curl, like this:
//   $ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/download
//   * "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//   * "dbowner" is the owner of the database
//   * "dbname" is the name of the database
func downloadHandler(w http.ResponseWriter, r *http.Request) {
	// Authenticate user and collect requested database details
	loggedInUser, dbOwner, dbName, apiKey, commitID, httpStatus, err := collectInfo(w, r)
	if err != nil {
		jsonErr(w, err.Error(), httpStatus)
		return
	}
	dbFolder := "/"

	// Make sure the API key has permission to run this function on the requested database
	err = permissionCheck(loggedInUser, apiKey, dbName, "download")
	if err != nil {
		jsonErr(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Return the requested database to the user
	_, err = com.DownloadDatabase(w, r, dbOwner, dbFolder, dbName, commitID, loggedInUser, "api")
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// indexesHandler returns the details of all indexes in a SQLite database
// This can be run from the command line using curl, like this:
//   $ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/indexes
//   * "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//   * "dbowner" is the owner of the database
//   * "dbname" is the name of the database
func indexesHandler(w http.ResponseWriter, r *http.Request) {
	// Do auth check, grab request info, open the database
	sdb, httpStatus, err := collectInfoAndOpen(w, r, "indexes")
	if err != nil {
		jsonErr(w, err.Error(), httpStatus)
		return
	}
	defer sdb.Close()

	// Retrieve the list of indexes
	idx, err := sdb.Indexes("")
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Retrieve the column details for each index
	var indexes []com.APIJSONIndex
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

	// Return the results
	jsonData, err := json.MarshalIndent(indexes, "", "  ")
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in indexesHandler(): %v\n", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// metadataHandler returns the commit, branch, release, tag and web page information for a database
// This can be run from the command line using curl, like this:
//   $ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/metadata
//   * "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//   * "dbowner" is the owner of the database
//   * "dbname" is the name of the database
func metadataHandler(w http.ResponseWriter, r *http.Request) {
	// Do auth check, grab request info
	loggedInUser, dbOwner, dbName, apiKey, _, httpStatus, err := collectInfo(w, r)
	if err != nil {
		jsonErr(w, err.Error(), httpStatus)
		return
	}
	dbFolder := "/"

	// Make sure the API key has permission to run this function on the requested database
	err = permissionCheck(loggedInUser, apiKey, dbName, "metadata")
	if err != nil {
		jsonErr(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Retrieve the metadata for the database
	meta, err := com.MetadataResponse(dbOwner, dbFolder, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the list as JSON
	jsonList, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		errMsg := fmt.Sprintf("Error when JSON marshalling the metadata: %v\n", err)
		log.Print(errMsg)
		jsonErr(w, errMsg, http.StatusBadRequest)
		return
	}
	fmt.Fprintf(w, string(jsonList))
	return
}

// queryHandler executes a SQL query on a SQLite database, returning the results to the caller
// This can be run from the command line using curl, like this:
//   $ curl -kD headers.out -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" \
//       -F sql="U0VMRUNUIHRhYmxlMS5OYW1lLCB0YWJsZTIudmFsdWUKRlJPTSB0YWJsZTEgSk9JTiB0YWJsZTIKVVNJTkcgKGlkKQpPUkRFUiBCWSB0YWJsZTEuaWQ7" \
//       https://api.dbhub.io/v1/query
//   * "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//   * "dbowner" is the owner of the database
//   * "dbname" is the name of the database
//   * "sql" is the SQL query to run, base64 encoded
func queryHandler(w http.ResponseWriter, r *http.Request) {
	loggedInUser, apiKey, err := checkAuth(w, r)
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

	// Make sure the API key has permission to run this function on the requested database
	err = permissionCheck(loggedInUser, apiKey, dbName, "query")
	if err != nil {
		jsonErr(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Grab the incoming SQLite query
	rawInput := r.FormValue("sql")
	decodedStr, err := com.CheckUnicode(rawInput)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckDBPermissions(loggedInUser, dbOwner, dbFolder, dbName, false)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !exists {
		jsonErr(w, fmt.Sprintf("Database '%s%s%s' doesn't exist", dbOwner, dbFolder, dbName),
			http.StatusNotFound)
		return
	}

	// Run the query
	var data com.SQLiteRecordSet
	data, err = com.SQLiteRunQueryDefensive(w, r, com.API, dbOwner, dbFolder, dbName, commitID, loggedInUser, decodedStr)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the results
	jsonData, err := json.MarshalIndent(data.Records, "", "  ")
	if err != nil {
		jsonErr(w, fmt.Sprintf("Error when JSON marshalling the returned data: %v\n", err),
			http.StatusBadRequest)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// releasesHandler returns the details of all releases for a database
// This can be run from the command line using curl, like this:
//   $ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/releases
//   * "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//   * "dbowner" is the owner of the database
//   * "dbname" is the name of the database
func releasesHandler(w http.ResponseWriter, r *http.Request) {
	// Do auth check, grab request info
	loggedInUser, dbOwner, dbName, apiKey, _, httpStatus, err := collectInfo(w, r)
	if err != nil {
		jsonErr(w, err.Error(), httpStatus)
		return
	}
	dbFolder := "/"

	// Make sure the API key has permission to run this function on the requested database
	// TODO: We probably need a special case for handling the Databases(), Releases(), and Tags() functions.
	//       Maybe set the dbName value here to a magic value, which permissionCheck() looks for?
	err = permissionCheck(loggedInUser, apiKey, "what should we do here", "releases")
	if err != nil {
		jsonErr(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Retrieve the list of releases
	rels, err := com.GetReleases(dbOwner, dbFolder, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the list as JSON
	jsonData, err := json.MarshalIndent(rels, "", "  ")
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in releasesHandler(): %v\n", err)
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
//   $ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/tables
//   * "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//   * "dbowner" is the owner of the database
//   * "dbname" is the name of the database
func tablesHandler(w http.ResponseWriter, r *http.Request) {
	// Do auth check, grab request info, open the database
	sdb, httpStatus, err := collectInfoAndOpen(w, r, "tables")
	if err != nil {
		jsonErr(w, err.Error(), httpStatus)
		return
	}
	defer sdb.Close()

	// Retrieve the list of tables
	tables, err := com.Tables(sdb)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the results
	jsonData, err := json.MarshalIndent(tables, "", "  ")
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in tablesHandler(): %v\n", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// tagsHandler returns the details of all tags for a database
// This can be run from the command line using curl, like this:
//   $ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/tags
//   * "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//   * "dbowner" is the owner of the database
//   * "dbname" is the name of the database
func tagsHandler(w http.ResponseWriter, r *http.Request) {
	// Do auth check, grab request info
	loggedInUser, dbOwner, dbName, apiKey, _, httpStatus, err := collectInfo(w, r)
	if err != nil {
		jsonErr(w, err.Error(), httpStatus)
		return
	}
	dbFolder := "/"

	// Make sure the API key has permission to run this function on the requested database
	// TODO: We probably need a special case for handling the Databases(), Releases(), and Tags() functions.
	//       Maybe set the dbName value here to a magic value, which permissionCheck() looks for?
	err = permissionCheck(loggedInUser, apiKey, "what should we do here", "tags")
	if err != nil {
		jsonErr(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Retrieve the tags
	tags, err := com.GetTags(dbOwner, dbFolder, dbName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the tags as JSON
	jsonData, err := json.MarshalIndent(tags, "", "  ")
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in tagsHandler(): %v\n", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// uploadHandler creates a new database in your account, or adds a new commit to an existing database
// This can be run from the command line using curl, like this:
//   $ curl -F apikey="YOUR_API_KEY_HERE" -F dbname="Join Testing.sqlite" -F file=@someupload.sqlite \
//       -F "branch=master" -F "commitmsg=stuff" -F "sourceurl=https://example.org" \
//       -F "lastmodified=2017-01-02T03:04:05Z"  -F "licence=CC0"  -F "public=true" \
//       -F "commit=51d494f2c5eb6734ddaa204eccb9597b426091c79c951924ac83c72038f22b55" https://api.dbhub.io/v1/upload
//   * "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//   * "dbname" is the name of the database being created
//   * "file" is the database file to upload
//   * "branch" (optional) is the database branch this commit is for.  Uses the default database branch if not specified
//   * "commitmsg" (optional) is a message to include with the commit.  Often a description of the changes in the new data
//   * "sourceurl" (optional) is the URL to the reference source of the data
//   * "lastmodified" (optional) is a datestamp in RFC3339 format
//   * "licence" (optional) is an identifier for a license that's "in the system"
//   * "public" (optional) is whether or not the database should be public.  True means "public", false means "not public"
//   * "commit" (ignored for new databases, required for existing ones) is the commit ID this new database revision
//      should be appended to.  For new databases it's not needed, but for existing databases it's required (its used to
//      detect out of date / conflicting uploads)
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	// Set the maximum accepted database size for uploading
	r.Body = http.MaxBytesReader(w, r.Body, com.MaxDatabaseSize*1024*1024)

	// Authenticate the request
	loggedInUser, apiKey, err := checkAuth(w, r)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Extract the database name and (optional) commit ID for the database from the request
	_, dbName, commitID, err := com.GetFormODC(r)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Make sure the API key has permission to run this function on the requested database
	err = permissionCheck(loggedInUser, apiKey, dbName, "upload")
	if err != nil {
		jsonErr(w, err.Error(), http.StatusUnauthorized)
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
	// TODO: Have a list of users (from the config.toml file) which don't have this check applied
	if r.ContentLength > (com.MaxDatabaseSize * 1024 * 1024) {
		jsonErr(w,
			fmt.Sprintf("Database is too large. Maximum database upload size is %d MB, yours is %d MB",
				com.MaxDatabaseSize, r.ContentLength/1024/1024), http.StatusBadRequest)
		log.Println(fmt.Sprintf("'%s' attempted to upload an oversized database %d MB in size.  Limit is %d MB\n",
			loggedInUser, r.ContentLength/1024/1024, com.MaxDatabaseSize))
		return
	}

	// Process the upload
	var httpStatus int
	var x map[string]string
	dbOwner := loggedInUser // We always use the API key / cert owner as the database owner for uploads
	x, httpStatus, err = com.UploadResponse(w, r, loggedInUser, dbOwner, dbName, commitID, "api")
	if err != nil {
		jsonErr(w, err.Error(), httpStatus)
		return
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
		log.Printf("Error when JSON marshalling returned data in uploadHandler(): %v\n", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))

	// Signal the successful database creation
	http.Error(w, "", http.StatusCreated)
}

// viewsHandler returns the list of views in a SQLite database
// This can be run from the command line using curl, like this:
//   $ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/views
//   * "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//   * "dbowner" is the owner of the database being queried
//   * "dbname" is the name of the database being queried
func viewsHandler(w http.ResponseWriter, r *http.Request) {
	// Do auth check, grab request info, open the database
	sdb, httpStatus, err := collectInfoAndOpen(w, r, "views")
	if err != nil {
		jsonErr(w, err.Error(), httpStatus)
		return
	}
	defer sdb.Close()

	// Retrieve the list of views
	views, err := com.Views(sdb)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the results
	jsonData, err := json.MarshalIndent(views, "", "  ")
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in viewsHandler(): %v\n", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// webpageHandler returns the address of the database in the webUI.  eg. for web browsers
// This can be run from the command line using curl, like this:
//   $ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/webpage
//   * "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//   * "dbowner" is the owner of the database being queried
//   * "dbname" is the name of the database being queried
func webpageHandler(w http.ResponseWriter, r *http.Request) {
	// Authenticate user and collect requested database details
	loggedInUser, dbOwner, dbName, apiKey, _, httpStatus, err := collectInfo(w, r)
	if err != nil {
		jsonErr(w, err.Error(), httpStatus)
		return
	}
	dbFolder := "/"

	// Make sure the API key has permission to run this function on the requested database
	err = permissionCheck(loggedInUser, apiKey, dbName, "webpage")
	if err != nil {
		jsonErr(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Return the database webUI URL to the user
	var z com.WebpageResponseContainer
	z.WebPage = "https://" + com.Conf.Web.ServerName + "/" + dbOwner + dbFolder + dbName
	jsonData, err := json.MarshalIndent(z, "", "  ")
	if err != nil {
		log.Printf("Error when JSON marshalling returned data in webpageHandler(): %v\n", err)
		jsonErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}
