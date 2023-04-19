package common

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"time"

	sqlite "github.com/gwenn/gosqlite"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	contextTimeout = 5 * time.Second
)

var (
	// AmqpChan is the AMQP channel handle we use for communication with our AMQP backend
	AmqpChan *amqp.Channel

	// AmqpDebug controls whether to output - via Log.Print*() functions -  useful messages during processing.  Mostly
	// useful for development / debugging purposes
	AmqpDebug = 1
)

// CloseMQChannel closes an open AMQP channel
func CloseMQChannel(channel *amqp.Channel) (err error) {
	err = channel.Close()
	return
}

// CloseMQConnection closes an open AMQP connection
func CloseMQConnection(connection *amqp.Connection) (err error) {
	err = connection.Close()
	return
}

// ConnectMQ creates a connection to the backend MQ server
func ConnectMQ() (channel *amqp.Channel, err error) {
	var conn *amqp.Connection
	if Conf.Environment.Environment == "production" {
		// If certificate/key files have been provided, then we can use mutual TLS (mTLS)
		if Conf.MQ.CertFile != "" && Conf.MQ.KeyFile != "" {
			var cert tls.Certificate
			cert, err = tls.LoadX509KeyPair(Conf.MQ.CertFile, Conf.MQ.KeyFile)
			if err != nil {
				return
			}
			cfg := &tls.Config{Certificates: []tls.Certificate{cert}}
			conn, err = amqp.DialTLS(fmt.Sprintf("amqps://%s:%s@%s:%d/", Conf.MQ.Username, Conf.MQ.Password, Conf.MQ.Server, Conf.MQ.Port), cfg)
			if err != nil {
				return
			}
			log.Printf("%s connected to AMQP server using mutual TLS (mTLS): %v:%d\n", Conf.Live.Nodename, Conf.MQ.Server, Conf.MQ.Port)
		} else {
			// Fallback to just verifying the server certs for TLS.  This is needed by the DB4S end point, as it
			// uses certs from our own CA, so mTLS won't easily work with it.
			conn, err = amqp.Dial(fmt.Sprintf("amqps://%s:%s@%s:%d/", Conf.MQ.Username, Conf.MQ.Password, Conf.MQ.Server, Conf.MQ.Port))
			if err != nil {
				return
			}
			log.Printf("%s connected to AMQP server with server-only TLS: %v:%d\n", Conf.Live.Nodename, Conf.MQ.Server, Conf.MQ.Port)
		}
	} else {
		// Everywhere else (eg docker container) doesn't *have* to use TLS
		conn, err = amqp.Dial(fmt.Sprintf("amqp://%s:%s@%s:%d/", Conf.MQ.Username, Conf.MQ.Password, Conf.MQ.Server, Conf.MQ.Port))
		if err != nil {
			return
		}
		log.Printf("%s connected to AMQP server without encryption: %v:%d\n", Conf.Live.Nodename, Conf.MQ.Server, Conf.MQ.Port)
	}

	channel, err = conn.Channel()
	return
}

// LiveBackup asks the AMQP backend to store the given database back into Minio
func LiveBackup(liveNode, loggedInUser, dbOwner, dbName string) (err error) {
	var rawResponse []byte
	rawResponse, err = MQRequest(AmqpChan, liveNode, "backup", loggedInUser, dbOwner, dbName, "")
	if err != nil {
		return
	}

	// Decode the response
	var resp LiveDBErrorResponse
	err = json.Unmarshal(rawResponse, &resp)
	if err != nil {
		return
	}

	// If the backup failed, then provide the error message to the user
	if resp.Error != "" {
		err = errors.New(resp.Error)
		return
	}
	if resp.Node == "" {
		log.Println("A node responded to a 'backup' request, but didn't identify itself.")
		return
	}
	return
}

// LiveColumns requests the AMQP backend to return a list of all columns of the given table
func LiveColumns(liveNode, loggedInUser, dbOwner, dbName, table string) (columns []sqlite.Column, pk []string, err error) {
	var rawResponse []byte
	rawResponse, err = MQRequest(AmqpChan, liveNode, "columns", loggedInUser, dbOwner, dbName, table)
	if err != nil {
		return
	}

	// Decode the response
	var resp LiveDBColumnsResponse
	err = json.Unmarshal(rawResponse, &resp)
	if err != nil {
		return
	}
	if resp.Error != "" {
		err = errors.New(resp.Error)
		return
	}
	if resp.Node == "" {
		log.Println("A node responded to a 'columns' request, but didn't identify itself.")
		return
	}
	columns = resp.Columns
	pk = resp.PkColumns
	return
}

