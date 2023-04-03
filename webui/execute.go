package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/sessions"
	com "github.com/sqlitebrowser/dbhub.io/common"
)

func executePage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		Data        com.SQLiteRecordSet
		DB          com.SQLiteDBinfo
		PageMeta    PageMetaInfo
		ParamsGiven bool
		SQL         string
		ExecNames   []string
		SelectedName string
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

	// Check if the database exists and the user has access to view it
	var exists bool
	exists, err = com.CheckDBPermissions(pageData.PageMeta.LoggedInUser, dbName.Owner, "/", dbName.Database, true)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", dbName.Owner, dbName.Folder,
			dbName.Database))
		return
	}

	// * Execution can only get here if the user has access to the requested database *

	// Ensure this is a live database
	var isLive bool
	var liveNode string
	isLive, liveNode, err = com.CheckDBLive(dbName.Owner, dbName.Folder, dbName.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !isLive {
		errorPage(w, r, http.StatusBadRequest, "Executing SQL statements is only supported for Live databases")
		return
	}

	// Retrieve the database details
	err = com.DBDetails(&pageData.DB, pageData.PageMeta.LoggedInUser, dbName.Owner, "/", dbName.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Get a list of all saved SQL statements for this user
	pageData.ExecNames, err = com.LiveExecuteSQLList(dbName.Owner)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Handle any saved SQL statements for this database
	if len(pageData.ExecNames) > 0 {
		// If there's a saved statement called "default", then we use that
		sqlName := "default"
		var defaultFound bool
		for _, j := range pageData.ExecNames {
			if j == "default" {
				defaultFound = true
			}
		}
		if !defaultFound {
			// No default was found, but there are saved SQL statements.  So we just use the first one
			sqlName = pageData.ExecNames[0]
		}

		// Retrieve the saved SQL statement text
		sqlNames, err := com.LiveExecuteSQLGet(dbName.Owner, sqlName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// If saved SQL statements were found, pass them through to the web page
		if len(sqlNames) > 0 {
			pageData.ParamsGiven = true
			pageData.SelectedName = sqlName
			pageData.SQL = sqlNames
		}
	}

	// Ask the AMQP backend for the database file size
	pageData.DB.Info.DBEntry.Size, err = com.LiveSize(liveNode, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database)
	if err != nil {
		log.Println(err)
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// If there are no saved SQL statements, indicate that using an empty slice instead of a null value. This makes
	// sure the array of statements names in the resulting JavaScript code is encoded correctly.
	if pageData.ExecNames == nil {
		pageData.ExecNames = make([]string, 0)
	}

	// Fill out various metadata fields
	pageData.PageMeta.Title = fmt.Sprintf("Execute SQL - %s / %s", dbName.Owner, dbName.Database)

	// Update database star and watch status for the logged in user
	// FIXME: Add Cypress tests for this, to ensure moving the code above isn't screwing anything up (especially caching)
	//pageData.MyStar = myStar
	//pageData.MyWatch = myWatch

	// Render the visualisation page
	pageData.PageMeta.PageSection = "db_exec"
	templateName := "executePage"
	t := tmpl.Lookup(templateName)
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// This function handles requests to delete a saved database execution query
func execDel(w http.ResponseWriter, r *http.Request) {
	// Retrieve user, database
	dbOwner, dbName, err := com.GetOD(2, r) // 2 = Ignore "/x/execdel/" at the start of the URL
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Required information is missing")
		return
	}
	dbFolder := "/"

	// Validate input
	input := com.VisGetFields{
		VisName: r.FormValue("sqlname"),
	}
	err = com.Validate.Struct(input)
	if err != nil {
		log.Printf("Input validation error for execDel(): %s", err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error when validating input: %s", err)
		return
	}
	sqlName := input.VisName

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
	allowed, err = com.CheckDBPermissions(loggedInUser, dbOwner, dbFolder, dbName, true)
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

	// Delete the saved SQL statement
	err = com.LiveExecuteSQLDelete(dbOwner, sqlName)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}

	// Deletion succeeded
	w.WriteHeader(http.StatusOK)
}

// execLiveSQL executes a user provided SQLite statement on a database.
func execLiveSQL(w http.ResponseWriter, r *http.Request) {
	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	var err error
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

	// Retrieve user and database info
	var dbOwner, dbName string
	dbOwner, dbName, _, err = com.GetODC(2, r) // 2 = Ignore "/x/execlivesql/" at the start of the URL
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err)
		return
	}
	dbFolder := "/"

	// Grab the incoming SQLite query
	rawInput := r.FormValue("sql")
	var sql string
	sql, err = com.CheckUnicode(rawInput)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err)
		return
	}

	// Check if the requested database exists
	var exists bool
	exists, err = com.CheckDBPermissions(loggedInUser, dbOwner, dbFolder, dbName, true)
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

	// Make sure this is a live database
	var isLive bool
	var liveNode string
	isLive, liveNode, err = com.CheckDBLive(dbOwner, dbFolder, dbName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}
	if !isLive {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Executing SQL statements is only supported on Live databases")
		return
	}

	// Send the SQL execution request to our AMQP backend
	var rowsChanged int
	rowsChanged, err = com.LiveExecute(liveNode, loggedInUser, dbOwner, dbName, sql)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}

	// The SQL statement execution succeeded, so pass along the # of rows changed
	z := com.ExecuteResponseContainer{RowsChanged: rowsChanged, Status: "OK"}
	jsonData, err := json.Marshal(z)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}
	fmt.Fprintf(w, "%s", jsonData)
	return
}

