package main

import (
	"encoding/csv"
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

	"github.com/bradfitz/gomemcache/memcache"
	gsm "github.com/bradleypeabody/gorilla-sessions-memcache"
	"github.com/gwenn/gosqlite"
	gfm "github.com/justinclift/github_flavored_markdown"
	com "github.com/sqlitebrowser/dbhub.io/common"
	"golang.org/x/oauth2"
)

var (
	// Log file for incoming HTTPS requests
	reqLog *os.File

	// Our parsed HTML templates
	tmpl *template.Template

	// Session cookie storage
	store *gsm.MemcacheStore
)

// auth0CallbackHandler is called at the end of the Auth0 authentication process, whether successful or not.
// If the authentication process was successful:
//  * if the user already has an account on our system then this function creates a login session for them.
//  * if the user doesn't yet have an account on our system, they're bounced to the username selection page.
// If the authentication process wasn't successful, an error message is displayed.
func auth0CallbackHandler(w http.ResponseWriter, r *http.Request) {
	// Auth0 login part, mostly copied from https://github.com/auth0-samples/auth0-golang-web-app (MIT License)
	conf := &oauth2.Config{
		ClientID:     com.Conf.Auth0.ClientID,
		ClientSecret: com.Conf.Auth0.ClientSecret,
		RedirectURL:  "https://" + com.Conf.Web.ServerName + "/x/callback",
		Scopes:       []string{"openid", "profile"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://" + com.Conf.Auth0.Domain + "/authorize",
			TokenURL: "https://" + com.Conf.Auth0.Domain + "/oauth/token",
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
	userInfo, err := conn.Get("https://" + com.Conf.Auth0.Domain + "/userinfo")
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
		sess, err := store.Get(r, "user-reg")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		sess.Values["registrationinprogress"] = true
		sess.Values["auth0id"] = auth0ID
		sess.Values["email"] = email
		sess.Values["nickname"] = nickName
		err = sess.Save(r, w)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// Bounce to a new page, for the user to select their preferred username
		http.Redirect(w, r, "/selectusername", http.StatusTemporaryRedirect)
	}

	// Create a session cookie for the user
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	sess.Values["UserName"] = userName
	sess.Save(r, w)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Login completed, so bounce to the users' profile page
	http.Redirect(w, r, "/"+userName, http.StatusTemporaryRedirect)
}

func createBranchHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
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
	bd := r.PostFormValue("branchdesc") // Optional

	// If given, validate the branch description field
	var branchDesc string
	if bd != "" {
		err = com.Validate.Var(bd, "markdownsource")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, "Invalid characters in branch description")
			return
		}
		branchDesc = bd
	}

	// Check if the requested database exists
	dbFolder := "/"
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", dbOwner, dbFolder,
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

	// Count the number of commits in the new branch
	commitList, err := com.GetCommitList(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	c := commitList[commit]
	commitCount := 1
	for c.Parent != "" {
		commitCount++
		c, ok = commitList[c.Parent]
		if !ok {
			log.Printf("Error when counting commits in new branch '%s' of database '%s%s%s'\n", branchName,
				dbOwner, dbFolder, dbName)
			return
		}
	}

	// Create the branch
	newBranch := com.BranchEntry{
		Commit:      commit,
		CommitCount: commitCount,
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

// Receives incoming info for adding a comment to an existing discussion
func createCommentHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "You need to be logged in")
		return
	}

	// Extract and validate the form variables
	dbOwner, dbFolder, dbName, err := com.GetUFD(r, false)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Missing or incorrect data supplied")
		return
	}

	// Ensure a discussion ID was given
	a := r.PostFormValue("discid")
	if a == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Missing discussion id")
		return
	}
	discID, err := strconv.Atoi(a)
	if err != nil {
		log.Printf("Error converting string '%s' to integer in function '%s': %s\n", a,
			com.GetCurrentFunctionName(), err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Error when parsing discussion id value")
		return
	}

	// Check if the discussion should also be closed or reopened
	discClose := false
	c := r.PostFormValue("close")
	if c == "true" {
		discClose = true
	}

	// If comment text was provided, then validate it.  Note that if the flag for closing/reopening the discussion has
	// been set, then comment text isn't required.  In all other situations it is
	rawTxt := r.PostFormValue("comtext")
	if rawTxt == "" && discClose == false {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Comment can't be empty!")
		return
	}
	var comText string
	if discClose == false || (discClose == true && rawTxt != "") {
		// Unescape and validate the comment text
		t, err := url.QueryUnescape(rawTxt)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "Error when unescaping comment field value")
			return
		}
		err = com.Validate.Var(t, "markdownsource")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "Invalid characters in the new discussions' main text field")
			return
		}
		comText = t
	}

	// Check if the requested database exists
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err.Error())
		return
	}
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Database '%s%s%s' doesn't exist", dbOwner, dbFolder, dbName)
		return
	}

	// Add the comment to PostgreSQL
	err = com.StoreComment(dbOwner, dbFolder, dbName, loggedInUser, discID, comText, discClose)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err.Error())
		return
	}

	// Invalidate the memcache data for the database, so if the discussion counter for the database was changed it
	// gets picked up
	if discClose {
		err = com.InvalidateCacheEntry(loggedInUser, dbOwner, dbFolder, dbName, "") // Empty string indicates "for all versions"
		if err != nil {
			// Something went wrong when invalidating memcached entries for the database
			log.Printf("Error when invalidating memcache entries: %s\n", err.Error())
			return
		}
	}

	// Send a success message
	w.WriteHeader(http.StatusOK)
}

// Receives incoming info from the "Create a new discussion" page, adds the discussion to PostgreSQL,
// then bounces to the discussion page
func createDiscussHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Extract and validate the form variables
	dbOwner, dbFolder, dbName, err := com.GetUFD(r, false)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Missing or incorrect data supplied")
		return
	}

	// Validate the discussions' title
	tl := r.PostFormValue("title")
	err = com.Validate.Var(tl, "discussiontitle,max=120") // 120 seems a reasonable first guess.
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Invalid characters in the new discussions' title")
		return
	}
	discTitle := tl

	// Validate the discussions' text
	txt := r.PostFormValue("disctxt")
	if txt == "" {
		errorPage(w, r, http.StatusBadRequest, "Discussion body can't be empty!")
		return
	}
	err = com.Validate.Var(txt, "markdownsource")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Invalid characters in the new discussions' main text field")
		return
	}
	discText := txt

	// Check if the requested database exists
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", dbOwner, dbFolder,
			dbName))
		return
	}

	// Add the discussion detail to PostgreSQL
	err = com.StoreDiscussion(dbOwner, dbFolder, dbName, loggedInUser, discTitle, discText)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Invalidate the memcache data for the database, so the new discussion count gets picked up
	err = com.InvalidateCacheEntry(loggedInUser, dbOwner, dbFolder, dbName, "") // Empty string indicates "for all versions"
	if err != nil {
		// Something went wrong when invalidating memcached entries for the database
		log.Printf("Error when invalidating memcache entries: %s\n", err.Error())
		return
	}

	// Bounce to the discussions page
	http.Redirect(w, r, fmt.Sprintf("/discuss/%s%s%s", dbOwner, dbFolder, dbName), http.StatusTemporaryRedirect)
}

func createTagHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
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
		errorPage(w, r, http.StatusBadRequest, "Missing or incorrect tag name")
		return
	}

	// If given, validate the tag description field
	td := r.PostFormValue("tagdesc") // Optional
	var tagDesc string
	if td != "" {
		err = com.Validate.Var(td, "markdownsource")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, "Invalid characters in tag description")
			return
		}
		tagDesc = td
	}

	// Validate the tag type field
	tagType := r.PostFormValue("tagtype")
	if tagType != "tag" && tagType != "release" {
		errorPage(w, r, http.StatusBadRequest, "Unknown tag type")
		return
	}

	// Check if the requested database exists
	dbFolder := "/"
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", dbOwner, dbFolder,
			dbName))
		return
	}

	// Make sure the database owner matches the logged in user
	if loggedInUser != dbOwner {
		errorPage(w, r, http.StatusUnauthorized, "You can't change databases you don't own")
		return
	}

	// Create a new tag or release as appropriate
	if tagType == "release" {
		// * It's a release *

		// Read the releases list from the database
		rels, err := com.GetReleases(dbOwner, dbFolder, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// Ensure the release doesn't already exist
		if _, ok := rels[tagName]; ok {
			errorPage(w, r, http.StatusConflict, "A release of that name already exists!")
			return
		}

		// Retrieve the size of the database for this release
		var tmp com.SQLiteDBinfo
		err = com.DBDetails(&tmp, loggedInUser, dbOwner, dbFolder, dbName, commit)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		size := tmp.Info.DBEntry.Size

		// Create the release
		name, email, err := com.GetUserDetails(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusConflict, "An error occurred when retrieving user details")
		}
		newRel := com.ReleaseEntry{
			Commit:        commit,
			Date:          time.Now(),
			Description:   tagDesc,
			ReleaserEmail: email,
			ReleaserName:  name,
			Size:          size,
		}
		rels[tagName] = newRel

		// Store it in PostgreSQL
		err = com.StoreReleases(dbOwner, dbFolder, dbName, rels)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// Invalidate the memcache data for the database
		err = com.InvalidateCacheEntry(loggedInUser, dbOwner, dbFolder, dbName, "") // Empty string indicates "for all versions"
		if err != nil {
			// Something went wrong when invalidating memcached entries for the database
			log.Printf("Error when invalidating memcache entries: %s\n", err.Error())
			return
		}

		// Bounce to the releases page
		http.Redirect(w, r, fmt.Sprintf("/releases/%s%s%s", loggedInUser, dbFolder, dbName),
			http.StatusTemporaryRedirect)
		return
	}

	// * It's a tag *

	// Read the tags list from the database
	tags, err := com.GetTags(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Make sure the tag doesn't already exist
	if _, ok := tags[tagName]; ok {
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
		Description: tagDesc,
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
	sess, err := store.Get(r, "user-reg")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	validRegSession := false
	va := sess.Values["registrationinprogress"]
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
	au := sess.Values["auth0id"]
	if au != nil {
		auth0ID = au.(string)
	} else {
		errorPage(w, r, http.StatusBadRequest, "Invalid user creation id")
		return
	}
	em := sess.Values["email"]
	if em != nil {
		email = em.(string)
	} else {
		errorPage(w, r, http.StatusBadRequest, "Invalid user creation email")
		return
	}

	// Gather submitted form data (if any)
	err = r.ParseForm()
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

		// Note : gorilla/sessions uses MaxAge < 0 to mean "delete this session"
		sess.Options.MaxAge = -1
		err = sess.Save(r, w)
		if err != nil {
			// Try to display both errors
			errorPage(w, r, http.StatusInternalServerError, err.Error()+" Username failed validation")
			return
		}

		// Alert the user to the validation problem
		errorPage(w, r, http.StatusInternalServerError, "Username failed validation")
		return
	}

	// Ensure the username isn't a reserved one
	err = com.ReservedUsernamesCheck(userName)
	if err != nil {
		log.Println(err)

		// Note : gorilla/sessions uses MaxAge < 0 to mean "delete this session"
		sess.Options.MaxAge = -1
		err2 := sess.Save(r, w)
		if err != nil {
			// Try to display both errors
			errorPage(w, r, http.StatusInternalServerError, err2.Error()+" "+err.Error())
			return
		}

		// Alert the user to the ReservedUsernamesCheck() failure
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if the username is already in our system
	exists, err := com.CheckUserExists(userName)
	if err != nil {
		// Note : gorilla/sessions uses MaxAge < 0 to mean "delete this session"
		sess.Options.MaxAge = -1
		err = sess.Save(r, w)
		if err != nil {
			// Try to display both errors
			errorPage(w, r, http.StatusInternalServerError, err.Error()+" Username check failed")
			return
		}

		// Alert the username check failure
		errorPage(w, r, http.StatusInternalServerError, "Username check failed")
		return
	}
	if exists {
		// Note : gorilla/sessions uses MaxAge < 0 to mean "delete this session"
		sess.Options.MaxAge = -1
		err = sess.Save(r, w)
		if err != nil {
			// Try to display both errors
			errorPage(w, r, http.StatusInternalServerError, err.Error()+" That username is already taken")
			return
		}

		// Let the user know their desired username is not available
		errorPage(w, r, http.StatusConflict, "That username is already taken")
		return
	}

	// If present, validate the users' full name
	if displayName != "" {
		err = com.Validate.Var(displayName, "required,displayname,min=1,max=80")
		if err != nil {
			log.Printf("Display name value failed validation: %s\n", err)
			errorPage(w, r, http.StatusBadRequest, "Error when parsing full name value")
			return
		}
	}

	// If present, validate the email address
	if email != "" {
		err = com.Validate.Var(email, "email")
		if err != nil {
			// Check for the special case of username@server, which may fail standard email validation checks
			// eg username@localhost, won't validate as an email address, but should be accepted anyway
			serverName := strings.Split(com.Conf.Web.ServerName, ":")
			em := fmt.Sprintf("%s@%s", userName, serverName[0])
			if email != em {
				log.Printf("Email value failed validation: %s\n", err)
				errorPage(w, r, http.StatusBadRequest, "Error when parsing email value")
				return
			}
		}
	}

	// Add the user to the system
	// NOTE: We generate a random password here (for now).  We may remove the password field itself from the
	// database at some point, depending on whether we continue to support local database users
	err = com.AddUser(auth0ID, userName, com.RandomString(32), email, displayName)
	if err != nil {
		// Note : gorilla/sessions uses MaxAge < 0 to mean "delete this session"
		sess.Options.MaxAge = -1
		err = sess.Save(r, w)
		if err != nil {
			// Try to display both errors
			errorPage(w, r, http.StatusInternalServerError, err.Error()+" Something went wrong during user creation")
			return
		}

		// Alert the user to the problem
		errorPage(w, r, http.StatusInternalServerError, "Something went wrong during user creation")
		return
	}

	// Remove the temporary username selection session data
	sess.Options.MaxAge = -1
	err = sess.Save(r, w)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Create normal session cookie for the user
	sess, err = store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	sess.Values["UserName"] = userName
	sess.Save(r, w)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// User creation completed, so bounce to the user's profile page
	http.Redirect(w, r, "/"+userName, http.StatusTemporaryRedirect)
}

// This is called from the username selection page, to check if a name is available.
func checkNameHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve the potential username from the URL
	userName := r.FormValue("name")

	// Validate the user supplied username
	err := com.ValidateUser(userName)
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
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Extract the required form variables
	usr, dbFolder, dbName, err := com.GetUFD(r, false)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	dbOwner := strings.ToLower(usr)

	// Check if a branch name was requested
	branchName, err := com.GetFormBranch(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// If any of the required values were empty, indicate failure
	if branchName == "" || dbFolder == "" || dbName == "" || dbOwner == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the database exists in the system
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != err {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !exists {
		log.Printf("%s: Validation failed for database to delete: %s", pageName, err)
		w.WriteHeader(http.StatusNotFound)
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
	branch, ok := branches[branchName]
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

	// Make sure that deleting this branch wouldn't result in any isolated tags.  For example, when there is a tag on
	// a commit which is only in this branch, deleting the branch would leave the tag in place with no way to reach it

	// Get the tag list for the database
	tags, err := com.GetTags(dbOwner, dbFolder, dbName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// If the database has tags, walk the commit history for the branch checking if any of the tags are on commits in
	// this branch
	branchTags := make(map[string]string)
	if len(tags) > 0 {
		// Walk the commit history for the branch checking if any of the tags are on commits in this branch
		commitList, err := com.GetCommitList(dbOwner, dbFolder, dbName)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		c := commitList[branch.Commit]
		for tName, tEntry := range tags {
			// Scan through the tags, checking if any of them are for this commit
			if tEntry.Commit == c.ID {
				// It's a match, so add this tag to the list of tags on this branch
				branchTags[tName] = c.ID
			}
		}
		for c.Parent != "" {
			c, ok = commitList[c.Parent]
			if !ok {
				log.Printf("Error when checking for isolated tags while deleting branch '%s' of database '%s%s%s'\n",
					branchName, dbOwner, dbFolder, dbName)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			for tName, tEntry := range tags {
				// Scan through the tags, checking if any of them are for this commit
				if tEntry.Commit == c.ID {
					// It's a match, so add this tag to the list of tags on this branch
					branchTags[tName] = c.ID
				}
			}
		}

		// For any tags on commits in this branch, check if they're also on other branches
		if len(branchTags) > 0 {
			for bName, bEntry := range branches {
				if bName == branchName {
					// We're only checking "other branches"
					continue
				}

				if len(branchTags) == 0 {
					// If there are no tags left to check, we might as well stop further looping
					break
				}

				c := commitList[bEntry.Commit]
				for tName, tCommit := range branchTags {
					if c.ID == tCommit {
						// This commit matches a tag, so remove the tag from the list
						delete(branchTags, tName)
					}
				}
				for c.Parent != "" {
					c, ok = commitList[c.Parent]
					if !ok {
						log.Printf("Error when checking for isolated tags while deleting branch '%s' of database '%s%s%s'\n",
							branchName, dbOwner, dbFolder, dbName)
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					for tName, tCommit := range branchTags {
						if c.ID == tCommit {
							// This commit matches a tag, so remove the tag from the list
							delete(branchTags, tName)
						}
					}
				}
			}
		}

		// If there are any tags left over which aren't on other branches, abort this branch deletion and tell the user
		if len(branchTags) > 0 {
			var conflictedTags string
			for tName := range branchTags {
				if conflictedTags == "" {
					conflictedTags = tName
				} else {
					conflictedTags += ", " + tName
				}
			}

			w.WriteHeader(http.StatusConflict)
			if len(branchTags) > 1 {
				w.Write([]byte(fmt.Sprintf("You need to delete the tags '%s' before you can delete this branch",
					conflictedTags)))
			} else {
				w.Write([]byte(fmt.Sprintf("You need to delete the tag '%s' before you can delete this branch",
					conflictedTags)))
			}
			return
		}
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

// This function deletes a given comment from a discussion.
func deleteCommentHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "You need to be logged in")
		return
	}

	// Extract and validate the form variables
	dbOwner, dbFolder, dbName, err := com.GetUFD(r, false)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Missing or incorrect data supplied")
		return
	}

	// Ensure a discussion ID was given
	a := r.PostFormValue("discid")
	if a == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Missing discussion id")
		return
	}
	discID, err := strconv.Atoi(a)
	if err != nil {
		log.Printf("Error converting string '%s' to integer in function '%s': %s\n", a,
			com.GetCurrentFunctionName(), err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Error when parsing discussion id value")
		return
	}

	// Ensure a comment ID was given
	a = r.PostFormValue("comid")
	if a == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Missing comment id")
		return
	}
	comID, err := strconv.Atoi(a)
	if err != nil {
		log.Printf("Error converting string '%s' to integer in function '%s': %s\n", a,
			com.GetCurrentFunctionName(), err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Error when parsing comment id value")
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err.Error())
		return
	}
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Database '%s%s%s' doesn't exist", dbOwner, dbFolder, dbName)
		return
	}

	// Check if the logged in user is allowed to delete the requested comment
	deleteAllowed := false
	if dbOwner == loggedInUser {
		// The database owner can delete any discussion comment on their databases
		deleteAllowed = true
	} else {
		// Retrieve the details for the requested comment, so we can check if the logged in user is the comment creator
		rq, err := com.DiscussionComments(dbOwner, dbFolder, dbName, discID, comID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, err.Error())
			return
		}
		if rq[0].Commenter == loggedInUser {
			deleteAllowed = true
		}
	}

	// If the logged in user isn't allowed to delete the requested comment, then reject the request
	if !deleteAllowed {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Delete the comment from PostgreSQL
	err = com.DeleteComment(dbOwner, dbFolder, dbName, discID, comID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// This function deletes the latest commit from a given branch.
func deleteCommitHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Delete commit handler"

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Extract the required form variables
	usr, dbFolder, dbName, err := com.GetUFD(r, false)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	dbOwner := strings.ToLower(usr)

	// Validate the supplied commit ID
	commit, err := com.GetFormCommit(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Validate the supplied branch name
	branchName, err := com.GetFormBranch(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// If any of the required values were empty, indicate failure
	if branchName == "" || dbFolder == "" || dbName == "" || dbOwner == "" || commit == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the database exists in the system
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != err {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !exists {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		w.WriteHeader(http.StatusNotFound)
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
		w.Write([]byte("Unknown branch name"))
		return
	}

	// Check that the given commit matches the head commit of the branch
	if b.Commit != commit {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte("Only the most recent commit for a branch can be deleted"))
		return
	}

	// Ensure that deleting this commit won't result in any isolated/unreachable tags
	tagList, err := com.GetTags(dbOwner, dbFolder, dbName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	commitTags := map[string]struct{}{}
	for tName, tEntry := range tagList {
		// Scan through the database tag list, checking if any of the tags is for the commit we're deleting
		if tEntry.Commit == commit {
			commitTags[tName] = struct{}{}
		}
	}
	if len(commitTags) > 0 {
		// If the commit we're deleting has a tag on it, we need to check if the commit is on other branches too
		//   * If it is, we're ok to delete the commit as the commit/tag can still be reached from the other branch(es)
		//   * If it isn't, we need to abort the commit (and tell the user), as the tag would become unreachable

		// Get the commit list for the database
		commitList, err := com.GetCommitList(dbOwner, dbFolder, dbName)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		isolatedTags := true
		for bName, bEntry := range branches {
			if bName == branchName {
				// We only run this comparison from "other branches", not the branch we're deleting from
				continue
			}
			c := commitList[bEntry.Commit]
			if c.ID == commit {
				// The commit is also on another branch, so we're ok to delete the commit
				isolatedTags = false
				break
			}
			for c.Parent != "" {
				c, ok = commitList[c.Parent]
				if !ok {
					log.Printf("Error when checking for isolated tags while deleting commit '%s' in branch '%s' of database '%s%s%s'\n",
						commit, branchName, dbOwner, dbFolder, dbName)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				if c.ID == commit {
					// The commit is also on another branch, so we're ok to delete the commit
					isolatedTags = false
					break
				}
			}
		}

		// Deleting this commit would result in isolated tags, so abort the delete and tell the user of the problem
		if isolatedTags {
			var conflictedTags string
			for tName := range commitTags {
				if conflictedTags == "" {
					conflictedTags = tName
				} else {
					conflictedTags += ", " + tName
				}
			}

			w.WriteHeader(http.StatusConflict)
			if len(commitTags) > 1 {
				w.Write([]byte(fmt.Sprintf("You need to delete the tags '%s' before you can delete this commit",
					conflictedTags)))
			} else {
				w.Write([]byte(fmt.Sprintf("You need to delete the tag '%s' before you can delete this commit",
					conflictedTags)))
			}
			return
		}
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
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "You need to be logged in")
		return
	}

	// Extract the required form variables
	usr, dbFolder, dbName, err := com.GetUFD(r, false)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Validation failed for owner or database name")
		return
	}
	dbOwner := strings.ToLower(usr)

	// If any of the required values were empty, indicate failure
	if dbFolder == "" || dbName == "" || dbOwner == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Owner, folder, or database name values() missing")
		return
	}

	// Make sure the database exists in the system
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != err {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Internal server error")
		return
	}
	if !exists {
		log.Printf("%s: Missing database for '%s%s%s' when attempting deletion\n", pageName, dbOwner, dbFolder,
			dbName)
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "Internal server error")
		return
	}

	// Make sure the database is owned by the logged in user. eg prevent changes to other people's databases
	if dbOwner != loggedInUser {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "You don't have permission to delete that database")
		return
	}

	// Invalidate the memcache data for the database, so the new branch count gets picked up
	// Note - on one hand this is a race condition, as new cache data could get into memcache between this invalidation
	// call and the delete.  On the other hand, once it's deleted the invalidation function would itself fail due to
	// needing the database to be present so it can look up the commit list.  At least doing the invalidation here lets
	// us clear stale data (hopefully) for the vast majority of the time
	err = com.InvalidateCacheEntry(loggedInUser, dbOwner, dbFolder, dbName, "") // Empty string indicates "for all versions"
	if err != nil {
		// Something went wrong when invalidating memcached entries for the database
		log.Printf("Error when invalidating memcache entries: %s\n", err.Error())
		return
	}

	// Delete the database
	err = com.DeleteDatabase(dbOwner, dbFolder, dbName)
	if err != err {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "Internal server error")
		return
	}

	// Update succeeded
	w.WriteHeader(http.StatusOK)
}

// This function deletes a release.
func deleteReleaseHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Delete Release handler"

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Extract the required form variables
	usr, dbFolder, dbName, err := com.GetUFD(r, false)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	dbOwner := strings.ToLower(usr)

	// Ensure a release name was supplied
	relName, err := com.GetFormRelease(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// If any of the required values were empty, indicate failure
	if relName == "" || dbFolder == "" || dbName == "" || dbOwner == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the database exists in the system
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != err {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !exists {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Make sure the database is owned by the logged in user. eg prevent changes to other people's databases
	if dbOwner != loggedInUser {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Load the existing releases for the database
	releases, err := com.GetReleases(loggedInUser, dbFolder, dbName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Make sure the given tag exists
	if _, ok := releases[relName]; !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Delete the release
	delete(releases, relName)
	err = com.StoreReleases(dbOwner, dbFolder, dbName, releases)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Invalidate the memcache data for the database, so the new release count gets picked up
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
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Extract the required form variables
	usr, dbFolder, dbName, err := com.GetUFD(r, false)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	dbOwner := strings.ToLower(usr)

	// Ensure a tag name was supplied
	tagName, err := com.GetFormTag(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// If any of the required values were empty, indicate failure
	if tagName == "" || dbFolder == "" || dbName == "" || dbOwner == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the database exists in the system
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != err {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !exists {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		w.WriteHeader(http.StatusNotFound)
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
	if _, ok := tags[tagName]; !ok {
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
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
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
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.cert.pem"`, loggedInUser))
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
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
	}

	// Verify the given database exists and is ok to be downloaded (and get the Minio bucket + id while at it)
	bucket, id, err := com.MinioLocation(dbOwner, "/", dbName, commitID, loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Get a handle from Minio for the database object
	sdb, err := com.OpenMinioObject(bucket, id)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Automatically close the SQLite database when this function finishes
	defer func() {
		sdb.Close()
	}()

	// Read the table data from the database object
	resultSet, err := com.ReadSQLiteDBCSV(sdb, dbTable)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Error reading table data from the database")
		return
	}

	// Convert resultSet into CSV and send to the user
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.csv"`, dbTable))
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
	dbFolder := "/"

	// Retrieve session data (if any)
	var loggedInUser string
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
	}

	// Verify the given database exists and is ok to be downloaded (and get the Minio bucket + id while at it)
	bucket, id, err := com.MinioLocation(dbOwner, dbFolder, dbName, commitID, loggedInUser)
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

	// Get the file details
	stat, err := userDB.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Was a user agent part of the request?
	var userAgent string
	if ua, ok := r.Header["User-Agent"]; ok {
		userAgent = ua[0]
	}

	// Make a record of the download
	err = com.LogDownload(dbOwner, dbFolder, dbName, loggedInUser, r.RemoteAddr, "webui", userAgent,
		time.Now(), bucket+id)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Send the database to the user
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, dbName))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size))
	w.Header().Set("Content-Type", "application/x-sqlite3")
	bytesWritten, err := io.Copy(w, userDB)
	if err != nil {
		log.Printf("%s: Error returning DB file: %v\n", pageName, err)
		fmt.Fprintf(w, "%s: Error returning DB file: %v\n", pageName, err)
		return
	}

	// If downloaded by someone other than the owner, increment the download count for the database
	if loggedInUser != dbOwner {
		err = com.IncrementDownloadCount(dbOwner, dbFolder, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Log the number of bytes written
	log.Printf("%s: '%s/%s' downloaded. %d bytes", pageName, dbOwner, dbName, bytesWritten)
}

// Forks a database for the logged in user.
func forkDBHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve username, database name, and commit ID
	dbOwner, dbName, commitID, err := com.GetODC(2, r) // 2 = Ignore "/x/forkdb/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	dbFolder := "/"

	// Make sure a database commit ID was given
	if commitID == "" {
		errorPage(w, r, http.StatusBadRequest, "No database commit ID given")
		return
	}

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		// No logged in username, so nothing to update
		errorPage(w, r, http.StatusBadRequest, "To fork a database, you need to be logged in")
		return
	}

	// Check the user has access to the specific version of the source database requested
	allowed, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !allowed {
		errorPage(w, r, http.StatusNotFound, "You don't have access to the requested database")
		return
	}

	// Make sure the source and destination owners are different
	if loggedInUser == dbOwner {
		errorPage(w, r, http.StatusBadRequest, "Forking your own database in-place doesn't make sense")
		return
	}

	// Make sure the user doesn't have a database of the same name already
	// Note the use of "loggedInUser" for the 2nd parameter in this call, unlike using "dbOwner" in the call above
	exists, err := com.CheckDBExists(loggedInUser, loggedInUser, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if exists {
		// Database of the same name already exists
		errorPage(w, r, http.StatusNotFound, "You already have a database of this name")
		return
	}

	// Add the forked database info to PostgreSQL
	_, err = com.ForkDatabase(dbOwner, dbFolder, dbName, loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Invalidate the old memcached entry for the database
	err = com.InvalidateCacheEntry(loggedInUser, dbOwner, dbFolder, dbName, "") // Empty string indicates "for all versions"
	if err != nil {
		// Something went wrong when invalidating memcached entries for the database
		log.Printf("Error when invalidating memcache entries: %s\n", err.Error())
		return
	}

	// Log the database fork
	log.Printf("Database '%s%s%s' forked to user '%s'\n", dbOwner, dbFolder, dbName, loggedInUser)

	// Bounce to the page of the forked database
	http.Redirect(w, r, fmt.Sprintf("/%s%s%s", loggedInUser, dbFolder, dbName), http.StatusTemporaryRedirect)
}

// Generates a client certificate for the user and gives it to the browser.
func generateCertHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		// No logged in user, so error out
		errorPage(w, r, http.StatusBadRequest, "Not logged in")
		return
	}

	// Generate a new certificate
	newCert, err := com.GenerateClientCert(loggedInUser)
	if err != nil {
		log.Printf("Error generating client certificate for user '%s': %s!\n", loggedInUser, err)
		errorPage(w, r, http.StatusInternalServerError, "Error generating client certificate")
		return
	}

	// Store the new certificate in the database
	err = com.SetClientCert(newCert, loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Storing the new client certificate failed")
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

// Removes the logged in users session information.
func logoutHandler(w http.ResponseWriter, r *http.Request) {
	// Remove session info
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	// Note : gorilla/sessions uses MaxAge < 0 to mean "delete this session"
	sess.Options.MaxAge = -1
	err = sess.Save(r, w)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
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
		sess, err := store.Get(r, "dbhub-user")
		if err != nil {
			if err == memcache.ErrCacheMiss {
				// If the memcache session token is stale (eg memcached has been restarted), delete the session
				// TODO: This should probably look for the session token in persistent storage (eg PG) instead, so
				// TODO  restarts of memcached don't nuke everyone's saved sessions

				// Delete the session
				// Note : gorilla/sessions uses MaxAge < 0 to mean "delete this session"
				sess.Options.MaxAge = -1
				err = sess.Save(r, w)
				if err != nil {
					errorPage(w, r, http.StatusInternalServerError, err.Error())
					return
				}

				// Reload the page
				http.Redirect(w, r, fmt.Sprintf("%s", r.URL), http.StatusTemporaryRedirect)
			} else {
				errorPage(w, r, http.StatusBadRequest, err.Error())
				return
			}
		}
		u := sess.Values["UserName"]
		if u != nil {
			loggedInUser = u.(string)
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

	// Set the temp dir environment variable
	err = os.Setenv("TMPDIR", com.Conf.DiskCache.Directory)
	if err != nil {
		log.Fatalf("Setting temp directory environment variable failed: '%s'\n", err.Error())
	}

	// Open the request log for writing
	reqLog, err = os.OpenFile(com.Conf.Web.RequestLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY|os.O_SYNC, 0750)
	if err != nil {
		log.Fatalf("Error when opening request log: %s\n", err)
	}
	defer reqLog.Close()
	log.Printf("Request log opened: %s\n", com.Conf.Web.RequestLog)

	// Parse our template files
	tmpl = template.Must(template.New("templates").Delims("[[", "]]").ParseGlob(
		filepath.Join("webui", "templates", "*.html")))

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

	// Add the default user to the system
	// Note - we don't check for an error here on purpose.  If we were to fail on an error, then subsequent runs after
	// the first would barf with PG errors about trying to insert multiple "default" users violating unique
	// constraints.  It would be solvable by creating a special purpose PL/pgSQL function just for this one use case...
	// or we could just ignore failures here. ;)
	com.AddDefaultUser()

	// Add the default licences to PostgreSQL
	err = com.AddDefaultLicences()
	if err != nil {
		log.Fatalf(err.Error())
	}

	// Connect to the Memcached server
	err = com.ConnectCache()
	if err != nil {
		log.Fatalf(err.Error())
	}

	// Setup session storage
	store = gsm.NewMemcacheStore(com.MemcacheHandle(), "dbhub_", []byte(com.Conf.Web.SessionStorePassword))

	// Start the view count flushing routine in the background
	go com.FlushViewCount()

	// Our pages
	http.HandleFunc("/", logReq(mainHandler))
	http.HandleFunc("/about", logReq(aboutPage))
	http.HandleFunc("/branches/", logReq(branchesPage))
	http.HandleFunc("/commits/", logReq(commitsPage))
	http.HandleFunc("/confirmdelete/", logReq(confirmDeletePage))
	http.HandleFunc("/contributors/", logReq(contributorsPage))
	http.HandleFunc("/createbranch/", logReq(createBranchPage))
	http.HandleFunc("/creatediscuss/", logReq(createDiscussionPage))
	http.HandleFunc("/createtag/", logReq(createTagPage))
	http.HandleFunc("/discuss/", logReq(discussPage))
	http.HandleFunc("/forks/", logReq(forksPage))
	http.HandleFunc("/logout", logReq(logoutHandler))
	http.HandleFunc("/pref", logReq(prefHandler))
	http.HandleFunc("/register", logReq(createUserHandler))
	http.HandleFunc("/releases/", logReq(releasesPage))
	http.HandleFunc("/selectusername", logReq(selectUserNamePage))
	http.HandleFunc("/settings/", logReq(settingsPage))
	http.HandleFunc("/stars/", logReq(starsPage))
	http.HandleFunc("/tags/", logReq(tagsPage))
	http.HandleFunc("/upload/", logReq(uploadPage))
	http.HandleFunc("/x/callback", logReq(auth0CallbackHandler))
	http.HandleFunc("/x/checkname", logReq(checkNameHandler))
	http.HandleFunc("/x/createbranch", logReq(createBranchHandler))
	http.HandleFunc("/x/createcomment/", logReq(createCommentHandler))
	http.HandleFunc("/x/creatediscuss", logReq(createDiscussHandler))
	http.HandleFunc("/x/createtag", logReq(createTagHandler))
	http.HandleFunc("/x/deletebranch/", logReq(deleteBranchHandler))
	http.HandleFunc("/x/deletecomment/", logReq(deleteCommentHandler))
	http.HandleFunc("/x/deletecommit/", logReq(deleteCommitHandler))
	http.HandleFunc("/x/deletedatabase/", logReq(deleteDatabaseHandler))
	http.HandleFunc("/x/deleterelease/", logReq(deleteReleaseHandler))
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
	http.HandleFunc("/x/updatecomment/", logReq(updateCommentHandler))
	http.HandleFunc("/x/updatediscuss/", logReq(updateDiscussHandler))
	http.HandleFunc("/x/updaterelease/", logReq(updateReleaseHandler))
	http.HandleFunc("/x/updatetag/", logReq(updateTagHandler))
	http.HandleFunc("/x/uploaddata/", logReq(uploadDataHandler))

	// CSS
	http.HandleFunc("/css/bootstrap-3.3.7.min.css", logReq(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("webui", "css", "bootstrap-3.3.7.min.css"))
	}))
	http.HandleFunc("/css/bootstrap.min.css.map", logReq(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("webui", "css", "bootstrap-3.3.7.min.css.map"))
	}))
	http.HandleFunc("/css/font-awesome-4.7.0.min.css", logReq(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("webui", "css", "font-awesome-4.7.0.min.css"))
	}))
	http.HandleFunc("/css/local.css", logReq(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("webui", "css", "local.css"))
	}))

	// Fonts
	http.HandleFunc("/css/FontAwesome.otf", logReq(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("webui", "fonts", "FontAwesome-4.7.0.otf"))
	}))
	http.HandleFunc("/css/fontawesome-webfont.eot", logReq(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("webui", "fonts", "fontawesome-webfont-4.7.0.eot"))
	}))
	http.HandleFunc("/css/fontawesome-webfont.svg", logReq(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("webui", "fonts", "fontawesome-webfont-4.7.0.svg"))
	}))
	http.HandleFunc("/css/fontawesome-webfont.ttf", logReq(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("webui", "fonts", "fontawesome-webfont-4.7.0.ttf"))
	}))
	http.HandleFunc("/css/fontawesome-webfont.woff", logReq(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("webui", "fonts", "fontawesome-webfont-4.7.0.woff"))
	}))
	http.HandleFunc("/css/fontawesome-webfont.woff2", logReq(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("webui", "fonts", "fontawesome-webfont-4.7.0.woff2"))
	}))

	// Javascript
	http.HandleFunc("/js/angular-1.6.5.min.js", logReq(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("webui", "js", "angular-1.6.5.min.js"))
	}))
	http.HandleFunc("/js/angular.min.js.map", logReq(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("webui", "js", "angular-1.6.5.min.js.map"))
	}))
	http.HandleFunc("/js/angular-sanitize-1.6.5.min.js", logReq(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("webui", "js", "angular-sanitize-1.6.5.min.js"))
	}))
	http.HandleFunc("/js/angular-sanitize.min.js.map", logReq(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("webui", "js", "angular-sanitize-1.6.5.min.js.map"))
	}))
	http.HandleFunc("/js/local.js", logReq(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("webui", "js", "local.js"))
	}))
	http.HandleFunc("/js/lock-10.20.min.js", logReq(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("webui", "js", "lock-10.20.min.js"))
	}))
	http.HandleFunc("/js/lock.min.js.map", logReq(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("webui", "js", "lock-10.20.min.js.map"))
	}))
	http.HandleFunc("/js/ui-bootstrap-tpls-2.2.0.min.js", logReq(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("webui", "js", "ui-bootstrap-tpls-2.2.0.min.js"))
	}))

	// Other static files
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

	// Start webUI server
	log.Printf("DBHub server starting on https://%s\n", com.Conf.Web.ServerName)
	err = http.ListenAndServeTLS(com.Conf.Web.BindAddress, com.Conf.Web.Certificate, com.Conf.Web.CertificateKey, nil)

	// Shut down nicely
	com.DisconnectPostgreSQL()

	if err != nil {
		log.Fatal(err)
	}
}

