package main

import (
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	com "github.com/sqlitebrowser/dbhub.io/common"
	gfm "github.com/sqlitebrowser/github_flavored_markdown"
)

// Renders the "About Us" page.
func aboutPage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		PageMeta PageMetaInfo
	}

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}
	pageData.PageMeta.Title = "What is DBHub.io?"

	// Render the page
	t := tmpl.Lookup("aboutPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Render the branches page, which lists the branches for a database.
func branchesPage(w http.ResponseWriter, r *http.Request) {
	// Structure to hold page data
	var pageData struct {
		Branches map[string]com.BranchEntry
		DB       com.SQLiteDBinfo
		PageMeta PageMetaInfo
	}
	pageData.PageMeta.Title = "Branch list"
	pageData.PageMeta.PageSection = "db_data"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}
	dbName, err := getDatabaseName(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Read the branch heads list from the database
	pageData.Branches, err = com.GetBranches(dbName.Owner, dbName.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Render the page
	t := tmpl.Lookup("branchesPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Render the commits page.  This shows all of the commits in a given branch, in reverse order from newest to oldest.
func commitsPage(w http.ResponseWriter, r *http.Request) {
	// Structure to hold page data
	type HistEntry struct {
		AuthorEmail    string     `json:"author_email"`
		AuthorName     string     `json:"author_name"`
		AuthorUserName string     `json:"author_user_name"`
		AvatarURL      string     `json:"avatar_url"`
		CommitterEmail string     `json:"committer_email"`
		CommitterName  string     `json:"committer_name"`
		ID             string     `json:"id"`
		Message        string     `json:"message"`
		Parent         string     `json:"parent"`
		Timestamp      time.Time  `json:"timestamp"`
		Tree           com.DBTree `json:"tree"`
	}
	var pageData struct {
		Branches map[string]com.BranchEntry
		DB       com.SQLiteDBinfo
		History  []HistEntry
		PageMeta PageMetaInfo
	}

	pageData.PageMeta.Title = "Commits"
	pageData.PageMeta.PageSection = "db_data"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}
	dbName, err := getDatabaseName(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Retrieve the branch name
	branchName, err := com.GetFormBranch(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if the user has access to the requested database (and get its details if available)
	err = com.DBDetails(&pageData.DB, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Read the branch heads list from the database
	pageData.Branches, err = com.GetBranches(dbName.Owner, dbName.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// If no branch name was given, we use the default branch
	if branchName == "" {
		branchName = pageData.DB.Info.DefaultBranch
	}

	// Work out the head commit ID for the requested branch
	headCom, ok := pageData.Branches[branchName]
	if !ok {
		// Unknown branch
		errorPage(w, r, http.StatusInternalServerError, fmt.Sprintf("Branch '%s' not found", branchName))
		return
	}
	headID := headCom.Commit
	if headID == "" {
		// The requested branch wasn't found.  Bad request?
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Walk the commit history backwards from the head commit, assembling the commit history for this branch from the
	// full list
	rawList, err := com.GetCommitList(dbName.Owner, dbName.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// To create the commit history we need to follow both, the parent commit id and the other parents commit ids,
	// to include merged commits. When following a merged branch we do however end up with the regular commits at
	// some point. In this example the first line is what you get by following the parent ids. We also want to
	// include the second line which we get by following c8's other parent's id.
	// c1 -> c2 -> c3 -> c4 -> c5 -> c8
	//               \-> c6 -> c7 /
	// However, we don't want c1 and c2 to be included twice. This is why we assemble a list of all regular parent
	// commit ids first as a look-up table for knowing when to stop traversing the other branches of the tree.
	regularBranchCommitIds := map[string]bool{}
	commitData := com.CommitEntry{Parent: rawList[headID].Parent}
	for commitData.Parent != "" {
		commitData, ok = rawList[commitData.Parent]
		if !ok {
			errorPage(w, r, http.StatusInternalServerError, "Internal error when retrieving commit data")
			return
		}

		regularBranchCommitIds[commitData.ID] = true
	}

	// This function recursively follows all branches of the tree
	var traverseTree func(string, bool) (err error)
	traverseTree = func(id string, stopAtRegularBranch bool) (err error) {
		for id != "" {
			// If we want to stop at the regular branch check if this commit id is a known regular branch
			// commit id. If so return here
			if stopAtRegularBranch {
				_, ok = regularBranchCommitIds[id]
				if ok {
					return
				}

				// Add this commit id to the list of known commit ids to stop at. Just to be sure
				// to avoid double commits in messes up commit histories.
				// TODO Maybe remove this when we are able to display an actual tree structure.
				regularBranchCommitIds[id] = true
			}

			// TODO: Ugh, this is an ugly approach just to add the username to the commit data.  Surely there's a better way?
			// TODO  Maybe store the username in the commit data structure in the database instead?
			// TODO: Display licence changes too
			commit, ok := rawList[id]
			if !ok {
				return fmt.Errorf("Internal error when retrieving commit data")
			}
			uName, avatarURL, err := com.GetUsernameFromEmail(commit.AuthorEmail)
			if err != nil {
				return err
			}
			if avatarURL != "" {
				avatarURL += "&s=30"
			}

			// Create a history entry
			newEntry := HistEntry{
				AuthorEmail:    commit.AuthorEmail,
				AuthorName:     commit.AuthorName,
				AuthorUserName: uName,
				AvatarURL:      avatarURL,
				CommitterEmail: commit.CommitterEmail,
				CommitterName:  commit.CommitterName,
				ID:             commit.ID,
				Message:        string(gfm.Markdown([]byte(commit.Message))),
				Parent:         commit.Parent,
				Timestamp:      commit.Timestamp,
			}
			pageData.History = append(pageData.History, newEntry)

			// Follow the other parents if there are any
			for _, v := range commit.OtherParents {
				traverseTree(v, true)
			}

			id = commit.Parent
		}

		return
	}

	// Create the history list
	err = traverseTree(headID, false)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Render the page
	t := tmpl.Lookup("commitsPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Render the compare page, for creating new merge requests
func comparePage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		CommitList            []com.CommitData
		DB                    com.SQLiteDBinfo
		DestDBBranches        []string
		DestDBDefaultBranch   string
		DestDBName            string
		DestOwner             string
		Forks                 []com.ForkEntry
		PageMeta              PageMetaInfo
		SourceDBBranches      []string
		SourceDBDefaultBranch string
		SourceDBName          string
		SourceOwner           string
	}
	pageData.PageMeta.Title = "Create a Merge Request"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}
	dbName, err := getDatabaseName(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Require login
	errCode, err = requireLogin(pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Retrieve list of forks for the database
	pageData.Forks, err = com.ForkTree(pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError,
			fmt.Sprintf("Error retrieving fork list for '%s/%s': %v\n", dbName.Owner, dbName.Database, err.Error()))
		return
	}

	// Use the database which the "New Merge Request" button was pressed on as the initially selected source
	pageData.SourceOwner = dbName.Owner
	pageData.SourceDBName = dbName.Database

	// If the source database has an (accessible) parent, use that as the default destination selected for the user.
	// If it doesn't, then set the source as the destination as well and the user will have to manually choose
	pageData.DestOwner, pageData.DestDBName, err = com.ForkParent(pageData.PageMeta.LoggedInUser, dbName.Owner,
		dbName.Database)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if pageData.DestOwner == "" || pageData.DestDBName == "" {
		pageData.DestOwner = dbName.Owner
		pageData.DestDBName = dbName.Database
	}

	// * Determine the source and destination database branches *

	// Retrieve the branch info for the source database
	srcBranchList, err := com.GetBranches(dbName.Owner, dbName.Database)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	for name := range srcBranchList {
		pageData.SourceDBBranches = append(pageData.SourceDBBranches, name)
	}
	pageData.SourceDBDefaultBranch = pageData.DB.Info.DefaultBranch

	// Retrieve the branch info for the destination database
	destBranchList, err := com.GetBranches(pageData.DestOwner, pageData.DestDBName)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	for name := range destBranchList {
		pageData.DestDBBranches = append(pageData.DestDBBranches, name)
	}
	pageData.DestDBDefaultBranch, err = com.GetDefaultBranchName(pageData.DestOwner,
		pageData.DestDBName)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// If the initially chosen source and destinations can be directly applied, fill out the initial commit list entries
	// for display to the user
	ancestorID, cList, errType, err := com.GetCommonAncestorCommits(dbName.Owner, dbName.Database,
		pageData.SourceDBDefaultBranch, pageData.DestOwner, pageData.DestDBName,
		pageData.DestDBDefaultBranch)
	if err != nil && errType != http.StatusBadRequest {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if ancestorID != "" {
		// Retrieve the commit ID for the destination branch
		destBranch, ok := destBranchList[pageData.DestDBDefaultBranch]
		if !ok {
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}
		}
		destCommitID := destBranch.Commit

		// Retrieve the current licence for the destination branch
		commitList, err := com.GetCommitList(pageData.DestOwner, pageData.DestDBName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		destCommit, ok := commitList[destCommitID]
		if !ok {
			errorPage(w, r, http.StatusInternalServerError, "Destination commit ID not found in commit list.")
			return
		}
		destLicenceSHA := destCommit.Tree.Entries[0].LicenceSHA

		// Convert the commit entries into something we can display in a commit list
		for _, j := range cList {
			var c com.CommitData
			c.AuthorEmail = j.AuthorEmail
			c.AuthorName = j.AuthorName
			c.ID = j.ID
			c.Parent = j.Parent
			c.Message = j.Message
			c.Timestamp = j.Timestamp
			c.AuthorUsername, c.AuthorAvatar, err = com.GetUsernameFromEmail(j.AuthorEmail)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}
			if c.AuthorAvatar != "" {
				c.AuthorAvatar += "&s=18"
			}

			// Check for licence changes
			commitLicSHA := j.Tree.Entries[0].LicenceSHA
			if commitLicSHA != destLicenceSHA {
				lName, _, err := com.GetLicenceInfoFromSha256(dbName.Owner, commitLicSHA)
				if err != nil {
					errorPage(w, r, http.StatusInternalServerError, err.Error())
					return
				}
				c.LicenceChange = fmt.Sprintf("This commit includes a licence change to '%s'", lName)
			}
			pageData.CommitList = append(pageData.CommitList, c)
		}
	}

	// Render the page
	pageData.PageMeta.PageSection = "db_merge"
	t := tmpl.Lookup("comparePage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Render the contributors page, which lists the contributors to a database.
func contributorsPage(w http.ResponseWriter, r *http.Request) {
	// Structures to hold page data
	type AuthorEntry struct {
		AuthorEmail    string `json:"author_email"`
		AuthorName     string `json:"author_name"`
		AuthorUserName string `json:"author_user_name"`
		AvatarURL      string `json:"avatar_url"`
		NumCommits     int    `json:"num_commits"`
	}
	var pageData struct {
		Contributors map[string]AuthorEntry
		DB           com.SQLiteDBinfo
		PageMeta     PageMetaInfo
	}
	pageData.PageMeta.Title = "Contributors"
	pageData.PageMeta.PageSection = "db_data"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}
	dbName, err := getDatabaseName(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Read the commit list from the database
	commitList, err := com.GetCommitList(dbName.Owner, dbName.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Fill out the metadata
	pageData.Contributors = make(map[string]AuthorEntry)
	for _, j := range commitList {
		// Look up the author's username
		// TODO: There are likely a bunch of ways to optimise this, from keeping the user name entries in a map to
		// TODO  directly storing the username in the jsonb commit data.  Storing the user name entry in the jsonb is
		// TODO  probably the way to go, as it would save lookups in a lot of places
		u, avatarURL, err := com.GetUsernameFromEmail(j.AuthorEmail)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if avatarURL != "" {
			avatarURL += "&s=30"
		}

		// This ok check is just a way to decide whether to increment the NumCommits counter
		if _, ok := pageData.Contributors[j.AuthorName]; !ok {
			// This is the first time in the loop we're adding the author to the Contributors list
			pageData.Contributors[j.AuthorName] = AuthorEntry{
				AuthorEmail:    j.AuthorEmail,
				AuthorName:     j.AuthorName,
				AuthorUserName: u,
				AvatarURL:      avatarURL,
				NumCommits:     1,
			}
		} else {
			// The author is already in the contributors list, so we increment their NumCommits counter
			n := pageData.Contributors[j.AuthorName].NumCommits + 1
			pageData.Contributors[j.AuthorName] = AuthorEntry{
				AuthorEmail:    j.AuthorEmail,
				AuthorName:     j.AuthorName,
				AuthorUserName: u,
				AvatarURL:      avatarURL,
				NumCommits:     n,
			}
		}
	}

	// Render the page
	t := tmpl.Lookup("contributorsPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Displays a web page asking for the new branch name.
func createBranchPage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		DB       com.SQLiteDBinfo
		PageMeta PageMetaInfo
		Commit   string
	}
	pageData.PageMeta.Title = "Create new branch"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}
	dbName, err := getDatabaseName(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Require login
	errCode, err = requireLogin(pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Retrieve the commit ID
	commit, err := com.GetFormCommit(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Make sure the logged in user has the permissions to proceed
	allowed, err := com.CheckDBPermissions(pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, true)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if allowed == false {
		errorPage(w, r, http.StatusUnauthorized, "You are not authorised to change this database")
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Fill out metadata for the page to be rendered
	pageData.Commit = commit

	// Render the page
	t := tmpl.Lookup("createBranchPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Displays a web page to input information needed for creating a new discussion.
func createDiscussionPage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		DB       com.SQLiteDBinfo
		PageMeta PageMetaInfo
	}
	pageData.PageMeta.Title = "Create new discussion"
	pageData.PageMeta.PageSection = "db_disc"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}
	dbName, err := getDatabaseName(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Require login
	errCode, err = requireLogin(pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Render the page
	t := tmpl.Lookup("createDiscussionPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Displays a web page asking for the new tag details.
func createTagPage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		DB       com.SQLiteDBinfo
		PageMeta PageMetaInfo
		Commit   string
	}
	pageData.PageMeta.Title = "Create new tag"

	// Retrieve the commit ID
	commit, err := com.GetFormCommit(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Validation failed for commit value")
		return
	}

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}
	dbName, err := getDatabaseName(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Require login
	errCode, err = requireLogin(pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Make sure the logged in user has the permissions to proceed
	allowed, err := com.CheckDBPermissions(pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, true)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if allowed == false {
		errorPage(w, r, http.StatusUnauthorized, "You are not authorised to change this database")
		return
	}

	// Fill out metadata for the page to be rendered
	pageData.Commit = commit

	// Render the page
	t := tmpl.Lookup("createTagPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func databasePage(w http.ResponseWriter, r *http.Request, dbOwner string, dbName string) {
	var pageData struct {
		DB           com.SQLiteDBinfo
		PageMeta     PageMetaInfo
		DB4S         com.DB4SInfo
		WriteEnabled bool
	}

	pageData.PageMeta.PageSection = "db_data"
	pageData.DB4S = com.Conf.DB4S

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Check if the database exists and the user has access to view it
	exists, err := com.CheckDBPermissions(pageData.PageMeta.LoggedInUser, dbOwner, dbName, false)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s/%s' doesn't exist", dbOwner, dbName))
		return
	}

	// Figure out the correct commit ID from the provided tag, branch, release name or commit id
	// For live databases these do not exist yet, so this step is skipped.
	var commitID string
	branchHeads := make(map[string]com.BranchEntry)
	if !pageData.DB.Info.IsLive {
		// Check if a specific database commit ID was given
		commitID, err = com.GetFormCommit(r)
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, "Invalid database commit ID")
			return
		}

		// Check if a branch name was requested
		branchName, err := com.GetFormBranch(r)
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, "Validation failed for branch name")
			return
		}

		// Check if a named tag was requested
		tagName, err := com.GetFormTag(r)
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, "Validation failed for tag name")
			return
		}

		// Check if a specific release was requested
		releaseName := r.FormValue("release")
		if releaseName != "" {
			err = com.ValidateBranchName(releaseName)
			if err != nil {
				errorPage(w, r, http.StatusBadRequest, "Validation failed for release name")
				return
			}
		}

		// If a specific commit was requested, make sure it exists in the database commit history
		if commitID != "" {
			commitList, err := com.GetCommitList(dbOwner, dbName)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}
			if _, ok := commitList[commitID]; !ok {
				// The requested commit isn't one in the database commit history so error out
				errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Unknown commit for database '%s/%s'", dbOwner,
					dbName))
				return
			}
		}

		// If a specific release was requested, and no commit ID was given, retrieve the commit ID matching the release
		if commitID == "" && releaseName != "" {
			releases, err := com.GetReleases(dbOwner, dbName)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve releases for database")
				return
			}
			rls, ok := releases[releaseName]
			if !ok {
				errorPage(w, r, http.StatusInternalServerError, "Unknown release requested for this database")
				return
			}
			commitID = rls.Commit
		}

		// Load the branch info for the database
		branchHeads, err = com.GetBranches(dbOwner, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve branch information for database")
			return
		}

		// If a specific branch was requested and no commit ID was given, use the latest commit for the branch
		if commitID == "" && branchName != "" {
			c, ok := branchHeads[branchName]
			if !ok {
				errorPage(w, r, http.StatusInternalServerError, "Unknown branch requested for this database")
				return
			}
			commitID = c.Commit
		}

		// If a specific tag was requested, and no commit ID was given, retrieve the commit ID matching the tag
		// TODO: If we need to reduce database calls, we can probably make a function merging this, GetBranches(), and
		// TODO  GetCommitList() above.  Potentially also the DBDetails() call below too.
		if commitID == "" && tagName != "" {
			tags, err := com.GetTags(dbOwner, dbName)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve tags for database")
				return
			}
			tg, ok := tags[tagName]
			if !ok {
				errorPage(w, r, http.StatusInternalServerError, "Unknown tag requested for this database")
				return
			}
			commitID = tg.Commit
		}

		// If we still haven't determined the required commit ID, use the head commit of the default branch
		if commitID == "" {
			commitID, err = com.DefaultCommit(dbOwner, dbName)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}
		}

		pageData.DB.Info.Branch = branchName
	}

	// Retrieve the database details
	err = com.DBDetails(&pageData.DB, pageData.PageMeta.LoggedInUser, dbOwner, dbName, commitID)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if the current user is allowed to write to the database
	pageData.WriteEnabled, err = com.CheckDBPermissions(pageData.PageMeta.LoggedInUser, dbOwner, dbName, true)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// For non-live databases, add branch, table and view information by querying it directly, otherwise we get the details from our AMQP backend
	if !pageData.DB.Info.IsLive {
		// Retrieve default branch name details
		if pageData.DB.Info.Branch == "" {
			pageData.DB.Info.Branch = pageData.DB.Info.DefaultBranch
		}
		for i := range branchHeads {
			pageData.DB.Info.BranchList = append(pageData.DB.Info.BranchList, i)
		}
		pageData.DB.Info.Commits = branchHeads[pageData.DB.Info.Branch].CommitCount

		// Query the database
		sdb, err := com.OpenSQLiteDatabaseDefensive(w, r, dbOwner, dbName, commitID, pageData.PageMeta.LoggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		defer sdb.Close()
		pageData.DB.Info.Tables, err = com.TablesAndViews(sdb, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		pageData.DB.Info.Tables, err = com.LiveTablesAndViews(pageData.DB.Info.LiveNode, pageData.PageMeta.LoggedInUser, dbOwner, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		pageData.DB.Info.DBEntry.Size, err = com.LiveSize(pageData.DB.Info.LiveNode, pageData.PageMeta.LoggedInUser, dbOwner, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Fill out various metadata fields
	pageData.PageMeta.Title = fmt.Sprintf("%s / %s", dbOwner, dbName)

	// Determine the number of rows to display
	if pageData.PageMeta.LoggedInUser != "" {
		pageData.DB.MaxRows = com.PrefUserMaxRows(pageData.PageMeta.LoggedInUser)
	} else {
		// Not logged in, so use the default number of rows
		pageData.DB.MaxRows = com.DefaultNumDisplayRows
	}

	// Render the full description as markdown
	pageData.DB.Info.FullDesc = string(gfm.Markdown([]byte(pageData.DB.Info.FullDesc)))

	// Increment the view counter for the database (excluding people viewing their own databases)
	if strings.ToLower(pageData.PageMeta.LoggedInUser) != strings.ToLower(dbOwner) {
		err = com.IncrementViewCount(dbOwner, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Render the page
	t := tmpl.Lookup("databasePage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func diffPage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		DB                com.SQLiteDBinfo
		Diffs             com.Diffs
		ColumnNamesBefore map[string][]string
		ColumnNamesAfter  map[string][]string
		PageMeta          PageMetaInfo
	}

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}
	dbName, err := getDatabaseName(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Get the commit ids
	commitA := r.FormValue("commit_a")
	commitB := r.FormValue("commit_b")

	// Validate the supplied information
	if commitA == "" || commitB == "" {
		errorPage(w, r, http.StatusBadRequest, "Missing commit ids")
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, commitA)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	err = com.DBDetails(&pageData.DB, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, commitB)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Retrieve the diffs for these commits
	pageData.Diffs, err = com.Diff(dbName.Owner, dbName.Database, commitA, dbName.Owner, dbName.Database, commitB, pageData.PageMeta.LoggedInUser, com.NoMerge, true)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Retrieve the column information for each table with data changes
	sdbBefore, err := com.OpenSQLiteDatabaseDefensive(w, r, dbName.Owner, dbName.Database, commitA, pageData.PageMeta.LoggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	defer sdbBefore.Close()
	sdbAfter, err := com.OpenSQLiteDatabaseDefensive(w, r, dbName.Owner, dbName.Database, commitB, pageData.PageMeta.LoggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	defer sdbAfter.Close()
	pageData.ColumnNamesBefore = make(map[string][]string)
	pageData.ColumnNamesAfter = make(map[string][]string)
	for _, diff := range pageData.Diffs.Diff {
		if diff.ObjectType == "table" && len(diff.Data) > 0 {
			pks, _, other, err := com.GetPrimaryKeyAndOtherColumns(sdbBefore, "main", diff.ObjectName)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}
			pageData.ColumnNamesBefore[diff.ObjectName] = append(pks, other...)

			pks, _, other, err = com.GetPrimaryKeyAndOtherColumns(sdbAfter, "main", diff.ObjectName)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}
			pageData.ColumnNamesAfter[diff.ObjectName] = append(pks, other...)
		}
	}

	// Fill out the metadata
	pageData.PageMeta.Title = "Changes"

	// Render the main discussion list page
	t := tmpl.Lookup("diffPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func discussPage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		CommentList    []com.DiscussionCommentEntry
		DB             com.SQLiteDBinfo
		DiscussionList []com.DiscussionEntry
		SelectedID     int
		PageMeta       PageMetaInfo
	}

	pageData.PageMeta.PageSection = "db_disc"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}
	dbName, err := getDatabaseName(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if a discussion id was provided
	a := r.FormValue("id")                   // Optional
	if a != "" && a != "{{ row.disc_id }}" { // Search engines have a habit of passing AngularJS tags, so we ignore when the field has the AngularJS tag in it
		pageData.SelectedID, err = strconv.Atoi(a)
		if err != nil {
			log.Printf("Error converting string '%s' to integer in function '%s': %s", com.SanitiseLogString(a),
				com.GetCurrentFunctionName(), err)
			errorPage(w, r, http.StatusBadRequest, "Error when parsing discussion id value")
			return
		}
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Retrieve the list of discussions for this database
	pageData.DiscussionList, err = com.Discussions(dbName.Owner, dbName.Database, com.DISCUSSION, pageData.SelectedID)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Fill out the metadata
	pageData.PageMeta.Title = "Discussion List"

	// If a specific discussion ID was given, then we display the discussion comments page
	if pageData.SelectedID != 0 {
		// Check if the discussion exists, and set the page title to the discussion info
		found := false
		for _, j := range pageData.DiscussionList {
			if pageData.SelectedID == j.ID {
				pageData.PageMeta.Title = fmt.Sprintf("Discussion #%d : %s", j.ID, j.Title)
				found = true
			}
		}
		if !found {
			errorPage(w, r, http.StatusNotFound, "Unknown discussion ID")
			return
		}

		// Load the comments for the requested discussion
		pageData.CommentList, err = com.DiscussionComments(dbName.Owner, dbName.Database, pageData.SelectedID, 0)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// If this discussion matches one of the user's status updates, remove the status update from the list
		if pageData.PageMeta.LoggedInUser != "" {
			pageData.PageMeta.NumStatusUpdates, err = com.StatusUpdateCheck(dbName.Owner, dbName.Database, pageData.SelectedID, pageData.PageMeta.LoggedInUser)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}
		}

		// Render the discussion comments page
		t := tmpl.Lookup("discussCommentsPage")
		err = t.Execute(w, pageData)
		if err != nil {
			log.Printf("Error: %s", err)
		}
		return
	}

	// Render the main discussion list page
	t := tmpl.Lookup("discussListPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// General error display page
func errorPage(w http.ResponseWriter, r *http.Request, httpCode int, msg string) {
	var pageData struct {
		Message  string
		PageMeta PageMetaInfo
	}
	pageData.Message = msg
	pageData.PageMeta.Title = "Error"

	// Get all meta information
	_, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		// We can't use errorPage() here, as it can lead to a recursive loop (which crashes)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `<html><head><title>Internal Server Error</title></head><body>Internal Server Error</body></html>`)
		return
	}

	// Render the page
	w.WriteHeader(httpCode)
	t := tmpl.Lookup("errorPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Render the page showing forks of the given database
func forksPage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		DB       com.SQLiteDBinfo
		Forks    []com.ForkEntry
		PageMeta PageMetaInfo
	}
	pageData.PageMeta.Title = "Forks"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}
	dbName, err := getDatabaseName(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if the user has access to the requested database (and get its details if available)
	err = com.DBDetails(&pageData.DB, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Retrieve list of forks for the database
	pageData.Forks, err = com.ForkTree(pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError,
			fmt.Sprintf("Error retrieving fork list for '%s/%s': %v\n", dbName.Owner, dbName.Database, err.Error()))
		return
	}

	// Render the page
	t := tmpl.Lookup("forksPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Renders the front page of the website.
func frontPage(w http.ResponseWriter, r *http.Request) {
	// Structure to hold page data
	var pageData struct {
		PageMeta PageMetaInfo
		Stats    map[com.ActivityRange]com.ActivityStats
	}

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Retrieve the database activity stats
	pageData.Stats = make(map[com.ActivityRange]com.ActivityStats)
	statsAll, err := com.GetActivityStats()
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.Stats[com.ALL_TIME] = statsAll

	// Set other relevant metadata
	pageData.PageMeta.Title = `SQLite storage "in the cloud"`

	// Render the page
	t := tmpl.Lookup("rootPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func mergePage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		CommentList         []com.DiscussionCommentEntry
		CommitList          []com.CommitData
		DB                  com.SQLiteDBinfo
		DestBranchNameOK    bool
		DestBranchUsable    bool
		LicenceWarning      string
		MRList              []com.DiscussionEntry
		PageMeta            PageMetaInfo
		SelectedID          int
		StatusMessage       string
		StatusMessageColour string
		SourceBranchOK      bool
		SourceDBOK          bool
	}

	pageData.PageMeta.PageSection = "db_merge"

	// Check if an MR id was provided
	a := r.FormValue("id")                   // Optional
	if a != "" && a != "{{ row.disc_id }}" { // Search engines have a habit of passing AngularJS tags, so we ignore when the field has the AngularJS tag in it
		var err error
		pageData.SelectedID, err = strconv.Atoi(a)
		if err != nil {
			log.Printf("Error converting string '%s' to integer in function '%s': %s", com.SanitiseLogString(a),
				com.GetCurrentFunctionName(), err)
			errorPage(w, r, http.StatusBadRequest, "Error when parsing discussion id value")
			return
		}
	}

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}
	dbName, err := getDatabaseName(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if the user has access to the requested database (and get its details if available)
	err = com.DBDetails(&pageData.DB, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Retrieve the list of MRs for this database
	pageData.MRList, err = com.Discussions(dbName.Owner, dbName.Database, com.MERGE_REQUEST, pageData.SelectedID)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Fill out the metadata
	pageData.PageMeta.Title = "Merge Requests"

	// Set the default status message colour
	pageData.StatusMessageColour = "green"

	// If a specific MR ID was given, then we display the MR comments page
	if pageData.SelectedID != 0 {
		// Check if the MR exists, and set the page title to the MR info
		found := false
		for _, j := range pageData.MRList {
			if pageData.SelectedID == j.ID {
				pageData.PageMeta.Title = fmt.Sprintf("Merge Request #%d : %s", j.ID, j.Title)
				found = true
			}
		}
		if !found {
			errorPage(w, r, http.StatusNotFound, "Unknown merge request ID")
			return
		}

		// * Check the current state of the source and destination branches *

		// Check if the source database has been deleted or renamed
		mr := &pageData.MRList[0]
		if mr.MRDetails.SourceDBID != 0 {
			pageData.SourceDBOK, mr.MRDetails.SourceDBName, err = com.CheckDBID(
				mr.MRDetails.SourceOwner, mr.MRDetails.SourceDBID)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}

			// Check if the source branch name is still available
			srcBranches, err := com.GetBranches(mr.MRDetails.SourceOwner, mr.MRDetails.SourceDBName)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}
			_, pageData.SourceBranchOK = srcBranches[mr.MRDetails.SourceBranch]
		} else {
			mr.MRDetails.SourceOwner = "[ unavailable"
			mr.MRDetails.SourceDBName = "database ]"
		}

		// Check if the destination branch name is still available
		destBranches, err := com.GetBranches(dbName.Owner, dbName.Database)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		var destBranchHead com.BranchEntry
		destBranchHead, pageData.DestBranchNameOK = destBranches[mr.MRDetails.DestBranch]
		if !pageData.DestBranchNameOK {
			pageData.StatusMessage = "Destination branch is no longer available. Merge cannot proceed."
			pageData.StatusMessageColour = "red"
		}

		// Get the head commit ID of the destination branch
		destCommitID := destBranchHead.Commit

		// If the MR is still open then make sure the source and destination branches can still be merged
		pageData.DestBranchUsable = true
		if mr.Open {
			// If we the source database (or source branch) isn't available, we can only check if the current mr list
			// still applies to the destination branch
			if !pageData.SourceDBOK || !pageData.SourceBranchOK {

				// Get the commit ID for the commit which would be joined to the destination head
				finalCommit := mr.MRDetails.Commits[len(mr.MRDetails.Commits)-1]

				// If the parent ID of finalCommit isn't the same as the destination head commit, then the destination
				// branch has changed and the merge cannot proceed
				if finalCommit.Parent != destCommitID {
					pageData.DestBranchUsable = false
					pageData.StatusMessage = "Destination branch has changed. Merge cannot proceed."
					pageData.StatusMessageColour = "red"
				}
			} else {
				// Check if the source branch can still be applied to the destination, and also check for new/changed
				// commits
				ancestorID, newCommitList, _, err := com.GetCommonAncestorCommits(mr.MRDetails.SourceOwner,
					mr.MRDetails.SourceDBName, mr.MRDetails.SourceBranch, dbName.Owner,
					dbName.Database, mr.MRDetails.DestBranch)
				if err != nil {
					errorPage(w, r, http.StatusInternalServerError, err.Error())
					return
				}
				if ancestorID == "" {
					// Commits have been added to the destination branch after the MR was created.  This isn't yet
					// a scenario we can successfully merge
					pageData.DestBranchUsable = false
					pageData.StatusMessage = "Destination branch has changed. Merge cannot proceed."
					pageData.StatusMessageColour = "red"
				} else {
					// The source can still be applied to the destination.  Update the merge commit list, just in case
					// the source branch commit list has changed
					mr.MRDetails.Commits = newCommitList

					// Save the updated commit list back to PostgreSQL
					err = com.UpdateMergeRequestCommits(dbName.Owner, dbName.Database, pageData.SelectedID,
						mr.MRDetails.Commits)
					if err != nil {
						errorPage(w, r, http.StatusInternalServerError, err.Error())
						return
					}
				}
			}
		}

		// Retrieve the current licence for the destination branch
		commitList, err := com.GetCommitList(dbName.Owner, dbName.Database)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		destCommit, ok := commitList[destCommitID]
		if !ok {
			errorPage(w, r, http.StatusInternalServerError, "Destination commit ID not found in commit list.")
			return
		}
		destLicenceSHA := destCommit.Tree.Entries[0].LicenceSHA

		// Add the commit author's username and avatar URL to the commit list entries, and check for licence changes
		var licenceChanges bool
		for _, j := range mr.MRDetails.Commits {
			var c com.CommitData
			c.AuthorEmail = j.AuthorEmail
			c.AuthorName = j.AuthorName
			c.ID = j.ID
			c.Parent = j.Parent
			c.Message = j.Message
			c.Timestamp = j.Timestamp
			c.AuthorUsername, c.AuthorAvatar, err = com.GetUsernameFromEmail(j.AuthorEmail)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}
			if c.AuthorAvatar != "" {
				c.AuthorAvatar += "&s=18"
			}

			// Check for licence changes
			commitLicSHA := j.Tree.Entries[0].LicenceSHA
			if commitLicSHA != destLicenceSHA {
				licenceChanges = true
				lName, _, err := com.GetLicenceInfoFromSha256(mr.MRDetails.SourceOwner, commitLicSHA)
				if err != nil {
					errorPage(w, r, http.StatusInternalServerError, err.Error())
					return
				}
				c.LicenceChange = fmt.Sprintf("This commit includes a licence change to '%s'", lName)
			}

			pageData.CommitList = append(pageData.CommitList, c)
		}

		// Warn the user if any of the commits would include a licence change
		if licenceChanges {
			pageData.LicenceWarning = "WARNING: At least one of the commits in the merge list includes a licence " +
				"change. Proceed with caution."
		}

		// Load the comments for the requested MR
		pageData.CommentList, err = com.DiscussionComments(dbName.Owner, dbName.Database, pageData.SelectedID, 0)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// If this MR matches one of the user's status updates, remove the status update from the list
		if pageData.PageMeta.LoggedInUser != "" {
			pageData.PageMeta.NumStatusUpdates, err = com.StatusUpdateCheck(dbName.Owner, dbName.Database, pageData.SelectedID, pageData.PageMeta.LoggedInUser)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}
		}

		// Render the MR comments page
		t := tmpl.Lookup("mergeRequestCommentsPage")
		err = t.Execute(w, pageData)
		if err != nil {
			log.Printf("Error: %s", err)
		}
		return
	}

	// Render the MR list page
	t := tmpl.Lookup("mergeRequestListPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Renders the user Settings page.
func prefPage(w http.ResponseWriter, r *http.Request, loggedInUser string) {
	var pageData struct {
		APIKeys     []com.APIKey
		DisplayName string
		Email       string
		MaxRows     int
		PageMeta    PageMetaInfo
	}
	pageData.PageMeta.Title = "Preferences"
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Grab the display name and email address for the user
	usr, err := com.User(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}
	pageData.DisplayName = usr.DisplayName
	pageData.Email = usr.Email

	// Set the server name, used for the placeholder email address suggestion
	serverName := strings.Split(com.Conf.Web.ServerName, ":")
	pageData.PageMeta.Server = serverName[0]

	// If the email address for the user is empty, use username@server by default.  This mirrors the suggestion on the
	// rendered HTML, so the user doesn't have to manually type it in
	if pageData.Email == "" {
		pageData.Email = fmt.Sprintf("%s@%s", loggedInUser, pageData.PageMeta.Server)
	}

	// Retrieve the user preference data
	pageData.MaxRows = com.PrefUserMaxRows(loggedInUser)

	// Retrieve the list of API keys for the user
	pageData.APIKeys, err = com.GetAPIKeys(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Render the page
	t := tmpl.Lookup("prefPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func profilePage(w http.ResponseWriter, r *http.Request, userName string) {
	var pageData struct {
		PageMeta         PageMetaInfo
		PrivateDBs       []com.DBInfo
		PrivateLiveDBS   []com.DBInfo
		PublicDBs        []com.DBInfo
		PublicLiveDBS    []com.DBInfo
		SharedWithOthers []com.ShareDatabasePermissionsOthers
		SharedWithYou    []com.ShareDatabasePermissionsUser
		Stars            []com.DBEntry
		Watching         []com.DBEntry
	}

	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}
	pageData.PageMeta.LoggedInUser = userName

	// Check if the desired user exists
	userExists, err := com.CheckUserExists(userName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// If the user doesn't exist, indicate that
	if !userExists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Unknown user: %s", userName))
		return
	}

	// Retrieve list of public databases for the user
	pageData.PublicDBs, err = com.UserDBs(userName, com.DB_PUBLIC)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Retrieve list of private databases for the user
	pageData.PrivateDBs, err = com.UserDBs(userName, com.DB_PRIVATE)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Retrieve the list of starred databases for the user
	pageData.Stars, err = com.UserStarredDBs(userName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Retrieve the list of databases being watched by the user
	pageData.Watching, err = com.UserWatchingDBs(userName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Retrieve the list of live databases created by the user
	pageData.PublicLiveDBS, err = com.LiveUserDBs(userName, com.DB_PUBLIC)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	pageData.PrivateLiveDBS, err = com.LiveUserDBs(userName, com.DB_PRIVATE)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// For each of the databases owned by the user, retrieve any share information
	var rawList []com.ShareDatabasePermissionsOthers
	for _, db := range pageData.PublicDBs {
		var z com.ShareDatabasePermissionsOthers
		z.DBName = db.Database
		z.Perms, err = com.GetShares(userName, z.DBName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if len(z.Perms) > 0 {
			rawList = append(rawList, z)
		}
	}
	for _, db := range pageData.PrivateDBs {
		var z com.ShareDatabasePermissionsOthers
		z.DBName = db.Database
		z.Perms, err = com.GetShares(userName, z.DBName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if len(z.Perms) > 0 {
			rawList = append(rawList, z)
		}
	}
	for _, db := range pageData.PublicLiveDBS {
		var z com.ShareDatabasePermissionsOthers
		z.DBName = db.Database
		z.IsLive = true
		z.Perms, err = com.GetShares(userName, z.DBName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if len(z.Perms) > 0 {
			rawList = append(rawList, z)
		}
	}
	for _, db := range pageData.PrivateLiveDBS {
		var z com.ShareDatabasePermissionsOthers
		z.DBName = db.Database
		z.IsLive = true
		z.Perms, err = com.GetShares(userName, z.DBName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if len(z.Perms) > 0 {
			rawList = append(rawList, z)
		}
	}

	// Sort the entries
	sort.SliceStable(rawList, func(i, j int) bool {
		return rawList[i].DBName < rawList[j].DBName
	})
	pageData.SharedWithOthers = rawList

	// Retrieve the list of all databases shared with the user
	pageData.SharedWithYou, err = com.GetSharesForUser(userName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Retrieve the details for the user
	usr, err := com.User(userName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	pageData.PageMeta.Title = usr.Username

	// Render the page
	t := tmpl.Lookup("profilePage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Render the releases page, which displays the releases for a database.
func releasesPage(w http.ResponseWriter, r *http.Request) {
	// Structure to hold page data
	type tgEntry struct {
		AvatarURL         string    `json:"avatar_url"`
		Commit            string    `json:"commit"`
		Date              time.Time `json:"date"`
		Description       string    `json:"description"`
		Size              int64     `json:"size"`
		TaggerUserName    string    `json:"tagger_user_name"`
		TaggerDisplayName string    `json:"tagger_display_name"`
	}
	var pageData struct {
		DB       com.SQLiteDBinfo
		PageMeta PageMetaInfo
		TagList  map[string]tgEntry
	}
	pageData.PageMeta.Title = "Release list"
	pageData.PageMeta.PageSection = "db_data"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}
	dbName, err := getDatabaseName(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Retrieve the release list for the database
	releases, err := com.GetReleases(dbName.Owner, dbName.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Create a small username/email lookup cache, so we don't have to query the database for usernames we've already
	// looked up
	type userCacheEntry struct {
		AvatarURL string
		Email     string
	}
	userNameCache := make(map[string]userCacheEntry)

	// Fill out the metadata
	pageData.TagList = make(map[string]tgEntry)
	if len(releases) > 0 {
		for i, j := range releases {
			// If the username/email address entry is already in the username cache then use it, else grab it from the
			// database (and put it in the cache)
			if _, ok := userNameCache[j.ReleaserEmail]; !ok {
				eml, avatarURL, err := com.GetUsernameFromEmail(j.ReleaserEmail)
				if err != nil {
					errorPage(w, r, http.StatusInternalServerError, err.Error())
					return
				}
				if avatarURL != "" {
					avatarURL += "&s=28"
				}
				userNameCache[j.ReleaserEmail] = userCacheEntry{AvatarURL: avatarURL, Email: eml}
			}

			// Create the tag info we pass to the tag list rendering page
			pageData.TagList[i] = tgEntry{
				AvatarURL:         userNameCache[j.ReleaserEmail].AvatarURL,
				Commit:            j.Commit,
				Date:              j.Date,
				Description:       j.Description,
				Size:              j.Size,
				TaggerUserName:    userNameCache[j.ReleaserEmail].Email,
				TaggerDisplayName: j.ReleaserName,
			}
		}
	}

	// Render the page
	t := tmpl.Lookup("releasesPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Displays a web page for new users to choose their username.
func selectUserNamePage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		Nick     string
		PageMeta PageMetaInfo
	}
	pageData.PageMeta.Title = "Select your username"

	// Retrieve session data (if any)
	sess, err := store.Get(r, "user-reg")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if sess.IsNew {
		// This isn't a valid username selection session, so abort
		errorPage(w, r, http.StatusBadRequest, "Invalid user creation session")
		return
	}
	rip := sess.Values["registrationinprogress"]
	if rip == nil {
		// This isn't a valid username selection session, so abort
		errorPage(w, r, http.StatusBadRequest, "Invalid user creation session")
		return
	}
	validRegSession := rip.(bool)
	if validRegSession != true {
		// For some reason this isn't a valid username selection session, so abort
		errorPage(w, r, http.StatusBadRequest, "Invalid user creation session")
		return
	}

	// If the Auth0 profile included a nickname, we use that to pre-fill the input field
	ni := sess.Values["nickname"]
	if ni != nil {
		pageData.Nick = ni.(string)
	}

	// Render the page
	t := tmpl.Lookup("selectUserNamePage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Render the settings page.
func settingsPage(w http.ResponseWriter, r *http.Request) {
	// Structures to hold page data
	var pageData struct {
		BranchLics       map[string]string
		DB               com.SQLiteDBinfo
		FullDescRendered string
		Licences         map[string]com.LicenceEntry
		NumLicences      int
		PageMeta         PageMetaInfo
		Shares           map[string]com.ShareDatabasePermissions
	}
	pageData.PageMeta.Title = "Database settings"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}
	dbName, err := getDatabaseName(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Require login
	errCode, err = requireLogin(pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Validate permissions
	if strings.ToLower(dbName.Owner) != strings.ToLower(pageData.PageMeta.LoggedInUser) {
		errorPage(w, r, http.StatusBadRequest,
			"You can only access the settings page for your own databases")
		return
	}

	// Retrieve the database details
	err = com.DBDetails(&pageData.DB, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// If it's a standard database then we query it directly, otherwise we query it via our AMQP backend
	if !pageData.DB.Info.IsLive {
		// Get a handle from Minio for the database object
		bkt := pageData.DB.Info.DBEntry.Sha256[:com.MinioFolderChars]
		id := pageData.DB.Info.DBEntry.Sha256[com.MinioFolderChars:]
		sdb, err := com.OpenSQLiteDatabase(bkt, id)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// Automatically close the SQLite database when this function finishes
		defer sdb.Close()

		// Retrieve the list of tables in the database
		pageData.DB.Info.Tables, err = com.TablesAndViews(sdb, fmt.Sprintf("%s/%s", dbName.Owner, dbName.Database))
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// Retrieve the list of branches
		branchHeads, err := com.GetBranches(dbName.Owner, dbName.Database)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// Store list of all branch names
		for i := range branchHeads {
			pageData.DB.Info.BranchList = append(pageData.DB.Info.BranchList, i)
		}

		// Retrieve all the commits for the database
		commitList, err := com.GetCommitList(dbName.Owner, dbName.Database)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// Work out the licence assigned to each of the branch heads
		pageData.BranchLics = make(map[string]string)
		for bName, bEntry := range branchHeads {
			c, ok := commitList[bEntry.Commit]
			if !ok {
				errorPage(w, r, http.StatusInternalServerError, fmt.Sprintf(
					"Couldn't retrieve branch '%s' head commit '%s' for database '%s/%s'\n", bName, bEntry.Commit,
					dbName.Owner, dbName.Database))
				return
			}
			licSHA := c.Tree.Entries[0].LicenceSHA

			// If the licence SHA256 field isn't empty, look up the licence info corresponding to it
			var a string
			if licSHA != "" {
				a, _, err = com.GetLicenceInfoFromSha256(dbName.Owner, licSHA)
				if err != nil {
					errorPage(w, r, http.StatusInternalServerError, err.Error())
					return
				}
			} else {
				a = "Not specified"
			}
			pageData.BranchLics[bName] = a
		}
	} else {
		// Retrieve the list of tables in the database
		pageData.DB.Info.Tables, err = com.LiveTablesAndViews(pageData.DB.Info.LiveNode, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// Also request the database size from our AMQP backend
		pageData.DB.Info.DBEntry.Size, err = com.LiveSize(pageData.DB.Info.LiveNode, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database)
		if err != nil {
			log.Println(err)
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// If the default table is blank, use the first one from the table list
	if pageData.DB.Info.DefaultTable == "" {
		pageData.DB.Info.DefaultTable = pageData.DB.Info.Tables[0]
	}

	// Render the full description markdown
	pageData.FullDescRendered = string(gfm.Markdown([]byte(pageData.DB.Info.FullDesc)))

	// Retrieve the share settings
	pageData.Shares, err = com.GetShares(dbName.Owner, dbName.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Populate the licence list
	pageData.Licences, err = com.GetLicences(pageData.PageMeta.LoggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Error when retrieving list of available licences")
		return
	}
	pageData.NumLicences = len(pageData.Licences)

	// Render the page
	pageData.PageMeta.PageSection = "db_settings"
	t := tmpl.Lookup("settingsPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Present the stars page to the user.
func starsPage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		DB       com.SQLiteDBinfo
		PageMeta PageMetaInfo
		Stars    []com.DBEntry
	}
	pageData.PageMeta.Title = "Stars"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}
	dbName, err := getDatabaseName(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Retrieve the database details
	err = com.DBDetails(&pageData.DB, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Retrieve list of users who starred the database
	pageData.Stars, err = com.UsersStarredDB(dbName.Owner, dbName.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Render the page
	t := tmpl.Lookup("starsPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Render the tag page, which displays the tags for a database.
func tagsPage(w http.ResponseWriter, r *http.Request) {
	// Structure to hold page data
	type tgEntry struct {
		AvatarURL         string    `json:"avatar_url"`
		Commit            string    `json:"commit"`
		Date              time.Time `json:"date"`
		Description       string    `json:"description"`
		Size              int       `json:"size"`
		TaggerUserName    string    `json:"tagger_user_name"`
		TaggerDisplayName string    `json:"tagger_display_name"`
	}
	var pageData struct {
		DB       com.SQLiteDBinfo
		PageMeta PageMetaInfo
		TagList  map[string]tgEntry
	}
	pageData.PageMeta.Title = "Tag list"
	pageData.PageMeta.PageSection = "db_data"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}
	dbName, err := getDatabaseName(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Retrieve the tag list for the database
	tags, err := com.GetTags(dbName.Owner, dbName.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Create a small username/email lookup cache, so we don't have to query the database for usernames we've already
	// looked up
	type userCacheEntry struct {
		AvatarURL string
		Email     string
	}
	userNameCache := make(map[string]userCacheEntry)

	// Fill out the metadata
	pageData.TagList = make(map[string]tgEntry)
	if len(tags) > 0 {
		for i, j := range tags {
			// If the username/email address entry is already in the username cache then use it, else grab it from the
			// database (and put it in the cache)
			if _, ok := userNameCache[j.TaggerEmail]; !ok {
				eml, avatarURL, err := com.GetUsernameFromEmail(j.TaggerEmail)
				if err != nil {
					errorPage(w, r, http.StatusInternalServerError, err.Error())
					return
				}
				if avatarURL != "" {
					avatarURL += "&s=28"
				}
				userNameCache[j.TaggerEmail] = userCacheEntry{AvatarURL: avatarURL, Email: eml}
			}

			// Create the tag info we pass to the tag list rendering page
			pageData.TagList[i] = tgEntry{
				AvatarURL:         userNameCache[j.TaggerEmail].AvatarURL,
				Commit:            j.Commit,
				Date:              j.Date,
				Description:       j.Description,
				TaggerUserName:    userNameCache[j.TaggerEmail].Email,
				TaggerDisplayName: j.TaggerName,
			}
		}
	}

	// Render the page
	t := tmpl.Lookup("tagsPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// This function presents the status updates page to logged in users.
func updatesPage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		PageMeta PageMetaInfo
		Updates  map[string][]com.StatusUpdateEntry
	}

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Require login
	errCode, err = requireLogin(pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Retrieve the list of status updates for the user
	pageData.Updates, err = com.StatusUpdates(pageData.PageMeta.LoggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Fill out page metadata
	pageData.PageMeta.Title = "Status updates"

	// Render the page
	t := tmpl.Lookup("updatesPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// This function presents the database upload form to logged in users.
func uploadPage(w http.ResponseWriter, r *http.Request) {
	// Data to pass to the upload form
	var pageData struct {
		Branches       []string
		DB             com.SQLiteDBinfo
		DefaultBranch  string
		Licences       map[string]com.LicenceEntry
		NumLicences    int
		PageMeta       PageMetaInfo
		SelectedBranch string
	}

	// Get meta information
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Require login
	errCode, err = requireLogin(pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Retrieve the database owner & name from GET parameters.
	// Purposefully not checking for errors here because not providing this information is permitted.
	dbOwner, _, dbDatabase, _ := com.GetUFD(r, true)

	// Retrieve correctly capitalised username for the database owner
	usr, err := com.User(dbOwner)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Store information
	var dbName com.DatabaseName
	dbName.Database = dbDatabase
	dbName.Owner = usr.Username

	// Check if the user has write access to this database, also set the public/private button to the existing value
	if dbName.Owner != "" && dbName.Database != "" {
		writeAccess, err := com.CheckDBPermissions(pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, true)
		if err != nil {
			errorPage(w, r, errCode, err.Error())
			return
		}
		if !writeAccess {
			errorPage(w, r, http.StatusUnauthorized, "You don't have write access to that database")
			return
		}

		// Pre-populate the public/private selection to match the existing setting
		err = com.DBDetails(&pageData.DB, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, "")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
	}

	// Get branch name, if it was passed.  Otherwise, default to "main"
	pageData.SelectedBranch, err = com.GetFormBranch(r)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}
	if pageData.SelectedBranch == "" {
		pageData.SelectedBranch = "main"
	}

	// Ensure the user has set their display name and email address
	usr, err = com.User(pageData.PageMeta.LoggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Error when retrieving user details")
		return
	}
	if usr.DisplayName == "" || usr.Email == "" {
		errorPage(w, r, http.StatusBadRequest,
			"You need to set your full name and email address in Preferences first")
		return
	}

	// Populate the licence list
	pageData.Licences, err = com.GetLicences(pageData.PageMeta.LoggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Error when retrieving list of available licences")
		return
	}
	pageData.NumLicences = len(pageData.Licences)

	// Fill out page metadata
	pageData.PageMeta.Title = "Upload database"

	// Render the page
	t := tmpl.Lookup("uploadPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Renders the user Usage page.
func usagePage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		PageMeta    PageMetaInfo
	}
	pageData.PageMeta.Title = "Usage"
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Require login
	errCode, err = requireLogin(pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// TODO: Figure out display of usage info here


	// Render the page
	t := tmpl.Lookup("usagePage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func userPage(w http.ResponseWriter, r *http.Request, userName string) {
	// Structure to hold page data
	var pageData struct {
		DBRows        []com.DBInfo
		FullName      string
		PageMeta      PageMetaInfo
		PublicLiveDBS []com.DBInfo
		UserAvatarURL string
		UserName      string
	}

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	if pageData.PageMeta.LoggedInUser != "" && strings.ToLower(pageData.PageMeta.LoggedInUser) == strings.ToLower(userName) {
		// The logged in user is looking at their own user page
		profilePage(w, r, pageData.PageMeta.LoggedInUser)
		return
	}

	// Check if the desired user exists
	userExists, err := com.CheckUserExists(userName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// If the user doesn't exist, indicate that
	if !userExists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Unknown user: %s", userName))
		return
	}

	// Retrieve the details for the user whose page we're looking at
	usr, err := com.User(userName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.FullName = usr.DisplayName
	pageData.PageMeta.Title = usr.Username
	pageData.UserName = usr.Username
	if usr.AvatarURL != "" {
		pageData.UserAvatarURL = usr.AvatarURL + "&s=48"
	}

	// Retrieve list of public standard databases owned by the user
	pageData.DBRows, err = com.UserDBs(userName, com.DB_PUBLIC)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Retrieve the list of public live databases created by the user
	pageData.PublicLiveDBS, err = com.LiveUserDBs(userName, com.DB_PUBLIC)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Render the page
	t := tmpl.Lookup("userPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Present the watchers page to the user.
func watchersPage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		DB       com.SQLiteDBinfo
		PageMeta PageMetaInfo
		Watchers []com.DBEntry
	}
	pageData.PageMeta.Title = "Watchers"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.PageMeta)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}
	dbName, err := getDatabaseName(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, pageData.PageMeta.LoggedInUser, dbName.Owner, dbName.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Retrieve list of users watching the database
	pageData.Watchers, err = com.UsersWatchingDB(dbName.Owner, dbName.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Render the page
	t := tmpl.Lookup("watchersPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}
