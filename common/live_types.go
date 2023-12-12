package common

import (
	"sync"

	sqlite "github.com/gwenn/gosqlite"
)

// JobRequest holds the fields used for sending requests to our job request backend
type JobRequest struct {
	Operation      string      `json:"operation"`
	DBOwner        string      `json:"dbowner"`
	DBName         string      `json:"dbname"`
	Data           interface{} `json:"data,omitempty"`
	RequestingUser string      `json:"requesting_user"`
}

// JobRequestRows holds the data used when making a rows request to our job queue backend
type JobRequestRows struct {
	DbTable   string `json:"db_table"`
	SortCol   string `json:"sort_col"`
	SortDir   string `json:"sort_dir"`
	CommitID  string `json:"commit_id"`
	RowOffset int    `json:"row_offset"`
	MaxRows   int    `json:"max_rows"`
}

// JobResponseDBColumns holds the fields used for receiving column list responses from our job queue backend
type JobResponseDBColumns struct {
	Columns   []sqlite.Column   `json:"columns"`
	Err       string            `json:"error"`
	ErrCode   JobQueueErrorCode `json:"error_code"`
	PkColumns []string          `json:"pkColumns"`
}

// JobResponseDBCreate holds the fields used for receiving database creation responses from our job queue backend
type JobResponseDBCreate struct {
	Err      string `json:"error"`
	NodeName string `json:"node_name"`
}

// JobResponseDBError holds the structure used when our job queue backend only needs to response with an error field (empty or not)
type JobResponseDBError struct {
	Err string `json:"error"`
}

// JobResponseDBExecute holds the fields used for receiving the database execute response from our job queue backend
type JobResponseDBExecute struct {
	Err         string `json:"error"`
	RowsChanged int    `json:"rows_changed"`
}

// JobResponseDBIndexes holds the fields used for receiving the database index list from our job queue backend
type JobResponseDBIndexes struct {
	Err     string         `json:"error"`
	Indexes []APIJSONIndex `json:"indexes"`
}

// JobResponseDBQuery holds the fields used for receiving database query results from our job queue backend
type JobResponseDBQuery struct {
	Err     string          `json:"error"`
	Results SQLiteRecordSet `json:"results"`
}

// JobResponseDBRows holds the fields used for receiving table row data from our job queue backend
type JobResponseDBRows struct {
	DatabaseSize int64           `json:"database_size"`
	DefaultTable string          `json:"default_table"`
	Err          string          `json:"error"`
	RowData      SQLiteRecordSet `json:"row_data"`
	Tables       []string        `json:"tables"`
}

// JobResponseDBSize holds the fields used for receiving database size responses from our job queue backend
type JobResponseDBSize struct {
	Err  string `json:"error"`
	Size int64  `json:"size"`
}

// JobResponseDBTables holds the fields used for receiving the database table list from our job queue backend
type JobResponseDBTables struct {
	Err    string   `json:"error"`
	Tables []string `json:"tables"`
}

// JobResponseDBViews holds the fields used for receiving the database views list from our job queue backend
type JobResponseDBViews struct {
	Err   string   `json:"error"`
	Views []string `json:"views"`
}

// ResponseInfo holds job queue responses.  Most of the useful info is json encoded in the payload field
type ResponseInfo struct {
	jobID      int
	responseID int
	payload    string
}

// ResponseReceivers is a simple structure used for matching up job queue responses to the caller who submitted the job
type ResponseReceivers struct {
	sync.RWMutex
	receivers map[int]*chan ResponseInfo
}

// NewResponseQueue is the constructor function for creating a new ResponseReceivers
func NewResponseQueue() *ResponseReceivers {
	z := ResponseReceivers{
		RWMutex:   sync.RWMutex{},
		receivers: nil,
	}
	z.receivers = make(map[int]*chan ResponseInfo)
	return &z
}

// AddReceiver adds a new response receiver
func (r *ResponseReceivers) AddReceiver(jobID int, newReceiver *chan ResponseInfo) {
	r.Lock()
	r.receivers[jobID] = newReceiver
	r.Unlock()
}

// RemoveReceiver removes a response receiver (generally after it has received the response it was waiting for)
func (r *ResponseReceivers) RemoveReceiver(jobID int) {
	r.Lock()
	delete(r.receivers, jobID)
	r.Unlock()
}

// *** Legacy (hopefully) AMQP related types

// LiveDBColumnsResponse holds the fields used for receiving column list responses from our AMQP backend
type LiveDBColumnsResponse struct {
	Node      string            `json:"node"`
	Columns   []sqlite.Column   `json:"columns"`
	PkColumns []string          `json:"pkColuns"`
	Error     string            `json:"error"`
	ErrCode   JobQueueErrorCode `json:"error_code"`
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
	Operation      string      `json:"operation"`
	DBOwner        string      `json:"dbowner"`
	DBName         string      `json:"dbname"`
	Data           interface{} `json:"data,omitempty"`
	RequestingUser string      `json:"requesting_user"`
}

// LiveDBResponse holds the fields used for receiving (non-query) responses from our AMQP backend
type LiveDBResponse struct {
	Node   string `json:"node"`
	Result string `json:"result"`
	Error  string `json:"error"`
}

// LiveDBRowsRequest holds the data used when making an AMQP rows request
type LiveDBRowsRequest struct {
	DbTable   string `json:"db_table"`
	SortCol   string `json:"sort_col"`
	SortDir   string `json:"sort_dir"`
	CommitID  string `json:"commit_id"`
	RowOffset int    `json:"row_offset"`
	MaxRows   int    `json:"max_rows"`
}

// LiveDBRowsResponse holds the fields used for receiving database page row responses from our AMQP backend
type LiveDBRowsResponse struct {
	Node         string          `json:"node"`
	DatabaseSize int64           `json:"database_size"`
	DefaultTable string          `json:"default_table"`
	Error        string          `json:"error"`
	RowData      SQLiteRecordSet `json:"row_data"`
	Tables       []string        `json:"tables"`
}

// LiveDBSizeResponse holds the fields used for receiving database size responses from our AMQP backend
type LiveDBSizeResponse struct {
	Node  string `json:"node"`
	Size  int64  `json:"size"`
	Error string `json:"error"`
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
