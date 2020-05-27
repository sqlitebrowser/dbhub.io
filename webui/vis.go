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

	com "github.com/sqlitebrowser/dbhub.io/common"
	gfm "github.com/sqlitebrowser/github_flavored_markdown"
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
		pageData.ShowXLabel = params.ShowXLabel
		pageData.ShowYLabel = params.ShowYLabel
		pageData.SQL = params.SQL
		pageData.XAxisCol = params.XAXisColumn
		pageData.YAxisCol = params.YAXisColumn

		// Automatically run the saved query
		var data com.SQLiteRecordSet
		data, err = visRunQuery(w, r, dbOwner, dbFolder, dbName, commitID, loggedInUser, params.SQL)
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
		//	hash := visHash(dbOwner, dbFolder, dbName, commitID, "default", params)
		//	data, ok, err := com.GetVisualisationData(dbOwner, dbFolder, dbName, commitID, hash)
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

// Checks if a given string is unicode, and safe for using in SQLite queries (eg no SQLite control characters)
func visCheckUnicode(rawInput string) (str string, err error) {
	var decoded []byte
	decoded, err = base64.StdEncoding.DecodeString(rawInput)
	if err != nil {
		//fmt.Errorf("Error when decoding query: '%s'", err)
		return
	}

	// Ensure the decoded string is valid UTF-8
	if !utf8.Valid(decoded) {
		//w.WriteHeader(http.StatusBadRequest)
		err = fmt.Errorf("SQL string contains invalid characters: '%v'", err)
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
		//w.WriteHeader(http.StatusBadRequest)
		err = fmt.Errorf("SQL string contains invalid characters: '%v'", err)
		return
	}

	// No errors, so return the string
	return decodedStr, nil
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
	decodedStr, err := visCheckUnicode(rawInput)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err.Error())
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

	// Run the query
	dataRows, err := visRunQuery(w, r, dbOwner, dbFolder, dbName, commitID, loggedInUser, decodedStr)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Return the results
	jsonResponse, err := json.Marshal(dataRows)
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

// Runs a user provided SQLite query
func visRunQuery(w http.ResponseWriter, r *http.Request, dbOwner, dbFolder, dbName, commitID, loggedInUser, query string) (com.SQLiteRecordSet, error) {
	// Retrieve the SQLite database from Minio (also doing appropriate permission/access checking)
	sdb, err := com.OpenSQLiteDatabaseDefensive(w, r, dbOwner, dbFolder, dbName, commitID, loggedInUser)
	if err != nil {
		// The return handling was already done in OpenSQLiteDatabaseDefensive()
		return com.SQLiteRecordSet{}, err
	}

	// Automatically close the SQLite database when this function finishes
	defer func() {
		sdb.Close()
	}()

	// Was a user agent part of the request?
	var userAgent string
	if ua, ok := r.Header["User-Agent"]; ok {
		userAgent = ua[0]
	}

	// Log the SQL query (prior to executing it)
	var logID int64
	logID, err = com.LogSQLiteQueryBefore(dbOwner, dbFolder, dbName, loggedInUser, r.RemoteAddr, userAgent, query)
	if err != nil {
		return com.SQLiteRecordSet{}, err
	}

	// Execute the SQLite select query (or queries)
	var dataRows com.SQLiteRecordSet
	var memUsed, memHighWater int64
	memUsed, memHighWater, dataRows, err = com.SQLiteRunQuery(sdb, query, true, true)
	if err != nil {
		return com.SQLiteRecordSet{}, err
	}

	// Add the SQLite execution stats to the log record
	err = com.LogSQLiteQueryAfter(logID, memUsed, memHighWater)
	if err != nil {
		return com.SQLiteRecordSet{}, err
	}
	return dataRows, err
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
	xAxis := r.FormValue("xaxis")
	yAxis := r.FormValue("yaxis")
	visName := r.FormValue("visname")
	sqlStr := r.FormValue("sql")
	showXStr := r.FormValue("showxlabel")
	showYStr := r.FormValue("showylabel")

	// Ensure minimum viable parameters are present
	if chartType == "" || xAxis == "" || yAxis == "" || visName == "" || sqlStr == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	//Ensure only valid chart types are accepted
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

	// Initial sanity check of the visualisation name
	// TODO: We'll probably need to figure out a better validation character set than the fieldname one
	//         * The unicode parsing code in sqlite.go OpenSQLiteDatabaseDefensive() may be reusable for this
	err = com.ValidateFieldName(visName)
	if err != nil {
		log.Printf("Validation failed on requested visualisation name '%v': %v\n", visName, err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the incoming SQLite query is "safe"
	decodedStr, err := visCheckUnicode(sqlStr)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err.Error())
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
	visData, err := visRunQuery(w, r, dbOwner, dbFolder, dbName, commitID, loggedInUser, decodedStr)
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