// LiveCreateDB requests the AMQP backend create a new live SQLite database
func LiveCreateDB(channel *amqp.Channel, dbOwner, dbName, objectID string, accessType SetAccessType) (err error) {
	// Send the database setup request to our AMQP backend
	var rawResponse []byte
	rawResponse, err = MQRequest(channel, "create_queue", "createdb", "", dbOwner, dbName, objectID)
	if err != nil {
		return
	}

	// Decode the response
	var resp LiveDBResponse
	err = json.Unmarshal(rawResponse, &resp)
	if err != nil {
		log.Println(err)
		return
	}
	if resp.Error != "" {
		err = errors.New(resp.Error)
		return
	}
	if resp.Node == "" {
		log.Println("A node responded to a 'create' request, but didn't identify itself.")
		return
	}
	if resp.Result != "success" {
		err = errors.New(fmt.Sprintf("LIVE database (%s/%s) creation apparently didn't fail, but the response didn't include a success message",
			dbOwner, dbName))
		return
	}

	// Update PG, so it has a record of this database existing and knows the node/queue name for querying it
	err = LiveAddDatabasePG(dbOwner, dbName, objectID, resp.Node, accessType)
	if err != nil {
		return
	}

	// Enable the watch flag for the uploader for this database
	err = ToggleDBWatch(dbOwner, dbOwner, dbName)
	return
}

// LiveDelete asks our AMQP backend to delete a database
func LiveDelete(liveNode, loggedInUser, dbOwner, dbName string) (err error) {
	// Delete the database from our AMQP backend
	var rawResponse []byte
	rawResponse, err = MQRequest(AmqpChan, liveNode, "delete", loggedInUser, dbOwner, dbName, "")
	if err != nil {
		log.Println(err)
		return
	}

	// Decode the response
	var resp LiveDBErrorResponse
	err = json.Unmarshal(rawResponse, &resp)
	if err != nil {
		log.Println(err)
		return
	}
	if resp.Error != "" {
		err = errors.New(resp.Error)
		return
	}
	if resp.Node == "" {
		log.Println("A node responded to a 'delete' request, but didn't identify itself.")
		return
	}
	return
}

// LiveExecute asks our AMQP backend to execute a SQL statement on a database
func LiveExecute(liveNode, loggedInUser, dbOwner, dbName, sql string) (rowsChanged int, err error) {
	var rawResponse []byte
	rawResponse, err = MQRequest(AmqpChan, liveNode, "execute", loggedInUser, dbOwner, dbName, sql)
	if err != nil {
		return
	}

	// Decode the response
	var resp LiveDBExecuteResponse
	err = json.Unmarshal(rawResponse, &resp)
	if err != nil {
		log.Println(err)
		return
	}

	// If the SQL execution failed, then provide the error message to the user
	if resp.Error != "" {
		err = errors.New(resp.Error)
		return
	}
	rowsChanged = resp.RowsChanged
	return
}

// LiveQueryDB sends a SQLite query to a live database on its hosting node
func LiveQueryDB(channel *amqp.Channel, nodeName, requestingUser, dbOwner, dbName, query string) (rows SQLiteRecordSet, err error) {
	// Send the query request to our AMQP backend
	var rawResponse []byte
	rawResponse, err = MQRequest(channel, nodeName, "query", requestingUser, dbOwner, dbName, query)
	if err != nil {
		return
	}

	// Decode the response
	var resp LiveDBQueryResponse
	err = json.Unmarshal(rawResponse, &resp)
	if err != nil {
		log.Println(err)
		return
	}
	if resp.Error != "" {
		err = errors.New(resp.Error)
		return
	}
	rows = resp.Results
	return
}

