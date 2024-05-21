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

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	// PostgreSQL connection pool handle
	DB *pgpool.Pool

	// Database connection via Gorm
	gormDB *gorm.DB

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

	// Gorm connection string
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s connect_timeout=10 sslmode=", config.Conf.Pg.Server, uint16(config.Conf.Pg.Port), config.Conf.Pg.Username, config.Conf.Pg.Password, config.Conf.Pg.Database)

	// Enable encrypted connections where needed
	if config.Conf.Pg.SSL {
		pgConfig.ConnConfig.TLSConfig = &tlsConfig
		dsn += "require"
	} else {
		dsn += "disable"
	}

	// Connect to database
	DB, err = pgpool.New(context.Background(), pgConfig.ConnString())
	if err != nil {
		return fmt.Errorf("%s: couldn't connect to PostgreSQL server: %v", config.Conf.Live.Nodename, err)
	}

	// Additional connection pool via Gorm
	gormDB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("could not connect to database: %v", err)
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

	// Add default usage limits to the system
	err = AddDefaultUsageLimits()
	if err != nil {
		return
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

// ResetDB resets the database to its default state. eg for testing purposes
func ResetDB() error {
	// We probably don't want to drop the database itself, as that'd screw up the current database
	// connection.  Instead, lets truncate all the tables then load their default values
	tableNames := []string{
		"api_call_log",
		"api_keys",
		"database_downloads",
		"database_licences",
		"database_shares",
		"database_stars",
		"database_uploads",
		"db4s_connects",
		"discussion_comments",
		"discussions",
		"email_queue",
		"events",
		"sql_terminal_history",
		"sqlite_databases",
		"usage_limits",
		"users",
		"vis_params",
		"vis_query_runs",
		"watchers",
		"webui_logins",
		"analysis_space_used",
		"job_submissions",
		"job_responses",
	}

	sequenceNames := []string{
		"api_keys_key_id_seq",
		"api_log_log_id_seq",
		"database_downloads_dl_id_seq",
		"database_licences_lic_id_seq",
		"database_uploads_up_id_seq",
		"db4s_connects_connect_id_seq",
		"discussion_comments_com_id_seq",
		"discussions_disc_id_seq",
		"email_queue_email_id_seq",
		"events_event_id_seq",
		"sql_terminal_history_history_id_seq",
		"sqlite_databases_db_id_seq",
		"usage_limits_id_seq",
		"users_user_id_seq",
		"vis_query_runs_query_run_id_seq",
	}

	// Begin a transaction
	tx, err := DB.Begin(context.Background())
	if err != nil {
		return err
	}
	// Set up an automatic transaction roll back if the function exits without committing
	defer tx.Rollback(context.Background())

	// Truncate the database tables
	for _, tbl := range tableNames {
		// Ugh, string smashing just feels so wrong when working with SQL
		dbQuery := fmt.Sprintf("TRUNCATE TABLE %s CASCADE", tbl)
		_, err := DB.Exec(context.Background(), dbQuery)
		if err != nil {
			log.Printf("Error truncating table while resetting database: %s", err)
			return err
		}
	}

	// Reset the sequences
	for _, seq := range sequenceNames {
		dbQuery := fmt.Sprintf("ALTER SEQUENCE %v RESTART", seq)
		_, err := DB.Exec(context.Background(), dbQuery)
		if err != nil {
			log.Printf("Error restarting sequence while resetting database: %v", err)
			return err
		}
	}

	// Add default usage limits to the system
	err = AddDefaultUsageLimits()
	if err != nil {
		log.Fatal(err)
	}

	// Add the default user to the system
	err = AddDefaultUser()
	if err != nil {
		log.Fatal(err)
	}

	// Add the default licences
	err = AddDefaultLicences()
	if err != nil {
		log.Fatal(err)
	}

	// Commit the transaction
	err = tx.Commit(context.Background())
	if err != nil {
		return err
	}

	// Log the database reset
	log.Println("Database reset")
	return nil
}
