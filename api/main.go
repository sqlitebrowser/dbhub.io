package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	gz "github.com/NYTimes/gziphandler"
	sqlite "github.com/gwenn/gosqlite"
	com "github.com/sqlitebrowser/dbhub.io/common"
)

var (
	// Our self signed Certificate Authority chain
	ourCAPool *x509.CertPool

	// Log file for incoming HTTPS requests
	reqLog *os.File

	// Address of our server, formatted for display
	server string

	// Our parsed HTML templates
	tmpl *template.Template
)

func main() {
	// Read server configuration
	var err error
	if err = com.ReadConfig(); err != nil {
		log.Fatalf("Configuration file problem\n\n%v", err)
	}

	// Open the request log for writing
	reqLog, err = os.OpenFile(com.Conf.Api.RequestLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY|os.O_SYNC, 0750)
	if err != nil {
		log.Fatalf("Error when opening request log: %s\n", err)
	}
	defer reqLog.Close()
	log.Printf("Request log opened: %s\n", com.Conf.Api.RequestLog)

	// Parse our template files
	tmpl = template.Must(template.New("templates").Delims("[[", "]]").ParseGlob(
		filepath.Join(com.Conf.Web.BaseDir, "api", "templates", "*.html")))

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
	err = com.AddDefaultUser()
	if err != nil {
		log.Fatalf(err.Error())
	}

	// Add the default licences to the system
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

	// Our pages
	http.Handle("/", gz.GzipHandler(handleWrapper(rootHandler)))
	http.Handle("/v1/branches", gz.GzipHandler(handleWrapper(branchesHandler)))
	http.Handle("/v1/columns", gz.GzipHandler(handleWrapper(columnsHandler)))
	http.Handle("/v1/commits", gz.GzipHandler(handleWrapper(commitsHandler)))
	http.Handle("/v1/databases", gz.GzipHandler(handleWrapper(databasesHandler)))
	http.Handle("/v1/delete", gz.GzipHandler(handleWrapper(deleteHandler)))
	http.Handle("/v1/diff", gz.GzipHandler(handleWrapper(diffHandler)))
	http.Handle("/v1/download", gz.GzipHandler(handleWrapper(downloadHandler)))
	http.Handle("/v1/indexes", gz.GzipHandler(handleWrapper(indexesHandler)))
	http.Handle("/v1/metadata", gz.GzipHandler(handleWrapper(metadataHandler)))
	http.Handle("/v1/query", gz.GzipHandler(handleWrapper(queryHandler)))
	http.Handle("/v1/releases", gz.GzipHandler(handleWrapper(releasesHandler)))
	http.Handle("/v1/tables", gz.GzipHandler(handleWrapper(tablesHandler)))
	http.Handle("/v1/tags", gz.GzipHandler(handleWrapper(tagsHandler)))
	http.Handle("/v1/upload", gz.GzipHandler(handleWrapper(uploadHandler)))
	http.Handle("/v1/views", gz.GzipHandler(handleWrapper(viewsHandler)))
	http.Handle("/v1/webpage", gz.GzipHandler(handleWrapper(webpageHandler)))

	// favicon.ico
	http.Handle("/favicon.ico", gz.GzipHandler(handleWrapper(func(w http.ResponseWriter, r *http.Request) {
		logReq(r, "-")
		http.ServeFile(w, r, filepath.Join(com.Conf.Web.BaseDir, "webui", "favicon.ico"))
	})))

	// Load our self signed CA Cert chain, check client certificates if given, and set TLS1.2 as minimum
	newTLSConfig := &tls.Config{
		ClientAuth:               tls.VerifyClientCertIfGiven,
		ClientCAs:                ourCAPool,
		MinVersion:               tls.VersionTLS12,
		PreferServerCipherSuites: true,
		RootCAs:                  ourCAPool,
	}
	srv := &http.Server{
		Addr:         com.Conf.Api.BindAddress,
		TLSConfig:    newTLSConfig,
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0),
	}

	// Generate the formatted server string
	server = fmt.Sprintf("https://%s", com.Conf.Api.ServerName)

	// Start API server
	log.Printf("API server starting on %s\n", server)
	err = srv.ListenAndServeTLS(com.Conf.Api.Certificate, com.Conf.Api.CertificateKey)

	// Shut down nicely
	com.DisconnectPostgreSQL()

	if err != nil {
		log.Fatal(err)
	}
}

