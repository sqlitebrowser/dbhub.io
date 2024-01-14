package main

// Stand alone (non-daemon) utility to analyse various aspects of user resource usage
// At some point this may be turned into a daemon, but for now it's likely just to be
// run from cron on a periodic basis (ie every few hours)

import (
	"log"
	"os"
	"time"

	"github.com/docker/go-units"
	com "github.com/sqlitebrowser/dbhub.io/common"
	"github.com/sqlitebrowser/dbhub.io/common/config"
	"github.com/sqlitebrowser/dbhub.io/common/database"
)

var (
	Debug = false

	// Historical controls whether to calculate historical space usage for each day, or just usage for the current date
	Historical = false
)

func main() {
	// Read server configuration
	err := config.ReadConfig()
	if err != nil {
		log.Fatalf("Configuration file problem: '%s'", err)
	}

	// Check if we should operate in Historical mode for this run
	if len(os.Args) > 1 && os.Args[1] == "--hist" {
		Historical = true
		log.Println("Historical mode enabled")
	}

	// Connect to database
	config.Conf.Live.Nodename = "Usage Analysis"
	err = database.Connect()
	if err != nil {
		log.Fatal(err)
	}

	// Get the list of all users with at least one database
	userList, err := com.AnalysisUsersWithDBs()
	if err != nil {
		log.Fatalln(err)
	}

	if Debug {
		log.Printf("# of users: %d", len(userList))
	}

	type dbSizes struct {
		Live     int64
		Standard int64
	}
	userStorage := make(map[string]dbSizes)

	// Loop through the users, calculating the total disk space used by each
	now := time.Now()
	if !Historical {
		for user, numDBs := range userList {
			if Debug {
				log.Printf("Processing user: %s, # databases: %d", user, numDBs)
			}

			// Get the list of standard databases for a user
			dbList, err := com.UserDBs(user, com.DB_BOTH)
			if err != nil {
				log.Fatal(err)
			}

			// For each standard database, count the list of commits and amount of space used
			var spaceUsedStandard int64
			for _, db := range dbList {
				// Get the commit list for the database
				commitList, err := com.GetCommitList(user, db.Database)
				if err != nil {
					log.Println(err)
				}

				// Calculate space used by standard databases across all time
				for _, commit := range commitList {
					tree := commit.Tree.Entries
					for _, j := range tree {
						spaceUsedStandard += j.Size
					}
				}

				if Debug {
					log.Printf("User: %s, Standard database: %s, # Commits: %d, Space used: %s", user, db.Database, len(commitList), units.HumanSize(float64(spaceUsedStandard)))
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

				// Ask our job queue backend for the database size
				z, err := com.LiveSize(liveNode, user, user, db.Database)
				if err != nil {
					log.Fatal(err)
				}
				spaceUsedLive += z

				if Debug {
					log.Printf("User: %s, Live database: %s, Space used: %s", user, db.Database, units.HumanSize(float64(spaceUsedLive)))
				}
			}
			userStorage[user] = dbSizes{Standard: spaceUsedStandard, Live: spaceUsedLive}
		}

		// Store the information in our PostgreSQL backend
		for user, z := range userStorage {
			err = com.AnalysisRecordUserStorage(user, now, z.Standard, z.Live)
			if err != nil {
				log.Fatalln()
			}
		}
	}

	// Do the historical storage analysis if requested by the caller
	if Historical {
		for user := range userList {
			// Get the date the user signed up
			details, err := database.User(user)
			if err != nil {
				log.Fatal(err)
			}
			joinDate := details.DateJoined

			if Debug {
				log.Printf("Processing user: '%s', Joined on: %s", user, joinDate.Format(time.RFC1123))
			}

			// Get the list of standard databases for a user
			dbList, err := com.UserDBs(user, com.DB_BOTH)
			if err != nil {
				log.Fatal(err)
			}

			type commitList map[string]database.CommitEntry
			dbCommits := make(map[string]commitList)

			// Loop through the days, calculating the space used each day since they joined until today
			pointInTime := joinDate
			for pointInTime.Before(now) {
				// Calculate the disk space used by all of the users' databases for the given day
				var spaceUsed int64
				for _, db := range dbList {
					// Get the commit list for the database, using a cache to reduce multiple database hits for the same info
					commits, ok := dbCommits[db.Database]
					if !ok {
						commits, err = com.GetCommitList(user, db.Database)
						if err != nil {
							log.Println(err)
						}
						dbCommits[db.Database] = commits
					}

					// Calculate the disk space used by this one database
					z, err := SpaceUsedBetweenDates(commits, joinDate, pointInTime)
					if err != nil {
						log.Fatal(err)
					}
					spaceUsed += z
				}

				// Record the storage space used by the database (until this date) to our backend
				err = com.AnalysisRecordUserStorage(user, pointInTime, spaceUsed, 0)
				if err != nil {
					log.Fatalln()
				}

				// Move the point in time forward by a day
				pointInTime = pointInTime.Add(time.Hour * 24)
			}
		}
	}

	log.Printf("%s run complete", config.Conf.Live.Nodename)
}

// SpaceUsedBetweenDates determines the storage space used by a standard database between two different dates
func SpaceUsedBetweenDates(commitList map[string]database.CommitEntry, startDate, endDate time.Time) (spaceUsed int64, err error) {
	// Check every commit in the database, adding the ones between the start and end dates to the usage total
	for _, commit := range commitList {
		if commit.Timestamp.After(startDate) && commit.Timestamp.Before(endDate) {
			// This commit is in the requested time range
			tree := commit.Tree.Entries
			for _, j := range tree {
				spaceUsed += j.Size
			}
		}
	}
	return
}
