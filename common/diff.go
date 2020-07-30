package common

import (
	"fmt"
	"io"
	"log"
	"strings"

	sqlite "github.com/gwenn/gosqlite"
)

type DiffType string

const (
	ACTION_ADD    DiffType = "add"
	ACTION_DELETE          = "delete"
	ACTION_MODIFY          = "modify"
)

type SchemaDiff struct {
	ActionType DiffType `json:"action_type"`
	Sql        string   `json:"sql"`
}

type DiffObjectChangeset struct {
	ObjectName string         `json:"object_name"`
	ObjectType string         `json:"object_type"`
	Schema  SchemaDiff        `json:"schema"`
}

type Diffs struct {
	Diff []DiffObjectChangeset  `json:"diff"`
}

// Diff generates the differences between the two commits commitA and commitB of the two databases specified in the other parameters
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

// dbDiff generates the differences between the two database files in dbA and dbD
func dbDiff(dbA string, dbB string) (Diffs, error) {
	var diff Diffs

	// Check if this is the same database and exit early
	if dbA == dbB {
		return diff, nil
	}

	// Open the first SQLite database in read only mode
	var sdb *sqlite.Conn
	sdb, err := sqlite.Open(dbA, sqlite.OpenReadOnly)
	if err != nil {
		log.Printf("Couldn't open database in dbDiff(): %s", err)
		return Diffs{}, err
	}
	if err = sdb.EnableExtendedResultCodes(true); err != nil {
		log.Printf("Couldn't enable extended result codes in dbDiff(): %v\n", err.Error())
		return Diffs{}, err
	}

	// Attach the second database
	err = sdb.Exec("ATTACH '" + dbB + "' AS aux")
	if err != nil {
		log.Printf("Couldn't attach database in dbDiff(): %s", err)
		return Diffs{}, err
	}

	// Get list of all objects in both databases, excluding virtual tables because they tend to be unpredictable
	var stmt *sqlite.Stmt
	stmt, err = sdb.Prepare("SELECT name, type FROM main.sqlite_master WHERE name NOT LIKE 'sqlite_%' AND (type != 'table' OR (type = 'table' AND sql NOT LIKE 'CREATE VIRTUAL%%'))\n" +
				" UNION\n" +
				"SELECT name, type FROM aux.sqlite_master WHERE name NOT LIKE 'sqlite_%' AND (type != 'table' OR (type = 'table' AND sql NOT LIKE 'CREATE VIRTUAL%%'))\n" +
				" ORDER BY name")
	if err != nil {
		log.Printf("Error when preparing statement for object list in dbDiff(): %s\n", err)
		return Diffs{}, err
	}
	defer stmt.Finalize()
	err = stmt.Select(func(s *sqlite.Stmt) error {
		objectName, _ := s.ScanText(0)
		objectType, _ := s.ScanText(1)
		changed, objectDiff, err := diffSingleObject(sdb, objectName, objectType)
		if err != nil {
			return err
		}
		if changed {
			diff.Diff = append(diff.Diff, objectDiff)
		}
		return nil
	})
	if err != nil {
		log.Printf("Error when diffing single object in dbDiff: %s\n", err)
		return Diffs{}, err
	}

	// Return
	return diff, nil
}

// diffSingleObject compares the object with name objectName and of type objectType in the main and aux schemata of the connection sdb
// and returns three values: a boolean to indicate whether there are differences, a DiffObjectChangeset object containing all the differences, and an optional error object
func diffSingleObject(sdb *sqlite.Conn, objectName string, objectType string) (bool, DiffObjectChangeset, error) {
	// Prepare diff object to return
	var diff DiffObjectChangeset
	diff.ObjectName = objectName
	diff.ObjectType = objectType

	// Check for object's existence in both databases
	var sqlInMain, sqlInAux string
	err := sdb.OneValue("SELECT sql FROM main.sqlite_master WHERE name = ? AND type = ?", &sqlInMain, objectName, objectType)
	if err != nil && err != io.EOF {	// io.EOF is okay. It is returned when the object does not exist in the main database
		return false, diff, err
	}
	err = sdb.OneValue("SELECT sql FROM aux.sqlite_master WHERE name = ? AND type = ?", &sqlInAux, objectName, objectType)
	if err != nil && err != io.EOF {	// io.EOF is okay. It is returned when the object does not exist in the aux database
		return false, diff, err
	}

	// Check for dropped object
	if sqlInMain != "" && sqlInAux == "" {
		diff.Schema.ActionType = ACTION_DELETE
		diff.Schema.Sql = "DROP " + strings.ToUpper(objectType) + " "  + EscapeId(objectName) + ";"

		// No data changes for added objects so we can return here
		return true, diff, nil 
	}

	// Check for added object
	if sqlInMain == "" && sqlInAux != "" {
		diff.Schema.ActionType = ACTION_ADD
		diff.Schema.Sql = sqlInAux + ";"

		// TODO If this is a table, also add all the data to the diff

		return true, diff, nil
	}

	// Check for modified object
	if sqlInMain != "" && sqlInAux != "" && sqlInMain != sqlInAux {
		diff.Schema.ActionType = ACTION_MODIFY
		diff.Schema.Sql = "DROP " + strings.ToUpper(objectType) + " "  + EscapeId(objectName) + ";" + sqlInAux + ";"

		// TODO If this is a table, be more clever and try to get away with ALTER TABLE instead of DROP and CREATE

		// TODO If this is a table, also add all the data to the diff

		return true, diff, nil
	}

	// TODO If this is a table, check for modified data

	// Nothing has changed
	return false, diff, nil
}
