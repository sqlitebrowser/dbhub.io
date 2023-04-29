package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	com "github.com/sqlitebrowser/dbhub.io/common"
)

func executePage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		DB         com.SQLiteDBinfo
		PageMeta   PageMetaInfo
		SqlHistory []com.SqlHistoryItem
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
	exists, err := com.CheckDBPermissions(pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, true)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s/%s' doesn't exist", dbName.Owner, dbName.Database))
		return
	}

	// * Execution can only get here if the user has access to the requested database *

	// Ensure this is a live database
	isLive, liveNode, err := com.CheckDBLive(dbName.Owner, dbName.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !isLive {
		errorPage(w, r, http.StatusBadRequest, "Executing SQL statements is only supported for Live databases")
		return
	}

	// Retrieve the database details
	err = com.DBDetails(&pageData.DB, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Ask the AMQP backend for the database file size
	pageData.DB.Info.DBEntry.Size, err = com.LiveSize(liveNode, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Get SQL history
	pageData.SqlHistory, err = com.LiveSqlHistoryGet(pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Fill out various metadata fields
	pageData.PageMeta.Title = fmt.Sprintf("Execute SQL - %s / %s", dbName.Owner, dbName.Database)
	pageData.PageMeta.PageSection = "db_exec"

	// Render the visualisation page
	t := tmpl.Lookup("executePage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// execClearHistory deletes all items in the user's SQL history
func execClearHistory(w http.ResponseWriter, r *http.Request) {
	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	var err error
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

	// Retrieve user and database info
	dbOwner, dbName, _, err := com.GetODC(2, r) // 2 = Ignore "/x/execlivesql/" at the start of the URL
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err)
		return
	}

	// Delete items
	err = com.LiveSqlHistoryDeleteOld(loggedInUser, dbOwner, dbName, 0) // 0 means "keep 0 items"
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}
}

// execLiveSQL executes a user provided SQLite statement on a database.
func execLiveSQL(w http.ResponseWriter, r *http.Request) {
	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	var err error
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

	// Retrieve user and database info
	dbOwner, dbName, _, err := com.GetODC(2, r) // 2 = Ignore "/x/execlivesql/" at the start of the URL
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err)
		return
	}

	// Grab the incoming SQLite query
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

	sql, err := com.CheckUnicode(data.Sql, false)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err)
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckDBPermissions(loggedInUser, dbOwner, dbName, true)
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

	// Make sure this is a live database
	isLive, liveNode, err := com.CheckDBLive(dbOwner, dbName)
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

	// In case of an error in the statement, save it in the terminal history as well.
	// This should not be registered too early because we do not want to save it when
	// there are validation or permission errors.
	var logError = func(e error) {
		// Store statement in sql terminal history
		err = com.LiveSqlHistoryAdd(loggedInUser, dbOwner, dbName, sql, com.Error, map[string]interface{}{"error": e.Error()})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err)
		}
	}

	// Send the SQL execution request to our AMQP backend
	var z interface{}
	rowsChanged, err := com.LiveExecute(liveNode, loggedInUser, dbOwner, dbName, sql)
	if err != nil {
		if !strings.HasPrefix(err.Error(), "don't use exec with") {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err)
			logError(err)
			return
		}

		// The user tried to run a SELECT query.  Let's just run with it...
		z, err = com.LiveQuery(liveNode, loggedInUser, dbOwner, dbName, sql)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err.Error())
			logError(err)
			return
		}

		// Store statement in sql terminal history
		err = com.LiveSqlHistoryAdd(loggedInUser, dbOwner, dbName, sql, com.Queried, z)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err)
		}
	} else {
		// The SQL statement execution succeeded, so pass along the # of rows changed
		z = com.ExecuteResponseContainer{RowsChanged: rowsChanged, Status: "OK"}

		// Store statement in sql terminal history
		err = com.LiveSqlHistoryAdd(loggedInUser, dbOwner, dbName, sql, com.Executed, z)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err)
		}
	}

	// Return the success message
	jsonData, err := json.Marshal(z)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		logError(err)
		return
	}
	fmt.Fprintf(w, "%s", jsonData)
}
