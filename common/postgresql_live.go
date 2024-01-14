package common

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/sqlitebrowser/dbhub.io/common/config"
	"github.com/sqlitebrowser/dbhub.io/common/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	// CheckJobQueue is used by the live daemons for triggering a check of the job queue
	CheckJobQueue chan struct{}

	// CheckResponsesQueue is used by the non-live daemons for triggering a check of the job responses queue
	CheckResponsesQueue chan struct{}

	// ResponseQueue is used to direct job queue responses back to the appropriate callers
	ResponseQueue *ResponseReceivers

	// SubmitterInstance is a random string generated at server start for identification purposes
	SubmitterInstance string
)

// JobQueueCheck checks if newly submitted work is available for processing
func JobQueueCheck() {
	if JobQueueDebug > 0 {
		log.Printf("%s: starting JobQueueCheck()...", config.Conf.Live.Nodename)
	}

	// Loop around checking for newly submitted jobs
	for range CheckJobQueue {
		if JobQueueDebug > 1 { // Only show when we have job queue debug verbosity turned up high
			log.Printf("%s: JobQueueCheck() received event", config.Conf.Live.Nodename)
		}

		// Retrieve job details from the database
		ctx := context.Background()
		tx, err := database.JobQueue.Begin(ctx)
		if err != nil {
			log.Printf("%s: error in JobQueueCheck(): %s", config.Conf.Live.Nodename, err)
			continue
		}

		// TODO: should we update the job state to 'error' on failure?

		dbQuery := `
			SELECT job_id, operation, submitter_node, details
			FROM job_submissions
			WHERE state = 'new'
		    	AND (target_node = 'any' OR target_node = $1)
				AND completed_date IS NULL
			ORDER BY submission_date ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1`
		var jobID int
		var details, subNode, op string
		err = tx.QueryRow(ctx, dbQuery, config.Conf.Live.Nodename).Scan(&jobID, &op, &subNode, &details)
		if err != nil {
			// Ignore any "no rows in result set" error
			if !errors.Is(err, pgx.ErrNoRows) {
				log.Printf("%v: retrieve job details error: %v", config.Conf.Live.Nodename, err)
			} else if JobQueueDebug > 1 { // Only show when we have job queue debug verbosity turned up high
				log.Printf("%s: --- No jobs waiting for processing ---", config.Conf.Live.Nodename)
			}
			tx.Rollback(ctx)
			continue
		}

		if JobQueueDebug > 0 {
			log.Printf("%s: picked up event for jobID = %d", config.Conf.Live.Nodename, jobID)
		}

		// Change the "state" field for the job entry to something other than 'new' so it's not unintentionally
		// picked up by future checks if something goes wrong before the job completes
		dbQuery = `
			UPDATE job_submissions
			SET state = 'in progress'
			WHERE job_id = $1`
		var t pgconn.CommandTag
		var responsePayload []byte
		t, err = tx.Exec(ctx, dbQuery, jobID)
		if err != nil {
			log.Printf("%s: error when updating job completion status to complete in backend database: %s", config.Conf.Live.Nodename, err)
			responsePayload = []byte(fmt.Sprintf(`{"error": "%s"}`, err))
		}

		// Safety check
		numRows := t.RowsAffected()
		if numRows != 1 {
			msg := fmt.Sprintf("something went wrong when updating jobID '%d' to 'in progress', number of rows updated = %d", jobID, numRows)
			log.Printf("%s: %s", config.Conf.Live.Nodename, msg)
			responsePayload = []byte(fmt.Sprintf(`{"error": "%s"}`, msg))
		}

		// Commit the transaction
		err = tx.Commit(ctx)
		if err != nil {
			log.Println(err)
		}

		// Unmarshal the job details
		var req JobRequest
		err = json.Unmarshal([]byte(details), &req)
		if err != nil {
			msg := fmt.Sprintf("error when unmarshalling job details: %v", err)
			log.Printf("%s: %s", config.Conf.Live.Nodename, msg)
			responsePayload = []byte(fmt.Sprintf(`{"error": "%s"}`, msg))
		}

		// Perform the desired operation
		switch op {
		case "backup":
			if JobQueueDebug > 0 {
				log.Printf("%s: running [BACKUP] on '%s/%s'", config.Conf.Live.Nodename, req.DBOwner, req.DBName)
			}

			// Return status of backup operation
			err = SQLiteBackupLive(config.Conf.Live.StorageDir, req.DBOwner, req.DBName)
			var response JobResponseDBError
			if err != nil {
				response.Err = err.Error()
			}
			responsePayload, err = json.Marshal(response) // Use an empty error message to indicate success
			if err != nil {
				log.Printf("%s: error when serialising backup response json: %s", config.Conf.Live.Nodename, err)
				responsePayload = []byte(fmt.Sprintf(`{"error": "%s"}`, err))
			}

		case "columns":
			if JobQueueDebug > 0 {
				log.Printf("%s: running [COLUMNS] on '%s/%s': '%s'", config.Conf.Live.Nodename, req.DBOwner, req.DBName, req.Data)
			}

			// Return the column list to the caller
			columns, pk, err, errCode := SQLiteGetColumnsLive(config.Conf.Live.StorageDir, req.DBOwner, req.DBName, fmt.Sprintf("%s", req.Data))
			response := JobResponseDBColumns{Columns: columns, PkColumns: pk, ErrCode: errCode}
			if err != nil {
				response.Err = err.Error()
			}
			responsePayload, err = json.Marshal(response)
			if err != nil {
				log.Printf("%s: error when serialising the column list response json: %s", config.Conf.Live.Nodename, err)
				responsePayload = []byte(fmt.Sprintf(`{"error": "%s"}`, err))
			}

		case "createdb":
			if JobQueueDebug > 0 {
				log.Printf("%s: running [CREATE DATABASE] on '%s/%s'", config.Conf.Live.Nodename, req.DBOwner, req.DBName)
			}

			// Return status of database creation
			err = JobQueueCreateDatabase(req)
			response := JobResponseDBCreate{NodeName: config.Conf.Live.Nodename}
			if err != nil {
				response.Err = err.Error()
			}
			responsePayload, err = json.Marshal(response) // Use an empty error message to indicate success
			if err != nil {
				log.Printf("%s: error when serialising create database response json: %s", config.Conf.Live.Nodename, err)
				responsePayload = []byte(fmt.Sprintf(`{"error": "%s"}`, err))
			}

		case "delete":
			if JobQueueDebug > 0 {
				log.Printf("%s: running [DELETE] on '%s/%s'", config.Conf.Live.Nodename, req.DBOwner, req.DBName)
			}

			// Delete the database file from the node
			err = RemoveLiveDB(req.DBOwner, req.DBName)
			var response JobResponseDBError
			if err != nil {
				response.Err = err.Error()
			}
			responsePayload, err = json.Marshal(response)
			if err != nil {
				log.Printf("%s: error when serialising delete database response json: %s", config.Conf.Live.Nodename, err)
				responsePayload = []byte(fmt.Sprintf(`{"error": "%s"}`, err))
			}

		case "execute":
			if JobQueueDebug > 0 {
				log.Printf("%s: running [EXECUTE] on '%s/%s': '%s'", config.Conf.Live.Nodename, req.DBOwner, req.DBName, req.Data)
			}

			// Execute a SQL statement on the database
			rowsChanged, err := SQLiteExecuteQueryLive(config.Conf.Live.StorageDir, req.DBOwner, req.DBName, req.RequestingUser, fmt.Sprintf("%s", req.Data))
			response := JobResponseDBExecute{RowsChanged: rowsChanged}
			if err != nil {
				response.Err = err.Error()
			}
			responsePayload, err = json.Marshal(response)
			if err != nil {
				log.Printf("%s: error when serialising execute request response json: %s", config.Conf.Live.Nodename, err)
				responsePayload = []byte(fmt.Sprintf(`{"error": "%s"}`, err))
			}

		case "indexes":
			if JobQueueDebug > 0 {
				log.Printf("%s: running [INDEXES] on '%s/%s'", config.Conf.Live.Nodename, req.DBOwner, req.DBName)
			}

			// Return the list of indexes
			indexes, err := SQLiteGetIndexesLive(config.Conf.Live.StorageDir, req.DBOwner, req.DBName)
			response := JobResponseDBIndexes{Indexes: indexes}
			if err != nil {
				response.Err = err.Error()
			}
			responsePayload, err = json.Marshal(response)
			if err != nil {
				log.Printf("%s: error when serialising index list response json: %s", config.Conf.Live.Nodename, err)
				responsePayload = []byte(fmt.Sprintf(`{"error": "%s"}`, err))
			}

		case "ping":
			// TODO: Write a ping responder so we can internally check if live nodes are responding

		case "query":
			if JobQueueDebug > 0 {
				log.Printf("%s: running [QUERY] on '%s/%s': '%s'", config.Conf.Live.Nodename, req.DBOwner, req.DBName, req.Data)
			}

			// Return the query result
			rows, err := SQLiteRunQueryLive(config.Conf.Live.StorageDir, req.DBOwner, req.DBName, req.RequestingUser, fmt.Sprintf("%s", req.Data))
			response := JobResponseDBQuery{Results: rows}
			if err != nil {
				response.Err = err.Error()
			}
			responsePayload, err = json.Marshal(response)
			if err != nil {
				log.Printf("%s: error when serialising query response json: %s", config.Conf.Live.Nodename, err)
				responsePayload = []byte(fmt.Sprintf(`{"error": "%s"}`, err))
			}

		case "rowdata":
			if JobQueueDebug > 0 {
				log.Printf("%s: running [ROWDATA] on '%s/%s'", config.Conf.Live.Nodename, req.DBOwner, req.DBName)
			}

			// Decode the base64 request data back to JSON
			b64, err := base64.StdEncoding.DecodeString(req.Data.(string))
			if err != nil {
				msg := fmt.Sprintf("error when base64 decoding rowdata job details: %v", err)
				log.Printf("%s: %s", config.Conf.Live.Nodename, msg)
				responsePayload = []byte(fmt.Sprintf(`{"error": "%s"}`, msg))
				break
			}

			// Extract the request information
			var reqData JobRequestRows
			err = json.Unmarshal(b64, &reqData)
			if err != nil {
				msg := fmt.Sprintf("error when unmarshalling rowdata job details: %v", err)
				log.Printf("%s: %s", config.Conf.Live.Nodename, msg)
				responsePayload = []byte(fmt.Sprintf(`{"error": "%s"}`, msg))
				break
			}
			dbTable := reqData.DbTable
			sortCol := reqData.SortCol
			sortDir := reqData.SortDir
			commitID := reqData.CommitID
			maxRows := reqData.MaxRows
			rowOffset := reqData.RowOffset

			// Read the desired row data and return it to the caller
			var tmpErr error
			resp := JobResponseDBRows{RowData: SQLiteRecordSet{}}
			resp.Tables, resp.DefaultTable, resp.RowData, resp.DatabaseSize, tmpErr =
				SQLiteReadDatabasePage("", "", req.RequestingUser, req.DBOwner, req.DBName, dbTable, sortCol, sortDir, commitID, rowOffset, maxRows, true)
			if tmpErr != nil {
				resp.Err = tmpErr.Error()
			}
			responsePayload, err = json.Marshal(resp)
			if err != nil {
				log.Printf("%s: error when serialising row data response json: %s", config.Conf.Live.Nodename, err)
				responsePayload = []byte(fmt.Sprintf(`{"error": "%s"}`, err))
			}

		case "size":
			if JobQueueDebug > 0 {
				log.Printf("%s: running [SIZE] on '%s/%s'", config.Conf.Live.Nodename, req.DBOwner, req.DBName)
			}

			// Return the on disk size of the database
			size, err := JobQueueGetSize(req.DBOwner, req.DBName)
			response := JobResponseDBSize{Size: size}
			if err != nil {
				response.Err = err.Error()
			}
			responsePayload, err = json.Marshal(response)
			if err != nil {
				log.Printf("%s: error when serialising size check response json: %s", config.Conf.Live.Nodename, err)
				responsePayload = []byte(fmt.Sprintf(`{"error": "%s"}`, err))
			}

		case "tables":
			if JobQueueDebug > 0 {
				log.Printf("%s: running [TABLES] on '%s/%s'", config.Conf.Live.Nodename, req.DBOwner, req.DBName)
			}

			// Return the list of tables
			tables, err := SQLiteGetTablesLive(config.Conf.Live.StorageDir, req.DBOwner, req.DBName)
			response := JobResponseDBTables{Tables: tables}
			if err != nil {
				response.Err = err.Error()
			}
			responsePayload, err = json.Marshal(response)
			if err != nil {
				log.Printf("%s: error when serialising table list response json: %s", config.Conf.Live.Nodename, err)
				responsePayload = []byte(fmt.Sprintf(`{"error": "%s"}`, err))
			}

		case "views":
			if JobQueueDebug > 0 {
				log.Printf("%s: running [VIEWS] on '%s/%s'", config.Conf.Live.Nodename, req.DBOwner, req.DBName)
			}

			// Return the list of views
			views, err := SQLiteGetViewsLive(config.Conf.Live.StorageDir, req.DBOwner, req.DBName)
			response := JobResponseDBViews{Views: views}
			if err != nil {
				response.Err = err.Error()
			}
			responsePayload, err = json.Marshal(response)
			if err != nil {
				log.Printf("%s: error when serialising view list response json: %s", config.Conf.Live.Nodename, err)
				responsePayload = []byte(fmt.Sprintf(`{"error": "%s"}`, err))
			}

		default:
			log.Printf("%v: notification received for unhandled operation '%s'\n", config.Conf.Live.Nodename, op)
		}

		// Update the job completion status in the backend database
		dbQuery = `
			UPDATE job_submissions
			SET state = 'complete', completed_date = now()
			WHERE job_id = $1`
		t, err = database.JobQueue.Exec(ctx, dbQuery, jobID)
		if err != nil {
			log.Printf("%s: error when updating job completion status to complete in backend database: %s", config.Conf.Live.Nodename, err)
			responsePayload = []byte(fmt.Sprintf(`{"error": "%s"}`, err))
		}

		// Safety check
		numRows = t.RowsAffected()
		if numRows != 1 {
			msg := fmt.Sprintf("something went wrong when updating jobID '%d' to 'complete', number of rows updated = %d", jobID, numRows)
			log.Printf("%s: %s", config.Conf.Live.Nodename, msg)
			responsePayload = []byte(fmt.Sprintf(`{"error": "%s"}`, msg))
		}

		// Add the response to the backend job queue database
		err = ResponseSubmit(jobID, subNode, responsePayload)
		if err != nil {
			log.Println(err)
		}
	}
}

