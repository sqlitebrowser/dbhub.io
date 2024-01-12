package common

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/minio/minio-go"
)

var (
	// Our custom http error logger
	httpErrorLogger *log.Logger
)

// FilteringErrorLogWriter is a custom error logger for our http servers, to filter out the copious
// 'TLS handshake error' messages we're getting.
// Very heavily based on: https://github.com/golang/go/issues/26918#issuecomment-974257205
type FilteringErrorLogWriter struct{}

func (*FilteringErrorLogWriter) Write(msg []byte) (int, error) {
	z := string(msg)
	if !(strings.HasPrefix(z, "http: TLS handshake error") && strings.HasSuffix(z, ": EOF\n")) {
		err := httpErrorLogger.Output(0, z)
		if err != nil {
			log.Println(err)
		}
	}
	return len(msg), nil
}

// HttpErrorLog is used to filter out the copious 'TLS handshake error' messages we're getting
func HttpErrorLog() *log.Logger {
	httpErrorLogger = log.New(&FilteringErrorLogWriter{}, "", log.LstdFlags)
	return httpErrorLogger
}

// AddDatabase is handles database upload processing
func AddDatabase(loggedInUser, dbOwner, dbName string, createBranch bool, branchName,
	commitID string, accessType SetAccessType, licenceName, commitMsg, sourceURL string, newDB io.Reader,
	lastModified, commitTime time.Time, authorName, authorEmail, committerName, committerEmail string,
	otherParents []string, dbSha string) (numBytes int64, newCommitID string, calculatedDbSha string, err error) {

	// Check if the database already exists in the system
	exists, err := CheckDBExists(dbOwner, dbName)
	if err != nil {
		return
	}

	// Check permissions
	if exists {
		allowed, err := CheckDBPermissions(loggedInUser, dbOwner, dbName, true)
		if err != nil {
			return 0, "", "", err
		}
		if !allowed {
			return 0, "", "", errors.New("Database not found")
		}
	} else if loggedInUser != dbOwner {
		return 0, "", "", errors.New("You cannot upload a database for another user")
	}

	// Store the incoming database to a temporary file on disk, and sanity check it
	var sha string
	var sTbls []string
	var tempDB *os.File
	numBytes, tempDB, sha, sTbls, err = WriteDBtoDisk(loggedInUser, dbOwner, dbName, newDB)
	if err != nil {
		return
	}
	defer os.Remove(tempDB.Name())

	// If we were given a SHA256 for the file, make sure it matches our calculated one
	if dbSha != "" && dbSha != sha {
		err = fmt.Errorf("SHA256 given (%s) for uploaded file doesn't match the calculated value (%s)", dbSha, sha)
		return
	}

	// Figure out the branch structure to use
	var defBranch string
	needDefaultBranchCreated := false
	var branches map[string]BranchEntry
	if exists {
		// Load the existing branchHeads for the database
		branches, err = GetBranches(dbOwner, dbName)
		if err != nil {
			return
		}

		// If no branch name was given, use the default for the database
		defBranch, err = GetDefaultBranchName(dbOwner, dbName)
		if err != nil {
			return
		}
		if branchName == "" {
			branchName = defBranch
		}
	} else {
		// No existing branches, so this will be the first
		branches = make(map[string]BranchEntry)

		// Set the default branch name for the database
		if branchName == "" {
			branchName = "main"
		}
		needDefaultBranchCreated = true
	}

	// Create a dbTree entry for the individual database file
	var e DBTreeEntry
	e.EntryType = DATABASE
	e.Name = dbName
	e.Sha256 = sha
	e.LastModified = lastModified.UTC()
	e.Size = numBytes
	if licenceName == "" || licenceName == "Not specified" {
		// No licence was specified by the client, so check if the database is already in the system and
		// already has one.  If so, we use that.
		if exists {
			lic, err := CommitLicenceSHA(dbOwner, dbName, commitID)
			if err != nil {
				return 0, "", "", err
			}
			if lic != "" {
				// The previous commit for the database had a licence, so we use that for this commit too
				e.LicenceSHA = lic
			}
		} else {
			// It's a new database, and the licence hasn't been specified
			e.LicenceSHA, err = GetLicenceSha256FromName(loggedInUser, licenceName)
			if err != nil {
				return
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
			return
		}

		// Generate an appropriate commit message if none was provided
		if commitMsg == "" {
			if !exists {
				// A reasonable commit message for new database
				commitMsg = fmt.Sprintf("Initial database upload, using licence %s.", licenceName)
			} else {
				// The database already exists, so check if the licence has changed
				lic, err := CommitLicenceSHA(dbOwner, dbName, commitID)
				if err != nil {
					return 0, "", "", err
				}
				if e.LicenceSHA != lic {
					// The licence has changed, so we create a reasonable commit message indicating this
					l, _, err := GetLicenceInfoFromSha256(loggedInUser, lic)
					if err != nil {
						return 0, "", "", err
					}
					commitMsg = fmt.Sprintf("Database licence changed from '%s' to '%s'.", l, licenceName)
				}
			}
		}
	}

	// Figure out new access type
	var public bool
	switch accessType {
	case SetToPublic:
		public = true
	case SetToPrivate:
		public = false
	case KeepCurrentAccessType:
		public, err = CommitPublicFlag(loggedInUser, dbOwner, dbName, commitID)
		if err != nil {
			return
		}
	}

	// Create a dbTree structure for the database entry
	var t DBTree
	t.Entries = append(t.Entries, e)
	t.ID = CreateDBTreeID(t.Entries)

	// Retrieve the details for the user
	usr, err := User(loggedInUser)
	if err != nil {
		return
	}

	// If either the display name or email address is empty, tell the user we need them first
	if usr.DisplayName == "" || usr.Email == "" {
		err = errors.New("You need to set your full name and email address in Preferences first")
		return
	}

	// Construct a commit structure pointing to the tree
	var c CommitEntry
	if authorName != "" {
		c.AuthorName = authorName
	} else {
		c.AuthorName = usr.DisplayName
	}
	if authorEmail != "" {
		c.AuthorEmail = authorEmail
	} else {
		c.AuthorEmail = usr.Email
	}
	c.Message = commitMsg
	if !commitTime.IsZero() {
		c.Timestamp = commitTime.UTC()
	} else {
		c.Timestamp = time.Now().UTC()
	}
	c.Tree = t
	if committerName != "" {
		c.CommitterName = committerName
	}
	if committerEmail != "" {
		c.CommitterEmail = committerEmail
	}
	if otherParents != nil {
		c.OtherParents = otherParents
	}

	// If the database already exists, determine the commit ID to use as the parent
	if exists {
		b, ok := branches[branchName]
		if ok {
			// We're adding to a known branch.  If a commit was specifically provided, use that as the parent commit,
			// otherwise use the head commit of the branch
			if commitID != "" {
				if b.Commit != commitID {
					// We're rewriting commit history
					iTags, iRels, err := DeleteBranchHistory(dbOwner, dbName, branchName, commitID)
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
							return 0, "", "", fmt.Errorf(msg)
						}
						return 0, "", "", err
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
				return 0, "", "", errors.New("Error when looking up branch details")
			}
			c.Parent = commitID
		}
	}

	// Create the commit ID for the new upload
	c.ID = CreateCommitID(c)

	// If the database already exists, count the number of commits in the new branch
	commitCount := 1
	if exists {
		commitList, err := GetCommitList(dbOwner, dbName)
		if err != nil {
			return 0, "", "", err
		}
		var ok bool
		var c2 CommitEntry
		c2.Parent = c.Parent
		for c2.Parent != "" {
			commitCount++
			c2, ok = commitList[c2.Parent]
			if !ok {
				m := fmt.Sprintf("Error when counting commits in branch '%s' of database '%s/%s'", branchName,
					dbOwner, dbName)
				log.Print(SanitiseLogString(m))
				return 0, "", "", errors.New(m)
			}
		}
	}

	// Return to the start of the temporary file again
	var newOffset int64
	newOffset, err = tempDB.Seek(0, 0)
	if err != nil {
		log.Printf("Seeking on the temporary file (2nd time) failed: %v", err.Error())
		return
	}
	if newOffset != 0 {
		return 0, "", "", errors.New("Seeking to start of temporary database file didn't work")
	}

	// Update the branch with the commit for this new database upload & the updated commit count for the branch
	b := branches[branchName]
	b.Commit = c.ID
	b.CommitCount = commitCount
	branches[branchName] = b
	err = StoreDatabase(dbOwner, dbName, branches, c, public, tempDB, sha, numBytes, "",
		"", needDefaultBranchCreated, branchName, sourceURL)
	if err != nil {
		return
	}

	// If the database already existed, update its contributor count
	if exists {
		err = UpdateContributorsCount(dbOwner, dbName)
		if err != nil {
			return
		}
	}

	// If a new branch was created, then update the branch count for the database
	// Note, this could probably be merged into the StoreDatabase() call above, but it should be good enough for now
	if createBranch {
		err = StoreBranches(dbOwner, dbName, branches)
		if err != nil {
			return
		}
	}

	// If the newly uploaded database is on the default branch, check if the default table is present in this version
	// of the database.  If it's not, we need to clear the default table value
	if branchName == defBranch {
		defTbl, err := GetDefaultTableName(dbOwner, dbName)
		if err != nil {
			return 0, "", "", err
		}
		defFound := false
		for _, j := range sTbls {
			if j == defTbl {
				defFound = true
			}
		}
		if !defFound {
			// The default table is present in the previous commit, so we clear the default table value
			err = StoreDefaultTableName(dbOwner, dbName, "")
			if err != nil {
				return 0, "", "", err
			}
		}
	}

	// If the database didn't previously exist, add the user to the watch list for the database
	if !exists {
		err = ToggleDBWatch(loggedInUser, dbOwner, dbName)
		if err != nil {
			return
		}
	}

	// Invalidate the memcached entry for the database (only really useful if we're updating an existing database)
	err = InvalidateCacheEntry(loggedInUser, dbOwner, dbName, "") // Empty string indicates "for all versions"
	if err != nil {
		// Something went wrong when invalidating memcached entries for the database
		log.Printf("Error when invalidating memcache entries: %s", err.Error())
		return
	}

	// Invalidate any memcached entries for the previous highest version # of the database
	err = InvalidateCacheEntry(loggedInUser, dbOwner, dbName, c.ID) // An empty string indicates "for all commits"
	if err != nil {
		// Something went wrong when invalidating memcached entries for any previous database
		log.Printf("Error when invalidating memcache entries: %s", err.Error())
		return
	}

	// Database successfully uploaded
	return numBytes, c.ID, sha, nil
}

// CommitPublicFlag returns the public flag of a given commit
func CommitPublicFlag(loggedInUser, dbOwner, dbName, commitID string) (public bool, err error) {
	var DB SQLiteDBinfo
	err = DBDetails(&DB, loggedInUser, dbOwner, dbName, commitID)
	if err != nil {
		return
	}
	return DB.Info.Public, nil
}

// CommitLicenceSHA returns the licence used by the database in a given commit
func CommitLicenceSHA(dbOwner, dbName, commitID string) (licenceSHA string, err error) {
	commits, err := GetCommitList(dbOwner, dbName)
	if err != nil {
		return "", err
	}
	c, ok := commits[commitID]
	if !ok {
		return "", fmt.Errorf("Commit not found in database commit list")
	}
	return c.Tree.Entries[0].LicenceSHA, nil
}

// CreateCommitID generate a stable SHA256 for a commit
func CreateCommitID(c CommitEntry) string {
	var b bytes.Buffer
	b.WriteString(fmt.Sprintf("tree %s\n", c.Tree.ID))
	if c.Parent != "" {
		b.WriteString(fmt.Sprintf("parent %s\n", c.Parent))
	}
	for _, j := range c.OtherParents {
		b.WriteString(fmt.Sprintf("parent %s\n", j))
	}
	b.WriteString(fmt.Sprintf("author %s <%s> %v\n", c.AuthorName, c.AuthorEmail,
		c.Timestamp.UTC().Format(time.UnixDate)))
	if c.CommitterEmail != "" {
		b.WriteString(fmt.Sprintf("committer %s <%s> %v\n", c.CommitterName, c.CommitterEmail,
			c.Timestamp.UTC().Format(time.UnixDate)))
	}
	b.WriteString("\n" + c.Message)
	b.WriteByte(0)
	s := sha256.Sum256(b.Bytes())
	return hex.EncodeToString(s[:])
}

// CreateDBTreeID generate the SHA256 for a tree
// Tree entry structure is:
// * [ entry type ] [ licence sha256] [ file sha256 ] [ file name ] [ last modified (timestamp) ] [ file size (bytes) ]
func CreateDBTreeID(entries []DBTreeEntry) string {
	var b bytes.Buffer
	for _, j := range entries {
		b.WriteString(string(j.EntryType))
		b.WriteByte(0)
		b.WriteString(j.LicenceSHA)
		b.WriteByte(0)
		b.WriteString(j.Sha256)
		b.WriteByte(0)
		b.WriteString(j.Name)
		b.WriteByte(0)
		b.WriteString(j.LastModified.Format(time.RFC3339))
		b.WriteByte(0)
		b.WriteString(fmt.Sprintf("%d\n", j.Size))
	}
	s := sha256.Sum256(b.Bytes())
	return hex.EncodeToString(s[:])
}

// DataValuesMatch compares two slices of DataValue objects. It returns true if the two are equal, false otherwise.
func DataValuesMatch(a []DataValue, b []DataValue) (equal bool) {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

// DeleteBranchHistory safely removes the commit history for a branch, from the head of the branch back to (but not
// including) the specified commit.  The new branch head will be at the commit ID specified
func DeleteBranchHistory(dbOwner, dbName, branchName, commitID string) (isolatedTags, isolatedRels []string, err error) {
	// Make sure the requested commit is in the history for the specified branch
	ok, err := IsCommitInBranchHistory(dbOwner, dbName, branchName, commitID)
	if err != nil {
		return
	}
	if !ok {
		err = fmt.Errorf("The specified commit isn't in the history of that branch")
		return
	}

	// Get the commit list for the database
	commitList, err := GetCommitList(dbOwner, dbName)
	if err != nil {
		return
	}

	// Walk the branch history, making a list of the commit IDs to delete
	delList := map[string]struct{}{}
	branchList, err := GetBranches(dbOwner, dbName)
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
			err = fmt.Errorf("Broken commit history encountered for branch '%s' in '%s/%s', when looking for "+
				"commit '%s'", branchName, dbOwner, dbName, c.Parent)
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

	tagList, err := GetTags(dbOwner, dbName)
	if err != nil {
		return
	}

	relList, err := GetReleases(dbOwner, dbName)
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
					"deleting commits in branch '%s' of database '%s/%s'", branchName, dbOwner, dbName)
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
						"while deleting commits in branch '%s' of database '%s/%s'", branchName, dbOwner,
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
					"while deleting commits in branch '%s' of database '%s/%s'", branchName, dbOwner,
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
						"while deleting commits in branch '%s' of database '%s/%s'", branchName, dbOwner,
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
					"branch '%s' of database '%s/%s'", branchName, dbOwner, dbName)
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
						"in branch '%s' of database '%s/%s'", branchName, dbOwner, dbName)
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

	// Count the number of commits in the updated branch
	c = commitList[commitID]
	commitCount := 1
	for c.Parent != "" {
		commitCount++
		c, ok = commitList[c.Parent]
		if !ok {
			log.Printf("Error when counting # of commits while rewriting branch '%s' of database '%s/%s'",
				SanitiseLogString(branchName), SanitiseLogString(dbOwner), SanitiseLogString(dbName))
			err = fmt.Errorf("Error when counting commits during branch history rewrite")
			return
		}
	}

	// Rewind the branch history
	b, _ := branchList[branchName]
	b.Commit = commitID
	b.CommitCount = commitCount
	branchList[branchName] = b
	err = StoreBranches(dbOwner, dbName, branchList)
	if err != nil {
		return
	}

	// Remove any no-longer-needed commits
	// TODO: We may want to consider clearing any Memcache entries for the deleted commits too
	for cid, del := range checkList {
		if del == true {
			delete(commitList, cid)
		}
	}
	err = StoreCommits(dbOwner, dbName, commitList)
	return
}

