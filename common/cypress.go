package common

/* Shared backend code we need that is specific to testing with Cypress */

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/sqlitebrowser/dbhub.io/common/config"
	"github.com/sqlitebrowser/dbhub.io/common/database"
)

// CypressSeed empties the backend database, then adds pre-defined test data (PostgreSQL and Minio)
func CypressSeed(w http.ResponseWriter, r *http.Request) {
	// Clear out database data
	if err := database.ResetDB(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Clear memcached
	if err := ClearCache(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Switch to the default user
	config.Conf.Environment.UserOverride = "default"

	// Change the email address of the default user to match the local server
	serverName := strings.Split(config.Conf.Web.ServerName, ":")
	err := database.SetUserPreferences("default", 10, "Default system user", fmt.Sprintf("default@%s", serverName[0]))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Add test SQLite databases
	testDB, err := os.Open(path.Join(config.Conf.Web.BaseDir, "cypress", "test_data", "Assembly Election 2017.sqlite"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer testDB.Close()
	_, _, _, err = AddDatabase("default", "default", "Assembly Election 2017.sqlite",
		false, "", "", database.SetToPublic, "CC-BY-SA-4.0", "Initial commit",
		"http://data.nicva.org/dataset/assembly-election-2017", testDB, time.Now(), time.Time{},
		"", "", "", "", nil, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	testDB2, err := os.Open(path.Join(config.Conf.Web.BaseDir, "cypress", "test_data", "Assembly Election 2017 with view.sqlite"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer testDB2.Close()
	_, _, _, err = AddDatabase("default", "default", "Assembly Election 2017 with view.sqlite",
		false, "", "", database.SetToPrivate, "CC-BY-SA-4.0", "Initial commit",
		"http://data.nicva.org/dataset/assembly-election-2017", testDB2, time.Now(), time.Time{},
		"", "", "", "", nil, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Add some tags
	commits, err := database.GetCommitList("default", "Assembly Election 2017.sqlite")
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
	liveDB1, err := os.Open(path.Join(config.Conf.Web.BaseDir, "cypress", "test_data", "Join Testing with index.sqlite"))
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
	liveNode, err := LiveCreateDB(dbOwner, dbName, objectID)
	if err != nil {
		log.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update PG, so it has a record of this database existing and knows the node/queue name for querying it
	err = database.LiveAddDatabasePG(dbOwner, dbName, objectID, liveNode, database.SetToPrivate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Enable the watch flag for the uploader for this database
	err = database.ToggleDBWatch(dbOwner, dbOwner, dbName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// *** Add a test LIVE SQLite database (end) ***

	// Add some test users
	err = database.AddUser("auth0first", "first", fmt.Sprintf("first@%s", serverName[0]), "First test user", "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = database.AddUser("auth0second", "second", fmt.Sprintf("second@%s", serverName[0]), "Second test user", "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = database.AddUser("auth0third", "third", fmt.Sprintf("third@%s", serverName[0]), "Third test user", "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = database.AddUser("auth0limited", "limited", fmt.Sprintf("limited@%s", serverName[0]), "Limited test user", "")
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
		"R4btZIUCGfLeIPJN1qDtBRuz7I6YWhiM2F0EOh3-neoLxqd9h7J8uw": "limited",
	}
	for key, user := range keys {
		_, err = database.APIKeySave(key, user, time.Now(), nil, database.MayReadAndWrite, "Cypress tests")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	_, err = database.APIKeySave("ReuYtI49nGGA6rEYaBPxS6qdK4mlYRvToucoxjw4ZDiOT9tJ6NxRXw", "default", time.Now(), nil, database.MayRead, "Cypress tests (ro)")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Println("API keys added to database")

	// Initialise random number generator with a static seed. This makes sure we get
	// reproducible numbers in each run.
	rand.Seed(0)

	// Simulate random API calls to fill usage table. This produces a relative uninteresting call log where
	// the simulated values are fairly evenly distributed.
	for i := 0; i < 500; i++ {
		sql := `INSERT INTO api_call_log (api_call_date, caller_id, db_owner_id, db_id, api_operation, api_caller_sw, key_id, method, status_code, runtime, request_size, response_size) VALUES
			($1, 1, 1, 1, '/v1/query', 'Cypress test', (SELECT key_id FROM api_keys WHERE user_id = 1 LIMIT 1), 'POST', 200, $2, $3, $4)`
		_, err := database.DB.Exec(context.Background(), sql, getRandomDate(), getRandomInt(500000, 7000000), getRandomInt(50, 300), getRandomInt(200, 5000))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	log.Println("API usage data added to database")

	// Add more restrictive usage limit for testing (max 1 call per second and 2 calls per hour)
	sql := `INSERT INTO usage_limits (name, description, rate_limits) VALUES ('restrictive', 'Used for Cypress testing', '[{"limit": 1, "period": "s", "increase": 1}, {"limit": 2, "period": "h", "increase": 4}]')`
	_, err = database.DB.Exec(context.Background(), sql)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Println("restrictive usage limit created")

	// Assign usage limits to test users. ID=2 always is the unlimited configuration.
	database.SetUserLimits("default", 2)
	database.SetUserLimits("first", 2)
	database.SetUserLimits("second", 2)
	database.SetUserLimits("third", 2)
	database.SetUserLimits("limited", 3) // ID=3 should be the 'restrictive' limit that was just created
	log.Println("Assigned usage limits to users")

	// Log the database reset
	log.Println("Test data added to database")
	return
}

// CreateRelease is used for creating a release when running tests
func CreateRelease(dbOwner, dbName, releaseName, releaseDescription, releaserName, releaserEmail, commitID string) (err error) {
	// Retrieve the existing releases for the database
	var releases map[string]database.ReleaseEntry
	releases, err = database.GetReleases(dbOwner, dbName)
	if err != nil {
		return
	}

	// Create the new release
	newRelease := database.ReleaseEntry{
		Commit:        commitID,
		Date:          time.Now(),
		Description:   releaseDescription,
		ReleaserEmail: releaserEmail,
		ReleaserName:  releaserName,
	}
	releases[releaseName] = newRelease

	// Store it in PostgreSQL
	err = database.StoreReleases(dbOwner, dbName, releases)
	return
}

// CreateTag is used for creating a tag when running tests
func CreateTag(dbOwner, dbName, tagName, tagDescription, taggerName, taggerEmail, commitID string) (err error) {
	// Retrieve the existing tags for the database
	var tags map[string]database.TagEntry
	tags, err = database.GetTags(dbOwner, dbName)
	if err != nil {
		return
	}

	// Create the new tag
	newTag := database.TagEntry{
		Commit:      commitID,
		Date:        time.Now(),
		Description: tagDescription,
		TaggerEmail: taggerEmail,
		TaggerName:  taggerName,
	}
	tags[tagName] = newTag

	// Store it in PostgreSQL
	err = database.StoreTags(dbOwner, dbName, tags)
	return
}

// EnvProd changes the running environment to be "production"
// NOTE - The route to call this is only available when the server is started in the "test" environment
func EnvProd(w http.ResponseWriter, r *http.Request) {
	config.Conf.Environment.Environment = "production"
	return
}

// EnvTest changes the running environment to be "test"
// NOTE - The route to call this is only available when the server is started in the "test" environment
func EnvTest(w http.ResponseWriter, r *http.Request) {
	config.Conf.Environment.Environment = "test"
	return
}

// GenCert generates a client certificate for the current user
func GenCert(w http.ResponseWriter, r *http.Request) {
	loggedInUser := config.Conf.Environment.UserOverride
	newCert, err := GenerateClientCert(loggedInUser)
	if err != nil {
		log.Printf("Error generating client certificate for user '%s': %s!", loggedInUser, err)
		return
	}

	// Send the client certificate to the user
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.cert.pem"`, loggedInUser))
	// Note, don't use "application/x-x509-user-cert", otherwise the browser may try to install it!
	// Useful reference info: https://pki-tutorial.readthedocs.io/en/latest/mime.html
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Write(newCert)
	return
}

// SwitchDefault changes the logged in user to be the user "default"
func SwitchDefault(w http.ResponseWriter, r *http.Request) {
	config.Conf.Environment.UserOverride = "default"
	return
}

// SwitchFirst changes the logged in user to be the test user "first"
func SwitchFirst(w http.ResponseWriter, r *http.Request) {
	config.Conf.Environment.UserOverride = "first"
	return
}

// SwitchSecond changes the logged in user to be the test user "second"
func SwitchSecond(w http.ResponseWriter, r *http.Request) {
	config.Conf.Environment.UserOverride = "second"
	return
}

// SwitchThird changes the logged in user to be the test user "third"
func SwitchThird(w http.ResponseWriter, r *http.Request) {
	config.Conf.Environment.UserOverride = "third"
	return
}

// TestLogout logs out the user for test runs
func TestLogout(w http.ResponseWriter, r *http.Request) {
	config.Conf.Environment.UserOverride = ""
	return
}

func getRandomDate() time.Time {
	min := time.Now().AddDate(0, -3, 0).Unix() // Three months ago
	max := time.Now().Unix()
	diff := max - min

	random := rand.Int63n(diff) + min
	return time.Unix(random, 0)
}

func getRandomInt(min, max int) int {
	return rand.Intn(max-min) + min
}