// JobQueueCreateDatabase creates a database on a live node
func JobQueueCreateDatabase(req JobRequest) (err error) {
	// Set up the live database locally
	_, err = LiveRetrieveDatabaseMinio(config.Conf.Live.StorageDir, req.DBOwner, req.DBName, req.Data.(string))
	if err != nil {
		log.Println(err)
		// TODO: Update the job status to failed and notify the caller
		return
	}
	return
}

// JobQueueGetSize returns the on disk size of a database on a live node
func JobQueueGetSize(DBOwner, DBName string) (size int64, err error) {
	dbPath := filepath.Join(config.Conf.Live.StorageDir, DBOwner, DBName, "live.sqlite")
	var db os.FileInfo
	db, err = os.Stat(dbPath)
	if err != nil {
		return
	}

	// Return the database size to the caller
	size = db.Size()
	return
}

// JobQueueListen listens for database notify events indicating newly submitted jobs
func JobQueueListen() {
	if JobQueueDebug > 0 {
		// Log the start of the loop
		log.Printf("%v: started JobQueueListen()", config.Conf.Live.Nodename)
	}

	// Listen for notify events
	_, err := database.JobListen.Exec(context.Background(), "LISTEN job_submissions_queue")
	if err != nil {
		log.Fatal(err)
	}

	// Start the endless loop handling database notifications
	for {
		_, err := database.JobListen.WaitForNotification(context.Background())
		if err != nil {
			log.Printf("%s: error in JobQueueListen(): %s", config.Conf.Live.Nodename, err)
		}

		// Send an event to the goroutine that checks for submitted jobs
		CheckJobQueue <- struct{}{}
	}
	return
}

