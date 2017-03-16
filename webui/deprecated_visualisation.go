package main

import (
	"net/http"
	"log"
	"github.com/icza/session"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"encoding/json"

	com "github.com/sqlitebrowser/dbhub.io/common"
)

func main() {
	http.HandleFunc("/x/visdata/", logReq(visData))
}

// Receives a request for specific table data from the front end, returning it as JSON
func visData(w http.ResponseWriter, r *http.Request) {
	pageName := "Visualisation data handler"

	var pageData struct {
		Meta com.MetaInfo
		DB   com.SQLiteDBinfo
		Data com.SQLiteRecordSet
	}

	// Retrieve user, database, and table name
	userName, dbName, requestedTable, err := com.GetODT(2, r) // 1 = Ignore "/x/table/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if X and Y column names were given
	var reqXCol, reqYCol, xCol, yCol string
	reqXCol = r.FormValue("xcol")
	reqYCol = r.FormValue("ycol")

	// Validate column names if present
	// FIXME: Create a proper validation function for SQLite column names
	if reqXCol != "" {
		err = com.ValidatePGTable(reqXCol)
		if err != nil {
			log.Printf("Validation failed for SQLite column name: %s", err)
			return
		}
		xCol = reqXCol
	}
	if reqYCol != "" {
		err = com.ValidatePGTable(reqYCol)
		if err != nil {
			log.Printf("Validation failed for SQLite column name: %s", err)
			return
		}
		yCol = reqYCol
	}

	// Validate WHERE clause values if present
	var reqWCol, reqWType, reqWVal, wCol, wType, wVal string
	reqWCol = r.FormValue("wherecol")
	reqWType = r.FormValue("wheretype")
	reqWVal = r.FormValue("whereval")

	// WHERE column
	if reqWCol != "" {
		err = com.ValidatePGTable(reqWCol)
		if err != nil {
			log.Printf("Validation failed for SQLite column name: %s", err)
			return
		}
		wCol = reqWCol
	}

	// WHERE type
	switch reqWType {
	case "":
		// We don't pass along empty values
	case "LIKE", "=", "!=", "<", "<=", ">", ">=":
		wType = reqWType
	default:
		// This should never be reached
		log.Printf("%s: Validation failed on WHERE clause type. wType = '%v'\n", pageName, wType)
		return
	}

	// TODO: Add ORDER BY clause
	// TODO: We'll probably need some kind of optional data transformation for columns too
	// TODO    eg column foo → DATE (type)

	// WHERE value
	var whereClauses []com.WhereClause
	if reqWVal != "" && wType != "" {
		whereClauses = append(whereClauses, com.WhereClause{Column: wCol, Type: wType, Value: reqWVal})

		// TODO: Double check if we should be filtering out potentially devious characters here. I don't
		// TODO  (at the moment) *think* we need to, as we're using parameter binding on the passed in values
		wVal = reqWVal
	}

	// Retrieve session data (if any)
	var loggedInUser string
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
		} else {
			session.Remove(sess, w)
		}
	}

	// Check if the user has access to the requested database
	err = com.CheckUserDBAccess(&pageData.DB, loggedInUser, userName, dbName)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// * Execution can only get here if the user has access to the requested database *

	// Generate a predictable cache key for the JSON data
	var pageCacheKey string
	if loggedInUser != userName {
		tempArr := md5.Sum([]byte(userName + "/" + dbName + "/" + requestedTable + xCol + yCol + wCol +
			wType + wVal))
		pageCacheKey = "visdat-pub-" + hex.EncodeToString(tempArr[:])
	} else {
		tempArr := md5.Sum([]byte(loggedInUser + "-" + userName + "/" + dbName + "/" + requestedTable +
			xCol + yCol + wCol + wType + wVal))
		pageCacheKey = "visdat-" + hex.EncodeToString(tempArr[:])
	}

	// If a cached version of the page data exists, use it
	var jsonResponse []byte
	ok, err := com.GetCachedData(pageCacheKey, &jsonResponse)
	if err != nil {
		log.Printf("%s: Error retrieving page data from cache: %v\n", pageName, err)
	}
	if ok {
		// Render the JSON response from cache
		fmt.Fprintf(w, "%s", jsonResponse)
		return
	}

	// Get a handle from Minio for the database object
	sdb, err := com.OpenMinioObject(pageData.DB.MinioBkt, pageData.DB.MinioId)
	if err != nil {
		return
	}

	// Retrieve the list of tables in the database
	tables, err := sdb.Tables("")
	if err != nil {
		log.Printf("%s: Error retrieving table names: %s", pageName, err)
		return
	}
	if len(tables) == 0 {
		// No table names were returned, so abort
		log.Printf("%s: The database '%s' doesn't seem to have any tables. Aborting.", pageName, dbName)
		return
	}
	pageData.DB.Info.Tables = tables

	// If a specific table was requested, check that it's present
	var dbTable string
	if requestedTable != "" {
		// Check the requested table is present
		for _, tbl := range tables {
			if tbl == requestedTable {
				dbTable = requestedTable
			}
		}
	}

	// If a specific table wasn't requested, use the first table in the database
	if dbTable == "" {
		dbTable = pageData.DB.Info.Tables[0]
	}

	// Retrieve the table data requested by the user
	maxVals := 2500 // 2500 row maximum for now
	if xCol != "" && yCol != "" {
		pageData.Data, err = com.ReadSQLiteDBCols(sdb, requestedTable, true, true, maxVals, whereClauses, xCol, yCol)
	} else {
		pageData.Data, err = com.ReadSQLiteDB(sdb, requestedTable, maxVals)
	}
	if err != nil {
		// Some kind of error when reading the database data
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Use json.MarshalIndent() for nicer looking output
	jsonResponse, err = json.Marshal(pageData.Data)
	if err != nil {
		log.Println(err)
		return
	}

	// Cache the JSON data
	err = com.CacheData(pageCacheKey, jsonResponse, com.CacheTime)
	if err != nil {
		log.Printf("%s: Error when caching JSON data: %v\n", pageName, err)
	}

	//w.Header().Set("Access-Control-Allow-Origin", "*")
	fmt.Fprintf(w, "%s", jsonResponse)
}

