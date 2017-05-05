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
	Server         string
	HTTPS          bool
	Certificate    string
	CertificateKey string `toml:"certificate_key"`
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
	Server    string
	AccessKey string `toml:"access_key"`
	Secret    string
	HTTPS     bool
}

// PostgreSQL connection parameters
type PGInfo struct {
	Server   string
	Port     int
	Username string
	Password string
	Database string
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
	Owner     string
	Folder    string
	DBName    string
	DateEntry time.Time
}

type DBInfo struct {
	Database     string
	Tables       []string
	Watchers     int
	Stars        int
	Forks        int
	Discussions  int
	MRs          int
	Description  string
	Updates      int
	Branches     int
	Releases     int
	Contributors int
	Readme       string
	DateCreated  time.Time
	LastModified time.Time
	Public       bool
	Size         int
	Version      int
	Folder       string
	License      LicenseType
	DefaultTable string
}

type ForkEntry struct {
	Owner      string
	Folder     string
	DBName     string
	ID         int
	IconList   []ForkType
	ForkedFrom int
	Processed  bool
}

type MetaInfo struct {
	Protocol     string
	Server       string
	Title        string
	Owner        string
	Database     string
	LoggedInUser string
	ForkOwner    string
	ForkFolder   string
	ForkDatabase string
}

type SQLiteDBinfo struct {
	Info     DBInfo
	MaxRows  int
	MinioBkt string
	MinioId  string
}

type SQLiteRecordSet struct {
	Tablename string
	ColNames  []string
	ColCount  int
	RowCount  int
	TotalRows int
	Records   []DataRow
}

type WhereClause struct {
	Column string
	Type   string
	Value  string
}

type UserInfo struct {
	Username     string
	LastModified time.Time
}

type UserDetails struct {
	Username   string
	Email      string
	Password   string
	PVerify    string
	DateJoined time.Time
	ClientCert []byte
	PHash      []byte
}