// checkAuth authenticates and logs the incoming request
func checkAuth(w http.ResponseWriter, r *http.Request) (loggedInUser, apiKey string, err error) {
	// Extract the API key from the request
	a := r.FormValue("apikey")

	// Check if an API key was provided
	if a != "" {
		// Validate the API key
		err = com.CheckAPIKey(a)
		if err != nil {
			err = fmt.Errorf("Incorrect or unknown API key and certificate")
			return
		}

		// API key passed validation
		apiKey = a

		// Look up the owner of the API key
		loggedInUser, err = com.GetAPIKeyUser(apiKey)
	} else {
		// No API key was provided. Check for a client certificate instead
		loggedInUser, err = extractUserFromClientCert(w, r)
	}

	// Check for any errors
	if err != nil || loggedInUser == "" {
		err = fmt.Errorf("Incorrect or unknown API key and certificate")
		return
	}

	// If the client authenticated through their certificate instead of an API key, then we need to pass
	// that information along for special handling
	if apiKey == "" {
		apiKey = "clientcert"
	}

	// Log the incoming request
	logReq(r, loggedInUser)
	return
}

// collectInfo is an internal function which:
//   1. Authenticates incoming requests
//   2. Extracts the database owner, name, api key, and commit ID from the request
//   3. Checks permissions
func collectInfo(w http.ResponseWriter, r *http.Request) (loggedInUser, dbOwner, dbName, apiKey, commitID string, httpStatus int, err error) {
	// Authenticate the request
	loggedInUser, apiKey, err = checkAuth(w, r)
	if err != nil {
		httpStatus = http.StatusUnauthorized
		return
	}

	// Extract the database owner name, database name, and (optional) commit ID for the database from the request
	dbOwner, dbName, commitID, err = com.GetFormODC(r)
	if err != nil {
		httpStatus = http.StatusInternalServerError
		return
	}
	dbFolder := "/"

	// Check if the user has access to the requested database
	// Check if the requested database exists
	exists, err := com.CheckDBPermissions(loggedInUser, dbOwner, dbFolder, dbName, false)
	if err != nil {
		httpStatus = http.StatusInternalServerError
		return
	}
	if !exists {
		httpStatus = http.StatusNotFound
		err = fmt.Errorf("Database does not exist, or user isn't authorised to access it")
		return
	}
	return
}

