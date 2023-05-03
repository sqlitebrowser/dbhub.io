package main

// Internal daemon for running SQLite queries sent by the other DBHub.io daemons

// FIXME: Note that all incoming AMQP requests _other_ than for database creation
//        are handled by the same single goroutine.  This should be changed to
//        something smarter, such as using a pool of worker goroutines to handle
//        the requests.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	sqlite "github.com/gwenn/gosqlite"
	com "github.com/sqlitebrowser/dbhub.io/common"
)

func main() {
	// Read server configuration
	err := com.ReadConfig()
	if err != nil {
		log.Fatalf("Configuration file problem: '%s'", err)
	}

	// If node name and base directory were provided on the command line, then override the config file values
	if len(os.Args) == 3 {
		com.Conf.Live.Nodename = os.Args[1]
		com.Conf.Live.StorageDir = os.Args[2]
	}

	// If we don't have the node name or storage dir after reading both the config and command line, then abort
	if com.Conf.Live.Nodename == "" || com.Conf.Live.StorageDir == "" {
		log.Fatal("Node name or Storage directory missing.  Aborting")
	}

	// If it doesn't exist, create the base directory for storing SQLite files
	_, err = os.Stat(com.Conf.Live.StorageDir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Fatal(err)
		}

		// The target location doesn't exist
		err = os.MkdirAll(com.Conf.Live.StorageDir, 0750)
		if err != nil {
			log.Fatal(err)
		}
	}

	// Connect to Minio server
	err = com.ConnectMinio()
	if err != nil {
		log.Fatal(err)
	}

	// Connect to PostgreSQL server
	err = com.ConnectPostgreSQL()
	if err != nil {
		log.Fatal(err)
	}

	// Connect to MQ server
	ch, err := com.ConnectMQ()
	if err != nil {
		log.Fatal(err)
	}

	// Create queue for receiving new database creation requests
	createQueue, err := com.MQCreateDBQueue(ch)
	if err != nil {
		log.Fatal(err)
	}

	// Start consuming database creation requests
	createDBMsgs, err := ch.Consume(createQueue.Name, "", false, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		for d := range createDBMsgs {
			// Decode JSON request
			var req com.LiveDBRequest
			err = json.Unmarshal(d.Body, &req)
			if err != nil {
				log.Println(err)
				err = com.MQCreateResponse(d, ch, com.Conf.Live.Nodename, "failure")
				if err != nil {
					log.Printf("Error: occurred on live node '%s' in the create db code, while constructing an AMQP error message response: '%s'", com.Conf.Live.Nodename, err)
				}
				continue
			}

			// Verify that the object ID was passed through the interface correctly
			objectID, ok := req.Data.(string)
			if !ok {
				err = com.MQCreateResponse(d, ch, com.Conf.Live.Nodename, "failure")
				if err != nil {
					log.Printf("Error: occurred on live node '%s' in the create db code, while converting the Minio object ID to a string: '%s'", com.Conf.Live.Nodename, err)
				}
				continue
			}

			// Set up the live database locally
			_, err = com.LiveRetrieveDatabaseMinio(com.Conf.Live.StorageDir, req.DBOwner, req.DBName, objectID)
			if err != nil {
				log.Println(err)
				err = com.MQCreateResponse(d, ch, com.Conf.Live.Nodename, "failure")
				if err != nil {
					log.Printf("Error: occurred on live node '%s' in the create db code, while constructing an AMQP error message response (location 2): '%s'", com.Conf.Live.Nodename, err)
				}
				continue
			}

			// Respond to the creation request with a success message
			err = com.MQCreateResponse(d, ch, com.Conf.Live.Nodename, "success")
			if err != nil {
				continue
			}
		}
	}()

	// Create the queue for receiving database queries
	queryQueue, err := com.MQCreateQueryQueue(ch, com.Conf.Live.Nodename)
	if err != nil {
		log.Fatal(err)
	}

	// Start consuming database query requests
	requests, err := ch.Consume(queryQueue.Name, "", false, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		for msg := range requests {
			if com.AmqpDebug > 1 {
				log.Printf("'%s' received AMQP REQUEST (of not-yet-determined type)", com.Conf.Live.Nodename)
			}

			// Decode JSON request
			var req com.LiveDBRequest
			err = json.Unmarshal(msg.Body, &req)
			if err != nil {
				resp := com.LiveDBErrorResponse{Node: com.Conf.Live.Nodename, Error: err.Error()}
				err = com.MQResponse("NOT-YET-DETERMINED", msg, ch, com.Conf.Live.Nodename, resp)
				if err != nil {
					log.Printf("Error: occurred on '%s' the main live node switch{} while constructing an AMQP error message response: '%s'", com.Conf.Live.Nodename, err)
				}
				continue
			}

			if com.AmqpDebug > 1 {
				log.Printf("Decoded request on '%s'.  Correlation ID: '%s', request operation: '%s', request query: '%v'", com.Conf.Live.Nodename, msg.CorrelationId, req.Operation, req.Data)
			} else if com.AmqpDebug == 1 {
				log.Printf("Decoded request on '%s'.  Correlation ID: '%s', request operation: '%s'", com.Conf.Live.Nodename, msg.CorrelationId, req.Operation)
			}

			// Handle each operation
			switch req.Operation {
			case "backup":
				err = com.SQLiteBackupLive(com.Conf.Live.StorageDir, req.DBOwner, req.DBName)
				if err != nil {
					resp := com.LiveDBErrorResponse{Node: com.Conf.Live.Nodename, Error: err.Error()}
					err = com.MQResponse("BACKUP", msg, ch, com.Conf.Live.Nodename, resp)
					if err != nil {
						log.Printf("Error: occurred on '%s' in MQResponse() while constructing an AMQP error message response: '%s'", com.Conf.Live.Nodename, err)
					}
					continue
				}

				if com.AmqpDebug > 0 {
					log.Printf("Running [BACKUP] on '%s/%s'", req.DBOwner, req.DBName)
				}

				// Return a success message to the caller
				resp := com.LiveDBErrorResponse{Node: com.Conf.Live.Nodename, Error: ""} // Use an empty error message to indicate success
				err = com.MQResponse("BACKUP", msg, ch, com.Conf.Live.Nodename, resp)
				if err != nil {
					log.Printf("Error: occurred on '%s' in MQResponse() while constructing the AMQP backup response: '%s'", com.Conf.Live.Nodename, err)
				}
				continue

			case "columns":
				columns, pk, err, errCode := com.SQLiteGetColumnsLive(com.Conf.Live.StorageDir, req.DBOwner, req.DBName, fmt.Sprintf("%s", req.Data))
				if err != nil {
					resp := com.LiveDBColumnsResponse{Node: com.Conf.Live.Nodename, Columns: []sqlite.Column{}, PkColumns: nil, Error: err.Error(), ErrCode: errCode}
					err = com.MQResponse("COLUMNS", msg, ch, com.Conf.Live.Nodename, resp)
					if err != nil {
						log.Printf("Error: occurred on '%s' in MQResponse() while constructing an AMQP error message response: '%s'", com.Conf.Live.Nodename, err)
					}
					continue
				}

				if com.AmqpDebug > 0 {
					log.Printf("Running [COLUMNS] on '%s/%s': '%s'", req.DBOwner, req.DBName, req.Data)
				}

				// Return the columns list to the caller
				resp := com.LiveDBColumnsResponse{Node: com.Conf.Live.Nodename, Columns: columns, PkColumns: pk, Error: "", ErrCode: com.AMQPNoError}
				err = com.MQResponse("COLUMNS", msg, ch, com.Conf.Live.Nodename, resp)
				if err != nil {
					log.Printf("Error: occurred on '%s' in MQResponse() while constructing the AMQP columns list response: '%s'", com.Conf.Live.Nodename, err)
				}
				continue

			case "delete":
				// Delete the database file on the node
				err = removeLiveDB(req.DBOwner, req.DBName)
				if err != nil {
					resp := com.LiveDBErrorResponse{Node: com.Conf.Live.Nodename, Error: err.Error()}
					err = com.MQResponse("DELETE", msg, ch, com.Conf.Live.Nodename, resp)
					if err != nil {
						log.Printf("Error: occurred on '%s' in MQResponse() while constructing an AMQP error message response: '%s'", com.Conf.Live.Nodename, err)
					}
					continue
				}

				if com.AmqpDebug > 0 {
					log.Printf("Running [DELETE] on '%s/%s'", req.DBOwner, req.DBName)
				}

				// Return a success message (empty string in this case) to the caller
				resp := com.LiveDBErrorResponse{Node: com.Conf.Live.Nodename, Error: ""}
				err = com.MQResponse("DELETE", msg, ch, com.Conf.Live.Nodename, resp)
				if err != nil {
					log.Printf("Error: occurred on '%s' in MQResponse() while constructing the AMQP delete database response: '%s'", com.Conf.Live.Nodename, err)
				}
				continue

			case "execute":
				// Execute a SQL statement on the database file
				var rowsChanged int
				rowsChanged, err = com.SQLiteExecuteQueryLive(com.Conf.Live.StorageDir, req.DBOwner, req.DBName, req.RequestingUser, fmt.Sprintf("%s", req.Data))
				if err != nil {
					resp := com.LiveDBExecuteResponse{Node: com.Conf.Live.Nodename, RowsChanged: 0, Error: err.Error()}
					err = com.MQResponse("EXECUTE", msg, ch, com.Conf.Live.Nodename, resp)
					if err != nil {
						log.Printf("Error: occurred on '%s' in MQResponse() while constructing an AMQP error message response: '%s'", com.Conf.Live.Nodename, err)
					}
					continue
				}

				if com.AmqpDebug > 0 {
					log.Printf("Running [EXECUTE] on '%s/%s': '%s'", req.DBOwner, req.DBName, req.Data)
				}

				// Return a success message to the caller
				resp := com.LiveDBExecuteResponse{Node: com.Conf.Live.Nodename, RowsChanged: rowsChanged, Error: ""}
				err = com.MQResponse("EXECUTE", msg, ch, com.Conf.Live.Nodename, resp)
				if err != nil {
					log.Printf("Error: occurred on '%s' in MQResponse() while constructing the AMQP execute query response: '%s'", com.Conf.Live.Nodename, err)
				}
				continue

			case "indexes":
				var indexes []com.APIJSONIndex
				indexes, err = com.SQLiteGetIndexesLive(com.Conf.Live.StorageDir, req.DBOwner, req.DBName)
				if err != nil {
					resp := com.LiveDBIndexesResponse{Node: com.Conf.Live.Nodename, Indexes: []com.APIJSONIndex{}, Error: err.Error()}
					err = com.MQResponse("INDEXES", msg, ch, com.Conf.Live.Nodename, resp)
					if err != nil {
						log.Printf("Error: occurred on '%s' in MQResponse() while constructing an AMQP error message response: '%s'", com.Conf.Live.Nodename, err)
					}
					continue
				}

				if com.AmqpDebug > 0 {
					log.Printf("Running [INDEXES] on '%s/%s'", req.DBOwner, req.DBName)
				}

				// Return the indexes list to the caller
				resp := com.LiveDBIndexesResponse{Node: com.Conf.Live.Nodename, Indexes: indexes, Error: ""}
				err = com.MQResponse("INDEXES", msg, ch, com.Conf.Live.Nodename, resp)
				if err != nil {
					log.Printf("Error: occurred on '%s' in MQResponse() while constructing the AMQP indexes list response: '%s'", com.Conf.Live.Nodename, err)
				}
				continue

			case "query":
				var rows com.SQLiteRecordSet
				rows, err = com.SQLiteRunQueryLive(com.Conf.Live.StorageDir, req.DBOwner, req.DBName, req.RequestingUser, fmt.Sprintf("%s", req.Data))
				if err != nil {
					resp := com.LiveDBQueryResponse{Node: com.Conf.Live.Nodename, Results: com.SQLiteRecordSet{}, Error: err.Error()}
					err = com.MQResponse("QUERY", msg, ch, com.Conf.Live.Nodename, resp)
					if err != nil {
						log.Printf("Error: occurred on '%s' in MQResponse() while constructing an AMQP error message response: '%s'", com.Conf.Live.Nodename, err)
					}
					continue
				}

				if com.AmqpDebug > 0 {
					log.Printf("Running [QUERY] on '%s/%s': '%s'", req.DBOwner, req.DBName, req.Data)
				}

				// Return the query response to the caller
				resp := com.LiveDBQueryResponse{Node: com.Conf.Live.Nodename, Results: rows, Error: ""}
				err = com.MQResponse("QUERY", msg, ch, com.Conf.Live.Nodename, resp)
				if err != nil {
					log.Printf("Error: occurred on '%s' in MQResponse() while constructing the AMQP query response: '%s'", com.Conf.Live.Nodename, err)
				}
				continue

			case "rowdata":
				// Extract the request information
				// FIXME: Add type checks for safety instead of blind coercing
				reqData := req.Data.(map[string]interface{})
				dbTable := reqData["db_table"].(string)
				sortCol := reqData["sort_col"].(string)
				sortDir := reqData["sort_dir"].(string)
				commitID := reqData["commit_id"].(string)
				maxRows := int(reqData["max_rows"].(float64))
				rowOffset := int(reqData["row_offset"].(float64))

				// Open the SQLite database and read the row data
				resp := com.LiveDBRowsResponse{Node: com.Conf.Live.Nodename, RowData: com.SQLiteRecordSet{}}
				resp.Tables, resp.DefaultTable, resp.RowData, resp.DatabaseSize, err =
					com.SQLiteReadDatabasePage("", "", req.RequestingUser, req.DBOwner, req.DBName, dbTable, sortCol, sortDir, commitID, rowOffset, maxRows, true)
				if err != nil {
					resp := com.LiveDBErrorResponse{Node: com.Conf.Live.Nodename, Error: err.Error()}
					err = com.MQResponse("ROWDATA", msg, ch, com.Conf.Live.Nodename, resp)
					if err != nil {
						log.Printf("Error: occurred on '%s' in MQResponse() while constructing an AMQP error message response: '%s'", com.Conf.Live.Nodename, err)
					}
					continue
				}

				if com.AmqpDebug > 0 {
					log.Printf("Running [ROWDATA] on '%s/%s'", req.DBOwner, req.DBName)
				}

				// Return the row data to the caller
				err = com.MQResponse("ROWDATA", msg, ch, com.Conf.Live.Nodename, resp)
				if err != nil {
					log.Printf("Error: occurred on '%s' in MQResponse() while constructing the AMQP query response: '%s'", com.Conf.Live.Nodename, err)
				}
				continue

			case "size":
				dbPath := filepath.Join(com.Conf.Live.StorageDir, req.DBOwner, req.DBName, "live.sqlite")
				var db os.FileInfo
				db, err = os.Stat(dbPath)
				if err != nil {
					resp := com.LiveDBSizeResponse{Node: com.Conf.Live.Nodename, Size: 0, Error: err.Error()}
					err = com.MQResponse("SIZE", msg, ch, com.Conf.Live.Nodename, resp)
					if err != nil {
						log.Printf("Error: occurred on '%s' in MQResponse() while constructing an AMQP error message response: '%s'", com.Conf.Live.Nodename, err)
					}
					continue
				}

				if com.AmqpDebug > 0 {
					log.Printf("Running [SIZE] on '%s/%s'", req.DBOwner, req.DBName)
				}

				// Return the database size to the caller
				resp := com.LiveDBSizeResponse{Node: com.Conf.Live.Nodename, Size: db.Size(), Error: ""}
				err = com.MQResponse("SIZE", msg, ch, com.Conf.Live.Nodename, resp)
				if err != nil {
					log.Printf("Error: occurred on '%s' in MQResponse() while constructing the AMQP size response: '%s'", com.Conf.Live.Nodename, err)
				}
				continue

			case "tables":
				var tables []string
				tables, err = com.SQLiteGetTablesLive(com.Conf.Live.StorageDir, req.DBOwner, req.DBName)
				if err != nil {
					resp := com.LiveDBTablesResponse{Node: com.Conf.Live.Nodename, Tables: nil, Error: err.Error()}
					err = com.MQResponse("TABLES", msg, ch, com.Conf.Live.Nodename, resp)
					if err != nil {
						log.Printf("Error: occurred on '%s' in MQResponse() while constructing an AMQP error message response: '%s'", com.Conf.Live.Nodename, err)
					}
					continue
				}

				if com.AmqpDebug > 0 {
					log.Printf("Running [TABLES] on '%s/%s'", req.DBOwner, req.DBName)
				}

				// Return the tables list to the caller
				resp := com.LiveDBTablesResponse{Node: com.Conf.Live.Nodename, Tables: tables, Error: ""}
				err = com.MQResponse("TABLES", msg, ch, com.Conf.Live.Nodename, resp)
				if err != nil {
					log.Printf("Error: occurred on '%s' in MQResponse() while constructing the AMQP tables list response: '%s'", com.Conf.Live.Nodename, err)
				}
				continue

			case "views":
				var views []string
				views, err = com.SQLiteGetViewsLive(com.Conf.Live.StorageDir, req.DBOwner, req.DBName)
				if err != nil {
					resp := com.LiveDBViewsResponse{Node: com.Conf.Live.Nodename, Views: nil, Error: err.Error()}
					err = com.MQResponse("VIEWS", msg, ch, com.Conf.Live.Nodename, resp)
					if err != nil {
						log.Printf("Error: occurred on '%s' in MQResponse() while constructing an AMQP error message response: '%s'", com.Conf.Live.Nodename, err)
					}
					continue
				}

				if com.AmqpDebug > 0 {
					log.Printf("Running [VIEWS] on '%s/%s'", req.DBOwner, req.DBName)
				}

				// Return the views list to the caller
				resp := com.LiveDBViewsResponse{Node: com.Conf.Live.Nodename, Views: views, Error: ""}
				err = com.MQResponse("VIEWS", msg, ch, com.Conf.Live.Nodename, resp)
				if err != nil {
					log.Printf("Error: occurred on '%s' in MQResponse() while constructing the AMQP views list response: '%s'", com.Conf.Live.Nodename, err)
				}
				continue

			default:
				log.Printf("'%s' received unknown '%s' request on this queue for %s/%s", com.Conf.Live.Nodename, req.Operation, req.DBOwner, req.DBName)
			}
		}
	}()

	log.Printf("Live server '%s' listening for requests", com.Conf.Live.Nodename)

	// Endless loop
	var forever chan struct{}
	<-forever

	// Close the channel to the MQ server
	_ = com.CloseMQChannel(ch)
}

// RemoveLiveDB deletes a live database from the local node.  For example, when the user deletes it from
// their account.
// Be aware, it leaves the database owners directory in place, to avoid any potential race condition of
// trying to delete that directory while other databases in their account are being worked with
func removeLiveDB(dbOwner, dbName string) (err error) {
	// Get the path to the database file, and it's containing directory
	dbDir := filepath.Join(com.Conf.Live.StorageDir, dbOwner, dbName)
	dbPath := filepath.Join(dbDir, "live.sqlite")
	if _, err = os.Stat(dbPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if com.AmqpDebug > 0 {
				log.Printf("Live node '%s': Database file '%s/%s' was requested to be deletet, but was missing " +
					"from filesystem path: '%s'", com.Conf.Live.Nodename, dbOwner, dbName, dbPath)
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

	if com.AmqpDebug > 0 {
		log.Printf("Live node '%s': Database file '%s/%s' removed from filesystem path: '%s'",
			com.Conf.Live.Nodename, dbOwner, dbName, dbPath)
	}
	return
}
