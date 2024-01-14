package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"

	"github.com/gin-gonic/gin"
	sqlite "github.com/gwenn/gosqlite"
	com "github.com/sqlitebrowser/dbhub.io/common"
	"github.com/sqlitebrowser/dbhub.io/common/config"
)

// collectInfo is an internal function which xtracts the database owner, name, and commit ID from the request
// and checks the permissions
func collectInfo(c *gin.Context) (loggedInUser, dbOwner, dbName, commitID string, httpStatus int, err error) {
	// Get user name
	loggedInUser = c.MustGet("user").(string)

	// Extract the database owner name, database name, and (optional) commit ID for the database from the request
	dbOwner, dbName, commitID, err = com.GetFormODC(c.Request)
	if err != nil {
		httpStatus = http.StatusInternalServerError
		return
	}

	// Check if the user has access to the requested database
	// Check if the requested database exists
	exists, err := com.CheckDBPermissions(loggedInUser, dbOwner, dbName, false)
	if err != nil {
		httpStatus = http.StatusInternalServerError
		return
	}
	if !exists {
		httpStatus = http.StatusNotFound
		err = fmt.Errorf("Database does not exist, or user isn't authorised to access it")
		return
	}
	return
}

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
func branchesHandler(c *gin.Context) {
	// Do auth check, grab request info
	loggedInUser, dbOwner, dbName, _, httpStatus, err := collectInfo(c)
	if err != nil {
		c.JSON(httpStatus, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Record the api call in our backend database
	com.ApiCallLog(loggedInUser, dbOwner, dbName, "branches", c.Request.UserAgent())

	// If the database is a live database, we return an error message
	isLive, _, err := com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	if isLive {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "That database is a live database.  It doesn't have branches.",
		})
		return
	}

	// Retrieve the branch list for the database
	brList, err := com.BranchListResponse(dbOwner, dbName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Return the list as JSON
	c.JSON(http.StatusOK, brList)
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
func columnsHandler(c *gin.Context) {
	// Do auth check, grab request info
	loggedInUser, dbOwner, dbName, _, httpStatus, err := collectInfo(c)
	if err != nil {
		c.JSON(httpStatus, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Record the api call in our backend database
	com.ApiCallLog(loggedInUser, dbOwner, dbName, "columns", c.Request.UserAgent())

	// Extract the table name
	table, err := com.GetFormTable(c.Request, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Make sure a table name was provided
	if table == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Missing table name",
		})
		return
	}

	// Check if the database is a live database, and get the node/queue to send the request to
	isLive, liveNode, err := com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// If a live database has been uploaded but doesn't have a live node handling its requests, then error out as this
	// should never happen
	if isLive && liveNode == "" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "No job queue node available for request",
		})
		return
	}

	// If it's a standard database, process it locally.  Else send the query to our job queue backend
	var cols []sqlite.Column
	if !isLive {
		// Get Minio bucket and object id for the SQLite file
		bucket, id, _, err := com.MinioLocation(dbOwner, dbName, "", loggedInUser)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		// Sanity check
		if id == "" {
			// The requested database wasn't found, or the user doesn't have permission to access it
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Requested database not found",
			})
			return
		}

		// Retrieve the database from Minio, then open it
		var sdb *sqlite.Conn
		sdb, err = com.OpenSQLiteDatabase(bucket, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
		defer sdb.Close()

		// Verify the requested table or view we're about to query does exist
		tablesViews, err := com.TablesAndViews(sdb, dbName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
		tableOrViewFound := false
		for _, t := range tablesViews {
			if t == table {
				tableOrViewFound = true
			}
		}
		if !tableOrViewFound {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Provided table or view name doesn't exist in this database",
			})
			return
		}

		// Retrieve the list of columns for the table
		cols, err = sdb.Columns("", table)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
	} else {
		// Send the columns request to our job queue backend
		cols, _, err = com.LiveColumns(liveNode, loggedInUser, dbOwner, dbName, table)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
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
	c.JSON(200, jsonCols)
}

