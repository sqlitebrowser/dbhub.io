package main

// Stand alone (non-daemon) utility to analyse various aspects of user resource usage
// At some point this may be turned into a daemon, but for now it's likely just to be
// run from cron on a periodic basis (ie every few hours)

import (
	"fmt"
	"log"

	"github.com/docker/go-units"
	com "github.com/sqlitebrowser/dbhub.io/common"
)

var (
	Debug = false
)

func main() {
	// Read server configuration
	err := com.ReadConfig()
	if err != nil {
		log.Fatalf("Configuration file problem: '%s'", err)
	}

	// Connect to PostgreSQL server
	err = com.ConnectPostgreSQL()
	if err != nil {
		log.Fatal(err)
	}

	// Connect to MQ server
	com.Conf.Live.Nodename = "Usage Analysis"
	com.AmqpChan, err = com.ConnectMQ()
	if err != nil {
		log.Fatal(err)
	}

	// Get the list of all users with at least one database
	userList, err := com.AnalysisUsersWithDBs()
	if err != nil {
		log.Fatalln(err)
	}

	type dbSizes struct {
		Live int64
		Standard int64
	}
	userStorage := make(map[string]dbSizes)

	// Loop through the users, calculating the total disk space used by each
	for user, numDBs := range userList {
		if Debug {
			fmt.Printf("User: %s, # databases: %d\n", user, numDBs)
		}

		// Get the list of standard databases for a user
		dbList, err := com.UserDBs(user, com.DB_BOTH)
		if err != nil {
			log.Fatal(err)
		}

		// For each standard database, count the list of commits and amount of space used
		var spaceUsedStandard int64
		for _, db := range dbList {
			commitList, err := com.GetCommitList(user, db.Database)
			if err != nil {
				log.Println(err)
			}

			// Calculate space used by standard databases
			for _, commit := range commitList {
				tree := commit.Tree.Entries
				for _, j := range tree {
					spaceUsedStandard += j.Size
				}
			}

			if Debug {
				fmt.Printf("User: %s, Standard database: %s, # Commits: %d, Space used: %s\n", user, db.Database, len(commitList), units.HumanSize(float64(spaceUsedStandard)))
			}
		}

		// Get the list of live databases for a user
		liveList, err := com.LiveUserDBs(user, com.DB_BOTH)
		if err != nil {
			log.Fatal(err)
		}

		// For each live database, get the amount of space used
		var spaceUsedLive int64
		for _, db := range liveList {
			_, liveNode, err := com.CheckDBLive(user, db.Database)
			if err != nil {
				log.Fatal(err)
				return
			}

			// Ask our AMQP backend for the database size
			z, err := com.LiveSize(liveNode, user, user, db.Database)
			if err != nil {
				log.Fatal(err)
			}
			spaceUsedLive += z

			if Debug {
				fmt.Printf("User: %s, Live database: %s, Space used: %s\n", user, db.Database, units.HumanSize(float64(spaceUsedLive)))
			}
		}
		userStorage[user] = dbSizes{Standard: spaceUsedStandard, Live: spaceUsedLive}
	}

	// Store the information in our PostgreSQL backend
	for user, z := range userStorage {
		err = com.AnalysisRecordUserStorage(user, z.Standard, z.Live)
		if err != nil {
			log.Fatalln()
		}
	}

	log.Printf("%s run complete", com.Conf.Live.Nodename)
}