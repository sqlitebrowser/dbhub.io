package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/icza/session"
	"github.com/rhinoman/go-commonmark"
	com "github.com/sqlitebrowser/dbhub.io/common"
	"golang.org/x/oauth2"
)

var (
	// Log file for incoming HTTPS requests
	reqLog *os.File

	// Our parsed HTML templates
	tmpl *template.Template
)

// auth0CallbackHandler is called at the end of the Auth0 authentication process, whether successful or not.
// If the authentication process was successful:
//  * if the user already has an account on our system then this function creates a login session for them.
//  * if the user doesn't yet have an account on our system, they're bounced to the username selection page.
// If the authentication process wasn't successful, an error message is displayed.
func auth0CallbackHandler(w http.ResponseWriter, r *http.Request) {
	// Auth0 login part, mostly copied from https://github.com/auth0-samples/auth0-golang-web-app (MIT License)
	conf := &oauth2.Config{
		ClientID:     com.Auth0ClientID(),
		ClientSecret: com.Auth0ClientSecret(),
		RedirectURL:  "https://" + com.WebServer() + "/x/callback",
		Scopes:       []string{"openid", "profile"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://" + com.Auth0Domain() + "/authorize",
			TokenURL: "https://" + com.Auth0Domain() + "/oauth/token",
		},
	}
	code := r.URL.Query().Get("code")
	token, err := conf.Exchange(oauth2.NoContext, code)
	if err != nil {
		log.Printf("Login failure: %s\n", err.Error())
		errorPage(w, r, http.StatusInternalServerError, "Login failed")
		return
	}

	// Retrieve the user info (JSON format)
	conn := conf.Client(oauth2.NoContext, token)
	userInfo, err := conn.Get("https://" + com.Auth0Domain() + "/userinfo")
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	raw, err := ioutil.ReadAll(userInfo.Body)
	defer userInfo.Body.Close()
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Convert the JSON into something usable
	var profile map[string]interface{}
	if err = json.Unmarshal(raw, &profile); err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Extract the information we need
	var auth0ID, email, nickName string
	em := profile["email"]
	if em != nil {
		email = em.(string)
	}
	au := profile["user_id"]
	if au != nil {
		auth0ID = au.(string)
	}
	if auth0ID == "" {
		log.Printf("Auth0 callback error: Auth0 ID string was empty. Email: %s\n", email)
		errorPage(w, r, http.StatusInternalServerError, "Error: Auth0 ID string was empty")
		return
	}
	ni := profile["nickname"]
	if ni != nil {
		nickName = ni.(string)
	}

	// If the user has an unverified email address, tell them to verify it before proceeding
	ve := profile["email_verified"]
	if ve != nil && ve.(bool) != true {
		// TODO: Create a nicer notice page for this, as errorPage() doesn't look friendly
		errorPage(w, r, http.StatusUnauthorized, "Please check your email.  You need to verify your "+
			"email address before logging in will work.")
		return
	}

	// Determine the DBHub.io username matching the given Auth0 ID
	userName, err := com.UserNameFromAuth0ID(auth0ID)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// If the user doesn't already exist, we need to create an account for them
	if userName == "" {
		if email != "" {
			// Check if the email address is already in our system
			exists, err := com.CheckEmailExists(email)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, "Email check failed.  Can't continue.")
				return
			}
			if exists {
				errorPage(w, r, http.StatusConflict,
					"Can't create new account: Your email address is already associated "+
						"with a different account in our system.")
				return
			}
		}
		// Create a special session cookie, purely for the registration page
		sess := session.NewSessionOptions(&session.SessOptions{
			CAttrs: map[string]interface{}{
				"registrationinprogress": true,
				"auth0id":                auth0ID,
				"email":                  email,
				"nickname":               nickName},
		})
		session.Add(sess, w)

		// Bounce to a new page, for the user to select their preferred username
		http.Redirect(w, r, "/selectusername", http.StatusTemporaryRedirect)
	}

	// Create session cookie for the user
	sess := session.NewSessionOptions(&session.SessOptions{
		CAttrs: map[string]interface{}{"UserName": userName},
	})
	session.Add(sess, w)

	// Login completed, so bounce to the users' profile page
	http.Redirect(w, r, "/"+userName, http.StatusTemporaryRedirect)
}

func createBranchHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			validSession = true
		} else {
			session.Remove(sess, w)
		}
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Extract and validate the form variables
	dbOwner, dbName, commit, err := com.GetFormUDC(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Missing or incorrect data supplied")
		return
	}
	branchName, err := com.GetFormBranch(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Missing or incorrect branch name")
		return
	}
	branchDesc := r.PostFormValue("branchdesc") // Optional

	// Check if the requested database exists
	dbFolder := "/"
	exists, err := com.CheckDBExists(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusBadRequest, fmt.Sprintf("Database '%s%s%s' doesn't exist", dbOwner, dbFolder,
			dbName))
		return
	}

	// Make sure the database owner matches the logged in user
	if loggedInUser != dbOwner {
		errorPage(w, r, http.StatusUnauthorized, "You can't change databases you don't own")
		return
	}

	// Read the branch heads list from the database
	branches, err := com.GetBranches(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Make sure the branch name doesn't already exist
	_, ok := branches[branchName]
	if ok {
		errorPage(w, r, http.StatusConflict, "A branch of that name already exists!")
		return
	}

	// Create the branch
	newBranch := com.BranchEntry{
		Commit:      commit,
		Description: branchDesc,
	}
	branches[branchName] = newBranch
	err = com.StoreBranches(dbOwner, dbFolder, dbName, branches)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Invalidate the memcache data for the database, so the new branch count gets picked up
	err = com.InvalidateCacheEntry(loggedInUser, dbOwner, dbFolder, dbName, "") // Empty string indicates "for all versions"
	if err != nil {
		// Something went wrong when invalidating memcached entries for the database
		log.Printf("Error when invalidating memcache entries: %s\n", err.Error())
		return
	}

	// Bounce to the branches page
	http.Redirect(w, r, fmt.Sprintf("/branches/%s%s%s", loggedInUser, dbFolder, dbName),
		http.StatusTemporaryRedirect)
}

func createTagHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			validSession = true
		} else {
			session.Remove(sess, w)
		}
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Extract and validate the form variables
	dbOwner, dbName, commit, err := com.GetFormUDC(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Missing or incorrect data supplied")
		return
	}
	tagName, err := com.GetFormTag(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Missing or incorrect branch name")
		return
	}
	tagMsg := r.PostFormValue("tagmsg") // Optional

	// Check if the requested database exists
	dbFolder := "/"
	exists, err := com.CheckDBExists(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusBadRequest, fmt.Sprintf("Database '%s%s%s' doesn't exist", dbOwner, dbFolder,
			dbName))
		return
	}

	// Make sure the database owner matches the logged in user
	if loggedInUser != dbOwner {
		errorPage(w, r, http.StatusUnauthorized, "You can't change databases you don't own")
		return
	}

	// Read the branch heads list from the database
	tags, err := com.GetTags(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Make sure the tag doesn't already exist
	_, ok := tags[tagName]
	if ok {
		errorPage(w, r, http.StatusConflict, "A tag of that name already exists!")
		return
	}

	// Create the tag
	name, email, err := com.GetUserDetails(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusConflict, "An error occurred when retrieving user details")
	}
	newTag := com.TagEntry{
		Commit:      commit,
		Date:        time.Now(),
		Message:     tagMsg,
		TaggerEmail: email,
		TaggerName:  name,
	}
	tags[tagName] = newTag

	// Store it in PostgreSQL
	err = com.StoreTags(dbOwner, dbFolder, dbName, tags)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Invalidate the memcache data for the database, so the new tag count gets picked up
	err = com.InvalidateCacheEntry(loggedInUser, dbOwner, dbFolder, dbName, "") // Empty string indicates "for all versions"
	if err != nil {
		// Something went wrong when invalidating memcached entries for the database
		log.Printf("Error when invalidating memcache entries: %s\n", err.Error())
		return
	}

	// Bounce to the tags page
	http.Redirect(w, r, fmt.Sprintf("/tags/%s%s%s", loggedInUser, dbFolder, dbName),
		http.StatusTemporaryRedirect)
}

