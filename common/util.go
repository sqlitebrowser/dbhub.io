// Useful utility functions
package common

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"time"
)

// The main function which handles database upload processing for both the webUI and DB4S end points
func AddDatabase(loggedInUser string, dbOwner string, dbFolder string, dbName string, branchName string,
	public bool, licenceName string, commitMsg string, sourceURL string, newDB io.Reader) (numBytes int64, err error) {

	// Write the temporary file locally, so we can try opening it with SQLite to verify it's ok
	var buf bytes.Buffer
	numBytes, err = io.Copy(&buf, newDB)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return numBytes, err
	}
	if numBytes == 0 {
		log.Printf("Database seems to be 0 bytes in length. Username: %s, Database: %s\n", loggedInUser, dbName)
		return numBytes, err
	}
	tempDB, err := ioutil.TempFile("", "dbhub-upload-")
	if err != nil {
		log.Printf("Error creating temporary file. User: '%s', Database: '%s%s%s', Filename: '%s', Error: %v\n",
			loggedInUser, dbOwner, dbFolder, dbName, tempDB.Name(), err)
		return numBytes, err
	}
	_, err = tempDB.Write(buf.Bytes())
	if err != nil {
		log.Printf("Error when writing the uploaded db to a temp file. User: '%s', Database: '%s%s%s' "+
			"Error: %v\n", loggedInUser, dbOwner, dbFolder, dbName, err)
		return numBytes, err
	}
	tempDBName := tempDB.Name()

	// Delete the temporary file when this function finishes
	defer os.Remove(tempDBName)

	// Sanity check the uploaded database
	err = SanityCheck(tempDBName)
	if err != nil {
		return numBytes, err
	}

	// Generate sha256 of the uploaded file
	s := sha256.Sum256(buf.Bytes())
	sha := hex.EncodeToString(s[:])

	// Check if the database already exists in the system
	needDefaultBranchCreated := false
	var branches map[string]BranchEntry
	exists, err := CheckDBExists(loggedInUser, dbFolder, dbName)
	if err != err {
		return numBytes, err
	}
	if exists {
		// Load the existing branchHeads for the database
		branches, err = GetBranches(loggedInUser, dbFolder, dbName)
		if err != nil {
			return numBytes, err
		}

		// If no branch name was given, use the default for the database
		if branchName == "" {
			branchName, err = GetDefaultBranchName(loggedInUser, dbFolder, dbName)
			if err != nil {
				return numBytes, err
			}
		}
	} else {
		// No existing branches, so this will be the first
		branches = make(map[string]BranchEntry)

		// Set the default branch name for the database
		if branchName == "" {
			branchName = "master"
		}
		needDefaultBranchCreated = true
	}

	// Create a dbTree entry for the individual database file
	var e DBTreeEntry
	e.EntryType = DATABASE
	e.Name = dbName
	e.Sha256 = sha
	e.Last_Modified = time.Now()
	// TODO: Check if there's a way to pass the last modified timestamp through a standard file upload control.  If
	// TODO  not, then it might only be possible through db4s, dio cli and similar
	//e.Last_Modified, err = time.Parse(time.RFC3339, modTime)
	//if err != nil {
	//	log.Println(err.Error())
	//	w.WriteHeader(http.StatusInternalServerError)
	//	return
	//}
	e.Size = buf.Len()
	if licenceName == "" || licenceName == "Not specified" {
		// No licence was specified by the client, so check if the database is already in the system and
		// already has one.  If so, we use that.
		if exists {
			headBranch, ok := branches[branchName]
			if !ok {
				return numBytes, errors.New("Error retrieving branch details")
			}
			commits, err := GetCommitList(loggedInUser, dbFolder, dbName)
			if err != nil {
				return numBytes, errors.New("Error retrieving commit list")
			}
			headCommit, ok := commits[headBranch.Commit]
			if !ok {
				return numBytes, fmt.Errorf("Err when looking up commit '%s' in commit list", headBranch.Commit)

			}
			if headCommit.Tree.Entries[0].LicenceSHA != "" {
				// The previous commit for the database had a licence, so we use that for this commit too
				e.LicenceSHA = headCommit.Tree.Entries[0].LicenceSHA
			}
		} else {
			// It's a new database, and the licence hasn't been specified
			e.LicenceSHA, err = GetLicenceSha256FromName(loggedInUser, licenceName)
			if err != nil {
				return numBytes, err
			}

			// If no commit message was given, use a default one and include the info of no licence being specified
			if commitMsg == "" {
				commitMsg = "Initial database upload, licence not specified."
			}
		}
	} else {
		// A licence was specified by the client, so use that
		e.LicenceSHA, err = GetLicenceSha256FromName(loggedInUser, licenceName)
		if err != nil {
			return numBytes, err
		}

		// Generate a reasonable commit message if none was given
		if !exists {
			commitMsg = fmt.Sprintf("Initial database upload, using licence %s.", licenceName)
		}
	}

	// Create a dbTree structure for the database entry
	var t DBTree
	t.Entries = append(t.Entries, e)
	t.ID = CreateDBTreeID(t.Entries)

	// Retrieve the display name and email address for the user
	dn, em, err := GetUserDetails(loggedInUser)
	if err != nil {
		return numBytes, err
	}

	// If either the display name or email address is empty, tell the user we need them first
	if dn == "" || em == "" {
		return numBytes, errors.New("You need to set your full name and email address in Preferences first")
	}

	// Construct a commit structure pointing to the tree
	var c CommitEntry
	c.AuthorName = dn
	c.AuthorEmail = em
	c.Message = commitMsg
	c.Timestamp = time.Now()
	c.Tree = t

	// If the database already exists, use the head commit for the appropriate branch as the parent for our new
	// uploads' commit
	if exists {
		b, ok := branches[branchName]
		if !ok {
			return numBytes, errors.New("Error when looking up branch details")
		}
		c.Parent = b.Commit
	}

	// Create the commit ID for the new upload
	c.ID = CreateCommitID(c)

	// If the database already exists, count the number of commits in the new branch
	commitCount := 1
	if exists {
		commitList, err := GetCommitList(loggedInUser, dbFolder, dbName)
		if err != nil {
			return numBytes, err
		}
		var ok bool
		var c2 CommitEntry
		c2.Parent = c.Parent
		for c2.Parent != "" {
			commitCount++
			c2, ok = commitList[c2.Parent]
			if !ok {
				m := fmt.Sprintf("Error when counting commits in branch '%s' of database '%s%s%s'\n", branchName,
					loggedInUser, dbFolder, dbName)
				log.Print(m)
				return numBytes, errors.New(m)
			}
		}
	}

	// Update the branch with the commit for this new database upload & the updated commit count for the branch
	b := branches[branchName]
	b.Commit = c.ID
	b.CommitCount = commitCount
	branches[branchName] = b
	err = StoreDatabase(loggedInUser, dbFolder, dbName, branches, c, public, buf.Bytes(), sha, "",
		"", needDefaultBranchCreated, branchName, sourceURL)
	if err != nil {
		return numBytes, err
	}

	// If the database already existed, update it's contributor count
	if exists {
		err = UpdateContributorsCount(loggedInUser, dbFolder, dbName)
		if err != nil {
			return numBytes, err
		}
	}

	// Invalidate the memcached entry for the database (only really useful if we're updating an existing database)
	err = InvalidateCacheEntry(loggedInUser, loggedInUser, "/", dbName, "") // Empty string indicates "for all versions"
	if err != nil {
		// Something went wrong when invalidating memcached entries for the database
		log.Printf("Error when invalidating memcache entries: %s\n", err.Error())
		return
	}

	// Invalidate any memcached entries for the previous highest version # of the database
	err = InvalidateCacheEntry(loggedInUser, loggedInUser, dbFolder, dbName, c.ID) // And empty string indicates "for all commits"
	if err != nil {
		// Something went wrong when invalidating memcached entries for any previous database
		log.Printf("Error when invalidating memcache entries: %s\n", err.Error())
		return
	}

	// Database successfully uploaded
	return numBytes, nil
}

