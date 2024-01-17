package common

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/sqlitebrowser/dbhub.io/common/config"
	"github.com/sqlitebrowser/dbhub.io/common/database"

	sqlite "github.com/gwenn/gosqlite"
)

// Merge merges the commits in commitDiffList into the destination branch destBranch of the given database
func Merge(destOwner, destName, destBranch, srcOwner, srcName string, commitDiffList []database.CommitEntry, message, loggedInUser string) (newCommitID string, err error) {
	// Get the details of the head commit for the destination database branch
	branchList, err := database.GetBranches(destOwner, destName) // Destination branch list
	if err != nil {
		return
	}
	branchDetails, ok := branchList[destBranch]
	if !ok {
		return "", fmt.Errorf("Could not retrieve details for the destination branch")
	}
	destCommitID := branchDetails.Commit

	// Check if the MR commits will still apply cleanly to the destination branch so we can fast-forward
	finalCommit := commitDiffList[len(commitDiffList)-1]
	fastForwardPossible := finalCommit.Parent == destCommitID

	// If fast-forwarding is possible just add a merge commit and save the new commit list.
	// If it is not possible save the source commits and perform the actual merging which creates its own merge commit.
	if fastForwardPossible {
		// We can fast-forward. So simply add a merge commit on top of the just added source commits and save
		// the new commit list and branch details.

		newCommitID, err = performFastForward(destOwner, destName, destBranch, destCommitID, commitDiffList, message, loggedInUser)
		if err != nil {
			return
		}
	} else {
		// We cannot fast-forward. This means we have to perform an actual merge. A merge commit is automatically created
		// by the performMerge() function so we do not have to worry about that.
		// Perform merge
		newCommitID, err = performMerge(destOwner, destName, destBranch, destCommitID, srcOwner, srcName, commitDiffList, message, loggedInUser)
		if err != nil {
			return
		}
	}

	return
}

// addCommitsForMerging simply adds the commits listed in commitDiffList to the destination branch of the databases.
// It neither performs any merging nor does it create a merge commit.
func addCommitsForMerging(destOwner, destName, destBranch string, commitDiffList []database.CommitEntry, newHead bool) (err error) {
	// Get the details of the head commit for the destination database branch
	branchList, err := database.GetBranches(destOwner, destName) // Destination branch list
	if err != nil {
		return err
	}
	branchDetails, ok := branchList[destBranch]
	if !ok {
		return fmt.Errorf("Could not retrieve details for the destination branch")
	}

	// Get destination commit list
	destCommitList, err := database.GetCommitList(destOwner, destName)
	if err != nil {
		return err
	}

	// Add the source commits directly to the destination commit list
	for _, j := range commitDiffList {
		destCommitList[j.ID] = j
	}

	// New head commit id
	var newHeadCommitId string
	if newHead {
		newHeadCommitId = commitDiffList[0].ID
	} else {
		newHeadCommitId = branchDetails.Commit
	}

	// Update the branch list
	b := database.BranchEntry{
		Commit:      newHeadCommitId,
		CommitCount: branchDetails.CommitCount + len(commitDiffList),
		Description: branchDetails.Description,
	}
	branchList[destBranch] = b
	err = database.StoreCommits(destOwner, destName, destCommitList)
	if err != nil {
		return err
	}
	err = database.StoreBranches(destOwner, destName, branchList)
	if err != nil {
		return err
	}

	return
}

