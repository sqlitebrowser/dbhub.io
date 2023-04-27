package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/sessions"
	com "github.com/sqlitebrowser/dbhub.io/common"
)

func visualisePage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		Data        com.SQLiteRecordSet
		DB          com.SQLiteDBinfo
		PageMeta    PageMetaInfo
		ParamsGiven bool
		DataGiven   bool
		ChartType   string
		XAxisCol    string
		YAxisCol    string
		ShowXLabel  bool
		ShowYLabel  bool
		SQL         string
		VisNames    []string
		IsLive      bool
	}

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}
	dbName, err := getDatabaseName(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if a specific database commit ID was given
	var commitID string
	commitID, err = com.GetFormCommit(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Invalid database commit ID")
		return
	}

	// Check if a branch name was requested
	var branchName string
	branchName, err = com.GetFormBranch(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Validation failed for branch name")
		return
	}

	// Check if a named tag was requested
	var tagName string
	tagName, err = com.GetFormTag(r)
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
	var exists bool
	exists, err = com.CheckDBPermissions(pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, false)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", dbName.Owner, "/",
			dbName.Database))
		return
	}

	// * Execution can only get here if the user has access to the requested database *

	// Check if this is a live database
	var liveNode string
	pageData.IsLive, liveNode, err = com.CheckDBLive(dbName.Owner, dbName.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Live databases are handled differently to standard ones
	if !pageData.IsLive {
		// If a specific commit was requested, make sure it exists in the database commit history
		if commitID != "" {
			commitList, err := com.GetCommitList(dbName.Owner, dbName.Database)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}
			if _, ok := commitList[commitID]; !ok {
				// The requested commit isn't one in the database commit history so error out
				errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Unknown commit for database '%s/%s'", dbName.Owner,
					dbName.Database))
				return
			}
		}

		// If a specific release was requested, and no commit ID was given, retrieve the commit ID matching the release
		if commitID == "" && releaseName != "" {
			releases, err := com.GetReleases(dbName.Owner, dbName.Database)
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
		branchHeads, err := com.GetBranches(dbName.Owner, dbName.Database)
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
			tags, err := com.GetTags(dbName.Owner, dbName.Database)
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
			commitID, err = com.DefaultCommit(dbName.Owner, dbName.Database)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}
		}

		// Retrieve default branch name details
		if branchName == "" {
			branchName, err = com.GetDefaultBranchName(dbName.Owner, dbName.Database)
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
				log.Printf("Duplicate branch name '%s' detected in returned branch list for database '%s/%s', "+
					"logged in user '%s'", com.SanitiseLogString(j), dbName.Owner, dbName.Database, pageData.PageMeta.LoggedInUser)
			}
		}
		pageData.DB.Info.Branch = branchName
	}

	// Retrieve the database details
	err = com.DBDetails(&pageData.DB, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, commitID)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Get a list of all saved visualisations for this database
	pageData.VisNames, err = com.GetVisualisations(dbName.Owner, dbName.Database)
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
			// No default was found, but there are saved visualisations, so we just use the first one
			visName = pageData.VisNames[0]
		}

		// Retrieve a set of visualisation parameters for this database
		params, ok, err := com.GetVisualisationParams(dbName.Owner, dbName.Database, visName)
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
			if !pageData.IsLive {
				// It's a standard database, so run the query locally
				data, err = com.SQLiteRunQueryDefensive(w, r, com.QuerySourceVisualisation, dbName.Owner, dbName.Database, commitID, pageData.PageMeta.LoggedInUser, params.SQL)
				if err != nil {
					errorPage(w, r, http.StatusInternalServerError, err.Error())
					return
				}

				//	TODO: Consider cacheing/retrieving the data for this visualisation
				//	hash := visHash(dbName.Owner, "/", dbName.Database, commitID, "default", params)
				//	data, ok, err := com.GetVisualisationData(dbName.Owner, "/", dbName.Database, commitID, hash)
			} else {
				// It's a live database, so run the query via our AMQP backend
				data, err = com.LiveQuery(liveNode, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, params.SQL)
				if err != nil {
					log.Println(err)
					errorPage(w, r, http.StatusInternalServerError, err.Error())
					return
				}
			}
			if len(data.Records) > 0 {
				// * If data was returned, automatically provide it to the page *
				pageData.Data = data
				pageData.DataGiven = true
			}
		}
	}

	// For live databases, we ask the AMQP backend for its file size
	if pageData.IsLive {
		pageData.DB.Info.DBEntry.Size, err = com.LiveSize(liveNode, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database)
		if err != nil {
			log.Println(err)
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// If there are no visualisations, indicate that using an empty slice instead of a null value. This makes sure the array of
	// visualisation names in the resulting JavaScript code is encoded correctly.
	if pageData.VisNames == nil {
		pageData.VisNames = make([]string, 0)
	}

	// Fill out various metadata fields
	pageData.PageMeta.Title = fmt.Sprintf("vis - %s %s %s", dbName.Owner, "/", dbName.Database)

	// Update database star and watch status for the logged in user
	// FIXME: Add Cypress tests for this, to ensure moving the code above isn't screwing anything up (especially cacheing)
	//pageData.MyStar = myStar
	//pageData.MyWatch = myWatch

	// Render the visualisation page
	pageData.PageMeta.PageSection = "db_vis"
	templateName := "visualisePage"
	t := tmpl.Lookup(templateName)
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

	// Validate input
	input := com.VisGetFields{
		VisName: r.FormValue("visname"),
	}
	err = com.Validate.Struct(input)
	if err != nil {
		log.Printf("Input validation error for visDel(): %s", err)
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
	var allowed bool
	allowed, err = com.CheckDBPermissions(loggedInUser, dbOwner, dbName, true)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}
	if allowed == false {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "Database not found")
		return
	}

	// Delete the saved visualisation for this database
	err = com.VisualisationDeleteParams(dbOwner, dbName, visName)
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
		log.Printf("Error when generating CSV: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}
}

// visExecuteSQL executes a custom SQLite SELECT query.
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

// visExecuteSQLShared is shared code used by various functions for executing visualisation SQL.
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

	// Grab the incoming SQLite query
	bodyData, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err)
		return
	}
	var reqData ExecuteSqlRequest
	err = json.Unmarshal([]byte(bodyData), &reqData)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err)
		return
	}

	decodedStr, err := com.CheckUnicode(reqData.Sql, false)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err)
		return
	}

	// Check if the requested database exists
	var exists bool
	exists, err = com.CheckDBPermissions(loggedInUser, dbOwner, dbName, false)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Database '%s/%s' doesn't exist", dbOwner, dbName)
		return
	}

	// Check if this is a live database
	var isLive bool
	var liveNode string
	isLive, liveNode, err = com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}

	// Run the visualisation query
	if !isLive {
		data, err = com.SQLiteRunQueryDefensive(w, r, com.QuerySourceVisualisation, dbOwner, dbName, commitID, loggedInUser, decodedStr)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err)
			return
		}
	} else {
		// Send the query to the appropriate backend live node
		data, err = com.LiveQuery(liveNode, loggedInUser, dbOwner, dbName, decodedStr)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err.Error())
			return
		}
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
	var allowed bool
	allowed, err = com.CheckDBPermissions(loggedInUser, dbOwner, dbName, false)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}
	if allowed == false {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "Database not found")
		return
	}

	// Retrieve a set of visualisation parameters for this database
	var params com.VisParamsV2
	var ok bool
	params, ok, err = com.GetVisualisationParams(dbOwner, dbName, visName)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}
	if ok {
		// Return the results
		var jsonResponse []byte
		jsonResponse, err = json.Marshal(params)
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

	// Extract X axis, Y axis, and aggregate type variables if present
	chartType := r.FormValue("charttype")
	xAxis := r.FormValue("xaxis")
	yAxis := r.FormValue("yaxis")
	showXStr := r.FormValue("showxlabel")
	showYStr := r.FormValue("showylabel")

	// Initial sanity check of the visualisation name
	visName := r.FormValue("visname")
	err = com.ValidateVisualisationName(visName)
	if err != nil {
		log.Printf("Input validation error for visSave(): %s", err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error when validating input: %s", err)
		return
	}

	// Get and check SQL query
	bodyData, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err)
		return
	}
	var data ExecuteSqlRequest
	err = json.Unmarshal([]byte(bodyData), &data)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err)
		return
	}

	decodedStr, err := com.CheckUnicode(data.Sql, false)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err)
		return
	}

	// Ensure minimum viable parameters are present
	if chartType == "" || xAxis == "" || yAxis == "" || visName == "" || decodedStr == "" {
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
		log.Printf("Validation failed on requested X axis field name '%v': %v", com.SanitiseLogString(xAxis), err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Validate the Y axis field name
	err = com.ValidateFieldName(yAxis)
	if err != nil {
		log.Printf("Validation failed on requested Y axis field name '%v': %v", com.SanitiseLogString(yAxis), err.Error())
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
	var allowed bool
	allowed, err = com.CheckDBPermissions(loggedInUser, dbOwner, dbName, true)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}
	if allowed == false {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "%s", "Database not found")
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

	// Check if this is a live database
	var isLive bool
	var liveNode string
	isLive, liveNode, err = com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}

	// Run the visualisation query, to ensure it doesn't generate an error
	if !isLive {
		_, err = com.SQLiteRunQueryDefensive(w, r, com.QuerySourceVisualisation, dbOwner, dbName, commitID, loggedInUser, decodedStr)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err)
			return
		}
	} else {
		// Send the query to the appropriate backend live node
		_, err = com.LiveQuery(liveNode, loggedInUser, dbOwner, dbName, decodedStr)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err)
			return
		}
	}

	// Save the SQLite visualisation parameters
	err = com.VisualisationSaveParams(dbOwner, dbName, visName, vParams)
	if err != nil {
		log.Printf("Error occurred when saving visualisation '%s' for' '%s/%s', commit '%s': %s", com.SanitiseLogString(visName),
			com.SanitiseLogString(dbOwner), com.SanitiseLogString(dbName), com.SanitiseLogString(commitID), err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Save succeeded
	w.WriteHeader(http.StatusOK)
}