// JobSubmit submits job details to our PostgreSQL based job queue
func JobSubmit[T any](response *T, targetNode, operation, requestingUser, dbOwner, dbName string, data interface{}) (err error) {
	// Format the request details into a JSON structure
	req := JobRequest{
		Operation:      operation,
		DBOwner:        dbOwner,
		DBName:         dbName,
		Data:           data,
		RequestingUser: requestingUser,
	}
	var details []byte
	details, err = json.Marshal(req)
	if err != nil {
		log.Println(err)
		return
	}

	// Start a new transaction
	ctx := context.Background()
	tx, err := database.JobQueue.Begin(ctx)
	if err != nil {
		log.Println(err)
		return
	}
	defer tx.Rollback(ctx)

	// Safety check
	if SubmitterInstance == "" {
		err = fmt.Errorf("%s: ERROR - JobSubmit() called before SubmitterInstance was set", config.Conf.Live.Nodename)
		return
	}

	// Insert the job details
	dbQuery := `
		INSERT INTO job_submissions (target_node, operation, submitter_node, details)
		VALUES ($1, $2, $3, $4)
		RETURNING job_id`
	var jobID int
	err = tx.QueryRow(ctx, dbQuery, targetNode, operation, SubmitterInstance, details).Scan(&jobID)
	if err != nil {
		log.Printf("%s: error when adding a job to the backend job submission table: %v", config.Conf.Live.Nodename, err)
		return
	}

	// Double check the job was submitted ok
	if jobID == 0 {
		// Something went wrong when adding the new job
		err = fmt.Errorf("%s: something went wrong when adding the new job to the queue.  Returned job_id was 0", config.Conf.Live.Nodename)
		return
	}

	// Commit the transaction
	tx.Commit(ctx)

	if JobQueueDebug > 0 {
		log.Printf("%s: job '%d' added to queue", config.Conf.Live.Nodename, jobID)
	}

	// Wait for response
	err = WaitForResponse(jobID, &response)
	if err != nil {
		return
	}
	return
}

