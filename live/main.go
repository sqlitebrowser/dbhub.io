package main

// Internal daemon for running SQLite queries sent by the other DBHub.io daemons

import (
	"errors"
	"log"
	"os"

	com "github.com/sqlitebrowser/dbhub.io/common"
)

func main() {
	// Read server configuration
	err := com.ReadConfig()
	if err != nil {
		log.Fatalf("Configuration file problem: '%s'", err)
	}

	// If node name and base directory were provided on the command line, then override the config file values
	if len(os.Args) == 3 {
		com.Conf.Live.Nodename = os.Args[1]
		com.Conf.Live.StorageDir = os.Args[2]
	}

	// If we don't have the node name or storage dir after reading both the config and command line, then abort
	if com.Conf.Live.Nodename == "" || com.Conf.Live.StorageDir == "" {
		log.Fatal("Node name or Storage directory missing.  Aborting")
	}

	// If it doesn't exist, create the base directory for storing SQLite files
	_, err = os.Stat(com.Conf.Live.StorageDir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Fatal(err)
		}

		// The target location doesn't exist
		err = os.MkdirAll(com.Conf.Live.StorageDir, 0750)
		if err != nil {
			log.Fatal(err)
		}
	}

	// Connect to Minio server
	err = com.ConnectMinio()
	if err != nil {
		log.Fatal(err)
	}

	// Connect to the main PostgreSQL server
	err = com.ConnectPostgreSQL()
	if err != nil {
		log.Fatal(err)
	}

	// Connect to the job queue
	com.CheckJobQueue = make(chan struct{})
	err = com.ConnectQueue()
	if err != nil {
		log.Fatal(err)
	}

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

	log.Printf("%s: listening for requests", com.Conf.Live.Nodename)

	// Endless loop
	var forever chan struct{}
	<-forever
}