// collectInfoAndOpen is an internal function which:
//   1. Calls collectInfo() to authenticate the request + collect the user/database/commit/etc details
//   2. Fetches the database from Minio
//   3. Opens the database, returning the connection handle
// This function exists purely because this code is common to most of the handlers
func collectInfoAndOpen(w http.ResponseWriter, r *http.Request, permName com.APIPermission) (sdb *sqlite.Conn, httpStatus int, err error) {
	// Authenticate the request and collect details for the requested database
	loggedInUser, dbOwner, dbName, apiKey, commitID, httpStatus, err := collectInfo(w, r)
	if err != nil {
		httpStatus = http.StatusInternalServerError
		return
	}
	dbFolder := "/"

	// Make sure the API key has permission to run this function on the requested database
	err = permissionCheck(loggedInUser, apiKey, dbName, permName)
	if err != nil {
		jsonErr(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Get Minio bucket
	bucket, id, _, err := com.MinioLocation(dbOwner, dbFolder, dbName, commitID, loggedInUser)
	if err != nil {
		httpStatus = http.StatusInternalServerError
		return
	}

	// Sanity check
	if id == "" {
		// The requested database wasn't found, or the user doesn't have permission to access it
		err = fmt.Errorf("Requested database not found")
		log.Printf("Requested database not found. Owner: '%s%s%s'", dbOwner, dbFolder, dbName)
		httpStatus = http.StatusNotFound
		return
	}

	// Retrieve database file from Minio, using locally cached version if it's already there
	var newDB string
	newDB, err = com.RetrieveDatabaseFile(bucket, id)
	if err != nil {
		httpStatus = http.StatusNotFound
		return
	}

	// Open the SQLite database in read only mode
	sdb, err = sqlite.Open(newDB, sqlite.OpenReadOnly)
	if err != nil {
		log.Printf("Couldn't open database in collectInfoAndOpen(): %s", err)
		httpStatus = http.StatusInternalServerError
		return
	}
	if err = sdb.EnableExtendedResultCodes(true); err != nil {
		log.Printf("Couldn't enable extended result codes in collectInfoAndOpen(): %v\n", err.Error())
		httpStatus = http.StatusInternalServerError
		return
	}
	return
}

func extractUserFromClientCert(w http.ResponseWriter, r *http.Request) (userAcc string, err error) {
	// Check if a client certificate was provided
	if len(r.TLS.PeerCertificates) == 0 {
		err = errors.New("No client certificate provided")
		return
	}

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
	certServer := s[1]
	if userAcc == "" || certServer == "" {
		// Missing details in common name field
		err = errors.New("Missing information in client certificate")
		return
	}

	// Verify the running server matches the one in the certificate
	db4sServer := com.Conf.DB4S.Server
	if certServer != db4sServer {
		err = fmt.Errorf("Server name in certificate '%s' doesn't match DB4S server '%s'\n", certServer,
			db4sServer)
		return
	}

	// Everything is ok, so return
	return
}

// handleWrapper does nothing useful except interface between types
// TODO: Get rid of this, as it shouldn't be needed
func handleWrapper(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Enable CORS (https://enable-cors.org)
		w.Header().Set("Access-Control-Allow-Origin", "*")

		// Call the original function
		fn(w, r)
	}
}

// jsonErr returns an error message wrapped in JSON, for (potentially) easier processing by an API caller
func jsonErr(w http.ResponseWriter, msg string, statusCode int) {
	je := com.JsonError{
		Error: msg,
	}
	jsonData, err := json.MarshalIndent(je, "", "  ")
	if err != nil {
		errMsg := fmt.Sprintf("A 2nd error occurred when JSON marshalling an error structure: %v\n", err)
		log.Print(errMsg)
		http.Error(w, errMsg, http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":"An error occurred when marshalling JSON inside jsonErr()"}`)
		return
	}
	w.WriteHeader(statusCode)
	fmt.Fprintf(w, string(jsonData))
}

// logReq writes an entry for the incoming request to the request log
func logReq(r *http.Request, loggedInUser string) {
	fmt.Fprintf(reqLog, "%v - %s [%s] \"%s %s %s\" \"-\" \"-\" \"%s\" \"%s\"\n", r.RemoteAddr,
		loggedInUser, time.Now().Format(time.RFC3339Nano), r.Method, r.URL, r.Proto,
		r.Referer(), r.Header.Get("User-Agent"))
}

// permissionCheck checks if a given incoming api key request is allowed to run on the requested database
func permissionCheck(loggedInUser, apiKey, dbName string, permName com.APIPermission) (err error) {
	// Retrieve the permission details for the api key
	var apiDetails com.APIKey
	apiDetails, err = com.APIKeyPerms(loggedInUser, apiKey)
	if err != nil {
		return
	}

	// If the user authenticated using their client certificate, we skip the permission checks as they have
	// full access to all their databases
	if apiKey != "clientcert" {
		// Ensure the database name matches
		// TODO: We probably need a special case for handling the Databases(), Releases(), and Tags() functions.
		if apiDetails.Database != dbName && apiDetails.Database != "" { // Empty string in the database means "All databases allowed"
			err = fmt.Errorf("Permission denied")
			return
		}

		// Ensure the required function permission has been granted
		if apiDetails.Permissions[permName] != true {
			err = fmt.Errorf("Permission denied")
			return
		}
	}
	return
}