func mainHandler(w http.ResponseWriter, r *http.Request) {
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

	// TODO: Add support for folders and sub-folders in request paths
	dbFolder := "/"

	// A specific database was requested
	databasePage(w, r, userName, dbFolder, dbName)
}

// Returns HTML rendered content from a given markdown string, for the settings page README preview tab.
func markdownPreview(w http.ResponseWriter, r *http.Request) {
	// Extract and unescape the markdown text form value
	a := r.PostFormValue("mkdown")
	mkDown, err := url.QueryUnescape(a)
	if err != nil {
		fmt.Fprint(w, "Something went wrong when unescaping provided value")
		return
	}

	// Validate the markdown source provided, just to be safe
	var renderedText []byte
	if mkDown != "" {
		err := com.Validate.Var(mkDown, "markdownsource")
		if err != nil {
			fmt.Fprint(w, "Invalid characters in Markdown")
			return
		}
		renderedText = gfm.Markdown([]byte(mkDown))
	}

	// Send the rendered version back to the caller
	fmt.Fprint(w, string(renderedText))
}

// This handles incoming requests for the preferences page by logged in users.
func prefHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Preferences handler"

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
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
	err = com.Validate.Var(maxRows, "required,numeric,min=1,max=500")
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
		log.Printf("%s: Display name '%s' failed validation: %s\n", pageName, displayName, err)
		errorPage(w, r, http.StatusBadRequest, "Error when parsing full name value")
		return
	}
	err = com.Validate.Var(email, "required,email")
	if err != nil {
		// Check for the special case of username@server, which may fail standard email validation checks
		// eg username@localhost, won't validate as an email address, but should be accepted anyway
		serverName := strings.Split(com.Conf.Web.ServerName, ":")
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
	// TODO  needed so looking up an old email finds the correct username.  For example when looking through historical
	// TODO  commit data

	// Update the preference data in the database
	err = com.SetUserPreferences(loggedInUser, maxRowsNum, displayName, email)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Error when updating preferences")
		return
	}

	// Bounce to the user home page
	http.Redirect(w, r, "/"+loggedInUser, http.StatusTemporaryRedirect)
}