func createUserHandler(w http.ResponseWriter, r *http.Request) {
	// Make sure this user creation session is valid
	sess := session.Get(r)
	if sess == nil {
		// This isn't a valid username selection session, so abort
		errorPage(w, r, http.StatusBadRequest, "Invalid user creation session")
		return
	}
	validRegSession := false
	va := sess.CAttr("registrationinprogress")
	if va == nil {
		// This isn't a valid username selection session, so abort
		errorPage(w, r, http.StatusBadRequest, "Invalid user creation session")
		return
	}
	validRegSession = va.(bool)
	if validRegSession != true {
		// This isn't a valid username selection session, so abort
		errorPage(w, r, http.StatusBadRequest, "Invalid user creation session")
		return
	}

	// Retrieve the registration data
	var auth0ID, email, displayName string
	au := sess.CAttr("auth0id")
	if au != nil {
		auth0ID = au.(string)
	} else {
		errorPage(w, r, http.StatusBadRequest, "Invalid user creation id")
		return
	}
	em := sess.CAttr("email")
	if em != nil {
		email = em.(string)
	} else {
		errorPage(w, r, http.StatusBadRequest, "Invalid user creation email")
		return
	}

	// Gather submitted form data (if any)
	err := r.ParseForm()
	if err != nil {
		log.Printf("Error when parsing user creation data: %s\n", err)
		errorPage(w, r, http.StatusBadRequest, "Error when parsing user creation data")
		return
	}
	userName := r.PostFormValue("username")

	// Ensure username was given
	if userName == "" {
		// No, so render the username selection page
		selectUserNamePage(w, r)
		return
	}

	// Validate the user supplied username
	err = com.ValidateUser(userName)
	if err != nil {
		log.Printf("Username failed validation: %s", err)
		session.Remove(sess, w)
		errorPage(w, r, http.StatusBadRequest, "Username failed validation")
		return
	}

	// Ensure the username isn't a reserved one
	err = com.ReservedUsernamesCheck(userName)
	if err != nil {
		log.Println(err)
		session.Remove(sess, w)
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if the username is already in our system
	exists, err := com.CheckUserExists(userName)
	if err != nil {
		session.Remove(sess, w)
		errorPage(w, r, http.StatusInternalServerError, "Username check failed")
		return
	}
	if exists {
		session.Remove(sess, w)
		errorPage(w, r, http.StatusConflict, "That username is already taken")
		return
	}

	// Add the user to the system
	// NOTE: We generate a random password here (for now).  We may remove the password field itself from the
	// database at some point, depending on whether we continue to support local database users
	err = com.AddUser(auth0ID, userName, com.RandomString(32), email, displayName)
	if err != nil {
		session.Remove(sess, w)
		errorPage(w, r, http.StatusInternalServerError, "Something went wrong during user creation")
		return
	}

	// Remove the temporary username selection session data
	session.Remove(sess, w)

	// Create normal session cookie for the user
	// TODO: This may leak a small amount of memory, but it's "good enough" for now while getting things working
	sess = session.NewSessionOptions(&session.SessOptions{
		CAttrs: map[string]interface{}{"UserName": userName},
	})
	session.Add(sess, w)

	// User creation completed, so bounce to the user's profile page
	http.Redirect(w, r, "/"+userName, http.StatusTemporaryRedirect)
}

// This is called from the username selection page, to check if a name is available.
func checkNameHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve the potential username from the URL
	err := r.ParseForm()
	userName := r.FormValue("name")

	// Validate the user supplied username
	err = com.ValidateUser(userName)
	if err != nil {
		fmt.Fprint(w, "n")
		return
	}

	// Ensure the username isn't a reserved one
	err = com.ReservedUsernamesCheck(userName)
	if err != nil {
		fmt.Fprint(w, "n")
		return
	}

	// Check if the username is already in our system
	exists, err := com.CheckUserExists(userName)
	if err != nil {
		fmt.Fprint(w, "n")
		return
	}
	if exists {
		fmt.Fprint(w, "n")
		return
	}

	// The username is available
	fmt.Fprint(w, "y")
	return
}

