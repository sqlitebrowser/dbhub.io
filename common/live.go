package common

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	AmqpDebug = true
)

// LiveDBColumnsResponse holds the fields used for receiving column list responses from our AMQP backend
type LiveDBColumnsResponse struct {
	Node    string          `json:"node"`
	Columns []sqlite.Column `json:"columns"`
	Error   string          `json:"error"`
}

// LiveDBErrorResponse holds just the node name and any error message used in responses by our AMQP backend
// It's useful for error message, and other responses where no other fields are needed
type LiveDBErrorResponse struct {
	Node  string `json:"node"`
	Error string `json:"error"`
}

// LiveDBExecuteResponse returns the number of rows changed by an Execute() call
type LiveDBExecuteResponse struct {
	Node        string `json:"node"`
	RowsChanged int    `json:"rows_changed"`
	Error       string `json:"error"`
}

// LiveDBIndexesResponse holds the fields used for receiving index list responses from our AMQP backend
type LiveDBIndexesResponse struct {
	Node    string         `json:"node"`
	Indexes []APIJSONIndex `json:"indexes"`
	Error   string         `json:"error"`
}

// LiveDBQueryResponse holds the fields used for receiving query responses from our AMQP backend
type LiveDBQueryResponse struct {
	Node    string          `json:"node"`
	Results SQLiteRecordSet `json:"results"`
	Error   string          `json:"error"`
}

// LiveDBRequest holds the fields used for sending requests to our AMQP backend
type LiveDBRequest struct {
	Operation      string `json:"operation"`
	DBOwner        string `json:"dbowner"`
	DBName         string `json:"dbname"`
	Query          string `json:"query"`
	RequestingUser string `json:"requesting_user"`
}

// LiveDBResponse holds the fields used for receiving (non-query) responses from our AMQP backend
type LiveDBResponse struct {
	Node   string `json:"node"`
	Result string `json:"result"`
	Error  string `json:"error"`
}

// LiveDBs is used for general purpose holding of details about live databases
type LiveDBs struct {
	DBOwner     string    `json:"owner_name"`
	DBName      string    `json:"database_name"`
	DateCreated time.Time `json:"date_created"`
}

// LiveDBTablesResponse holds the fields used for receiving table list responses from our AMQP backend
type LiveDBTablesResponse struct {
	Node   string   `json:"node"`
	Tables []string `json:"tables"`
	Error  string   `json:"error"`
}

// LiveDBViewsResponse holds the fields used for receiving view list responses from our AMQP backend
type LiveDBViewsResponse struct {
	Node  string   `json:"node"`
	Views []string `json:"views"`
	Error string   `json:"error"`
}

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
		// TODO: Getting mTLS working was pretty easy with Lets Encrypt certs.  Do we still need the server-only TLS
		//       fallback below?
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
			// Fallback to just verifying the server certs for TLS
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

// LiveCreateDB requests the AMQP backend create a new live SQLite database
func LiveCreateDB(channel *amqp.Channel, dbOwner, dbName string) (err error) {
	// Send the database setup request to our AMQP backend
	var rawResponse []byte
	rawResponse, err = MQSendRequest(channel, "create_queue", "createdb", "", dbOwner, dbName, "")
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
		err = errors.New("A node responded, but didn't identify itself. :(")
		return
	}
	if resp.Result != "success" {
		err = errors.New(fmt.Sprintf("LIVE database (%s/%s) creation apparently didn't fail, but the response didn't include a success message",
			dbOwner, dbName))
		return
	}

	// Update PG, so it has a record of this database existing and knows the node/queue name for querying it
	err = LiveAddDatabasePG(dbOwner, dbName, resp.Node)
	return
}