// Handler for the Database Settings page
func saveSettingsHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Extract the username, folder, and (current) database name form variables
	usr, dbFolder, dbName, err := com.GetUFD(r, false)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	dbOwner := strings.ToLower(usr)

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
	defTable := r.PostFormValue("defaulttable") // TODO: Update the default table to be "per branch"
	licences := r.PostFormValue("licences")

	// Validate the licence names
	branchLics := make(map[string]string)
	err = json.Unmarshal([]byte(licences), &branchLics)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	for bName, lName := range branchLics {
		err = com.ValidateLicence(lName)
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, fmt.Sprintf(
				"Validation failed on licence name for branch '%s'", bName))
			return
		}
	}

	// Validate the source URL
	sourceURL, err := com.GetFormSourceURL(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Validation failed for source URL value")
		return
	}

	// Grab and validate the supplied default branch name
	defBranch, err := com.GetFormBranch(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Validation failed for branch name")
		return
	}

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

	// Validate characters and length of the one line description
	if oneLineDesc != "" {
		err = com.Validate.Var(oneLineDesc, "markdownsource,max=120")
		if err != nil {
			log.Printf("One line description '%s' failed validation", oneLineDesc)
			errorPage(w, r, http.StatusBadRequest, "One line description failed validation")
			return
		}
	}

	// Validate the full description
	if fullDesc != "" {
		err = com.Validate.Var(fullDesc, "markdownsource,max=8192") // 8192 seems reasonable.  Maybe too long?
		if err != nil {
			log.Printf("Full description '%s' failed validation", fullDesc)
			errorPage(w, r, http.StatusBadRequest, "Full description failed validation")
			return
		}
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
	sdb, err := com.OpenMinioObject(bkt, id)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Automatically close the SQLite database when this function finishes
	defer func() {
		sdb.Close()
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

	// Grab the complete commit list for the database
	commitList, err := com.GetCommitList(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Loop through the branches of the database, processing the user submitted licence choice for each
	branchHeads, err := com.GetBranches(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	branchesUpdated := false
	newBranchHeads := make(map[string]com.BranchEntry)
	for bName, bEntry := range branchHeads {
		// Get the previous licence entry for the branch
		c, ok := commitList[bEntry.Commit]
		if !ok {
			errorPage(w, r, http.StatusInternalServerError, fmt.Sprintf(
				"Error when retrieving commit ID '%s', branch '%s' for database '%s%s%s'", bEntry.Commit,
				bName, dbOwner, dbFolder, dbName))
			return
		}
		dbEntry := c.Tree.Entries[0]
		licSHA := dbEntry.LicenceSHA
		var oldLic string
		if licSHA != "" {
			oldLic, _, err = com.GetLicenceInfoFromSha256(loggedInUser, licSHA)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}
		}

		// Get the new licence entry for the branch
		newLic, ok := branchLics[bName]
		if !ok {
			errorPage(w, r, http.StatusInternalServerError, fmt.Sprintf(
				"Missing licence entry for branch '%s'", bName))
			return
		}

		// If the new licence given for a branch is different from the old one, generate a new commit, add it to the
		// commit list, and update the branch with it
		if oldLic != newLic {
			// Retrieve the SHA256 of the new licence
			newLicSHA, err := com.GetLicenceSha256FromName(loggedInUser, newLic)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}

			// Create a new dbTree entry for the database file
			var e com.DBTreeEntry
			e.EntryType = com.DATABASE
			e.Last_Modified = dbEntry.Last_Modified
			e.LicenceSHA = newLicSHA
			e.Name = dbEntry.Name
			e.Sha256 = dbEntry.Sha256
			e.Size = dbEntry.Size

			// Create a new dbTree structure for the new database entry
			var t com.DBTree
			t.Entries = append(t.Entries, e)
			t.ID = com.CreateDBTreeID(t.Entries)

			// Create a new commit for the new tree
			newCom := com.CommitEntry{
				CommitterName:  c.AuthorName,
				CommitterEmail: c.AuthorEmail,
				Message:        fmt.Sprintf("Licence changed from '%s' to '%s'.", oldLic, newLic),
				Parent:         bEntry.Commit,
				Timestamp:      time.Now(),
				Tree:           t,
			}
			newCom.AuthorName, newCom.AuthorEmail, err = com.GetUserDetails(loggedInUser)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}

			// Calculate the new commit ID, which incorporates the updated tree ID (and thus the new licence sha256)
			newCom.ID = com.CreateCommitID(newCom)

			// Add the new commit to the commit list
			commitList[newCom.ID] = newCom

			// Add the commit to the new branch heads list, and set a flag indicating it needs to be stored to the
			// database after the licence processing finishes
			newBranchEntry := com.BranchEntry{
				Commit:      newCom.ID,
				CommitCount: bEntry.CommitCount + 1,
				Description: bEntry.Description,
			}
			newBranchHeads[bName] = newBranchEntry
			branchesUpdated = true
		} else {
			// Copy the old branch entry to the new list
			newBranchHeads[bName] = branchHeads[bName]
		}
	}

	// If the branches were updated, store the new commit list and branch heads
	if branchesUpdated {
		err = com.StoreCommits(dbOwner, dbFolder, dbName, commitList)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		err = com.StoreBranches(dbOwner, dbFolder, dbName, newBranchHeads)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
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
	err = com.SaveDBSettings(dbOwner, dbFolder, dbName, oneLineDesc, fullDesc, defTable, public, sourceURL, defBranch)
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
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Extract the required form variables
	usr, dbFolder, dbName, err := com.GetUFD(r, false)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	dbOwner := strings.ToLower(usr)

	// Check if a branch name was requested
	branchName, err := com.GetFormBranch(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// If any of the required values were empty, indicate failure
	if branchName == "" || dbFolder == "" || dbName == "" || dbOwner == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the database exists in the system
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != err {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !exists {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		w.WriteHeader(http.StatusNotFound)
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
	if _, ok := branches[branchName]; !ok {
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
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		// No logged in username, so nothing to update
		// TODO: We should probably use a http status code instead of using -1
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

	// Retrieve user, database, table, and commit ID
	// TODO: Add folder support
	dbOwner, dbName, requestedTable, commitID, err := com.GetODTC(2, r) // 1 = Ignore "/x/table/" at the start of the URL
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	dbFolder := "/"

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
			w.WriteHeader(http.StatusBadRequest)
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
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	// If a sort direction was provided, validate it
	if sortDir != "" {
		if sortDir != "ASC" && sortDir != "DESC" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	// Retrieve session data (if any)
	var loggedInUser string
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
	}

	// Check if the user has access to the requested database
	bucket, id, err := com.MinioLocation(dbOwner, dbFolder, dbName, commitID, loggedInUser)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Sanity check
	if id == "" {
		// The requested database wasn't found
		log.Printf("%s: Requested database not found. Owner: '%s%s%s'", pageName, dbOwner, dbFolder, dbName)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Determine the number of rows to display
	var maxRows int
	if loggedInUser != "" {
		// Retrieve the user preference data
		maxRows = com.PrefUserMaxRows(loggedInUser)
	} else {
		// Not logged in, so use the default number of display rows
		maxRows = com.DefaultNumDisplayRows
	}

	// If the data is available from memcached, use that instead of reading from the SQLite database itself
	dataCacheKey := com.TableRowsCacheKey(fmt.Sprintf("tablejson/%s/%s/%d", sortCol, sortDir, rowOffset),
		loggedInUser, dbOwner, dbFolder, dbName, commitID, requestedTable, maxRows)

	// If a cached version of the page data exists, use it
	var dataRows com.SQLiteRecordSet
	ok, err := com.GetCachedData(dataCacheKey, &dataRows)
	if err != nil {
		log.Printf("%s: Error retrieving table data from cache: %v\n", pageName, err)
	}
	if !ok {
		// * Data wasn't in cache, so we gather it from the SQLite database *

		// Open the Minio database
		sdb, err := com.OpenMinioObject(bucket, id)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Automatically close the SQLite database when this function finishes
		defer func() {
			sdb.Close()
		}()

		// Retrieve the list of tables in the database
		tables, err := sdb.Tables("")
		if err != nil {
			// An error occurred, so get the extended error code
			if cerr, ok := err.(sqlite.ConnError); ok {
				// Check if the error was due to the table being locked
				extCode := cerr.ExtendedCode()
				if extCode == 5 { // Magic number which (in this case) means "database is locked"
					// Wait 3 seconds then try again
					time.Sleep(3 * time.Second)
					tables, err = sdb.Tables("")
					if err != nil {
						log.Printf("Error retrieving table names: %s", err)
						return
					}
				} else {
					log.Printf("Error retrieving table names: %s", err)
					return
				}
			} else {
				log.Printf("Error retrieving table names: %s", err)
				return
			}
		}
		if len(tables) == 0 {
			// No table names were returned, so abort
			log.Printf("The database '%s' doesn't seem to have any tables. Aborting.", dbName)
			return
		}
		vw, err := sdb.Views("")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		tables = append(tables, vw...)

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
				w.WriteHeader(http.StatusBadRequest)
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
				w.WriteHeader(http.StatusInternalServerError)
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
			log.Printf("Error occurred when reading table data for '%s%s%s', commit '%s': %s\n", dbOwner,
				dbFolder, dbName, commitID, err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Count the total number of rows in the requested table
		dataRows.TotalRows, err = com.GetSQLiteRowCount(sdb, requestedTable)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Cache the data in memcache
		err = com.CacheData(dataCacheKey, dataRows, com.Conf.Memcache.DefaultCacheTime)
		if err != nil {
			log.Printf("%s: Error when caching table data for '%s%s%s': %v\n", pageName, dbOwner, dbFolder,
				dbName, err)
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
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Extract the required form variables
	usr, dbFolder, dbName, err := com.GetUFD(r, false)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	dbOwner := strings.ToLower(usr)

	// Unescape, then validate the new branch name
	a := r.PostFormValue("newbranch")
	nb, err := url.QueryUnescape(a)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	err = com.Validate.Var(nb, "branchortagname,min=1,max=32") // 32 seems a reasonable first guess.
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	newName := nb

	// Validate new branch description
	var newDesc string
	b := r.PostFormValue("newdesc") // Optional
	if b != "" {
		nd, err := url.QueryUnescape(b)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		err = com.Validate.Var(nd, "markdownsource")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		newDesc = nd
	}

	// Make sure a branch name was provided
	branchName, err := com.GetFormBranch(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// If any of the required values were empty, indicate failure
	if branchName == "" || dbFolder == "" || dbName == "" || dbOwner == "" || newName == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the database exists in the system
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != err {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !exists {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		w.WriteHeader(http.StatusNotFound)
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

	// If the branch being changed is the default branch, and it's being renamed, we need to update the default branch
	// entry in the database with the new branch name
	defBranch, err := com.GetDefaultBranchName(dbOwner, dbFolder, dbName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if defBranch == branchName {
		// Update the default branch name for the database
		err = com.StoreDefaultBranchName(dbOwner, dbFolder, dbName, newName)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	// Update the branch info
	delete(branches, branchName)
	branches[newName] = com.BranchEntry{
		Commit:      oldInfo.Commit,
		CommitCount: oldInfo.CommitCount,
		Description: newDesc,
	}
	err = com.StoreBranches(dbOwner, dbFolder, dbName, branches)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Invalidate the memcache data for the database, so the new branch name gets picked up
	err = com.InvalidateCacheEntry(loggedInUser, dbOwner, dbFolder, dbName, "") // Empty string indicates "for all versions"
	if err != nil {
		// Something went wrong when invalidating memcached entries for the database
		log.Printf("Error when invalidating memcache entries: %s\n", err.Error())
		return
	}

	// Update succeeded
	w.WriteHeader(http.StatusOK)
}

// This function processes comment text updates.
func updateCommentHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Update Comment handler"

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Extract the required form variables
	usr, dbFolder, dbName, err := com.GetUFD(r, false)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	dbOwner := strings.ToLower(usr)

	// Ensure a discussion ID was given
	a := r.PostFormValue("discid")
	if a == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Missing discussion id")
		return
	}
	discID, err := strconv.Atoi(a)
	if err != nil {
		log.Printf("Error converting string '%s' to integer in function '%s': %s\n", a,
			com.GetCurrentFunctionName(), err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Error when parsing discussion id value")
		return
	}

	// Ensure a comment ID was given
	b := r.PostFormValue("comid")
	if b == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Missing comment id")
		return
	}
	comID, err := strconv.Atoi(b)
	if err != nil {
		log.Printf("Error converting string '%s' to integer in function '%s': %s\n", a,
			com.GetCurrentFunctionName(), err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Error when parsing comment id value")
		return
	}

	// Unescape, then validate the new comment text
	var newTxt string
	c := r.PostFormValue("comtext")
	nt, err := url.QueryUnescape(c)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if nt != "" {
		err = com.Validate.Var(nt, "markdownsource")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		newTxt = nt
	}

	// If any of the required values were empty, indicate failure
	if dbOwner == "" || dbFolder == "" || dbName == "" || discID == 0 || comID == 0 || newTxt == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Required values missing!")
		return
	}

	// Make sure the database exists in the system
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != err {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !exists {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Update the discussion text
	err = com.UpdateComment(dbOwner, dbFolder, dbName, loggedInUser, discID, comID, newTxt)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err.Error())
		return
	}

	// Update succeeded
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(gfm.Markdown([]byte(newTxt))))
}

// This function processes discussion title and body text updates.
func updateDiscussHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Extract the required form variables
	usr, dbFolder, dbName, err := com.GetUFD(r, false)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Bad request")
		return
	}
	dbOwner := strings.ToLower(usr)

	// Ensure a discussion ID was given
	a := r.PostFormValue("discid")
	if a == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Missing discussion id")
		return
	}
	discID, err := strconv.Atoi(a)
	if err != nil {
		log.Printf("Error converting string '%s' to integer in function '%s': %s\n", a,
			com.GetCurrentFunctionName(), err)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Error when parsing discussion id value")
		return
	}

	// Unescape, then validate the new discussion text
	var newTxt string
	b := r.PostFormValue("disctext")
	nt, err := url.QueryUnescape(b)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err.Error())
		return
	}
	if nt != "" {
		err = com.Validate.Var(nt, "markdownsource")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "Discussion text failed validation")
			return
		}
		newTxt = nt
	}

	// Unescape, then validate the new discussion title
	var newTitle string
	c := r.PostFormValue("disctitle")
	ti, err := url.QueryUnescape(c)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err.Error())
		return
	}
	if ti != "" {
		err = com.Validate.Var(ti, "discussiontitle,max=120")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "Discussion title failed validation")
			return
		}
		newTitle = ti
	}

	// If any of the required values were empty, indicate failure
	if dbOwner == "" || dbFolder == "" || dbName == "" || discID == 0 || newTitle == "" || newTxt == "" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "Required values missing!")
		return
	}

	// Make sure the database exists in the system
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != err {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err.Error())
		return
	}
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "Database not found")
		return
	}

	// Update the discussion text
	err = com.UpdateDiscussion(dbOwner, dbFolder, dbName, loggedInUser, discID, newTitle, newTxt)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err.Error())
		return
	}

	// Update succeeded
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(gfm.Markdown([]byte(newTxt))))
}

