package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	gz "github.com/NYTimes/gziphandler"
	"github.com/pkg/errors"
	com "github.com/sqlitebrowser/dbhub.io/common"
)

var (
	// Our self signed Certificate Authority chain
	ourCAPool *x509.CertPool

	// Address of our server, formatted for display
	server string
)

func main() {
	// Read server configuration
	var err error
	if err = com.ReadConfig(); err != nil {
		log.Fatalf("Configuration file problem: '%s'", err)
	}

	// Set the temp dir environment variable
	err = os.Setenv("TMPDIR", com.Conf.DiskCache.Directory)
	if err != nil {
		log.Fatalf("Setting temp directory environment variable failed: '%s'", err)
	}

	// Connect to Minio server
	err = com.ConnectMinio()
	if err != nil {
		log.Fatal(err)
	}

	// Connect to PostgreSQL server
	err = com.ConnectPostgreSQL()
	if err != nil {
		log.Fatal(err)
	}

	// Connect to the Memcached server
	err = com.ConnectCache()
	if err != nil {
		log.Fatal(err)
	}

	// Add the default user to the system
	err = com.AddDefaultUser()
	if err != nil {
		log.Fatal(err)
	}

	// Add the default licences to PostgreSQL
	err = com.AddDefaultLicences()
	if err != nil {
		log.Fatal(err)
	}

	// Load our self signed CA chain
	ourCAPool = x509.NewCertPool()
	certFile, err := os.ReadFile(com.Conf.DB4S.CAChain)
	if err != nil {
		log.Fatalf("Error opening Certificate Authority chain file: '%s'", err)
	}
	ok := ourCAPool.AppendCertsFromPEM(certFile)
	if !ok {
		log.Fatal("Error appending certificate file")
	}

	// URL handler
	mux := http.NewServeMux()
	mux.HandleFunc("/", rootHandler)
	mux.HandleFunc("/branch/list", branchListHandler)
	mux.HandleFunc("/licence/add", licenceAddHandler)
	mux.HandleFunc("/licence/get", licenceGetHandler)
	mux.HandleFunc("/licence/list", licenceListHandler)
	mux.HandleFunc("/licence/remove", licenceRemoveHandler)
	mux.HandleFunc("/metadata/get", metadataGetHandler)

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
		ErrorLog:     com.HttpErrorLog(),
		Handler:      gz.GzipHandler(mux),
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
	log.Printf("Starting DB4S end point on %s", server)
	log.Fatal(newServer.ListenAndServeTLS(com.Conf.DB4S.Certificate, com.Conf.DB4S.CertificateKey))
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
	dbOwner, _, dbName, err := com.GetUFD(r, true)
	if err != nil {
		http.Error(w, "Missing or incorrect data supplied", http.StatusBadRequest)
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckDBPermissions(userAcc, dbOwner, dbName, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !exists {
		fmt.Fprint(w, "{}")
		return
	}

	// Retrieve the branch list for the database
	brList, err := com.BranchListResponse(dbOwner, dbName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the list as JSON
	jsonList, err := json.MarshalIndent(brList, "", "  ")
	if err != nil {
		errMsg := fmt.Sprintf("Error when JSON marshalling the branch list: %v", err)
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

	// If the user has been banned, reject their authentication
	for _, u := range com.Conf.UserMgmt.BannedUsers {
		if u == userAcc {
			log.Printf("Banned user '%s' attempted to connect using DB4S", userAcc)
			err = errors.New("User has been banned.  Get in contact with us if you want the ban removed.")
			return
		}
	}

	// Everything is ok, so return
	return
}

func generateDefaultList(pageName string, userAcc string) (defaultList []byte, err error) {
	pageName += ":generateDefaultList()"

	// Retrieve the list of most recently modified (available) databases
	var userList com.UserInfoSlice
	userList, err = com.DB4SDefaultList(userAcc)
	if err != nil {
		// Return an empty set
		return []byte{'[', ']'}, err
	}

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
			LastModified: j.LastModified.Format(time.RFC3339)}
		linkRows = append(linkRows, newLink)
		rowCount++
	}

	if rowCount > 0 {
		// Use json.MarshalIndent() for nicer looking output
		defaultList, err = json.MarshalIndent(linkRows, "", "  ")
		if err != nil {
			log.Printf("%s: Error when JSON marshalling the default list: %v", pageName, err)
			return nil, errors.Wrap(err, fmt.Sprintf("%s: Error when JSON marshalling the default list",
				pageName))
		}
	} else {
		// Return an empty set indicator, instead of "null"
		defaultList = []byte{'[', ']'}
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
			// Yep, root directory request.  Log it and generate the browse list
			var userAgent string
			ua, ok := r.Header["User-Agent"]
			if ok {
				userAgent = ua[0]
			}
			if err := com.LogDB4SConnect(userAcc, r.RemoteAddr, userAgent, time.Now().UTC()); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			// Generate the list of potential user directories for browsing
			defaultList, err := generateDefaultList(pageName, userAcc)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			fmt.Fprintf(w, "%s", defaultList)
			return
		}

		// The request was for a user directory, so return that list
		desiredUserDir := pathStrings[1]
		err := com.ValidateUser(desiredUserDir)
		if err != nil {
			log.Printf("db4s: Validation failed for username: %s", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		dbList, err := userDatabaseList(userAcc, desiredUserDir)
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
		desiredUserDir := pathStrings[1]
		err := com.ValidateUser(desiredUserDir)
		if err != nil {
			log.Printf("db4s: Validation failed for username: %s", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		dbList, err := userDatabaseList(userAcc, desiredUserDir)
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
	exists, err := com.CheckDBPermissions(userAcc, dbOwner, dbName, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, fmt.Sprintf("Database '%s/%s' doesn't exist", com.SanitiseLogString(dbOwner),
			com.SanitiseLogString(dbName)), http.StatusNotFound)
		return
	}

	// Extract the requested database commit id from the form data
	commit, err := com.GetFormCommit(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get the branch heads list for the database
	branchList, err := com.GetBranches(dbOwner, dbName)
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
		branchName, err = com.GetDefaultBranchName(dbOwner, dbName)
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
	commitList, err := com.GetCommitList(dbOwner, dbName)
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
	err = retrieveDatabase(w, r, pageName, userAcc, dbOwner, dbName, branchName, commit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Adds a new licence to DBHub.io
func licenceAddHandler(w http.ResponseWriter, r *http.Request) {
	// Set the maximum accepted size for uploading
	r.Body = http.MaxBytesReader(w, r.Body, com.MaxLicenceSize*1024*1024) // 1MB.  Seems fairly generous for licence text.

	// Extract the account name and associated server from the validated client certificate
	userAcc, _, err := extractUserAndServer(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// The "public" user isn't allowed to make changes
	if userAcc == "public" {
		log.Printf("User from '%s' attempted to add a licence using the public certificate", r.RemoteAddr)
		http.Error(w, "You're using the 'public' certificate, which isn't allowed to make changes on the server",
			http.StatusUnauthorized)
		return
	}

	// Check whether the uploaded licence file is too large
	if r.ContentLength > (com.MaxLicenceSize * 1024 * 1024) {
		http.Error(w,
			fmt.Sprintf("Licence file is too large. Maximum licence upload size is %d MB, yours is %d MB",
				com.MaxLicenceSize, r.ContentLength/1024/1024), http.StatusBadRequest)
		log.Println(fmt.Sprintf("'%s' attempted to upload an oversized licence %d MB in size.  Limit is %d MB",
			userAcc, r.ContentLength/1024/1024, com.MaxLicenceSize))
		return
	}

	// Make sure a licence ID (short name) was provided
	l := r.FormValue("licence_id")
	if l == "" {
		http.Error(w, "No licence ID supplied", http.StatusBadRequest)
		return
	}
	err = com.ValidateLicence(l)
	if err != nil {
		log.Printf("Validation failed for licence ID: '%s': %s", com.SanitiseLogString(l), err)
		http.Error(w, "Validation of licence ID failed", http.StatusBadRequest)
		return
	}
	licID := l

	// If an (optional) full name for the licence was provided, then validate it
	var licName string
	if z := r.FormValue("licence_name"); z != "" {
		err = com.ValidateLicenceFullName(z)
		if err != nil {
			log.Printf("Validation failed for licence full name: '%s': %s", com.SanitiseLogString(z), err)
			http.Error(w, "Validation of licence full name failed", http.StatusBadRequest)
			return
		}
		licName = z
	}

	// The display order parameter is required
	do := r.FormValue("display_order")
	dispOrder, err := strconv.Atoi(do)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid display order: %v", html.EscapeString(do)), http.StatusBadRequest)
		return
	}

	// Ensure a licence by the same name (for this user) doesn't already exist, and the supplied display order isn't
	// already used
	licList, err := com.GetLicences(userAcc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for i, j := range licList {
		if i == licID {
			http.Error(w, "A licence with that short name/id already exists", http.StatusConflict)
			return
		}
		if dispOrder == j.Order {
			http.Error(w, "That display order number is already used by another licence", http.StatusConflict)
			return
		}
	}

	// The file format parameter is required, and can only be "text" or "html"
	var fileFormat string
	ff := r.FormValue("file_format")
	switch ff {
	case "text":
		fileFormat = "text"
	case "html":
		fileFormat = "html"
	default:
		http.Error(w, fmt.Sprintf("Unknown file format: %s", html.EscapeString(ff)), http.StatusBadRequest)
		return
	}

	// If a source URL was provided then validate it
	var sourceURL string
	su := r.FormValue("source_url")
	if su != "" {
		err = com.Validate.Var(su, "url,min=5,max=255") // 255 seems like a reasonable first guess
		if err != nil {
			http.Error(w, "Validation failed for source URL value", http.StatusBadRequest)
			return
		}
		sourceURL = su
	}

	// Grab the uploaded file and form variables
	tempFile, _, err := r.FormFile("file1")
	if err != nil {
		log.Printf("Uploading licence failed: %v", err)
		http.Error(w, fmt.Sprintf("Something went wrong when extracting the licence text: '%s'", err.Error()),
			http.StatusBadRequest)
		return
	}
	defer tempFile.Close()

	// Just use an in-memory buffer for the licence text
	licText := new(bytes.Buffer)
	_, err = io.Copy(licText, tempFile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Make sure this licence isn't a duplicate of an existing one
	tmpSHA := sha256.Sum256(licText.Bytes())
	licSHA := hex.EncodeToString(tmpSHA[:])
	licCheckName, licCheckURL, err := com.GetLicenceInfoFromSha256(userAcc, string(licSHA[:]))
	if err != nil && err.Error() != "No matching licence found, something has gone wrong!" {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if licCheckName != "" || licCheckURL != "" {
		http.Error(w, fmt.Sprintf("This licence is already in the system, using the short name of '%s'",
			licCheckName), http.StatusConflict)
		return
	}

	// Save the licence in the database
	err = com.StoreLicence(userAcc, licID, licText.Bytes(), sourceURL, dispOrder, licName, fileFormat)
	if err != nil {
		http.Error(w, fmt.Sprintf("Something went wrong when storing the new licence file: '%s'", err.Error()),
			http.StatusInternalServerError)
		return
	}

	// Send a success message back to the client
	w.WriteHeader(http.StatusCreated)
	_, _ = fmt.Fprintf(w, "Success")

	// Log the new licence addition
	log.Printf("New licence '%s' added to the server by user '%v'", com.SanitiseLogString(licID), userAcc)
	return
}

// Returns the text or html document for a specific licence
func licenceGetHandler(w http.ResponseWriter, r *http.Request) {
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
		log.Printf("Validation failed for licence name: '%s': %s", com.SanitiseLogString(l), err)
		http.Error(w, "Validation of licence name failed", http.StatusBadRequest)
		return
	}
	licenceName := l

	// Retrieve the licence from our database
	lic, format, err := com.GetLicence(userAcc, licenceName)
	if err != nil {
		if err.Error() == "unknown licence" {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
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
		log.Printf("Error returning licence file: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log the transfer
	log.Printf("Licence '%s' downloaded by user '%v', %d bytes", com.SanitiseLogString(licenceName), userAcc, bytesWritten)
	return
}

// Returns the list of licences known to the server
func licenceListHandler(w http.ResponseWriter, r *http.Request) {
	// Extract the account name and associated server from the validated client certificate
	userAcc, _, err := extractUserAndServer(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Return the list of licences as JSON
	licList, err := com.GetLicences(userAcc)
	jsonLicList, err := json.MarshalIndent(licList, "", "  ")
	if err != nil {
		errMsg := fmt.Sprintf("Error when JSON marshalling the licence list: %v", err)
		log.Print(errMsg)
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}
	fmt.Fprintf(w, string(jsonLicList))
	return
}

// Removes a (user added) licence from the system
func licenceRemoveHandler(w http.ResponseWriter, r *http.Request) {
	// Extract the account name and associated server from the validated client certificate
	userAcc, _, err := extractUserAndServer(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// The "public" user isn't allowed to make changes
	if userAcc == "public" {
		log.Printf("User from '%s' attempted to remove a licence using the public certificate", r.RemoteAddr)
		http.Error(w, "You're using the 'public' certificate, which isn't allowed to make changes on the server",
			http.StatusUnauthorized)
		return
	}

	// Make sure a licence short name was provided
	l := r.FormValue("licence_id")
	if l == "" {
		http.Error(w, "No licence name supplied", http.StatusBadRequest)
		return
	}

	// Validate the licence name
	err = com.ValidateLicence(l)
	if err != nil {
		log.Printf("Validation failed for licence name: '%s': %s", com.SanitiseLogString(l), err)
		http.Error(w, "Validation of licence name failed", http.StatusBadRequest)
		return
	}
	licenceName := l

	// Check if the licence to be deleted is in the system
	exists, err := com.CheckLicenceExists(userAcc, licenceName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !exists {
		http.Error(w, "A user supplied licence with that short name can't be found", http.StatusNotFound)
		return
	}

	// Remove the licence from our database
	err = com.DeleteLicence(userAcc, licenceName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Send a success message back to the client
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Success")

	// Log the transfer
	log.Printf("Licence '%s' removed by user '%v'", com.SanitiseLogString(licenceName), userAcc)
	return
}

// Returns the JSON metadata for a specific database
func metadataGetHandler(w http.ResponseWriter, r *http.Request) {
	// Extract the account name and associated server from the validated client certificate
	userAcc, _, err := extractUserAndServer(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Extract and validate the form variables
	dbOwner, _, dbName, err := com.GetUFD(r, true)
	if err != nil {
		http.Error(w, "Missing or incorrect data supplied", http.StatusBadRequest)
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckDBPermissions(userAcc, dbOwner, dbName, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, fmt.Sprintf("Database '%s/%s' doesn't exist", dbOwner, dbName),
			http.StatusNotFound)
		return
	}

	// Retrieve the metadata for the database
	meta, err := com.MetadataResponse(dbOwner, dbName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the list as JSON
	jsonList, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		errMsg := fmt.Sprintf("Error when JSON marshalling the branch list: %v", err)
		log.Print(errMsg)
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}
	fmt.Fprintf(w, string(jsonList))
	return
}

// postHandler receives uploaded files from DB4S. To simulate a DB4S upload, the following curl command can be used:
//
//	$ curl -kE ~/my.cert.pem -D headers.out -F file=@someupload.sqlite -F "branch=main" -F "commitmsg=stuff" \
//	    -F "sourceurl=https://example.org" -F "lastmodified=2017-01-02T03:04:05Z"  -F "licence=CC0"  -F "public=true" \
//	    https://db4s.dbhub.io:5550/someuser
//
// Subsequent uploads to the same database name will need to include an additional "commit" field, with the value of
// the commit ID last known to DB4S.  An example curl command demonstrating this:
//
//	$ curl -kE ~/my.cert.pem -D headers.out -F file=@someupload.sqlite -F "branch=main" -F "commitmsg=stuff" \
//	    -F "sourceurl=https://example.org" -F "lastmodified=2017-01-02T03:04:05Z"  -F "licence=CC0"  -F "public=true" \
//	    -F "commit=51d494f2c5eb6734ddaa204eccb9597b426091c79c951924ac83c72038f22b55" \
//	    https://db4s.dbhub.io:5550/someuser
func postHandler(w http.ResponseWriter, r *http.Request, userAcc string) {
	// Set the maximum accepted database size for uploading
	oversizeAllowed := false
	for _, user := range com.Conf.UserMgmt.SizeOverrideUsers {
		if userAcc == user {
			oversizeAllowed = true
		}
	}
	if !oversizeAllowed {
		r.Body = http.MaxBytesReader(w, r.Body, com.MaxDatabaseSize*1024*1024)
	}

	// The "public" user isn't allowed to make changes
	if userAcc == "public" {
		log.Printf("User from '%s' attempted to add a database using the public certificate", r.RemoteAddr)
		http.Error(w, "You're using the 'public' certificate, which isn't allowed to make changes on the server",
			http.StatusUnauthorized)
		return
	}

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
	if !oversizeAllowed {
		if r.ContentLength > (com.MaxDatabaseSize * 1024 * 1024) {
			http.Error(w,
				fmt.Sprintf("Database is too large. Maximum database upload size is %d MB, yours is %d MB",
					com.MaxDatabaseSize, r.ContentLength/1024/1024), http.StatusBadRequest)
			log.Println(fmt.Sprintf("'%s' attempted to upload an oversized database %d MB in size.  Limit is %d MB",
				userAcc, r.ContentLength/1024/1024, com.MaxDatabaseSize))
			return
		}
	}

	// Do the remaining input validation, and add the database to the system in the appropriate spot
	m, httpStatus, err := com.UploadResponse(w, r, userAcc, targetUser, "", "", "db4s")
	if err != nil {
		http.Error(w, err.Error(), httpStatus)
		return
	}

	// Convert to JSON
	var msg bytes.Buffer
	enc := json.NewEncoder(&msg)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send return message back to the client
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, msg.String())
}

// Returns a file requested by the client.  An example curl command to simulate the request is:
//
//	$ curl -OL -kE ~/my.cert.pem -D headers.out -G https://db4s.dbhub.io:5550/someuser/somedb.sqlite
func retrieveDatabase(w http.ResponseWriter, r *http.Request, pageName string, userAcc string, dbOwner string,
	dbName string, branchName string, commit string) (err error) {
	pageName += ":retrieveDatabase()"

	// Retrieve the Minio details and last modified date for the requested database
	bucket, id, lastMod, err := com.MinioLocation(dbOwner, dbName, commit, userAcc)
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
			log.Printf("%s: Error closing object handle: %v", pageName, err)
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
	err = com.LogDownload(dbOwner, dbName, userAcc, r.RemoteAddr, "db4s", userAgent, time.Now().UTC(),
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
	w.Header().Set("Branch", branchName)
	w.Header().Set("Commit-ID", commit)
	bytesWritten, err := io.Copy(w, userDB)
	if err != nil {
		log.Printf("%s: Error returning DB file: %v", pageName, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If downloaded by someone other than the owner, increment the download count for the database
	if strings.ToLower(userAcc) != strings.ToLower(dbOwner) {
		err = com.IncrementDownloadCount(dbOwner, dbName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Log the transfer
	log.Printf("'%s/%s' downloaded by user '%v', %v bytes", com.SanitiseLogString(dbOwner),
		com.SanitiseLogString(dbName), userAcc, bytesWritten)
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
		log.Printf("%s: Unknown request method received from '%v", pageName, userAcc)
		http.Error(w, fmt.Sprintf("Unknown request type: %v", reqType), http.StatusBadRequest)
	}
	return
}

// Returns the list of databases available to the user.  To simulate, the following curl command can be used:
//
//	$ curl -kE ~/my.cert.pem -D headers.out -G https://db4s.dbhub.io:5550/someuser
func userDatabaseList(userAcc string, user string) (dbList []byte, err error) {

	// Structure to hold the results, to apply JSON marshalling to
	type linkRow struct {
		CommitID     string `json:"commit_id"`
		DefBranch    string `json:"default_branch"`
		LastModified string `json:"last_modified"`
		Licence      string `json:"licence"`
		Name         string `json:"name"`
		OneLineDesc  string `json:"one_line_description"`
		Public       bool   `json:"public"`
		RepoModified string `json:"repo_modified"`
		SHA256       string `json:"sha256"`
		Size         int64  `json:"size"`
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
		tempRow.Name = j.Database
		tempRow.URL = fmt.Sprintf("%s/%s/%s?commit=%v", server, user, url.PathEscape(j.Database), j.CommitID)

		if j.DefaultBranch != "" {
			tempRow.DefBranch = j.DefaultBranch
			tempRow.URL += fmt.Sprintf("&branch=%s", j.DefaultBranch)
		}
		tempRow.OneLineDesc = j.OneLineDesc
		tempRow.Size = j.Size
		tempRow.SHA256 = j.SHA256
		tempRow.LastModified = j.LastModified.Format(time.RFC3339)
		tempRow.RepoModified = j.RepoModified.Format(time.RFC3339)
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
		dbList = []byte{'[', ']'}
	}
	return dbList, nil
}
