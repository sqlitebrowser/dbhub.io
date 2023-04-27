package main

// TODO: API functions that still need updating for Live databases
//         * diff - already updated to just return an error for live databases.  needs testing though

// FIXME: After the API and webui pieces are done, figure out how the DB4S end
//        point and dio should be updated to use live databases too

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
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
		log.Fatalf("Error when opening request log: %s", err)
	}
	defer reqLog.Close()
	log.Printf("Request log opened: %s", com.Conf.Api.RequestLog)

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

	// Connect to MQ server
	com.Conf.Live.Nodename = "API server"
	com.AmqpChan, err = com.ConnectMQ()
	if err != nil {
		log.Fatal(err)
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
	certFile, err := os.ReadFile(com.Conf.DB4S.CAChain)
	if err != nil {
		log.Printf("Error opening Certificate Authority chain file: %v", err)
		return
	}
	ok := ourCAPool.AppendCertsFromPEM(certFile)
	if !ok {
		log.Println("Error appending certificate file")
		return
	}

	// Our pages
	http.Handle("/", gz.GzipHandler(corsWrapper(rootHandler)))
	http.Handle("/changelog", gz.GzipHandler(corsWrapper(changeLogHandler)))
	http.Handle("/changelog.html", gz.GzipHandler(corsWrapper(changeLogHandler)))
	http.Handle("/v1/branches", gz.GzipHandler(corsWrapper(branchesHandler)))
	http.Handle("/v1/columns", gz.GzipHandler(corsWrapper(columnsHandler)))
	http.Handle("/v1/commits", gz.GzipHandler(corsWrapper(commitsHandler)))
	http.Handle("/v1/databases", gz.GzipHandler(corsWrapper(databasesHandler)))
	http.Handle("/v1/delete", gz.GzipHandler(corsWrapper(deleteHandler)))
	http.Handle("/v1/diff", gz.GzipHandler(corsWrapper(diffHandler)))
	http.Handle("/v1/download", gz.GzipHandler(corsWrapper(downloadHandler)))
	http.Handle("/v1/execute", gz.GzipHandler(corsWrapper(executeHandler)))
	http.Handle("/v1/indexes", gz.GzipHandler(corsWrapper(indexesHandler)))
	http.Handle("/v1/metadata", gz.GzipHandler(corsWrapper(metadataHandler)))
	http.Handle("/v1/query", gz.GzipHandler(corsWrapper(queryHandler)))
	http.Handle("/v1/releases", gz.GzipHandler(corsWrapper(releasesHandler)))
	http.Handle("/v1/tables", gz.GzipHandler(corsWrapper(tablesHandler)))
	http.Handle("/v1/tags", gz.GzipHandler(corsWrapper(tagsHandler)))
	http.Handle("/v1/upload", gz.GzipHandler(corsWrapper(uploadHandler)))
	http.Handle("/v1/views", gz.GzipHandler(corsWrapper(viewsHandler)))
	http.Handle("/v1/webpage", gz.GzipHandler(corsWrapper(webpageHandler)))

	// favicon.ico
	http.Handle("/favicon.ico", gz.GzipHandler(corsWrapper(func(w http.ResponseWriter, r *http.Request) {
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
		ErrorLog:     com.HttpErrorLog(),
		TLSConfig:    newTLSConfig,
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0),
	}

	// Generate the formatted server string
	server = fmt.Sprintf("https://%s", com.Conf.Api.ServerName)

	// Start API server
	log.Printf("API server starting on %s", server)
	err = srv.ListenAndServeTLS(com.Conf.Api.Certificate, com.Conf.Api.CertificateKey)

	// Shut down nicely
	com.DisconnectPostgreSQL()

	if err != nil {
		log.Fatal(err)
	}
}

// checkAuth authenticates and logs the incoming request
func checkAuth(w http.ResponseWriter, r *http.Request) (loggedInUser string, err error) {
	// Extract the API key from the request
	apiKey := r.FormValue("apikey")

	// Check if an API key was provided
	if apiKey != "" {
		// Validate the API key
		err = com.CheckAPIKey(apiKey)
		if err != nil {
			err = fmt.Errorf("Incorrect or unknown API key and certificate")
			return
		}

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

	// Log the incoming request
	logReq(r, loggedInUser)
	return
}

// collectInfo is an internal function which:
//  1. Authenticates incoming requests
//  2. Extracts the database owner, name, and commit ID from the request
//  3. Checks permissions
func collectInfo(w http.ResponseWriter, r *http.Request) (loggedInUser, dbOwner, dbName, commitID string, httpStatus int, err error) {
	// Authenticate the request
	loggedInUser, err = checkAuth(w, r)
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

	// Check if the user has access to the requested database
	// Check if the requested database exists
	exists, err := com.CheckDBPermissions(loggedInUser, dbOwner, dbName, false)
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
//  1. Calls collectInfo() to authenticate the request + collect the user/database/commit/etc details
//  2. Fetches the database from Minio
//  3. Opens the database, returning the connection handle
//
// This function exists purely because this code is common to most of the handlers
func collectInfoAndOpen(w http.ResponseWriter, r *http.Request) (sdb *sqlite.Conn, httpStatus int, err error) {
	// Authenticate the request and collect details for the requested database
	loggedInUser, dbOwner, dbName, commitID, httpStatus, err := collectInfo(w, r)
	if err != nil {
		httpStatus = http.StatusInternalServerError
		return
	}

	// Get Minio bucket
	bucket, id, _, err := com.MinioLocation(dbOwner, dbName, commitID, loggedInUser)
	if err != nil {
		httpStatus = http.StatusInternalServerError
		return
	}

	// Sanity check
	if id == "" {
		// The requested database wasn't found, or the user doesn't have permission to access it
		err = fmt.Errorf("Requested database not found")
		log.Printf("Requested database not found. Owner: '%s/%s'", com.SanitiseLogString(dbOwner),
			com.SanitiseLogString(dbName))
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
		log.Printf("Couldn't enable extended result codes in collectInfoAndOpen(): %v", err.Error())
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
		err = fmt.Errorf("Server name in certificate '%s' doesn't match DB4S server '%s'", certServer,
			db4sServer)
		return
	}

	// Everything is ok, so return
	return
}

// corsWrapper sets a general allow for all our api calls
func corsWrapper(fn http.HandlerFunc) http.HandlerFunc {
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
		errMsg := fmt.Sprintf("A 2nd error occurred when JSON marshalling an error structure: %v", err)
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
		loggedInUser, time.Now().Format(time.RFC3339Nano), r.Method, r.URL, r.Proto, r.Referer(), r.Header.Get("User-Agent"))
}
