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
//        -> Minio folder: "34f425"
//        -> Minio filename: "5a737156147fbd0a44323a895d18ade79d4db521564d1b0dbb8764cbbc"
const MinioFolderChars = 6

// QuerySource is used internally to help choose the output format from a SQL query
type QuerySource int

const (
	WebUI QuerySource = iota
	DB4S
	Visualisation
	API
	Internal
)

// SetAccessType is used for setting the public flag of a database
type SetAccessType int

const (
	SetToPublic SetAccessType = iota
	SetToPrivate
	KeepCurrentAccessType
)

// ************************
// Configuration file types

// TomlConfig is a top level structure containing the server configuration info
type TomlConfig struct {
	Api         ApiInfo
	Auth0       Auth0Info
	DB4S        DB4SInfo
	Environment EnvInfo
	DiskCache   DiskCacheInfo
	Event       EventProcessingInfo
	Licence     LicenceInfo
	Memcache    MemcacheInfo
	Minio       MinioInfo
	Pg          PGInfo
	Sign        SigningInfo
	Web         WebInfo
}

// ApiInfo contains configuration info for the API daemon
type ApiInfo struct {
	BaseDir        string `toml:"base_dir"`
	BindAddress    string `toml:"bind_address"`
	Certificate    string `toml:"certificate"`
	CertificateKey string `toml:"certificate_key"`
	RequestLog     string `toml:"request_log"`
	ServerName     string `toml:"server_name"`
}

// Auth0Info contains the Auth0 connection info used authenticating webUI users
type Auth0Info struct {
	ClientID     string
	ClientSecret string
	Domain       string
}

// DB4SInfo contains configuration info for the DB4S end point daemon
type DB4SInfo struct {
	CAChain        string `toml:"ca_chain"`
	Certificate    string
	CertificateKey string `toml:"certificate_key"`
	Debug          bool
	Port           int
	Server         string
}

// DiskCacheInfo contains the path to the root of the local disk cache
type DiskCacheInfo struct {
	Directory string
}

// EnvInfo holds information about the purpose of the running server.  eg "is this a production, docker,
// or development" instance?
type EnvInfo struct {
	Environment  string
	UserOverride string `toml:"user_override"`
}

// EventProcessingInfo hold configuration for the event processing loop
type EventProcessingInfo struct {
	Delay                     time.Duration `toml:"delay"`
	EmailQueueDir             string        `toml:"email_queue_dir"`
	EmailQueueProcessingDelay time.Duration `toml:"email_queue_processing_delay"`
}

// LicenceDir holds the path to the licence files
type LicenceInfo struct {
	LicenceDir string `toml:"licence_dir"`
}

// MemcacheInfo contains the Memcached configuration parameters
type MemcacheInfo struct {
	DefaultCacheTime    int           `toml:"default_cache_time"`
	Server              string        `toml:"server"`
	ViewCountFlushDelay time.Duration `toml:"view_count_flush_delay"`
}

// MinioInfo contains the Minio connection parameters
type MinioInfo struct {
	AccessKey string `toml:"access_key"`
	HTTPS     bool
	Secret    string
	Server    string
}

// PGInfo contains the PostgreSQL connection parameters
type PGInfo struct {
	Database       string
	NumConnections int `toml:"num_connections"`
	Port           int
	Password       string
	Server         string
	SSL            bool
	Username       string
}

// SigningInfo contains the info used for signing DB4S client certificates
type SigningInfo struct {
	CertDaysValid    int    `toml:"cert_days_valid"`
	Enabled          bool   `toml:"enabled"`
	IntermediateCert string `toml:"intermediate_cert"`
	IntermediateKey  string `toml:"intermediate_key"`
}

// WebInfo contains configuration info for the webUI daemon
type WebInfo struct {
	BaseDir              string `toml:"base_dir"`
	BindAddress          string `toml:"bind_address"`
	Certificate          string `toml:"certificate"`
	CertificateKey       string `toml:"certificate_key"`
	RequestLog           string `toml:"request_log"`
	ServerName           string `toml:"server_name"`
	SessionStorePassword string `toml:"session_store_password"`
}

// End of configuration file types
// *******************************

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
	Database    string                 `json:"database_name"`
	DateCreated time.Time              `json:"date_created"`
	Key         string                 `json:"key"`
	Permissions map[APIPermission]bool `json:"permissions"`
}

type APIPermission string

const (
	APIPermBranches  = "branches"
	APIPermColumns   = "columns"
	APIPermCommits   = "commits"
	APIPermDatabases = "databases"
	APIPermDelete    = "delete"
	APIPermDiff      = "diff"
	APIPermDownload  = "download"
	APIPermIndexes   = "indexes"
	APIPermMetadata  = "metadata"
	APIPermQuery     = "query"
	APIPermReleases  = "releases"
	APIPermTables    = "tables"
	APIPermTags      = "tags"
	APIPermUpload    = "upload"
	APIPermViews     = "views"
	APIPermWebpage   = "webpage"
)

type Auth0Set struct {
	CallbackURL string
	ClientID    string
	Domain      string
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

type DataValue struct {
	Name  string
	Type  ValType
	Value interface{}
}
type DataRow []DataValue

type DBEntry struct {
	Folder           string
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
	Folder        string
	Forks         int
	FullDesc      string
	LastModified  time.Time
	Licence       string
	LicenceURL    string
	MRs           int
	OneLineDesc   string
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
	Folder    string    `json:"database_folder"`
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
	Folder     string     `json:"database_folder"`
	ForkedFrom int        `json:"forked_from"`
	IconList   []ForkType `json:"icon_list"`
	ID         int        `json:"id"`
	Owner      string     `json:"database_owner"`
	Processed  bool       `json:"processed"`
	Public     bool       `json:"public"`
	Deleted    bool       `json:"deleted"`
}

// JsonError holds the details of an error.  It's used by our API for returning error information
type JsonError struct {
	Error string `json:"error"`
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
	SourceFolder string            `json:"source_folder"`
	SourceOwner  string            `json:"source_owner"`
	State        MergeRequestState `json:"state"`
}

type MetaInfo struct {
	AvatarURL        string
	Database         string
	Folder           string
	ForkDatabase     string
	ForkDeleted      bool
	ForkFolder       string
	ForkOwner        string
	LoggedInUser     string
	NumStatusUpdates int
	Owner            string
	PageSection      string
	Protocol         string
	Server           string
	Title            string
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

type SQLiteDBinfo struct {
	Info     DBInfo
	MaxRows  int
	MinioBkt string
	MinioId  string
}

type SQLiteRecordSet struct {
	ColCount  int
	ColNames  []string
	Offset    int
	Records   []DataRow
	RowCount  int
	SortCol   string
	SortDir   string
	Tablename string
	TotalRows int
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

type VisRowV1 struct {
	Name  string
	Value int
}

type WhereClause struct {
	Column string
	Type   string
	Value  string
}
