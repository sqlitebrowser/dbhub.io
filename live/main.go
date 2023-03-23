package main

// Internal daemon for running SQLite queries sent by the other DBHub.io daemons

// FIXME: Note that all incoming AMQP requests _other_ than for database creation
//        are handled by the same single goroutine.  This should be changed to
//        something smarter, such as using a pool of worker goroutines to handle
//        the requests.

import (
	"encoding/json"
	"errors"
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
		log.Fatalf("Configuration file problem\n\n%v", err)
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

			// Set up the live database locally
			err = setupLiveDB(req.DBOwner, req.DBName)
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
			if com.AmqpDebug {
				log.Printf("'%s' received AMQP REQUEST (of not-yet-determined type)", com.Conf.Live.Nodename)
			}

			// Decode JSON request
			var req com.LiveDBRequest
			err = json.Unmarshal(msg.Body, &req)
			if err != nil {
				log.Println(err)
				err = com.MQErrorResponse(msg, ch, com.Conf.Live.Nodename, err.Error())
				if err != nil {
					log.Printf("Error: occurred on '%s' the main live node switch{} while constructing an AMQP error message response: '%s'", com.Conf.Live.Nodename, err)
				}
				continue
			}

			if com.AmqpDebug {
				log.Printf("Decoded request on '%s'.  Correlation ID: '%s', request operation: '%s', request query: '%s'", com.Conf.Live.Nodename, msg.CorrelationId, req.Operation, req.Query)
			}

			// Handle each operation
			switch req.Operation {
			case "columns":
				var columns []sqlite.Column
				columns, err = com.SQLiteGetColumnsLive(com.Conf.Live.StorageDir, req.DBOwner, req.DBName, req.Query) // We use the req.Query field to pass the table name
				if err != nil {
					err = com.MQColumnsResponse(msg, ch, com.Conf.Live.Nodename, nil, err.Error())
					if err != nil {
						log.Printf("Error: occurred on '%s' in MQColumnsResponse() while constructing an AMQP error message response: '%s'", com.Conf.Live.Nodename, err)
					}
					continue
				}

				// Return the columns list to the caller
				err = com.MQColumnsResponse(msg, ch, com.Conf.Live.Nodename, columns, "")
				if err != nil {
					log.Printf("Error: occurred on '%s' in MQColumnsResponse() while constructing the AMQP columns list response: '%s'", com.Conf.Live.Nodename, err)
				}
				continue

			case "delete":
				// Delete the database file on the node
				err = removeLiveDB(req.DBOwner, req.DBName)
				if err != nil {
					err = com.MQDeleteResponse(msg, ch, com.Conf.Live.Nodename, err.Error())
					continue
				}

				// Return a success message (empty string in this case) to the caller
				err = com.MQDeleteResponse(msg, ch, com.Conf.Live.Nodename, "")
				if err != nil {
					log.Printf("Error: occurred on '%s' in MQDeleteResponse() while constructing the AMQP delete database response: '%s'", com.Conf.Live.Nodename, err)
				}
				continue

			case "exec":
				// Execute a query on the database file
				err = com.SQLiteExecQueryLive(com.Conf.Live.StorageDir, req.DBOwner, req.DBName, req.RequestingUser, req.Query)
				if err != nil {
					err = com.MQExecResponse(msg, ch, com.Conf.Live.Nodename, err.Error())
					continue
				}

				// Return a success message (empty string in this case) to the caller
				err = com.MQExecResponse(msg, ch, com.Conf.Live.Nodename, "")
				if err != nil {
					log.Printf("Error: occurred on '%s' in MQExecResponse() while constructing the AMQP execute query response: '%s'", com.Conf.Live.Nodename, err)
				}
				continue

			case "indexes":
				var indexes []com.APIJSONIndex
				indexes, err = com.SQLiteGetIndexesLive(com.Conf.Live.StorageDir, req.DBOwner, req.DBName)
				if err != nil {
					err = com.MQIndexesResponse(msg, ch, com.Conf.Live.Nodename, nil, err.Error())
					if err != nil {
						log.Printf("Error: occurred on '%s' in MQIndexesResponse() while constructing an AMQP error message response: '%s'", com.Conf.Live.Nodename, err)
					}
					continue
				}

				// Return the indexes list to the caller
				err = com.MQIndexesResponse(msg, ch, com.Conf.Live.Nodename, indexes, "")
				if err != nil {
					log.Printf("Error: occurred on '%s' in MQIndexesResponse() while constructing the AMQP indexes list response: '%s'", com.Conf.Live.Nodename, err)
				}
				continue

			case "query":
				var rows com.SQLiteRecordSet
				rows, err = com.SQLiteRunQueryLive(com.Conf.Live.StorageDir, req.DBOwner, req.DBName, req.RequestingUser, req.Query)
				if err != nil {
					err = com.MQQueryResponse(msg, ch, com.Conf.Live.Nodename, com.SQLiteRecordSet{}, err.Error())
					if err != nil {
						log.Printf("Error: occurred on '%s' in MQQueryResponse() while constructing an AMQP error message response: '%s'", com.Conf.Live.Nodename, err)
					}
					continue
				}

				// Return the query response to the caller
				err = com.MQQueryResponse(msg, ch, com.Conf.Live.Nodename, rows, "")
				if err != nil {
					log.Printf("Error: occurred on '%s' in MQQueryResponse() while constructing the AMQP query response: '%s'", com.Conf.Live.Nodename, err)
				}
				continue

			case "tables":
				var tables []string
				tables, err = com.SQLiteGetTablesLive(com.Conf.Live.StorageDir, req.DBOwner, req.DBName)
				if err != nil {
					err = com.MQTablesResponse(msg, ch, com.Conf.Live.Nodename, nil, err.Error())
					if err != nil {
						log.Printf("Error: occurred on '%s' in MQTablesResponse() while constructing an AMQP error message response: '%s'", com.Conf.Live.Nodename, err)
					}
					continue
				}

				// Return the tables list to the caller
				err = com.MQTablesResponse(msg, ch, com.Conf.Live.Nodename, tables, "")
				if err != nil {
					log.Printf("Error: occurred on '%s' in MQTablesResponse() while constructing the AMQP tables list response: '%s'", com.Conf.Live.Nodename, err)
				}
				continue

			case "views":
				var views []string
				views, err = com.SQLiteGetViewsLive(com.Conf.Live.StorageDir, req.DBOwner, req.DBName)
				if err != nil {
					err = com.MQViewsResponse(msg, ch, com.Conf.Live.Nodename, nil, err.Error())
					if err != nil {
						log.Printf("Error: occurred on '%s' in MQViewsResponse() while constructing an AMQP error message response: '%s'", com.Conf.Live.Nodename, err)
					}
					continue
				}

				// Return the views list to the caller
				err = com.MQViewsResponse(msg, ch, com.Conf.Live.Nodename, views, "")
				if err != nil {
					log.Printf("Error: occurred on '%s' in MQViewsResponse() while constructing the AMQP views list response: '%s'", com.Conf.Live.Nodename, err)
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

	if com.AmqpDebug {
		log.Printf("Live node '%s': Database file '%s/%s' removed from filesystem path: '%s'",
			com.Conf.Live.Nodename, dbOwner, dbName, dbPath)
	}
	return
}

// setupLiveDB sets up a new instance of a given live database on the local node
func setupLiveDB(dbOwner, dbName string) (err error) {
	// Retrieve the uploaded database file from Minio, and save it to local disk
	_, err = com.LiveRetrieveDatabaseMinio(com.Conf.Live.StorageDir, dbOwner, dbName)
	return
}
