package common

// A set of error codes returned by our job queue back end.  We use these so we can
// change the user facing text as desired without having to worry about potentially
// breaking the job queue communication interface

type JobQueueErrorCode int

const (
	JobQueueNoError JobQueueErrorCode = iota
	JobQueueRequestedTableNotPresent
)

func JobQueueErrorString(errCode JobQueueErrorCode) string {
	switch errCode {
	case JobQueueNoError:
		return "no error"
	case JobQueueRequestedTableNotPresent:
		return "Provided table or view name doesn't exist in this database"
	default:
		return "unknown error"
	}
}
