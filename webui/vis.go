package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/sessions"

	com "github.com/sqlitebrowser/dbhub.io/common"
)

func visualisePage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		Auth0       com.Auth0Set
		Data        com.SQLiteRecordSet
		DB          com.SQLiteDBinfo
		Meta        com.MetaInfo
		MyStar      bool
		MyWatch     bool
		ParamsGiven bool
		DataGiven   bool
		ChartType   string
		XAxisCol    string
		YAxisCol    string
		ShowXLabel  bool
		ShowYLabel  bool
		SQL         string
		VisNames    []string
	}

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, false, true, false)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Check if a specific database commit ID was given
	commitID, err := com.GetFormCommit(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Invalid database commit ID")
		return
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
	exists, err := com.CheckDBPermissions(pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, false)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", pageData.Meta.Owner, pageData.Meta.Folder,
			pageData.Meta.Database))
		return
	}

	// * Execution can only get here if the user has access to the requested database *

	// If a specific commit was requested, make sure it exists in the database commit history
	if commitID != "" {
		commitList, err := com.GetCommitList(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if _, ok := commitList[commitID]; !ok {
			// The requested commit isn't one in the database commit history so error out
			errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Unknown commit for database '%s%s%s'", pageData.Meta.Owner,
				pageData.Meta.Folder, pageData.Meta.Database))
			return
		}
	}

	// If a specific release was requested, and no commit ID was given, retrieve the commit ID matching the release
	if commitID == "" && releaseName != "" {
		releases, err := com.GetReleases(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
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
	branchHeads, err := com.GetBranches(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
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
		tags, err := com.GetTags(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
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
		commitID, err = com.DefaultCommit(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Retrieve the database details
	err = com.DBDetails(&pageData.DB, pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, commitID)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if the database was starred by the logged in user
	myStar, err := com.CheckDBStarred(pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve database star status")
		return
	}

	// Check if the database is being watched by the logged in user
	myWatch, err := com.CheckDBWatched(pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve database watch status")
		return
	}

	// TODO: Cache/retrieve the cached SQLite table and column names from memcached.
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
	defer sdb.Close()

	// Retrieve the list of tables and views in the database
	tables, err := com.TablesAndViews(sdb, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.DB.Info.Tables = tables

	// Get a list of all saved visualisations for this database
	pageData.VisNames, err = com.GetVisualisations(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Handle any saved visualisations for this database
	if len(pageData.VisNames) > 0 {
		// If there's a saved vis called "default", use that for the default vis settings
		visName := "default"
		var defaultFound bool
		for _, j := range pageData.VisNames {
			if j == "default" {
				defaultFound = true
			}
		}
		if !defaultFound {
			// No default was found, but there are saved visualisations so we just use the first one
			visName = pageData.VisNames[0]
		}

		// Retrieve a set of visualisation parameters for this database
		params, ok, err := com.GetVisualisationParams(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, visName)
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
			pageData.ShowXLabel = params.ShowXLabel
			pageData.ShowYLabel = params.ShowYLabel
			pageData.SQL = params.SQL
			pageData.XAxisCol = params.XAXisColumn
			pageData.YAxisCol = params.YAXisColumn

			// Automatically run the saved query
			var data com.SQLiteRecordSet
			data, err = com.SQLiteRunQueryDefensive(w, r, com.Visualisation, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, commitID, pageData.Meta.LoggedInUser, params.SQL)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}
			if len(data.Records) > 0 {
				// * If data was returned, automatically provide it to the page *
				pageData.Data = data
				pageData.DataGiven = true
			}

			//	TODO: Cache/retrieve the data for this visualisation too
			//	hash := visHash(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, commitID, "default", params)
			//	data, ok, err := com.GetVisualisationData(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, commitID, hash)
		}
	}

	// If there are no visualisations, indicate that using an empty slice instead of a null value. This makes sure the array of
	// visualisation names in the resulting JavaScript code is encoded correctly.
	if pageData.VisNames == nil {
		pageData.VisNames = make([]string, 0)
	}

	// Fill out various metadata fields
	pageData.Meta.Title = fmt.Sprintf("vis - %s %s %s", pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)

	// Retrieve default branch name details
	if branchName == "" {
		branchName, err = com.GetDefaultBranchName(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
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
				"logged in user '%s'", j, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, pageData.Meta.LoggedInUser)
		}
	}

	pageData.DB.Info.Branch = branchName

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

	// Update database star and watch status for the logged in user
	pageData.MyStar = myStar
	pageData.MyWatch = myWatch

	// Render the visualisation page
	pageData.Meta.PageSection = "db_vis"
	t := tmpl.Lookup("visualisePage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// This function handles requests to delete a saved database visualisation
func visDel(w http.ResponseWriter, r *http.Request) {
	// Retrieve user, database
	dbOwner, dbName, err := com.GetOD(2, r) // 2 = Ignore "/x/visdel/" at the start of the URL
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Required information is missing")
		return
	}
	dbFolder := "/"

	// Validate input
	input := com.VisGetFields{
		VisName: r.FormValue("visname"),
	}
	err = com.Validate.Struct(input)
	if err != nil {
		log.Printf("Input validation error for visGet(): %s", err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error when validating input: %s", err)
		return
	}
	visName := input.VisName

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	validSession := false
	if com.Conf.Environment.Environment == "production" {
		sess, err := store.Get(r, "dbhub-user")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = com.Conf.Environment.UserOverride
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

	// Make sure the logged in user has the permissions to proceed
	allowed, err := com.CheckDBPermissions(loggedInUser, dbOwner, dbFolder, dbName, true)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if allowed == false {
		errorPage(w, r, http.StatusNotFound, "Database not found")
		return
	}

	// Delete the saved visualisation for this database
	err = com.VisualisationDeleteParams(dbOwner, dbFolder, dbName, visName)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}

	// Deletion succeeded
	w.WriteHeader(http.StatusOK)
}

// Sends the visualisation query results back to the user as a CSV delimited file
func visDownloadResults(w http.ResponseWriter, r *http.Request) {
	// Validate user input, fetch results
	data, err := visExecuteSQLShared(w, r)
	if err != nil {
		// Error handling is already done in visExecuteSQLShared()
		return
	}

	// Turn the query results into a form which csv.WriteAll() can process
	var newData [][]string
	for _, j := range data.Records {
		var oneRow []string
		for _, k := range j {
			oneRow = append(oneRow, fmt.Sprintf("%s", k.Value))
		}
		newData = append(newData, oneRow)
	}

	// If the request came from a Windows based device, give it CRLF line endings
	var userAgent string
	if ua, ok := r.Header["User-Agent"]; ok {
		userAgent = strings.ToLower(ua[0])
	}
	win := strings.Contains(userAgent, "windows")

	// Return the results as CSV
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="results.csv"`))
	w.Header().Set("Content-Type", "text/csv")
	output := csv.NewWriter(w)
	output.UseCRLF = win
	err = output.WriteAll(newData)
	if err != nil {
		log.Printf("Error when generating CSV: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}
}

// Executes a custom SQLite SELECT query.
func visExecuteSQL(w http.ResponseWriter, r *http.Request) {
	// Validate user input, fetch results
	data, err := visExecuteSQLShared(w, r)
	if err != nil {
		// Error handling is already done in visExecuteSQLShared()
		return
	}

	// Return the results as JSON
	jsonResponse, err := json.Marshal(data)
	if err != nil {
		log.Println(err)
		return
	}
	fmt.Fprintf(w, "%s", jsonResponse)
}

// Shared code used by various functions for executing visualisation SQL.
func visExecuteSQLShared(w http.ResponseWriter, r *http.Request) (data com.SQLiteRecordSet, err error) {
	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	if com.Conf.Environment.Environment == "production" {
		var sess *sessions.Session
		sess, err = store.Get(r, "dbhub-user")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = com.Conf.Environment.UserOverride
	}
	if u != nil {
		loggedInUser = u.(string)
	}

	// Retrieve user, database, and commit ID
	dbOwner, dbName, commitID, err := com.GetODC(2, r) // 2 = Ignore "/x/execsql/" at the start of the URL
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
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
	exists, err := com.CheckDBPermissions(loggedInUser, dbOwner, dbFolder, dbName, false)
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
	data, err = com.SQLiteRunQueryDefensive(w, r, com.Visualisation, dbOwner, dbFolder, dbName, commitID, loggedInUser, decodedStr)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}
	return
}

// This function handles requests to retrieve database visualisation parameters
func visGet(w http.ResponseWriter, r *http.Request) {
	// Retrieve user, database
	dbOwner, dbName, err := com.GetOD(2, r) // 2 = Ignore "/x/visget/" at the start of the URL
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Required information is missing")
		return
	}
	dbFolder := "/"

	// Validate input
	input := com.VisGetFields{
		VisName: r.FormValue("visname"),
	}
	err = com.Validate.Struct(input)
	if err != nil {
		log.Printf("Input validation error for visGet(): %s", err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error when validating input: %s", err)
		return
	}
	visName := input.VisName

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	if com.Conf.Environment.Environment == "production" {
		sess, err := store.Get(r, "dbhub-user")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = com.Conf.Environment.UserOverride
	}
	if u != nil {
		loggedInUser = u.(string)
	}

	// Make sure the logged in user has the permissions to proceed
	allowed, err := com.CheckDBPermissions(loggedInUser, dbOwner, dbFolder, dbName, false)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if allowed == false {
		errorPage(w, r, http.StatusNotFound, "Database not found")
		return
	}

	// Retrieve a set of visualisation parameters for this database
	params, ok, err := com.GetVisualisationParams(dbOwner, dbFolder, dbName, visName)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}
	if ok {
		// Return the results
		jsonResponse, err := json.Marshal(params)
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err)
			return
		}
		fmt.Fprintf(w, "%s", jsonResponse)
		return
	}

	// No saved visualisations were found
	w.WriteHeader(http.StatusNoContent)
}

// This function handles requests to save the database visualisation parameters
func visSave(w http.ResponseWriter, r *http.Request) {
	// Retrieve user, database, and commit ID
	dbOwner, dbName, commitID, err := com.GetODC(2, r) // 2 = Ignore "/x/vissave/" at the start of the URL
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	dbFolder := "/"

	// Extract X axis, Y axis, and aggregate type variables if present
	chartType := r.FormValue("charttype")
	xAxis := r.FormValue("xaxis")
	yAxis := r.FormValue("yaxis")
	sqlStr := r.FormValue("sql")
	showXStr := r.FormValue("showxlabel")
	showYStr := r.FormValue("showylabel")

	// Initial sanity check of the visualisation name
	// TODO: Expand this approach out to the other fields
	input := com.VisGetFields{ // TODO: Create a new com.VisSaveFields{} type
		VisName: r.FormValue("visname"),
	}
	err = com.Validate.Struct(input)
	if err != nil {
		log.Printf("Input validation error for visGet(): %s", err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error when validating input: %s", err)
		return
	}
	visName := input.VisName

	// Ensure minimum viable parameters are present
	if chartType == "" || xAxis == "" || yAxis == "" || visName == "" || sqlStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Ensure only valid chart types are accepted
	if chartType != "hbc" && chartType != "vbc" && chartType != "lc" && chartType != "pie" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Unknown chart type")
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

	// Validate the X and Y axis label booleans
	var showX, showY bool
	if showXStr == "true" {
		showX = true
	}
	if showYStr == "true" {
		showY = true
	}

	// Make sure the incoming SQLite query is "safe"
	decodedStr, err := com.CheckUnicode(sqlStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err.Error())
		return
	}

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	validSession := false
	if com.Conf.Environment.Environment == "production" {
		sess, err := store.Get(r, "dbhub-user")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = com.Conf.Environment.UserOverride
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

	// Make sure the logged in user has the permissions to proceed
	allowed, err := com.CheckDBPermissions(loggedInUser, dbOwner, dbFolder, dbName, true)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if allowed == false {
		errorPage(w, r, http.StatusNotFound, "Database not found")
		return
	}

	// Retrieve the visualisation query result, so we can save that too
	vParams := com.VisParamsV2{
		ChartType:   chartType,
		ShowXLabel:  showX,
		ShowYLabel:  showY,
		SQL:         decodedStr,
		XAXisColumn: xAxis,
		YAXisColumn: yAxis,
	}

	// Run the visualisation query, to make sure it returns valid data
	visData, err := com.SQLiteRunQueryDefensive(w, r, com.Visualisation, dbOwner, dbFolder, dbName, commitID, loggedInUser, decodedStr)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return
	}

	// If the # of rows returned from the query is 0, let the user know + don't save
	if len(visData.Records) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Can't save query, it returns no data")
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

	// TODO: Cache the SQLite visualisation data
	//         * We'll probably need to update the SQLite authoriser code to catch SQLite functions which shouldn't be
	//           cached - such as random() - and not cache those results
	//hash := visHash(dbOwner, dbFolder, dbName, commitID, visName, vParams)
	//err = com.VisualisationSaveData(dbOwner, dbFolder, dbName, commitID, hash, visData)
	//if err != nil {
	//	log.Printf("Error occurred when saving visualisation '%s' for' '%s%s%s', commit '%s': %s\n", visName,
	//		dbOwner, dbFolder, dbName, commitID, err.Error())
	//	w.WriteHeader(http.StatusInternalServerError)
	//	return
	//}

	// Save succeeded
	w.WriteHeader(http.StatusOK)
}