// GetCommonAncestorCommits determines the common ancestor commit (if any) between a source and destination branch.
// Returns the commit ID of the ancestor and a slice of the commits between them.  If no common ancestor exists, the
// returned ancestorID will be an empty string. Created for use by our Merge Request functions.
func GetCommonAncestorCommits(srcOwner, srcDBName, srcBranch, destOwner, destName,
	destBranch string) (ancestorID string, commitList []CommitEntry, errType int, err error) {

	// To determine the common ancestor, we retrieve the source and destination commit lists, then starting from the
	// end of the source list, step backwards looking for a matching ID in the destination list.
	//   * If none is found then there's nothing in common (so abort).
	//   * If one is found, that one is the last common commit

	// Get the details of the head commit for the source and destination database branches
	branchList, err := GetBranches(destOwner, destName) // Destination branch list
	if err != nil {
		errType = http.StatusInternalServerError
		return
	}
	branchDetails, ok := branchList[destBranch]
	if !ok {
		errType = http.StatusInternalServerError
		err = fmt.Errorf("Could not retrieve details for the destination branch")
		return
	}
	destCommitID := branchDetails.Commit
	srcBranchList, err := GetBranches(srcOwner, srcDBName)
	if err != nil {
		errType = http.StatusInternalServerError
		return
	}
	srcBranchDetails, ok := srcBranchList[srcBranch]
	if !ok {
		errType = http.StatusInternalServerError
		err = fmt.Errorf("Could not retrieve details for the source branch")
		return
	}
	srcCommitID := srcBranchDetails.Commit

	// If the source and destination commit IDs are the same, then abort
	if srcCommitID == destCommitID {
		errType = http.StatusBadRequest
		err = fmt.Errorf("Source and destination commits are identical, no merge needs doing")
		return
	}

	// Get list of all commits
	allCommits, err := GetCommitList(srcOwner, srcDBName)
	if err != nil {
		errType = http.StatusInternalServerError
		return
	}

	// Build path from each commit, source and destination, to the root commit
	s, ok := allCommits[srcCommitID]
	if !ok {
		errType = http.StatusInternalServerError
		err = fmt.Errorf("Could not retrieve details for the source branch commit")
		return
	}
	var sourcePath []CommitEntry
	for {
		sourcePath = append(sourcePath, s)
		if s.Parent == "" {
			break
		}
		s, ok = allCommits[s.Parent]
		if !ok {
			errType = http.StatusInternalServerError
			err = fmt.Errorf("Error when walking the source branch commit list")
			return
		}
	}

	d, ok := allCommits[destCommitID]
	if !ok {
		errType = http.StatusInternalServerError
		err = fmt.Errorf("Could not retrieve details for the destination branch commit")
		return
	}
	var destPath []CommitEntry
	for {
		destPath = append(destPath, d)
		if d.Parent == "" {
			break
		}
		d, ok = allCommits[d.Parent]
		if !ok {
			errType = http.StatusInternalServerError
			err = fmt.Errorf("Error when walking the destination branch commit list")
			return
		}
	}

	// Look for the common ancestor by traversing from the end (the root commit) to the start (the current HEAD) of each
	// commit list and looking for the first commit ID that is different
	for i := 1; ; i++ {
		if i > len(sourcePath) || i > len(destPath) || sourcePath[len(sourcePath)-i].ID != destPath[len(destPath)-i].ID {
			// The last common ancestor is the commit before this one. Since we're working in reverse order here,
			// the commit before this one in time is the one after this one in the array
			if i <= len(sourcePath) {
				ancestorID = sourcePath[len(sourcePath)-i+1].ID
			}

			// Having found the first different commit, build a list of all commits that after it
			for j := len(sourcePath); j >= i; j-- {
				// Exclude merge commits
				if len(sourcePath[len(sourcePath)-j].OtherParents) == 0 {
					commitList = append(commitList, sourcePath[len(sourcePath)-j])
				}
			}

			return
		}
	}

	return
}

