package common

import (
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sqlitebrowser/dbhub.io/common/config"
	"github.com/sqlitebrowser/dbhub.io/common/database"
)

// BranchListResponseContainer holds the response to a client request for the database branch list. It's a temporary
// structure, mainly so the JSON created for it is consistent between our various daemons
type BranchListResponseContainer struct {
	Branches      map[string]database.BranchEntry `json:"branches"`
	DefaultBranch string                          `json:"default_branch"`
}

// BranchListResponse returns the branch list for a database.  It's used by both the DB4S and API daemons, to ensure
// they return exactly the same data
func BranchListResponse(dbOwner, dbName string) (list BranchListResponseContainer, err error) {
	// Retrieve the branch list for the database
	list.Branches, err = database.GetBranches(dbOwner, dbName)
	if err != nil {
		return
	}

	// Retrieve the default branch for the database
	list.DefaultBranch, err = database.GetDefaultBranchName(dbOwner, dbName)
	if err != nil {
		return
	}
	return
}

// ExecuteResponseContainer is used by our job queue backend, to return information in response to an
// Execute() call on a live database.  It holds the success/failure status of the remote call,
// and also the number of rows changed by the Execute() call (if it succeeded)
type ExecuteResponseContainer struct {
	RowsChanged int    `json:"rows_changed"`
	Status      string `json:"status"`
}

// MetadataResponseContainer holds the response to a client request for database metadata. It's a temporary structure,
// mainly so the JSON created for it is consistent between our various daemons
type MetadataResponseContainer struct {
	Branches  map[string]database.BranchEntry  `json:"branches"`
	Commits   map[string]database.CommitEntry  `json:"commits"`
	DefBranch string                           `json:"default_branch"`
	Releases  map[string]database.ReleaseEntry `json:"releases"`
	Tags      map[string]database.TagEntry     `json:"tags"`
	WebPage   string                           `json:"web_page"`
}

// MetadataResponse returns the metadata for a database.  It's used by both the DB4S and API daemons, to ensure they
// return exactly the same data
func MetadataResponse(dbOwner, dbName string) (meta MetadataResponseContainer, err error) {
	// Get the branch heads list for the database
	meta.Branches, err = database.GetBranches(dbOwner, dbName)
	if err != nil {
		return
	}

	// Get the default branch for the database
	meta.DefBranch, err = database.GetDefaultBranchName(dbOwner, dbName)
	if err != nil {
		return
	}

	// Get the complete commit list for the database
	meta.Commits, err = database.GetCommitList(dbOwner, dbName)
	if err != nil {
		return
	}

	// Get the releases for the database
	meta.Releases, err = database.GetReleases(dbOwner, dbName)
	if err != nil {
		return
	}

	// Get the tags for the database
	meta.Tags, err = database.GetTags(dbOwner, dbName)
	if err != nil {
		return
	}

	// Generate the link to the web page of this database in the webUI module
	meta.WebPage = "https://" + config.Conf.Web.ServerName + "/" + dbOwner + "/" + dbName
	return
}