// ResponseQueueCheck checks if a newly submitted response is available for processing
func ResponseQueueCheck() {

	if JobQueueDebug > 0 {
		log.Printf("%s: starting ResponseQueueCheck()...", config.Conf.Live.Nodename)
	}

	// Loop around checking for newly submitted responses
	for range CheckResponsesQueue {

		if JobQueueDebug > 0 {
			log.Printf("%s: responseQueueCheck() received event", config.Conf.Live.Nodename)
		}

		// Check for new responses here
		dbQuery := `
			SELECT response_id, job_id, details
			FROM job_responses
			WHERE processed_date IS NULL
			AND submitter_node = $1
			ORDER BY response_date ASC`
		var jobID, responseID int
		var details string
		ctx := context.Background()
		rows, err := database.JobQueue.Query(ctx, dbQuery, SubmitterInstance)
		if err != nil {
			// Ignore any "no rows in result set" error
			if !errors.Is(err, pgx.ErrNoRows) {
				log.Printf("%v: retrieve response details error: %v", config.Conf.Live.Nodename, err)
			}
			continue
		}

		// For each new response, send its details to any matching waiting caller
		_, err = pgx.ForEachRow(rows, []any{&responseID, &jobID, &details}, func() error {
			if JobQueueDebug > 0 {
				log.Printf("%s: picked up response %d for jobID %d", config.Conf.Live.Nodename, responseID, jobID)
			}

			ResponseQueue.RLock()
			receiverChan, ok := ResponseQueue.receivers[jobID]
			if ok {
				*receiverChan <- ResponseInfo{jobID: jobID, responseID: responseID, payload: details}
			}
			ResponseQueue.RUnlock()
			return nil
		})
		if err != nil {
			log.Printf("%s: error in ResponseQueueCheck when running pgx.ForEachRow(): '%v' ", config.Conf.Live.Nodename, err)
			continue
		}
	}
}

