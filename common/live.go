package common

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sqlitebrowser/dbhub.io/common/config"

	sqlite "github.com/gwenn/gosqlite"
	"github.com/jackc/pgx/v5"
	pgpool "github.com/jackc/pgx/v5/pgxpool"
)

const (
	contextTimeout = 5 * time.Second
)

var (
	// JobListenConn is the PG server connection used for receiving PG notifications
	JobListenConn *pgx.Conn

	// JobQueueConn is the PG server connection used for submitting and retrieving jobs
	JobQueueConn *pgpool.Pool

	// JobQueueDebug tells the daemons whether or not to output debug messages while running job queue code
	// Mostly useful for development / debugging purposes.  0 means no debug messages, higher values means more verbosity
	JobQueueDebug = 0

	// Configuration info for the PostgreSQL job queue
	listenConfig *pgx.ConnConfig
)

// ConnectQueue creates the connections to the backend queue server
func ConnectQueue() (err error) {
	// Create the connection string for the dedicated PostgreSQL notification connection
	listenConfig, err = pgx.ParseConfig(fmt.Sprintf("host=%s port=%d user= %s password = %s dbname=%s connect_timeout=10", config.Conf.Pg.Server, uint16(config.Conf.Pg.Port), config.Conf.Pg.Username, config.Conf.Pg.Password, config.Conf.Pg.Database))
	if err != nil {
		return
	}
	listenTLSConfig := tls.Config{}
	if config.Conf.Environment.Environment == "production" {
		listenTLSConfig.ServerName = config.Conf.Pg.Server
		listenTLSConfig.InsecureSkipVerify = false
	} else {
		listenTLSConfig.InsecureSkipVerify = true
	}
	if config.Conf.Pg.SSL {
		listenConfig.TLSConfig = &listenTLSConfig
	} else {
		listenConfig.TLSConfig = nil
	}

	// Connect to PostgreSQL based queue server
	// Note: JobListenConn uses a dedicated, non-pooled connection to the job queue database, while JobQueueConn uses
	// a standard database connection pool
	JobListenConn, err = pgx.ConnectConfig(context.Background(), listenConfig)
	if err != nil {
		return fmt.Errorf("%s: couldn't connect to backend queue server: %v", config.Conf.Live.Nodename, err)
	}
	JobQueueConn, err = pgpool.New(context.Background(), pgConfig.ConnString())
	if err != nil {
		return fmt.Errorf("%s: couldn't connect to backend queue server: %v", config.Conf.Live.Nodename, err)
	}
	return
}

// LiveBackup asks the job queue backend to store the given database back into Minio
func LiveBackup(liveNode, loggedInUser, dbOwner, dbName string) (err error) {
	// Send the backup request to our job queue backend
	var resp JobResponseDBError
	err = JobSubmit(&resp, liveNode, "backup", loggedInUser, dbOwner, dbName, "")
	if err != nil {
		return
	}

	log.Printf("%s: node which handled the database backup request: %s", config.Conf.Live.Nodename, liveNode)

	// Handle error response from the live node
	if resp.Err != "" {
		err = errors.New(resp.Err)
		log.Printf("%s: an error was returned during database backup on '%s': '%v'", config.Conf.Live.Nodename, liveNode, resp.Err)
	}
	return
}

// LiveColumns requests the job queue backend to return a list of all columns of the given table
func LiveColumns(liveNode, loggedInUser, dbOwner, dbName, table string) (columns []sqlite.Column, pk []string, err error) {
	// Send the column list request to our job queue backend
	var resp JobResponseDBColumns
	err = JobSubmit(&resp, liveNode, "columns", loggedInUser, dbOwner, dbName, table)
	if err != nil {
		return
	}

	// Return the requested data
	columns = resp.Columns
	pk = resp.PkColumns

	// Handle error response from the live node
	if resp.Err != "" {
		err = errors.New(resp.Err)
		log.Printf("%s: an error was returned when retrieving the column list for '%s/%s': '%v'", config.Conf.Live.Nodename, dbOwner, dbName, resp.Err)
	}
	return
}

// LiveCreateDB requests the job queue backend create a new live SQLite database
func LiveCreateDB(dbOwner, dbName, objectID string) (liveNode string, err error) {
	// Send the database setup request to our job queue backend
	var resp JobResponseDBCreate
	err = JobSubmit(&resp, "any", "createdb", "", dbOwner, dbName, objectID)
	if err != nil {
		return
	}

	// Return the name of the node which has the database
	liveNode = resp.NodeName

	log.Printf("%s: node which handled the database creation request: %s", config.Conf.Live.Nodename, liveNode)

	// Handle error response from the live node
	if resp.Err != "" {
		err = errors.New(resp.Err)
		log.Printf("%s: an error was returned during database creation on '%s': '%v'", config.Conf.Live.Nodename, resp.NodeName, resp.Err)
	}
	return
}