// This function deletes a branch.
func deleteBranchHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Delete Branch handler"

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			validSession = true
		} else {
			session.Remove(sess, w)
		}
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Extract the required form variables
	branchName := r.PostFormValue("branchName")
	dbFolder := r.PostFormValue("dbFolder")
	dbName := r.PostFormValue("dbName")
	dbOwner := r.PostFormValue("dbOwner")

	// If any of the required values were empty, indicate failure
	if branchName == "" || dbFolder == "" || dbName == "" || dbOwner == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// TODO: Validate the variables

	// Validate the database name
	err := com.ValidateDB(dbName)
	if err != nil {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the database exists in the system
	exists, err := com.CheckDBExists(dbOwner, dbFolder, dbName)
	if err != err {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !exists {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the database is owned by the logged in user. eg prevent changes to other people's databases
	if dbOwner != loggedInUser {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Load the existing branchHeads for the database
	branches, err := com.GetBranches(loggedInUser, dbFolder, dbName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Make sure the given branch exists
	_, ok := branches[branchName]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the branch being deleted isn't the default one
	defBranch, err := com.GetDefaultBranchName(dbOwner, dbFolder, dbName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if defBranch == branchName {
		w.WriteHeader(http.StatusConflict)
		return
	}

	// Delete the branch
	delete(branches, branchName)
	err = com.StoreBranches(dbOwner, dbFolder, dbName, branches)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Invalidate the memcache data for the database, so the new branch count gets picked up
	err = com.InvalidateCacheEntry(loggedInUser, dbOwner, dbFolder, dbName, "") // Empty string indicates "for all versions"
	if err != nil {
		// Something went wrong when invalidating memcached entries for the database
		log.Printf("Error when invalidating memcache entries: %s\n", err.Error())
		return
	}

	// Update succeeded
	w.WriteHeader(http.StatusOK)
}

// This function deletes the latest commit from a given branch.
func deleteCommitHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Delete commit handler"

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			validSession = true
		} else {
			session.Remove(sess, w)
		}
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Extract the required form variables
	branchName := r.PostFormValue("branchName")
	commit := r.PostFormValue("commit")
	dbFolder := r.PostFormValue("dbFolder")
	dbName := r.PostFormValue("dbName")
	dbOwner := r.PostFormValue("dbOwner")

	// If any of the required values were empty, indicate failure
	if branchName == "" || dbFolder == "" || dbName == "" || dbOwner == "" || commit == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// TODO: Validate the variables

	// Validate the database name
	err := com.ValidateDB(dbName)
	if err != nil {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the database exists in the system
	exists, err := com.CheckDBExists(dbOwner, dbFolder, dbName)
	if err != err {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !exists {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the database is owned by the logged in user. eg prevent changes to other people's databases
	if dbOwner != loggedInUser {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Load the existing branchHeads for the database
	branches, err := com.GetBranches(loggedInUser, dbFolder, dbName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Make sure the given branch exists
	b, ok := branches[branchName]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Check that the given commit matches the head commit of the branch
	if b.Commit != commit {
		w.WriteHeader(http.StatusConflict)
		return
	}

	// Delete the commit
	err = com.DeleteLatestBranchCommit(dbOwner, dbFolder, dbName, branchName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Invalidate the memcache data for the database, so the new branch count gets picked up
	err = com.InvalidateCacheEntry(loggedInUser, dbOwner, dbFolder, dbName, "") // Empty string indicates "for all versions"
	if err != nil {
		// Something went wrong when invalidating memcached entries for the database
		log.Printf("Error when invalidating memcache entries: %s\n", err.Error())
		return
	}

	// Update succeeded
	w.WriteHeader(http.StatusOK)
}

// This function deletes a database.
func deleteDatabaseHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Delete Database handler"

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			validSession = true
		} else {
			session.Remove(sess, w)
		}
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Extract the required form variables
	dbFolder := r.PostFormValue("dbFolder")
	dbName := r.PostFormValue("dbName")
	dbOwner := r.PostFormValue("dbOwner")

	// If any of the required values were empty, indicate failure
	if dbFolder == "" || dbName == "" || dbOwner == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// TODO: Validate the variables

	// Validate the database name
	err := com.ValidateDB(dbName)
	if err != nil {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the database exists in the system
	exists, err := com.CheckDBExists(dbOwner, dbFolder, dbName)
	if err != err {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !exists {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the database is owned by the logged in user. eg prevent changes to other people's databases
	if dbOwner != loggedInUser {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err = com.DeleteDatabase(dbOwner, dbFolder, dbName)
	if err != err {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Invalidate the memcache data for the database, so the new branch count gets picked up
	err = com.InvalidateCacheEntry(loggedInUser, dbOwner, dbFolder, dbName, "") // Empty string indicates "for all versions"
	if err != nil {
		// Something went wrong when invalidating memcached entries for the database
		log.Printf("Error when invalidating memcache entries: %s\n", err.Error())
		return
	}

	// Update succeeded
	w.WriteHeader(http.StatusOK)
}

// This function deletes a tag.
func deleteTagHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Delete Tag handler"

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			validSession = true
		} else {
			session.Remove(sess, w)
		}
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Extract the required form variables
	tagName := r.PostFormValue("tagName")
	dbFolder := r.PostFormValue("dbFolder")
	dbName := r.PostFormValue("dbName")
	dbOwner := r.PostFormValue("dbOwner")

	// If any of the required values were empty, indicate failure
	if tagName == "" || dbFolder == "" || dbName == "" || dbOwner == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// TODO: Validate the variables

	// Validate the database name
	err := com.ValidateDB(dbName)
	if err != nil {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the database exists in the system
	exists, err := com.CheckDBExists(dbOwner, dbFolder, dbName)
	if err != err {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !exists {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the database is owned by the logged in user. eg prevent changes to other people's databases
	if dbOwner != loggedInUser {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Load the existing tags for the database
	tags, err := com.GetTags(loggedInUser, dbFolder, dbName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Make sure the given tag exists
	_, ok := tags[tagName]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Delete the tag
	delete(tags, tagName)
	err = com.StoreTags(dbOwner, dbFolder, dbName, tags)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Invalidate the memcache data for the database, so the new tag count gets picked up
	err = com.InvalidateCacheEntry(loggedInUser, dbOwner, dbFolder, dbName, "") // Empty string indicates "for all versions"
	if err != nil {
		// Something went wrong when invalidating memcached entries for the database
		log.Printf("Error when invalidating memcache entries: %s\n", err.Error())
		return
	}

	// Update succeeded
	w.WriteHeader(http.StatusOK)
}

// Sends the X509 DB4S certificate to the user
func downloadCertHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			validSession = true
		} else {
			session.Remove(sess, w)
		}
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		// No logged in user, so error out
		errorPage(w, r, http.StatusBadRequest, "Not logged in")
		return
	}

	// Retrieve the client certificate from the PG database
	cert, err := com.ClientCert(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, fmt.Sprintf("Retrieving client cert from "+
			"database failed for user: %v", loggedInUser))
		return
	}

	// Send the client certificate to the user
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s",
		loggedInUser+".cert.pem"))
	// Note, don't use "application/x-x509-user-cert", otherwise the browser may try to install it!
	// Useful reference info: https://pki-tutorial.readthedocs.io/en/latest/mime.html
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Write(cert)
	return
}

func downloadCSVHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Download CSV"

	// Extract the username, database, table, and commit ID requested
	// NOTE - The commit ID is optional.  Without it, we just pick the latest commit from the (for now) default branch
	// TODO: Add support for passing in a specific branch, to get the latest commit for that instead
	dbOwner, dbName, dbTable, commitID, err := com.GetODTC(2, r) // 2 = Ignore "/x/download/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Abort if the table name was missing
	if dbTable == "" {
		log.Printf("%s: Missing table name\n", pageName)
		errorPage(w, r, http.StatusBadRequest, "Missing table name")
		return
	}

	// Retrieve session data (if any)
	var loggedInUser string
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
		} else {
			session.Remove(sess, w)
		}
	}

	// Verify the given database exists and is ok to be downloaded (and get the Minio bucket + id while at it)
	bucket, id, err := com.MinioLocation(dbOwner, "/", dbName, commitID, loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Get a handle from Minio for the database object
	sdb, tempFile, err := com.OpenMinioObject(bucket, id)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Read the table data from the database object
	resultSet, err := com.ReadSQLiteDBCSV(sdb, dbTable)

	// Close the SQLite database and delete the temp file
	defer func() {
		sdb.Close()
		os.Remove(tempFile)
	}()

	// Convert resultSet into CSV and send to the user
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.csv", url.QueryEscape(dbTable)))
	w.Header().Set("Content-Type", "text/csv")
	csvFile := csv.NewWriter(w)
	err = csvFile.WriteAll(resultSet)
	if err != nil {
		log.Printf("%s: Error when generating CSV: %v\n", pageName, err)
		errorPage(w, r, http.StatusInternalServerError, "Error when generating CSV")
		return
	}
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Download Handler"

	// NOTE - The commit ID is optional.  Without it, we just pick the latest commit from the (for now) default branch
	// TODO: Add support for passing in a specific branch, to get the latest commit for that instead
	dbOwner, dbName, commitID, err := com.GetODC(2, r) // 2 = Ignore "/x/download/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Retrieve session data (if any)
	var loggedInUser string
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
		} else {
			session.Remove(sess, w)
		}
	}

	// Verify the given database exists and is ok to be downloaded (and get the Minio bucket + id while at it)
	bucket, id, err := com.MinioLocation(dbOwner, "/", dbName, commitID, loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Get a handle from Minio for the database object
	userDB, err := com.MinioHandle(bucket, id)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Close the object handle when this function finishes
	defer func() {
		com.MinioHandleClose(userDB)
	}()

	// Send the database to the user
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", url.QueryEscape(dbName)))
	w.Header().Set("Content-Type", "application/x-sqlite3")
	bytesWritten, err := io.Copy(w, userDB)
	if err != nil {
		log.Printf("%s: Error returning DB file: %v\n", pageName, err)
		fmt.Fprintf(w, "%s: Error returning DB file: %v\n", pageName, err)
		return
	}

	// Log the number of bytes written
	log.Printf("%s: '%s/%s' downloaded. %d bytes", pageName, dbOwner, dbName, bytesWritten)
}

// Forks a database for the logged in user.
func forkDBHandler(w http.ResponseWriter, r *http.Request) {

	// TODO: This function will need updating to support folders

	// Retrieve user and database name
	dbOwner, dbName, commitID, err := com.GetODC(2, r) // 2 = Ignore "/x/forkdb/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Make sure a version number was given
	if commitID == "" {
		errorPage(w, r, http.StatusBadRequest, "No database version number given")
		return
	}

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			validSession = true
		} else {
			session.Remove(sess, w)
		}
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		// No logged in username, so nothing to update
		errorPage(w, r, http.StatusBadRequest, "To fork a database, you need to be logged in")
		return
	}

	// Check the user has access to the specific version of the source database requested
	allowed, err := com.CheckUserDBAccess(dbOwner, "/", dbName, loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !allowed {
		errorPage(w, r, http.StatusBadRequest, "You don't have access to the requested database version")
		return
	}

	// Make sure the source and destination owners are different
	if loggedInUser == dbOwner {
		errorPage(w, r, http.StatusBadRequest, "Forking your own database in-place doesn't make sense")
		return
	}

	// Make sure the user doesn't have a database of the same name already
	exists, err := com.CheckDBExists(loggedInUser, "/", dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if exists {
		// Database of the same name already exists
		errorPage(w, r, http.StatusBadRequest, "You already have a database of this name")
		return
	}

	// Add the forked database info to PostgreSQL
	_, err = com.ForkDatabase(dbOwner, "/", dbName, loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Invalidate the old memcached entry for the database
	err = com.InvalidateCacheEntry(loggedInUser, dbOwner, "/", dbName, "") // Empty string indicates "for all versions"
	if err != nil {
		// Something went wrong when invalidating memcached entries for the database
		log.Printf("Error when invalidating memcache entries: %s\n", err.Error())
		return
	}

	// Log the database fork
	log.Printf("Database '%s/%s' forked to user '%s'\n", dbOwner, dbName, loggedInUser)

	// Bounce to the page of the forked database
	http.Redirect(w, r, "/"+loggedInUser+"/"+dbName, http.StatusTemporaryRedirect)
}

// Present the forks page to the user
func forksHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve user and database name
	dbOwner, dbName, err := com.GetOD(1, r) // 1 = Ignore "/forks/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Render the forks page
	forksPage(w, r, dbOwner, "/", dbName)
}

// Generates a client certificate for the user and gives it to the browser.
func generateCertHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			validSession = true
		} else {
			session.Remove(sess, w)
		}
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		// No logged in user, so error out
		errorPage(w, r, http.StatusBadRequest, "Not logged in")
		return
	}

	// Generate a new certificate
	// TODO: Use 14 days for now.  Extend this when things work properly.
	newCert, err := com.GenerateClientCert(loggedInUser, 14)
	if err != nil {
		log.Printf("Error generating client certificate for user '%s': %s!\n", loggedInUser, err)
		http.Error(w, fmt.Sprintf("Error generating client certificate for user '%s': %s!\n",
			loggedInUser, err), http.StatusInternalServerError)
		return
	}

	// Store the new certificate in the database
	err = com.SetClientCert(newCert, loggedInUser)
	if err != nil {
		http.Error(w, fmt.Sprintf("Updating client certificate failed: %v", err),
			http.StatusInternalServerError)
		return
	}

	// Send the client certificate to the user
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s",
		loggedInUser+".cert.pem"))
	// Note, don't use "application/x-x509-user-cert", otherwise the browser may try to install it!
	// Useful reference info: https://pki-tutorial.readthedocs.io/en/latest/mime.html
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Write(newCert)
	return
}

// Removes the logged in users session information.
func logoutHandler(w http.ResponseWriter, r *http.Request) {
	// Remove session info
	sess := session.Get(r)
	if sess != nil {
		// Session data was present, so remove it
		session.Remove(sess, w)
	}

	// Bounce to the front page
	// TODO: This should probably reload the existing page instead
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

// Wrapper function to log incoming https requests.
func logReq(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if user is logged in
		var loggedInUser string
		sess := session.Get(r)
		if sess != nil {
			u := sess.CAttr("UserName")
			if u != nil {
				loggedInUser = u.(string)
			} else {
				loggedInUser = "-"
			}
		} else {
			loggedInUser = "-"
		}

		// Write request details to the request log
		fmt.Fprintf(reqLog, "%v - %s [%s] \"%s %s %s\" \"-\" \"-\" \"%s\" \"%s\"\n", r.RemoteAddr,
			loggedInUser, time.Now().Format(time.RFC3339Nano), r.Method, r.URL, r.Proto,
			r.Referer(), r.Header.Get("User-Agent"))

		// Call the original function
		fn(w, r)
	}
}

func main() {
	// Read server configuration
	var err error
	if err = com.ReadConfig(); err != nil {
		log.Fatalf("Configuration file problem\n\n%v", err)
	}

	// Open the request log for writing
	reqLog, err = os.OpenFile(com.WebRequestLog(), os.O_CREATE|os.O_APPEND|os.O_WRONLY|os.O_SYNC, 0750)
	if err != nil {
		log.Fatalf("Error when opening request log: %s\n", err)
	}
	defer reqLog.Close()
	log.Printf("Request log opened: %s\n", com.WebRequestLog())

	// Setup session storage
	session.Global.Close()
	session.Global = session.NewCookieManagerOptions(session.NewInMemStore(),
		&session.CookieMngrOptions{AllowHTTP: false})

	// Parse our template files
	tmpl = template.Must(template.New("templates").Delims("[[", "]]").ParseGlob("webui/templates/*.html"))

	// Connect to Minio server
	err = com.ConnectMinio()
	if err != nil {
		log.Fatalf(err.Error())
	}

	// Connect to PostgreSQL server
	err = com.ConnectPostgreSQL()
	if err != nil {
		log.Fatalf(err.Error())
	}

	// Connect to cache server
	err = com.ConnectCache()
	if err != nil {
		log.Fatalf(err.Error())
	}

	// Our pages
	http.HandleFunc("/", logReq(mainHandler))
	http.HandleFunc("/about", logReq(aboutPage))
	http.HandleFunc("/branches/", logReq(branchesPage))
	http.HandleFunc("/commits/", logReq(commitsPage))
	http.HandleFunc("/contributors/", logReq(contributorsPage))
	http.HandleFunc("/createbranch/", logReq(createBranchPage))
	http.HandleFunc("/createtag/", logReq(createTagPage))
	http.HandleFunc("/forks/", logReq(forksHandler))
	http.HandleFunc("/logout", logReq(logoutHandler))
	http.HandleFunc("/pref", logReq(prefHandler))
	http.HandleFunc("/register", logReq(createUserHandler))
	http.HandleFunc("/selectusername", logReq(selectUserNamePage))
	http.HandleFunc("/settings/", logReq(settingsPage))
	http.HandleFunc("/stars/", logReq(starsPage))
	http.HandleFunc("/tags/", logReq(tagsPage))
	http.HandleFunc("/upload/", logReq(uploadFormHandler))
	http.HandleFunc("/x/callback", logReq(auth0CallbackHandler))
	http.HandleFunc("/x/checkname", logReq(checkNameHandler))
	http.HandleFunc("/x/createbranch", logReq(createBranchHandler))
	http.HandleFunc("/x/createtag", logReq(createTagHandler))
	http.HandleFunc("/x/deletebranch/", logReq(deleteBranchHandler))
	http.HandleFunc("/x/deletecommit/", logReq(deleteCommitHandler))
	http.HandleFunc("/x/deletedatabase/", logReq(deleteDatabaseHandler))
	http.HandleFunc("/x/deletetag/", logReq(deleteTagHandler))
	http.HandleFunc("/x/download/", logReq(downloadHandler))
	http.HandleFunc("/x/downloadcert", logReq(downloadCertHandler))
	http.HandleFunc("/x/downloadcsv/", logReq(downloadCSVHandler))
	http.HandleFunc("/x/forkdb/", logReq(forkDBHandler))
	http.HandleFunc("/x/gencert", logReq(generateCertHandler))
	http.HandleFunc("/x/markdownpreview/", logReq(markdownPreview))
	http.HandleFunc("/x/savesettings", logReq(saveSettingsHandler))
	http.HandleFunc("/x/setdefaultbranch/", logReq(setDefaultBranchHandler))
	http.HandleFunc("/x/star/", logReq(starToggleHandler))
	http.HandleFunc("/x/table/", logReq(tableViewHandler))
	http.HandleFunc("/x/updatebranch/", logReq(updateBranchHandler))
	http.HandleFunc("/x/updatetag/", logReq(updateTagHandler))
	http.HandleFunc("/x/uploaddata/", logReq(uploadDataHandler))

	// Static files
	http.HandleFunc("/images/auth0.svg", logReq(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("webui", "images", "auth0.svg"))
	}))
	http.HandleFunc("/images/rackspace.svg", logReq(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("webui", "images", "rackspace.svg"))
	}))
	http.HandleFunc("/images/sqlitebrowser.svg", logReq(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("webui", "images", "sqlitebrowser.svg"))
	}))
	http.HandleFunc("/favicon.ico", logReq(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("webui", "favicon.ico"))
	}))
	http.HandleFunc("/robots.txt", logReq(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("webui", "robots.txt"))
	}))

	// Start server
	log.Printf("DBHub server starting on https://%s\n", com.WebServer())
	err = http.ListenAndServeTLS(com.WebBindAddress(), com.WebServerCert(), com.WebServerCertKey(), nil)

	// Shut down nicely
	com.DisconnectPostgreSQL()

	if err != nil {
		log.Fatal(err)
	}
}

func mainHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Main handler"

	// Split the request URL into path components
	pathStrings := strings.Split(r.URL.Path, "/")

	// numPieces will be 2 if the request was for the root directory (https://server/), or if
	// the request included only a single path component (https://server/someuser/)
	numPieces := len(pathStrings)
	if numPieces == 2 {
		userName := pathStrings[1]
		// Check if the request was for the root directory
		if pathStrings[1] == "" {
			// Yep, root directory request
			frontPage(w, r)
			return
		}

		// The request was for a user page
		userPage(w, r, userName)
		return
	}

	userName := pathStrings[1]
	dbName := pathStrings[2]

	// Validate the user supplied user and database name
	err := com.ValidateUserDB(userName, dbName)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Invalid user or database name")
		return
	}

	// This catches the case where a "/" is on the end of a user page URL
	// TODO: Refactor this and the above identical code.  Doing it this way is non-optimal
	if pathStrings[2] == "" {
		// The request was for a user page
		userPage(w, r, userName)
		return
	}

	// * A specific database was requested *

	// Check if a version number was also requested
	commitID, err := com.GetFormCommit(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Invalid database version number")
		return
	}

	// Check if a table name was also requested
	err = r.ParseForm()
	if err != nil {
		log.Printf("%s: Error with ParseForm() in main handler: %s\n", pageName, err)
	}
	dbTable := r.FormValue("table")

	// If a table name was supplied, validate it
	if dbTable != "" {
		err = com.ValidatePGTable(dbTable)
		if err != nil {
			// Validation failed, so don't pass on the table name
			log.Printf("%s: Validation failed for table name: %s", pageName, err)
			dbTable = ""
		}
	}

	// Check if a branch name was requested
	branchName, err := com.GetFormBranch(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Validation failed for branch name")
		return
	}

	// Check if a named tag was requested
	tagName, err := com.GetFormTag(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Validation failed for tag name")
		return
	}

	// Extract sort column, sort direction, and offset variables if present
	sortCol := r.FormValue("sort")
	sortDir := r.FormValue("dir")
	offsetStr := r.FormValue("offset")
	var rowOffset int
	if offsetStr == "" {
		rowOffset = 0
	} else {
		rowOffset, err = strconv.Atoi(offsetStr)
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}

		// Ensure the row offset isn't negative
		if rowOffset < 0 {
			rowOffset = 0
		}
	}

	// Sanity check the sort column name
	if sortCol != "" {
		// Validate the sort column text, as we use it in string smashing SQL queries so need to be even more
		// careful than usual
		err = com.ValidateFieldName(sortCol)
		if err != nil {
			log.Printf("Validation failed on requested sort field name '%v': %v\n", sortCol,
				err.Error())
			errorPage(w, r, http.StatusBadRequest, "Validation failed on requested sort field name")
			return
		}
	}

	// If a sort direction was provided, validate it
	if sortDir != "" {
		if sortDir != "ASC" && sortDir != "DESC" {
			errorPage(w, r, http.StatusBadRequest, "Invalid sort direction")
			return
		}
	}

	// TODO: Add support for folders and sub-folders in request paths
	databasePage(w, r, userName, dbName, commitID, dbTable, sortCol, sortDir, rowOffset, branchName, tagName)
}