// ResponseQueueListen listens for database notify events with responses from the other DBHub.io daemons
func ResponseQueueListen() {
	if JobQueueDebug > 0 {
		// Log the start of the loop
		log.Printf("%v: started ResponseQueueListen()", config.Conf.Live.Nodename)
	}

	// Listen for notify events
	if database.JobListen == nil {
		log.Fatalf("%v: ERROR, couldn't start ResponseQueueListen() as JobListenConn IS NILL", config.Conf.Live.Nodename)
	}
	if database.JobListen.IsClosed() {
		log.Fatalf("%v: ERROR, couldn't start ResponseQueueListen() as connection to job responses listener is NOT open", config.Conf.Live.Nodename)
	}
	_, err := database.JobListen.Exec(context.Background(), "LISTEN job_responses_queue")
	if err != nil {
		log.Fatal(err)
	}

	// Start the endless loop handling database notifications
	for {
		n, err := database.JobListen.WaitForNotification(context.Background())
		if err != nil {
			log.Printf("%s: error in ResponseQueueListen(): %s", config.Conf.Live.Nodename, err)
		}

		if JobQueueDebug > 0 && n.Payload == SubmitterInstance {
			log.Printf("%s: picked up response notification for submitter '%s'", config.Conf.Live.Nodename, n.Payload)
		}

		// Send an event to the response checking goroutine, letting it know there's a new response available
		if n.Payload == SubmitterInstance {
			CheckResponsesQueue <- struct{}{}
		}
	}
}

