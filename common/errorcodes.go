package common

// A set of error codes returned by our job queue back end.  We use these so we can
// change the user facing text as desired without having to worry about potentially
// breaking the job queue communication interface

type JobQueueErrorCode int

const (
	JobQueueNoError JobQueueErrorCode = iota
	JobQueueRequestedTableNotPresent
)