// DownloadDatabase returns the SQLite database file to the requester
func DownloadDatabase(w http.ResponseWriter, r *http.Request, dbOwner, dbName, commitID,
	loggedInUser, sourceSw string) (bytesWritten int64, err error) {
	// Check if the database is a live database, and get the node/queue to send requests to
	isLive, liveNode, err := CheckDBLive(dbOwner, dbName)
	if err != nil {
		return
	}

	// Depending on whether this is a live database there's different ways to get a handle
	// to the minio file
	var userDB *minio.Object
	var logStr string
	if isLive {
		// It's a live database, so we tell the job queue backend to back it up into Minio, which we then provide to the user
		err = LiveBackup(liveNode, loggedInUser, dbOwner, dbName)
		if err != nil {
			return
		}

		// Retrieve the Minio bucket info for the database
		var bucket, objectId string
		bucket, objectId, err = LiveGetMinioNames(dbOwner, dbOwner, dbName)
		if err != nil {
			return
		}

		// Open a connection to Minio for the file
		userDB, err = MinioHandle(bucket, objectId)
		if err != nil {
			return
		}

		// Identifier of database for logging
		logStr = fmt.Sprintf("%s/%s", dbOwner, dbName)
	} else {
		// Verify the given database exists and is ok to be downloaded (and get the Minio bucket + id while at it)
		var bucket, id string
		bucket, id, _, err = MinioLocation(dbOwner, dbName, commitID, loggedInUser)
		if err != nil {
			return
		}

		// Get a handle from Minio for the database object
		userDB, err = MinioHandle(bucket, id)
		if err != nil {
			return
		}

		// Identifier of database for logging
		logStr = bucket + id
	}

	// Close the object handle when this function finishes
	defer func() {
		MinioHandleClose(userDB)
	}()

	// Get the file details
	stat, err := userDB.Stat()
	if err != nil {
		return
	}

	// Was a user agent part of the request?
	var userAgent string
	if ua, ok := r.Header["User-Agent"]; ok {
		userAgent = ua[0]
	}

	// Make a record of the download
	err = LogDownload(dbOwner, dbName, loggedInUser, r.RemoteAddr, sourceSw, userAgent, time.Now(), logStr)
	if err != nil {
		return
	}

	// Send the database to the user
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, dbName))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size))
	w.Header().Set("Content-Type", "application/x-sqlite3")
	bytesWritten, err = io.Copy(w, userDB)
	if err != nil {
		log.Printf("Error returning DB file: %v", err)
		return
	}

	// If downloaded by someone other than the owner, increment the download count for the database
	if strings.ToLower(loggedInUser) != strings.ToLower(dbOwner) {
		err = IncrementDownloadCount(dbOwner, dbName)
		if err != nil {
			return
		}
	}

	return
}

