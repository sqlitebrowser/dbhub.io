package common

import (
	"time"
)

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

type ValType int

const (
	Binary ValType = iota
	Image
	Null
	Text
	Integer
	Float
)

// Number of rows to display by default on the database page
const DefaultNumDisplayRows = 25

// The maximum database size accepted for upload (in MB)
const MaxDatabaseSize = 100

// The number of leading characters of a files' sha256 used as the Minio folder name
// eg: When set to 6, then "34f4255a737156147fbd0a44323a895d18ade79d4db521564d1b0dbb8764cbbc"
//        -> Minio folder: "34f425"
//        -> Minio filename: "5a737156147fbd0a44323a895d18ade79d4db521564d1b0dbb8764cbbc"
const MinioFolderChars = 6

// ************************
// Configuration file types

// Configuration file
type TomlConfig struct {
	Admin     AdminInfo
	Auth0     Auth0Info
	DB4S      DB4SInfo
	DiskCache DiskCacheInfo
	Memcache  MemcacheInfo
	Minio     MinioInfo
	Pg        PGInfo
	Sign      SigningInfo
	Web       WebInfo
}

// Config info for the admin server
type AdminInfo struct {
	Certificate    string
	CertificateKey string `toml:"certificate_key"`
	HTTPS          bool
	Server         string
}

// Auth0 connection parameters
type Auth0Info struct {
	ClientID     string
	ClientSecret string
	Domain       string
}

// Configuration info for the DB4S end point
type DB4SInfo struct {
	CAChain        string `toml:"ca_chain"`
	Certificate    string
	CertificateKey string `toml:"certificate_key"`
	Port           int
	Server         string
}

// Disk cache info
type DiskCacheInfo struct {
	Directory string
}

// Memcached connection parameters
type MemcacheInfo struct {
	DefaultCacheTime    int           `toml:"default_cache_time"`
	Server              string        `toml:"server"`
	ViewCountFlushDelay time.Duration `toml:"view_count_flush_delay"`
}

// Minio connection parameters
type MinioInfo struct {
	AccessKey string `toml:"access_key"`
	HTTPS     bool
	Secret    string
	Server    string
}

// PostgreSQL connection parameters
type PGInfo struct {
	Database       string
	NumConnections int `toml:"num_connections"`
	Port           int
	Password       string
	Server         string
	SSL            bool
	Username       string
}

// Used for signing DB4S client certificates
type SigningInfo struct {
	CertDaysValid    int    `toml:"cert_days_valid"`
	IntermediateCert string `toml:"intermediate_cert"`
	IntermediateKey  string `toml:"intermediate_key"`
}

type WebInfo struct {
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

type CommitEntry struct {
	AuthorEmail    string    `json:"author_email"`
	AuthorName     string    `json:"author_name"`
	CommitterEmail string    `json:"committer_email"`
	CommitterName  string    `json:"committer_name"`
	ID             string    `json:"id"`
	Message        string    `json:"message"`
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
	Folder    string
	DateEntry time.Time
	DBName    string
	Owner     string
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
	EntryType     DBTreeEntryType `json:"entry_type"`
	Last_Modified time.Time       `json:"last_modified"`
	LicenceSHA    string          `json:"licence"`
	Name          string          `json:"name"`
	Sha256        string          `json:"sha256"`
	Size          int             `json:"size"`
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
	Folder        string
	Forks         int
	FullDesc      string
	LastModified  time.Time
	Licence       string
	LicenceURL    string
	MRs           int
	OneLineDesc   string
	Public        bool
	Releases      int
	SHA256        string
	Size          int
	SourceURL     string
	Stars         int
	Tables        []string
	Tags          int
	Watchers      int
}

type ForkEntry struct {
	DBName     string
	Folder     string
	ForkedFrom int
	IconList   []ForkType
	ID         int
	Owner      string
	Processed  bool
	Public     bool
	Deleted    bool
}

type LicenceEntry struct {
	FileFormat string `json:"file_format"`
	FullName   string `json:"full_name"`
	Order      int    `json:"order"`
	Sha256     string `json:"sha256"`
	URL        string `json:"url"`
}

type MetaInfo struct {
	Database     string
	ForkDatabase string
	ForkDeleted  bool
	ForkFolder   string
	ForkOwner    string
	LoggedInUser string
	Owner        string
	Protocol     string
	Server       string
	Title        string
}

type ReleaseEntry struct {
	Commit        string    `json:"commit"`
	Date          time.Time `json:"date"`
	Description   string    `json:"description"`
	ReleaserEmail string    `json:"email"`
	ReleaserName  string    `json:"name"`
	Size          int       `json:"size"`
}

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

type WhereClause struct {
	Column string
	Type   string
	Value  string
}

type UserInfo struct {
	FullName     string `json:"full_name"`
	LastModified time.Time
	Username     string
}

type UserDetails struct {
	ClientCert []byte
	DateJoined time.Time
	Email      string
	Password   string
	PHash      []byte
	PVerify    string
	Username   string
}
