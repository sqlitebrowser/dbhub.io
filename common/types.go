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

// Store cached data in memcache for 30 days days (as a first guess, which will probably need tuning)
const CacheTime = 2592000

// Number of rows to display by default on the database page
const DefaultNumDisplayRows = 25

// The number of leading characters of a files' sha256 used as the Minio folder name
// eg: When set to 6, then "34f4255a737156147fbd0a44323a895d18ade79d4db521564d1b0dbb8764cbbc"
//        -> Minio folder: "34f425"
//        -> Minio filename: "5a737156147fbd0a44323a895d18ade79d4db521564d1b0dbb8764cbbc"
const MinioFolderChars = 6

// Number of connections to PostgreSQL to use
const PGConnections = 5

// ************************
// Configuration file types

// Configuration file
type TomlConfig struct {
	Admin AdminInfo
	Auth0 Auth0Info
	Cache CacheInfo
	DB4S  DB4SInfo
	Minio MinioInfo
	Pg    PGInfo
	Sign  SigningInfo
	Web   WebInfo
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

// Memcached connection parameters
type CacheInfo struct {
	Server string
}

// Configuration info for the DB4S end point
type DB4SInfo struct {
	CAChain        string `toml:"ca_chain"`
	Certificate    string
	CertificateKey string `toml:"certificate_key"`
	Port           int
	Server         string
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
	Database string
	Port     int
	Password string
	Server   string
	Username string
}

// Used for signing DB4S client certificates
type SigningInfo struct {
	IntermediateCert string `toml:"intermediate_cert"`
	IntermediateKey  string `toml:"intermediate_key"`
}

type WebInfo struct {
	BindAddress    string `toml:"bind_address"`
	Certificate    string
	CertificateKey string `toml:"certificate_key"`
	RequestLog     string `toml:"request_log"`
	ServerName     string `toml:"server_name"`
}

// End of configuration file types
// *******************************

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
	Order  int    `json:"order"`
	Sha256 string `json:"sha256"`
	URL    string `json:"url"`
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
	Message     string    `json:"message"`
	TaggerEmail string    `json:"email"`
	TaggerName  string    `json:"name"`
}

type WhereClause struct {
	Column string
	Type   string
	Value  string
}

type UserInfo struct {
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
