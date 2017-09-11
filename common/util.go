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
	"net/http"
	"os"
	"runtime"
	"time"
)

// The main function which handles database upload processing for both the webUI and DB4S end points
func AddDatabase(r *http.Request, loggedInUser string, dbOwner string, dbFolder string, dbName string,
	createBranch bool, branchName string, commitID string, public bool, licenceName string, commitMsg string, sourceURL string, newDB io.Reader,
	serverSw string) (numBytes int64, newCommitID string, err error) {

	// Create a temporary file to store the database in
	tempDB, err := ioutil.TempFile(Conf.DiskCache.Directory, "dbhub-upload-")
	if err != nil {
		log.Printf("Error creating temporary file. User: '%s', Database: '%s%s%s', Filename: '%s', Error: %v\n",
			loggedInUser, dbOwner, dbFolder, dbName, tempDB.Name(), err)
		return 0, "", err
	}
	tempDBName := tempDB.Name()

	// Delete the temporary file when this function finishes
	defer os.Remove(tempDBName)

	// Write the database to the temporary file, so we can try opening it with SQLite to verify it's ok
	bufSize := 16 << 20 // 16MB
	buf := make([]byte, bufSize)
	numBytes, err = io.CopyBuffer(tempDB, newDB, buf)
	if err != nil {
		log.Printf("Error when writing the uploaded db to a temp file. User: '%s', Database: '%s%s%s' "+
			"Error: %v\n", loggedInUser, dbOwner, dbFolder, dbName, err)
		return 0, "", err
	}

	// Sanity check the uploaded database
	err = SanityCheck(tempDBName)
	if err != nil {
		return 0, "", err
	}

	// Return to the start of the temporary file
	newOff, err := tempDB.Seek(0, 0)
	if err != nil {
		log.Printf("Seeking on the temporary file failed: %v\n", err.Error())
		return 0, "", err
	}
	if newOff != 0 {
		return 0, "", errors.New("Seeking to the start of the temporary file failed")
	}

	// Generate sha256 of the uploaded file
	// TODO: Using an io.MultiWriter to feed data from newDB into both the temp file and this sha256 function at the
	// TODO  same time might be a better approach here
	s := sha256.New()
	_, err = io.CopyBuffer(s, tempDB, buf)
	if err != nil {
		return 0, "", err
	}
	sha := hex.EncodeToString(s.Sum(nil))

	// Check if the database already exists in the system
	needDefaultBranchCreated := false
	var branches map[string]BranchEntry
	exists, err := CheckDBExists(loggedInUser, loggedInUser, dbFolder, dbName)
	if err != err {
		return 0, "", err
	}
	if exists {
		// Load the existing branchHeads for the database
		branches, err = GetBranches(loggedInUser, dbFolder, dbName)
		if err != nil {
			return 0, "", err
		}

		// If no branch name was given, use the default for the database
		if branchName == "" {
			branchName, err = GetDefaultBranchName(loggedInUser, dbFolder, dbName)
			if err != nil {
				return 0, "", err
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
	e.Size = int(numBytes)
	if licenceName == "" || licenceName == "Not specified" {
		// No licence was specified by the client, so check if the database is already in the system and
		// already has one.  If so, we use that.
		if exists {
			lic, err := CommitLicenceSHA(loggedInUser, dbFolder, dbName, commitID)
			if err != nil {
				return 0, "", err
			}
			if lic != "" {
				// The previous commit for the database had a licence, so we use that for this commit too
				e.LicenceSHA = lic
			}
		} else {
			// It's a new database, and the licence hasn't been specified
			e.LicenceSHA, err = GetLicenceSha256FromName(loggedInUser, licenceName)
			if err != nil {
				return 0, "", err
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
			return 0, "", err
		}

		// Generate an appropriate commit message if none was provided
		if commitMsg == "" {
			if !exists {
				// A reasonable commit message for new database
				commitMsg = fmt.Sprintf("Initial database upload, using licence %s.", licenceName)
			} else {
				// The database already exists, so check if the licence has changed
				lic, err := CommitLicenceSHA(loggedInUser, dbFolder, dbName, commitID)
				if err != nil {
					return 0, "", err
				}
				if e.LicenceSHA != lic {
					// The licence has changed, so we create a reasonable commit message indicating this
					l, _, err := GetLicenceInfoFromSha256(loggedInUser, lic)
					if err != nil {
						return 0, "", err
					}
					commitMsg = fmt.Sprintf("Database licence changed from '%s' to '%s'.", l, licenceName)
				}
			}
		}
	}

	// Create a dbTree structure for the database entry
	var t DBTree
	t.Entries = append(t.Entries, e)
	t.ID = CreateDBTreeID(t.Entries)

	// Retrieve the display name and email address for the user
	dn, em, err := GetUserDetails(loggedInUser)
	if err != nil {
		return 0, "", err
	}

	// If either the display name or email address is empty, tell the user we need them first
	if dn == "" || em == "" {
		return 0, "", errors.New("You need to set your full name and email address in Preferences first")
	}

	// Construct a commit structure pointing to the tree
	var c CommitEntry
	c.AuthorName = dn
	c.AuthorEmail = em
	c.Message = commitMsg
	c.Timestamp = time.Now()
	c.Tree = t

	// If the database already exists, determine the commit ID to use as the parent
	if exists {
		b, ok := branches[branchName]
		if ok {
			// We're adding to a known branch.  If a commit was specifically provided, use that as the parent commit,
			// otherwise use the head commit of the branch
			if commitID != "" {
				if b.Commit != commitID {
					// We're rewriting commit history
					iTags, iRels, err := DeleteBranchHistory(dbOwner, dbFolder, dbName, branchName, commitID)
					if err != nil {
						if (len(iTags) > 0) || (len(iRels) > 0) {
							msg := fmt.Sprintln("You need to delete the following tags and releases before doing " +
								"this:")
							var rList, tList string
							if len(iTags) > 0 {
								// Would-be-isolated tags were identified.  Warn the user.
								msg += "  TAGS: "
								for _, tName := range iTags {
									if tList == "" {
										msg += fmt.Sprintf("'%s'", tName)
									} else {
										msg += fmt.Sprintf(", '%s'", tName)
									}
								}
							}
							if len(iRels) > 0 {
								// Would-be-isolated releases were identified.  Warn the user.
								msg += "  RELEASES: "
								for _, rName := range iRels {
									if rList == "" {
										msg += fmt.Sprintf("'%s'", rName)
									} else {
										msg += fmt.Sprintf(", '%s'", rName)
									}
								}
							}
							return 0, "", fmt.Errorf(msg)
						}
						return 0, "", err
					}
				}
				c.Parent = commitID
			} else {
				c.Parent = b.Commit
			}
		} else {
			// The branch name given isn't (yet) part of the database.  If we've been told to create the branch, then
			// we use the commit also passed (a requirement!) as the parent.  Otherwise, we error out
			if !createBranch {
				return 0, "", errors.New("Error when looking up branch details")
			}
			c.Parent = commitID
		}
	}

	// Create the commit ID for the new upload
	c.ID = CreateCommitID(c)

	// If the database already exists, count the number of commits in the new branch
	commitCount := 1
	if exists {
		commitList, err := GetCommitList(loggedInUser, dbFolder, dbName)
		if err != nil {
			return 0, "", err
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
				return 0, "", errors.New(m)
			}
		}
	}

	// Return to the start of the temporary file again
	newOff, err = tempDB.Seek(0, 0)
	if err != nil {
		log.Printf("Seeking on the temporary file (2nd time) failed: %v\n", err.Error())
		return 0, "", err
	}
	if newOff != 0 {
		return 0, "", errors.New("Seeking to start of temporary database file didn't work")
	}

	// Update the branch with the commit for this new database upload & the updated commit count for the branch
	b := branches[branchName]
	b.Commit = c.ID
	b.CommitCount = commitCount
	branches[branchName] = b
	err = StoreDatabase(loggedInUser, dbFolder, dbName, branches, c, public, tempDB, sha, numBytes, "",
		"", needDefaultBranchCreated, branchName, sourceURL)
	if err != nil {
		return 0, "", err
	}

	// If the database already existed, update it's contributor count
	if exists {
		err = UpdateContributorsCount(loggedInUser, dbFolder, dbName)
		if err != nil {
			return 0, "", err
		}
	}

	// If a new branch was created, then update the branch count for the database
	// Note, this could probably be merged into the StoreDatabase() call above, but it should be good enough for now
	if createBranch {
		err = StoreBranches(dbOwner, dbFolder, dbName, branches)
		if err != nil {
			return 0, "", err
		}
	}

	// Was a user agent part of the request?
	var userAgent string
	ua, ok := r.Header["User-Agent"]
	if ok {
		userAgent = ua[0]
	}

	// Make a record of the upload
	err = LogUpload(loggedInUser, dbFolder, dbName, loggedInUser, r.RemoteAddr, serverSw, userAgent, time.Now(), sha)
	if err != nil {
		return 0, "", err
	}

	// Invalidate the memcached entry for the database (only really useful if we're updating an existing database)
	err = InvalidateCacheEntry(loggedInUser, loggedInUser, "/", dbName, "") // Empty string indicates "for all versions"
	if err != nil {
		// Something went wrong when invalidating memcached entries for the database
		log.Printf("Error when invalidating memcache entries: %s\n", err.Error())
		return 0, "", err
	}

	// Invalidate any memcached entries for the previous highest version # of the database
	err = InvalidateCacheEntry(loggedInUser, loggedInUser, dbFolder, dbName, c.ID) // And empty string indicates "for all commits"
	if err != nil {
		// Something went wrong when invalidating memcached entries for any previous database
		log.Printf("Error when invalidating memcache entries: %s\n", err.Error())
		return 0, "", err
	}

	// Database successfully uploaded
	return numBytes, c.ID, nil
}

// Returns the licence used by the database in a given commit
func CommitLicenceSHA(dbOwner string, dbFolder string, dbName string, commitID string) (licenceSHA string, err error) {
	commits, err := GetCommitList(dbOwner, dbFolder, dbName)
	if err != nil {
		return "", err
	}
	c, ok := commits[commitID]
	if !ok {
		return "", fmt.Errorf("Commit not found in database commit list")
	}
	return c.Tree.Entries[0].LicenceSHA, nil
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

// Safely removes the commit history for a branch, from the head of the branch back to (but not including) the
// specified commit.  The new branch head will be at the commit ID specified
func DeleteBranchHistory(dbOwner string, dbFolder string, dbName string, branchName string, commitID string) (isolatedTags []string, isolatedRels []string, err error) {
	// Make sure the requested commit is in the history for the specified branch
	ok, err := IsCommitInBranchHistory(dbOwner, dbFolder, dbName, branchName, commitID)
	if err != nil {
		return
	}
	if !ok {
		err = fmt.Errorf("The specified commit isn't in the history of that branch")
		return
	}

	// Get the commit list for the database
	commitList, err := GetCommitList(dbOwner, dbFolder, dbName)
	if err != nil {
		return
	}

	// Walk the branch history, making a list of the commit IDs to delete
	delList := map[string]struct{}{}
	branchList, err := GetBranches(dbOwner, dbFolder, dbName)
	if err != nil {
		return
	}
	head, ok := branchList[branchName]
	if !ok {
		err = fmt.Errorf("Could not locate the head commit info for branch '%s'.  This shouldn't happen",
			branchName)
		return
	}
	if head.Commit == commitID {
		// The branch head is already at the specified commit.  There's nothing to do
		return // err still = nil
	}
	delList[head.Commit] = struct{}{}
	c, ok := commitList[head.Commit]
	if !ok {
		// The head commit wasn't found in the commit list.  This shouldn't happen
		err = fmt.Errorf("Head commit not found in database commit list.  This shouldn't happen")
		return
	}
	for c.Parent != "" {
		c, ok = commitList[c.Parent]
		if !ok {
			err = fmt.Errorf("Broken commit history encountered for branch '%s' in '%s%s%s', when looking for "+
				"commit '%s'\n", branchName, dbOwner, dbFolder, dbName, c.Parent)
			log.Printf(err.Error())
			return
		}
		if c.ID == commitID {
			// We've reached the desired commit, no need to keep walking the history
			break
		}

		// Add the commit ID to the deletion list
		delList[c.ID] = struct{}{}
	}

	// * To get here, we have the list of commits to delete *

	tagList, err := GetTags(dbOwner, dbFolder, dbName)
	if err != nil {
		return
	}

	relList, err := GetReleases(dbOwner, dbFolder, dbName)
	if err != nil {
		return
	}

	// Check if deleting the commits would leave isolated tags or releases
	type isolCheck struct {
		safe   bool
		commit string
	}
	commitTags := map[string]isolCheck{}
	commitRels := map[string]isolCheck{}
	for delCommit := range delList {

		// Ensure that deleting this commit won't result in any isolated/unreachable tags
		for tName, tEntry := range tagList {
			// Scan through the database tag list, checking if any of the tags is for the commit we're deleting
			if tEntry.Commit == delCommit {
				commitTags[tName] = isolCheck{safe: false, commit: delCommit}
			}
		}

		// Ensure that deleting this commit won't result in any isolated/unreachable releases
		for rName, rEntry := range relList {
			// Scan through the database release list, checking if any of the releases is on the commit we're deleting
			if rEntry.Commit == delCommit {
				commitRels[rName] = isolCheck{safe: false, commit: delCommit}
			}
		}
	}

	if len(commitTags) > 0 {
		// If a commit we're deleting has a tag on it, we need to check whether the commit is on other branches too
		//   * If it is, we're ok to proceed as the tag can still be reached from the other branch(es)
		//   * If it isn't, we need to abort this deletion (and tell the user), as the tag would become unreachable

		for bName, bEntry := range branchList {
			if bName == branchName {
				// We only run this comparison from "other branches", not the branch whose history we're changing
				continue
			}
			c, ok = commitList[bEntry.Commit]
			if !ok {
				err = fmt.Errorf("Broken commit history encountered when checking for isolated tags while "+
					"deleting commits in branch '%s' of database '%s%s%s'\n", branchName, dbOwner, dbFolder, dbName)
				log.Print(err.Error()) // Broken commit history is pretty serious, so we log it for admin investigation
				return
			}
			for tName, tEntry := range commitTags {
				if c.ID == tEntry.commit {
					// The commit is also on another branch, so we're ok to delete the commit
					tmp := commitTags[tName]
					tmp.safe = true
					commitTags[tName] = tmp
				}
			}
			for c.Parent != "" {
				c, ok = commitList[c.Parent]
				if !ok {
					err = fmt.Errorf("Broken commit history encountered when checking for isolated tags "+
						"while deleting commits in branch '%s' of database '%s%s%s'\n", branchName, dbOwner, dbFolder,
						dbName)
					log.Print(err.Error()) // Broken commit history is pretty serious, so we log it for admin investigation
					return
				}
				for tName, tEntry := range commitTags {
					if c.ID == tEntry.commit {
						// The commit is also on another branch, so we're ok to delete the commit
						tmp := commitTags[tName]
						tmp.safe = true
						commitTags[tName] = tmp
					}
				}
			}
		}

		// Create a list of would-be-isolated tags
		for tName, tEntry := range commitTags {
			if tEntry.safe == false {
				isolatedTags = append(isolatedTags, tName)
			}
		}
	}

	// Check if deleting the commits would leave isolated releases
	if len(commitRels) > 0 {
		// If a commit we're deleting has a release on it, we need to check whether the commit is on other branches too
		//   * If it is, we're ok to proceed as the release can still be reached from the other branch(es)
		//   * If it isn't, we need to abort this deletion (and tell the user), as the release would become unreachable
		for bName, bEntry := range branchList {
			if bName == branchName {
				// We only run this comparison from "other branches", not the branch whose history we're changing
				continue
			}
			c, ok = commitList[bEntry.Commit]
			if !ok {
				err = fmt.Errorf("Broken commit history encountered when checking for isolated releases "+
					"while deleting commits in branch '%s' of database '%s%s%s'\n", branchName, dbOwner, dbFolder,
					dbName)
				log.Print(err.Error()) // Broken commit history is pretty serious, so we log it for admin investigation
				return
			}
			for rName, rEntry := range commitRels {
				if c.ID == rEntry.commit {
					// The commit is also on another branch, so we're ok to delete the commit
					tmp := commitRels[rName]
					tmp.safe = true
					commitRels[rName] = tmp
				}
			}
			for c.Parent != "" {
				c, ok = commitList[c.Parent]
				if !ok {
					err = fmt.Errorf("Broken commit history encountered when checking for isolated releases "+
						"while deleting commits in branch '%s' of database '%s%s%s'\n", branchName, dbOwner, dbFolder,
						dbName)
					log.Print(err.Error()) // Broken commit history is pretty serious, so we log it for admin investigation
					return
				}
				for rName, rEntry := range commitRels {
					if c.ID == rEntry.commit {
						// The commit is also on another branch, so we're ok to delete the commit
						tmp := commitRels[rName]
						tmp.safe = true
						commitRels[rName] = tmp
					}
				}
			}
		}

		// Create a list of would-be-isolated releases
		for rName, rEntry := range commitRels {
			if rEntry.safe == false {
				isolatedRels = append(isolatedRels, rName)
			}
		}
	}

	// If any tags or releases would be isolated, abort
	if (len(isolatedTags) > 0) || (len(isolatedRels) > 0) {
		err = fmt.Errorf("Can't proceed, as isolated tags or releases would be left over")
		return
	}

	// Make a list of commits which aren't on any other branches, so should be removed from the commit list entirely
	checkList := map[string]bool{}
	for delCommit := range delList {
		checkList[delCommit] = true
	}
	for delCommit := range delList {
		for bName, bEntry := range branchList {
			if bName == branchName {
				// We only run this comparison from "other branches", not the branch whose history we're changing
				continue
			}

			// Walk the commit history for the branch, checking if it matches the current "delCommit" value
			c, ok = commitList[bEntry.Commit]
			if !ok {
				err = fmt.Errorf("Broken commit history encountered when checking for commits to remove in "+
					"branch '%s' of database '%s%s%s'\n", branchName, dbOwner, dbFolder, dbName)
				log.Print(err.Error()) // Broken commit history is pretty serious, so we log it for admin investigation
				return
			}
			if c.ID == delCommit {
				// The commit is also on another branch, so we *must not* remove it
				checkList[delCommit] = false
			}
			for c.Parent != "" {
				c, ok = commitList[c.Parent]
				if !ok {
					err = fmt.Errorf("Broken commit history encountered when checking for commits to remove "+
						"in branch '%s' of database '%s%s%s'\n", branchName, dbOwner, dbFolder, dbName)
					log.Print(err.Error()) // Broken commit history is pretty serious, so we log it for admin investigation
					return
				}
				if c.ID == delCommit {
					// The commit is also on another branch, so we *must not* remove it
					checkList[delCommit] = false
				}
			}
		}
	}

	// Rewind the branch history
	b, ok := branchList[branchName]
	b.Commit = commitID
	branchList[branchName] = b
	err = StoreBranches(dbOwner, dbFolder, dbName, branchList)
	if err != nil {
		return
	}

	// Remove any no-longer-needed commits
	// TODO: We may want to consider clearing any memcache entries for the deleted commits too
	for cid, del := range checkList {
		if del == true {
			delete(commitList, cid)
		}
	}
	err = StoreCommits(dbOwner, dbFolder, dbName, commitList)
	return
}

// Returns the name of the function this was called from
func GetCurrentFunctionName() (FuncName string) {
	stk := make([]uintptr, 1)
	runtime.Callers(2, stk[:])
	FuncName = runtime.FuncForPC(stk[0]).Name() + "()"
	return
}

// Checks if a given commit ID is in the history of the given branch
func IsCommitInBranchHistory(dbOwner string, dbFolder string, dbName string, branchName string, commitID string) (bool, error) {
	// Get the commit list for the database
	commitList, err := GetCommitList(dbOwner, dbFolder, dbName)
	if err != nil {
		return false, err
	}

	branchList, err := GetBranches(dbOwner, dbFolder, dbName)
	if err != nil {
		return false, err
	}

	// Walk the branch history, looking for the given commit ID
	head, ok := branchList[branchName]
	if !ok {
		// The given branch name wasn't found in the database branch list
		return false, fmt.Errorf("Branch '%s' not found in the database", branchName)
	}

	found := false
	c, ok := commitList[head.Commit]
	if !ok {
		// The head commit wasn't found in the commit list.  This shouldn't happen
		return false, fmt.Errorf("Head commit not found in database commit list.  This shouldn't happen")
	}
	for c.Parent != "" {
		c, ok = commitList[c.Parent]
		if !ok {
			log.Printf("Broken commit history encountered for branch '%s' in '%s%s%s', when looking for "+
				"commit '%s'\n", branchName, dbOwner, dbFolder, dbName, c.Parent)
			return false, fmt.Errorf("Broken commit history encountered for branch '%s' when looking up "+
				"commit details", branchName)
		}
		if c.ID == commitID {
			// The commit was found
			found = true
			break
		}
	}
	return found, nil
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