// LiveDelete asks our job queue backend to delete a database
func LiveDelete(liveNode, loggedInUser, dbOwner, dbName string) (err error) {
	// Send the database setup request to our job queue backend
	var resp JobResponseDBError
	err = JobSubmit(&resp, liveNode, "delete", loggedInUser, dbOwner, dbName, "")
	if err != nil {
		return
	}

	// Handle error response from the live node
	if resp.Err != "" {
		err = errors.New(resp.Err)
		log.Printf("%s: an error was returned during database deletion on '%s': '%v'", config.Conf.Live.Nodename, liveNode, resp.Err)
	}
	return
}

// LiveExecute asks our job queue backend to execute a SQL statement on a database
func LiveExecute(liveNode, loggedInUser, dbOwner, dbName, sql string) (rowsChanged int, err error) {
	// Send the execute request to our job queue backend
	var resp JobResponseDBExecute
	err = JobSubmit(&resp, liveNode, "execute", loggedInUser, dbOwner, dbName, sql)
	if err != nil {
		return
	}

	// Return the number of rows changed by the execution run
	rowsChanged = resp.RowsChanged

	// Handle error response from the live node
	if resp.Err != "" {
		err = errors.New(resp.Err)
		if !strings.HasPrefix(err.Error(), "don't use exec with") {
			log.Printf("%s: an error was returned when retrieving the execution result for '%s/%s': '%v'", config.Conf.Live.Nodename, dbOwner, dbName, resp.Err)
		}
	}

	// If no error was thrown, then update the "last_modified" field for the database
	if err == nil {
		err = UpdateModified(dbOwner, dbName)
	}
	return
}

// LiveIndexes asks our job queue backend to provide the list of indexes in a database
func LiveIndexes(liveNode, loggedInUser, dbOwner, dbName string) (indexes []APIJSONIndex, err error) {
	// Send the index request to our job queue backend
	var resp JobResponseDBIndexes
	err = JobSubmit(&resp, liveNode, "indexes", loggedInUser, dbOwner, dbName, "")
	if err != nil {
		return
	}

	// Return the index list for the live database
	indexes = resp.Indexes

	// Handle error response from the live node
	if resp.Err != "" {
		err = errors.New(resp.Err)
		log.Printf("%s: an error was returned when retrieving the index list for '%s/%s': '%v'", config.Conf.Live.Nodename, dbOwner, dbName, resp.Err)
	}
	return
}

// LiveQuery sends a SQLite query to a live database on its hosting node
func LiveQuery(liveNode, loggedInUser, dbOwner, dbName, query string) (rows SQLiteRecordSet, err error) {
	// Send the query to our job queue backend
	var resp JobResponseDBQuery
	err = JobSubmit(&resp, liveNode, "query", loggedInUser, dbOwner, dbName, query)
	if err != nil {
		return
	}

	// Return the query response
	rows = resp.Results

	// Handle error response from the live node
	if resp.Err != "" {
		err = errors.New(resp.Err)
		log.Printf("%s: an error was returned when retrieving the query response for '%s/%s': '%v'", config.Conf.Live.Nodename, dbOwner, dbName, resp.Err)
	}
	return
}

// LiveRowData asks our job queue backend to send us the SQLite table data for a given range of rows
func LiveRowData(liveNode, loggedInUser, dbOwner, dbName string, reqData JobRequestRows) (rowData SQLiteRecordSet, err error) {
	// Serialise the row data request to JSON
	// NOTE - This actually causes the serialised field to be stored in PG as base64 instead.  Not sure why, but we can work with it.
	var reqJSON []byte
	reqJSON, err = json.Marshal(reqData)
	if err != nil {
		log.Println(err)
		return
	}

	// Send the row data request to our job queue backend
	var resp JobResponseDBRows
	err = JobSubmit(&resp, liveNode, "rowdata", loggedInUser, dbOwner, dbName, reqJSON)
	if err != nil {
		return
	}

	// Return the row data for the requested table
	rowData = resp.RowData

	// Handle error response from the live node
	if resp.Err != "" {
		err = errors.New(resp.Err)
		log.Printf("%s: an error was returned when retrieving the row data for '%s/%s': '%v'", config.Conf.Live.Nodename, dbOwner, dbName, resp.Err)
	}
	return
}

// LiveSize asks our job queue backend for the file size of a database
func LiveSize(liveNode, loggedInUser, dbOwner, dbName string) (size int64, err error) {
	// Send the size request to our job queue backend
	var resp JobResponseDBSize
	err = JobSubmit(&resp, liveNode, "size", loggedInUser, dbOwner, dbName, "")
	if err != nil {
		return
	}

	// Return the size of the live database
	size = resp.Size

	// Handle error response from the live node
	if resp.Err != "" {
		err = errors.New(resp.Err)
		log.Printf("%s: an error was returned when checking the on disk database size for '%s/%s': '%v'", config.Conf.Live.Nodename, dbOwner, dbName, resp.Err)
	}
	return
}

