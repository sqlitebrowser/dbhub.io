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
	http.Handle("/v1/query", gz.GzipHandler(handleWrapper(queryHandler)))

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

// Wrapper function that doesn't nothing useful except interface between types
// TODO: Get rid of this, as it shouldn't be needed
func handleWrapper(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Call the original function
		fn(w, r)
	}
}

// Wrapper function to log incoming https requests.
func logReq(r *http.Request, loggedInUser string) {
	// Write request details to the request log
	fmt.Fprintf(reqLog, "%v - %s [%s] \"%s %s %s\" \"-\" \"-\" \"%s\" \"%s\"\n", r.RemoteAddr,
		loggedInUser, time.Now().Format(time.RFC3339Nano), r.Method, r.URL, r.Proto,
		r.Referer(), r.Header.Get("User-Agent"))
}

// Run a SQL query on the database
// This can be run from the command line using curl, like this:
//   curl -kD headers.out -F apikey="YOUR_API_KEY_HERE" -F dbowner="justinclift" -F dbname="Join Testing.sqlite" \
//     -F sql="U0VMRUNUIHRhYmxlMS5OYW1lLCB0YWJsZTIudmFsdWUKRlJPTSB0YWJsZTEgSk9JTiB0YWJsZTIKVVNJTkcgKGlkKQpPUkRFUiBCWSB0YWJsZTEuaWQ7" \
//     https://api.dbhub.io/v1/query
//   * "apikey" is one of your API keys.  These can be generated from your Settings page once logged in
//   * "dbowner" is the owner of the database being queried
//   * "dbname" is the name of the database being queried
//   * "sql" is the SQL query to run, base64 encoded
func queryHandler(w http.ResponseWriter, r *http.Request) {
	// Extract the API key from the request
	apiKey := r.FormValue("apikey")
	if apiKey == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Look up the owner of the API key
	loggedInUser, err := com.GetAPIKeyUser(apiKey)
	if err != nil || loggedInUser == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Log the incoming request
	logReq(r, loggedInUser)

	// Extract the database owner name, database name, and (optional) commit ID for the database from the request
	dbOwner, dbName, commitID, err := com.GetFormODC(r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	dbFolder := "/"

	// Grab the incoming SQLite query
	rawInput := r.FormValue("sql")
	decodedStr, err := com.CheckUnicode(rawInput)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, err)
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckDBExists(loggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Database '%s%s%s' doesn't exist", dbOwner, dbFolder, dbName)
		return
	}

	// Run the query
	var data com.SQLiteRecordSet
	data, err = com.SQLiteRunQueryDefensive(w, r, com.API, dbOwner, dbFolder, dbName, commitID, loggedInUser, decodedStr)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, err)
		return
	}

	// Return the results
	jsonData, err := json.Marshal(data.Records)
	if err != nil {
		errMsg := fmt.Sprintf("Error when JSON marshalling the returned data: %v\n", err)
		log.Print(errMsg)
		http.Error(w, errMsg, http.StatusBadRequest)
		return
	}
	fmt.Fprintf(w, string(jsonData))
}

// Handles requests for "/" and all unknown paths
func rootHandler(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		ServerName string
	}

	// Log the incoming request
	logReq(r, "-")

	// If the incoming request is for anything other than the index page, return a 404
	if r.URL.Path != "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Pass through some variables, useful for the generated docs
	pageData.ServerName = com.Conf.Web.ServerName

	// Display our API documentation
	t := tmpl.Lookup("docs")
	err := t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}
