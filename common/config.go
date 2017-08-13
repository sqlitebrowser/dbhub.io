package common

import (
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/BurntSushi/toml"
	"github.com/jackc/pgx"
	"github.com/minio/go-homedir"
)

var (
	// Our configuration info
	Conf TomlConfig

	// PostgreSQL configuration info
	pgConfig = new(pgx.ConnConfig)
)

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
	if _, err := toml.DecodeFile(configFile, &Conf); err != nil {
		return fmt.Errorf("Config file couldn't be parsed: %v\n", err)
	}

	// Override config file via environment variables
	tempString := os.Getenv("MINIO_SERVER")
	if tempString != "" {
		Conf.Minio.Server = tempString
	}
	tempString = os.Getenv("MINIO_ACCESS_KEY")
	if tempString != "" {
		Conf.Minio.AccessKey = tempString
	}
	tempString = os.Getenv("MINIO_SECRET")
	if tempString != "" {
		Conf.Minio.Secret = tempString
	}
	tempString = os.Getenv("MINIO_HTTPS")
	if tempString != "" {
		Conf.Minio.HTTPS, err = strconv.ParseBool(tempString)
		if err != nil {
			return fmt.Errorf("Failed to parse MINIO_HTTPS: %v\n", err)
		}
	}
	tempString = os.Getenv("PG_SERVER")
	if tempString != "" {
		Conf.Pg.Server = tempString
	}
	tempString = os.Getenv("PG_PORT")
	if tempString != "" {
		tempInt, err := strconv.ParseInt(tempString, 10, 0)
		if err != nil {
			return fmt.Errorf("Failed to parse PG_PORT: %v\n", err)
		}
		Conf.Pg.Port = int(tempInt)
	}
	tempString = os.Getenv("PG_USER")
	if tempString != "" {
		Conf.Pg.Username = tempString
	}
	tempString = os.Getenv("PG_PASS")
	if tempString != "" {
		Conf.Pg.Password = tempString
	}
	tempString = os.Getenv("PG_DBNAME")
	if tempString != "" {
		Conf.Pg.Database = tempString
	}

	// Verify we have the needed configuration information
	// Note - We don't check for a valid Conf.Pg.Password here, as the PostgreSQL password can also be kept
	// in a .pgpass file as per https://www.postgresql.org/docs/current/static/libpq-pgpass.html
	var missingConfig []string
	if Conf.Minio.Server == "" {
		missingConfig = append(missingConfig, "Minio server:port string")
	}
	if Conf.Minio.AccessKey == "" {
		missingConfig = append(missingConfig, "Minio access key string")
	}
	if Conf.Minio.Secret == "" {
		missingConfig = append(missingConfig, "Minio secret string")
	}
	if Conf.Pg.Server == "" {
		missingConfig = append(missingConfig, "PostgreSQL server string")
	}
	if Conf.Pg.Port == 0 {
		missingConfig = append(missingConfig, "PostgreSQL port number")
	}
	if Conf.Pg.Username == "" {
		missingConfig = append(missingConfig, "PostgreSQL username string")
	}
	if Conf.Pg.Password == "" {
		missingConfig = append(missingConfig, "PostgreSQL password string")
	}
	if Conf.Pg.Database == "" {
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

	// Warn if the certificate validity period isn't set in the config file
	if Conf.Sign.CertDaysValid == 0 {
		log.Printf("WARN: Cert validity period for cert signing isn't set in the config file. Defaulting to 60 days.\n")
		Conf.Sign.CertDaysValid = 60
	}

	// Warn if the default Memcache cache time isn't set in the config file
	if Conf.Memcache.DefaultCacheTime == 0 {
		log.Printf("WARN: Default Memcache cache time isn't set in the config file. Defaulting to 30 days.\n")
		Conf.Memcache.DefaultCacheTime = 2592000
	}

	// Warn if the view count flush delay isn't set in the config file
	if Conf.Memcache.ViewCountFlushDelay == 0 {
		log.Printf("WARN: Memcache view count flush delay isn't set in the config file. Defaulting to 2 minutes.\n")
		Conf.Memcache.ViewCountFlushDelay = 120
	}

	// Set the PostgreSQL configuration values
	pgConfig.Host = Conf.Pg.Server
	pgConfig.Port = uint16(Conf.Pg.Port)
	pgConfig.User = Conf.Pg.Username
	pgConfig.Password = Conf.Pg.Password
	pgConfig.Database = Conf.Pg.Database
	clientTLSConfig := tls.Config{InsecureSkipVerify: true}
	if Conf.Pg.SSL {
		pgConfig.TLSConfig = &clientTLSConfig
	} else {
		pgConfig.TLSConfig = nil
	}

	// TODO: Add environment variable overrides for memcached

	// The configuration file seems good
	return nil
}