// Returns HTML rendered content from a given markdown string, for the settings page README preview tab.
func markdownPreview(w http.ResponseWriter, r *http.Request) {
	// Extract the markdown text form value
	mkDown := r.PostFormValue("mkdown")

	// Send the rendered version back to the caller
	renderedText := commonmark.Md2Html(mkDown, commonmark.CMARK_OPT_DEFAULT)
	fmt.Fprint(w, renderedText)
}

// This handles incoming requests for the preferences page by logged in users.
func prefHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Preferences handler"

	// Ensure user is logged in
	var loggedInUser string
	validSession := false
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			validSession = true
		} else {
			session.Remove(sess, w)
		}
	}
	if validSession != true {
		// Display an error message
		// TODO: Show the login dialog (also for the settings page)
		errorPage(w, r, http.StatusForbidden, "Error: Must be logged in to view that page.")
		return
	}

	// Gather submitted form data (if any)
	maxRows := r.PostFormValue("maxrows")
	displayName := r.PostFormValue("fullname")
	email := r.PostFormValue("email")

	// If no form data was submitted, display the preferences page form
	if maxRows == "" {
		prefPage(w, r, fmt.Sprintf("%s", loggedInUser))
		return
	}

	// Basic sanity check
	if displayName == "" {
		errorPage(w, r, http.StatusBadRequest, "Full name can't be blank!")
		return
	}
	if email == "" {
		errorPage(w, r, http.StatusBadRequest, "Email address can't be blank!")
		return
	}

	// Validate submitted form data
	err := com.Validate.Var(maxRows, "required,numeric,min=1,max=500")
	if err != nil {
		log.Printf("%s: Maximum rows value failed validation: %s\n", pageName, err)
		errorPage(w, r, http.StatusBadRequest, "Error when parsing maximum rows preference value")
		return
	}
	maxRowsNum, err := strconv.Atoi(maxRows)
	if err != nil {
		log.Printf("%s: Error converting string '%v' to integer: %s\n", pageName, maxRows, err)
		errorPage(w, r, http.StatusBadRequest, "Error when parsing preference data")
		return
	}
	err = com.Validate.Var(displayName, "required,displayname,min=1,max=80")
	if err != nil {
		log.Printf("%s: Display name value failed validation: %s\n", pageName, err)
		errorPage(w, r, http.StatusBadRequest, "Error when parsing full name value")
		return
	}
	err = com.Validate.Var(email, "required,email")
	if err != nil {
		// Check for the special case of username@server, which may fail standard email validation checks
		// eg username@localhost, won't validate as an email address, but should be accepted anyway
		serverName := strings.Split(com.WebServer(), ":")
		em := fmt.Sprintf("%s@%s", loggedInUser, serverName[0])
		if email != em {
			log.Printf("%s: Email value failed validation: %s\n", pageName, err)
			errorPage(w, r, http.StatusBadRequest, "Error when parsing email value")
			return
		}
	}

	// Make sure the email address isn't already assigned to a different user
	a, err := com.GetUsernameFromEmail(email)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Error when checking email address")
		return
	}
	if a != "" && a != loggedInUser {
		errorPage(w, r, http.StatusBadRequest, "That email address is already associated with a different user")
		return
	}

	// TODO: Store previous email addresses in a database table that associates them with the username.  This will be
	// TODO  needed so looking an old email finds the correct username, such as looking through historical commit data

	// Update the preference data in the database
	err = com.SetPrefUserMaxRows(loggedInUser, maxRowsNum, displayName, email)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Error when updating preferences")
		return
	}

	// Bounce to the user home page
	http.Redirect(w, r, "/"+loggedInUser, http.StatusTemporaryRedirect)
}

