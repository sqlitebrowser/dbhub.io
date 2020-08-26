package common

import (
	"fmt"
	"time"
)

// Merge merges the commits in commitDiffList into the destination branch destBranch of the given database
func Merge(dbOwner string, dbFolder string, dbName string, destBranch string, commitDiffList []CommitEntry, message string, loggedInUser string) (err error) {
	// Get the details of the head commit for the destination database branch
	branchList, err := GetBranches(dbOwner, dbFolder, dbName) // Destination branch list
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

	// TODO Actual merging is not yet implemented
	if !fastForwardPossible {
		err = fmt.Errorf("Merging other than by fast-forwarding is not yet implemented")
		return
	}

	// Get destination commit list
	destCommitList, err := GetCommitList(dbOwner, dbFolder, dbName)
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
	err = StoreCommits(dbOwner, dbFolder, dbName, destCommitList)
	if err != nil {
		return err
	}
	err = StoreBranches(dbOwner, dbFolder, dbName, branchList)
	if err != nil {
		return err
	}

	return
}
