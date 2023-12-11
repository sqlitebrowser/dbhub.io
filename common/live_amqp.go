package common

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

var (
	// AmqpChan is the AMQP channel handle we use for communication with our AMQP backend
	AmqpChan *amqp.Channel

	// UseAMQP switches between running in AMQP mode (true) or job queue server mode (false)
	UseAMQP = true
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

	if JobQueueDebug > 0 {
		log.Printf("[%s] Live node '%s' responded with ACK to message with correlationID: '%s', msg.ReplyTo: '%s'", requestType, nodeName, msg.CorrelationId, msg.ReplyTo)
		return
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
	if JobQueueDebug > 0 {
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
