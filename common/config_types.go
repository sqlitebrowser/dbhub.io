package common

import "time"

// TomlConfig is a top level structure containing the server configuration information
type TomlConfig struct {
	Api         ApiInfo
	Auth0       Auth0Info
	DB4S        DB4SInfo
	Environment EnvInfo
	DiskCache   DiskCacheInfo
	Event       EventProcessingInfo
	Licence     LicenceInfo
	Live        LiveInfo
	Memcache    MemcacheInfo
	Minio       MinioInfo
	MQ          MQInfo
	Pg          PGInfo
	Sign        SigningInfo
	UserMgmt    UserMgmtInfo
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
	EmailQueueProcessingDelay time.Duration `toml:"email_queue_processing_delay"`
	Smtp2GoKey                string        `toml:"smtp2go_key"` // The SMTP2GO API key
}

// LicenceInfo -> LicenceDir holds the path to the licence files
type LicenceInfo struct {
	LicenceDir string `toml:"licence_dir"`
}

// LiveInfo holds configuration info for the Live database daemon
type LiveInfo struct {
	Nodename   string `toml:"node_name"`
	StorageDir string `toml:"storage_dir"`
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

// MQInfo contains the AMQP backend connection configuration info
type MQInfo struct {
	CertFile string `toml:"cert_file"`
	KeyFile  string `toml:"key_file"`
	Password string `toml:"password"`
	Port     int    `toml:"port"`
	Server   string `toml:"server"`
	Username string `toml:"username"`
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

// UserMgmtInfo contains the various settings for specific users, or groups of users
type UserMgmtInfo struct {
	BannedUsers       []string `toml:"banned_users"`        // List of users banned from the service
	SizeOverrideUsers []string `toml:"size_override_users"` // List of users allowed to override the database upload size limits
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