// Handler for the Database Settings page
func saveSettingsHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Licence

	// Ensure user is logged in
	var loggedInUser string
	validSession := false
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			validSession = true
		} else {
			session.Remove(sess, w)
		}
	}
	if validSession != true {
		// Display an error message
		// TODO: Show the login dialog (also for the preferences page)
		errorPage(w, r, http.StatusForbidden, "Error: Must be logged in to view that page.")
		return
	}

	// Extract the username, folder, and (current) database name form variables
	u, dbFolder, dbName, err := com.GetFormUFD(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	dbOwner := strings.ToLower(u)

	// Default to the root folder if none was given
	if dbFolder == "" {
		dbFolder = "/"
	}

	// Make sure a username was given
	if len(dbOwner) == 0 || dbOwner == "" {
		// No username supplied
		errorPage(w, r, http.StatusBadRequest, "No username supplied!")
		return
	}

	// Make sure the database owner matches the logged in user
	if loggedInUser != dbOwner {
		errorPage(w, r, http.StatusBadRequest, "You can only change settings for your own databases.")
		return
	}

	// Extract the form variables
	oneLineDesc := r.PostFormValue("onelinedesc")
	newName := r.PostFormValue("newname")
	fullDesc := r.PostFormValue("fulldesc")
	sourceURL := r.PostFormValue("sourceurl")   // Optional
	defTable := r.PostFormValue("defaulttable") // TODO: Update the default table to be "per branch"

	// TODO: Validate the sourceURL field

	// Grab and validate the supplied "public" form field
	public, err := com.GetPub(r)
	if err != nil {
		log.Printf("Error when converting public value to boolean: %v\n", err)
		errorPage(w, r, http.StatusBadRequest, "Public value incorrect")
		return
	}

	// If set, validate the new database name
	if newName != dbName {
		err := com.ValidateDB(newName)
		if err != nil {
			log.Printf("Validation failed for new database name '%s': %s", newName, err)
			errorPage(w, r, http.StatusBadRequest, "New database name failed validation")
			return
		}
	}

	// Ensure the description is 80 chars or less
	if len(oneLineDesc) > 80 {
		errorPage(w, r, http.StatusBadRequest, "Description line needs to be 80 characters or less")
		return
	}

	// Validate the name of the default table
	err = com.ValidatePGTable(defTable)
	if err != nil {
		// Validation failed
		log.Printf("Validation failed for name of default table '%s': %s", defTable, err)
		errorPage(w, r, http.StatusBadRequest, "Validation failed for name of default table")
		return
	}

	// Retrieve the SHA256 for the database file
	var db com.SQLiteDBinfo
	err = com.DBDetails(&db, loggedInUser, dbOwner, dbFolder, dbName, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	dbSHA := db.Info.DBEntry.Sha256

	// Get a handle from Minio for the database object
	bkt := dbSHA[:com.MinioFolderChars]
	id := dbSHA[com.MinioFolderChars:]
	sdb, tempFile, err := com.OpenMinioObject(bkt, id)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Close the SQLite database and delete the temp file
	defer func() {
		sdb.Close()
		os.Remove(tempFile)
	}()

	// Retrieve the list of tables in the database
	// TODO: Update this to handle having a default table "per branch".  Even though it would mean looping here, it
	// TODO  seems like the only way to be flexible and accurate enough for our purposes
	tables, err := com.Tables(sdb, fmt.Sprintf("%s%s%s", dbOwner, dbFolder, dbName))
	defer sdb.Close()
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// If a specific table was requested, check that it's present
	if defTable != "" {
		// Check the requested table is present
		tablePresent := false
		for _, tbl := range tables {
			if tbl == defTable {
				tablePresent = true
			}
		}
		if tablePresent == false {
			// The requested table doesn't exist in the database
			log.Printf("Requested table '%s' not present in database '%s%s%s'\n",
				defTable, dbOwner, dbFolder, dbName)
			errorPage(w, r, http.StatusBadRequest, "Requested table not present")
			return
		}
	}

	// If the database doesn't have a 1-liner description, don't save the placeholder text as one
	if oneLineDesc == "No description" {
		oneLineDesc = ""
	}

	// Same thing, but for the full length description
	if fullDesc == "No full description" {
		fullDesc = ""
	}

	// Save settings
	err = com.SaveDBSettings(dbOwner, dbFolder, dbName, oneLineDesc, fullDesc, defTable, public, sourceURL)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// If the new database name is different from the old one, perform the rename
	// Note - It's useful to do this *after* the SaveDBSettings() call, so the cache invalidation code at the
	// end of that function gets run and we don't have to repeat it here
	// TODO: We'll probably need to add support for renaming folders somehow too
	if newName != "" && newName != dbName {
		err = com.RenameDatabase(dbOwner, dbFolder, dbName, newName)
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
	}

	// Settings saved, so bounce back to the database page
	http.Redirect(w, r, fmt.Sprintf("/%s%s%s", dbOwner, dbFolder, newName), http.StatusTemporaryRedirect)
}

// This function sets a branch as the default for a given database.
func setDefaultBranchHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Set default branch handler"

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			validSession = true
		} else {
			session.Remove(sess, w)
		}
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Extract the required form variables
	branchName := r.PostFormValue("branchName")
	dbFolder := r.PostFormValue("dbFolder")
	dbName := r.PostFormValue("dbName")
	dbOwner := r.PostFormValue("dbOwner")

	// If any of the required values were empty, indicate failure
	if branchName == "" || dbFolder == "" || dbName == "" || dbOwner == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// TODO: Validate the variables

	// Validate the database name
	err := com.ValidateDB(dbName)
	if err != nil {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the database exists in the system
	exists, err := com.CheckDBExists(dbOwner, dbFolder, dbName)
	if err != err {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !exists {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the database is owned by the logged in user. eg prevent changes to other people's databases
	if dbOwner != loggedInUser {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Load the existing branchHeads for the database
	branches, err := com.GetBranches(loggedInUser, dbFolder, dbName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Make sure the given branch exists
	_, ok := branches[branchName]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Set the default branch
	err = com.StoreDefaultBranchName(dbOwner, dbFolder, dbName, branchName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Update succeeded
	w.WriteHeader(http.StatusOK)
}

// Handles JSON requests from the front end to toggle a database's star.
func starToggleHandler(w http.ResponseWriter, r *http.Request) {
	// Extract the user and database name
	// TODO: Add folder support
	dbOwner, dbName, err := com.GetOD(2, r) // 2 = Ignore "/x/star/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			validSession = true
		} else {
			session.Remove(sess, w)
		}
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		// No logged in username, so nothing to update
		fmt.Fprint(w, "-1") // -1 tells the front end not to update the displayed star count
		return
	}

	// Toggle on or off the starring of a database by a user
	err = com.ToggleDBStar(loggedInUser, dbOwner, "/", dbName)
	if err != nil {
		fmt.Fprint(w, "-1") // -1 tells the front end not to update the displayed star count
		return
	}

	// Invalidate the old memcached entry for the database
	err = com.InvalidateCacheEntry(loggedInUser, dbOwner, "/", dbName, "") // Empty string indicates "for all versions"
	if err != nil {
		// Something went wrong when invalidating memcached entries for the database
		log.Printf("Error when invalidating memcache entries: %s\n", err.Error())
		return
	}

	// Return the updated star count
	newStarCount, err := com.DBStars(dbOwner, "/", dbName)
	if err != nil {
		fmt.Fprint(w, "-1") // -1 tells the front end not to update the displayed star count
		return
	}
	fmt.Fprint(w, newStarCount)
}

// This passes table row data back to the main UI in JSON format.
func tableViewHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Table data handler"

	// Retrieve user, database, and table name
	// TODO: Add folder support
	dbOwner, dbName, requestedTable, commitID, err := com.GetODTC(2, r) // 1 = Ignore "/x/table/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Extract sort column, sort direction, and offset variables if present
	sortCol := r.FormValue("sort")
	sortDir := r.FormValue("dir")
	offsetStr := r.FormValue("offset")
	var rowOffset int
	if offsetStr == "" {
		rowOffset = 0
	} else {
		rowOffset, err = strconv.Atoi(offsetStr)
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}

		// Ensure the row offset isn't negative
		if rowOffset < 0 {
			rowOffset = 0
		}
	}

	// Sanity check the sort column name
	if sortCol != "" {
		// Validate the sort column text, as we use it in string smashing SQL queries so need to be even more
		// careful than usual
		err = com.ValidateFieldName(sortCol)
		if err != nil {
			log.Printf("Validation failed on requested sort field name '%v': %v\n", sortCol,
				err.Error())
			errorPage(w, r, http.StatusBadRequest, "Validation failed on requested sort field name")
			return
		}
	}

	// If a sort direction was provided, validate it
	if sortDir != "" {
		if sortDir != "ASC" && sortDir != "DESC" {
			errorPage(w, r, http.StatusBadRequest, "Invalid sort direction")
			return
		}
	}

	// Retrieve session data (if any)
	var loggedInUser string
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
		} else {
			session.Remove(sess, w)
		}
	}

	// Check if the user has access to the requested database
	bucket, id, err := com.MinioLocation(dbOwner, "/", dbName, commitID, loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Sanity check
	if id == "" {
		// The requested database wasn't found
		log.Printf("%s: Requested database not found. Owner: '%s' Database: '%s'", pageName, dbOwner,
			dbName)
		return
	}

	// Determine the number of rows to display
	var maxRows int
	if loggedInUser != "" {
		// Retrieve the user preference data
		maxRows = com.PrefUserMaxRows(loggedInUser)
	} else {
		// Not logged in, so default to 10 rows
		maxRows = com.DefaultNumDisplayRows
	}

	// If the data is available from memcached, use that instead of reading from the SQLite database itself
	dataCacheKey := com.TableRowsCacheKey(fmt.Sprintf("tablejson/%s/%s/%d", sortCol, sortDir, rowOffset),
		loggedInUser, dbOwner, "/", dbName, commitID, requestedTable, maxRows)

	// If a cached version of the page data exists, use it
	var dataRows com.SQLiteRecordSet
	ok, err := com.GetCachedData(dataCacheKey, &dataRows)
	if err != nil {
		log.Printf("%s: Error retrieving table data from cache: %v\n", pageName, err)
	}
	if !ok {
		// * Data wasn't in cache, so we gather it from the SQLite database *

		// Open the Minio database
		sdb, tempFile, err := com.OpenMinioObject(bucket, id)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// Close the SQLite database and delete the temp file
		defer func() {
			sdb.Close()
			os.Remove(tempFile)
		}()

		// Retrieve the list of tables in the database
		tables, err := sdb.Tables("")
		if err != nil {
			log.Printf("Error retrieving table names: %s", err)
			return
		}
		if len(tables) == 0 {
			// No table names were returned, so abort
			log.Printf("The database '%s' doesn't seem to have any tables. Aborting.", dbName)
			return
		}

		// If a specific table was requested, check it exists
		if requestedTable != "" {
			tablePresent := false
			for _, tableName := range tables {
				if requestedTable == tableName {
					tablePresent = true
				}
			}
			if tablePresent == false {
				// The requested table doesn't exist
				errorPage(w, r, http.StatusBadRequest, "Requested table does not exist")
				return
			}
		}

		// If no specific table was requested, use the first one
		if requestedTable == "" {
			requestedTable = tables[0]
		}

		// If a sort column was requested, verify it exists
		if sortCol != "" {
			colList, err := sdb.Columns("", requestedTable)
			if err != nil {
				log.Printf("Error when reading column names for table '%s': %v\n", requestedTable,
					err.Error())
				errorPage(w, r, http.StatusInternalServerError, "Error when reading from the database")
				return
			}
			colExists := false
			for _, j := range colList {
				if j.Name == sortCol {
					colExists = true
				}
			}
			if colExists == false {
				// The requested sort column doesn't exist, so we fall back to no sorting
				sortCol = ""
			}
		}

		// Read the data from the database
		dataRows, err = com.ReadSQLiteDB(sdb, requestedTable, maxRows, sortCol, sortDir, rowOffset)
		if err != nil {
			// Some kind of error when reading the database data
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}

		// Count the total number of rows in the requested table
		dataRows.TotalRows, err = com.GetSQLiteRowCount(sdb, requestedTable)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// Cache the data in memcache
		err = com.CacheData(dataCacheKey, dataRows, com.CacheTime)
		if err != nil {
			log.Printf("%s: Error when caching table data: %v\n", pageName, err)
		}
	}

	// Format the output.  Use json.MarshalIndent() for nicer looking output
	jsonResponse, err := json.MarshalIndent(dataRows, "", " ")
	if err != nil {
		log.Println(err)
		return
	}

	//w.Header().Set("Access-Control-Allow-Origin", "*")
	fmt.Fprintf(w, "%s", jsonResponse)
}

