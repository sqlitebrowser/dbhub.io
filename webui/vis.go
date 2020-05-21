package main

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	sqlite "github.com/gwenn/gosqlite"
	com "github.com/sqlitebrowser/dbhub.io/common"
	gfm "github.com/sqlitebrowser/github_flavored_markdown"
)

func visualisePage(w http.ResponseWriter, r *http.Request) {
	pageName := "Visualise data page"

	var pageData struct {
		Auth0         com.Auth0Set
		Data          com.SQLiteRecordSet
		DB            com.SQLiteDBinfo
		Meta          com.MetaInfo
		MyStar        bool
		MyWatch       bool
		ParamsGiven   bool
		DataGiven     bool
		ChartType     string
		XAxisTable    string
		XAxisCol      string
		XAxisColNames []string
		YAxisTable    string
		YAxisCol      string
		YAxisColNames []string
		AggType       string
		JoinType      string
		JoinXCol      string
		JoinYCol      string
		Records       []com.VisRowV1
	}

	// Retrieve the database owner & name
	// TODO: Add folder support
	dbFolder := "/"
	dbOwner, dbName, err := com.GetOD(1, r) // 1 = Ignore "/discuss/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Validate the supplied information
	if dbOwner == "" || dbName == "" {
		errorPage(w, r, http.StatusBadRequest, "Missing database owner or database name")
		return
	}

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "dbhub-user")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Check if a specific database commit ID was given
	commitID, err := com.GetFormCommit(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Invalid database commit ID")
		return
	}

	// If any table names were supplied, validate them
	xTable := r.FormValue("xtable")
	if xTable != "" {
		err = com.ValidatePGTable(xTable)
		if err != nil {
			// Validation failed, so don't pass on the table name
			log.Printf("%s: Validation failed for X axis table name: %s", pageName, err)
			xTable = ""
		}
	}
	yTable := r.FormValue("ytable")
	if yTable != "" {
		err = com.ValidatePGTable(yTable)
		if err != nil {
			// Validation failed, so don't pass on the table name
			log.Printf("%s: Validation failed for Y axis table name: %s", pageName, err)
			yTable = ""
		}
	}

	// Check if a branch name was requested
	branchName, err := com.GetFormBranch(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Validation failed for branch name")
		return
	}

	// Check if a named tag was requested
	tagName, err := com.GetFormTag(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Validation failed for tag name")
		return
	}

	// Check if a specific release was requested
	releaseName := r.FormValue("release")
	if releaseName != "" {
		err = com.ValidateBranchName(releaseName)
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, "Validation failed for release name")
			return
		}
	}

	// Check if the database exists and the user has access to view it
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", dbOwner, dbFolder,
			dbName))
		return
	}

	// * Execution can only get here if the user has access to the requested database *

	// Increment the view counter for the database (excluding people viewing their own databases)
	if strings.ToLower(loggedInUser) != strings.ToLower(dbOwner) {
		err = com.IncrementViewCount(dbOwner, dbFolder, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// If a specific commit was requested, make sure it exists in the database commit history
	if commitID != "" {
		commitList, err := com.GetCommitList(dbOwner, dbFolder, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if _, ok := commitList[commitID]; !ok {
			// The requested commit isn't one in the database commit history so error out
			errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Unknown commit for database '%s%s%s'", dbOwner,
				dbFolder, dbName))
			return
		}
	}

	// If a specific release was requested, and no commit ID was given, retrieve the commit ID matching the release
	if commitID == "" && releaseName != "" {
		releases, err := com.GetReleases(dbOwner, dbFolder, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve releases for database")
			return
		}
		rls, ok := releases[releaseName]
		if !ok {
			errorPage(w, r, http.StatusInternalServerError, "Unknown release requested for this database")
			return
		}
		commitID = rls.Commit
	}

	// Load the branch info for the database
	branchHeads, err := com.GetBranches(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve branch information for database")
		return
	}

	// If a specific branch was requested and no commit ID was given, use the latest commit for the branch
	if commitID == "" && branchName != "" {
		c, ok := branchHeads[branchName]
		if !ok {
			errorPage(w, r, http.StatusInternalServerError, "Unknown branch requested for this database")
			return
		}
		commitID = c.Commit
	}

	// If a specific tag was requested, and no commit ID was given, retrieve the commit ID matching the tag
	if commitID == "" && tagName != "" {
		tags, err := com.GetTags(dbOwner, dbFolder, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve tags for database")
			return
		}
		tg, ok := tags[tagName]
		if !ok {
			errorPage(w, r, http.StatusInternalServerError, "Unknown tag requested for this database")
			return
		}
		commitID = tg.Commit
	}

	// If we still haven't determined the required commit ID, use the head commit of the default branch
	if commitID == "" {
		commitID, err = com.DefaultCommit(dbOwner, dbFolder, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Retrieve the database details
	err = com.DBDetails(&pageData.DB, loggedInUser, dbOwner, dbFolder, dbName, commitID)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Get the latest discussion and merge request count directly from PG, skipping the ones (incorrectly) stored in memcache
	currentDisc, currentMRs, err := com.GetDiscussionAndMRCount(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// If an sha256 was in the licence field, retrieve it's friendly name and url for displaying
	licSHA := pageData.DB.Info.DBEntry.LicenceSHA
	if licSHA != "" {
		pageData.DB.Info.Licence, pageData.DB.Info.LicenceURL, err = com.GetLicenceInfoFromSha256(dbOwner, licSHA)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		pageData.DB.Info.Licence = "Not specified"
	}

	// Check if the database was starred by the logged in user
	myStar, err := com.CheckDBStarred(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve database star status")
		return
	}

	// Check if the database is being watched by the logged in user
	myWatch, err := com.CheckDBWatched(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve database watch status")
		return
	}

	// If a specific table wasn't requested, use the user specified default (if present)
	if xTable == "" || yTable == "" {
		// Ensure the default table name validates.  This catches a case where a database was uploaded with an invalid
		// table name and somehow because selected as the default
		a := pageData.DB.Info.DefaultTable
		if a != "" {
			err = com.ValidatePGTable(a)
			if err == nil {
				// The database table name is acceptable, so use it
				if xTable == "" {
					xTable = pageData.DB.Info.DefaultTable
				}
				if yTable == "" {
					yTable = pageData.DB.Info.DefaultTable
				}
			}
		}
	}

	// Retrieve the details for the logged in user
	var avatarURL string
	if loggedInUser != "" {
		ur, err := com.User(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if ur.AvatarURL != "" {
			avatarURL = ur.AvatarURL + "&s=48"
		}
	}

	// TODO: Retrieve the cached SQLite table and column names from memcached.
	//       Keyed to something like username+dbname+commitID+tablename
	//       This can be done at a later point, if it turns out people are using the visualisation feature :)

	// Get a handle for the database object
	sdb, err := com.OpenSQLiteDatabase(pageData.DB.Info.DBEntry.Sha256[:com.MinioFolderChars],
		pageData.DB.Info.DBEntry.Sha256[com.MinioFolderChars:])
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Close the SQLite database and delete the temp file
	defer func() {
		sdb.Close()
	}()

	// Retrieve the list of tables and views in the database
	tables, err := com.Tables(sdb, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.DB.Info.Tables = tables

	// If a specific table was requested, check that it's present
	if xTable != "" || yTable != "" {
		// Check the requested table is present
		xTablePresent := false
		yTablePresent := false
		for _, tbl := range tables {
			if tbl == xTable {
				xTablePresent = true
			}
			if tbl == yTable {
				yTablePresent = true
			}
		}
		if xTablePresent == false {
			// The requested X axis table doesn't exist in the database, so pick one of the tables that is
			for _, t := range tables {
				err = com.ValidatePGTable(t)
				if err == nil {
					// Validation passed, so use this table
					xTable = t
					pageData.DB.Info.DefaultTable = t
					break
				}
			}
		}
		if yTablePresent == false {
			// The requested Y axis table doesn't exist in the database, so pick one of the tables that is
			for _, t := range tables {
				err = com.ValidatePGTable(t)
				if err == nil {
					// Validation passed, so use this table
					yTable = t
					pageData.DB.Info.DefaultTable = t
					break
				}
			}
		}
	}

	// If specific tables weren't requested, use the first table in the database that passes validation
	if xTable == "" {
		for _, i := range pageData.DB.Info.Tables {
			if i != "" {
				err = com.ValidatePGTable(i)
				if err == nil {
					// The database table name is acceptable, so use it
					xTable = i
					break
				}
			}
		}
	}
	if yTable == "" {
		for _, i := range pageData.DB.Info.Tables {
			if i != "" {
				err = com.ValidatePGTable(i)
				if err == nil {
					// The database table name is acceptable, so use it
					yTable = i
					break
				}
			}
		}
	}

	// Validate the table names, just to be careful
	if xTable != "" {
		err = com.ValidatePGTable(xTable)
		if err != nil {
			// Validation failed, so don't pass on the table name

			// If the failed table name is "{{ db.Tablename }}", don't bother logging it.  It's just a search
			// bot picking up AngularJS in a string and doing a request with it
			if xTable != "{{ db.Tablename }}" {
				log.Printf("%s: Validation failed for table name: '%s': %s", pageName, xTable, err)
			}
			errorPage(w, r, http.StatusBadRequest, "Validation failed for X axis table name")
			return
		}
	}
	if yTable != "" {
		err = com.ValidatePGTable(yTable)
		if err != nil {
			// Validation failed, so don't pass on the table name

			// If the failed table name is "{{ db.Tablename }}", don't bother logging it.  It's just a search
			// bot picking up AngularJS in a string and doing a request with it
			if yTable != "{{ db.Tablename }}" {
				log.Printf("%s: Validation failed for table name: '%s': %s", pageName, yTable, err)
			}
			errorPage(w, r, http.StatusBadRequest, "Validation failed for Y axis table name")
			return
		}
	}

	// Retrieve the SQLite X axis column names
	pageData.XAxisTable = xTable
	xColList, err := sdb.Columns("", xTable)
	if err != nil {
		log.Printf("Error when reading column names for table '%s': %v\n", xTable, err.Error())
		errorPage(w, r, http.StatusInternalServerError, "Error when reading from the database")
		return
	}
	var c []string
	for _, j := range xColList {
		c = append(c, j.Name)
	}
	pageData.XAxisColNames = c

	// Retrieve the SQLite Y axis column names
	pageData.YAxisTable = yTable
	yColList, err := sdb.Columns("", yTable)
	if err != nil {
		log.Printf("Error when reading column names for table '%s': %v\n", yTable, err.Error())
		errorPage(w, r, http.StatusInternalServerError, "Error when reading from the database")
		return
	}
	var c2 []string
	for _, j := range yColList {
		c2 = append(c2, j.Name)
	}
	pageData.YAxisColNames = c2

	// Retrieve the default visualisation parameters for this database, if they've been set
	params, ok, err := com.GetVisualisationParams(dbOwner, dbFolder, dbName, "default")
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// If saved parameters were found, pass them through to the web page
	if ok {
		pageData.ParamsGiven = true
		switch params.ChartType {
		case "hbc":
			pageData.ChartType = "Horizontal bar chart"
		case "vbc":
			pageData.ChartType = "Vertical bar chart"
		case "lc":
			pageData.ChartType = "Line chart"
		case "pie":
			pageData.ChartType = "Pie chart"
		default:
			pageData.ChartType = "Vertical bar chart"
		}
		pageData.XAxisTable = params.XAxisTable
		pageData.XAxisCol = params.XAXisColumn
		pageData.YAxisTable = params.YAxisTable
		pageData.YAxisCol = params.YAXisColumn
		switch params.AggType {
		case 0:
			pageData.AggType = "---"
		case 1:
			pageData.AggType = "avg"
		case 2:
			pageData.AggType = "count"
		case 3:
			pageData.AggType = "group_concat"
		case 4:
			pageData.AggType = "max"
		case 5:
			pageData.AggType = "min"
		case 6:
			pageData.AggType = "sum"
		case 7:
			pageData.AggType = "total"
		default:
			errorPage(w, r, http.StatusInternalServerError, "Unknown aggregate type returned from database")
			log.Printf("Unknown aggregate type: %v\n", params.JoinType)
			return
		}
		switch params.JoinType {
		case 0:
			pageData.JoinType = "---"
		case 1:
			pageData.JoinType = "INNER JOIN"
		case 2:
			pageData.JoinType = "LEFT OUTER JOIN"
		case 3:
			pageData.JoinType = "CROSS JOIN"
		default:
			errorPage(w, r, http.StatusInternalServerError, "Unknown join type returned from database")
			log.Printf("Unknown JOIN type: %v\n", params.JoinType)
			return
		}
		pageData.JoinXCol = params.JoinXCol
		pageData.JoinYCol = params.JoinYCol

		// Retrieve the saved data for this visualisation too, if it's available
		hash := visHash(dbOwner, dbFolder, dbName, commitID, "default", params)
		data, ok, err := com.GetVisualisationData(dbOwner, dbFolder, dbName, commitID, hash)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if ok {
			pageData.Records = data
			pageData.DataGiven = true
		}
	}

	// Retrieve correctly capitalised username for the user
	usr, err := com.User(dbOwner)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.Meta.Owner = usr.Username

	// Ensure the correct Avatar URL is displayed
	pageData.Meta.AvatarURL = avatarURL

	// Retrieve the status updates count for the logged in user
	if loggedInUser != "" {
		pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Fill out various metadata fields
	pageData.Meta.Database = dbName
	pageData.Meta.Server = com.Conf.Web.ServerName
	pageData.Meta.Title = fmt.Sprintf("vis - %s %s %s", dbOwner, dbFolder, dbName)

	// Retrieve default branch name details
	if branchName == "" {
		branchName, err = com.GetDefaultBranchName(dbOwner, dbFolder, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, "Error retrieving default branch name")
			return
		}
	}

	// Fill out the branch info
	pageData.DB.Info.BranchList = []string{}
	if branchName != "" {
		// If a specific branch was requested, ensure it's the first entry of the drop down
		pageData.DB.Info.BranchList = append(pageData.DB.Info.BranchList, branchName)
	}
	for i := range branchHeads {
		if i != branchName {
			err = com.ValidateBranchName(i)
			if err == nil {
				pageData.DB.Info.BranchList = append(pageData.DB.Info.BranchList, i)
			}
		}
	}

	// Check for duplicate branch names in the returned list, and log the problem so an admin can investigate
	bCheck := map[string]struct{}{}
	for _, j := range pageData.DB.Info.BranchList {
		_, ok := bCheck[j]
		if !ok {
			// The branch name value isn't in the map already, so add it
			bCheck[j] = struct{}{}
		} else {
			// This branch name is already in the map.  Duplicate detected.  This shouldn't happen
			log.Printf("Duplicate branch name '%s' detected in returned branch list for database '%s%s%s', "+
				"logged in user '%s'", j, dbOwner, dbFolder, dbName, loggedInUser)
		}
	}

	pageData.DB.Info.Branch = branchName
	pageData.DB.Info.Commits = branchHeads[branchName].CommitCount

	// Retrieve the "forked from" information
	frkOwn, frkFol, frkDB, frkDel, err := com.ForkedFrom(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failure")
		return
	}
	pageData.Meta.ForkOwner = frkOwn
	pageData.Meta.ForkFolder = frkFol
	pageData.Meta.ForkDatabase = frkDB
	pageData.Meta.ForkDeleted = frkDel

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Update database star and watch status for the logged in user
	pageData.MyStar = myStar
	pageData.MyWatch = myWatch

	// Render the full description as markdown
	pageData.DB.Info.FullDesc = string(gfm.Markdown([]byte(pageData.DB.Info.FullDesc)))

	// Restore the correct discussion and MR count
	pageData.DB.Info.Discussions = currentDisc
	pageData.DB.Info.MRs = currentMRs

	// Render the visualisation page
	t := tmpl.Lookup("visualisePage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Executes a custom SQLite SELECT query.
func visExecuteSQLHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "dbhub-user")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
	}

	// Retrieve user, database, and commit ID
	dbOwner, dbName, commitID, err := com.GetODC(2, r) // 1 = Ignore "/x/vis/" at the start of the URL
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	dbFolder := "/"

	// Grab the incoming SQLite query
	rawInput := r.FormValue("sql")
	decoded, err := base64.StdEncoding.DecodeString(rawInput)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error when decoding query: '%s'", err)
		return
	}

	// Ensure the decoded string is valid UTF-8
	if !utf8.Valid(decoded) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "SQL string contains invalid characters: '%v'", err)
		return
	}

	// Check for the presence of unicode control characters and similar in the decoded string
	invalidChar := false
	decodedStr := string(decoded)
	for _, j := range decodedStr {
		if unicode.IsControl(j) || unicode.Is(unicode.C, j) {
			if j != 10 { // 10 == new line, which is safe to allow.  Everything else should (probably) raise an error
				invalidChar = true
			}
		}
	}
	if invalidChar {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "SQL string contains invalid characters: '%v'", err)
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err.Error())
		return
	}
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Database '%s%s%s' doesn't exist", dbOwner, dbFolder, dbName)
		return
	}

	// Retrieve the SQLite database from Minio (also doing appropriate permission/access checking)
	sdb, err := com.OpenSQLiteDatabaseDefensive(w, r, dbOwner, dbFolder, dbName, commitID, loggedInUser)
	if err != nil {
		// The return handled was already done in OpenSQLiteDatabaseDefensive()
		return
	}

	// Automatically close the SQLite database when this function finishes
	defer func() {
		sdb.Close()
	}()

	// TODO: Finish opening the database in defensive mode: https://www.sqlite.org/security.html

	// Execute the SQLite select query (or queries)
	var results []com.VisRowV1
	results, err = com.RunUserVisQuery(sdb, decodedStr)
	if err != nil {
		// Some kind of error when running the visualisation query
		log.Printf("Error occurred when running visualisation query '%s%s%s', commit '%s': %s\n", dbOwner,
			dbFolder, dbName, commitID, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return
	}

	// Return the results
	jsonResponse, err := json.Marshal(results)
	if err != nil {
		log.Println(err)
		return
	}
	fmt.Fprintf(w, "%s", jsonResponse)
}

