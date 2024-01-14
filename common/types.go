package common

import (
	"time"

	"github.com/sqlitebrowser/dbhub.io/common/database"
)

// AccessType is whether a database is private, or public, or both
type AccessType int

const (
	DB_BOTH AccessType = iota
	DB_PRIVATE
	DB_PUBLIC
)

type ForkType int

const (
	SPACE ForkType = iota
	ROOT
	STEM
	BRANCH
	END
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

// MaxDatabaseSize is the maximum database size accepted for upload (in MB)
const MaxDatabaseSize = 512

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

// SetAccessType is used for setting the public flag of a database
type SetAccessType int

const (
	SetToPublic SetAccessType = iota
	SetToPrivate
	KeepCurrentAccessType
)

// SetDBType is used for setting what type of database we're working with
type SetDBType int

const (
	DBTypeStandard SetDBType = iota
	DBTypeLive
)

type ActivityRow struct {
	Count  int    `json:"count"`
	DBName string `json:"dbname"`
	Owner  string `json:"owner"`
}

type ActivityStats struct {
	Downloads []ActivityRow
	Forked    []ActivityRow
	Starred   []ActivityRow
	Uploads   []UploadRow
	Viewed    []ActivityRow
}

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

type BranchEntry struct {
	Commit      string `json:"commit"`
	CommitCount int    `json:"commit_count"`
	Description string `json:"description"`
}

type CommitData struct {
	AuthorAvatar   string    `json:"author_avatar"`
	AuthorEmail    string    `json:"author_email"`
	AuthorName     string    `json:"author_name"`
	AuthorUsername string    `json:"author_username"`
	ID             string    `json:"id"`
	Parent         string    `json:"parent"`
	LicenceChange  string    `json:"licence_change"`
	Message        string    `json:"message"`
	Timestamp      time.Time `json:"timestamp"`
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

type DBInfo struct {
	Branch        string
	Branches      int
	BranchList    []string
	Commits       int
	CommitID      string
	Contributors  int
	Database      string
	DateCreated   time.Time
	DBEntry       database.DBTreeEntry
	DefaultBranch string
	DefaultTable  string
	Discussions   int
	Downloads     int
	ForkDatabase  string
	ForkDeleted   bool
	ForkOwner     string
	Forks         int
	FullDesc      string
	IsLive        bool
	LastModified  time.Time
	Licence       string
	LicenceURL    string
	LiveNode      string
	MRs           int
	MyStar        bool
	MyWatch       bool
	OneLineDesc   string
	Owner         string
	Public        bool
	RepoModified  time.Time
	Releases      int
	SHA256        string
	Size          int64
	SourceURL     string
	Stars         int
	Tables        []string
	Tags          int
	Views         int
	Watchers      int
}

type ForkEntry struct {
	DBName     string     `json:"database_name"`
	ForkedFrom int        `json:"forked_from"`
	IconList   []ForkType `json:"icon_list"`
	ID         int        `json:"id"`
	Owner      string     `json:"database_owner"`
	Processed  bool       `json:"processed"`
	Public     bool       `json:"public"`
	Deleted    bool       `json:"deleted"`
}

type ReleaseEntry struct {
	Commit        string    `json:"commit"`
	Date          time.Time `json:"date"`
	Description   string    `json:"description"`
	ReleaserEmail string    `json:"email"`
	ReleaserName  string    `json:"name"`
	Size          int64     `json:"size"`
}

type SQLiteDBinfo struct {
	Info     DBInfo
	MaxRows  int
	MinioBkt string
	MinioId  string
}

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

type TagEntry struct {
	Commit      string    `json:"commit"`
	Date        time.Time `json:"date"`
	Description string    `json:"description"`
	TaggerEmail string    `json:"email"`
	TaggerName  string    `json:"name"`
}

type UploadRow struct {
	DBName     string    `json:"dbname"`
	Owner      string    `json:"owner"`
	UploadDate time.Time `json:"upload_date"`
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