// Generate a stable SHA256 for a commit.
func CreateCommitID(c CommitEntry) string {
	var b bytes.Buffer
	b.WriteString(fmt.Sprintf("tree %s\n", c.Tree.ID))
	if c.Parent != "" {
		b.WriteString(fmt.Sprintf("parent %s\n", c.Parent))
	}
	b.WriteString(fmt.Sprintf("author %s <%s> %v\n", c.AuthorName, c.AuthorEmail,
		c.Timestamp.Format(time.UnixDate)))
	if c.CommitterEmail != "" {
		b.WriteString(fmt.Sprintf("committer %s <%s> %v\n", c.CommitterName, c.CommitterEmail,
			c.Timestamp.Format(time.UnixDate)))
	}
	b.WriteString("\n" + c.Message)
	b.WriteByte(0)
	s := sha256.Sum256(b.Bytes())
	return hex.EncodeToString(s[:])
}

// Generate the SHA256 for a tree.
// Tree entry structure is:
// * [ entry type ] [ licence sha256] [ file sha256 ] [ file name ] [ last modified (timestamp) ] [ file size (bytes) ]
func CreateDBTreeID(entries []DBTreeEntry) string {
	var b bytes.Buffer
	for _, j := range entries {
		b.WriteString(string(j.EntryType))
		b.WriteByte(0)
		b.WriteString(string(j.LicenceSHA))
		b.WriteByte(0)
		b.WriteString(j.Sha256)
		b.WriteByte(0)
		b.WriteString(j.Name)
		b.WriteByte(0)
		b.WriteString(j.Last_Modified.Format(time.RFC3339))
		b.WriteByte(0)
		b.WriteString(fmt.Sprintf("%d\n", j.Size))
	}
	s := sha256.Sum256(b.Bytes())
	return hex.EncodeToString(s[:])
}