// LiveQueryDB sends a SQLite query to a live database on its hosting node
func LiveQueryDB(channel *amqp.Channel, nodeName, requestingUser, dbOwner, dbName, query string) (rows SQLiteRecordSet, err error) {
	// Send the query request to our AMQP backend
	var rawResponse []byte
	rawResponse, err = MQSendRequest(channel, nodeName, "query", requestingUser, dbOwner, dbName, query)
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

// MQColumnsResponse sends a columns list response
func MQColumnsResponse(msg amqp.Delivery, channel *amqp.Channel, nodeName string, columns []sqlite.Column, errMsg string) (err error) {
	// Construct the response
	resp := LiveDBColumnsResponse{
		Node:    nodeName,
		Columns: columns,
		Error:   errMsg,
	}
	var z []byte
	z, err = json.Marshal(resp)
	if err != nil {
		log.Println(err)
		return
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
	msg.Ack(false)
	if AmqpDebug {
		log.Printf("[COLUMNS] Live node '%s' responded with ACK to message with correlationID: '%s', msg.ReplyTo: '%s'", nodeName, msg.CorrelationId, msg.ReplyTo)
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
	if AmqpDebug {
		log.Printf("[CREATE] Live node '%s' responded with ACK to message with correlationID: '%s', msg.ReplyTo: '%s'", nodeName, msg.CorrelationId, msg.ReplyTo)
	}
	return
}

// MQErrorResponse sends an error message in response to an AMQP request
// It is probably only useful for returning errors that occur before we've decoded the incoming AMQP
// request to know what type it is
func MQErrorResponse(msg amqp.Delivery, channel *amqp.Channel, nodeName string, errMsg string) (err error) {
	// Construct the response
	resp := LiveDBErrorResponse{
		Node:  nodeName,
		Error: errMsg,
	}
	var z []byte
	z, err = json.Marshal(resp)
	if err != nil {
		log.Println(err)
		return
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
	msg.Ack(false)
	if AmqpDebug {
		log.Printf("[NOT-YET-DETERMINED] Live node '%s' responded with ACK to message with correlationID: '%s', msg.ReplyTo: '%s'", nodeName, msg.CorrelationId, msg.ReplyTo)
	}
	return
}

// MQDeleteResponse sends an error message in response to an AMQP database deletion request
func MQDeleteResponse(msg amqp.Delivery, channel *amqp.Channel, nodeName string, errMsg string) (err error) {
	// Construct the response
	resp := LiveDBErrorResponse{ // Yep, we're reusing this super basic error response instead of creating a new one
		Node:  nodeName,
		Error: errMsg,
	}
	var z []byte
	z, err = json.Marshal(resp)
	if err != nil {
		log.Println(err)
		return
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
	msg.Ack(false)
	if AmqpDebug {
		log.Printf("[DELETE] Live node '%s' responded with ACK to message with correlationID: '%s', msg.ReplyTo: '%s'", nodeName, msg.CorrelationId, msg.ReplyTo)
	}
	return
}

// MQExecuteResponse sends a message in response to an AMQP database execute request
func MQExecuteResponse(msg amqp.Delivery, channel *amqp.Channel, nodeName string, rowsChanged int, errMsg string) (err error) {
	// Construct the response
	resp := LiveDBExecuteResponse{
		Node:        nodeName,
		RowsChanged: rowsChanged,
		Error:       errMsg,
	}
	var z []byte
	z, err = json.Marshal(resp)
	if err != nil {
		log.Println(err)
		return
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
	msg.Ack(false)
	if AmqpDebug {
		log.Printf("[EXECUTE] Live node '%s' responded with ACK to message with correlationID: '%s', msg.ReplyTo: '%s'", nodeName, msg.CorrelationId, msg.ReplyTo)
	}
	return
}

// MQIndexesResponse sends an indexes list response
func MQIndexesResponse(msg amqp.Delivery, channel *amqp.Channel, nodeName string, indexes []APIJSONIndex, errMsg string) (err error) {
	// Construct the response
	resp := LiveDBIndexesResponse{
		Node:    nodeName,
		Indexes: indexes,
		Error:   errMsg,
	}
	var z []byte
	z, err = json.Marshal(resp)
	if err != nil {
		log.Println(err)
		return
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
	msg.Ack(false)
	if AmqpDebug {
		log.Printf("[INDEXES] Live node '%s' responded with ACK to message with correlationID: '%s', msg.ReplyTo: '%s'", nodeName, msg.CorrelationId, msg.ReplyTo)
	}
	return
}

// MQQueryResponse sends a successful query response back
func MQQueryResponse(msg amqp.Delivery, channel *amqp.Channel, nodeName string, results SQLiteRecordSet, errMsg string) (err error) {
	// Construct the response
	resp := LiveDBQueryResponse{
		Node:    nodeName,
		Results: results,
		Error:   errMsg,
	}
	var z []byte
	z, err = json.Marshal(resp)
	if err != nil {
		log.Println(err)
		return
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
	msg.Ack(false)
	if AmqpDebug {
		log.Printf("[QUERY] Live node '%s' responded with ACK to message with correlationID: '%s', msg.ReplyTo: '%s'", nodeName, msg.CorrelationId, msg.ReplyTo)
	}
	return
}

func MQSendRequest(channel *amqp.Channel, queue, operation, requestingUser, dbOwner, dbName, query string) (result []byte, err error) {
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
		Query:          query,
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

// MQTablesResponse sends a tables list response to an AMQP caller
func MQTablesResponse(msg amqp.Delivery, channel *amqp.Channel, nodeName string, tables []string, errMsg string) (err error) {
	// Construct the response
	resp := LiveDBTablesResponse{
		Node:   nodeName,
		Tables: tables,
		Error:  errMsg,
	}
	var z []byte
	z, err = json.Marshal(resp)
	if err != nil {
		log.Println(err)
		return
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
	msg.Ack(false)
	if AmqpDebug {
		log.Printf("[TABLES] Live node '%s' responded with ACK to message with correlationID: '%s', msg.ReplyTo: '%s'", nodeName, msg.CorrelationId, msg.ReplyTo)
	}
	return
}

// MQViewsResponse sends a views list response to an AMQP caller
func MQViewsResponse(msg amqp.Delivery, channel *amqp.Channel, nodeName string, views []string, errMsg string) (err error) {
	// Construct the response
	resp := LiveDBViewsResponse{
		Node:  nodeName,
		Views: views,
		Error: errMsg,
	}
	var z []byte
	z, err = json.Marshal(resp)
	if err != nil {
		log.Println(err)
		return
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
	msg.Ack(false)
	if AmqpDebug {
		log.Printf("[VIEWS] Live node '%s' responded with ACK to message with correlationID: '%s', msg.ReplyTo: '%s'", nodeName, msg.CorrelationId, msg.ReplyTo)
	}
	return
}
