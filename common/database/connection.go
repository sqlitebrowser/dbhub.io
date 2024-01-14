package database

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/url"

	"github.com/sqlitebrowser/dbhub.io/common/config"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	pgx "github.com/jackc/pgx/v5"
	pgpool "github.com/jackc/pgx/v5/pgxpool"
)

var (
	// PostgreSQL connection pool handle
	DB *pgpool.Pool

	// JobListen is the PG server connection used for receiving PG notifications
	JobListen *pgx.Conn

	// JobQueue is the PG server connection used for submitting and retrieving jobs
	JobQueue *pgpool.Pool
)

// Connect creates a connection pool to the PostgreSQL server and a connection to the backend queue server
func Connect() (err error) {
	// Prepare TLS configuration
	tlsConfig := tls.Config{}
	if config.Conf.Environment.Environment == "production" {
		tlsConfig.ServerName = config.Conf.Pg.Server
		tlsConfig.InsecureSkipVerify = false
	} else {
		tlsConfig.InsecureSkipVerify = true
	}

	// Set the main PostgreSQL database configuration values
	pgConfig, err := pgpool.ParseConfig(fmt.Sprintf("host=%s port=%d user= %s password = %s dbname=%s pool_max_conns=%d connect_timeout=10", config.Conf.Pg.Server, uint16(config.Conf.Pg.Port), config.Conf.Pg.Username, config.Conf.Pg.Password, config.Conf.Pg.Database, config.Conf.Pg.NumConnections))
	if err != nil {
		return
	}

	if config.Conf.Pg.SSL {
		pgConfig.ConnConfig.TLSConfig = &tlsConfig
	}

	// Connect to database
	DB, err = pgpool.New(context.Background(), pgConfig.ConnString())
	if err != nil {
		return fmt.Errorf("Couldn't connect to PostgreSQL server: %v", err)
	}

	// migrate doesn't handle pgx connection strings, so we need to manually create something it can use
	var mConnStr string
	if config.Conf.Environment.Environment == "production" {
		mConnStr = fmt.Sprintf("pgx5://%s@%s:%d/%s?password=%s&connect_timeout=10", config.Conf.Pg.Username, config.Conf.Pg.Server,
			uint16(config.Conf.Pg.Port), config.Conf.Pg.Database, url.PathEscape(config.Conf.Pg.Password))
	} else {
		// Non-production, so probably our Docker test container
		mConnStr = "pgx5://dbhub@localhost:5432/dbhub"
	}
	if config.Conf.Pg.SSL {
		mConnStr += "&sslmode=require"
	}
	m, err := migrate.New(fmt.Sprintf("file://%s/database/migrations", config.Conf.Web.BaseDir), mConnStr)
	if err != nil {
		return
	}

	// Bizarrely, migrate throws a "no change" error when there are no migrations to apply.  So, we work around it:
	// https://github.com/golang-migrate/migrate/issues/485
	err = m.Up()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return
	}

	// Log successful connection
	log.Printf("%v: connected to PostgreSQL server: %v:%v", config.Conf.Live.Nodename, config.Conf.Pg.Server, uint16(config.Conf.Pg.Port))

	// Create the connection string for the dedicated PostgreSQL notification connection
	listenConfig, err := pgx.ParseConfig(fmt.Sprintf("host=%s port=%d user= %s password = %s dbname=%s connect_timeout=10", config.Conf.Pg.Server, uint16(config.Conf.Pg.Port), config.Conf.Pg.Username, config.Conf.Pg.Password, config.Conf.Pg.Database))
	if err != nil {
		return
	}

	if config.Conf.Pg.SSL {
		listenConfig.TLSConfig = &tlsConfig
	}

	// Connect to PostgreSQL based queue server
	// Note: JobListen uses a dedicated, non-pooled connection to the job queue database, while JobQueue uses
	// a standard database connection pool
	JobListen, err = pgx.ConnectConfig(context.Background(), listenConfig)
	if err != nil {
		return fmt.Errorf("%s: couldn't connect to backend queue server: %v", config.Conf.Live.Nodename, err)
	}
	JobQueue, err = pgpool.New(context.Background(), pgConfig.ConnString())
	if err != nil {
		return fmt.Errorf("%s: couldn't connect to backend queue server: %v", config.Conf.Live.Nodename, err)
	}

	// Add the default user to the system
	err = AddDefaultUser()
	if err != nil {
		return
	}

	// Add the default licences to the system
	err = AddDefaultLicences()
	if err != nil {
		return
	}

	return nil
}

// Disconnect disconnects the database connections
func Disconnect() {
	if DB != nil {
		DB.Close()
	}

	// Don't bother trying to close the job responses listener connection, as it just blocks
	//JobListen.Close(context.Background())

	// We're ok to close the Job Queue connection though, as that one doesn't block
	if JobQueue != nil {
		JobQueue.Close()
	}
}