// ResponseComplete marks a response as processed
func ResponseComplete(responseID int) (err error) {
	// Start a new transaction
	ctx := context.Background()
	tx, err := database.JobQueue.Begin(ctx)
	if err != nil {
		log.Println(err)
		return
	}
	defer tx.Rollback(ctx)

	// Insert the response
	dbQuery := `
		UPDATE job_responses
		SET processed_date = now()
		WHERE response_id = $1`
	tag, err := tx.Exec(ctx, dbQuery, responseID)
	if err != nil {
		log.Printf("%s: error when updating a response in the backend job responses table: %v", config.Conf.Live.Nodename, err)
		return
	}

	// Double check the response was updated ok
	numRows := tag.RowsAffected()
	if numRows != 1 {
		err = fmt.Errorf("%s: something went wrong when updating a response in the job responses table.  Number of rows affected (%d) wasn't 1'", config.Conf.Live.Nodename, numRows)
		return
	}

	// Commit the transaction
	tx.Commit(ctx)
	return
}

// ResponseSubmit adds a response to the job_responses table
func ResponseSubmit(jobID int, submitterNode string, payload []byte) (err error) {
	// Start a new transaction
	ctx := context.Background()
	tx, err := database.JobQueue.Begin(ctx)
	if err != nil {
		log.Println(err)
		return
	}
	defer tx.Rollback(ctx)

	// Insert the response details
	dbQuery := `
		INSERT INTO job_responses (job_id, submitter_node, details)
		VALUES ($1, $2, $3)`
	tag, err := tx.Exec(ctx, dbQuery, jobID, submitterNode, payload)
	if err != nil {
		log.Printf("%s: error when adding a response to the backend job responses table: %v", config.Conf.Live.Nodename, err)
		return
	}

	// Double check the response was added ok
	numRows := tag.RowsAffected()
	if numRows != 1 {
		err = fmt.Errorf("%s: something went wrong when adding the new response to the job responses table.  Number of rows (%d) wasn't 1", config.Conf.Live.Nodename, numRows)
		return
	}

	// Commit the transaction
	tx.Commit(ctx)
	return
}
