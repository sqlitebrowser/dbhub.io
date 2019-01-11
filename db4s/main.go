package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	com "github.com/sqlitebrowser/dbhub.io/common"
)

// User details struct used by many functions
type userDetails struct {
	Name       string
	Email      string
	Password   string
	PVerify    string
	DateJoined time.Time
	ClientCert []byte
	PHash      []byte
}

type dbDetails struct {
	Folder       string
	Name         string
	Version      int
	Size         int
	DateCreated  time.Time
	LastModified time.Time
	Public       bool
	MinioID      string
}

var (
	// Our self signed Certificate Authority chain
	ourCAPool *x509.CertPool

	// Address of our server, formatted for display
	server string
)

// For sorting a UserInfo list by Last Modified date descending
type UserInfoSlice []com.UserInfo

func (u UserInfoSlice) Len() int {
	return len(u)
}

func (u UserInfoSlice) Less(i, j int) bool {
	return u[i].LastModified.After(u[j].LastModified) // Swap to Before() for an ascending list
}

func (u UserInfoSlice) Swap(i, j int) {
	u[i], u[j] = u[j], u[i]
}

// Returns the list of branches for a database
func branchListHandler(w http.ResponseWriter, r *http.Request) {
	// Extract the account name and associated server from the validated client certificate
	userAcc, _, err := extractUserAndServer(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Extract and validate the form variables
	dbOwner, dbFolder, dbName, err := com.GetUFD(r, true)
	if err != nil {
		http.Error(w, "Missing or incorrect data supplied", http.StatusBadRequest)
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckDBExists(userAcc, dbOwner, dbFolder, dbName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !exists {
		fmt.Fprint(w, "{}")
		return
	}

	// Retrieve the branch list for the database
	brList, err := com.GetBranches(dbOwner, dbFolder, dbName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Retrieve the default branch for the database
	defBranch, err := com.GetDefaultBranchName(dbOwner, dbFolder, dbName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the list as JSON
	info := struct {
		Def     string                     `json:"default_branch"`
		Entries map[string]com.BranchEntry `json:"branches"`
	}{
		defBranch,
		brList,
	}
	jsonList, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		errMsg := fmt.Sprintf("Error when JSON marshalling the branch list: %v\n", err)
		log.Print(errMsg)
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}
	fmt.Fprintf(w, string(jsonList))
	return
}

func extractUserAndServer(w http.ResponseWriter, r *http.Request) (userAcc string, certServer string, err error) {
	// Extract the account name and associated server from the validated client certificate
	cn := r.TLS.PeerCertificates[0].Subject.CommonName
	if cn == "" {
		// Common name is empty
		err = errors.New("Common name is blank in client certificate")
		return
	}
	s := strings.Split(cn, "@")
	if len(s) < 2 {
		err = errors.New("Missing information in client certificate")
		return
	}
	userAcc = s[0]
	certServer = s[1]
	if userAcc == "" || certServer == "" {
		// Missing details in common name field
		err = errors.New("Missing information in client certificate")
		return
	}

	// Verify the running server matches the one in the certificate
	runningServer := com.Conf.DB4S.Server
	if certServer != runningServer {
		err = fmt.Errorf("Server name in certificate '%s' doesn't match running server '%s'\n", certServer,
			runningServer)
		return
	}

	// Everything is ok, so return
	return
}

func generateDefaultList(pageName string, userAcc string) (defaultList []byte, err error) {
	pageName += ":generateDefaultList()"

	// Retrieve the list of most recently modified (available) databases
	unsorted, err := com.DB4SDefaultList(userAcc)
	if err != nil {
		// Return an empty set
		return []byte{'{', '}'}, err
	}

	// Sort the list by last_modified order, from most recent to oldest
	userList := make(UserInfoSlice, 0, len(unsorted))
	for _, j := range unsorted {
		userList = append(userList, j)
	}
	sort.Sort(userList)

	// Ready the data for JSON Marshalling
	type linkRow struct {
		Type         string `json:"type"`
		Name         string `json:"name"`
		URL          string `json:"url"`
		LastModified string `json:"last_modified"`
	}
	var linkRows []linkRow
	var rowCount int
	for _, j := range userList {
		newLink := linkRow{
			Type:         "folder",
			Name:         j.Username,
			URL:          server + "/" + j.Username,
			LastModified: j.LastModified.Format(time.RFC822)}
		linkRows = append(linkRows, newLink)
		rowCount++
	}

	if rowCount > 0 {
		// Use json.MarshalIndent() for nicer looking output
		defaultList, err = json.MarshalIndent(linkRows, "", "  ")
		if err != nil {
			log.Printf("%s: Error when JSON marshalling the default list: %v\n", pageName, err)
			return nil, errors.Wrap(err, fmt.Sprintf("%s: Error when JSON marshalling the default list",
				pageName))
		}
	} else {
		// Return an empty set indicator, instead of "null"
		defaultList = []byte{'{', '}'}
	}
	return defaultList, nil
}

func getHandler(w http.ResponseWriter, r *http.Request, userAcc string) {
	pageName := "GET request handler"

	// Split the request URL into path components
	pathStrings := strings.Split(r.URL.Path, "/")

	// numPieces will be 2 if the request was for the root directory (https://server/), or if
	// the request included only a single path component (https://server/someuser/)
	numPieces := len(pathStrings)
	if numPieces == 2 {
		// Check if the request was for the root directory
		if pathStrings[1] == "" {
			// Yep, root directory request
			defaultList, err := generateDefaultList(pageName, userAcc)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			fmt.Fprintf(w, "%s", defaultList)
			return
		}

		// The request was for a user directory, so return that list
		dbList, err := userDatabaseList(pageName, userAcc, pathStrings[1])
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "%s", dbList)
		return
	}

	// This catches the case where a "/" is on the end of the URL
	// TODO: Refactor this and the above identical code.  Doing it this way is non-optimal
	if pathStrings[2] == "" {
		// The request was for a user directory, so return that list
		dbList, err := userDatabaseList(pageName, userAcc, pathStrings[1])
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "%s", dbList)
		return
	}

	// Use easily understandable variable names
	dbOwner := pathStrings[1]
	dbName := pathStrings[2]

	// TODO: Add support for folders
	dbFolder := "/"

	// Validate the dbOwner and dbName inputs
	err := com.ValidateUser(dbOwner)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = com.ValidateDB(dbName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckDBExists(userAcc, dbOwner, dbFolder, dbName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, fmt.Sprintf("Database '%s%s%s' doesn't exist", dbOwner, dbFolder, dbName),
			http.StatusNotFound)
		return
	}

	// Extract the requested database commit id from the form data
	commit, err := com.GetFormCommit(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get the branch heads list for the database
	branchList, err := com.GetBranches(dbOwner, dbFolder, dbName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If a branch name was provided use it, else use the default branch for the database
	var branchName string
	bn, err := com.GetFormBranch(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if bn != "" {
		_, ok := branchList[bn]
		if !ok {
			http.Error(w, "Unknown branch name", http.StatusNotFound)
			return
		}
		branchName = bn
	} else {
		// No branch name was given, so retrieve the default for the database
		branchName, err = com.GetDefaultBranchName(dbOwner, dbFolder, dbName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// If no commit ID was given, we use the latest commit in the branch
	if commit == "" {
		branch, ok := branchList[branchName]
		if !ok {
			http.Error(w, "Unknown branch name", http.StatusNotFound)
			return
		}
		commit = branch.Commit
	}

	// Check that the commit is known to the database
	commitList, err := com.GetCommitList(dbOwner, dbFolder, dbName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, ok := commitList[commit]
	if !ok {
		http.Error(w, "Commit not found", http.StatusNotFound)
		return
	}

	// A specific database was requested, so send it to the user
	err = retrieveDatabase(w, r, pageName, userAcc, dbOwner, dbFolder, dbName, commit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Returns the text or html document for a specific licence
func getLicenceHandler(w http.ResponseWriter, r *http.Request) {
	// Extract the account name and associated server from the validated client certificate
	userAcc, _, err := extractUserAndServer(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Make sure a licence name was provided
	l := r.FormValue("licence")
	if l == "" {
		http.Error(w, "No licence name supplied", http.StatusBadRequest)
		return
	}

	// Validate the licence name
	err = com.ValidateLicence(l)
	if err != nil {
		log.Printf("Validation failed for licence name: '%s': %s", l, err)
		http.Error(w, "Validation of licence name failed", http.StatusBadRequest)
		return
	}
	licenceName := l

	// Retrieve the licence from our database
	lic, format, err := com.GetLicence(userAcc, licenceName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Determine file extension and mime type
	var fileExt, mimeType string
	switch format {
	case "text":
		fileExt = "txt"
		mimeType = "text/plain"
	case "html":
		fileExt = "html"
		mimeType = "text/html"
	default:
		// Unknown licence file format
		http.Error(w, fmt.Sprintf("Unknown licence format '%s'", format), http.StatusInternalServerError)
		return
	}

	// Send the licence file to the user
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s",
		url.QueryEscape(licenceName+"."+fileExt)))
	w.Header().Set("Content-Type", mimeType)
	bytesWritten, err := fmt.Fprint(w, lic)
	if err != nil {
		log.Printf("Error returning licence file: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log the transfer
	log.Printf("Licence '%s' downloaded by user '%v', %d bytes\n", licenceName, userAcc, bytesWritten)
	return
}

// Returns the list of licences known to the server
func licenceListHandler(w http.ResponseWriter, r *http.Request) {
	type licEntry struct {
		FullName string `json:"full_name"`
		SHA256   string `json:"sha256"`
		Type     string `json:"type"`
		URL      string `json:"url"`
	}

	// Extract the account name and associated server from the validated client certificate
	userAcc, _, err := extractUserAndServer(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Create the list of licences, ready for JSON formatting
	rawLicenceList, err := com.GetLicences(userAcc)
	licList := make(map[string]licEntry)
	for name, details := range rawLicenceList {
		if name == "Not specified" {
			// No need to include an entry for "Not specified"
			continue
		}
		licList[name] = licEntry{
			FullName: details.FullName,
			SHA256:   details.Sha256,
			Type:     "licence",
			URL:      details.URL,
		}
	}

	// Return the list as JSON
	jsonLicList, err := json.MarshalIndent(licList, "", "  ")
	if err != nil {
		errMsg := fmt.Sprintf("Error when JSON marshalling the licence list: %v\n", err)
		log.Print(errMsg)
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}
	fmt.Fprintf(w, string(jsonLicList))
	return
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

	// Connect to the Memcached server
	err = com.ConnectCache()
	if err != nil {
		log.Fatalf(err.Error())
	}

	// Add the default user to the system
	// Note - we don't check for an error here on purpose.  If we were to fail on an error, then subsequent runs after
	// the first would barf with PG errors about trying to insert multiple "default" users violating unique
	// constraints.  It would be solvable by creating a special purpose PL/pgSQL function just for this one use case...
	// or we could just ignore failures here. ;)
	// TODO: We might be able to use an "ON CONFLICT" PG clause instead, to (eg) "DO NOTHING"
	com.AddDefaultUser()

	// Add the default licences to PostgreSQL
	err = com.AddDefaultLicences()
	if err != nil {
		log.Fatalf(err.Error())
	}

	// Load our self signed CA chain
	ourCAPool = x509.NewCertPool()
	certFile, err := ioutil.ReadFile(com.Conf.DB4S.CAChain)
	if err != nil {
		fmt.Printf("Error opening Certificate Authority chain file: %v\n", err)
		return
	}
	ok := ourCAPool.AppendCertsFromPEM(certFile)
	if !ok {
		fmt.Println("Error appending certificate file")
		return
	}

	// URL handler
	mux := http.NewServeMux()
	mux.HandleFunc("/", rootHandler)
	mux.HandleFunc("/branch/list", branchListHandler)
	mux.HandleFunc("/licence/get", getLicenceHandler)
	mux.HandleFunc("/licence/list", licenceListHandler)

	// Load our self signed CA Cert chain, request client certificates, and set TLS1.2 as minimum
	newTLSConfig := &tls.Config{
		ClientAuth:               tls.RequireAndVerifyClientCert,
		ClientCAs:                ourCAPool,
		MinVersion:               tls.VersionTLS12,
		PreferServerCipherSuites: true,
		RootCAs:                  ourCAPool,
	}
	newServer := &http.Server{
		Addr:         ":" + fmt.Sprint(com.Conf.DB4S.Port),
		Handler:      mux,
		TLSConfig:    newTLSConfig,
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0),
	}

	// Generate the formatted server string
	if com.Conf.DB4S.Port == 443 {
		server = fmt.Sprintf("https://%s", com.Conf.DB4S.Server)
	} else {
		server = fmt.Sprintf("https://%s:%d", com.Conf.DB4S.Server, com.Conf.DB4S.Port)
	}

	// Start server
	log.Printf("Starting DB4S end point on %s\n", server)
	log.Fatal(newServer.ListenAndServeTLS(com.Conf.DB4S.Certificate, com.Conf.DB4S.CertificateKey))
}

// postHandler receives uploaded files from DB4S. To simulate a DB4S upload, the following curl command can be used:
//
//   $ curl -kE ~/my.cert.pem -D headers.out -F file=@someupload.sqlite -F "branch=master" -F "commitmsg=stuff" \
//       -F "sourceurl=https://example.org" -F "lastmodified=2017-01-02T03:04:05Z"  -F "licence=CC0"  -F "public=true" \
//       https://db4s-beta.dbhub.io:5550/someuser
//
// Subsequent uploads to the same database name will need to include an additional "commit" field, with the value of
// the commit ID last known to DB4S.  An example curl command demonstrating this:
//
//   $ curl -kE ~/my.cert.pem -D headers.out -F file=@someupload.sqlite -F "branch=master" -F "commitmsg=stuff" \
//       -F "sourceurl=https://example.org" -F "lastmodified=2017-01-02T03:04:05Z"  -F "licence=CC0"  -F "public=true" \
//       -F "commit=51d494f2c5eb6734ddaa204eccb9597b426091c79c951924ac83c72038f22b55" \
//       https://db4s-beta.dbhub.io:5550/someuser
//
func postHandler(w http.ResponseWriter, r *http.Request, userAcc string) {
	pageName := "POST request handler"

	// Set the maximum accepted database size for uploading
	r.Body = http.MaxBytesReader(w, r.Body, com.MaxDatabaseSize*1024*1024)

	// Split the request URL into path components
	pathStrings := strings.Split(r.URL.Path, "/")

	// Ensure a target username was given
	targetUser := pathStrings[1]
	if targetUser == "" {
		http.Error(w, "Missing target user", http.StatusBadRequest)
		return
	}

	// Validate the target user
	err := com.ValidateUser(targetUser)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check whether the uploaded database is too large
	// TODO: Have a list of users (from the config.toml file) which don't have this check applied
	if r.ContentLength > (com.MaxDatabaseSize * 1024 * 1024) {
		http.Error(w,
			fmt.Sprintf("Database is too large. Maximum database upload size is %d MB, yours is %d MB",
				com.MaxDatabaseSize, r.ContentLength/1024/1024), http.StatusBadRequest)
		log.Println(fmt.Sprintf("'%s' attempted to upload an oversized database %d MB in size.  Limit is %d MB\n",
			userAcc, r.ContentLength/1024/1024, com.MaxDatabaseSize))
		return
	}

	// Grab the uploaded file and form variables
	tempFile, handler, err := r.FormFile("file")
	if err != nil {
		log.Printf("%s: Uploading file failed: %v\n", pageName, err)
		http.Error(w, fmt.Sprintf("Something went wrong when grabbing the file data: '%s'", err.Error()), http.StatusBadRequest)
		return
	}
	defer tempFile.Close()

	// Validate the database name
	targetDB := handler.Filename
	err = com.ValidateDB(targetDB)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// TODO: Add support for folders
	targetFolder := "/"

	// TODO: Add support for DB4S passing the last_modified date of the database

	// If a branch name was provided then validate it
	var branchName string
	bn := r.FormValue("branch")
	if bn != "" {
		err := com.Validate.Var(bn, "branchortagname,min=1,max=32") // 32 seems a reasonable first guess.
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid branch name value: '%v'", bn), http.StatusBadRequest)
			return
		}
		branchName = bn
	}

	// If DB4S sent a "force" field, validate it
	force := false
	f := r.FormValue("force")
	if f != "" {
		force, err = strconv.ParseBool(f)
		if err != nil {
			// Force value couldn't be parsed
			http.Error(w, fmt.Sprintf("Error when converting force '%s' value to boolean: %v\n", f, err),
				http.StatusBadRequest)
			return
		}
	}

	// If a licence name was provided then use it, else default to "Not specified"
	licenceName := "Not specified"
	ln := r.FormValue("licence")
	if ln != "" {
		// Validate the licence name
		err = com.ValidateLicence(ln)
		if err != nil {
			http.Error(w, fmt.Sprintf("Validation failed for licence name value: '%s': %s", ln, err),
				http.StatusBadRequest)
			return
		}

		// Make sure the licence is one that's known to us
		licenceList, err := com.GetLicences(userAcc)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, ok := licenceList[ln]
		if !ok {
			http.Error(w, fmt.Sprintf("Unknown licence: '%s'", ln), http.StatusBadRequest)
			return
		}
		licenceName = ln
	}

	// If a source URL was provided then use it
	var sourceURL string
	su := r.FormValue("sourceurl")
	if su != "" {
		// Validate the source URL
		err = com.Validate.Var(su, "url,min=5,max=255") // 255 seems like a reasonable first guess
		if err != nil {
			http.Error(w, "Validation failed for source URL value", http.StatusBadRequest)
			return
		}
		sourceURL = su
	}

	// If a database commit id was provided, then extract it
	commit, err := com.GetFormCommit(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If a commit message was provided then use it
	var commitMsg string
	cm := r.FormValue("commitmsg")
	if cm != "" {
		// Validate the commit message
		err = com.Validate.Var(cm, "markdownsource,max=1024") // 1024 seems like a reasonable first guess
		if err != nil {
			http.Error(w, "Validation failed for the commit message", http.StatusBadRequest)
			return
		}
		commitMsg = cm
	}

	// If a public/private setting was provided then use it
	var public bool
	pub := r.FormValue("public")
	if pub != "" {
		public, err = strconv.ParseBool(pub)
		if err != nil {
			// Public/private value couldn't be parsed, so default to private
			http.Error(w, fmt.Sprintf("Error when converting public value to boolean: %v\n", err),
				http.StatusBadRequest)
			return
		}
	}

	// If the last modified timestamp was provided, then validate it
	var lastMod time.Time
	lm := r.FormValue("lastmodified")
	if lm != "" {
		lastMod, err = time.Parse(time.RFC3339, lm)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid lastmodified value: '%v'", lm), http.StatusBadRequest)
			return
		}
	} else {
		// No last modified time provided, so just use the current server time
		lastMod = time.Now()
	}

	// Verify the user is uploading to a location they have write access for
	if strings.ToLower(targetUser) != strings.ToLower(userAcc) {
		log.Printf("%s: Attempt by '%s' to write to unauthorised location: %v\n", pageName, userAcc,
			r.URL.Path)
		http.Error(w, fmt.Sprintf("Error code 401: You don't have write permission for '%s'",
			r.URL.Path), http.StatusForbidden)
		return
	}

	// Check if the database exists already
	exists, err := com.CheckDBExists(userAcc, targetUser, targetFolder, targetDB)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !exists && branchName == "" {
		// If the database doesn't already exist, and no branch name was provided, then default to master
		branchName = "master"
	}

	// If the database already exists, we need to do collision detection, check for forking, and check for force pushes
	createBranch := false
	if !exists {
		createBranch = true
	} else {
		if commit == "" {
			http.Error(w, "No commit ID was provided.  You probably need to upgrade DB4S before trying this "+
				"again.", http.StatusUpgradeRequired)
			return
		}

		// Retrieve the branch list for the database
		branchList, err := com.GetBranches(targetUser, targetFolder, targetDB)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// If a branch name was given, check if it's a branch we know about
		knownBranch := false
		var brDetails com.BranchEntry
		if branchName != "" {
			brDetails, knownBranch = branchList[branchName]
		}

		// * Fork detection piece *
		if !knownBranch {
			// An unknown branch name was given, so this is a fork.
			createBranch = true

			// Make sure the given commit ID is in the commit history.  If it's not, we error out
			found := false
			for branch := range branchList {
				// Loop through the branches, checking if the commit ID is in any of them
				a, err := com.IsCommitInBranchHistory(targetUser, targetFolder, targetDB, branch, commit)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				if a {
					found = true
				}
			}
			if !found {
				// The commit wasn't found in the history of any branch
				http.Error(w, fmt.Sprintf("Unknown commit ID: '%s'", commit), http.StatusNotFound)
				return
			}
		} else {
			// * Collision detection piece *

			// Check if the provided commit ID is the latest head commit for the branch.  If it is, then things
			// are in order and this new upload should be a new commit on the branch.
			if brDetails.Commit != commit {
				// * The provided commit doesn't match the HEAD commit for the specified branch *

				// Check if the provided commit is present in the history for the branch.  If it is, then the
				// database being pushed is out of date compared to the HEAD commit.  We'll need to abort
				// (with a suitable warning message), unless the force flag was passed + set to true
				found, err := com.IsCommitInBranchHistory(targetUser, targetFolder, targetDB, branchName, commit)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				if !found {
					// The provided commit ID isn't in the commit history for the branch, so there's something
					// wrong.  We need to error out and let DB4S know
					http.Error(w, fmt.Sprintf("Commit ID '%s' isn't in the commit history of branch '%s'",
						commit, branchName), http.StatusNotFound)
					return
				}

				// * To get here, this push is a collision *

				// The commit ID provided was found in the branch history but isn't the latest (HEAD) commit for
				// the branch.  Unless the "force" flag was provided by DB4S (and set to true), we error out to
				// notify DB4S of the collision.  It probably just means the database has been updated on the
				// server (eg through the webUI) but the user is still using an older version and needs to update

				if !force {
					http.Error(w, fmt.Sprintf("Outdated commit '%s' provided.  You're probably using an "+
						"old version of the database", commit), http.StatusConflict)
					return
				}

				// * To get here, DB4S has told us to rewrite the commit history for a branch, given us the
				//   required info, and provided the "force" flag set to true.  So, we drop through here and get
				//   it done *

			} else {
				// The provided commit ID matched the branch head, so things are in order.  We drop through and
				// create a new commit
			}
		}
	}

	// Sanity check the uploaded database, and if ok then add it to the system
	numBytes, commitID, err := com.AddDatabase(r, userAcc, targetUser, targetFolder, targetDB, createBranch,
		branchName, commit, public, licenceName, commitMsg, sourceURL, tempFile, "db4s", lastMod)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log the successful database upload
	log.Printf("Database uploaded: '%s%s%s', bytes: %v\n", userAcc, targetFolder, targetDB, numBytes)

	// Construct message data for returning to DB4S
	u := filepath.Join(server, targetUser, targetFolder, targetDB)
	u += fmt.Sprintf(`?branch=%s&commit=%s`, branchName, commitID)
	m := map[string]string{"commit_id": commitID, "url": u}

	// Convert to JSON
	var msg bytes.Buffer
	enc := json.NewEncoder(&msg)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send return message back to DB4S
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, msg.String())
}

// Returns a file requested by DB4S.  An example curl command to simulate the DB4S request is:
//
//   $ curl -OL -kE ~/my.cert.pem -D headers.out -G https://db4s-beta.dbhub.io:5550/someuser/somedb.sqlite
//
func retrieveDatabase(w http.ResponseWriter, r *http.Request, pageName string, userAcc string, dbOwner string,
	dbFolder string, dbName string, commit string) (err error) {
	pageName += ":retrieveDatabase()"

	// Retrieve the Minio details and last modified date for the requested database
	bucket, id, lastMod, err := com.MinioLocation(dbOwner, dbFolder, dbName, commit, userAcc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get a handle from Minio for the database object
	userDB, err := com.MinioHandle(bucket, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Close the object handle when this function finishes
	defer func() {
		err := com.MinioHandleClose(userDB)
		if err != nil {
			log.Printf("%s: Error closing object handle: %v\n", pageName, err)
		}
	}()

	// Get the file details
	stat, err := userDB.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Was a user agent part of the request?
	var userAgent string
	ua, ok := r.Header["User-Agent"]
	if ok {
		userAgent = ua[0]
	}

	// Make a record of the download
	err = com.LogDownload(dbOwner, dbFolder, dbName, userAcc, r.RemoteAddr, "db4s", userAgent, time.Now(),
		bucket+id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send the database to the user
	// Note: modification-date parameter format copied from RFC 2183 (the closest match I could find easily)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; modification-date="%s";`,
		url.QueryEscape(dbName), lastMod.Format(time.RFC3339)))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size))
	w.Header().Set("Content-Type", "application/x-sqlite3")
	bytesWritten, err := io.Copy(w, userDB)
	if err != nil {
		log.Printf("%s: Error returning DB file: %v\n", pageName, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If downloaded by someone other than the owner, increment the download count for the database
	if strings.ToLower(userAcc) != strings.ToLower(dbOwner) {
		err = com.IncrementDownloadCount(dbOwner, dbFolder, dbName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Log the transfer
	log.Printf("'%s%s%s' downloaded by user '%v', %v bytes", dbOwner, dbFolder, dbName, userAcc, bytesWritten)
	return nil
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Main page"

	// Extract the account name and associated server from the validated client certificate
	userAcc, _, err := extractUserAndServer(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// The handler to use depends upon the request type
	reqType := r.Method
	switch reqType {
	case "GET":
		getHandler(w, r, userAcc)
	case "POST":
		postHandler(w, r, userAcc)
	default:
		log.Printf("%s: Unknown request method received from '%v\n", pageName, userAcc)
		http.Error(w, fmt.Sprintf("Unknown request type: %v\n", reqType), http.StatusBadRequest)
	}
	return
}

// Returns the list of databases available to the user.  To simulate DB4S, the following curl command can be used:
//
//   $ curl -kE ~/my.cert.pem -D headers.out -G https://db4s-beta.dbhub.io:5550/someuser
//
func userDatabaseList(pageName string, userAcc string, user string) (dbList []byte, err error) {
	pageName += ":userDatabaseList()"

	// Structure to hold the results, to apply JSON marshalling to
	type linkRow struct {
		CommitID     string `json:"commit_id"`
		LastModified string `json:"last_modified"`
		Licence      string `json:"licence"`
		Name         string `json:"name"`
		Public       bool   `json:"public"`
		SHA256       string `json:"sha256"`
		Size         int    `json:"size"`
		Type         string `json:"type"`
		URL          string `json:"url"`
	}

	// Retrieve the list of databases for the requested username.  Only include those accessible to the logged
	// in user (userAcc) though
	var pubSetting com.AccessType
	if strings.ToLower(userAcc) != strings.ToLower(user) {
		// The user is requesting someone else's list, so only return public databases
		pubSetting = com.DB_PUBLIC
	} else {
		// The logged in user is requesting their own database list, so give them both public and private
		pubSetting = com.DB_BOTH
	}

	// Retrieve the database list
	pubDBs, err := com.UserDBs(user, pubSetting)
	if err != nil {
		return nil, err
	}

	// Ready the results for JSON Marshalling
	var rowList []linkRow
	var rowCount int
	var tempRow linkRow
	for _, j := range pubDBs {
		tempRow.Type = "database"
		tempRow.CommitID = j.CommitID
		tempRow.Licence = j.Licence
		if j.Folder == "/" {
			tempRow.Name = j.Database
			tempRow.URL = fmt.Sprintf("%s/%s/%s?commit=%v", server, user,
				url.PathEscape(j.Database), j.CommitID)
		} else {
			tempRow.Name = fmt.Sprintf("%s/%s", strings.TrimPrefix(j.Folder, "/"),
				j.Database)
			tempRow.URL = fmt.Sprintf("%s/%s%s/%s?commit=%v", server, user, j.Folder,
				url.PathEscape(j.Database), j.CommitID)
		}
		if j.DefaultBranch != "" {
			tempRow.URL += fmt.Sprintf("&branch=%s", j.DefaultBranch)
		}
		tempRow.Size = j.Size
		tempRow.SHA256 = j.SHA256
		tempRow.LastModified = j.LastModified.Format(time.RFC822)
		tempRow.Public = j.Public
		rowList = append(rowList, tempRow)
		rowCount += 1
	}

	// Convert the list to JSON, ready to send
	if rowCount > 0 {
		// Note that we can't use json.MarshalIndent() here, as that escapes '&' characters which stuffs up the url field
		var msg bytes.Buffer
		enc := json.NewEncoder(&msg)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rowList); err != nil {
			return nil, err
		}
		dbList = msg.Bytes()
	} else {
		// Return an empty set indicator, instead of "null"
		dbList = []byte{'{', '}'}
	}
	return dbList, nil
}
