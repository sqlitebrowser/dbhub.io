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

type LicenseType int

const (
	// From http://opendefinition.org/licenses/
	CC0 LicenseType = iota
	PDDL
	CCBY
	ODCBY
	CCBYSA
	ODbL
	CCA
	CCSA
	DLDEBY
	DLDE0
	DSL
	FAL
	GNUFDL
	MIROSL
	OGLC
	OGLUK
	NONE
	OTHER
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

// Stored cached data in memcache for 1 full day (as a first guess, which will probably need training)
const CacheTime = 86400

// Number of rows to display by default on the database page
const DefaultNumDisplayRows = 25

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

type DBInfo struct {
	Branches     int
	Contributors int
	Database     string
	DateCreated  time.Time
	DefaultTable string
	Description  string
	Discussions  int
	Folder       string
	Forks        int
	LastModified time.Time
	License      LicenseType
	MRs          int
	Public       bool
	Readme       string
	Releases     int
	SHA256       string
	Size         int
	Stars        int
	Tables       []string
	Updates      int
	Version      int
	Watchers     int
}

type ForkEntry struct {
	DBName     string
	Folder     string
	ForkedFrom int
	IconList   []ForkType
	ID         int
	Owner      string
	Processed  bool
}

type MetaInfo struct {
	Database     string
	ForkDatabase string
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