// GetCurrentFunctionName returns the name of the function this was called from
func GetCurrentFunctionName() (FuncName string) {
	stk := make([]uintptr, 1)
	runtime.Callers(2, stk[:])
	FuncName = runtime.FuncForPC(stk[0]).Name() + "()"
	return
}

// IsCommitInBranchHistory checks if a given commit ID is in the history of the given branch
func IsCommitInBranchHistory(dbOwner, dbName, branchName, commitID string) (bool, error) {
	// Get the commit list for the database
	commitList, err := GetCommitList(dbOwner, dbName)
	if err != nil {
		return false, err
	}

	branchList, err := GetBranches(dbOwner, dbName)
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
	if c.ID == commitID {
		// The commit was found
		found = true
	}
	for c.Parent != "" {
		c, ok = commitList[c.Parent]
		if !ok {
			log.Printf("Broken commit history encountered for branch '%s' in '%s/%s', when looking for "+
				"commit '%s'", SanitiseLogString(branchName), SanitiseLogString(dbOwner), SanitiseLogString(dbName), c.Parent)
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

// nextChild looks for the next child fork in a fork tree
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
				if !rawList[j].Public && (strings.ToLower(rawList[j].Owner) != strings.ToLower(loggedInUser)) {
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

// RandomString generates a random alphanumeric string of the desired length
func RandomString(length int) string {
	rand.Seed(time.Now().UnixNano())
	const alphaNum = "abcdefghijklmnopqrstuvwxyz0123456789"
	randomString := make([]byte, length)
	for i := range randomString {
		randomString[i] = alphaNum[rand.Intn(len(alphaNum))]
	}
	return string(randomString)
}

// SignalHandler is a background goroutine that exists to catch *nix termination signals then shut the daemon down cleanly
func SignalHandler(done *chan struct{}) {
	// Catch signals
	z := make(chan os.Signal, 1)
	signal.Notify(z, syscall.SIGINT, syscall.SIGTERM)
	sig := <-z
	log.Printf("%s: received signal '%s', shutting down", Conf.Live.Nodename, sig)

	// On non-live nodes, wait for the job response queue to be empty. aka not be waiting on in-flight responses from the live nodes
	if ResponseQueue != nil {
		loop := 0
		ResponseQueue.RLock()
		queueLength := len(ResponseQueue.receivers)
		ResponseQueue.RUnlock()
		for queueLength > 0 {
			log.Printf("%s: response queue not empty (%d), waiting for 1/2 second then trying again", Conf.Live.Nodename, queueLength)
			time.Sleep(500 * time.Millisecond)
			loop++
			if loop >= 20 {
				log.Printf("%s: response queue not empty (%d) after 10 seconds.  Exiting anyway", Conf.Live.Nodename, queueLength)
				break
			}
			ResponseQueue.RLock()
			queueLength = len(ResponseQueue.receivers)
			ResponseQueue.RUnlock()
			if queueLength == 0 {
				log.Printf("%s: response queue now empty, shutting down", Conf.Live.Nodename)
			}
		}
	}

	// Shut down connections
	DisconnectPostgreSQL()

	// The application is ready to exit
	*done <- struct{}{}
	return
}

// StatusUpdateCheck checks if a status update for the user exists for a given discussion or MR, and if so then removes it
func StatusUpdateCheck(dbOwner, dbName string, thisID int, userName string) (numStatusUpdates int, err error) {
	var lst map[string][]StatusUpdateEntry
	lst, err = StatusUpdates(userName)
	if err != nil {
		return
	}
	db := fmt.Sprintf("%s/%s", dbOwner, dbName)
	b, ok := lst[db]
	if ok {
		for i, j := range b {
			if j.DiscID == thisID {
				// Delete the matching status update
				b = append(b[:i], b[i+1:]...)
				if len(b) > 0 {
					lst[db] = b
				} else {
					delete(lst, db)
				}

				// Store the updated list for the user
				err = StoreStatusUpdates(userName, lst)
				if err != nil {
					return
				}

				// Update the status updates # stored in memcached
				for _, z := range lst {
					numStatusUpdates += len(z)
				}
				err = SetUserStatusUpdates(userName, numStatusUpdates)
				if err != nil {
					log.Printf("Error when updating user status updates # in memcached: %v", err)
					return
				}
				return
			}
		}
	}

	// Return the # of status updates for the user
	for _, z := range lst {
		numStatusUpdates += len(z)
	}
	return
}

// WriteDBtoDisk gets an uploaded database file from the user's incoming request, and writes it to a local temporary file
func WriteDBtoDisk(loggedInUser, dbOwner, dbName string, newDB io.Reader) (numBytes int64, tempDB *os.File, sha string, sTbls []string, err error) {
	// Create a temporary file to store the database in
	tempDB, err = os.CreateTemp(Conf.DiskCache.Directory, "dbhub-upload-")
	if err != nil {
		log.Printf("Error creating temporary file. User: '%s', Database: '%s/%s', Error: %v", loggedInUser,
			SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
		return
	}
	tempDBName := tempDB.Name()

	// Write the database to the temporary file, so we can try opening it with SQLite to verify it's ok
	bufSize := 16 << 22 // 64MB
	buf := make([]byte, bufSize)
	numBytes, err = io.CopyBuffer(tempDB, newDB, buf)
	if err != nil {
		log.Printf("Error when writing the uploaded db to a temp file. User: '%s', Database: '%s/%s' "+
			"Error: %v", loggedInUser, SanitiseLogString(dbOwner), SanitiseLogString(dbName), err)
		return
	}
	if numBytes == 0 {
		err = errors.New("Copying file failed")
		return
	}

	// Sanity check the uploaded database, and get the list of tables in the database
	sTbls, err = SQLiteSanityCheck(tempDBName)
	if err != nil {
		return
	}

	// Return to the start of the temporary file
	var newOffSet int64
	newOffSet, err = tempDB.Seek(0, 0)
	if err != nil {
		log.Printf("Seeking on the temporary file failed: %s", err.Error())
		return
	}
	if newOffSet != 0 {
		err = errors.New("Seeking to the start of the temporary file failed")
		return
	}

	// Generate sha256 of the uploaded file
	s := sha256.New()
	_, err = io.CopyBuffer(s, tempDB, buf)
	if err != nil {
		return
	}
	sha = hex.EncodeToString(s.Sum(nil))
	return
}
