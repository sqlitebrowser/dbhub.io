package common

import (
	"fmt"
	"log"
)

type Diffs struct {
	Dummy string `json:"dummy"`	// TODO: This is only used for debugging purposes and needs to be removed later
}

// Diff
func Diff(ownerA string, folderA string, nameA string, commitA string, ownerB string, folderB string, nameB string, commitB string, loggedInUser string) (Diffs, error) {
	// Check if the user has access to the requested databases
	bucketA, idA, _, err := MinioLocation(ownerA, folderA, nameA, commitA, loggedInUser)
	if err != nil {
		return Diffs{}, err
	}
	bucketB, idB, _, err := MinioLocation(ownerB, folderB, nameB, commitB, loggedInUser)
	if err != nil {
		return Diffs{}, err
	}

	// Sanity check
	if idA == "" {
		// The requested database wasn't found, or the user doesn't have permission to access it
		err = fmt.Errorf("Requested database not found")
		log.Printf("Requested database not found: '%s%s%s'", ownerA, folderA, nameA)
		return Diffs{}, err
	}
	if idB == "" {
		// The requested database wasn't found, or the user doesn't have permission to access it
		err = fmt.Errorf("Requested database not found")
		log.Printf("Requested database not found: '%s%s%s'", ownerB, folderB, nameB)
		return Diffs{}, err
	}

	// Retrieve database files from Minio, using locally cached version if it's already there
	dbA, err := RetrieveDatabaseFile(bucketA, idA)
	if err != nil {
		return Diffs{}, err
	}
	dbB, err := RetrieveDatabaseFile(bucketB, idB)
	if err != nil {
		return Diffs{}, err
	}

	// Call dbDiff which does the actual diffing of the database files
	return dbDiff(dbA, dbB)
}

// dbDiff
func dbDiff(dbA string, dbB string) (Diffs, error) {
	var diff Diffs

	// Check if this is the same database and exit early
	if dbA == dbB {
		diff.Dummy = "same dbs"
		return diff, nil
	}

	// TODO
	diff.Dummy = "different dbs"

	// Return
	return diff, nil
}