// This function processes branch rename and description updates.
func updateBranchHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Update Branch handler"

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			validSession = true
		} else {
			session.Remove(sess, w)
		}
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Extract the required form variables
	branchName := r.PostFormValue("branchName")
	dbFolder := r.PostFormValue("dbFolder")
	dbName := r.PostFormValue("dbName")
	dbOwner := r.PostFormValue("dbOwner")
	newDesc := r.PostFormValue("newDesc")
	newName := r.PostFormValue("newName")

	// If any of the required values were empty, indicate failure
	if branchName == "" || dbFolder == "" || dbName == "" || dbOwner == "" || newDesc == "" || newName == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// TODO: Validate the variables

	// Validate the database name
	err := com.ValidateDB(dbName)
	if err != nil {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the database exists in the system
	exists, err := com.CheckDBExists(dbOwner, dbFolder, dbName)
	if err != err {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !exists {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the database is owned by the logged in user. eg prevent changes to other people's databases
	if dbOwner != loggedInUser {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Load the existing branchHeads for the database
	branches, err := com.GetBranches(loggedInUser, dbFolder, dbName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Make sure the given branch exists
	oldInfo, ok := branches[branchName]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Update the branch info
	delete(branches, branchName)
	branches[newName] = com.BranchEntry{
		Commit:      oldInfo.Commit,
		Description: newDesc,
	}
	err = com.StoreBranches(dbOwner, dbFolder, dbName, branches)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Update succeeded
	w.WriteHeader(http.StatusOK)
}

// This function processes tag rename and message updates.
func updateTagHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Update Tag handler"

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			validSession = true
		} else {
			session.Remove(sess, w)
		}
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Extract the required form variables
	tagName := r.PostFormValue("tagName")
	dbFolder := r.PostFormValue("dbFolder")
	dbName := r.PostFormValue("dbName")
	dbOwner := r.PostFormValue("dbOwner")
	newMsg := r.PostFormValue("newDesc")
	newName := r.PostFormValue("newName")

	// If any of the required values were empty, indicate failure
	if tagName == "" || dbFolder == "" || dbName == "" || dbOwner == "" || newMsg == "" || newName == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// TODO: Validate the variables

	// Validate the database name
	err := com.ValidateDB(dbName)
	if err != nil {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the database exists in the system
	exists, err := com.CheckDBExists(dbOwner, dbFolder, dbName)
	if err != err {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !exists {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the database is owned by the logged in user. eg prevent changes to other people's databases
	if dbOwner != loggedInUser {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Load the existing tags for the database
	tags, err := com.GetTags(loggedInUser, dbFolder, dbName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Make sure the given tag exists
	oldInfo, ok := tags[tagName]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Update the tag info
	delete(tags, tagName)
	tags[newName] = com.TagEntry{
		Commit:      oldInfo.Commit,
		Date:        oldInfo.Date,
		Message:     newMsg,
		TaggerEmail: oldInfo.TaggerEmail,
		TaggerName:  oldInfo.TaggerName,
	}

	err = com.StoreTags(dbOwner, dbFolder, dbName, tags)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Update succeeded
	w.WriteHeader(http.StatusOK)
}

// This function presents the database upload form to logged in users.
func uploadFormHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			validSession = true
		} else {
			session.Remove(sess, w)
		}
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Ensure the user has set their display name and email address
	displayName, email, err := com.GetUserDetails(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Error when retrieving user details")
		return
	}
	if displayName == "" || email == "" {
		errorPage(w, r, http.StatusBadRequest,
			"You need to set your full name and email address in Preferences first")
		return
	}

	// Render the upload page
	uploadPage(w, r, fmt.Sprintf("%s", loggedInUser))
}

// This function processes new database data submitted through the upload form.
func uploadDataHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Upload DB handler"

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess := session.Get(r)
	if sess != nil {
		u := sess.CAttr("UserName")
		if u != nil {
			loggedInUser = u.(string)
			validSession = true
		} else {
			session.Remove(sess, w)
		}
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Prepare the form data
	r.ParseMultipartForm(32 << 20) // 64MB of ram max
	if err := r.ParseForm(); err != nil {
		log.Printf("%s: ParseForm() error: %v\n", pageName, err)
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Grab and validate the supplied "public" form field
	public, err := com.GetPub(r)
	if err != nil {
		log.Printf("%s: Error when converting public value to boolean: %v\n", pageName, err)
		errorPage(w, r, http.StatusBadRequest, "Public value incorrect")
		return
	}

	// Extract the other form variables
	commitMsg := r.PostFormValue("commitmsg")
	sourceURL := r.PostFormValue("sourceurl")

	// TODO: Validate the input fields

	// TODO: Add (optional) branch name field to the upload form
	branchName, err := com.GetFormBranch(r) // Optional
	if err != nil {
		log.Printf("%s: Error when validating branch name '%s': %v\n", pageName, branchName, err)
		errorPage(w, r, http.StatusBadRequest, "Branch name value failed validation")
		return
	}

	// Ensure the one line description is 1024 chars or less.  1024 chars is probably a reasonable first guess as to a
	// useful limit
	if len(commitMsg) > 80 {
		errorPage(w, r, http.StatusBadRequest, "Commit message needs to be 1024 characters or less")
		return
	}

	// TODO: Add support for folders and sub-folders
	dbFolder := "/"

	tempFile, handler, err := r.FormFile("database")
	if err != nil {
		log.Printf("%s: Uploading file failed: %v\n", pageName, err)
		errorPage(w, r, http.StatusInternalServerError, "Database file missing from upload data?")
		return
	}
	dbName := handler.Filename
	defer tempFile.Close()

	// Validate the database name
	err = com.ValidateDB(dbName)
	if err != nil {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		errorPage(w, r, http.StatusBadRequest, "Invalid database name")
		return
	}

	// Write the temporary file locally, so we can try opening it with SQLite to verify it's ok
	var buf bytes.Buffer
	bytesWritten, err := io.Copy(&buf, tempFile)
	if err != nil {
		log.Printf("%s: Error: %v\n", pageName, err)
		errorPage(w, r, http.StatusInternalServerError, "Internal error")
		return
	}
	if bytesWritten == 0 {
		log.Printf("%s: Database seems to be 0 bytes in length. Username: %s, Database: %s\n", pageName,
			loggedInUser, dbName)
		errorPage(w, r, http.StatusBadRequest, "Database file is 0 length?")
		return
	}
	tempDB, err := ioutil.TempFile("", "dbhub-upload-")
	if err != nil {
		log.Printf("%s: Error creating temporary file. User: %s, Database: %s, Filename: %s, Error: %v\n",
			pageName, loggedInUser, dbName, tempDB.Name(), err)
		errorPage(w, r, http.StatusInternalServerError, "Internal error")
		return
	}
	_, err = tempDB.Write(buf.Bytes())
	if err != nil {
		log.Printf("%s: Error when writing the uploaded db to a temp file. User: %s, Database: %s"+
			"Error: %v\n", pageName, loggedInUser, dbName, err)
		errorPage(w, r, http.StatusInternalServerError, "Internal error")
		return
	}
	tempDBName := tempDB.Name()

	// Delete the temporary file when this function finishes
	defer os.Remove(tempDBName)

	// Sanity check the uploaded database
	err = com.SanityCheck(tempDBName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Generate sha256 of the uploaded file
	s := sha256.Sum256(buf.Bytes())
	sha := hex.EncodeToString(s[:])

	// Check if the database already exists in the system
	needDefaultBranchCreated := false
	var branches map[string]com.BranchEntry
	exists, err := com.CheckDBExists(loggedInUser, dbFolder, dbName)
	if err != err {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if exists {
		// Load the existing branchHeads for the database
		branches, err = com.GetBranches(loggedInUser, dbFolder, dbName)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// If no branch name was given, use the default for the database
		if branchName == "" {
			branchName, err = com.GetDefaultBranchName(loggedInUser, dbFolder, dbName)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
	} else {
		// No existing branches, so this will be the first
		branches = make(map[string]com.BranchEntry)

		// Set the default branch name for the database
		if branchName == "" {
			branchName = "master"
		}
		needDefaultBranchCreated = true
	}

	// Create a dbTree entry for the individual database file
	var e com.DBTreeEntry
	e.EntryType = com.DATABASE
	e.Name = dbName
	e.Sha256 = sha
	e.Last_Modified = time.Now()
	// TODO: Check if there's a way to pass the last modified timestamp through a standard file upload control.  If
	// TODO  not, then it might only be possible through dio cli and similar
	//e.Last_Modified, err = time.Parse(time.RFC3339, modTime)
	//if err != nil {
	//	log.Println(err.Error())
	//	w.WriteHeader(http.StatusInternalServerError)
	//	return
	//}
	e.Size = buf.Len()
	// TODO: Add the licence support pieces, so the licence info can be included at upload time
	//if lName == "" {
	//	// No licence was specified by the client, so check if the database is already in the system and
	//	// already has one.  If so, we use that.
	//	if exists {
	//		headBranch, ok := branches[branchName]
	//		if !ok {
	//			w.WriteHeader(http.StatusBadRequest)
	//			return
	//		}
	//		headCommit, err := getCommit(dbName, headBranch.Commit)
	//		if err != nil {
	//			w.WriteHeader(http.StatusInternalServerError)
	//			return
	//
	//		}
	//		if headCommit.Tree.Entries[0].Licence != "" {
	//			// The previous commit for the database had a licence, so we use that for this commit
	//			// too
	//			e.Licence = headCommit.Tree.Entries[0].Licence
	//		}
	//	}
	//} else {
	//	// A licence was specified by the client, so use that
	//	e.Licence, err = getLicenceSha256(lName)
	//	if err != nil {
	//		w.WriteHeader(http.StatusInternalServerError)
	//		return
	//	}
	//}

	// Create a dbTree structure for the database entry
	var t com.DBTree
	t.Entries = append(t.Entries, e)
	t.ID = com.CreateDBTreeID(t.Entries)

	// Retrieve the display name and email address for the user
	dn, em, err := com.GetUserDetails(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Error retrieving user details")
		return
	}

	// If either the display name or email address is empty, tell the user we need them first
	if dn == "" || em == "" {
		errorPage(w, r, http.StatusInternalServerError,
			"You need to set your full name and email address in Preferences first")
		return
	}

	// Construct a commit structure pointing to the tree
	var c com.CommitEntry
	c.AuthorName = dn
	c.AuthorEmail = em
	c.Message = commitMsg
	c.Timestamp = time.Now()
	c.Tree = t

	// If the database already exists, use the head commit for the appropriate branch as the parent for our new
	// uploads' commit
	if exists {
		b, ok := branches[branchName]
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		c.Parent = b.Commit
	}

	// Update the branch with the commit for this new database upload
	c.ID = com.CreateCommitID(c)
	b := branches[branchName]
	b.Commit = c.ID
	branches[branchName] = b
	err = com.StoreDatabase(loggedInUser, dbFolder, dbName, branches, c, public, buf.Bytes(), sha, "", "", needDefaultBranchCreated, branchName, sourceURL)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Error storing database")
		return
	}

	// If the database already existed, update it's contributor count
	if exists {
		err = com.UpdateContributorsCount(loggedInUser, dbFolder, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, "Error when updating contributor count for the database")
			return
		}
	}

	// Invalidate the memcached entry for the database (only really useful if we're updating an existing database)
	err = com.InvalidateCacheEntry(loggedInUser, loggedInUser, "/", dbName, "") // Empty string indicates "for all versions"
	if err != nil {
		// Something went wrong when invalidating memcached entries for the database
		log.Printf("Error when invalidating memcache entries: %s\n", err.Error())
		return
	}

	// Invalidate any memcached entries for the previous highest version # of the database
	err = com.InvalidateCacheEntry(loggedInUser, loggedInUser, dbFolder, dbName, c.ID) // And empty string indicates "for all commits"
	if err != nil {
		// Something went wrong when invalidating memcached entries for any previous database
		log.Printf("Error when invalidating memcache entries: %s\n", err.Error())
		return
	}

	// Log the successful database upload
	log.Printf("%s: Username: %v, database '%v' uploaded as '%v/%v', bytes: %v\n", pageName, loggedInUser, dbName,
		sha[:com.MinioFolderChars], sha[com.MinioFolderChars:], bytesWritten)

	// Database upload succeeded.  Bounce the user to the page for their new database
	http.Redirect(w, r, fmt.Sprintf("/%s%s%s", loggedInUser, "/", dbName), http.StatusTemporaryRedirect)
}
