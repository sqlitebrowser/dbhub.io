package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	com "github.com/sqlitebrowser/dbhub.io/common"
	"golang.org/x/crypto/bcrypt"
)

var (
	// Our parsed HTML templates
	tmpl *template.Template
)

func certDownloadHandler(w http.ResponseWriter, r *http.Request) {
	// Extract the username
	u, err := com.GetU(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	userName := strings.ToLower(u)

	// Retrieve the client certificate from the PG database
	cert, err := com.ClientCert(userName)
	if err != nil {
		//log.Printf("%s: Retrieving client cert from database failed: %v\n", pageName, err)
		http.Error(w, fmt.Sprintf("Retrieving client cert from database failed for user: %v", userName),
			http.StatusInternalServerError)
		return
	}

	// Send the client certificate to the user
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", userName+".cert.pem"))
	// Note, don't use "application/x-x509-user-cert", otherwise the browser may try to install it!
	// Useful reference info: https://pki-tutorial.readthedocs.io/en/latest/mime.html
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Write(cert)
	return
}

func certGenerateHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Client cert generate"

	// Extract the username
	u, err := com.GetU(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	userName := strings.ToLower(u)

	// Generate a new certificate
	// TODO: Use 14 days for now.  Extend this when things work properly.
	newCert, err := com.GenerateClientCert(userName, 14)
	if err != nil {
		log.Printf("%s: Error generating client certificate for user '%s': %s!\n", pageName, userName, err)
		http.Error(w, fmt.Sprintf("Error generating client certificate for user '%s': %s!\n", userName, err),
			http.StatusInternalServerError)
		return
	}

	// Store the new certificate in the database
	err = com.SetClientCert(newCert, userName)
	if err != nil {
		http.Error(w, fmt.Sprintf("Updating client certificate failed: %v", err),
			http.StatusInternalServerError)
		return
	}

	// Generate succeeded, so bounce back to the user modification page
	http.Redirect(w, r, fmt.Sprintf("/usermod?username=%s", userName), http.StatusSeeOther)
}

func certUploadHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Client cert upload"

	// Extract the username
	u, err := com.GetU(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	userName := strings.ToLower(u)

	// Grab the uploaded client certificate file
	tempFile, _, err := r.FormFile("cert")
	if err != nil {
		log.Printf("%s: Uploading new client certificate failed: %v\n", pageName, err)
		http.Error(w, fmt.Sprintf("Uploading new client certificate failed failed: %v\n", err),
			http.StatusInternalServerError)
		return
	}
	defer tempFile.Close()
	var certBuffer bytes.Buffer
	nBytes, err := certBuffer.ReadFrom(tempFile)
	if nBytes == 0 {
		log.Printf("%s: Extracting new client certificate failed\n", pageName)
		http.Error(w, fmt.Sprint("Extracting new client certificate failed\n"),
			http.StatusInternalServerError)
		return
	}

	// Update the user certificate
	err = com.SetClientCert(certBuffer.Bytes(), userName)
	if err != nil {
		http.Error(w, fmt.Sprintf("Updating client certificate failed: %v", err),
			http.StatusInternalServerError)
		return
	}

	// Log the successful certificate upload
	log.Printf("%s: Username: %v, new certificate uploaded, %v bytes\n", pageName, userName, nBytes)

	// Upload succeeded, so bounce back to the user modification page
	http.Redirect(w, r, fmt.Sprintf("/usermod?username=%s", userName), http.StatusSeeOther)
}

func dbDeleteHandler(w http.ResponseWriter, r *http.Request) {
	// Extract the form data
	dbOwner, dbName, dbVersion, err := com.GetUDV(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// TODO: Extract folder name from form data too

	// Retrieve the Minio bucket and id
	bucket, id, err := com.MinioBucketID(dbOwner, dbName, dbVersion, dbOwner)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Remove the database file from Minio
	err = com.RemoveMinioFile(bucket, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Remove the database version entry from PostgreSQL
	// TODO: Update this to handle folder names properly
	err = com.RemoveDBVersion(dbOwner, "/", dbName, dbVersion)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log the successful database removal
	log.Printf("Database entry removed for '%s/%s' version %v\n", dbOwner, dbName, dbVersion)

	// Success, so bounce back to the database management page
	http.Redirect(w, r, fmt.Sprintf("/dbmanage?username=%s", dbOwner), http.StatusSeeOther)
}

func dbDownloadHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Download DB"

	// Extract the form data
	dbOwner, dbName, dbVersion, err := com.GetUDV(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Retrieve the Minio bucket and id
	bucket, id, err := com.MinioBucketID(dbOwner, dbName, dbVersion, "")
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
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", dbName))
	w.Header().Set("Content-Type", "application/x-sqlite3")
	bytesWritten, err := io.Copy(w, userDB)
	if err != nil {
		log.Printf("%s: Error returning DB file: %v\n", pageName, err)
		fmt.Fprintf(w, "%s: Error returning DB file: %v\n", pageName, err)
		return
	}

	// Log the number of bytes written
	log.Printf("%s: '%v' downloaded by user '%v', %v bytes", pageName, dbName, dbOwner, bytesWritten)
}

// Handler to manage uploaded databases
func dbManageHandler(w http.ResponseWriter, r *http.Request) {
	// Extract the username
	u, err := com.GetU(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	userName := strings.ToLower(u)

	// If username wasn't in the form data, check if it's present URL encoded instead
	if userName == "" {
		u = r.FormValue("username")

		// Validate the username
		err = com.ValidateUser(u)
		if err != nil {
			log.Printf("Validation failed for username: %s", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Validation passed, so use this
		userName = strings.ToLower(u)
	}

	// Ensure a username was given
	if len(userName) < 1 {
		fmt.Fprint(w, "No username supplied!")
		return
	}

	// Assemble the row data into something the template can use
	type tempStruct struct {
		Username string
		PubDBs   []com.DBInfo
		PrivDBs  []com.DBInfo
		Bucket   string
	}
	var tempRows tempStruct
	tempRows.Username = userName

	// Gather list of public databases for the user
	tempRows.PubDBs, err = com.UserDBs(userName, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Gather list of private databases for the user
	tempRows.PrivDBs, err = com.UserDBs(userName, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get the Minio bucket for the user
	tempRows.Bucket, err = com.MinioUserBucket(userName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse the template file
	templateFile := filepath.Join("templates", "databases.html")
	t, err := template.ParseFiles(templateFile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Execute the template
	err = t.Execute(w, &tempRows)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Handler for database upload requests
func dbUploadHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "Database upload page"

	// Extract the form data
	r.ParseMultipartForm(32 << 20) // 64MB of ram max
	u, err := com.GetU(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	userName := strings.ToLower(u)
	folder, err := com.GetF(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	public, err := com.GetPub(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Default to the root folder if none was given
	if folder == "" {
		folder = "/"
	}

	// Make sure a username was given
	if len(userName) == 0 {
		// No username supplied
		log.Printf("%s: No username supplied!", pageName)
		fmt.Fprint(w, "No username supplied!")
		return
	}

	// Grab the uploaded database file
	tempFile, handler, err := r.FormFile("file")
	if err != nil {
		log.Printf("%s: Uploading file failed: %v\n", pageName, err)
		http.Error(w, fmt.Sprintf("Uploading file failed: %v\n", err), http.StatusInternalServerError)
		return
	}
	dbName := handler.Filename
	defer tempFile.Close()

	// Validate the database name
	err = com.ValidateDB(dbName)
	if err != nil {
		log.Printf("%s: Validation failed for database name: %s", pageName, err)
		http.Error(w, fmt.Sprintf("Invalid database name: %s", err), http.StatusBadRequest)
		return
	}

	var tempBuf bytes.Buffer
	nBytes, err := io.Copy(&tempBuf, tempFile)
	if err != nil {
		log.Printf("%s: Reading uploaded file failed: %v\n", pageName, err)
		http.Error(w, fmt.Sprintf("Reading uploaded file failed: %s", err), http.StatusInternalServerError)
		return
	}
	if nBytes == 0 {
		http.Error(w, "File size is 0 bytes", http.StatusInternalServerError)
		return
	}

	// Generate sha256 of the uploaded file
	shaSum := sha256.Sum256(tempBuf.Bytes())

	// Check if the database already exists
	ver, err := com.HighestDBVersion(userName, dbName, folder)
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

	// Retrieve the Minio bucket for the user
	bucket, err := com.MinioUserBucket(userName)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error retrieving Minio bucket: %v", err), http.StatusInternalServerError)
		return
	}

	// Store the database file in Minio
	// TODO: Is there a potential security problem here from using handler.Header[] directly?  Maybe needs validation
	bytesWritten, err := com.StoreMinioObject(bucket, minioID, &tempBuf, handler.Header["Content-Type"][0])
	if err != nil {
		log.Printf("%s: Storing file in Minio failed: %v\n", pageName, err)
		http.Error(w, fmt.Sprintf("Storing file in Minio failed: %v\n", err), http.StatusInternalServerError)
		return
	}

	// Add the new database details to the PG database
	err = com.AddDatabase(userName, folder, dbName, ver, shaSum[:], bytesWritten, public, bucket, minioID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Adding database to PostgreSQL failed: %v\n", err),
			http.StatusInternalServerError)
		return
	}

	// Log the successful database upload
	log.Printf("%s: Username: %v, database '%v' uploaded as '%v', bytes: %v\n", pageName, userName, dbName,
		minioID, bytesWritten)

	// Database upload succeeded, so bounce back to the database management page
	http.Redirect(w, r, fmt.Sprintf("/dbmanage?username=%s", userName), http.StatusSeeOther)
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

	// URL handlers
	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/certdownload", certDownloadHandler)
	http.HandleFunc("/certgenerate", certGenerateHandler)
	http.HandleFunc("/certupload", certUploadHandler)
	http.HandleFunc("/dbdel", dbDeleteHandler)
	http.HandleFunc("/dbdownload", dbDownloadHandler)
	http.HandleFunc("/dbmanage", dbManageHandler)
	http.HandleFunc("/dbupload", dbUploadHandler)
	http.HandleFunc("/useradd", userAddHandler)
	http.HandleFunc("/userdel", userDelHandler)
	http.HandleFunc("/usermod", userModFormHandler)
	http.HandleFunc("/usermodaction", userModActionHandler)

	// Start server
	if com.AdminServerHTTPS() {
		log.Printf("Starting DBHub datagen server on https://%s\n", com.AdminServerAddress())
		log.Fatal(http.ListenAndServeTLS(com.AdminServerAddress(), com.AdminServerCert(),
			com.AdminServerCertKey(), nil))
	} else {
		log.Printf("Starting DataGen datagen server on http://%s\n", com.AdminServerAddress())
		log.Fatal(http.ListenAndServe(com.AdminServerAddress(), nil))
	}
}

// Handler to generate the front page
func rootHandler(w http.ResponseWriter, _ *http.Request) {
	// Parse the template file
	templateFile := filepath.Join("templates", "index.html")
	t, err := template.ParseFiles(templateFile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Gather list of DBHub.io users
	userList, err := com.UserList()
	if err != nil {
		http.Error(w, fmt.Sprint("Couldn't retrieve list of users"), http.StatusInternalServerError)
		return
	}

	// Execute the template
	err = t.Execute(w, &userList)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Handler to add a new DBHub.io user
func userAddHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "User add page"

	// Extract the form data
	var err error
	if err = r.ParseForm(); err != nil {
		log.Printf("%s: ParseForm() error: %v\n", pageName, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	userName := strings.ToLower(r.PostFormValue("username"))
	email := r.PostFormValue("email")
	pass := r.PostFormValue("pword")
	verify := r.PostFormValue("pverify")

	// Validate the user supplied username and email address
	err = com.ValidateUserEmail(userName, email)
	if err != nil {
		log.Printf("Validation failed of username or email: %s", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check the password and confirmation match
	if len(pass) != len(verify) || pass != verify {
		http.Error(w, "Password and confirmation do not match", http.StatusBadRequest)
		return
	}

	// Check the password isn't too short
	if len(pass) < 6 {
		http.Error(w, "Password must be 6 characters or greater", http.StatusBadRequest)
		return
	}

	// Ensure both username and email address are present
	if userName == "" || email == "" {
		http.Error(w, "Username and email address must be present", http.StatusBadRequest)
		return
	}

	// Check if the user already exists
	x, err := com.CheckUserExists(userName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if x == true {
		// The user already exists
		http.Error(w, "Not continuing, the user already exists", http.StatusBadRequest)
		return
	}

	// Add the user
	err = com.AddUser(userName, pass, email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log the successful user creation
	log.Printf("%s: User created: %v\n", pageName, userName)

	// User creation succeeded, so bounce back to the front page
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Handler to delete a DBHub.io user
func userDelHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "User delete page"

	// Extract the username
	u, err := com.GetU(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	userName := strings.ToLower(u)

	// Retrieve the Minio bucket for the user
	bucket, err := com.MinioUserBucket(userName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if a Minio bucket for the user exists
	found, err := com.MinioBucketExists(bucket)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if found {
		// Remove the bucket and all files inside it
		err = com.RemoveMinioBucket(bucket)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Remove the user from PostgreSQL
	err = com.UserDelete(userName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log the successful user deletion
	log.Printf("%s: User deleted: %v\n", pageName, userName)

	// User deletion succeeded, so bounce back to the front page
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Handler to modify a DBHub.io user
func userModActionHandler(w http.ResponseWriter, r *http.Request) {
	pageName := "User modify action page"

	// Extract the form data
	var err error
	if err = r.ParseForm(); err != nil {
		log.Printf("%s: ParseForm() error: %v\n", pageName, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	userName := strings.ToLower(r.PostFormValue("username"))
	email := r.PostFormValue("email")
	pass := r.PostFormValue("pword")
	verify := r.PostFormValue("pverify")

	// Validate the user supplied username and email address
	err = com.ValidateUserEmail(userName, email)
	if err != nil {
		log.Printf("Validation failed of username or email: %s", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check the password and confirmation match
	if len(pass) != len(verify) || pass != verify {
		http.Error(w, "Password and confirmation do not match", http.StatusBadRequest)
		return
	}

	// Check the password isn't too short
	if len(pass) < 6 {
		http.Error(w, "Password must be 6 characters or greater", http.StatusBadRequest)
		return
	}

	// Ensure both username and email address are present
	if userName == "" || email == "" {
		http.Error(w, "Username and email address must be present", http.StatusBadRequest)
		return
	}

	// TODO: Add code to handle changes for the other fields

	// Handle whether the user password does/doesn't need to be changed
	var pHash []byte
	if pass != "" {
		// Hash the user's password
		pHash, err = bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("%s: Failed to bcrypt hash user password. User: '%v', error: %v.\n", pageName,
				userName, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		err = com.SetUserEmailPHash(userName, email, pHash)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		// Password wasn't supplied
		err = com.SetUserEmail(userName, email)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Log the successful user modification
	log.Printf("%s: User modified: %v\n", pageName, userName)

	// User modification succeeded, so bounce back to the front page
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Handler which generates a form to modify a DBHub.io user
func userModFormHandler(w http.ResponseWriter, r *http.Request) {
	// Extract the username
	u, err := com.GetU(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	userName := strings.ToLower(u)

	// If username wasn't in the form data, check if it's present URL encoded instead
	if userName == "" {
		u = r.FormValue("username")

		// Validate the username
		err = com.ValidateUser(u)
		if err != nil {
			log.Printf("Validation failed for username: %s", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Validation passed, so use this
		userName = strings.ToLower(u)
	}

	// Parse the template file
	templateFile := filepath.Join("templates", "modify_user.html")
	t, err := template.ParseFiles(templateFile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Retrieve the user info from the database
	user, err := com.User(userName)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error retrieving user info from database: %v\n", err),
			http.StatusInternalServerError)
	}

	// Execute the template
	err = t.Execute(w, user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
