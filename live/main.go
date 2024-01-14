package main

// Internal daemon for running SQLite queries sent by the other DBHub.io daemons

import (
	"errors"
	"log"
	"os"

	com "github.com/sqlitebrowser/dbhub.io/common"
	"github.com/sqlitebrowser/dbhub.io/common/config"
	"github.com/sqlitebrowser/dbhub.io/common/database"
)

func main() {
	// Read server configuration
	err := config.ReadConfig()
	if err != nil {
		log.Fatalf("Configuration file problem: '%s'", err)
	}

	// If node name and base directory were provided on the command line, then override the config file values
	if len(os.Args) == 3 {
		config.Conf.Live.Nodename = os.Args[1]
		config.Conf.Live.StorageDir = os.Args[2]
	}

	// If we don't have the node name or storage dir after reading both the config and command line, then abort
	if config.Conf.Live.Nodename == "" || config.Conf.Live.StorageDir == "" {
		log.Fatal("Node name or Storage directory missing.  Aborting")
	}

	// If it doesn't exist, create the base directory for storing SQLite files
	_, err = os.Stat(config.Conf.Live.StorageDir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Fatal(err)
		}

		// The target location doesn't exist
		err = os.MkdirAll(config.Conf.Live.StorageDir, 0750)
		if err != nil {
			log.Fatal(err)
		}
	}

	// Connect to Minio server
	err = com.ConnectMinio()
	if err != nil {
		log.Fatal(err)
	}

	// Connect to database
	com.CheckJobQueue = make(chan struct{})
	err = database.Connect()
	if err != nil {
		log.Fatal(err)
	}

	// Start background signal handler
	exitSignal := make(chan struct{}, 1)
	go com.SignalHandler(&exitSignal)

	// Launch go workers to process submitted jobs
	go com.JobQueueCheck()
	go com.JobQueueListen()

	// Launch goroutine event generator for checking submitted jobs
	// TODO: This seems to work fine, but is kind of a pita to have enabled while developing this code atm.  So we disable it for now.
	// TODO: Instead of this, should we run some code on startup of the live nodes that checks the database for
	//       (recent) unhandled requests, and automatically generates a JobQueueCheck() event if some are found?
	//go func() {
	//	for {
	//		// Tell the JobQueueCheck() goroutine to check for newly submitted jobs
	//		com.CheckJobQueue <- struct{}{}
	//
	//		// Wait a second before the next check
	//		time.Sleep(1 * time.Second)
	//	}
	//}()

	log.Printf("%s: listening for requests", config.Conf.Live.Nodename)

	// Wait for exit signal
	<-exitSignal
}
