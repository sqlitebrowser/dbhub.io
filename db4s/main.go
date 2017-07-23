package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
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

func generateDefaultList(pageName string, userAcc string) (defaultList []byte, err error) {
	pageName += ":generateDefaultList()"

	// Generate list of most recently modified (available) databases
	var userList []com.UserInfo
	userList, err = com.DB4SDefaultList(userAcc)
	if err != nil {
		// Return an empty set
		return []byte{'{', '}'}, err
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

	// TODO: Add support for folders and branch names
	dbFolder := "/"
	branchName := "master"

	// Extract the requested database commit id from the form data
	commit, err := com.GetFormCommit(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If no commit ID was given, we grab the commit ID of the latest database from the default branch
	if commit == "" {
		if branchName == "" {
			commit, err = com.DefaultCommit(dbOwner, dbFolder, dbName)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}

	// A specific database was requested, so send it to the user
	err = retrieveDatabase(w, pageName, userAcc, dbOwner, dbFolder, dbName, commit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func main() {
	// Read server configuration
	var err error
	if err = com.ReadConfig(); err != nil {
		log.Fatalf("Configuration file problem\n\n%v", err)
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

	// Load our self signed CA chain
	ourCAPool = x509.NewCertPool()
	certFile, err := ioutil.ReadFile(com.DB4SCAChain())
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

	// Load our self signed CA Cert chain, request client certificates, and set TLS1.2 as minimum
	newTLSConfig := &tls.Config{
		ClientAuth:               tls.RequireAndVerifyClientCert,
		ClientCAs:                ourCAPool,
		MinVersion:               tls.VersionTLS12,
		PreferServerCipherSuites: true,
		RootCAs:                  ourCAPool,
	}
	newServer := &http.Server{
		Addr:         com.DB4SServer() + ":" + fmt.Sprint(com.DB4SServerPort()),
		Handler:      mux,
		TLSConfig:    newTLSConfig,
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0),
	}

	// Generate the formatted server string
	if com.DB4SServerPort() == 443 {
		server = fmt.Sprintf("https://%s", com.DB4SServer())
	} else {
		server = fmt.Sprintf("https://%s:%d", com.DB4SServer(), com.DB4SServerPort())
	}

	// Start server
	log.Printf("Starting DB4S end point on %s\n", server)
	log.Fatal(newServer.ListenAndServeTLS(com.DB4SServerCert(), com.DB4SServerCertKey()))
}

func putHandler(w http.ResponseWriter, r *http.Request, userAcc string) {
	pageName := "PUT request handler"

	// Split the request URL into path components
	pathStrings := strings.Split(r.URL.Path, "/")

	// Ensure both a username and target database name were given
	if len(pathStrings) <= 2 {
		http.Error(w, fmt.Sprintf("Bad target database URL: https://%s%s", com.DB4SServer(),
			r.URL.Path), http.StatusBadRequest)
		return
	}
	targetUser := pathStrings[1]
	targetDB := pathStrings[2]

	// Validate the targetUser and targetDB inputs
	err := com.ValidateUser(targetUser)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = com.ValidateDB(targetDB)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// TODO: Add support for folders, branches, licences, a commit message, and a source URL
	targetFolder := "/"
	branchName := "master"
	licenceName := "Not specified"
	commitMsg := ""
	sourceURL := ""

	// Get public/private setting for the database
	var public bool
	val := r.Header.Get("public")
	if val == "" {
		// No public/private variable found, so default to private
		public = false
	} else {
		public, err = strconv.ParseBool(val)
		if err != nil {
			// Public/private value couldn't be parsed, so default to private
			log.Printf("Error when converting public value to boolean: %v\n", err)
			public = false
		}
	}

	// Validate the database name
	err = com.ValidateDB(targetDB)
	if err != nil {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		http.Error(w, fmt.Sprintf("Invalid database name: %s", err), http.StatusBadRequest)
		return
	}

	// Verify the user is uploading to a location they have write access for
	if targetUser != userAcc {
		log.Printf("%s: Attempt by '%s' to write to unauthorised location: %v\n", pageName, userAcc,
			r.URL.Path)
		http.Error(w, fmt.Sprintf("Error code 401: You don't have write permission for '%s'",
			r.URL.Path), http.StatusForbidden)
		return
	}

	// Sanity check the uploaded database, and if ok then add it to the system
	numBytes, err := com.AddDatabase(userAcc, targetUser, targetFolder, targetDB, branchName, public, licenceName,
		commitMsg, sourceURL, r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log the successful database upload
	log.Printf("Database uploaded: '%s%s%s', bytes: %v\n", userAcc, targetFolder, targetDB, numBytes)

	// Indicate success back to DB4S
	http.Error(w, fmt.Sprintf("Database created: %s", r.URL.Path), http.StatusCreated)
}

func retrieveDatabase(w http.ResponseWriter, pageName string, userAcc string, dbOwner string, dbFolder string,
	dbName string, commit string) (err error) {
	pageName += ":retrieveDatabase()"

	// Retrieve the Minio details for the requested database
	bucket, id, err := com.MinioLocation(dbOwner, dbFolder, dbName, commit, userAcc)
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

	// Send the database to the user
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", url.QueryEscape(dbName)))
	w.Header().Set("Content-Type", "application/x-sqlite3")
	bytesWritten, err := io.Copy(w, userDB)
	if err != nil {
		log.Printf("%s: Error returning DB file: %v\n", pageName, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log the transfer
	log.Printf("'%s%s%s' downloaded by user '%v', %v bytes", dbOwner, dbFolder, dbName, userAcc, bytesWritten)
	return nil
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Main page"

	// Extract the account name and associated server from the validated client certificate
	var certServer string
	cn := r.TLS.PeerCertificates[0].Subject.CommonName
	if cn == "" {
		// Common name is empty
		http.Error(w, "Common name is blank in client certificate", http.StatusBadRequest)
		return
	}
	s := strings.Split(cn, "@")
	if len(s) < 2 {
		http.Error(w, "Missing information in client certificate", http.StatusBadRequest)
		return
	}
	userAcc := s[0]
	certServer = s[1]
	if userAcc == "" || certServer == "" {
		// Missing details in common name field
		http.Error(w, "Missing information in client certificate", http.StatusBadRequest)
		return
	}

	// Verify the running server matches the one in the certificate
	runningServer := com.DB4SServer()
	if certServer != runningServer {
		http.Error(w, fmt.Sprintf("Server name in certificate '%s' doesn't match running server '%s'\n",
			certServer, runningServer), http.StatusBadRequest)
		return
	}

	// ** By this point we have a validated user, and know their username (in userAcc) **
	reqType := r.Method
	switch reqType {
	case "GET":
		getHandler(w, r, userAcc)
	case "PUT":
		putHandler(w, r, userAcc)
	default:
		log.Printf("%s: Unknown request method received from '%v\n", pageName, userAcc)
		http.Error(w, fmt.Sprintf("Unknown request type: %v\n", reqType), http.StatusBadRequest)
	}
	return
}

// Returns the list of database available to the user
func userDatabaseList(pageName string, userAcc string, user string) (dbList []byte, err error) {
	pageName += ":userDatabaseList()"

	// Structure to hold the results, to apply JSON marshalling to
	type linkRow struct {
		Type         string `json:"type"`
		Name         string `json:"name"`
		CommitID     string `json:"commit_id"`
		URL          string `json:"url"`
		Size         int    `json:"size"`
		SHA256       string `json:"sha256"`
		Public       bool   `json:"public"`
		LastModified string `json:"last_modified"`
	}

	// Retrieve the list of databases for the requested username.  Only include those accessible to the logged
	// in user (userAcc) though
	var pubSetting com.AccessType
	if userAcc != user {
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
		tempRow.Size = j.Size
		tempRow.SHA256 = j.SHA256
		tempRow.LastModified = j.LastModified.Format(time.RFC822)
		tempRow.Public = j.Public
		rowList = append(rowList, tempRow)
		rowCount += 1
	}

	// Convert the list to JSON, ready to send
	if rowCount > 0 {
		// Use json.MarshalIndent() for nicer looking output
		dbList, err = json.MarshalIndent(rowList, "", "  ")
		if err != nil {
			log.Printf("%s: Error when JSON marshalling the default list: %v\n", pageName, err)
			return nil, errors.Wrap(err,
				fmt.Sprintf("%s: Error when JSON marshalling the default list", pageName))
		}
	} else {
		// Return an empty set indicator, instead of "null"
		dbList = []byte{'{', '}'}
	}
	return dbList, nil
}
