package common

import (
	"time"
)

// ValType indicates the type of data in a field returned from a SQLite query
type ValType int

const (
	Binary ValType = iota
	Image
	Null
	Text
	Integer
	Float
)

// MaxLicenceSize is the maximum licence size accepted for upload (in MB)
const MaxLicenceSize = 1

// MinioFolderChars is the number of leading characters of a files' sha256 used as the Minio folder name
// eg: When set to 6, then "34f4255a737156147fbd0a44323a895d18ade79d4db521564d1b0dbb8764cbbc"
//
//	-> Minio folder: "34f425"
//	-> Minio filename: "5a737156147fbd0a44323a895d18ade79d4db521564d1b0dbb8764cbbc"
const MinioFolderChars = 6

// QuerySource is used internally to help choose the output format from a SQL query
type QuerySource int

const (
	QuerySourceDB4S QuerySource = iota
	QuerySourceVisualisation
	QuerySourceAPI
	QuerySourceInternal
)

// APIJSONIndexColumn holds the details of one column of a SQLite database index.  It's used by our API for returning
// index information
type APIJSONIndexColumn struct {
	CID  int    `json:"id"`
	Name string `json:"name"`
}

// APIJSONIndex holds the details of an index for a SQLite database.  It's used by our API for returning index information
type APIJSONIndex struct {
	Name    string               `json:"name"`
	Table   string               `json:"table"`
	Columns []APIJSONIndexColumn `json:"columns"`
}

type DatabaseName struct {
	Database string
	Owner    string
}

type DataValue struct {
	Name  string
	Type  ValType
	Value interface{}
}
type DataRow []DataValue

type SQLiteRecordSet struct {
	ColCount          int
	ColNames          []string
	Offset            int
	PrimaryKeyColumns []string `json:"primaryKeyColumns,omitempty"`
	Records           []DataRow
	RowCount          int
	SortCol           string
	SortDir           string
	Tablename         string
	TotalRows         int
}

// UserInfoSlice is used for sorting a UserInfo list by Last Modified date descending
type UserInfoSlice []UserInfo

func (u UserInfoSlice) Len() int {
	return len(u)
}

func (u UserInfoSlice) Less(i, j int) bool {
	return u[i].LastModified.After(u[j].LastModified) // Swap to Before() for an ascending list
}

func (u UserInfoSlice) Swap(i, j int) {
	u[i], u[j] = u[j], u[i]
}

type UserInfo struct {
	FullName     string `json:"full_name"`
	LastModified time.Time
	Username     string
}