// LiveRowData asks our AMQP backend to send us the SQLite table data for a given range of rows
func LiveRowData(liveNode, loggedInUser, dbOwner, dbName string, reqData LiveDBRowsRequest) (rowData SQLiteRecordSet, err error) {
	var rawResponse []byte
	rawResponse, err = MQRequest(AmqpChan, liveNode, "rowdata", loggedInUser, dbOwner, dbName, reqData)
	if err != nil {
		log.Println(err)
		return
	}

	// Decode the response
	var resp LiveDBRowsResponse
	err = json.Unmarshal(rawResponse, &resp)
	if err != nil {
		log.Println(err)
		return
	}
	if resp.Error != "" {
		err = errors.New(resp.Error)
		log.Println(err)
		return
	}
	if resp.Node == "" {
		log.Println("A node responded to a 'rowdata' request, but didn't identify itself.")
		return
	}
	rowData = resp.RowData
	return
}

// LiveSize asks our AMQP backend for the file size of a database
func LiveSize(liveNode, loggedInUser, dbOwner, dbName string) (size int64, err error) {
	// Send the size request to our AMQP backend
	var rawResponse []byte
	rawResponse, err = MQRequest(AmqpChan, liveNode, "size", loggedInUser, dbOwner, dbName, "")
	if err != nil {
		return
	}

	// Decode the response
	var resp LiveDBSizeResponse
	err = json.Unmarshal(rawResponse, &resp)
	if err != nil {
		return
	}
	if resp.Error != "" {
		err = errors.New(resp.Error)
		return
	}
	if resp.Node == "" {
		log.Println("A node responded to a 'size' request, but didn't identify itself.")
		return
	}
	size = resp.Size
	return
}

// LiveTables asks our AMQP backend to provide the list of tables (not including views!) in a database
func LiveTables(liveNode, loggedInUser, dbOwner, dbName string) (tables []string, err error) {
	// Send the tables request to our AMQP backend
	var rawResponse []byte
	rawResponse, err = MQRequest(AmqpChan, liveNode, "tables", loggedInUser, dbOwner, dbName, "")
	if err != nil {
		return
	}

	// Decode the response
	var resp LiveDBTablesResponse
	err = json.Unmarshal(rawResponse, &resp)
	if err != nil {
		return
	}
	if resp.Error != "" {
		err = errors.New(resp.Error)
		return
	}
	if resp.Node == "" {
		log.Println("A node responded to a 'tables' request, but didn't identify itself.")
		return
	}
	tables = resp.Tables
	return
}