// This function handles requests to retrieve database execution query parameters
func execGet(w http.ResponseWriter, r *http.Request) {
	// Retrieve user, database
	dbOwner, dbName, err := com.GetOD(2, r) // 2 = Ignore "/x/execget/" at the start of the URL
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Required information is missing")
		return
	}
	dbFolder := "/"

	// Validate input
	input := com.VisGetFields{ // Reuse the visualisation names validation rules
		VisName: r.FormValue("sqlname"),
	}
	err = com.Validate.Struct(input)
	if err != nil {
		log.Printf("Input validation error for execGet(): %s", err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error when validating input: %s", err)
		return
	}
	sqlName := input.VisName

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
	allowed, err = com.CheckDBPermissions(loggedInUser, dbOwner, dbFolder, dbName, false)
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

	// Retrieve and return the text of a saved SQL statement
	var sqlText string
	sqlText, err = com.LiveExecuteSQLGet(dbOwner, sqlName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}
	if sqlText == "" {
		// No saved SQL statement was found
		w.WriteHeader(http.StatusNoContent)
		return
	}
	fmt.Fprintf(w, "%s", sqlText)
	return
}

// This function handles requests to save the database execution statement parameters
func execSave(w http.ResponseWriter, r *http.Request) {
	// Retrieve user and database name
	dbOwner, dbName, _, err := com.GetODC(2, r) // 2 = Ignore "/x/execsave/" at the start of the URL
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	dbFolder := "/"

	// SQL statement provided by the user
	rawSQL := r.FormValue("sql")

	// Initial sanity check of the SQL statements' name
	input := com.VisGetFields{ // Reuse the validation rules for saved visualisation names
		VisName: r.FormValue("sqlname"),
	}
	err = com.Validate.Struct(input)
	if err != nil {
		log.Printf("Input validation error for execSave(): %s", err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Error when validating input: %s", err)
		return
	}
	sqlName := input.VisName

	// Ensure minimum viable parameters are present
	if sqlName == "" || rawSQL == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the incoming SQLite query is "safe"
	var decodedStr string
	decodedStr, err = com.CheckUnicode(rawSQL)
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
	var allowed bool
	allowed, err = com.CheckDBPermissions(loggedInUser, dbOwner, dbFolder, dbName, true)
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

	// Ensure this is a live database
	var isLive bool
	isLive, _, err = com.CheckDBLive(dbOwner, dbFolder, dbName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}
	if !isLive {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Saving SQL statements is only supported for Live databases")
		return
	}

	// Save the SQL statement
	err = com.LiveExecuteSQLSave(dbOwner, sqlName, decodedStr)
	if err != nil {
		log.Printf("Error occurred when saving SQL statement '%s' for' '%s/%s': %s",
			com.SanitiseLogString(sqlName), com.SanitiseLogString(dbOwner), com.SanitiseLogString(dbName), err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Save succeeded
	w.WriteHeader(http.StatusOK)
}
