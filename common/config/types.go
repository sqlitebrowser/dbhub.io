package config

import "time"

// TomlConfig is a top level structure containing the server configuration information
type TomlConfig struct {
	Api         ApiConfig
	Auth0       Auth0Config
	DB4S        DB4SConfig
	Environment EnvConfig
	DiskCache   DiskCacheConfig
	Event       EventProcessingConfig
	Licence     LicenceConfig
	Live        LiveConfig
	Memcache    MemcacheConfig
	Minio       MinioConfig
	Pg          PGConfig
	Sign        SigningConfig
	Web         WebConfig
}

// ApiConfig contains configuration info for the API daemon
type ApiConfig struct {
	BaseDir        string `toml:"base_dir"`
	BindAddress    string `toml:"bind_address"`
	Certificate    string `toml:"certificate"`
	CertificateKey string `toml:"certificate_key"`
	RequestLog     string `toml:"request_log"`
	ServerName     string `toml:"server_name"`
}

// Auth0Config contains the Auth0 connection info used authenticating webUI users
type Auth0Config struct {
	ClientID     string
	ClientSecret string
	Domain       string
}

// DB4SConfig contains configuration info for the DB4S end point daemon
type DB4SConfig struct {
	CAChain        string `toml:"ca_chain"`
	Certificate    string
	CertificateKey string `toml:"certificate_key"`
	Debug          bool
	Port           int
	Server         string
}

// DiskCacheConfig contains the path to the root of the local disk cache
type DiskCacheConfig struct {
	Directory string
}

// EnvConfig holds information about the purpose of the running server.  eg "is this a production, docker,
// or development" instance?
type EnvConfig struct {
	Environment  string
	UserOverride string `toml:"user_override"`
}

// EventProcessingConfig hold configuration for the event processing loop
type EventProcessingConfig struct {
	Delay                     time.Duration `toml:"delay"`
	EmailQueueProcessingDelay time.Duration `toml:"email_queue_processing_delay"`
	Smtp2GoKey                string        `toml:"smtp2go_key"` // The SMTP2GO API key
}

// LicenceConfig -> LicenceDir holds the path to the licence files
type LicenceConfig struct {
	LicenceDir string `toml:"licence_dir"`
}

// LiveConfig holds configuration info for the Live database daemon
type LiveConfig struct {
	Nodename   string `toml:"node_name"`
	StorageDir string `toml:"storage_dir"`
}

// MemcacheConfig contains the Memcached configuration parameters
type MemcacheConfig struct {
	DefaultCacheTime    int           `toml:"default_cache_time"`
	Server              string        `toml:"server"`
	ViewCountFlushDelay time.Duration `toml:"view_count_flush_delay"`
}

// MinioConfig contains the Minio connection parameters
type MinioConfig struct {
	AccessKey string `toml:"access_key"`
	HTTPS     bool
	Secret    string
	Server    string
}

// PGConfig contains the PostgreSQL connection parameters
type PGConfig struct {
	Database       string
	NumConnections int `toml:"num_connections"`
	Port           int
	Password       string
	Server         string
	SSL            bool
	Username       string
}

// SigningConfig contains the info used for signing DB4S client certificates
type SigningConfig struct {
	CertDaysValid    int    `toml:"cert_days_valid"`
	Enabled          bool   `toml:"enabled"`
	IntermediateCert string `toml:"intermediate_cert"`
	IntermediateKey  string `toml:"intermediate_key"`
}

// WebConfig contains configuration info for the webUI daemon
type WebConfig struct {
	BaseDir              string `toml:"base_dir"`
	BindAddress          string `toml:"bind_address"`
	Certificate          string `toml:"certificate"`
	CertificateKey       string `toml:"certificate_key"`
	RequestLog           string `toml:"request_log"`
	ServerName           string `toml:"server_name"`
	SessionStorePassword string `toml:"session_store_password"`
}