// commitsHandler returns the details of all commits for a database
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/commits
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbowner" is the owner of the database
//	* "dbname" is the name of the database
func commitsHandler(c *gin.Context) {
	// Do auth check, grab request info
	loggedInUser, dbOwner, dbName, _, httpStatus, err := collectInfo(c)
	if err != nil {
		c.JSON(httpStatus, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Record the api call in our backend database
	com.ApiCallLog(loggedInUser, dbOwner, dbName, "commits", c.Request.UserAgent())

	// If the database is a live database, we return an error message
	isLive, _, err := com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	if isLive {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "That database is a live database.  It doesn't have commits.",
		})
		return
	}

	// Retrieve the commits
	commits, err := com.GetCommitList(dbOwner, dbName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Return the tags as JSON
	c.JSON(200, commits)
}

// databasesHandler returns the list of databases in the requesting users account.
// If the new (optional) "live" boolean text field is set to true, then it will return the list of live
// databases.  Otherwise, it will return the list of standard databases.
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F live="true" https://api.dbhub.io/v1/databases
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "live" is whether to show Live databases, or standard ones
func databasesHandler(c *gin.Context) {
	loggedInUser := c.MustGet("user").(string)

	// Get "live" boolean value, if provided by the caller
	live, err := com.GetFormLive(c.Request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Record the api call in our backend database
	operation := "databases"
	if live {
		operation = "LIVE databases"
	}
	com.ApiCallLog(loggedInUser, "", "", operation, c.Request.UserAgent())

	// Retrieve the list of databases in the user account
	var databases []com.DBInfo
	if !live {
		// Get the list of standard databases
		databases, err = com.UserDBs(loggedInUser, com.DB_BOTH)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
	} else {
		// Get the list of live databases
		databases, err = com.LiveUserDBs(loggedInUser, com.DB_BOTH)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
	}

	// Extract just the database names
	var list []string
	for _, j := range databases {
		list = append(list, j.Database)
	}

	// Return the results
	c.JSON(200, list)
}

// deleteHandler deletes a database from the requesting users account
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/delete
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbname" is the name of the database
func deleteHandler(c *gin.Context) {
	loggedInUser := c.MustGet("user").(string)

	// Validate the database name
	dbName, err := com.GetDatabase(c.Request, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}
	dbOwner := loggedInUser

	// Record the api call in our backend database
	com.ApiCallLog(loggedInUser, dbOwner, dbName, "delete", c.Request.UserAgent())

	// Check if the database exists
	exists, err := com.CheckDBPermissions(loggedInUser, dbOwner, dbName, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Database does not exist, or user isn't authorised to access it",
		})
		return
	}

	// For a standard database, invalidate its memcache data
	isLive, liveNode, err := com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}
	if !isLive {
		err = com.InvalidateCacheEntry(loggedInUser, dbOwner, dbName, "") // Empty string indicates "for all versions"
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
	}

	// For a live database, delete it from both Minio and our job queue backend
	var bucket, id string
	if isLive {
		// Get the Minio bucket and object names for this database
		bucket, id, err = com.LiveGetMinioNames(loggedInUser, dbOwner, dbName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		// Delete the database from Minio
		err = com.MinioDeleteDatabase("API server", dbOwner, dbName, bucket, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		// Delete the database from our job queue backend
		err = com.LiveDelete(liveNode, loggedInUser, dbOwner, dbName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
	}

	// Delete the database in PostgreSQL
	err = com.DeleteDatabase(dbOwner, dbName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Return a "success" message
	c.JSON(200, gin.H{
		"status": "OK",
	})
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
func diffHandler(c *gin.Context) {
	loggedInUser := c.MustGet("user").(string)

	// Get merge strategy and parse value. Default to "none"
	merge := c.PostForm("merge")
	mergeStrategy := com.NoMerge
	if merge == "preserve_pk" {
		mergeStrategy = com.PreservePkMerge
	} else if merge == "new_pk" {
		mergeStrategy = com.NewPkMerge
	} else if merge != "" && merge != "none" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid merge strategy",
		})
		return
	}

	// Get include data parameter
	includeDataValue := c.PostForm("include_data")
	includeData := false
	if includeDataValue == "1" {
		includeData = true
	}

	// Retrieve owner, name, and commit ids
	oa := c.PostForm("dbowner_a")
	na := c.PostForm("dbname_a")
	ca := c.PostForm("commit_a")
	ob := c.PostForm("dbowner_b")
	nb := c.PostForm("dbname_b")
	cb := c.PostForm("commit_b")

	// If no primary database owner and name are given or if no commit ids are given, return
	if oa == "" || na == "" || ca == "" || cb == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Incomplete database details provided",
		})
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
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}
	dbOwnerB, err := url.QueryUnescape(ob)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}
	dbNameA, err := url.QueryUnescape(na)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}
	dbNameB, err := url.QueryUnescape(nb)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}
	err = com.ValidateUser(dbOwnerA)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}
	err = com.ValidateUser(dbOwnerB)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}
	err = com.ValidateDB(dbNameA)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}
	err = com.ValidateDB(dbNameB)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}
	err = com.ValidateCommitID(ca)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}
	err = com.ValidateCommitID(cb)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Record the api call in our backend database
	// Note - Lets not bother adding additional api logging fields just for the diff function at this stage
	com.ApiCallLog(loggedInUser, dbOwnerA, dbNameA, "diff", c.Request.UserAgent())

	// Check permissions of the first database
	allowed, err := com.CheckDBPermissions(loggedInUser, dbOwnerA, dbNameA, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	if !allowed {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Database not found",
		})
		return
	}

	// Check permissions of the second database
	allowed, err = com.CheckDBPermissions(loggedInUser, dbOwnerB, dbNameB, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	if !allowed {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Database not found",
		})
		return
	}

	// If either database is a live database, we return an error message
	isLive, _, err := com.CheckDBLive(dbOwnerA, dbNameA)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	if isLive {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("'%s/%s' is a live database.  It doesn't support diffs.", dbOwnerA, dbNameA),
		})
		return
	}
	isLive, _, err = com.CheckDBLive(dbOwnerB, dbNameB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	if isLive {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("'%s/%s' is a live database.  It doesn't support diffs.", dbOwnerB, dbNameB),
		})
		return
	}

	// Perform diff
	diffs, err := com.Diff(dbOwnerA, dbNameA, ca, dbOwnerB, dbNameB, cb, loggedInUser, mergeStrategy, includeData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Return the results
	c.JSON(200, diffs)
}

