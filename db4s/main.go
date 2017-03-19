package main

import (
	"bytes"
	"crypto/sha256"
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

func generateDefaultList(pageName string) (defaultList []byte, err error) {
	pageName += ":generateDefaultList()"

	// TODO: Decide what a good default/initial list should really contain

	// Gather list of DBHub.io users
	var userList []com.UserInfo
	userList, err = com.PublicUserDBs()
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

	// TODO: Update this function to handle folder names

	// Split the request URL into path components
	pathStrings := strings.Split(r.URL.Path, "/")

	// numPieces will be 2 if the request was for the root directory (https://server/), or if
	// the request included only a single path component (https://server/someuser/)
	numPieces := len(pathStrings)
	if numPieces == 2 {
		// Check if the request was for the root directory
		if pathStrings[1] == "" {
			// Yep, root directory request
			defaultList, err := generateDefaultList(pageName)
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

	// Extract the requested version number from the form data
	dbVersion, err := com.GetVersion(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// If no version number was given, we need to determine the highest available to the requesting user
	if dbVersion == 0 {
		dbVersion, err = com.HighestDBVersion(dbOwner, dbName, "/", userAcc)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// A specific database was requested, so send it to the user
	err = retrieveDatabase(w, pageName, userAcc, dbOwner, dbName, dbVersion)
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

	// Get public/private setting for the database
	public, err := strconv.ParseBool(r.Header.Get("public"))
	if err != nil {
		// Public/private value couldn't be parsed, so default to private
		log.Printf("Error when converting public value to boolean: %v\n", err)
		public = false
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

	// TODO: Check the uploaded file is a SQLite database.  Code for doing this is already in
	// TODO  dbhub-application:uploadDataHandler()

	// Copy the file into a local buffer
	var tempBuf bytes.Buffer
	nBytes, err := io.Copy(&tempBuf, r.Body)
	if err != nil {
		log.Printf("%s: Reading uploaded file failed: %v\n", pageName, err)
		http.Error(w, fmt.Sprintf("Reading uploaded file failed: %s", err),
			http.StatusInternalServerError)
		return
	}
	if nBytes == 0 {
		http.Error(w, "File size is 0 bytes", http.StatusInternalServerError)
		return
	}

	// Generate sha256 of the uploaded file
	shaSum := sha256.Sum256(tempBuf.Bytes())

	// Check if the database already exists
	ver, err := com.HighestDBVersion(userAcc, targetDB, "/", userAcc)
	if err != nil {
		// No database with that folder/name exists yet
		http.Error(w, fmt.Sprintf("Database query failure: %v", err), http.StatusInternalServerError)
		return
	}

	// Increment the highest version number (this also sets it to 1 if the database didn't exist previously)
	ver++

	// Generate random filename to store the database as
	minioID := com.RandomString(8) + ".db"

	// TODO: Do we need to check if that randomly generated filename is already used?

	// Get the Minio bucket name for the user
	bucket, err := com.MinioUserBucket(userAcc)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error retrieving Minio bucket: %v", err),
			http.StatusInternalServerError)
		return
	}

	// Store the database file in Minio
	dbSize, err := com.StoreMinioObject(bucket, minioID, &tempBuf, "application/x-sqlite3")
	if err != nil {
		log.Printf("%s: Storing file in Minio failed: %v\n", pageName, err)
		http.Error(w, fmt.Sprintf("Storing file in Minio failed: %v\n", err),
			http.StatusInternalServerError)
		return
	}

	// Add the new database details to the PG database
	err = com.AddDatabase(userAcc, "/", targetDB, ver, shaSum[:], dbSize, public, bucket, minioID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Adding database to PostgreSQL failed: %v\n", err),
			http.StatusInternalServerError)
		return
	}

	// Log the successful database upload
	log.Printf("Database uploaded: '%v'/'%v' version '%v', bytes: %v\n", userAcc, targetDB, ver, dbSize)

	// Indicate success back to DB4S
	http.Error(w, fmt.Sprintf("Database created: %s", r.URL.Path), http.StatusCreated)
}

func retrieveDatabase(w http.ResponseWriter, pageName string, userAcc string, user string, database string,
	version int) (err error) {
	pageName += ":retrieveDatabase()"

	// Retrieve the Minio bucket and id
	bucket, id, err := com.MinioBucketID(user, database, version, userAcc)
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
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", url.QueryEscape(database)))
	w.Header().Set("Content-Type", "application/x-sqlite3")
	bytesWritten, err := io.Copy(w, userDB)
	if err != nil {
		log.Printf("%s: Error returning DB file: %v\n", pageName, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log the transfer
	log.Printf("%s: '%v' downloaded by user '%v', %v bytes", pageName, database, user, bytesWritten)
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

func userDatabaseList(pageName string, userAcc string, user string) (dbList []byte, err error) {
	pageName += ":userDatabaseList()"

	// Structure to hold the results, to apply JSON marshalling to
	type linkRow struct {
		Type         string `json:"type"`
		Name         string `json:"name"`
		Version      int    `json:"version"`
		URL          string `json:"url"`
		Size         int    `json:"size"`
		LastModified string `json:"last_modified"`
	}

	// Retrieve the list of databases for the requested username.  Only include those accessible to the logged
	// in user (userAcc) though
	var pubSetting com.ValType
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
		tempRow.Version = j.Version
		if j.Folder == "/" {
			tempRow.Name = j.Database
			tempRow.URL = fmt.Sprintf("%s/%s/%s?version=%v", server, user,
				url.PathEscape(j.Database), j.Version)
		} else {
			tempRow.Name = fmt.Sprintf("%s/%s", strings.TrimPrefix(j.Folder, "/"),
				j.Database)
			tempRow.URL = fmt.Sprintf("%s/%s%s/%s?version=%v", server, user, j.Folder,
				url.PathEscape(j.Database), j.Version)
		}
		tempRow.Size = j.Size
		tempRow.LastModified = j.LastModified.Format(time.RFC822)
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
