package common

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"time"

	sqlite "github.com/gwenn/gosqlite"
)

// Merge merges the commits in commitDiffList into the destination branch destBranch of the given database
func Merge(destOwner string, destFolder string, destName string, destBranch string, srcOwner string, srcFolder string, srcName string, commitDiffList []CommitEntry, message string, loggedInUser string) (err error) {
	// Get the details of the head commit for the destination database branch
	branchList, err := GetBranches(destOwner, destFolder, destName) // Destination branch list
	if err != nil {
		return err
	}
	branchDetails, ok := branchList[destBranch]
	if !ok {
		err = fmt.Errorf("Could not retrieve details for the destination branch")
		return
	}
	destCommitID := branchDetails.Commit

	// Check if the MR commits will still apply cleanly to the destination branch so we can fast-forward
	finalCommit := commitDiffList[len(commitDiffList)-1]
	fastForwardPossible := finalCommit.Parent == destCommitID

	// If fast forwarding doesn't work we need to perform an actual merge of the branch heads
	if !fastForwardPossible {
		// Perform merge
		err = performMerge(destOwner, destFolder, destName, destCommitID, srcOwner, srcFolder, srcName, commitDiffList, loggedInUser)
		if err != nil {
			return
		}

		// TODO If the merge is actually successful, stop anyway. This is because storing the resulting
		// database and creating a proper merge commit isn't implemented yet.
		err = fmt.Errorf("Merging other than by fast-forwarding is not yet implemented")
		return
	}

	// Get destination commit list
	destCommitList, err := GetCommitList(destOwner, destFolder, destName)
	if err != nil {
		return err
	}

	// Add the source commits directly to the destination commit list
	for _, j := range commitDiffList {
		destCommitList[j.ID] = j
	}

	// Retrieve details for the logged in user
	usr, err := User(loggedInUser)
	if err != nil {
		return err
	}

	// Create a merge commit, using the details of the source commit (this gets us a correctly filled in DB tree
	// structure easily)
	mrg := commitDiffList[0]
	mrg.AuthorEmail = usr.Email
	mrg.AuthorName = usr.DisplayName
	mrg.Message = message
	mrg.Parent = commitDiffList[0].ID
	mrg.OtherParents = append(mrg.OtherParents, destCommitID)
	mrg.Timestamp = time.Now().UTC()
	mrg.ID = CreateCommitID(mrg)

	// Add the new commit to the destination db commit list, and update the branch list with it
	destCommitList[mrg.ID] = mrg
	b := BranchEntry{
		Commit:      mrg.ID,
		CommitCount: branchDetails.CommitCount + len(commitDiffList) + 1,
		Description: branchDetails.Description,
	}
	branchList[destBranch] = b
	err = StoreCommits(destOwner, destFolder, destName, destCommitList)
	if err != nil {
		return err
	}
	err = StoreBranches(destOwner, destFolder, destName, branchList)
	if err != nil {
		return err
	}

	return
}