// This function processes release rename and description updates.
func updateReleaseHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Update Release handler"

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Extract the required form variables
	usr, dbFolder, dbName, err := com.GetUFD(r, false)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	dbOwner := strings.ToLower(usr)

	// Validate new release name
	a := r.PostFormValue("newrel")
	nr, err := url.QueryUnescape(a)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	err = com.Validate.Var(nr, "branchortagname,min=1,max=32") // 32 seems a reasonable first guess.
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	newName := nr

	// Validate new release description
	var newDesc string
	b := r.PostFormValue("newmsg") // Optional
	if b != "" {
		nd, err := url.QueryUnescape(b)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		err = com.Validate.Var(nd, "markdownsource")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		newDesc = nd
	}

	// Ensure a release name was supplied
	relName, err := com.GetFormRelease(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// If any of the required values were empty, indicate failure
	if relName == "" || dbFolder == "" || dbName == "" || dbOwner == "" || newName == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the database exists in the system
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != err {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !exists {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Make sure the database is owned by the logged in user. eg prevent changes to other people's databases
	if dbOwner != loggedInUser {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Load the existing releases for the database
	releases, err := com.GetReleases(loggedInUser, dbFolder, dbName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Make sure the given release exists
	oldInfo, ok := releases[relName]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Update the release info
	delete(releases, relName)
	releases[newName] = com.ReleaseEntry{
		Commit:        oldInfo.Commit,
		Date:          oldInfo.Date,
		Description:   newDesc,
		ReleaserEmail: oldInfo.ReleaserEmail,
		ReleaserName:  oldInfo.ReleaserName,
		Size:          oldInfo.Size,
	}

	err = com.StoreReleases(dbOwner, dbFolder, dbName, releases)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Update succeeded
	w.WriteHeader(http.StatusOK)
}

// This function processes tag rename and description updates.
func updateTagHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Update Tag handler"

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Extract the required form variables
	usr, dbFolder, dbName, err := com.GetUFD(r, false)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	dbOwner := strings.ToLower(usr)

	// Validate new tag name
	a := r.PostFormValue("newtag")
	nt, err := url.QueryUnescape(a)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	err = com.Validate.Var(nt, "branchortagname,min=1,max=32") // 32 seems a reasonable first guess.
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	newName := nt

	// If given, validate new tag description
	var newMsg string
	b := r.PostFormValue("newmsg") // Optional
	if b != "" {
		nm, err := url.QueryUnescape(b)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		err = com.Validate.Var(nm, "markdownsource")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		newMsg = nm
	}

	// Ensure a tag name was supplied
	tagName, err := com.GetFormTag(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// If any of the required values were empty, indicate failure
	if tagName == "" || dbFolder == "" || dbName == "" || dbOwner == "" || newName == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Make sure the database exists in the system
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != err {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !exists {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		w.WriteHeader(http.StatusNotFound)
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
		Description: newMsg,
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

// This function processes new database data submitted through the upload form.
func uploadDataHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Upload DB handler"

	// Set the maximum accepted database size for uploading
	r.Body = http.MaxBytesReader(w, r.Body, com.MaxDatabaseSize*1024*1024)

	// Retrieve session data (if any)
	var loggedInUser string
	validSession := false
	sess, err := store.Get(r, "dbhub-user")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	u := sess.Values["UserName"]
	if u != nil {
		loggedInUser = u.(string)
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Check whether the uploaded database is too large
	if r.ContentLength > (com.MaxDatabaseSize * 1024 * 1024) {
		errorPage(w, r, http.StatusBadRequest,
			fmt.Sprintf("Database is too large. Maximum database upload size is %d MB, yours is %d MB",
				com.MaxDatabaseSize, r.ContentLength/1024/1024))
		log.Println(fmt.Sprintf("'%s' attempted to upload an oversided database %d MB in size.  Limit is %d MB\n",
			loggedInUser, r.ContentLength/1024/1024, com.MaxDatabaseSize))
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

	// Validate the licence value
	licenceName, err := com.GetFormLicence(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Validation failed for licence value")
		return
	}

	// Validate the source URL
	sourceURL, err := com.GetFormSourceURL(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Validation failed for source URL value")
		return
	}

	// Validate the commit message
	var commitMsg string
	cm := r.PostFormValue("commitmsg")
	if cm != "" {
		err = com.Validate.Var(cm, "markdownsource,max=1024") // 1024 seems like a reasonable first guess
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, "Validation failed for the commit message")
			return
		}
		commitMsg = cm
	}

	// Validate the (optional) branch name
	branchName, err := com.GetFormBranch(r)
	if err != nil {
		log.Printf("%s: Error when validating branch name '%s': %v\n", pageName, branchName, err)
		errorPage(w, r, http.StatusBadRequest, "Branch name value failed validation")
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

	// Sanity check the uploaded database, and if ok then add it to the system
	numBytes, _, err := com.AddDatabase(r, loggedInUser, loggedInUser, dbFolder, dbName, branchName, public,
		licenceName, commitMsg, sourceURL, tempFile, "webui")
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Log the successful database upload
	log.Printf("%s: Username: '%s', database '%s%s%s' uploaded', bytes: %v\n", pageName, loggedInUser,
		loggedInUser, dbFolder, dbName, numBytes)

	// Database upload succeeded.  Bounce the user to the page for their new database
	http.Redirect(w, r, fmt.Sprintf("/%s%s%s", loggedInUser, "/", dbName), http.StatusTemporaryRedirect)
}