// downloadHandler returns the requested SQLite database file.
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" -OJ https://api.dbhub.io/v1/download
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbowner" is the owner of the database
//	* "dbname" is the name of the database
func downloadHandler(c *gin.Context) {
	// Authenticate user and collect requested database details
	loggedInUser, dbOwner, dbName, commitID, httpStatus, err := collectInfo(c)
	if err != nil {
		c.JSON(httpStatus, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Record the api call in our backend database
	com.ApiCallLog(loggedInUser, dbOwner, dbName, "download", c.Request.UserAgent())

	// Return the requested database to the user
	_, err = com.DownloadDatabase(c.Writer, c.Request, dbOwner, dbName, commitID, loggedInUser, "api")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
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
func executeHandler(c *gin.Context) {
	// Note - This code is useful for very specific debugging of incoming POST data, so there's no need to leave it uncommented at all times
	//if false {
	//	// Duplicate the request body in such a way that the existing functions don't need changing
	//	postData, err := io.ReadAll(r.Body)
	//	r.Body = io.NopCloser(bytes.NewBuffer(postData))
	//
	//	// Write the post data into a file
	//	tmpFileName := "/tmp/postdata.log"
	//	tmpFile, err := os.OpenFile(tmpFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	//	if err != nil {
	//		log.Printf("Couldn't open temp file '%s' for writing POST data: %v", tmpFileName, err)
	//	} else {
	//		fmt.Fprintf(tmpFile, "URL: '%s'\n", r.URL.Path)
	//		fmt.Fprintf(tmpFile, "POST DATA: '%s'\n\n", postData)
	//		defer tmpFile.Close()
	//	}
	//}

	loggedInUser := c.MustGet("user").(string)

	// Extract the database owner name, database name, and (optional) commit ID for the database from the request
	dbOwner, dbName, _, err := com.GetFormODC(c.Request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Record the api call in our backend database
	com.ApiCallLog(loggedInUser, dbOwner, dbName, "execute", c.Request.UserAgent())

	// Grab the incoming SQLite query
	rawInput := c.PostForm("sql")
	sql, err := com.CheckUnicode(rawInput, true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckDBPermissions(loggedInUser, dbOwner, dbName, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Database '%s/%s' doesn't exist", dbOwner, dbName),
		})
		return
	}

	// Check if the database is a live database, and get the node/queue to send the request to
	isLive, liveNode, err := com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Reject attempts to run Execute() on non-live databases
	if !isLive {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Execute() only runs on Live databases.  This is not a live database.",
		})
		return
	}

	// If a live database has been uploaded but doesn't have a live node handling its requests, then error out as this
	// should never happen
	if isLive && liveNode == "" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "No job queue node available for request",
		})
		return
	}

	// Send the SQL execution request to our job queue backend
	rowsChanged, err := com.LiveExecute(liveNode, loggedInUser, dbOwner, dbName, sql)
	if err != nil {
		log.Println(err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// The Execute() succeeded, so pass along the # of rows changed
	z := com.ExecuteResponseContainer{RowsChanged: rowsChanged, Status: "OK"}
	c.JSON(200, z)
}

// indexesHandler returns the details of all indexes in a SQLite database
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/indexes
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbowner" is the owner of the database
//	* "dbname" is the name of the database
func indexesHandler(c *gin.Context) {
	// Do auth check, grab request info
	loggedInUser, dbOwner, dbName, _, httpStatus, err := collectInfo(c)
	if err != nil {
		c.JSON(httpStatus, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Record the api call in our backend database
	com.ApiCallLog(loggedInUser, dbOwner, dbName, "indexes", c.Request.UserAgent())

	// Check if the database is a live database, and get the node/queue to send the request to
	isLive, liveNode, err := com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// If a live database has been uploaded but doesn't have a live node handling its requests, then error out as this
	// should never happen
	if isLive && liveNode == "" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "No job queue node available for request",
		})
		return
	}

	// If it's a standard database, process it locally.  Else send the query to our job queue backend
	var indexes []com.APIJSONIndex
	if !isLive {
		// Get Minio bucket and object id for the SQLite file
		bucket, id, _, err := com.MinioLocation(dbOwner, dbName, "", loggedInUser)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		// Sanity check
		if id == "" {
			// The requested database wasn't found, or the user doesn't have permission to access it
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Requested database not found",
			})
			return
		}

		// Retrieve the database from Minio, then open it
		var sdb *sqlite.Conn
		sdb, err = com.OpenSQLiteDatabase(bucket, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
		defer sdb.Close()

		// Retrieve the list of indexes
		var idx map[string]string
		idx, err = sdb.Indexes("")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
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
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": err.Error(),
				})
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
		// Send the indexes request to our job queue backend
		indexes, err = com.LiveIndexes(liveNode, loggedInUser, dbOwner, dbName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
	}

	// Return the results
	c.JSON(200, indexes)
}

