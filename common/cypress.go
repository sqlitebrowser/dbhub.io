package common

/* Shared backend code we need that is specific to testing with Cypress */

import (
	"log"
	"net/http"
	"os"
	"path"
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

	// Add test SQLite databases
	testDB, err := os.Open(path.Join(Conf.Web.BaseDir, "cypress", "test_data", "Assembly Election 2017.sqlite"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer testDB.Close()
	_, _, _, err = AddDatabase("default", "default", "/", "Assembly Election 2017.sqlite",
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
	_, _, _, err = AddDatabase("default", "default", "/", "Assembly Election 2017 with view.sqlite",
		false, "", "", SetToPrivate, "CC-BY-SA-4.0", "Initial commit",
		"http://data.nicva.org/dataset/assembly-election-2017", testDB2, time.Now(), time.Time{},
		"", "", "", "", nil, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Add some test users
	err = AddUser("auth0first", "first", RandomString(32), "first@example.org", "First test user", "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = AddUser("auth0second", "second", RandomString(32), "second@example.org", "Second test user", "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = AddUser("auth0third", "third", RandomString(32), "third@example.org", "Third test user", "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Add some API keys
	keys := map[string]string{
		"2MXwA5jGZkIQ3UNEcKsuDNSPMlx": "default",
		"2MXw0cd7IBAGR6mm0JX6O5BdySJ": "default",
		"2MXwB8hvXgUHlCkXq5odLe4L05j": "default",
		"2MXwGkD0il29I0e98rptPlfnABr": "first",
		"2MXwIsi2wUIqvzN6lNkpxqmsDQK": "second",
		"2MXwJkTQVonjJqNlpIFyA9BNtE6": "third",
	}
	for key, user := range keys {
		err = APIKeySave(key, user, time.Now())
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
