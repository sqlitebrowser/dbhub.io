package common

import (
	"time"
)

// AccessType is whether a database is private, or public, or both
type AccessType int

const (
	DB_BOTH AccessType = iota
	DB_PRIVATE
	DB_PUBLIC
)

type ActivityRange string

const (
	TODAY      ActivityRange = "today"
	THIS_WEEK                = "week"
	THIS_MONTH               = "month"
	ALL_TIME                 = "all"
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

// DefaultNumDisplayRows is the number of rows to display by default on the database page
const DefaultNumDisplayRows = 25

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

// APIJSONColumn is a copy of the Column type from github.com/gwenn/gosqlite, but including JSON field name info
type APIJSONColumn struct {
	Cid       int    `json:"column_id"`
	Name      string `json:"name"`
	DataType  string `json:"data_type"`
	NotNull   bool   `json:"not_null"`
	DfltValue string `json:"default_value"`
	Pk        int    `json:"primary_key"`
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

// APIKey is an internal structure used for passing around user API keys
type APIKey struct {
	Uuid        string     `json:"uuid"`
	Key         string     `json:"key"`
	DateCreated time.Time  `json:"date_created"`
	ExpiryDate  *time.Time `json:"expiry_date"`
	Comment     string     `json:"comment"`
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

type CommitEntry struct {
	AuthorEmail    string    `json:"author_email"`
	AuthorName     string    `json:"author_name"`
	CommitterEmail string    `json:"committer_email"`
	CommitterName  string    `json:"committer_name"`
	ID             string    `json:"id"`
	Message        string    `json:"message"`
	OtherParents   []string  `json:"other_parents"`
	Parent         string    `json:"parent"`
	Timestamp      time.Time `json:"timestamp"`
	Tree           DBTree    `json:"tree"`
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

type DBEntry struct {
	DateEntry        time.Time
	DBName           string
	Owner            string
	OwnerDisplayName string `json:"display_name"`
}

type DBTreeEntryType string

const (
	TREE     DBTreeEntryType = "tree"
	DATABASE                 = "db"
	LICENCE                  = "licence"
)

type DBTree struct {
	ID      string        `json:"id"`
	Entries []DBTreeEntry `json:"entries"`
}
type DBTreeEntry struct {
	EntryType    DBTreeEntryType `json:"entry_type"`
	LastModified time.Time       `json:"last_modified"`
	LicenceSHA   string          `json:"licence"`
	Name         string          `json:"name"`
	Sha256       string          `json:"sha256"`
	Size         int64           `json:"size"`
}

type DBInfo struct {
	Branch        string
	Branches      int
	BranchList    []string
	Commits       int
	CommitID      string
	Contributors  int
	Database      string
	DateCreated   time.Time
	DBEntry       DBTreeEntry
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

type DiscussionCommentType string

const (
	TEXT   DiscussionCommentType = "txt"
	CLOSE                        = "cls"
	REOPEN                       = "rop"
)

type DiscussionCommentEntry struct {
	AvatarURL    string                `json:"avatar_url"`
	Body         string                `json:"body"`
	BodyRendered string                `json:"body_rendered"`
	Commenter    string                `json:"commenter"`
	DateCreated  time.Time             `json:"creation_date"`
	EntryType    DiscussionCommentType `json:"entry_type"`
	ID           int                   `json:"com_id"`
}

type DiscussionType int

const (
	DISCUSSION    DiscussionType = 0 // These are not iota, as it would be seriously bad for these numbers to change
	MERGE_REQUEST                = 1
)

type DiscussionEntry struct {
	AvatarURL    string            `json:"avatar_url"`
	Body         string            `json:"body"`
	BodyRendered string            `json:"body_rendered"`
	CommentCount int               `json:"comment_count"`
	Creator      string            `json:"creator"`
	DateCreated  time.Time         `json:"creation_date"`
	ID           int               `json:"disc_id"`
	LastModified time.Time         `json:"last_modified"`
	MRDetails    MergeRequestEntry `json:"mr_details"`
	Open         bool              `json:"open"`
	Title        string            `json:"title"`
	Type         DiscussionType    `json:"discussion_type"`
}

type EventDetails struct {
	DBName    string    `json:"database_name"`
	DiscID    int       `json:"discussion_id"`
	ID        string    `json:"event_id"`
	Message   string    `json:"message"`
	Owner     string    `json:"database_owner"`
	Timestamp time.Time `json:"event_timestamp"`
	Title     string    `json:"title"`
	Type      EventType `json:"event_type"`
	URL       string    `json:"event_url"`
	UserName  string    `json:"username"`
}

type EventType int

const (
	EVENT_NEW_DISCUSSION    EventType = 0 // These are not iota, as it would be seriously bad for these numbers to change
	EVENT_NEW_MERGE_REQUEST           = 1
	EVENT_NEW_COMMENT                 = 2
	EVENT_NEW_RELEASE                 = 3
)

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

type LicenceEntry struct {
	FileFormat string `json:"file_format"`
	FullName   string `json:"full_name"`
	Order      int    `json:"order"`
	Sha256     string `json:"sha256"`
	URL        string `json:"url"`
}

type MergeRequestState int

const (
	OPEN                 MergeRequestState = 0 // These are not iota, as it would be seriously bad for these numbers to change
	CLOSED_WITH_MERGE                      = 1
	CLOSED_WITHOUT_MERGE                   = 2
)

type MergeRequestEntry struct {
	Commits      []CommitEntry     `json:"commits"`
	DestBranch   string            `json:"destination_branch"`
	SourceBranch string            `json:"source_branch"`
	SourceDBID   int64             `json:"source_database_id"`
	SourceDBName string            `json:"source_database_name"`
	SourceOwner  string            `json:"source_owner"`
	State        MergeRequestState `json:"state"`
}

type ReleaseEntry struct {
	Commit        string    `json:"commit"`
	Date          time.Time `json:"date"`
	Description   string    `json:"description"`
	ReleaserEmail string    `json:"email"`
	ReleaserName  string    `json:"name"`
	Size          int64     `json:"size"`
}

type ShareDatabasePermissions string

const (
	MayRead         ShareDatabasePermissions = "r"
	MayReadAndWrite ShareDatabasePermissions = "rw"
)

// ShareDatabasePermissionsOthers contains a list of user permissions for a given database
type ShareDatabasePermissionsOthers struct {
	DBName string                              `json:"database_name"`
	IsLive bool                                `json:"is_live"`
	Perms  map[string]ShareDatabasePermissions `json:"user_permissions"`
}

// ShareDatabasePermissionsUser contains a list of shared database permissions for a given user
type ShareDatabasePermissionsUser struct {
	OwnerName  string                   `json:"owner_name"`
	DBName     string                   `json:"database_name"`
	IsLive     bool                     `json:"is_live"`
	Permission ShareDatabasePermissions `json:"permission"`
}

type SqlHistoryItemStates string

const (
	Executed SqlHistoryItemStates = "executed"
	Queried  SqlHistoryItemStates = "queried"
	Error    SqlHistoryItemStates = "error"
)

type SqlHistoryItem struct {
	Statement string               `json:"input"`
	Result    interface{}          `json:"output"`
	State     SqlHistoryItemStates `json:"state"`
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

type StatusUpdateEntry struct {
	DiscID int    `json:"discussion_id"`
	Title  string `json:"title"`
	URL    string `json:"event_url"`
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

type UserDetails struct {
	AvatarURL   string
	ClientCert  []byte
	DateJoined  time.Time
	DisplayName string
	Email       string
	MinioBucket string
	Password    string
	PHash       []byte
	PVerify     string
	Username    string
}

type UserInfo struct {
	FullName     string `json:"full_name"`
	LastModified time.Time
	Username     string
}

type VisParamsV2 struct {
	ChartType   string `json:"chart_type"`
	ShowXLabel  bool   `json:"show_x_label"`
	ShowYLabel  bool   `json:"show_y_label"`
	SQL         string `json:"sql"`
	XAXisColumn string `json:"x_axis_label"`
	YAXisColumn string `json:"y_axis_label"`
}