// UploadResponse validates incoming upload requests from the db4s and api daemons, then processes the upload
func UploadResponse(w http.ResponseWriter, r *http.Request, loggedInUser, targetUser, targetDB, commitID, serverSw string) (retMsg map[string]string, httpStatus int, err error) {
	// Grab the uploaded file and form variables
	var tempFile multipart.File
	var handler *multipart.FileHeader
	tempFile, handler, err = r.FormFile("file")
	if err != nil && err.Error() != "http: no such file" {
		log.Printf("Uploading file failed: %v", err)
		httpStatus = http.StatusBadRequest
		err = fmt.Errorf("Something went wrong when grabbing the file data: '%s'", err.Error())
		return
	}
	if err != nil {
		if err.Error() == "http: no such file" {
			// Check for a 'file1' FormFile too, as some clients can't use 'file' (without a number) due to a design bug
			tempFile, handler, err = r.FormFile("file1")
			if err != nil {
				log.Printf("Uploading file failed: %v", err)
				httpStatus = http.StatusBadRequest
				err = fmt.Errorf("Something went wrong when grabbing the file data: '%s'", err.Error())
				return
			}
		}
	}
	defer tempFile.Close()

	// If no database name was passed as a function argument, use the name given in the upload itself
	if targetDB == "" {
		targetDB = handler.Filename
	}

	// Validate the database name
	err = ValidateDB(targetDB)
	if err != nil {
		httpStatus = http.StatusBadRequest
		return
	}

	// TODO: These validation functions should probably be in the common library instead

	// Check if the database exists already
	exists, err := database.CheckDBExists(targetUser, targetDB)
	if err != nil {
		httpStatus = http.StatusInternalServerError
		return
	}

	// Check permissions
	if exists {
		allowed, err := database.CheckDBPermissions(loggedInUser, targetUser, targetDB, true)
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}
		if !allowed {
			return nil, http.StatusBadRequest, fmt.Errorf("Database not found")
		}
	} else if loggedInUser != targetUser {
		httpStatus = http.StatusForbidden
		err = fmt.Errorf("You cannot upload a database for another user")
		return
	}

	// If a branch name was provided then validate it
	var branchName string
	if z := r.FormValue("branch"); z != "" {
		err = Validate.Var(z, "branchortagname,min=1,max=32") // 32 seems a reasonable first guess.
		if err != nil {
			httpStatus = http.StatusBadRequest
			err = fmt.Errorf("Invalid branch name value: '%v'", z)
			return
		}
		branchName = z
	}

	// If the client sent a "force" field, validate it
	force := false
	if z := r.FormValue("force"); z != "" {
		force, err = strconv.ParseBool(z)
		if err != nil {
			// Force value couldn't be parsed
			httpStatus = http.StatusBadRequest
			err = fmt.Errorf("Error when converting force '%s' value to boolean: %v\n", z, err)
			return
		}
	}

	// If a licence name was provided then use it, else default to "Not specified"
	licenceName := "Not specified"
	if z := r.FormValue("licence"); z != "" {
		err = ValidateLicence(z)
		if err != nil {
			httpStatus = http.StatusBadRequest
			err = fmt.Errorf("Validation failed for licence name value: '%s': %s", z, err)
			return
		}

		// Make sure the licence is one that's known to us
		var licenceList map[string]database.LicenceEntry
		licenceList, err = database.GetLicences(loggedInUser)
		if err != nil {
			httpStatus = http.StatusInternalServerError
			return
		}
		_, ok := licenceList[z]
		if !ok {
			httpStatus = http.StatusBadRequest
			err = fmt.Errorf("Unknown licence: '%s'", z)
			return
		}
		licenceName = z
	}

	// If a source URL was provided then use it
	var sourceURL string
	if z := r.FormValue("sourceurl"); z != "" {
		err = Validate.Var(z, "url,min=5,max=255") // 255 seems like a reasonable first guess
		if err != nil {
			httpStatus = http.StatusBadRequest
			err = fmt.Errorf("Validation failed for source URL value")
			return
		}
		sourceURL = z
	}

	// If a database commit id was provided, then extract it
	if commitID == "" {
		commitID, err = GetFormCommit(r)
		if err != nil {
			httpStatus = http.StatusInternalServerError
			return
		}
	}

	// If a commit message was provided then use it
	var commitMsg string
	if z := r.FormValue("commitmsg"); z != "" {
		err = Validate.Var(z, "markdownsource,max=1024") // 1024 seems like a reasonable first guess
		if err != nil {
			httpStatus = http.StatusBadRequest
			err = fmt.Errorf("Validation failed for the commit message")
			return
		}
		commitMsg = z
	}

	// If a public/private setting was provided then use it
	var accessType database.SetAccessType
	if z := r.FormValue("public"); z != "" {
		var public bool
		public, err = strconv.ParseBool(z)
		if err != nil {
			httpStatus = http.StatusBadRequest
			err = fmt.Errorf("Error when converting public value to boolean: %v\n", err)
			return
		}

		if public {
			accessType = database.SetToPublic
		} else {
			accessType = database.SetToPrivate
		}
	}

	// If the last modified timestamp for the database file was provided, then validate it
	var lastMod time.Time
	if z := r.FormValue("lastmodified"); z != "" {
		lastMod, err = time.Parse(time.RFC3339, z)
		if err != nil {
			httpStatus = http.StatusBadRequest
			err = fmt.Errorf("Invalid lastmodified value: '%v'", z)
			return
		}
		lastMod = lastMod.UTC()
	} else {
		// No last modified time provided, so just use the current server time
		lastMod = time.Now().UTC()
	}

	// If the timestamp for the commit was provided, then validate it
	var commitTime time.Time
	if z := r.FormValue("committimestamp"); z != "" {
		commitTime, err = time.Parse(time.RFC3339, z)
		if err != nil {
			httpStatus = http.StatusBadRequest
			err = fmt.Errorf("Invalid commit timestamp value: '%v'", z)
			return
		}
		commitTime = commitTime.UTC()
	}

	// If the author name was provided then use it
	var authorName string
	if z := r.FormValue("authorname"); z != "" {
		err = ValidateDisplayName(z)
		if err != nil {
			httpStatus = http.StatusBadRequest
			err = fmt.Errorf("Validation failed for the author name")
			return
		}
		authorName = z
	}

	// If the author email was provided then use it
	var authorEmail string
	if z := r.FormValue("authoremail"); z != "" {
		err = ValidateEmail(z)
		if err != nil {
			httpStatus = http.StatusBadRequest
			err = fmt.Errorf("Validation failed for the author email")
			return
		}
		authorEmail = z
	}

	// If the committer name was provided then use it
	var committerName string
	if z := r.FormValue("committername"); z != "" {
		err = ValidateDisplayName(z)
		if err != nil {
			httpStatus = http.StatusBadRequest
			err = fmt.Errorf("Validation failed for the committer name")
			return
		}
		committerName = z
	}

	// If the committer email was provided then use it
	var committerEmail string
	if z := r.FormValue("committeremail"); z != "" {
		err = ValidateEmail(z)
		if err != nil {
			httpStatus = http.StatusBadRequest
			err = fmt.Errorf("Validation failed for the committer email")
			return
		}
		committerEmail = z
	}

	// If Other Parents info was provided then use it
	var otherParents []string
	if z := r.FormValue("otherparents"); z != "" {
		var x string
		x, err = url.QueryUnescape(z)
		if err != nil {
			httpStatus = http.StatusBadRequest
			err = fmt.Errorf("Validation failed for the other parents field")
			return
		}
		commits := strings.Split(x, ",")
		for _, j := range commits {
			// Validate each commit in the other parents field
			err = ValidateCommitID(j)
			if err != nil {
				httpStatus = http.StatusBadRequest
				err = fmt.Errorf("Validation failed for the other parents field")
				return
			}
			otherParents = append(otherParents, j)
		}
	}

	// If the database sha256 was provided then use it
	var dbSHA256 string
	if z := r.FormValue("dbshasum"); z != "" {
		err = Validate.Var(z, "hexadecimal,min=64,max=64")
		if err != nil {
			httpStatus = http.StatusBadRequest
			err = fmt.Errorf("Validation failed for the database SHA256")
			return
		}
		dbSHA256 = z
	}

	// Check if the database exists already
	if !exists && branchName == "" {
		// If the database doesn't already exist, and no branch name was provided, then default to "main"
		branchName = "main"
	}

	// If the database already exists, we need to do collision detection, check for forking, and check for force pushes
	createBranch := false
	if !exists {
		createBranch = true
	} else {
		if commitID == "" {
			httpStatus = http.StatusForbidden
			err = fmt.Errorf("A database with that name already exists.  Please choose a different name or clone the " +
				"existing database first.")
			return
		}

		// Retrieve the branch list for the database
		var branchList map[string]database.BranchEntry
		branchList, err = database.GetBranches(targetUser, targetDB)
		if err != nil {
			httpStatus = http.StatusInternalServerError
			return
		}

		// If a branch name was given, check if it's a branch we know about
		knownBranch := false
		var brDetails database.BranchEntry
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
				var a bool
				a, err = IsCommitInBranchHistory(targetUser, targetDB, branch, commitID)
				if err != nil {
					httpStatus = http.StatusInternalServerError
					return
				}
				if a {
					found = true
				}
			}
			if !found {
				// The commit wasn't found in the history of any branch
				httpStatus = http.StatusNotFound
				err = fmt.Errorf("Unknown commit ID: '%s'", commitID)
				return
			}
		} else {
			// * Collision detection piece *

			// Check if the provided commit ID is the latest head commit for the branch.  If it is, then things
			// are in order and this new upload should be a new commit on the branch.
			if brDetails.Commit != commitID {
				// * The provided commit doesn't match the HEAD commit for the specified branch *

				// Check if the provided commit is present in the history for the branch.  If it is, then the
				// database being pushed is out of date compared to the HEAD commit.  We'll need to abort
				// (with a suitable warning message), unless the force flag was passed + set to true
				var found bool
				found, err = IsCommitInBranchHistory(targetUser, targetDB, branchName, commitID)
				if err != nil {
					httpStatus = http.StatusInternalServerError
					return
				}

				if !found {
					// The provided commit ID isn't in the commit history for the branch, so there's something
					// wrong.  We need to error out and let the client know
					httpStatus = http.StatusNotFound
					err = fmt.Errorf("Commit ID '%s' isn't in the commit history of branch '%s'", commitID, branchName)
					return
				}

				// * To get here, this push is a collision *

				// The commit ID provided was found in the branch history but isn't the latest (HEAD) commit for
				// the branch.  Unless the "force" flag was provided by the client (and set to true), we error out to
				// notify the client of the collision.  It probably just means the database has been updated on the
				// server (eg through the webUI) but the user is still using an older version and needs to update

				if !force {
					httpStatus = http.StatusConflict
					err = fmt.Errorf("Outdated commit '%s' provided.  You're probably using an "+
						"old version of the database", commitID)
					return
				}

				// * To get here, the client has told us to rewrite the commit history for a branch, given us the
				//   required info, and provided the "force" flag set to true.  So, we drop through here and get
				//   it done *

			} else {
				// The provided commit ID matched the branch head, so things are in order.  We drop through and
				// create a new commit
			}
		}
	}

	// Sanity check the uploaded database, and if ok then add it to the system
	numBytes, returnCommitID, sha, err := AddDatabase(loggedInUser, targetUser, targetDB, createBranch,
		branchName, commitID, accessType, licenceName, commitMsg, sourceURL, tempFile, lastMod,
		commitTime, authorName, authorEmail, committerName, committerEmail, otherParents, dbSHA256)
	if err != nil {
		httpStatus = http.StatusInternalServerError
		return
	}

	// Was a user agent part of the request?
	var userAgent string
	ua, ok := r.Header["User-Agent"]
	if ok {
		userAgent = ua[0]
	}

	// Make a record of the upload
	err = database.LogUpload(loggedInUser, targetDB, loggedInUser, r.RemoteAddr, serverSw, userAgent, time.Now().UTC(), sha)
	if err != nil {
		httpStatus = http.StatusInternalServerError
		return
	}

	// Log the successful database upload
	log.Printf("Database uploaded: '%s/%s', bytes: %v", loggedInUser, SanitiseLogString(targetDB), numBytes)

	// Generate the formatted server string
	var server string
	if config.Conf.DB4S.Port == 443 {
		server = fmt.Sprintf("https://%s", config.Conf.DB4S.Server)
	} else {
		server = fmt.Sprintf("https://%s:%d", config.Conf.DB4S.Server, config.Conf.DB4S.Port)
	}

	// Construct message data for returning to DB4S (only) callers
	u := server + filepath.Join("/", targetUser, targetDB)
	u += fmt.Sprintf(`?branch=%s&commit=%s`, branchName, returnCommitID)
	retMsg = map[string]string{"commit_id": returnCommitID, "url": u}
	return
}