// metadataHandler returns the commit, branch, release, tag and web page information for a database
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/metadata
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbowner" is the owner of the database
//	* "dbname" is the name of the database
func metadataHandler(c *gin.Context) {
	// Do auth check, grab request info
	loggedInUser, dbOwner, dbName, _, httpStatus, err := collectInfo(c)
	if err != nil {
		c.JSON(httpStatus, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Record the api call in our backend database
	com.ApiCallLog(loggedInUser, dbOwner, dbName, "metadata", c.Request.UserAgent())

	// If the database is a live database, we return an error message
	isLive, _, err := com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	if isLive {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "That database is a live database.  It doesn't support metadata.",
		})
		return
	}

	// Retrieve the metadata for the database
	meta, err := com.MetadataResponse(dbOwner, dbName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Return the list as JSON
	c.JSON(200, meta)
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
func queryHandler(c *gin.Context) {
	loggedInUser := c.MustGet("user").(string)

	// Extract the database owner name, database name, and (optional) commit ID for the database from the request
	dbOwner, dbName, commitID, err := com.GetFormODC(c.Request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Record the api call in our backend database
	com.ApiCallLog(loggedInUser, dbOwner, dbName, "query", c.Request.UserAgent())

	// Grab the incoming SQLite query
	rawInput := c.PostForm("sql")
	query, err := com.CheckUnicode(rawInput, true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckDBPermissions(loggedInUser, dbOwner, dbName, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("Database '%s/%s' doesn't exist", dbOwner, dbName),
		})
		return
	}

	// Check if the database is a live database, and get the node/queue to send the request to
	isLive, liveNode, err := com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// If a live database has been uploaded but doesn't have a live node handling its requests, then error out as this
	// should never happen
	if isLive && liveNode == "" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "No job queue node available for request",
		})
		return
	}

	// Run the query
	var data com.SQLiteRecordSet
	if !isLive {
		// Standard database
		data, err = com.SQLiteRunQueryDefensive(c.Writer, c.Request, com.QuerySourceAPI, dbOwner, dbName, commitID, loggedInUser, query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
	} else {
		// Send the query to the appropriate backend live node
		data, err = com.LiveQuery(liveNode, loggedInUser, dbOwner, dbName, query)
		if err != nil {
			log.Println(err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
	}

	// Return the results
	c.JSON(200, data.Records)
}

// releasesHandler returns the details of all releases for a database
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/releases
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbowner" is the owner of the database
//	* "dbname" is the name of the database
func releasesHandler(c *gin.Context) {
	// Do auth check, grab request info
	loggedInUser, dbOwner, dbName, _, httpStatus, err := collectInfo(c)
	if err != nil {
		c.JSON(httpStatus, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Record the api call in our backend database
	com.ApiCallLog(loggedInUser, dbOwner, dbName, "releases", c.Request.UserAgent())

	// If the database is a live database, we return an error message
	isLive, _, err := com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	if isLive {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "That database is a live database.  It doesn't support releases.",
		})
		return
	}

	// Retrieve the list of releases
	rels, err := com.GetReleases(dbOwner, dbName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Return the list as JSON
	c.JSON(200, rels)
}

// tablesHandler returns the list of tables in a SQLite database
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/tables
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbowner" is the owner of the database
//	* "dbname" is the name of the database
func tablesHandler(c *gin.Context) {
	// Do auth check, grab request info
	loggedInUser, dbOwner, dbName, _, httpStatus, err := collectInfo(c)
	if err != nil {
		c.JSON(httpStatus, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Record the api call in our backend database
	com.ApiCallLog(loggedInUser, dbOwner, dbName, "tables", c.Request.UserAgent())

	// Check if the database is a live database, and get the node/queue to send the request to
	isLive, liveNode, err := com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// If a live database has been uploaded but doesn't have a live node handling its requests, then error out as this
	// should never happen
	if isLive && liveNode == "" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "No job queue node available for request",
		})
		return
	}

	// If it's a standard database, process it locally.  Else send the query to our job queue backend
	var tables []string
	if !isLive {
		// Get Minio bucket and object id for the SQLite file
		bucket, id, _, err := com.MinioLocation(dbOwner, dbName, "", loggedInUser)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		// Sanity check
		if id == "" {
			// The requested database wasn't found, or the user doesn't have permission to access it
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Requested database not found",
			})
			return
		}

		// Retrieve the database from Minio, then open it
		var sdb *sqlite.Conn
		sdb, err = com.OpenSQLiteDatabase(bucket, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
		defer sdb.Close()

		// Retrieve the list of tables
		tables, err = com.Tables(sdb)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
	} else {
		// Send the tables request to our job queue backend
		tables, err = com.LiveTables(liveNode, loggedInUser, dbOwner, dbName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
	}

	// Return the results
	sort.Strings(tables)
	c.JSON(200, tables)
}

// tagsHandler returns the details of all tags for a database
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/tags
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbowner" is the owner of the database
//	* "dbname" is the name of the database
func tagsHandler(c *gin.Context) {
	// Do auth check, grab request info
	loggedInUser, dbOwner, dbName, _, httpStatus, err := collectInfo(c)
	if err != nil {
		c.JSON(httpStatus, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Record the api call in our backend database
	com.ApiCallLog(loggedInUser, dbOwner, dbName, "tags", c.Request.UserAgent())

	// If the database is a live database, we return an error message
	isLive, _, err := com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}
	if isLive {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "That database is a live database.  It doesn't support tags.",
		})
		return
	}

	// Retrieve the tags
	tags, err := com.GetTags(dbOwner, dbName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Return the tags as JSON
	c.JSON(200, tags)
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
func uploadHandler(c *gin.Context) {
	loggedInUser := c.MustGet("user").(string)

	// Set the maximum accepted database size for uploading
	oversizeAllowed := false
	for _, user := range config.Conf.UserMgmt.SizeOverrideUsers {
		if loggedInUser == user {
			oversizeAllowed = true
		}
	}
	if !oversizeAllowed {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, com.MaxDatabaseSize*1024*1024)
	}

	// Extract the database name and (optional) commit ID for the database from the request
	_, dbName, commitID, err := com.GetFormODC(c.Request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// The "public" user isn't allowed to make changes
	if loggedInUser == "public" {
		log.Printf("User from '%s' attempted to add a database using the public certificate", c.Request.RemoteAddr)
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "You're using the 'public' certificate, which isn't allowed to make changes on the server",
		})
		return
	}

	// Check whether the uploaded database is too large
	if !oversizeAllowed {
		if c.Request.ContentLength > (com.MaxDatabaseSize * 1024 * 1024) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("Database is too large. Maximum database upload size is %d MB, yours is %d MB", com.MaxDatabaseSize, c.Request.ContentLength/1024/1024),
			})
			log.Printf("'%s' attempted to upload an oversized database %d MB in size.  Limit is %d MB",
				loggedInUser, c.Request.ContentLength/1024/1024, com.MaxDatabaseSize)
			return
		}
	}

	// Get "live" boolean value, if provided by the caller
	live, err := com.GetFormLive(c.Request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Process the upload
	var httpStatus int
	var x map[string]string
	dbOwner := loggedInUser // We always use the API key / cert owner as the database owner for uploads
	if !live {
		x, httpStatus, err = com.UploadResponse(c.Writer, c.Request, loggedInUser, dbOwner, dbName, commitID, "api")
		if err != nil {
			c.JSON(httpStatus, gin.H{
				"error": err.Error(),
			})
			return
		}
	} else {
		// FIXME: The code below is grabbed from com.UploadResponse(), and is also very similar to the code in the
		//        webui uploadDataHandler().  May be able to refactor them.

		// Grab the uploaded file and form variables
		tempFile, err := c.FormFile("file")
		if err != nil && err.Error() != "http: no such file" {
			log.Printf("Uploading file failed: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("Something went wrong when grabbing the file data: '%s'", err.Error()),
			})
			return
		}
		if err != nil {
			if err.Error() == "http: no such file" {
				// Check for a 'file1' FormFile too, as some clients can't use 'file' (without a number) due to a design bug
				tempFile, err = c.FormFile("file1")
				if err != nil {
					log.Printf("Uploading file failed: %v", err)
					c.JSON(http.StatusBadRequest, gin.H{
						"error": fmt.Sprintf("Something went wrong when grabbing the file data: '%s'", err.Error()),
					})
					return
				}
			}
		}

		// If no database name was passed as a function argument, use the name given in the upload itself
		if dbName == "" {
			dbName = tempFile.Filename
		}

		// Validate the database name
		err = com.ValidateDB(dbName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return
		}

		// Check if the database exists already
		exists, err := com.CheckDBExists(loggedInUser, dbName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		// If the upload is a live database, but the database already exists, then abort the upload
		// TODO: Consider if we want the existing "force" flag to be useful here, to potentially allow overwriting a
		//       live database
		if exists && live {
			c.JSON(http.StatusConflict, gin.H{
				"error": "You're uploading a live database, but the same database name already exists. Delete that one first if you really want to overwrite it",
			})
			return
		}

		// Open uploaded file for reading
		src, err := tempFile.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to open uploaded file for reading",
			})
			return
		}
		defer src.Close()

		// Write the incoming database to a temporary file on disk, and sanity check it
		numBytes, tempDB, _, _, err := com.WriteDBtoDisk(loggedInUser, dbOwner, dbName, src)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
		defer os.Remove(tempDB.Name())

		// Rewind the internal cursor in the temporary file back to the start again
		var newOffset int64
		newOffset, err = tempDB.Seek(0, 0)
		if err != nil {
			log.Printf("Seeking on the temporary file (2nd time) failed: %s", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
		if newOffset != 0 {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Seeking to start of temporary database file didn't work",
			})
			return
		}

		// Store the database in Minio
		objectID, err := com.LiveStoreDatabaseMinio(tempDB, dbOwner, dbName, numBytes)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		// Log the successful database upload
		log.Printf("API Server: Username '%s' uploaded LIVE database '%s/%s', bytes: %v", loggedInUser,
			com.SanitiseLogString(dbOwner), com.SanitiseLogString(dbName), numBytes)

		// Send a request to the job queue to set up the database
		liveNode, err := com.LiveCreateDB(dbOwner, dbName, objectID)
		if err != nil {
			log.Println(err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		// Update PG, so it has a record of this database existing and knows the node/queue name for querying it
		err = com.LiveAddDatabasePG(dbOwner, dbName, objectID, liveNode, com.SetToPrivate)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		// Enable the watch flag for the uploader for this database
		err = com.ToggleDBWatch(dbOwner, dbOwner, dbName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		// Upload was successful, so we construct a fake commit ID then return a success message to the user
		x = make(map[string]string)
		x["commit_id"] = ""
		x["url"] = server + filepath.Join("/", dbOwner, dbName)
	}

	// Record the api call in our backend database
	operation := "upload"
	if live {
		operation = "LIVE upload"
	}
	com.ApiCallLog(loggedInUser, loggedInUser, dbName, operation, c.Request.UserAgent())

	// Construct the response message
	var ok bool
	var newCommit, newURL string
	if newCommit, ok = x["commit_id"]; !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Something went wrong when uploading the database, no commit ID was returned",
		})
		return
	}
	if newURL, ok = x["url"]; !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Something went wrong when uploading the database, no url was returned",
		})
		return
	}

	// Signal the successful database creation
	c.JSON(http.StatusCreated, gin.H{
		"commit": newCommit,
		"url": newURL,
	})
}

