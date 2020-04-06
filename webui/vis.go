package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	sqlite "github.com/gwenn/gosqlite"
	com "github.com/sqlitebrowser/dbhub.io/common"
	gfm "github.com/sqlitebrowser/github_flavored_markdown"
)

func visualisePage(w http.ResponseWriter, r *http.Request) {
	pageName := "Visualise data page"

	var pageData struct {
		Auth0       com.Auth0Set
		Data        com.SQLiteRecordSet
		DB          com.SQLiteDBinfo
		Meta        com.MetaInfo
		MyStar      bool
		MyWatch     bool
		ParamsGiven bool
		DataGiven   bool
		XAxis       string
		YAxis       string
		AggType     string
		OrderBy     int
		OrderDir    int
		Records     []com.VisRowV1
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

	// If a table name was supplied, validate it
	dbTable := r.FormValue("table")
	if dbTable != "" {
		err = com.ValidatePGTable(dbTable)
		if err != nil {
			// Validation failed, so don't pass on the table name
			log.Printf("%s: Validation failed for table name: %s", pageName, err)
			dbTable = ""
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

	// If an order and direction were given, pass that through to the rendered page
	orderByStr := r.FormValue("orderby")
	orderDirStr := r.FormValue("orderdir")
	switch orderByStr {
	case "0":
		pageData.OrderBy = 0
	case "1":
		pageData.OrderBy = 1
	default:
		// Ignore any empty or invalid values
	}
	// Parse the order direction input
	switch orderDirStr {
	case "0":
		pageData.OrderDir = 0
	case "1":
		pageData.OrderDir = 1
	default:
		// Ignore any empty or invalid values
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
	if dbTable == "" {
		// Ensure the default table name validates.  This catches a case where a database was uploaded with an invalid
		// table name and somehow because selected as the default
		a := pageData.DB.Info.DefaultTable
		if a != "" {
			err = com.ValidatePGTable(a)
			if err == nil {
				// The database table name is acceptable, so use it
				dbTable = pageData.DB.Info.DefaultTable
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

	// Get a handle from Minio for the database object
	sdb, err := com.OpenMinioObject(pageData.DB.Info.DBEntry.Sha256[:com.MinioFolderChars],
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
	if dbTable != "" {
		// Check the requested table is present
		tablePresent := false
		for _, tbl := range tables {
			if tbl == dbTable {
				tablePresent = true
			}
		}
		if tablePresent == false {
			// The requested table doesn't exist in the database, so pick one of the tables that is
			for _, t := range tables {
				err = com.ValidatePGTable(t)
				if err == nil {
					// Validation passed, so use this table
					dbTable = t
					pageData.DB.Info.DefaultTable = t
					break
				}
			}
		}
	}

	// If a specific table wasn't requested, use the first table in the database that passes validation
	if dbTable == "" {
		for _, i := range pageData.DB.Info.Tables {
			if i != "" {
				err = com.ValidatePGTable(i)
				if err == nil {
					// The database table name is acceptable, so use it
					dbTable = i
					break
				}
			}
		}
	}

	// Validate the table name, just to be careful
	if dbTable != "" {
		err = com.ValidatePGTable(dbTable)
		if err != nil {
			// Validation failed, so don't pass on the table name

			// If the failed table name is "{{ db.Tablename }}", don't bother logging it.  It's just a search
			// bot picking up AngularJS in a string and doing a request with it
			if dbTable != "{{ db.Tablename }}" {
				log.Printf("%s: Validation failed for table name: '%s': %s", pageName, dbTable, err)
			}
			errorPage(w, r, http.StatusBadRequest, "Validation failed for table name")
			return
		}
	}

	// Retrieve the SQLite table and column names
	pageData.Data.Tablename = dbTable
	colList, err := sdb.Columns("", dbTable)
	if err != nil {
		log.Printf("Error when reading column names for table '%s': %v\n", dbTable,
			err.Error())
		errorPage(w, r, http.StatusInternalServerError, "Error when reading from the database")
		return
	}
	var c []string
	for _, j := range colList {
		c = append(c, j.Name)
	}
	pageData.Data.ColNames = c

	// Retrieve the default visualisation parameters for this database, if they've been set
	params, ok, err := com.GetVisualisationParams(dbOwner, dbFolder, dbName, "default")
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// If saved parameters were found, pass them through to the web page
	if ok {
		pageData.ParamsGiven = true
		pageData.XAxis = params.XAXisColumn
		pageData.YAxis = params.YAXisColumn
		switch params.AggType {
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
			return
		}
		pageData.OrderBy = params.OrderBy
		pageData.OrderDir = params.OrderDir

		// Retrieve the saved data for this visualisation too, if it's available
		hash := visHash(dbOwner, dbFolder, dbName, commitID, params.XAXisColumn, params.YAXisColumn, params.AggType, "default")
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

// This function handles requests for database visualisation data
func visRequestHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Visualisation Request Handler"

	// Retrieve user, database, table, and commit ID
	dbOwner, dbName, requestedTable, commitID, err := com.GetODTC(2, r) // 1 = Ignore "/x/vis/" at the start of the URL
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	dbFolder := "/"

	// Extract X axis, Y axis, and aggregate type variables if present
	xAxis := r.FormValue("xaxis")
	yAxis := r.FormValue("yaxis")
	aggTypeStr := r.FormValue("agg")

	// Ensure minimum viable parameters are present
	if xAxis == "" || yAxis == "" || aggTypeStr == "" || requestedTable == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	aggType := 0
	switch aggTypeStr {
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

	// Check if the user has access to the requested database
	bucket, id, _, err := com.MinioLocation(dbOwner, dbFolder, dbName, commitID, loggedInUser)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Sanity check
	if id == "" {
		// The requested database wasn't found
		log.Printf("%s: Requested database not found. Owner: '%s%s%s'", pageName, dbOwner, dbFolder, dbName)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Open the Minio database
	sdb, err := com.OpenMinioObject(bucket, id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Automatically close the SQLite database when this function finishes
	defer func() {
		sdb.Close()
	}()

	// Retrieve the list of tables in the database
	tables, err := sdb.Tables("")
	if err != nil {
		// An error occurred, so get the extended error code
		if cerr, ok := err.(sqlite.ConnError); ok {
			// Check if the error was due to the table being locked
			extCode := cerr.ExtendedCode()
			if extCode == 5 { // Magic number which (in this case) means "database is locked"
				// Wait 3 seconds then try again
				time.Sleep(3 * time.Second)
				tables, err = sdb.Tables("")
				if err != nil {
					log.Printf("Error retrieving table names: %s", err)
					return
				}
			} else {
				log.Printf("Error retrieving table names: %s", err)
				return
			}
		} else {
			log.Printf("Error retrieving table names: %s", err)
			return
		}
	}
	if len(tables) == 0 {
		// No table names were returned, so abort
		log.Printf("The database '%s' doesn't seem to have any tables. Aborting.", dbName)
		return
	}
	vw, err := sdb.Views("")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	tables = append(tables, vw...)

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
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	// Verify the X and Y axis columns exist
	colList, err := sdb.Columns("", requestedTable)
	if err != nil {
		log.Printf("Error when reading column names for table '%s': %v\n", requestedTable, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	xColExists := false
	for _, j := range colList {
		if j.Name == xAxis {
			xColExists = true
		}
	}
	if xColExists == false {
		// The requested X axis column doesn't exist
		log.Printf("Requested X axis column doesn't exist '%s' in table: '%v'\n", xAxis, requestedTable)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	yColExists := false
	for _, j := range colList {
		if j.Name == yAxis {
			yColExists = true
		}
	}
	if yColExists == false {
		// The requested Y axis column doesn't exist
		log.Printf("Requested Y axis column doesn't exist '%s' in table: '%v'\n", yAxis, requestedTable)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Run the SQLite visualisation query
	visRows, err := com.RunSQLiteVisQuery(sdb, requestedTable, xAxis, yAxis, aggType)
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
	pageName := "Visualisation Save Request Handler"

	// Retrieve user, database, table, and commit ID
	dbOwner, dbName, requestedTable, commitID, err := com.GetODTC(2, r) // 1 = Ignore "/x/vis/" at the start of the URL
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	dbFolder := "/"

	// Extract X axis, Y axis, and aggregate type variables if present
	xAxis := r.FormValue("xaxis")
	yAxis := r.FormValue("yaxis")
	aggTypeStr := r.FormValue("agg")
	visName := r.FormValue("visname")
	orderByStr := r.FormValue("orderby")
	orderDirStr := r.FormValue("orderdir")

	// Ensure minimum viable parameters are present
	if xAxis == "" || yAxis == "" || requestedTable == "" || visName == "" || orderByStr == "" || orderDirStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Parse the aggregation type
	aggType := 0
	switch aggTypeStr {
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

	// Parse the order by input
	orderBy := 0
	switch orderByStr {
	case "0":
		orderBy = 0
	case "1":
		orderBy = 1
	default:
		log.Println("Unknown order by input")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Parse the order direction input
	orderDir := 0
	switch orderDirStr {
	case "0":
		orderDir = 0
	case "1":
		orderDir = 1
	default:
		log.Println("Unknown order direction input")
		w.WriteHeader(http.StatusBadRequest)
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

	// Initial sanity check of the visualisation name
	// TODO: We'll probably need to figure out a better validation character set than the fieldname one
	err = com.ValidateFieldName(visName)
	if err != nil {
		log.Printf("Validation failed on requested visualisation name '%v': %v\n", visName, err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
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

	// Make sure the save request is coming from the database owner
	if loggedInUser != dbOwner {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "Only the database owner is allowed to save a visualisation (at least for now)")
		return
	}

	// Check if the user has access to the requested database
	bucket, id, _, err := com.MinioLocation(dbOwner, dbFolder, dbName, commitID, loggedInUser)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Sanity check
	if id == "" {
		// The requested database wasn't found
		log.Printf("%s: Requested database not found. Owner: '%s%s%s'", pageName, dbOwner, dbFolder, dbName)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Open the Minio database
	sdb, err := com.OpenMinioObject(bucket, id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Automatically close the SQLite database when this function finishes
	defer func() {
		sdb.Close()
	}()

	// Retrieve the list of tables in the database
	tables, err := sdb.Tables("")
	if err != nil {
		// An error occurred, so get the extended error code
		if cerr, ok := err.(sqlite.ConnError); ok {
			// Check if the error was due to the table being locked
			extCode := cerr.ExtendedCode()
			if extCode == 5 { // Magic number which (in this case) means "database is locked"
				// Wait 3 seconds then try again
				time.Sleep(3 * time.Second)
				tables, err = sdb.Tables("")
				if err != nil {
					log.Printf("Error retrieving table names: %s", err)
					return
				}
			} else {
				log.Printf("Error retrieving table names: %s", err)
				return
			}
		} else {
			log.Printf("Error retrieving table names: %s", err)
			return
		}
	}
	if len(tables) == 0 {
		// No table names were returned, so abort
		log.Printf("The database '%s' doesn't seem to have any tables. Aborting.", dbName)
		return
	}
	vw, err := sdb.Views("")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	tables = append(tables, vw...)

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
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	// Verify the X and Y axis columns exist
	colList, err := sdb.Columns("", requestedTable)
	if err != nil {
		log.Printf("Error when reading column names for table '%s': %v\n", requestedTable, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	xColExists := false
	for _, j := range colList {
		if j.Name == xAxis {
			xColExists = true
		}
	}
	if xColExists == false {
		// The requested X axis column doesn't exist
		log.Printf("Requested X axis column doesn't exist '%s' in table: '%v'\n", xAxis, requestedTable)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	yColExists := false
	for _, j := range colList {
		if j.Name == yAxis {
			yColExists = true
		}
	}
	if yColExists == false {
		// The requested Y axis column doesn't exist
		log.Printf("Requested Y axis column doesn't exist '%s' in table: '%v'\n", yAxis, requestedTable)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Retrieve the visualisation query result, so we can save that too
	visParams := com.VisParamsV1{
		XAXisColumn: xAxis,
		YAXisColumn: yAxis,
		AggType:     aggType,
		OrderBy:     orderBy,
		OrderDir:    orderDir,
	}
	visData, err := com.RunSQLiteVisQuery(sdb, requestedTable, xAxis, yAxis, aggType)
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
	err = com.VisualisationSaveParams(dbOwner, dbFolder, dbName, visName, visParams)
	if err != nil {
		log.Printf("Error occurred when saving visualisation '%s' for' '%s%s%s', commit '%s': %s\n", visName,
			dbOwner, dbFolder, dbName, commitID, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Save the SQLite visualisation data
	hash := visHash(dbOwner, dbFolder, dbName, commitID, xAxis, yAxis, aggType, visName)
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

// Calculate the hash string to save or retrieve any visualisation data with
func visHash(dbOwner string, dbFolder string, dbName string, commitID string, xAxis string, yAxis string, aggType int, visName string) string {
	z := md5.Sum([]byte(fmt.Sprintf("%s/%s/%s/%s/%s/%s/%d/%s", strings.ToLower(dbOwner), dbFolder, dbName, commitID, xAxis, yAxis, aggType, visName)))
	return hex.EncodeToString(z[:])
}
