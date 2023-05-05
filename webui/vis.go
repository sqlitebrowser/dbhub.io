package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	com "github.com/sqlitebrowser/dbhub.io/common"
)

func visualisePage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		DB             com.SQLiteDBinfo
		PageMeta       PageMetaInfo
		Branches       map[string]com.BranchEntry
		Visualisations map[string]com.VisParamsV2
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
	exists, err := com.CheckDBPermissions(pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, false)
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
	isLive, liveNode, err := com.CheckDBLive(dbName.Owner, dbName.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Live databases are handled differently to standard ones
	if !isLive {
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

		// Read the branch heads list from the database
		pageData.Branches, err = com.GetBranches(dbName.Owner, dbName.Database)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// If a specific branch was requested and no commit ID was given, use the latest commit for the branch
		if commitID == "" && branchName != "" {
			c, ok := pageData.Branches[branchName]
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

		pageData.DB.Info.Branch = branchName
	}

	// Retrieve the database details
	err = com.DBDetails(&pageData.DB, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, commitID)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Get a list of all saved visualisations for this database
	pageData.Visualisations, err = com.GetVisualisations(dbName.Owner, dbName.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// For live databases, we ask the AMQP backend for its file size
	if isLive {
		pageData.DB.Info.DBEntry.Size, err = com.LiveSize(liveNode, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database)
		if err != nil {
			log.Println(err)
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Fill out various metadata fields
	pageData.PageMeta.Title = fmt.Sprintf("Visualisations - %s %s %s", dbName.Owner, "/", dbName.Database)
	pageData.PageMeta.PageSection = "db_vis"

	// Render the visualisation page
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
	allowed, err := com.CheckDBPermissions(loggedInUser, dbOwner, dbName, true)
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

	// Initial sanity check of the visualisation name
	visName := r.FormValue("visname")
	err = com.ValidateVisualisationName(visName)
	if err != nil {
		log.Printf("Input validation error for visDel(): %s", err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error when validating input: %s", err)
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

// visExecuteSQL executes a custom SQLite SELECT query.
func visExecuteSQL(w http.ResponseWriter, r *http.Request) {
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
	exists, err := com.CheckDBPermissions(loggedInUser, dbOwner, dbName, false)
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
	isLive, liveNode, err := com.CheckDBLive(dbOwner, dbName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}

	// Run the visualisation query
	var data com.SQLiteRecordSet
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

	// Return the results as JSON
	jsonResponse, err := json.Marshal(data)
	if err != nil {
		log.Println(err)
		return
	}
	fmt.Fprintf(w, "%s", jsonResponse)
}

// This function handles requests to rename an existing saved visualisation
func visRename(w http.ResponseWriter, r *http.Request) {
	// Retrieve user and database
	dbOwner, dbName, err := com.GetOD(2, r) // 2 = Ignore "/x/visrename/" at the start of the URL
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
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
	allowed, err := com.CheckDBPermissions(loggedInUser, dbOwner, dbName, true)
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

	// Initial sanity check of the visualisation name
	visName := r.FormValue("visname")
	err = com.ValidateVisualisationName(visName)
	if err != nil {
		log.Printf("Input validation error for visRename(): %s", err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error when validating input: %s", err)
		return
	}

	// Initial sanity check of the new visualisation name
	visNewName := r.FormValue("visnewname")
	err = com.ValidateVisualisationName(visNewName)
	if err != nil {
		log.Printf("Input validation error for visRename(): %s", err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error when validating input: %s", err)
		return
	}

	// Perform rename
	err = com.VisualisationRename(dbOwner, dbName, visName, visNewName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

// This function handles requests to save the database visualisation parameters
func visSave(w http.ResponseWriter, r *http.Request) {
	// Retrieve user, database, and commit ID
	dbOwner, dbName, commitID, err := com.GetODC(2, r) // 2 = Ignore "/x/vissave/" at the start of the URL
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
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
	allowed, err := com.CheckDBPermissions(loggedInUser, dbOwner, dbName, true)
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

	// Initial sanity check of the visualisation name
	visName := r.FormValue("visname")
	err = com.ValidateVisualisationName(visName)
	if err != nil {
		log.Printf("Input validation error for visSave(): %s", err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error when validating input: %s", err)
		return
	}

	// Grab the incoming visualisation object
	bodyData, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err)
		return
	}
	var data com.VisParamsV2
	err = json.Unmarshal([]byte(bodyData), &data)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err)
		return
	}

	// Ensure minimum viable parameters are present
	if data.ChartType == "" || data.XAXisColumn == "" || data.YAXisColumn == "" || visName == "" || data.SQL == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Ensure only valid chart types are accepted
	if data.ChartType != "hbc" && data.ChartType != "vbc" && data.ChartType != "lc" && data.ChartType != "pie" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Unknown chart type")
		return
	}

	// Validate the X axis field name
	err = com.ValidateFieldName(data.XAXisColumn)
	if err != nil {
		log.Printf("Validation failed on requested X axis field name '%v': %v", com.SanitiseLogString(data.XAXisColumn), err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Validate the Y axis field name
	err = com.ValidateFieldName(data.YAXisColumn)
	if err != nil {
		log.Printf("Validation failed on requested Y axis field name '%v': %v", com.SanitiseLogString(data.YAXisColumn), err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Validate SQL string
	_, err = com.CheckUnicode(data.SQL, false)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Save the SQLite visualisation parameters
	err = com.VisualisationSaveParams(dbOwner, dbName, visName, data)
	if err != nil {
		log.Printf("Error occurred when saving visualisation '%s' for' '%s/%s', commit '%s': %s", com.SanitiseLogString(visName),
			com.SanitiseLogString(dbOwner), com.SanitiseLogString(dbName), com.SanitiseLogString(commitID), err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Save succeeded
	w.WriteHeader(http.StatusOK)
}