// viewsHandler returns the list of views in a SQLite database
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/views
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbowner" is the owner of the database being queried
//	* "dbname" is the name of the database being queried
func viewsHandler(c *gin.Context) {
	// Do auth check, grab request info
	loggedInUser, dbOwner, dbName, _, httpStatus, err := collectInfo(c)
	if err != nil {
		c.JSON(httpStatus, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Record the api call in our backend database
	com.ApiCallLog(loggedInUser, loggedInUser, dbName, "views", c.Request.UserAgent())

	// Check if the database is a live database, and get the node/queue to send the request to
	isLive, liveNode, err := com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// If a live database has been uploaded but doesn't have a live node handling its requests, then error out as this
	// should never happen
	if isLive && liveNode == "" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "No job queue node available for request",
		})
		return
	}

	// If it's a standard database, process it locally.  Else send the query to our job queue backend
	var views []string
	if !isLive {
		// Get Minio bucket and object id for the SQLite file
		bucket, id, _, err := com.MinioLocation(dbOwner, dbName, "", loggedInUser)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		// Sanity check
		if id == "" {
			// The requested database wasn't found, or the user doesn't have permission to access it
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Requested database not found",
			})
			return
		}

		// Retrieve the database from Minio, then open it
		var sdb *sqlite.Conn
		sdb, err = com.OpenSQLiteDatabase(bucket, id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
		defer sdb.Close()

		// Retrieve the list of views
		views, err = com.Views(sdb)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
	} else {
		// Send the views request to our job queue backend
		views, err = com.LiveViews(liveNode, loggedInUser, dbOwner, dbName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
	}

	// Return the results
	sort.Strings(views)
	c.JSON(200, views)
}

// webpageHandler returns the address of the database in the webUI.  eg. for web browsers
// This can be run from the command line using curl, like this:
//
//	$ curl -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" https://api.dbhub.io/v1/webpage
//	* "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//	* "dbowner" is the owner of the database being queried
//	* "dbname" is the name of the database being queried
func webpageHandler(c *gin.Context) {
	// Authenticate user and collect requested database details
	loggedInUser, dbOwner, dbName, _, httpStatus, err := collectInfo(c)
	if err != nil {
		c.JSON(httpStatus, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Record the api call in our backend database
	com.ApiCallLog(loggedInUser, dbOwner, dbName, "webpage", c.Request.UserAgent())

	// Return the database webUI URL to the user
	c.JSON(200, gin.H{
		"web_page": "https://" + config.Conf.Web.ServerName + "/" + dbOwner + "/" + dbName,
	})
}
