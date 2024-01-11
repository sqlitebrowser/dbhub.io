package common

/* Shared backend code we need that is specific to testing with Cypress */

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

// CypressSeed empties the backend database, then adds pre-defined test data (PostgreSQL and Minio)
func CypressSeed(w http.ResponseWriter, r *http.Request) {
	// Clear out database data
	if err := ResetDB(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Clear memcached
	if err := ClearCache(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Switch to the default user
	Conf.Environment.UserOverride = "default"

	// Change the email address of the default user to match the local server
	serverName := strings.Split(Conf.Web.ServerName, ":")
	err := SetUserPreferences("default", 10, "Default system user", fmt.Sprintf("default@%s", serverName[0]))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Add test SQLite databases
	testDB, err := os.Open(path.Join(Conf.Web.BaseDir, "cypress", "test_data", "Assembly Election 2017.sqlite"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer testDB.Close()
	_, _, _, err = AddDatabase("default", "default", "Assembly Election 2017.sqlite",
		false, "", "", SetToPublic, "CC-BY-SA-4.0", "Initial commit",
		"http://data.nicva.org/dataset/assembly-election-2017", testDB, time.Now(), time.Time{},
		"", "", "", "", nil, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	testDB2, err := os.Open(path.Join(Conf.Web.BaseDir, "cypress", "test_data", "Assembly Election 2017 with view.sqlite"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer testDB2.Close()
	_, _, _, err = AddDatabase("default", "default", "Assembly Election 2017 with view.sqlite",
		false, "", "", SetToPrivate, "CC-BY-SA-4.0", "Initial commit",
		"http://data.nicva.org/dataset/assembly-election-2017", testDB2, time.Now(), time.Time{},
		"", "", "", "", nil, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Add some tags
	commits, err := GetCommitList("default", "Assembly Election 2017.sqlite")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var commitID string
	for _, commit := range commits {
		commitID = commit.ID
	}
	err = CreateTag("default", "Assembly Election 2017.sqlite", "first",
		"First tag", "Example Tagger", "example@example.org", commitID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = CreateTag("default", "Assembly Election 2017.sqlite", "second",
		"Second tag", "Example Tagger", "example@example.org", commitID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Add some releases
	err = CreateRelease("default", "Assembly Election 2017.sqlite", "first",
		"First release", "Example Releaser", "example@example.org", commitID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = CreateRelease("default", "Assembly Election 2017.sqlite", "second",
		"Second release", "Example Releaser", "example@example.org", commitID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// *** Add a test LIVE SQLite database (start) ***

	// Open the live database file
	liveDB1, err := os.Open(path.Join(Conf.Web.BaseDir, "cypress", "test_data", "Join Testing with index.sqlite"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer liveDB1.Close()

	// Store the live database in Minio
	objectID, err := LiveStoreDatabaseMinio(liveDB1, "default", "Join Testing with index.sqlite", 16384)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send the live database file to our job queue backend for setup
	dbOwner := "default"
	dbName := "Join Testing with index.sqlite"
	liveNode, err := LiveCreateDB(AmqpChan, dbOwner, dbName, objectID)
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update PG, so it has a record of this database existing and knows the node/queue name for querying it
	err = LiveAddDatabasePG(dbOwner, dbName, objectID, liveNode, SetToPrivate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Enable the watch flag for the uploader for this database
	err = ToggleDBWatch(dbOwner, dbOwner, dbName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// *** Add a test LIVE SQLite database (end) ***

	// Add some test users
	err = AddUser("auth0first", "first", RandomString(32), fmt.Sprintf("first@%s", serverName[0]), "First test user", "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = AddUser("auth0second", "second", RandomString(32), fmt.Sprintf("second@%s", serverName[0]), "Second test user", "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = AddUser("auth0third", "third", RandomString(32), fmt.Sprintf("third@%s", serverName[0]), "Third test user", "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Add some API keys
	keys := map[string]string{
		// Old API key format
		"2MXwA5jGZkIQ3UNEcKsuDNSPMlx": "default",
		"2MXw0cd7IBAGR6mm0JX6O5BdySJ": "default",
		"2MXwB8hvXgUHlCkXq5odLe4L05j": "default",
		"2MXwGkD0il29I0e98rptPlfnABr": "first",
		"2MXwIsi2wUIqvzN6lNkpxqmsDQK": "second",
		"2MXwJkTQVonjJqNlpIFyA9BNtE6": "third",

		// New API key format
		"Rh3fPl6cl84XEw2FeWtj-FlUsn9OrxKz9oSJfe6kho7jT_1l5hizqw": "default",
		"Sr7oqnzG_l5yqf-fOtifYBPhMghnwQwSuIhoSciMqES2eD6kq7s52Q": "default",
		"JnEdDFCPFYggjNqIsS4kAUC_FJfEWdbseY4ZHH6ocgRhaLpok0VoeQ": "default",
		"KqHOvobv-lPcwFFYhQe426JWrsejPDWcaTJt3AKDTICeZDxOVpLt6Q": "first",
		"EdmNqQcJZQzIoArVCAu6bByhmVUe_Oa780avsoluO-yFixGxrQQuGw": "second",
		"NvPG_Vh8uxK4BqkN7yJiRA4HP2HxCC0XXw0TBQGXbsaSlVhXZDrb1g": "third",
	}
	for key, user := range keys {
		_, err = APIKeySave(key, user, time.Now(), nil, "Cypress tests")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	log.Println("API keys added to database")

	// Log the database reset
	log.Println("Test data added to database")
	return
}

// CreateRelease is used for creating a release when running tests
func CreateRelease(dbOwner, dbName, releaseName, releaseDescription, releaserName, releaserEmail, commitID string) (err error) {
	// Retrieve the existing releases for the database
	var releases map[string]ReleaseEntry
	releases, err = GetReleases(dbOwner, dbName)
	if err != nil {
		return
	}

	// Create the new release
	newRelease := ReleaseEntry{
		Commit:        commitID,
		Date:          time.Now(),
		Description:   releaseDescription,
		ReleaserEmail: releaserEmail,
		ReleaserName:  releaserName,
	}
	releases[releaseName] = newRelease

	// Store it in PostgreSQL
	err = StoreReleases(dbOwner, dbName, releases)
	return
}

// CreateTag is used for creating a tag when running tests
func CreateTag(dbOwner, dbName, tagName, tagDescription, taggerName, taggerEmail, commitID string) (err error) {
	// Retrieve the existing tags for the database
	var tags map[string]TagEntry
	tags, err = GetTags(dbOwner, dbName)
	if err != nil {
		return
	}

	// Create the new tag
	newTag := TagEntry{
		Commit:      commitID,
		Date:        time.Now(),
		Description: tagDescription,
		TaggerEmail: taggerEmail,
		TaggerName:  taggerName,
	}
	tags[tagName] = newTag

	// Store it in PostgreSQL
	err = StoreTags(dbOwner, dbName, tags)
	return
}

// EnvProd changes the running environment to be "production"
// NOTE - This route to call this is only available when the server is _started_ in the "test" environment
func EnvProd(w http.ResponseWriter, r *http.Request) {
	Conf.Environment.Environment = "production"
	return
}

// EnvTest changes the running environment to be "test"
// NOTE - This route to call this is only available when the server is _started_ in the "test" environment
func EnvTest(w http.ResponseWriter, r *http.Request) {
	Conf.Environment.Environment = "test"
	return
}

// SwitchDefault changes the logged in user to be the user "default"
func SwitchDefault(w http.ResponseWriter, r *http.Request) {
	Conf.Environment.UserOverride = "default"
	return
}

// SwitchFirst changes the logged in user to be the test user "first"
func SwitchFirst(w http.ResponseWriter, r *http.Request) {
	Conf.Environment.UserOverride = "first"
	return
}

// SwitchSecond changes the logged in user to be the test user "second"
func SwitchSecond(w http.ResponseWriter, r *http.Request) {
	Conf.Environment.UserOverride = "second"
	return
}

// SwitchThird changes the logged in user to be the test user "third"
func SwitchThird(w http.ResponseWriter, r *http.Request) {
	Conf.Environment.UserOverride = "third"
	return
}

// TestLogout logs out the user for test runs
func TestLogout(w http.ResponseWriter, r *http.Request) {
	Conf.Environment.UserOverride = ""
	return
}