// performFastForward performs a merge by simply fast-forwarding to a new head.
func performFastForward(destOwner, destName, destBranch, destCommitID string, commitDiffList []database.CommitEntry, message, loggedInUser string) (newCommitID string, err error) {
	// Retrieve details for the logged in user
	usr, err := database.User(loggedInUser)
	if err != nil {
		return
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

	// Add the merge commit to the list of new commits to be added to the destination branch
	var newCommitDiffList []database.CommitEntry
	newCommitDiffList = append(newCommitDiffList, mrg)
	newCommitDiffList = append(newCommitDiffList, commitDiffList...)

	// Add the source commits and the new merge commit to the destination db commit list and update the branch list with it
	err = addCommitsForMerging(destOwner, destName, destBranch, newCommitDiffList, true)
	if err != nil {
		return
	}

	return mrg.ID, nil
}

// performMerge takes the destination database and applies the changes from commitDiffList on it.
func performMerge(destOwner, destName, destBranch, destCommitID, srcOwner, srcName string, commitDiffList []database.CommitEntry, message, loggedInUser string) (newCommitID string, err error) {
	// Figure out the last common ancestor and the current head of the branch to merge
	lastCommonAncestorId := commitDiffList[len(commitDiffList)-1].Parent
	currentHeadToMerge := commitDiffList[0].ID

	// Figure out the changes made to the destination branch since this common ancestor.
	// For this we don't need any SQLs generated because this information is only required
	// for checking for conflicts.
	destDiffs, err := Diff(destOwner, destName, lastCommonAncestorId, destOwner, destName, destCommitID, loggedInUser, NoMerge, false)
	if err != nil {
		return
	}

	// Figure out the changes made to the source branch since this common ancestor.
	// For this we do want SQLs generated because these need to be applied on top of
	// the destination branch head.
	srcDiffs, err := Diff(srcOwner, srcName, lastCommonAncestorId, srcOwner, srcName, currentHeadToMerge, loggedInUser, NewPkMerge, false)
	if err != nil {
		return
	}

	// Check for conflicts
	conflicts := checkForConflicts(srcDiffs, destDiffs, NewPkMerge)
	if conflicts != nil {
		// TODO We don't have developed an intelligent conflict strategy yet.
		// So in the case of a conflict, just abort with an error message.
		return "", fmt.Errorf("The two branches are in conflict. Please fix this manually.\n" + strings.Join(conflicts, "\n"))
	}

	// Get Minio location
	bucket, id, _, err := MinioLocation(destOwner, destName, destCommitID, loggedInUser)
	if err != nil {
		return
	}

	// Sanity check
	if id == "" {
		// The requested database wasn't found, or the user doesn't have permission to access it
		return "", fmt.Errorf("Requested database not found")
	}

	// Retrieve database file from Minio, using locally cached version if it's already there
	dbFile, err := RetrieveDatabaseFile(bucket, id)
	if err != nil {
		return
	}

	// Create a temporary file for the new database
	tmpFile, err := os.CreateTemp(config.Conf.DiskCache.Directory, "dbhub-merge-*.db")
	if err != nil {
		return
	}

	// Delete the file when we are done
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Copy destination database to temporary location
	err = func() (err error) {
		inFile, err := os.Open(dbFile)
		if err != nil {
			return
		}
		defer inFile.Close()
		_, err = io.Copy(tmpFile, inFile)
		if err != nil {
			return
		}

		return
	}()
	if err != nil {
		return
	}

	// Open temporary database file for writing
	err = func() (err error) {
		var sdb *sqlite.Conn
		sdb, err = sqlite.Open(tmpFile.Name(), sqlite.OpenReadWrite)
		if err != nil {
			return
		}
		defer sdb.Close()
		if err = sdb.EnableExtendedResultCodes(true); err != nil {
			return
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
	}()
	if err != nil {
		return
	}

	// Retrieve details for the logged in user
	usr, err := database.User(loggedInUser)
	if err != nil {
		return
	}

	// Seek to start of temporary file. When not doing this AddDatabase() cannot copy the file
	_, err = tmpFile.Seek(0, 0)
	if err != nil {
		return
	}

	// The merging was successful. This means we can add the list of source commits to the destination branch.
	// This needs to be done before calling AddDatabase() because AddDatabase() adds its own merge commit on
	// top of the just updated commit list.
	err = addCommitsForMerging(destOwner, destName, destBranch, commitDiffList, false)
	if err != nil {
		return
	}

	// Store merged database
	_, newCommitID, _, err = AddDatabase(loggedInUser, destOwner, destName, false, destBranch, destCommitID,
		database.KeepCurrentAccessType, "", message, "", tmpFile, time.Now(), time.Time{}, usr.DisplayName, usr.Email, usr.DisplayName, usr.Email,
		[]string{currentHeadToMerge}, "")
	if err != nil {
		return
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

							// TODO For two UPDATE statements we could check whether they only change different
							// columns and allow them in this case.

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
