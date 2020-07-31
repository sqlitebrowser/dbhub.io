package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	gz "github.com/NYTimes/gziphandler"
	sqlite "github.com/gwenn/gosqlite"
	com "github.com/sqlitebrowser/dbhub.io/common"
)

var (
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

	// Our pages
	http.Handle("/", gz.GzipHandler(handleWrapper(rootHandler)))
	http.Handle("/v1/columns", gz.GzipHandler(handleWrapper(columnsHandler)))
	http.Handle("/v1/diff", gz.GzipHandler(handleWrapper(diffHandler)))
	http.Handle("/v1/indexes", gz.GzipHandler(handleWrapper(indexesHandler)))
	http.Handle("/v1/query", gz.GzipHandler(handleWrapper(queryHandler)))
	http.Handle("/v1/tables", gz.GzipHandler(handleWrapper(tablesHandler)))
	http.Handle("/v1/views", gz.GzipHandler(handleWrapper(viewsHandler)))

	// Generate the formatted server string
	server = fmt.Sprintf("https://%s", com.Conf.Api.ServerName)

	// Start webUI server
	log.Printf("API server starting on %s\n", server)
	srv := &http.Server{
		Addr: com.Conf.Api.BindAddress,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12, // TLS 1.2 is now the lowest acceptable level
		},
	}
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
	if apiKey == "" {
		err = fmt.Errorf("Missing API key")
		return
	}

	// Look up the owner of the API key
	loggedInUser, err = com.GetAPIKeyUser(apiKey)
	if err != nil || loggedInUser == "" {
		err = fmt.Errorf("Incorrect or unknown API key")
		return
	}

	// Log the incoming request
	logReq(r, loggedInUser)
	return
}

// collectInfo is an internal function which:
//   1. Authenticates incoming requests
//   2. Extracts the database owner, name, & commitID from the request
//   3. Fetches the database from Minio (with appropriate permission checks)
//   4. Opens the database, returning the connection handle
// This function exists purely because this code is common to most of the handlers
func collectInfo(w http.ResponseWriter, r *http.Request) (sdb *sqlite.Conn, err error, httpStatus int) {
	var loggedInUser string
	loggedInUser, err = checkAuth(w, r)
	if err != nil {
		httpStatus = http.StatusUnauthorized
		return
	}

	// Extract the database owner name, database name, and (optional) commit ID for the database from the request
	var dbOwner, dbName, commitID string
	dbOwner, dbName, commitID, err = com.GetFormODC(r)
	if err != nil {
		httpStatus = http.StatusInternalServerError
		return
	}
	dbFolder := "/"

	// Check if the user has access to the requested database
	var bucket, id string
	bucket, id, _, err = com.MinioLocation(dbOwner, dbFolder, dbName, commitID, loggedInUser)
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
		log.Printf("Couldn't open database in viewsHandler(): %s", err)
		httpStatus = http.StatusInternalServerError
		return
	}
	if err = sdb.EnableExtendedResultCodes(true); err != nil {
		log.Printf("Couldn't enable extended result codes in viewsHandler(): %v\n", err.Error())
		httpStatus = http.StatusInternalServerError
		return
	}
	return
}

// handleWrapper does nothing useful except interface between types
// TODO: Get rid of this, as it shouldn't be needed
func handleWrapper(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Call the original function
		fn(w, r)
	}
}

// jsonErr returns an error message wrapped in JSON, for (potentially) easier processing by an API caller
func jsonErr(w http.ResponseWriter, msg string, statusCode int) {
	je := com.JsonError{
		Error: msg,
	}
	jsonData, err := json.Marshal(je)
	if err != nil {
		errMsg := fmt.Sprintf("A 2nd error occurred when JSON marshalling an error structure: %v\n", err)
		log.Print(errMsg)
		http.Error(w, errMsg, http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":"An error occured when marshalling JSON inside jsonErr()"}`)
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