// LiveTables asks our job queue backend to provide the list of tables (not including views!) in a database
func LiveTables(liveNode, loggedInUser, dbOwner, dbName string) (tables []string, err error) {
	// Send the tables request to our job queue backend
	var resp JobResponseDBTables
	err = JobSubmit(&resp, liveNode, "tables", loggedInUser, dbOwner, dbName, "")
	if err != nil {
		return
	}

	// Return the table list for the live database
	tables = resp.Tables

	// Handle error response from the live node
	if resp.Err != "" {
		err = errors.New(resp.Err)
		log.Printf("%s: an error was returned when retrieving the table list for '%s/%s': '%v'", config.Conf.Live.Nodename, dbOwner, dbName, resp.Err)
	}
	return
}

// LiveTablesAndViews asks our job queue backend to provide the list of tables and views in a database
func LiveTablesAndViews(liveNode, loggedInUser, dbOwner, dbName string) (list []string, err error) {
	// Send the tables request to our job queue backend
	list, err = LiveTables(liveNode, loggedInUser, dbOwner, dbName)
	if err != nil {
		return
	}

	// Send the tables request to our job queue backend
	var vw []string
	vw, err = LiveViews(liveNode, loggedInUser, dbOwner, dbName)
	if err != nil {
		return
	}

	// Merge the table and view lists
	list = append(list, vw...)
	sort.Strings(list)
	return
}

// LiveViews asks our job queue backend to provide the list of views (not including tables!) in a database
func LiveViews(liveNode, loggedInUser, dbOwner, dbName string) (views []string, err error) {
	// Send the views request to our job queue backend
	var resp JobResponseDBViews
	err = JobSubmit(&resp, liveNode, "views", loggedInUser, dbOwner, dbName, "")
	if err != nil {
		return
	}

	// Return the view list for the live database
	views = resp.Views

	// Handle error response from the live node
	if resp.Err != "" {
		err = errors.New(resp.Err)
		log.Printf("%s: an error was returned when retrieving the view list for '%s/%s': '%v'", config.Conf.Live.Nodename, dbOwner, dbName, resp.Err)
	}
	return
}

// RemoveLiveDB deletes a live database from the local node.  For example, when the user deletes it from
// their account.
// Be aware, it leaves the database owners directory in place, to avoid any potential race condition of
// trying to delete that directory while other databases in their account are being worked with
func RemoveLiveDB(dbOwner, dbName string) (err error) {
	// Get the path to the database file, and it's containing directory
	dbDir := filepath.Join(config.Conf.Live.StorageDir, dbOwner, dbName)
	dbPath := filepath.Join(dbDir, "live.sqlite")
	if _, err = os.Stat(dbPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if JobQueueDebug > 0 {
				log.Printf("%s: database file '%s/%s' was supposed to get deleted here, but was missing from "+
					"filesystem path: '%s'", config.Conf.Live.Nodename, dbOwner, dbName, dbPath)
			}
			return
		}

		// Something wrong with the database file
		log.Println(err)
		return
	}

	// Delete the "live.sqlite" file
	// NOTE: If this seems to leave wal or other files hanging around in actual production use, we could
	//       instead use filepath.RemoveAll(dbDir).  That should kill the containing directory and
	//       all files within, thus not leave anything hanging around
	err = os.Remove(dbPath)
	if err != nil {
		log.Println(err)
		return
	}

	// Remove the containing directory
	err = os.Remove(dbDir)
	if err != nil {
		log.Println(err)
		return
	}

	if JobQueueDebug > 0 {
		log.Printf("%s: database file '%s/%s' removed from filesystem path: '%s'", config.Conf.Live.Nodename, dbOwner,
			dbName, dbPath)
	}
	return
}

// WaitForResponse waits for the job queue server to provide a response for a given job id
func WaitForResponse[T any](jobID int, resp *T) (err error) {
	// Add the response receiver
	responseChan := make(chan ResponseInfo)
	ResponseQueue.AddReceiver(jobID, &responseChan)

	// Wait for a response
	response := <-responseChan

	// Remove the response receiver
	ResponseQueue.RemoveReceiver(jobID)

	// Update the response status to 'processed' (should be fine done async)
	go ResponseComplete(response.responseID)

	// Unmarshall the response
	err = json.Unmarshal([]byte(response.payload), resp)
	if err != nil {
		err = fmt.Errorf("couldn't decode response payload: '%s'", err)
		log.Printf("%s: %s", config.Conf.Live.Nodename, err)
	}
	return
}