// performMerge takes the destination database and applies the changes from commitDiffList on it.
func performMerge(destOwner string, destFolder string, destName string, destCommitID string, srcOwner string, srcFolder string, srcName string, commitDiffList []CommitEntry, loggedInUser string) (err error) {
	// Figure out the last common ancestor and the current head of the branch to merge
	lastCommonAncestorId := commitDiffList[len(commitDiffList)-1].Parent
	currentHeadToMerge := commitDiffList[0].ID

	// Figure out the changes made to the destination branch since this common ancestor.
	// For this we don't need any SQLs generated because this information is only required
	// for checking for conflicts.
	destDiffs, err := Diff(destOwner, destFolder, destName, lastCommonAncestorId, destOwner, destFolder, destName, destCommitID, loggedInUser, NoMerge, false)
	if err != nil {
		return err
	}

	// Figure out the changes made to the source branch since this common ancestor.
	// For this we do want SQLs generated because these need to be applied on top of
	// the destination branch head.
	srcDiffs, err := Diff(srcOwner, srcFolder, srcName, lastCommonAncestorId, srcOwner, srcFolder, srcName, currentHeadToMerge, loggedInUser, NewPkMerge, false)
	if err != nil {
		return err
	}

	// Check for conflicts
	conflicts := checkForConflicts(srcDiffs, destDiffs, NewPkMerge)
	if conflicts != nil {
		// TODO We don't have developed an intelligent conflict strategy yet.
		// So in the case of a conflict, just abort with an error message.
		return fmt.Errorf("The two branches are in conflict. Please fix this manually.\n" + strings.Join(conflicts, "\n"))
	}

	// Get Minio location
	bucket, id, _, err := MinioLocation(destOwner, destFolder, destName, destCommitID, loggedInUser)
	if err != nil {
		return
	}

	// Sanity check
	if id == "" {
		// The requested database wasn't found, or the user doesn't have permission to access it
		return fmt.Errorf("Requested database not found")
	}

	// Retrieve database file from Minio, using locally cached version if it's already there
	dbFile, err := RetrieveDatabaseFile(bucket, id)
	if err != nil {
		return
	}

	// Create a temporary file for the new database
	tmpFile, err := ioutil.TempFile(os.TempDir(), "merge-*.db")
	if err != nil {
		return
	}

	// Delete the file when we are done
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Copy destination database to temporary location
	{
		inFile, err := os.Open(dbFile)
		if err != nil {
			return err
		}
		defer inFile.Close()
		_, err = io.Copy(tmpFile, inFile)
		if err != nil {
			return err
		}
	}

	// Open temporary database file for writing
	var sdb *sqlite.Conn
	sdb, err = sqlite.Open(tmpFile.Name(), sqlite.OpenReadWrite)
	if err != nil {
		return err
	}
	defer sdb.Close()
	if err = sdb.EnableExtendedResultCodes(true); err != nil {
		return err
	}

	// Apply all the SQL statements from the diff on the temporary database
	for _, diff := range srcDiffs.Diff {
		// First apply schema changes
		if diff.Schema != nil {
			err = sdb.Exec(diff.Schema.Sql)
			if err != nil {
				return
			}
		}

		// Then apply data changes
		for _, row := range diff.Data {
			err = sdb.Exec(row.Sql)
			if err != nil {
				return
			}
		}
	}

	return
}

// checkForConflicts takes two diff changesets and checks whether they are compatible or not.
// Compatible changesets don't change the same objects or rows and thus can be combined without
// side effects. The function returns an empty slice if there are no conflicts. If there are
// conflicts the returned slice contains a list of the detected conflicts.
func checkForConflicts(srcDiffs Diffs, destDiffs Diffs, mergeStrategy MergeStrategy) (conflicts []string) {
	// Check if an object in the source diff is also part of the destination diff
	for _, srcDiff := range srcDiffs.Diff {
		for _, destDiff := range destDiffs.Diff {
			// Check if the object names are the same
			if srcDiff.ObjectName == destDiff.ObjectName {
				// If the schema of this object has changed in one of the branches, this is
				// a conflict we cannot solve
				if srcDiff.Schema != nil || destDiff.Schema != nil {
					conflicts = append(conflicts, "Schema for "+srcDiff.ObjectName+" has changed")

					// No need to look further in this case
					break
				}

				// Check if there are any changed rows with the same primary key
				for _, srcRow := range srcDiff.Data {
					for _, destRow := range destDiff.Data {
						if DataValuesMatch(srcRow.Pk, destRow.Pk) {
							// We have found two changes which affect the same primary key. So this is a potential
							// conflict. The question now is whether it is actually a problem or not.

							// Every combination of updates, inserts, and deletes is a conflict except for the
							// case where the source row is inserted using the NewPkMerge strategy which generates
							// a new primary key which doesn't conflict.
							if !(srcRow.ActionType == "add" && mergeStrategy == NewPkMerge) {
								// Generate and add conflict description
								conflictString := "Conflict in " + srcDiff.ObjectName + " for "
								for _, pk := range srcRow.Pk {
									conflictString += pk.Name + "=" + pk.Value.(string) + ","
								}
								conflicts = append(conflicts, strings.TrimSuffix(conflictString, ","))
							}

							// No need to look through the rest of the destination rows
							break
						}
					}
				}

				// No need to look through the remaining destination diff items.
				// Just continue with the next source diff item.
				break
			}
		}
	}

	return
}
