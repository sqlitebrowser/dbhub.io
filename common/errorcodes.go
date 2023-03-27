package common

// A set of error codes returned by the AMQP back end.  We use these so we can
// change the user facing text as desired without having to worry about
// potentially breaking the AMQP communication interface

type AMQPErrorCode int

const (
	AMQPNoError AMQPErrorCode = iota
	AMQPRequestedTableNotPresent
)

func AMQPErrorString(errCode AMQPErrorCode) string {
	switch errCode {
	case AMQPNoError:
		return "no error"
	case AMQPRequestedTableNotPresent:
		return "Provided table or view name doesn't exist in this database"
	default:
		return "unknown error"
	}
}
