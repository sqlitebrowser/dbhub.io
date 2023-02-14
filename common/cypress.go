package common

/* Shared backend code we need that is specific to testing with Cypress */

import (
	"log"
	"net/http"
	"os"
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

	// Add test SQLite database
	testDB, err := os.Open("/dbhub.io/cypress/test_data/Assembly Election 2017.sqlite")
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

	// Log the database reset
	log.Println("Test data added to database")
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