// LiveTablesAndViews asks our AMQP backend to provide the list of tables and views in a database
func LiveTablesAndViews(liveNode, loggedInUser, dbOwner, dbName string) (list []string, err error) {
	// Send the tables request to our AMQP backend
	list, err = LiveTables(liveNode, loggedInUser, dbOwner, dbName)
	if err != nil {
		return
	}

	// Send the tables request to our AMQP backend
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

// LiveViews asks our AMQP backend to provide the list of views (not including tables!) in a database
func LiveViews(liveNode, loggedInUser, dbOwner, dbName string) (views []string, err error) {
	var rawResponse []byte
	rawResponse, err = MQRequest(AmqpChan, liveNode, "views", loggedInUser, dbOwner, dbName, "")
	if err != nil {
		return
	}

	// Decode the response
	var resp LiveDBViewsResponse
	err = json.Unmarshal(rawResponse, &resp)
	if err != nil {
		return
	}
	if resp.Error != "" {
		err = errors.New(resp.Error)
		return
	}
	if resp.Node == "" {
		log.Println("A node responded to a 'views' request, but didn't identify itself.")
		return
	}
	views = resp.Views
	return
}

// MQResponse sends an AMQP response back to its requester
func MQResponse(requestType string, msg amqp.Delivery, channel *amqp.Channel, nodeName string, responseData interface{}) (err error) {
	var z []byte
	z, err = json.Marshal(responseData)
	if err != nil {
		log.Println(err)
		// It's super unlikely we can safely return here without ack-ing the message.  So as something has gone
		// wrong with json.Marshall() we'd better just attempt passing back info about that error message instead (!)
		z = []byte(fmt.Sprintf(`{"node":"%s","error":"%s"}`, nodeName, err.Error())) // This is a LiveDBErrorResponse structure
	}

	// Send the message
	ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
	defer cancel()
	err = channel.PublishWithContext(ctx, "", msg.ReplyTo, false, false,
		amqp.Publishing{
			ContentType:   "text/json",
			CorrelationId: msg.CorrelationId,
			Body:          z,
		})
	if err != nil {
		log.Println(err)
	}

	// Acknowledge the request, so it doesn't stick around in the queue
	err = msg.Ack(false)
	if err != nil {
		log.Println(err)
	}

	if AmqpDebug > 0 {
		log.Printf("[%s] Live node '%s' responded with ACK to message with correlationID: '%s', msg.ReplyTo: '%s'", requestType, nodeName, msg.CorrelationId, msg.ReplyTo)
	}
	return
}

// MQCreateDBQueue creates a queue on the MQ server for "create database" messages
func MQCreateDBQueue(channel *amqp.Channel) (queue amqp.Queue, err error) {
	queue, err = channel.QueueDeclare("create_queue", true, false, false, false, nil)
	if err != nil {
		return
	}

	// FIXME: Re-read the docs for this, and work out if this is needed
	err = channel.Qos(1, 0, false)
	if err != nil {
		return
	}
	return
}

// MQCreateQueryQueue creates a queue on the MQ server for sending database queries to
func MQCreateQueryQueue(channel *amqp.Channel, nodeName string) (queue amqp.Queue, err error) {
	queue, err = channel.QueueDeclare(nodeName, false, false, false, false, nil)
	if err != nil {
		return
	}

	// FIXME: Re-read the docs for this, and work out if this is needed
	err = channel.Qos(0, 0, false)
	if err != nil {
		return
	}
	return
}

// MQCreateResponse sends a success/failure response back
func MQCreateResponse(msg amqp.Delivery, channel *amqp.Channel, nodeName, result string) (err error) {
	// Construct the response.  It's such a simple string we just create it directly instead of using json.Marshall()
	resp := fmt.Sprintf(`{"node":"%s","dbowner":"","dbname":"","result":"%s","error":""}`, nodeName, result)

	// Send the message
	ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
	defer cancel()
	err = channel.PublishWithContext(ctx, "", msg.ReplyTo, false, false,
		amqp.Publishing{
			ContentType:   "text/json",
			CorrelationId: msg.CorrelationId,
			Body:          []byte(resp),
		})
	if err != nil {
		log.Println(err)
	}
	msg.Ack(false)
	if AmqpDebug > 0 {
		log.Printf("[CREATE] Live node '%s' responded with ACK to message with correlationID: '%s', msg.ReplyTo: '%s'", nodeName, msg.CorrelationId, msg.ReplyTo)
	}
	return
}

// MQRequest is the main function used for sending requests to our AMQP backend
func MQRequest(channel *amqp.Channel, queue, operation, requestingUser, dbOwner, dbName string, data interface{}) (result []byte, err error) {
	// Create a temporary AMQP queue for receiving the response
	var q amqp.Queue
	q, err = channel.QueueDeclare("", false, false, true, false, nil)
	if err != nil {
		return
	}

	// Construct the request
	bar := LiveDBRequest{
		Operation:      operation,
		DBOwner:        dbOwner,
		DBName:         dbName,
		Data:           data,
		RequestingUser: requestingUser,
	}
	var z []byte
	z, err = json.Marshal(bar)
	if err != nil {
		log.Println(err)
		return
	}

	// Send the request via AMQP
	ctx, cancel := context.WithTimeout(context.Background(), contextTimeout)
	defer cancel()
	corrID := RandomString(32)
	err = channel.PublishWithContext(ctx, "", queue, false, false,
		amqp.Publishing{
			ContentType:   "text/json",
			CorrelationId: corrID,
			ReplyTo:       q.Name,
			Body:          z,
		})
	if err != nil {
		log.Println(err)
		return
	}

	// Start processing messages from the AMQP response queue
	msgs, err := channel.Consume(q.Name, "", true, false, false, false, nil)
	if err != nil {
		return
	}

	// Wait for, then extract the response.  Without json unmarshalling it yet
	for d := range msgs {
		if corrID == d.CorrelationId {
			result = d.Body
			break
		}
	}

	// Delete the temporary queue
	_, err = channel.QueueDelete(q.Name, false, false, false)
	if err != nil {
		log.Println(err)
	}
	return
}
