package common

import (
	"time"

	sqlite "github.com/gwenn/gosqlite"
)

// LiveDBColumnsResponse holds the fields used for receiving column list responses from our AMQP backend
type LiveDBColumnsResponse struct {
	Node    string          `json:"node"`
	Columns []sqlite.Column `json:"columns"`
	Error   string          `json:"error"`
	ErrCode AMQPErrorCode   `json:"error_code"`
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
	RowData      SQLiteRecordSet `json:"row_data"`
	Tables       []string        `json:"tables"`
	Error        string          `json:"error"`
}

// LiveDBs is used for general purpose holding of details about live databases
type LiveDBs struct {
	DBOwner     string    `json:"owner_name"`
	DBName      string    `json:"database_name"`
	DateCreated time.Time `json:"date_created"`
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
