package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	com "github.com/sqlitebrowser/dbhub.io/common"
	gfm "github.com/sqlitebrowser/github_flavored_markdown"
)

// Renders the "About Us" page.
func aboutPage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		Auth0 com.Auth0Set
		Meta  com.MetaInfo
	}

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, false, false)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}
	pageData.Meta.Title = "What is DBHub.io?"

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

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
	type brEntry struct {
		Commit       string `json:"commit"`
		Description  string `json:"description"`
		MarkDownDesc string `json:"mkdowndesc"`
		Name         string `json:"name"`
	}
	var pageData struct {
		Auth0         com.Auth0Set
		Branches      []brEntry
		DB            com.SQLiteDBinfo
		DefaultBranch string
		Meta          com.MetaInfo
	}
	pageData.Meta.Title = "Branch list"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, false, true)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Read the branch heads list from the database
	branches, err := com.GetBranches(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	pageData.DefaultBranch, err = com.GetDefaultBranchName(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	for i, j := range branches {
		// Create a branch entry
		var r string
		if j.Description == "" {
			r = "No description"
		} else {
			r = string(gfm.Markdown([]byte(j.Description)))
		}
		k := brEntry{
			Commit:       j.Commit,
			Description:  j.Description,
			MarkDownDesc: r,
			Name:         i,
		}
		pageData.Branches = append(pageData.Branches, k)
	}

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

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
		Auth0    com.Auth0Set
		Branch   string
		Branches []string
		DB       com.SQLiteDBinfo
		History  []HistEntry
		Meta     com.MetaInfo
	}
	pageData.Meta.Title = "Commits settings"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, false, true)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Retrieve the branch name
	branchName, err := com.GetFormBranch(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Read the branch heads list from the database
	branches, err := com.GetBranches(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// If no branch name was given, we use the default branch
	if branchName == "" {
		branchName, err = com.GetDefaultBranchName(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Work out the head commit ID for the requested branch
	headCom, ok := branches[branchName]
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
	rawList, err := com.GetCommitList(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
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

	// Fill out the metadata
	pageData.Branch = branchName
	for i := range branches {
		pageData.Branches = append(pageData.Branches, i)
	}

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

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
		Auth0                 com.Auth0Set
		CommitList            []com.CommitData
		DB                    com.SQLiteDBinfo
		DestDBBranches        []string
		DestDBDefaultBranch   string
		DestDBName            string
		DestFolder            string
		DestOwner             string
		Forks                 []com.ForkEntry
		Meta                  com.MetaInfo
		MyStar                bool
		MyWatch               bool
		SourceDBBranches      []string
		SourceDBDefaultBranch string
		SourceDBName          string
		SourceFolder          string
		SourceOwner           string
	}
	pageData.Meta.Title = "Create a Merge Request"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, true, true)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Retrieve list of forks for the database
	pageData.Forks, err = com.ForkTree(pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError,
			fmt.Sprintf("Error retrieving fork list for '%s%s%s': %v\n", pageData.Meta.Owner, pageData.Meta.Folder,
				pageData.Meta.Database, err.Error()))
		return
	}

	// Use the database which the "New Merge Request" button was pressed on as the initially selected source
	pageData.SourceOwner = pageData.Meta.Owner
	pageData.SourceFolder = pageData.Meta.Folder
	pageData.SourceDBName = pageData.Meta.Database

	// If the source database has an (accessible) parent, use that as the default destination selected for the user.
	// If it doesn't, then set the source as the destination as well and the user will have to manually choose
	pageData.DestOwner, pageData.DestFolder, pageData.DestDBName, err = com.ForkParent(pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder,
		pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if pageData.DestOwner == "" || pageData.DestFolder == "" || pageData.DestDBName == "" {
		pageData.DestOwner = pageData.Meta.Owner
		pageData.DestFolder = pageData.Meta.Folder
		pageData.DestDBName = pageData.Meta.Database
	}

	// * Determine the source and destination database branches *

	// Retrieve the branch info for the source database
	srcBranchList, err := com.GetBranches(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	for name := range srcBranchList {
		pageData.SourceDBBranches = append(pageData.SourceDBBranches, name)
	}
	pageData.SourceDBDefaultBranch, err = com.GetDefaultBranchName(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Retrieve the branch info for the destination database
	destBranchList, err := com.GetBranches(pageData.DestOwner, pageData.DestFolder, pageData.DestDBName)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	for name := range destBranchList {
		pageData.DestDBBranches = append(pageData.DestDBBranches, name)
	}
	pageData.DestDBDefaultBranch, err = com.GetDefaultBranchName(pageData.DestOwner, pageData.DestFolder,
		pageData.DestDBName)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if the database was starred by the logged in user
	pageData.MyStar, err = com.CheckDBStarred(pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve latest social stats")
		return
	}

	// Check if the database is being watched by the logged in user
	pageData.MyWatch, err = com.CheckDBWatched(pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve database watch status")
		return
	}

	// If the initially chosen source and destinations can be directly applied, fill out the initial commit list entries
	// for display to the user
	ancestorID, cList, errType, err := com.GetCommonAncestorCommits(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database,
		pageData.SourceDBDefaultBranch, pageData.DestOwner, pageData.DestFolder, pageData.DestDBName,
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
		commitList, err := com.GetCommitList(pageData.DestOwner, pageData.DestFolder, pageData.DestDBName)
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
				lName, _, err := com.GetLicenceInfoFromSha256(pageData.Meta.Owner, commitLicSHA)
				if err != nil {
					errorPage(w, r, http.StatusInternalServerError, err.Error())
					return
				}
				c.LicenceChange = fmt.Sprintf("This commit includes a licence change to '%s'", lName)
			}
			pageData.CommitList = append(pageData.CommitList, c)
		}
	}

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

	// Render the page
	pageData.Meta.PageSection = "db_merge"
	t := tmpl.Lookup("comparePage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Displays a web page asking the user to confirm deleting their database.
func confirmDeletePage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		Auth0 com.Auth0Set
		Meta  com.MetaInfo
	}
	pageData.Meta.Title = "Confirm database deletion"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, true, true)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Make sure the database owner matches the logged in user
	if strings.ToLower(pageData.Meta.LoggedInUser) != strings.ToLower(pageData.Meta.Owner) {
		errorPage(w, r, http.StatusUnauthorized, "You can't delete databases you don't own")
		return
	}

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

	// Render the page
	t := tmpl.Lookup("confirmDeletePage")
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
		Auth0        com.Auth0Set
		Contributors map[string]AuthorEntry
		DB           com.SQLiteDBinfo
		Meta         com.MetaInfo
	}
	pageData.Meta.Title = "Branch list"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, false, true)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Read the commit list from the database
	commitList, err := com.GetCommitList(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
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

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

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
		Auth0  com.Auth0Set
		Meta   com.MetaInfo
		Commit string
	}
	pageData.Meta.Title = "Create new branch"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, true, true)
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
	allowed, err := com.CheckDBPermissions(pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, true)
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

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

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
		Auth0   com.Auth0Set
		DB      com.SQLiteDBinfo
		Meta    com.MetaInfo
		MyStar  bool
		MyWatch bool
	}
	pageData.Meta.Title = "Create new discussion"
	pageData.Meta.PageSection = "db_disc"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, true, true)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if the database was starred by the logged in user
	pageData.MyStar, err = com.CheckDBStarred(pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve latest social stats")
		return
	}

	// Check if the database is being watched by the logged in user
	pageData.MyWatch, err = com.CheckDBWatched(pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve database watch status")
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
		Auth0  com.Auth0Set
		Meta   com.MetaInfo
		Commit string
	}
	pageData.Meta.Title = "Create new tag"

	// Retrieve the commit ID
	commit, err := com.GetFormCommit(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Validation failed for commit value")
		return
	}

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, true, true)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Make sure the logged in user has the permissions to proceed
	allowed, err := com.CheckDBPermissions(pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, true)
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

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

	// Render the page
	t := tmpl.Lookup("createTagPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func databasePage(w http.ResponseWriter, r *http.Request, dbOwner string, dbFolder string, dbName string) {
	pageName := "Render database page"

	var pageData struct {
		Auth0   com.Auth0Set
		Data    com.SQLiteRecordSet
		DB      com.SQLiteDBinfo
		Meta    com.MetaInfo
		MyStar  bool
		MyWatch bool
		Config  com.TomlConfig
	}

	pageData.Meta.PageSection = "db_data"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, false, false)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}
	pageData.Meta.Owner = dbOwner
	pageData.Meta.Folder = dbFolder
	pageData.Meta.Database = dbName

	// Store settings
	pageData.Config = com.Conf

	// Check if a specific database commit ID was given
	commitID, err := com.GetFormCommit(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Invalid database commit ID")
		return
	}

	// If a table name was supplied, validate it
	dbTable := r.FormValue("table")
	if dbTable != "" {
		// TODO: Figure out a better validation approach than using our current PG one.  SQLite clearly has some way
		//       of recognising "unicode characters usable in IDs", so the optimal approach is probably to better grok
		//       tokenize.c and replicate that:
		//         https://github.com/sqlite/sqlite/blob/f25f8d58349db52398168579a1d696fa4937dc1f/src/tokenize.c#L31
		err = com.ValidatePGTable(dbTable)
		if err != nil {
			// Validation failed, so don't pass on the table name
			log.Printf("%s: Validation failed for table name: %s", pageName, err)
			dbTable = ""
		}
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

	// Extract sort column, sort direction, and offset variables if present
	sortCol := r.FormValue("sort")
	sortDir := r.FormValue("dir")
	offsetStr := r.FormValue("offset")

	// If an offset was provided, validate it
	var rowOffset int
	if offsetStr != "" {
		rowOffset, err = strconv.Atoi(offsetStr)
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}

		// Ensure the row offset isn't negative
		if rowOffset < 0 {
			rowOffset = 0
		}
	}

	// Sanity check the sort column name
	if sortCol != "" {
		// Validate the sort column text, as we use it in string smashing SQL queries so need to be even more
		// careful than usual
		err = com.ValidateFieldName(sortCol)
		if err != nil {
			log.Printf("Validation failed on requested sort field name '%v': %v\n", sortCol,
				err.Error())
			errorPage(w, r, http.StatusBadRequest, "Validation failed on requested sort field name")
			return
		}
	}

	// If a sort direction was provided, validate it
	if sortDir != "" {
		if sortDir != "ASC" && sortDir != "DESC" {
			errorPage(w, r, http.StatusBadRequest, "Invalid sort direction")
			return
		}
	}

	// Check if the database exists and the user has access to view it
	exists, err := com.CheckDBPermissions(pageData.Meta.LoggedInUser, dbOwner, dbFolder, dbName, false)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", dbOwner, dbFolder,
			dbName))
		return
	}

	// * Execution can only get here if the user has access to the requested database *

	// Increment the view counter for the database (excluding people viewing their own databases)
	if strings.ToLower(pageData.Meta.LoggedInUser) != strings.ToLower(dbOwner) {
		err = com.IncrementViewCount(dbOwner, dbFolder, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// If a specific commit was requested, make sure it exists in the database commit history
	if commitID != "" {
		commitList, err := com.GetCommitList(dbOwner, dbFolder, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if _, ok := commitList[commitID]; !ok {
			// The requested commit isn't one in the database commit history so error out
			errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Unknown commit for database '%s%s%s'", dbOwner,
				dbFolder, dbName))
			return
		}
	}

	// If a specific release was requested, and no commit ID was given, retrieve the commit ID matching the release
	if commitID == "" && releaseName != "" {
		releases, err := com.GetReleases(dbOwner, dbFolder, dbName)
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
	branchHeads, err := com.GetBranches(dbOwner, dbFolder, dbName)
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
		tags, err := com.GetTags(dbOwner, dbFolder, dbName)
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
		commitID, err = com.DefaultCommit(dbOwner, dbFolder, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Retrieve the database details
	err = com.DBDetails(&pageData.DB, pageData.Meta.LoggedInUser, dbOwner, dbFolder, dbName, commitID)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Get the latest discussion and merge request count directly from PG, skipping the ones (incorrectly) stored in memcache
	currentDisc, currentMRs, err := com.GetDiscussionAndMRCount(dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Check if the database was starred by the logged in user
	myStar, err := com.CheckDBStarred(pageData.Meta.LoggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve database star status")
		return
	}

	// Check if the database is being watched by the logged in user
	myWatch, err := com.CheckDBWatched(pageData.Meta.LoggedInUser, dbOwner, dbFolder, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve database watch status")
		return
	}

	// If a specific table wasn't requested, use the user specified default (if present)
	if dbTable == "" {
		// Ensure the default table name validates.  This catches a case where a database was uploaded with an invalid
		// table name and somehow because selected as the default
		a := pageData.DB.Info.DefaultTable
		if a != "" {
			err = com.ValidatePGTable(a)
			if err == nil {
				// The database table name is acceptable, so use it
				dbTable = pageData.DB.Info.DefaultTable
			}
		}
	}

	// Determine the number of rows to display
	var tempMaxRows int
	if pageData.Meta.LoggedInUser != "" {
		tempMaxRows = com.PrefUserMaxRows(pageData.Meta.LoggedInUser)
		pageData.DB.MaxRows = tempMaxRows
	} else {
		// Not logged in, so use the default number of rows
		tempMaxRows = com.DefaultNumDisplayRows
		pageData.DB.MaxRows = tempMaxRows
	}

	// Generate predictable cache keys for the metadata and sqlite table rows
	// TODO: The cache approach needs redoing, taking into account the life cycle of each info piece
	mdataCacheKey := com.MetadataCacheKey("dwndb-meta", pageData.Meta.LoggedInUser, dbOwner, dbFolder, dbName,
		commitID)
	rowCacheKey := com.TableRowsCacheKey(fmt.Sprintf("tablejson/%s/%s/%d", sortCol, sortDir, rowOffset),
		pageData.Meta.LoggedInUser, dbOwner, dbFolder, dbName, commitID, dbTable, pageData.DB.MaxRows)

	// If a cached version of the page data exists, use it
	ok, err := com.GetCachedData(mdataCacheKey, &pageData)
	if err != nil {
		log.Printf("%s: Error retrieving page data from cache: %v\n", pageName, err)
	}
	if ok {
		// Grab the cached table data as well
		ok, err := com.GetCachedData(rowCacheKey, &pageData.Data)
		if err != nil {
			log.Printf("%s: Error retrieving page data from cache: %v\n", pageName, err)
		}

		// Restore the correct MaxRow value
		pageData.DB.MaxRows = tempMaxRows

		// Restore the correct discussion and MR count
		pageData.DB.Info.Discussions = currentDisc
		pageData.DB.Info.MRs = currentMRs

		// Set the selected branch name
		if branchName != "" {
			pageData.DB.Info.Branch = branchName
		}

		// Fill out the branch info
		pageData.DB.Info.BranchList = []string{}
		if branchName != "" {
			// If a specific branch was requested, ensure it's the first entry of the drop down
			pageData.DB.Info.BranchList = append(pageData.DB.Info.BranchList, branchName)
		}
		for i := range branchHeads {
			if i != branchName {
				err = com.ValidateBranchName(i)
				if err == nil {
					pageData.DB.Info.BranchList = append(pageData.DB.Info.BranchList, i)
				}
			}
		}

		// Check for duplicate branch names in the returned list, and log the problem so an admin can investigate
		bCheck := map[string]struct{}{}
		for _, j := range pageData.DB.Info.BranchList {
			_, ok := bCheck[j]
			if !ok {
				// The branch name value isn't in the map already, so add it
				bCheck[j] = struct{}{}
			} else {
				// This branch name is already in the map.  Duplicate detected.  This shouldn't happen
				log.Printf("Duplicate branch name '%s' detected in returned branch list for database '%s%s%s', "+
					"logged in user '%s'", j, dbOwner, dbFolder, dbName, pageData.Meta.LoggedInUser)
			}
		}

		// Render the page (using the caches)
		if ok {
			t := tmpl.Lookup("databasePage")
			err = t.Execute(w, pageData)
			if err != nil {
				log.Printf("Error: %s", err)
			}
			return
		}

		// Note - If the row data wasn't found in cache, we fall through and continue on with the rest of this
		//        function, which grabs it and caches it for future use
	}

	// Get a handle from Minio for the database object
	sdb, err := com.OpenSQLiteDatabase(pageData.DB.Info.DBEntry.Sha256[:com.MinioFolderChars],
		pageData.DB.Info.DBEntry.Sha256[com.MinioFolderChars:])
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Close the SQLite database and delete the temp file
	defer sdb.Close()

	// Retrieve the list of tables and views in the database
	tables, err := com.TablesAndViews(sdb, dbName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.DB.Info.Tables = tables

	// If a specific table was requested, check that it's present
	if dbTable != "" {
		// Check the requested table is present
		tablePresent := false
		for _, tbl := range tables {
			if tbl == dbTable {
				tablePresent = true
			}
		}
		if tablePresent == false {
			// The requested table doesn't exist in the database, so pick one of the tables that is
			for _, t := range tables {
				err = com.ValidatePGTable(t)
				if err == nil {
					// Validation passed, so use this table
					dbTable = t
					pageData.DB.Info.DefaultTable = t
					break
				}
			}
		}
	}

	// If a specific table wasn't requested, use the first table in the database that passes validation
	if dbTable == "" {
		for _, i := range pageData.DB.Info.Tables {
			if i != "" {
				err = com.ValidatePGTable(i)
				if err == nil {
					// The database table name is acceptable, so use it
					dbTable = i
					break
				}
			}
		}
	}

	// If a sort column was requested, verify it exists
	if sortCol != "" {
		colList, err := sdb.Columns("", dbTable)
		if err != nil {
			log.Printf("Error when reading column names for table '%s': %v\n", dbTable,
				err.Error())
			errorPage(w, r, http.StatusInternalServerError, "Error when reading from the database")
			return
		}
		colExists := false
		for _, j := range colList {
			if j.Name == sortCol {
				colExists = true
			}
		}
		if colExists == false {
			// The requested sort column doesn't exist, so we fall back to no sorting
			sortCol = ""
		}
	}

	// Validate the table name, just to be careful
	if dbTable != "" {
		err = com.ValidatePGTable(dbTable)
		if err != nil {
			// Validation failed, so don't pass on the table name

			// If the failed table name is "{{ db.Tablename }}", don't bother logging it.  It's just a search
			// bot picking up AngularJS in a string and doing a request with it
			if dbTable != "{{ db.Tablename }}" {
				log.Printf("%s: Validation failed for table name: '%s': %s", pageName, dbTable, err)
			}
			errorPage(w, r, http.StatusBadRequest, "Validation failed for table name")
			return
		}
	}

	// Fill out various metadata fields
	pageData.Meta.Title = fmt.Sprintf("%s %s %s", dbOwner, dbFolder, dbName)

	// Retrieve default branch name details
	if branchName == "" {
		branchName, err = com.GetDefaultBranchName(dbOwner, dbFolder, dbName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, "Error retrieving default branch name")
			return
		}
	}

	// Fill out the branch info
	pageData.DB.Info.BranchList = []string{}
	if branchName != "" {
		// If a specific branch was requested, ensure it's the first entry of the drop down
		pageData.DB.Info.BranchList = append(pageData.DB.Info.BranchList, branchName)
	}
	for i := range branchHeads {
		if i != branchName {
			err = com.ValidateBranchName(i)
			if err == nil {
				pageData.DB.Info.BranchList = append(pageData.DB.Info.BranchList, i)
			}
		}
	}

	// Check for duplicate branch names in the returned list, and log the problem so an admin can investigate
	bCheck := map[string]struct{}{}
	for _, j := range pageData.DB.Info.BranchList {
		_, ok := bCheck[j]
		if !ok {
			// The branch name value isn't in the map already, so add it
			bCheck[j] = struct{}{}
		} else {
			// This branch name is already in the map.  Duplicate detected.  This shouldn't happen
			log.Printf("Duplicate branch name '%s' detected in returned branch list for database '%s%s%s', "+
				"logged in user '%s'", j, dbOwner, dbFolder, dbName, pageData.Meta.LoggedInUser)
		}
	}

	pageData.DB.Info.Branch = branchName
	pageData.DB.Info.Commits = branchHeads[branchName].CommitCount

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

	// Update database star and watch status for the logged in user
	pageData.MyStar = myStar
	pageData.MyWatch = myWatch

	// Render the full description as markdown
	pageData.DB.Info.FullDesc = string(gfm.Markdown([]byte(pageData.DB.Info.FullDesc)))

	// Restore the correct discussion and MR count
	pageData.DB.Info.Discussions = currentDisc
	pageData.DB.Info.MRs = currentMRs

	// Cache the page metadata
	err = com.CacheData(mdataCacheKey, pageData, com.Conf.Memcache.DefaultCacheTime)
	if err != nil {
		log.Printf("%s: Error when caching page data: %v\n", pageName, err)
	}

	// Grab the cached table data if it's available
	ok, err = com.GetCachedData(rowCacheKey, &pageData.Data)
	if err != nil {
		log.Printf("%s: Error retrieving page data from cache: %v\n", pageName, err)
	}

	// If the row data wasn't in cache, read it from the database
	if !ok {
		pageData.Data, err = com.ReadSQLiteDB(sdb, dbTable, sortCol, sortDir, pageData.DB.MaxRows, rowOffset)
		if err != nil {
			// Some kind of error when reading the database data
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		pageData.Data.Tablename = dbTable
	}

	// Cache the table row data
	err = com.CacheData(rowCacheKey, pageData.Data, com.Conf.Memcache.DefaultCacheTime)
	if err != nil {
		log.Printf("%s: Error when caching page data: %v\n", pageName, err)
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
		Auth0             com.Auth0Set
		DB                com.SQLiteDBinfo
		Diffs             com.Diffs
		ColumnNamesBefore map[string][]string
		ColumnNamesAfter  map[string][]string
		Meta              com.MetaInfo
		MyStar            bool
		MyWatch           bool
	}

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, false, true)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
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
	err = com.DBDetails(&pageData.DB, pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, commitA)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	err = com.DBDetails(&pageData.DB, pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, commitB)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if the database was starred by the logged in user
	pageData.MyStar, err = com.CheckDBStarred(pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve latest social stats")
		return
	}

	// Check if the database is being watched by the logged in user
	pageData.MyWatch, err = com.CheckDBWatched(pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve database watch status")
		return
	}

	// Retrieve the diffs for these commits
	pageData.Diffs, err = com.Diff(pageData.Meta.Owner, "/", pageData.Meta.Database, commitA, pageData.Meta.Owner, "/", pageData.Meta.Database, commitB, pageData.Meta.LoggedInUser, com.NoMerge, true)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Retrieve the column information for each table with data changes
	sdbBefore, err := com.OpenSQLiteDatabaseDefensive(w, r, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, commitA, pageData.Meta.LoggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	defer sdbBefore.Close()
	sdbAfter, err := com.OpenSQLiteDatabaseDefensive(w, r, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, commitB, pageData.Meta.LoggedInUser)
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
	pageData.Meta.Title = "Changes"

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

	// Render the main discussion list page
	t := tmpl.Lookup("diffPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func discussPage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		Auth0          com.Auth0Set
		CommentList    []com.DiscussionCommentEntry
		DB             com.SQLiteDBinfo
		DiscussionList []com.DiscussionEntry
		Meta           com.MetaInfo
		SelectedID     int
		MyStar         bool
		MyWatch        bool
	}

	pageData.Meta.PageSection = "db_disc"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, false, true)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Check if a discussion id was provided
	a := r.FormValue("id")                   // Optional
	if a != "" && a != "{{ row.disc_id }}" { // Search engines have a habit of passing AngularJS tags, so we ignore when the field has the AngularJS tag in it
		pageData.SelectedID, err = strconv.Atoi(a)
		if err != nil {
			log.Printf("Error converting string '%s' to integer in function '%s': %s\n", a,
				com.GetCurrentFunctionName(), err)
			errorPage(w, r, http.StatusBadRequest, "Error when parsing discussion id value")
			return
		}
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if the database was starred by the logged in user
	pageData.MyStar, err = com.CheckDBStarred(pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve latest social stats")
		return
	}

	// Check if the database is being watched by the logged in user
	pageData.MyWatch, err = com.CheckDBWatched(pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve database watch status")
		return
	}

	// Retrieve the list of discussions for this database
	pageData.DiscussionList, err = com.Discussions(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, com.DISCUSSION, pageData.SelectedID)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Fill out the metadata
	pageData.Meta.Title = "Discussion List"

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

	// If a specific discussion ID was given, then we display the discussion comments page
	if pageData.SelectedID != 0 {
		// Check if the discussion exists, and set the page title to the discussion info
		found := false
		for _, j := range pageData.DiscussionList {
			if pageData.SelectedID == j.ID {
				pageData.Meta.Title = fmt.Sprintf("Discussion #%d : %s", j.ID, j.Title)
				found = true
			}
		}
		if !found {
			errorPage(w, r, http.StatusNotFound, "Unknown discussion ID")
			return
		}

		// Load the comments for the requested discussion
		pageData.CommentList, err = com.DiscussionComments(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, pageData.SelectedID, 0)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// If this discussion matches one of the user's status updates, remove the status update from the list
		if pageData.Meta.LoggedInUser != "" {
			pageData.Meta.NumStatusUpdates, err = com.StatusUpdateCheck(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, pageData.SelectedID, pageData.Meta.LoggedInUser)
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

// General error display page.
func errorPage(w http.ResponseWriter, r *http.Request, httpCode int, msg string) {
	var pageData struct {
		Auth0   com.Auth0Set
		Message string
		Meta    com.MetaInfo
	}
	pageData.Message = msg
	pageData.Meta.Title = "Error"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, false, false)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

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
		Auth0 com.Auth0Set
		Forks []com.ForkEntry
		Meta  com.MetaInfo
	}
	pageData.Meta.Title = "Forks"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, false, true)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Make sure the logged in user has the permissions to proceed
	allowed, err := com.CheckDBPermissions(pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, false)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if allowed == false {
		errorPage(w, r, http.StatusNotFound, "Database not found")
		return
	}

	// Retrieve list of forks for the database
	pageData.Forks, err = com.ForkTree(pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError,
			fmt.Sprintf("Error retrieving fork list for '%s%s%s': %v\n", pageData.Meta.Owner, pageData.Meta.Folder,
				pageData.Meta.Database, err.Error()))
		return
	}

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

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
		Auth0 com.Auth0Set
		Meta  com.MetaInfo
		Stats map[com.ActivityRange]com.ActivityStats
	}

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, false, false)
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
	pageData.Meta.Title = `SQLite storage "in the cloud"`

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

	// Render the page
	t := tmpl.Lookup("rootPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func mergePage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		Auth0               com.Auth0Set
		CommentList         []com.DiscussionCommentEntry
		CommitList          []com.CommitData
		DB                  com.SQLiteDBinfo
		DestBranchNameOK    bool
		DestBranchUsable    bool
		LicenceWarning      string
		MRList              []com.DiscussionEntry
		Meta                com.MetaInfo
		SelectedID          int
		StatusMessage       string
		StatusMessageColour string
		SourceBranchOK      bool
		SourceDBOK          bool
		MyStar              bool
		MyWatch             bool
	}

	pageData.Meta.PageSection = "db_merge"

	// Check if an MR id was provided
	a := r.FormValue("id")                   // Optional
	if a != "" && a != "{{ row.disc_id }}" { // Search engines have a habit of passing AngularJS tags, so we ignore when the field has the AngularJS tag in it
		var err error
		pageData.SelectedID, err = strconv.Atoi(a)
		if err != nil {
			log.Printf("Error converting string '%s' to integer in function '%s': %s\n", a,
				com.GetCurrentFunctionName(), err)
			errorPage(w, r, http.StatusBadRequest, "Error when parsing discussion id value")
			return
		}
	}

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, false, true)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if the database was starred by the logged in user
	pageData.MyStar, err = com.CheckDBStarred(pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve latest social stats")
		return
	}

	// Check if the database is being watched by the logged in user
	pageData.MyWatch, err = com.CheckDBWatched(pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve database watch status")
		return
	}

	// Retrieve the list of MRs for this database
	pageData.MRList, err = com.Discussions(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, com.MERGE_REQUEST, pageData.SelectedID)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Fill out the metadata
	pageData.Meta.Title = "Merge Requests"

	// Set the default status message colour
	pageData.StatusMessageColour = "green"

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

	// If a specific MR ID was given, then we display the MR comments page
	if pageData.SelectedID != 0 {
		// Check if the MR exists, and set the page title to the MR info
		found := false
		for _, j := range pageData.MRList {
			if pageData.SelectedID == j.ID {
				pageData.Meta.Title = fmt.Sprintf("Merge Request #%d : %s", j.ID, j.Title)
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
			pageData.SourceDBOK, mr.MRDetails.SourceFolder, mr.MRDetails.SourceDBName, err = com.CheckDBID(
				mr.MRDetails.SourceOwner, mr.MRDetails.SourceDBID)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}

			// Check if the source branch name is still available
			srcBranches, err := com.GetBranches(mr.MRDetails.SourceOwner, mr.MRDetails.SourceFolder,
				mr.MRDetails.SourceDBName)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}
			_, pageData.SourceBranchOK = srcBranches[mr.MRDetails.SourceBranch]
		} else {
			mr.MRDetails.SourceOwner = "[ unavailable"
			mr.MRDetails.SourceFolder = " "
			mr.MRDetails.SourceDBName = "database ]"
		}

		// Check if the destination branch name is still available
		destBranches, err := com.GetBranches(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
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
					mr.MRDetails.SourceFolder, mr.MRDetails.SourceDBName, mr.MRDetails.SourceBranch, pageData.Meta.Owner, pageData.Meta.Folder,
					pageData.Meta.Database, mr.MRDetails.DestBranch)
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
					err = com.UpdateMergeRequestCommits(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, pageData.SelectedID,
						mr.MRDetails.Commits)
					if err != nil {
						errorPage(w, r, http.StatusInternalServerError, err.Error())
						return
					}
				}
			}
		}

		// Retrieve the current licence for the destination branch
		commitList, err := com.GetCommitList(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
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
		pageData.CommentList, err = com.DiscussionComments(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, pageData.SelectedID, 0)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// If this MR matches one of the user's status updates, remove the status update from the list
		if pageData.Meta.LoggedInUser != "" {
			pageData.Meta.NumStatusUpdates, err = com.StatusUpdateCheck(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, pageData.SelectedID, pageData.Meta.LoggedInUser)
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
		APIKeys     map[string]com.APIKey
		Auth0       com.Auth0Set
		DBNames     []string
		DisplayName string
		Email       string
		MaxRows     int
		Meta        com.MetaInfo
	}
	pageData.Meta.Title = "Settings"
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, false, false)
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
	pageData.Meta.Server = serverName[0]

	// If the email address for the user is empty, use username@server by default.  This mirrors the suggestion on the
	// rendered HTML, so the user doesn't have to manually type it in
	if pageData.Email == "" {
		pageData.Email = fmt.Sprintf("%s@%s", loggedInUser, pageData.Meta.Server)
	}

	// Retrieve the user preference data
	pageData.MaxRows = com.PrefUserMaxRows(loggedInUser)

	// Retrieve the list of API keys for the user
	pageData.APIKeys, err = com.GetAPIKeys(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// The API keys with no database value set should have their text say "All databases"
	for key, j := range pageData.APIKeys {
		if j.Database == "" {
			tmp := pageData.APIKeys[key]
			tmp.Database = "All databases"
			pageData.APIKeys[key] = tmp
		}
	}

	// Create the list of databases belonging to the user
	dbList, err := com.UserDBs(loggedInUser, com.DB_BOTH)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Retrieving list of databases failed")
		return
	}
	for _, db := range dbList {
		pageData.DBNames = append(pageData.DBNames, db.Database)
	}

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

	// Render the page
	t := tmpl.Lookup("prefPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func profilePage(w http.ResponseWriter, r *http.Request, userName string) {
	var pageData struct {
		Auth0      com.Auth0Set
		Meta       com.MetaInfo
		PrivateDBs []com.DBInfo
		PublicDBs  []com.DBInfo
		Stars      []com.DBEntry
		Watching   []com.DBEntry
	}
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, false, false)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}
	pageData.Meta.LoggedInUser = userName

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

	// Retrieve the details for the user
	usr, err := com.User(userName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	pageData.Meta.Owner = usr.Username
	pageData.Meta.Title = usr.Username

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

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
	type relEntry struct {
		AvatarURL           string    `json:"avatar_url"`
		Commit              string    `json:"commit"`
		Date                time.Time `json:"date"`
		Description         string    `json:"description"`
		DescriptionMarkdown string    `json:"description_markdown"`
		ReleaserUserName    string    `json:"releaser_user_name"`
		ReleaserDisplayName string    `json:"releaser_display_name"`
		Size                int64     `json:"size"`
	}
	var pageData struct {
		Auth0       com.Auth0Set
		DB          com.SQLiteDBinfo
		Meta        com.MetaInfo
		ReleaseList map[string]relEntry
	}
	pageData.Meta.Title = "Release list"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, false, true)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Retrieve the release list for the database
	releases, err := com.GetReleases(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
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
	pageData.ReleaseList = make(map[string]relEntry)
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
			var r string
			if j.Description == "" {
				r = "No description"
			} else {
				r = string(gfm.Markdown([]byte(j.Description)))
			}
			pageData.ReleaseList[i] = relEntry{
				AvatarURL:           userNameCache[j.ReleaserEmail].AvatarURL,
				Commit:              j.Commit,
				Date:                j.Date,
				Description:         j.Description,
				DescriptionMarkdown: r,
				ReleaserUserName:    userNameCache[j.ReleaserEmail].Email,
				ReleaserDisplayName: j.ReleaserName,
				Size:                j.Size,
			}
		}
	}

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

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
		Auth0 com.Auth0Set
		Meta  com.MetaInfo
		Nick  string
	}
	pageData.Meta.Title = "Select your username"

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

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

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
		Auth0            com.Auth0Set
		BranchLics       map[string]string
		DB               com.SQLiteDBinfo
		FullDescRendered string
		Licences         map[string]com.LicenceEntry
		Meta             com.MetaInfo
		NumLicences      int
		MyStar           bool
		MyWatch          bool
		Shares           map[string]com.ShareDatabasePermissions
	}
	pageData.Meta.Title = "Database settings"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, true, true)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Validate permissions
	if strings.ToLower(pageData.Meta.Owner) != strings.ToLower(pageData.Meta.LoggedInUser) {
		errorPage(w, r, http.StatusBadRequest,
			"You can only access the settings page for your own databases")
		return
	}

	// Check if the database was starred by the logged in user
	pageData.MyStar, err = com.CheckDBStarred(pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve latest social stats")
		return
	}

	// Check if the database is being watched by the logged in user
	pageData.MyWatch, err = com.CheckDBWatched(pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve database watch status")
		return
	}

	// Retrieve the database details
	err = com.DBDetails(&pageData.DB, pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

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
	pageData.DB.Info.Tables, err = com.TablesAndViews(sdb, fmt.Sprintf("%s%s%s", pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database))
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Retrieve the list of branches
	branchHeads, err := com.GetBranches(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Retrieve all of the commits for the database
	commitList, err := com.GetCommitList(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
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
				"Couldn't retrieve branch '%s' head commit '%s' for database '%s%s%s'\n", bName, bEntry.Commit,
				pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database))
			return
		}
		licSHA := c.Tree.Entries[0].LicenceSHA

		// If the licence SHA256 field isn't empty, look up the licence info corresponding to it
		var a string
		if licSHA != "" {
			a, _, err = com.GetLicenceInfoFromSha256(pageData.Meta.Owner, licSHA)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}
		} else {
			a = "Not specified"
		}
		pageData.BranchLics[bName] = a
	}

	// Populate the licence list
	pageData.Licences, err = com.GetLicences(pageData.Meta.LoggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Error when retrieving list of available licences")
		return
	}
	pageData.NumLicences = len(pageData.Licences)

	// Render the full description markdown
	pageData.FullDescRendered = string(gfm.Markdown([]byte(pageData.DB.Info.FullDesc)))

	// If the default table is blank, use the first one from the table list
	if pageData.DB.Info.DefaultTable == "" {
		pageData.DB.Info.DefaultTable = pageData.DB.Info.Tables[0]
	}

	// Retrieve the share settings
	pageData.Shares, err = com.GetShares(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

	// Render the page
	pageData.Meta.PageSection = "db_settings"
	t := tmpl.Lookup("settingsPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Present the stars page to the user.
func starsPage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		Auth0 com.Auth0Set
		Meta  com.MetaInfo
		Stars []com.DBEntry
	}
	pageData.Meta.Title = "Stars"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, false, true)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Make sure the logged in user has the permissions to proceed
	allowed, err := com.CheckDBPermissions(pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, false)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if allowed == false {
		errorPage(w, r, http.StatusNotFound, "Database not found")
		return
	}

	// Retrieve list of users who starred the database
	pageData.Stars, err = com.UsersStarredDB(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

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
		AvatarURL           string    `json:"avatar_url"`
		Commit              string    `json:"commit"`
		Date                time.Time `json:"date"`
		Description         string    `json:"description"`
		DescriptionMarkdown string    `json:"description_markdown"`
		TaggerUserName      string    `json:"tagger_user_name"`
		TaggerDisplayName   string    `json:"tagger_display_name"`
	}
	var pageData struct {
		Auth0   com.Auth0Set
		DB      com.SQLiteDBinfo
		Meta    com.MetaInfo
		TagList map[string]tgEntry
	}
	pageData.Meta.Title = "Tag list"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, false, true)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Retrieve the tag list for the database
	tags, err := com.GetTags(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
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
			var r string
			if j.Description == "" {
				r = "No description"
			} else {
				r = string(gfm.Markdown([]byte(j.Description)))
			}
			pageData.TagList[i] = tgEntry{
				AvatarURL:           userNameCache[j.TaggerEmail].AvatarURL,
				Commit:              j.Commit,
				Date:                j.Date,
				Description:         j.Description,
				DescriptionMarkdown: r,
				TaggerUserName:      userNameCache[j.TaggerEmail].Email,
				TaggerDisplayName:   j.TaggerName,
			}
		}
	}

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

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
		Auth0   com.Auth0Set
		Meta    com.MetaInfo
		Updates map[string][]com.StatusUpdateEntry
	}

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, true, false)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Retrieve the list of status updates for the user
	pageData.Updates, err = com.StatusUpdates(pageData.Meta.LoggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Fill out page metadata
	pageData.Meta.Title = "Status updates"

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

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
		Auth0         com.Auth0Set
		Branches      []string
		DefaultBranch string
		Licences      map[string]com.LicenceEntry
		Meta          com.MetaInfo
		NumLicences   int
	}

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, true, false)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Ensure the user has set their display name and email address
	usr, err := com.User(pageData.Meta.LoggedInUser)
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
	pageData.Licences, err = com.GetLicences(pageData.Meta.LoggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Error when retrieving list of available licences")
		return
	}
	pageData.NumLicences = len(pageData.Licences)

	// Fill out page metadata
	pageData.Meta.Title = "Upload database"

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

	// Render the page
	t := tmpl.Lookup("uploadPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func userPage(w http.ResponseWriter, r *http.Request, userName string) {
	// Structure to hold page data
	var pageData struct {
		Auth0         com.Auth0Set
		DBRows        []com.DBInfo
		FullName      string
		Meta          com.MetaInfo
		UserAvatarURL string
	}

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, false, false)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	if pageData.Meta.LoggedInUser != "" && strings.ToLower(pageData.Meta.LoggedInUser) == strings.ToLower(userName) {
		// The logged in user is looking at their own user page
		profilePage(w, r, pageData.Meta.LoggedInUser)
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

	// Retrieve the details for the user who's page we're looking at
	usr, err := com.User(userName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.FullName = usr.DisplayName
	pageData.Meta.Owner = usr.Username
	pageData.Meta.Title = usr.Username
	if usr.AvatarURL != "" {
		pageData.UserAvatarURL = usr.AvatarURL + "&s=48"
	}

	// Retrieve list of public databases for the user
	pageData.DBRows, err = com.UserDBs(userName, com.DB_PUBLIC)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

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
		Auth0    com.Auth0Set
		Meta     com.MetaInfo
		Watchers []com.DBEntry
	}
	pageData.Meta.Title = "Watchers"

	// Get all meta information
	errCode, err := collectPageMetaInfo(r, &pageData.Meta, false, true)
	if err != nil {
		errorPage(w, r, errCode, err.Error())
		return
	}

	// Make sure the logged in user has the permissions to proceed
	allowed, err := com.CheckDBPermissions(pageData.Meta.LoggedInUser, pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database, false)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if allowed == false {
		errorPage(w, r, http.StatusNotFound, "Database not found")
		return
	}

	// Retrieve list of users watching the database
	pageData.Watchers, err = com.UsersWatchingDB(pageData.Meta.Owner, pageData.Meta.Folder, pageData.Meta.Database)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Add Auth0 info to the page data
	pageData.Auth0 = collectPageAuth0Info()

	// Render the page
	t := tmpl.Lookup("watchersPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}
