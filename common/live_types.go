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

// JobResponseDBError holds the structure used when our job queue backend only needs to respond with an error field (empty or not)
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