// Calculate the hash string for saving or retrieving any visualisation data
func visHash(dbOwner, dbFolder, dbName, commitID, visName string, params com.VisParamsV1) string {
	z := md5.Sum([]byte(fmt.Sprintf("%s/%s/%s/%s/%s/%s/%s/%s/%d/%d/%s/%s/%s", strings.ToLower(dbOwner), dbFolder,
		dbName, commitID, params.XAxisTable, params.XAXisColumn, params.YAxisTable, params.YAXisColumn, params.AggType,
		params.JoinType, params.JoinXCol, params.JoinYCol, visName)))
	return hex.EncodeToString(z[:])
}

// This function handles requests for database visualisation data
func visRequestHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve user, database, and commit ID
	dbOwner, dbName, commitID, err := com.GetODC(2, r) // 1 = Ignore "/x/vis/" at the start of the URL
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	dbFolder := "/"

	// Extract X axis, Y axis, and aggregate type variables if present
	xTable := r.FormValue("xtable")
	xAxis := r.FormValue("xaxis")
	yTable := r.FormValue("ytable")
	yAxis := r.FormValue("yaxis")
	aggTypeStr := r.FormValue("agg")
	joinTypeStr := r.FormValue("join")
	joinXCol := r.FormValue("joinxcol")
	joinYCol := r.FormValue("joinycol")

	// Ensure minimum viable parameters are present
	if xTable == "" || xAxis == "" || yTable == "" || yAxis == "" || aggTypeStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Validate the X axis table name
	if xTable != "" {
		err := com.ValidatePGTable(xTable)
		if err != nil {
			msg := fmt.Sprintf("Invalid table name '%s' for X Axis", xTable)
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "%s", msg)
			return
		}
	}

	// Validate the Y axis table name
	if yTable != "" {
		err := com.ValidatePGTable(yTable)
		if err != nil {
			msg := fmt.Sprintf("Invalid table name '%s' for Y Axis", yTable)
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "%s", msg)
			return
		}
	}

	// Validate the aggregate type
	aggType := 0
	switch aggTypeStr {
	case "---":
		aggType = 0
	case "avg":
		aggType = 1
	case "count":
		aggType = 2
	case "group_concat":
		aggType = 3
	case "max":
		aggType = 4
	case "min":
		aggType = 5
	case "sum":
		aggType = 6
	case "total":
		aggType = 7
	default:
		log.Println("Unknown aggregate type requested")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", "Unknown aggregate type requested")
		return
	}

	// Validate the join type
	joinType := 0
	switch joinTypeStr {
	case "":
	case "INNER JOIN":
		joinType = 1
	case "LEFT OUTER JOIN":
		joinType = 2
	case "CROSS JOIN":
		joinType = 3
	default:
		log.Printf("Unknown join type requested: '%v'", joinTypeStr)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", "Unknown join type requested")
		return
	}

	// Validate the X axis field name
	err = com.ValidateFieldName(xAxis)
	if err != nil {
		log.Printf("Validation failed on requested X axis field name '%v': %v\n", xAxis, err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Validate the Y axis field name
	err = com.ValidateFieldName(yAxis)
	if err != nil {
		log.Printf("Validation failed on requested Y axis field name '%v': %v\n", yAxis, err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Validate the join columns
	if joinXCol != "" {
		err = com.ValidateFieldName(joinXCol)
		if err != nil {
			log.Printf("Validation failed on requested JOIN column field name '%v': %v\n", joinXCol, err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	if joinYCol != "" {
		err = com.ValidateFieldName(joinYCol)
		if err != nil {
			log.Printf("Validation failed on requested JOIN column field name '%v': %v\n", joinYCol, err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "dbhub-user")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
	}

	// Retrieve the SQLite database from Minio (also doing appropriate permission/access checking)
	sdb, err := com.OpenSQLiteDatabaseDefensive(w, r, dbOwner, dbFolder, dbName, commitID, loggedInUser)
	if err != nil {
		// The return handled was already done in OpenSQLiteDatabaseDefensive()
		return
	}

	// Automatically close the SQLite database when this function finishes
	defer func() {
		sdb.Close()
	}()

	// Retrieve the list of tables and views in the database
	tables, err := sdb.Tables("")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Internal error occurred when retrieving SQLite table and view names")
		return
	}

	// Verify the desired tables and columns exist
	err = visVerifyTablesAndColumns(w, r, sdb, tables, xTable, xAxis, yTable, yAxis, joinXCol, joinYCol, joinType)
	if err != nil {
		// The return handling was already done in visVerifyTablesAndColumns()
		return
	}

	// Run the SQLite visualisation query
	vParams := com.VisParamsV1{
		XAxisTable:  xTable,
		XAXisColumn: xAxis,
		YAxisTable:  yTable,
		YAXisColumn: yAxis,
		AggType:     aggType,
		JoinType:    joinType,
		JoinXCol:    joinXCol,
		JoinYCol:    joinYCol,
	}
	visRows, err := com.RunSQLiteVisQuery(sdb, vParams)
	if err != nil {
		// Some kind of error when running the visualisation query
		log.Printf("Error occurred when running visualisation query '%s%s%s', commit '%s': %s\n", dbOwner,
			dbFolder, dbName, commitID, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return
	}

	// Format the output using json.MarshalIndent() for nice looking output
	jsonResponse, err := json.MarshalIndent(visRows, "", " ")
	if err != nil {
		log.Println(err)
		return
	}
	fmt.Fprintf(w, "%s", jsonResponse)
}

// This function handles requests to save the database visualisation parameters
func visSaveRequestHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve user, database, table, and commit ID
	dbOwner, dbName, commitID, err := com.GetODC(2, r) // 1 = Ignore "/x/vis/" at the start of the URL
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	dbFolder := "/"

	// Extract X axis, Y axis, and aggregate type variables if present
	chartType := r.FormValue("charttype")
	xTable := r.FormValue("xtable")
	xAxis := r.FormValue("xaxis")
	yTable := r.FormValue("ytable")
	yAxis := r.FormValue("yaxis")
	aggTypeStr := r.FormValue("agg")
	visName := r.FormValue("visname")
	joinTypeStr := r.FormValue("join")
	joinXCol := r.FormValue("joinxcol")
	joinYCol := r.FormValue("joinycol")

	// Ensure minimum viable parameters are present
	if chartType == "" || xTable == "" || xAxis == "" || yTable == "" || yAxis == "" || visName == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	//Ensure only valid chart types are accepted
	if chartType != "hbc" && chartType != "vbc" && chartType != "lc" && chartType != "pie" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Unknown chart type")
		return
	}

	// Validate the X axis table name
	if xTable != "" {
		err := com.ValidatePGTable(xTable)
		if err != nil {
			msg := fmt.Sprintf("Invalid table name '%s' for X Axis", xTable)
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "%s", msg)
			return
		}
	}

	// Validate the Y axis table name
	if yTable != "" {
		err := com.ValidatePGTable(yTable)
		if err != nil {
			msg := fmt.Sprintf("Invalid table name '%s' for Y Axis", yTable)
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "%s", msg)
			return
		}
	}

	// Parse the aggregation type
	aggType := 0
	switch aggTypeStr {
	case "---":
		aggType = 0
	case "avg":
		aggType = 1
	case "count":
		aggType = 2
	case "group_concat":
		aggType = 3
	case "max":
		aggType = 4
	case "min":
		aggType = 5
	case "sum":
		aggType = 6
	case "total":
		aggType = 7
	default:
		log.Println("Unknown aggregate type requested")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Validate the join type
	joinType := 0
	switch joinTypeStr {
	case "":
	case "INNER JOIN":
		joinType = 1
	case "LEFT OUTER JOIN":
		joinType = 2
	case "CROSS JOIN":
		joinType = 3
	default:
		log.Println("Unknown join type requested")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", "Unknown join type requested")
		return
	}

	// Validate the join columns
	if joinXCol != "" {
		err = com.ValidateFieldName(joinXCol)
		if err != nil {
			log.Printf("Validation failed on requested JOIN column field name '%v': %v\n", joinXCol, err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	if joinYCol != "" {
		err = com.ValidateFieldName(joinYCol)
		if err != nil {
			log.Printf("Validation failed on requested JOIN column field name '%v': %v\n", joinYCol, err.Error())
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	// Validate the X axis field name
	err = com.ValidateFieldName(xAxis)
	if err != nil {
		log.Printf("Validation failed on requested X axis field name '%v': %v\n", xAxis, err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Validate the Y axis field name
	err = com.ValidateFieldName(yAxis)
	if err != nil {
		log.Printf("Validation failed on requested Y axis field name '%v': %v\n", yAxis, err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Initial sanity check of the visualisation name
	// TODO: We'll probably need to figure out a better validation character set than the fieldname one
	//       Something along the lines of "all valid unicode letters", excluding things like control characters and
	//       any of the special characters the SQLite tokeniser recognises:
	//           https://github.com/sqlite/sqlite/blob/5a8cd2e40ce5287e638f77d4922068dbf7ba7e03/src/tokenize.c#L21-L57
	//       ICU also has a StringPrep page, which looks to have info on potential ways to check for Unicode control
	//       characters and similar: http://userguide.icu-project.org/strings/stringprep
	//       The outdated goodsign/icu project may be useful too: https://github.com/goodsign/icu
	err = com.ValidateFieldName(visName)
	if err != nil {
		log.Printf("Validation failed on requested visualisation name '%v': %v\n", visName, err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	validSession := false
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "dbhub-user")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "You need to be logged in")
		return
	}

	// Make sure the save request is coming from the database owner
	if loggedInUser != dbOwner {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "Only the database owner is allowed to save a visualisation (at least for now)")
		return
	}

	// Retrieve the SQLite database from Minio (also doing appropriate permission/access checking)
	sdb, err := com.OpenSQLiteDatabaseDefensive(w, r, dbOwner, dbFolder, dbName, commitID, loggedInUser)
	if err != nil {
		// The return handling was already done in OpenSQLiteDatabaseDefensive()
		return
	}

	// Automatically close the SQLite database when this function finishes
	defer func() {
		sdb.Close()
	}()

	// Retrieve the list of tables and views in the SQLite database
	tables, err := sdb.Tables("")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Internal error occurred when retrieving SQLite table and view names")
		return
	}

	// Verify the desired tables and columns exist
	err = visVerifyTablesAndColumns(w, r, sdb, tables, xTable, xAxis, yTable, yAxis, joinXCol, joinYCol, joinType)
	if err != nil {
		// The return handling was already done in visVerifyTablesAndColumns()
		return
	}

	// Retrieve the visualisation query result, so we can save that too
	vParams := com.VisParamsV1{
		ChartType:   chartType,
		XAxisTable:  xTable,
		XAXisColumn: xAxis,
		YAxisTable:  yTable,
		YAXisColumn: yAxis,
		AggType:     aggType,
		JoinType:    joinType,
		JoinXCol:    joinXCol,
		JoinYCol:    joinYCol,
	}
	visData, err := com.RunSQLiteVisQuery(sdb, vParams)
	if err != nil {
		// Some kind of error when running the visualisation query
		log.Printf("Error occurred when running visualisation query '%s%s%s', commit '%s': %s\n", dbOwner,
			dbFolder, dbName, commitID, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// If the # of rows returned from the query is 0, let the user know + don't save
	if len(visData) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Query returned no result")
		return
	}

	// Save the SQLite visualisation parameters
	err = com.VisualisationSaveParams(dbOwner, dbFolder, dbName, visName, vParams)
	if err != nil {
		log.Printf("Error occurred when saving visualisation '%s' for' '%s%s%s', commit '%s': %s\n", visName,
			dbOwner, dbFolder, dbName, commitID, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Save the SQLite visualisation data
	hash := visHash(dbOwner, dbFolder, dbName, commitID, visName, vParams)
	err = com.VisualisationSaveData(dbOwner, dbFolder, dbName, commitID, hash, visData)
	if err != nil {
		log.Printf("Error occurred when saving visualisation '%s' for' '%s%s%s', commit '%s': %s\n", visName,
			dbOwner, dbFolder, dbName, commitID, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Save succeeded
	w.WriteHeader(http.StatusOK)
}

// Verify the desired tables and columns exist in the given SQLite database
func visVerifyTablesAndColumns(w http.ResponseWriter, r *http.Request, sdb *sqlite.Conn, tables []string, xTable, xAxis,
	yTable, yAxis, joinXCol, joinYCol string, joinType int) (err error) {
	// Check the desired tables exist
	xTablePresent := false
	yTablePresent := false
	for _, tableName := range tables {
		if xTable == tableName {
			xTablePresent = true
		}
		if yTable == tableName {
			yTablePresent = true
		}
	}
	if xTablePresent == false {
		// The requested X axis table doesn't exist
		err = fmt.Errorf("The requested X axis table '%v' doesn't exist in the database", xAxis)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, err.Error())
		return
	}
	if yTablePresent == false {
		// The requested Y axis table doesn't exist
		err = fmt.Errorf("The requested Y axis table '%v' doesn't exist in the database", yAxis)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, err.Error())
		return
	}

	// Verify the X axis column exists
	var xColList []sqlite.Column
	xColList, err = sdb.Columns("", xTable)
	if err != nil {
		log.Printf("Error when reading column names for table '%s': %v\n", xTable, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	xColExists := false
	for _, j := range xColList {
		if j.Name == xAxis {
			xColExists = true
		}
	}
	if xColExists == false {
		// The requested X axis column doesn't exist
		err = fmt.Errorf("Requested X axis column '%s' doesn't exist in table: '%v'", xAxis, xTable)
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, err.Error())
		return
	}

	// Verify the Y axis column exists
	var yColList []sqlite.Column
	yColList, err = sdb.Columns("", yTable)
	if err != nil {
		log.Printf("Error when reading column names for table '%s': %v\n", yTable, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	yColExists := false
	for _, j := range yColList {
		if j.Name == yAxis {
			yColExists = true
		}
	}
	if yColExists == false {
		// The requested Y axis column doesn't exist
		err = fmt.Errorf("Requested Y axis column '%s' doesn't exist in table: '%v'\n", yAxis, yTable)
		log.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, err.Error())
		return
	}

	// Verify the join columns exist
	if (xTable != yTable) && (joinType == 1 || joinType == 2) {
		joinXColExists := false
		for _, j := range xColList {
			if j.Name == joinXCol {
				joinXColExists = true
			}
		}
		if joinXColExists == false {
			// The requested X axis join column doesn't exist
			err = fmt.Errorf("Requested X axis join column '%s' doesn't exist in table: '%v'", joinXCol, xTable)
			log.Println(err)
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, err.Error())
			return
		}

		joinYColExists := false
		for _, j := range yColList {
			if j.Name == joinYCol {
				joinYColExists = true
			}
		}
		if joinYColExists == false {
			// The requested Y axis join column doesn't exist
			err = fmt.Errorf("Requested Y axis join column '%s' doesn't exist in table: '%v'", joinYCol, yTable)
			log.Println(err)
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, err.Error())
			return
		}
	}
	return
}
