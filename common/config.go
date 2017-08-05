package common

import (
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/BurntSushi/toml"
	"github.com/jackc/pgx"
	"github.com/minio/go-homedir"
)

var (
	// Our configuration info
	conf TomlConfig

	// PostgreSQL configuration info
	pgConfig = new(pgx.ConnConfig)
)

// Return the admin server certificate path.
func AdminServerCert() string {
	return conf.Admin.Certificate
}

// Return the admin server certificate key path.
func AdminServerCertKey() string {
	return conf.Admin.CertificateKey
}

// Should the admin server start using HTTPS?
func AdminServerHTTPS() bool {
	return conf.Admin.HTTPS
}

// Return the admin server address:port.
func AdminServerAddress() string {
	return conf.Admin.Server
}

// Return the Auth0 client ID.
func Auth0ClientID() string {
	return conf.Auth0.ClientID
}

// Return the Auth0 client secret.
func Auth0ClientSecret() string {
	return conf.Auth0.ClientSecret
}

// Return the Auth0 authentication domain.
func Auth0Domain() string {
	return conf.Auth0.Domain
}

// Return the path to the DB4S CA Chain file.
func DB4SCAChain() string {
	return conf.DB4S.CAChain
}

// Return the host:port string of the DB4S server.
func DB4SServer() string {
	return conf.DB4S.Server
}

// Return the path to the DB4S Server Certificate.
func DB4SServerCert() string {
	return conf.DB4S.Certificate
}

// Return the path to the DB4S Server Certificate key.
func DB4SServerCertKey() string {
	return conf.DB4S.CertificateKey
}

// Return the port number for the DB4S Server.
func DB4SServerPort() int {
	return conf.DB4S.Port
}

// Return the Minio server access key.
func MinioAccessKey() string {
	return conf.Minio.AccessKey
}

// Should we connect to the Minio server using HTTPS?
func MinioHTTPS() bool {
	return conf.Minio.HTTPS
}

// Return the Minio server secret.
func MinioSecret() string {
	return conf.Minio.Secret
}

// Return the Minio server string.
func MinioServer() string {
	return conf.Minio.Server
}

// Read the server configuration file.
func ReadConfig() error {
	// Reads the server configuration from disk
	// TODO: Might be a good idea to add permission checks of the dir & conf file, to ensure they're not
	// TODO: world readable.  Similar in concept to what ssh does for its config files.
	userHome, err := homedir.Dir()
	if err != nil {
		return fmt.Errorf("User home directory couldn't be determined: %s", "\n")
	}
	configFile := filepath.Join(userHome, ".dbhub", "config.toml")
	if _, err := toml.DecodeFile(configFile, &conf); err != nil {
		return fmt.Errorf("Config file couldn't be parsed: %v\n", err)
	}

	// Override config file via environment variables
	tempString := os.Getenv("MINIO_SERVER")
	if tempString != "" {
		conf.Minio.Server = tempString
	}
	tempString = os.Getenv("MINIO_ACCESS_KEY")
	if tempString != "" {
		conf.Minio.AccessKey = tempString
	}
	tempString = os.Getenv("MINIO_SECRET")
	if tempString != "" {
		conf.Minio.Secret = tempString
	}
	tempString = os.Getenv("MINIO_HTTPS")
	if tempString != "" {
		conf.Minio.HTTPS, err = strconv.ParseBool(tempString)
		if err != nil {
			return fmt.Errorf("Failed to parse MINIO_HTTPS: %v\n", err)
		}
	}
	tempString = os.Getenv("PG_SERVER")
	if tempString != "" {
		conf.Pg.Server = tempString
	}
	tempString = os.Getenv("PG_PORT")
	if tempString != "" {
		tempInt, err := strconv.ParseInt(tempString, 10, 0)
		if err != nil {
			return fmt.Errorf("Failed to parse PG_PORT: %v\n", err)
		}
		conf.Pg.Port = int(tempInt)
	}
	tempString = os.Getenv("PG_USER")
	if tempString != "" {
		conf.Pg.Username = tempString
	}
	tempString = os.Getenv("PG_PASS")
	if tempString != "" {
		conf.Pg.Password = tempString
	}
	tempString = os.Getenv("PG_DBNAME")
	if tempString != "" {
		conf.Pg.Database = tempString
	}

	// Verify we have the needed configuration information
	// Note - We don't check for a valid conf.Pg.Password here, as the PostgreSQL password can also be kept
	// in a .pgpass file as per https://www.postgresql.org/docs/current/static/libpq-pgpass.html
	var missingConfig []string
	if conf.Minio.Server == "" {
		missingConfig = append(missingConfig, "Minio server:port string")
	}
	if conf.Minio.AccessKey == "" {
		missingConfig = append(missingConfig, "Minio access key string")
	}
	if conf.Minio.Secret == "" {
		missingConfig = append(missingConfig, "Minio secret string")
	}
	if conf.Pg.Server == "" {
		missingConfig = append(missingConfig, "PostgreSQL server string")
	}
	if conf.Pg.Port == 0 {
		missingConfig = append(missingConfig, "PostgreSQL port number")
	}
	if conf.Pg.Username == "" {
		missingConfig = append(missingConfig, "PostgreSQL username string")
	}
	if conf.Pg.Password == "" {
		missingConfig = append(missingConfig, "PostgreSQL password string")
	}
	if conf.Pg.Database == "" {
		missingConfig = append(missingConfig, "PostgreSQL database string")
	}
	if len(missingConfig) > 0 {
		// Some config is missing
		returnMessage := fmt.Sprint("Missing or incomplete value(s):\n")
		for _, value := range missingConfig {
			returnMessage += fmt.Sprintf("\n \t→ %v", value)
		}
		return fmt.Errorf(returnMessage)
	}

	// Set the PostgreSQL configuration values
	pgConfig.Host = conf.Pg.Server
	pgConfig.Port = uint16(conf.Pg.Port)
	pgConfig.User = conf.Pg.Username
	pgConfig.Password = conf.Pg.Password
	pgConfig.Database = conf.Pg.Database
	clientTLSConfig := tls.Config{InsecureSkipVerify: true}
	if conf.Pg.SSL {
		pgConfig.TLSConfig = &clientTLSConfig
	} else {
		pgConfig.TLSConfig = nil
	}

	// TODO: Add environment variable overrides for memcached

	// The configuration file seems good
	return nil
}

// Return the path to the certificate used to sign DB4S client certs.
func SigningCert() string {
	return conf.Sign.IntermediateCert
}

// Return the path to the key for the certificate used to sign DB4S client certs.
func SigningCertKey() string {
	return conf.Sign.IntermediateKey
}

// Return the address the server listens on.
func WebBindAddress() string {
	return conf.Web.BindAddress
}

// Return the path to the Web server request log.
func WebRequestLog() string {
	return conf.Web.RequestLog
}

// Return the name of the Web server (from our configuration file).
func WebServer() string {
	return conf.Web.ServerName
}

// Return the path to the Web server certificate.
func WebServerCert() string {
	return conf.Web.Certificate
}

// Return the path to the Web server certificate key.
func WebServerCertKey() string {
	return conf.Web.CertificateKey
}

// Return the password for the Web server session store.
func WebServerSessionStorePassword() string {
	return conf.Web.SessionStorePassword
}