// Look for the next child fork in a fork tree
func nextChild(loggedInUser string, rawListPtr *[]ForkEntry, outputListPtr *[]ForkEntry, forkTrailPtr *[]int, iconDepth int) ([]ForkEntry, []int, bool) {
	// TODO: This approach feels half arsed.  Maybe redo it as a recursive function instead?

	// Resolve the pointers
	rawList := *rawListPtr
	outputList := *outputListPtr
	forkTrail := *forkTrailPtr

	// Grab the last database ID from the fork trail
	parentID := forkTrail[len(forkTrail)-1:][0]

	// Scan unprocessed rows for the first child of parentID
	numResults := len(rawList)
	for j := 1; j < numResults; j++ {
		// Skip already processed entries
		if rawList[j].Processed == false {
			if rawList[j].ForkedFrom == parentID {
				// * Found a fork of the parent *

				// Set the icon list for display in the browser
				for k := 0; k < iconDepth; k++ {
					rawList[j].IconList = append(rawList[j].IconList, SPACE)
				}
				rawList[j].IconList = append(rawList[j].IconList, END)

				// If the database is no longer public, then use placeholder details instead
				if !rawList[j].Public && (rawList[j].Owner != loggedInUser) {
					rawList[j].DBName = "private database"
				}

				// If the database is deleted, use a placeholder indicating that instead
				if rawList[j].Deleted {
					rawList[j].DBName = "deleted database"
				}

				// Add this database to the output list
				outputList = append(outputList, rawList[j])

				// Append this database ID to the fork trail
				forkTrail = append(forkTrail, rawList[j].ID)

				// Mark this database entry as processed
				rawList[j].Processed = true

				// Indicate a child fork was found
				return outputList, forkTrail, true
			}
		}
	}

	// Indicate no child fork was found
	return outputList, forkTrail, false
}

// Generate a random string
func RandomString(length int) string {
	rand.Seed(time.Now().UnixNano())
	const alphaNum = "abcdefghijklmnopqrstuvwxyz0123456789"
	randomString := make([]byte, length)
	for i := range randomString {
		randomString[i] = alphaNum[rand.Intn(len(alphaNum))]
	}

	return string(randomString)
}
