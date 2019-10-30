package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	com "github.com/justinclift/3dhub.io/common"
	gfm "github.com/sqlitebrowser/github_flavored_markdown"
)

// Renders the "About Us" page.
func aboutPage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		Auth0 com.Auth0Set
		Meta  com.MetaInfo
	}

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "3dhub-user")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Retrieve the details and status updates count for the logged in user
	if loggedInUser != "" {
		ur, err := com.User(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if ur.AvatarURL != "" {
			pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
		}
		pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	pageData.Meta.Title = "What is 3DHub.io?"
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	t := tmpl.Lookup("aboutPage")
	err := t.Execute(w, pageData)
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

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "3dhub-user")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Retrieve the database owner & name
	// TODO: Add folder and branch name support
	folder := "/"
	owner, fileName, err := com.GetOD(1, r) // 1 = Ignore "/branches/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Validate the supplied information
	if owner == "" || fileName == "" {
		errorPage(w, r, http.StatusBadRequest, "Missing database owner or database name")
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckFileExists(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", owner, folder,
			fileName))
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, loggedInUser, owner, folder, fileName, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Read the branch heads list from the database
	branches, err := com.GetBranches(owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	pageData.DefaultBranch, err = com.GetDefaultBranchName(owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Retrieve correctly capitalised username for the user
	usr, err := com.User(owner)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.Meta.Owner = usr.Username
	pageData.Meta.Database = fileName

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

	// Retrieve the details and status updates count for the logged in user
	if loggedInUser != "" {
		ur, err := com.User(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if ur.AvatarURL != "" {
			pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
		}
		pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
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

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "3dhub-user")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Retrieve the database owner & name, and branch name
	// TODO: Add folder support
	folder := "/"
	owner, fileName, err := com.GetOD(1, r) // 1 = Ignore "/commits/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	branchName, err := com.GetFormBranch(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Validate the supplied information
	if owner == "" || fileName == "" {
		errorPage(w, r, http.StatusBadRequest, "Missing database owner or database name")
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckFileExists(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", owner, folder,
			fileName))
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, loggedInUser, owner, folder, fileName, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Read the branch heads list from the database
	branches, err := com.GetBranches(owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// If no branch name was given, we use the default branch
	if branchName == "" {
		branchName, err = com.GetDefaultBranchName(owner, folder, fileName)
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
	rawList, err := com.GetCommitList(owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	// TODO: Ugh, this is an ugly approach just to add the username to the commit data.  Surely there's a better way?
	// TODO  Maybe store the username in the commit data structure in the database instead?
	// TODO: Display licence changes too
	uName, avatarURL, err := com.GetUsernameFromEmail(rawList[headID].AuthorEmail)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if avatarURL != "" {
		avatarURL += "&s=30"
	}

	// Create the history entry
	pageData.History = []HistEntry{
		{
			AuthorEmail:    rawList[headID].AuthorEmail,
			AuthorName:     rawList[headID].AuthorName,
			AuthorUserName: uName,
			AvatarURL:      avatarURL,
			CommitterEmail: rawList[headID].CommitterEmail,
			CommitterName:  rawList[headID].CommitterName,
			ID:             rawList[headID].ID,
			Message:        string(gfm.Markdown([]byte(rawList[headID].Message))),
			Parent:         rawList[headID].Parent,
			Timestamp:      rawList[headID].Timestamp,
		},
	}
	commitData := com.CommitEntry{Parent: rawList[headID].Parent}
	for commitData.Parent != "" {
		commitData, ok = rawList[commitData.Parent]
		if !ok {
			errorPage(w, r, http.StatusInternalServerError, "Internal error when retrieving commit data")
			return
		}
		uName, avatarURL, err = com.GetUsernameFromEmail(commitData.AuthorEmail)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if avatarURL != "" {
			avatarURL += "&s=30"
		}

		// Create a history entry
		newEntry := HistEntry{
			AuthorEmail:    commitData.AuthorEmail,
			AuthorName:     commitData.AuthorName,
			AuthorUserName: uName,
			AvatarURL:      avatarURL,
			CommitterEmail: commitData.CommitterEmail,
			CommitterName:  commitData.CommitterName,
			ID:             commitData.ID,
			Message:        string(gfm.Markdown([]byte(commitData.Message))),
			Parent:         commitData.Parent,
			Timestamp:      commitData.Timestamp,
		}
		pageData.History = append(pageData.History, newEntry)
	}

	// Retrieve correctly capitalised username for the user
	usr, err := com.User(owner)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.Meta.Owner = usr.Username

	// Retrieve the details and status updates count for the logged in user
	if loggedInUser != "" {
		ur, err := com.User(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if ur.AvatarURL != "" {
			pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
		}
		pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Fill out the metadata
	pageData.Meta.Database = fileName
	pageData.Branch = branchName
	for i := range branches {
		pageData.Branches = append(pageData.Branches, i)
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
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
		SourceDBBranches      []string
		SourceDBDefaultBranch string
		SourceDBName          string
		SourceFolder          string
		SourceOwner           string
	}
	pageData.Meta.Title = "Create a Merge Request"

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	validSession := false
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "3dhub-user")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Retrieve the database owner & name, and branch name
	// TODO: Add folder support
	folder := "/"
	owner, fileName, err := com.GetOD(1, r) // 1 = Ignore "/compare/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Retrieve list of forks for the database
	pageData.Forks, err = com.ForkTree(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError,
			fmt.Sprintf("Error retrieving fork list for '%s%s%s': %v\n", owner, folder,
				fileName, err.Error()))
		return
	}

	// Use the database which the "New Merge Request" button was pressed on as the initially selected source
	pageData.SourceOwner = owner
	pageData.SourceFolder = folder
	pageData.SourceDBName = fileName

	// If the source database has an (accessible) parent, use that as the default destination selected for the user.
	// If it doesn't, then set the source as the destination as well and the user will have to manually choose
	pageData.DestOwner, pageData.DestFolder, pageData.DestDBName, err = com.ForkParent(loggedInUser, owner, folder,
		fileName)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if pageData.DestOwner == "" || pageData.DestFolder == "" || pageData.DestDBName == "" {
		pageData.DestOwner = owner
		pageData.DestFolder = folder
		pageData.DestDBName = fileName
	}

	// * Determine the source and destination database branches *

	// Retrieve the branch info for the source database
	srcBranchList, err := com.GetBranches(owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	for name := range srcBranchList {
		pageData.SourceDBBranches = append(pageData.SourceDBBranches, name)
	}
	pageData.SourceDBDefaultBranch, err = com.GetDefaultBranchName(owner, folder, fileName)
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
	err = com.DBDetails(&pageData.DB, loggedInUser, owner, folder, fileName, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Get latest star and fork count
	_, pageData.DB.Info.Stars, pageData.DB.Info.Forks, err = com.SocialStats(owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Check if the database was starred by the logged in user
	pageData.MyStar, err = com.CheckDBStarred(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve latest social stats")
		return
	}

	// Retrieve correctly capitalised username for the database owner
	usr, err := com.User(owner)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.Meta.Owner = usr.Username

	// Retrieve the details and status updates count for the logged in user
	if loggedInUser != "" {
		ur, err := com.User(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if ur.AvatarURL != "" {
			pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
		}
		pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// If the initially chosen source and destinations can be directly applied, fill out the initial commit list entries
	// for display to the user
	ancestorID, cList, err, errType := com.GetCommonAncestorCommits(owner, folder, fileName,
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
				lName, _, err := com.GetLicenceInfoFromSha256(owner, commitLicSHA)
				if err != nil {
					errorPage(w, r, http.StatusInternalServerError, err.Error())
					return
				}
				c.LicenceChange = fmt.Sprintf("This commit includes a licence change to '%s'", lName)
			}
			pageData.CommitList = append(pageData.CommitList, c)
		}
	}

	// Retrieve the "forked from" information
	frkOwn, frkFol, frkDB, frkDel, err := com.ForkedFrom(owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failure")
		return
	}
	pageData.Meta.ForkOwner = frkOwn
	pageData.Meta.ForkFolder = frkFol
	pageData.Meta.ForkDatabase = frkDB
	pageData.Meta.ForkDeleted = frkDel

	// Fill out the metadata
	pageData.Meta.Database = fileName

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
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

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	validSession := false
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "3dhub-user")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Retrieve the owner and database name
	owner, fileName, err := com.GetOD(1, r) // "1" means skip the first URL word
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Validation failed for owner or database value")
		return
	}
	// TODO: Add folder support
	folder := "/"

	// Check if the requested database exists
	exists, err := com.CheckFileExists(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", owner, folder,
			fileName))
		return
	}

	// Make sure the database owner matches the logged in user
	if strings.ToLower(loggedInUser) != strings.ToLower(owner) {
		errorPage(w, r, http.StatusUnauthorized, "You can't change databases you don't own")
		return
	}

	// Retrieve correctly capitalised username for the database owner
	usr, err := com.User(owner)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.Meta.Owner = usr.Username

	// Retrieve the details and status updates count for the logged in user
	ur, err := com.User(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if ur.AvatarURL != "" {
		pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
	}
	pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Fill out metadata for the page to be rendered
	pageData.Meta.Database = fileName

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
	t := tmpl.Lookup("confirmDeletePage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// The user wants to view a specific piece of content.  This function determines the type of content, and displays it
// to the user if they have appropriate access permission
func contentPage(w http.ResponseWriter, r *http.Request, owner string, folder string, fileName string) {
	var pageData struct {
		Auth0   com.Auth0Set
		Data    com.SQLiteRecordSet
		DB      com.SQLiteDBinfo
		Meta    com.MetaInfo
		MyStar  bool
		MyWatch bool
	}

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "3dhub-user")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Check if the requested content exists and the user has access to view it
	exists, err := com.CheckFileExists(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("File '%s%s%s' doesn't exist", owner, folder,
			fileName))
		return
	}

	// * Execution can only get here if the user has access to the requested database *

	// Check if a specific commit ID was given
	commitID, err := com.GetFormCommit(r)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Invalid commit ID")
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

	// Determine the type of content, then display it as appropriate
	switch com.GetContentType(loggedInUser, owner, folder, fileName, commitID, branchName, tagName, releaseName) {
	case com.THREE_D_MODEL:
		// If the content is a 3D model, use the 3D model content display page
		threeDModelPage(w, r, loggedInUser, owner, folder, fileName, commitID, branchName, tagName, releaseName)
		return
	case com.DATABASE:
		// If the content is a database, use the existing database content display page
		databasePage(w, r, loggedInUser, owner, folder, fileName, commitID, branchName, tagName, releaseName)
		return
	case com.LICENCE:
		// TODO: Would it be useful to have some capability for displaying the text/html/etc for a given licence?
		errorPage(w, r, http.StatusInternalServerError, "Handler to display licence text hasn't been written yet")
		return
	}
	errorPage(w, r, http.StatusInternalServerError, "Handler for this content type hasn't been written yet")
	return
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

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "3dhub-user")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Retrieve the database owner & name
	// TODO: Add folder and branch support
	folder := "/"
	owner, fileName, err := com.GetOD(1, r) // 1 = Ignore "/branches/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Validate the supplied information
	if owner == "" || fileName == "" {
		errorPage(w, r, http.StatusBadRequest, "Missing database owner or database name")
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckFileExists(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", owner, folder,
			fileName))
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, loggedInUser, owner, folder, fileName, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Read the commit list from the database
	commitList, err := com.GetCommitList(owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Retrieve correctly capitalised username for the user
	usr, err := com.User(owner)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.Meta.Owner = usr.Username

	// Fill out the metadata
	pageData.Meta.Database = fileName
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

	// Retrieve the details and status updates count for the logged in user
	if loggedInUser != "" {
		ur, err := com.User(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if ur.AvatarURL != "" {
			pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
		}
		pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
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

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	validSession := false
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "3dhub-user")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Retrieve the owner, database, and commit ID
	owner, fileName, commit, err := com.GetODC(1, r) // "1" means skip the first URL word
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Validation failed for commit value")
		return
	}
	// TODO: Add folder support
	folder := "/"

	// Check if the requested database exists
	exists, err := com.CheckFileExists(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", owner, folder,
			fileName))
		return
	}

	// Make sure the database owner matches the logged in user
	if strings.ToLower(loggedInUser) != strings.ToLower(owner) {
		errorPage(w, r, http.StatusUnauthorized, "You can't change databases you don't own")
		return
	}

	// Retrieve correctly capitalised username for the database owner
	usr, err := com.User(owner)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.Meta.Owner = usr.Username

	// Retrieve the details and status updates count for the logged in user
	ur, err := com.User(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if ur.AvatarURL != "" {
		pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
	}
	pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Fill out metadata for the page to be rendered
	pageData.Meta.Database = fileName
	pageData.Commit = commit

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
	t := tmpl.Lookup("createBranchPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Displays a web page to input information needed for creating a new discussion.
func createDiscussionPage(w http.ResponseWriter, r *http.Request) {
	var pageData struct {
		Auth0 com.Auth0Set
		Meta  com.MetaInfo
	}
	pageData.Meta.Title = "Create new discussion"

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	validSession := false
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "3dhub-user")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Retrieve the owner, database name
	owner, fileName, err := com.GetOD(1, r) // "1" means skip the first URL word
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	// TODO: Add folder support
	folder := "/"

	// Check if the requested database exists
	exists, err := com.CheckFileExists(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", owner, folder,
			fileName))
		return
	}

	// Retrieve correctly capitalised username for the database owner
	usr, err := com.User(owner)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.Meta.Owner = usr.Username

	// Retrieve the details and status updates count for the logged in user
	if loggedInUser != "" {
		ur, err := com.User(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if ur.AvatarURL != "" {
			pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
		}
		pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Fill out metadata for the page to be rendered
	pageData.Meta.Database = fileName

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
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

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	validSession := false
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "3dhub-user")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Retrieve the owner, database, and commit ID
	owner, fileName, commit, err := com.GetODC(1, r) // "1" means skip the first URL word
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, "Validation failed for commit value")
		return
	}
	// TODO: Add folder support
	folder := "/"

	// Check if the requested database exists
	exists, err := com.CheckFileExists(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", owner, folder,
			fileName))
		return
	}

	// Make sure the database owner matches the logged in user
	if strings.ToLower(loggedInUser) != strings.ToLower(owner) {
		errorPage(w, r, http.StatusUnauthorized, "You can't change databases you don't own")
		return
	}

	// Retrieve correctly capitalised username for the database owner
	usr, err := com.User(owner)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.Meta.Owner = usr.Username

	// Retrieve the details and status updates count for the logged in user
	ur, err := com.User(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if ur.AvatarURL != "" {
		pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
	}
	pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Fill out metadata for the page to be rendered
	pageData.Meta.Database = fileName
	pageData.Commit = commit

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
	t := tmpl.Lookup("createTagPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Displays the database view page to the user, with the requested content
func databasePage(w http.ResponseWriter, r *http.Request, loggedInUser string, owner string, folder string, fileName string, commitID string, branchName string, tagName string, releaseName string) {
	pageName := "Display database page"

	var pageData struct {
		Auth0   com.Auth0Set
		Data    com.SQLiteRecordSet
		DB      com.SQLiteDBinfo
		Meta    com.MetaInfo
		MyStar  bool
		MyWatch bool
	}
	pageData.Meta.LoggedInUser = loggedInUser

	// If a table name was supplied, validate it
	var err error
	dbTable := r.FormValue("table")
	if dbTable != "" {
		err = com.ValidatePGTable(dbTable)
		if err != nil {
			// Validation failed, so don't pass on the table name
			log.Printf("%s: Validation failed for table name: %s", pageName, err)
			dbTable = ""
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

	// Increment the view counter for the database (excluding people viewing their own databases)
	if strings.ToLower(loggedInUser) != strings.ToLower(owner) {
		err = com.IncrementViewCount(owner, folder, fileName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// If a specific commit was requested, make sure it exists in the database commit history
	if commitID != "" {
		commitList, err := com.GetCommitList(owner, folder, fileName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if _, ok := commitList[commitID]; !ok {
			// The requested commit isn't one in the database commit history so error out
			errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Unknown commit for database '%s%s%s'", owner,
				folder, fileName))
			return
		}
	}

	// If a specific release was requested, and no commit ID was given, retrieve the commit ID matching the release
	if commitID == "" && releaseName != "" {
		releases, err := com.GetReleases(owner, folder, fileName)
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
	branchHeads, err := com.GetBranches(owner, folder, fileName)
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
		tags, err := com.GetTags(owner, folder, fileName)
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
		commitID, err = com.DefaultCommit(owner, folder, fileName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Retrieve the database details
	err = com.DBDetails(&pageData.DB, loggedInUser, owner, folder, fileName, commitID)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Get the latest discussion and merge request count directly from PG, skipping the ones (incorrectly) stored in memcache
	currentDisc, currentMRs, err := com.GetDiscussionAndMRCount(owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// If an sha256 was in the licence field, retrieve it's friendly name and url for displaying
	licSHA := pageData.DB.Info.DBEntry.LicenceSHA
	if licSHA != "" {
		pageData.DB.Info.Licence, pageData.DB.Info.LicenceURL, err = com.GetLicenceInfoFromSha256(owner, licSHA)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		pageData.DB.Info.Licence = "Not specified"
	}

	// Check if the database was starred by the logged in user
	myStar, err := com.CheckDBStarred(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve database star status")
		return
	}

	// Check if the database is being watched by the logged in user
	myWatch, err := com.CheckDBWatched(loggedInUser, owner, folder, fileName)
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
	if loggedInUser != "" {
		tempMaxRows = com.PrefUserMaxRows(loggedInUser)
		pageData.DB.MaxRows = tempMaxRows
	} else {
		// Not logged in, so use the default number of rows
		tempMaxRows = com.DefaultNumDisplayRows
		pageData.DB.MaxRows = tempMaxRows
	}

	// Retrieve the details for the logged in user
	var avatarURL string
	if loggedInUser != "" {
		ur, err := com.User(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if ur.AvatarURL != "" {
			avatarURL = ur.AvatarURL + "&s=48"
		}
	}

	// Generate predictable cache keys for the metadata and sqlite table rows
	mdataCacheKey := com.MetadataCacheKey("dwndb-meta", loggedInUser, owner, folder, fileName,
		commitID)
	rowCacheKey := com.TableRowsCacheKey(fmt.Sprintf("tablejson/%s/%s/%d", sortCol, sortDir, rowOffset),
		loggedInUser, owner, folder, fileName, commitID, dbTable, pageData.DB.MaxRows)

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

		// Restore the correct username
		pageData.Meta.LoggedInUser = loggedInUser

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
					"logged in user '%s'", j, owner, folder, fileName, loggedInUser)
			}
		}

		// Retrieve the "forked from" information
		frkOwn, frkFol, frkDB, frkDel, err := com.ForkedFrom(owner, folder, fileName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, "Database query failure")
			return
		}
		pageData.Meta.ForkOwner = frkOwn
		pageData.Meta.ForkFolder = frkFol
		pageData.Meta.ForkDatabase = frkDB
		pageData.Meta.ForkDeleted = frkDel

		// Get latest star and fork count
		_, pageData.DB.Info.Stars, pageData.DB.Info.Forks, err = com.SocialStats(owner, folder, fileName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// Retrieve the status updates count for the logged in user
		if loggedInUser != "" {
			pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}
		}

		// Ensure the correct Avatar URL is displayed
		pageData.Meta.AvatarURL = avatarURL

		// Render the page (using the caches)
		if ok {
			pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
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
	sdb, err := com.OpenMinioObject(pageData.DB.Info.DBEntry.Sha256[:com.MinioFolderChars],
		pageData.DB.Info.DBEntry.Sha256[com.MinioFolderChars:])
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Close the SQLite database and delete the temp file
	defer func() {
		sdb.Close()
	}()

	// Retrieve the list of tables and views in the database
	tables, err := com.Tables(sdb, fileName)
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

	// Retrieve correctly capitalised username for the user
	usr, err := com.User(owner)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.Meta.Owner = usr.Username

	// Ensure the correct Avatar URL is displayed
	pageData.Meta.AvatarURL = avatarURL

	// Retrieve the status updates count for the logged in user
	if loggedInUser != "" {
		pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Fill out various metadata fields
	pageData.Meta.Database = fileName
	pageData.Meta.Server = com.Conf.Web.ServerName
	pageData.Meta.Title = fmt.Sprintf("%s %s %s", owner, folder, fileName)

	// Retrieve default branch name details
	if branchName == "" {
		branchName, err = com.GetDefaultBranchName(owner, folder, fileName)
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
				"logged in user '%s'", j, owner, folder, fileName, loggedInUser)
		}
	}

	pageData.DB.Info.Branch = branchName
	pageData.DB.Info.Commits = branchHeads[branchName].CommitCount

	// Retrieve the "forked from" information
	frkOwn, frkFol, frkDB, frkDel, err := com.ForkedFrom(owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failure")
		return
	}
	pageData.Meta.ForkOwner = frkOwn
	pageData.Meta.ForkFolder = frkFol
	pageData.Meta.ForkDatabase = frkDB
	pageData.Meta.ForkDeleted = frkDel

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

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
		pageData.Data, err = com.ReadSQLiteDB(sdb, dbTable, pageData.DB.MaxRows, sortCol, sortDir, rowOffset)
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
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
	t := tmpl.Lookup("databasePage")
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

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "3dhub-user")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Retrieve the database owner & name
	// TODO: Add folder support
	folder := "/"
	owner, fileName, err := com.GetOD(1, r) // 1 = Ignore "/discuss/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
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

	// Validate the supplied information
	if owner == "" || fileName == "" {
		errorPage(w, r, http.StatusBadRequest, "Missing database owner or database name")
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckFileExists(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", owner, folder,
			fileName))
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, loggedInUser, owner, folder, fileName, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Get latest star and fork count
	_, pageData.DB.Info.Stars, pageData.DB.Info.Forks, err = com.SocialStats(owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Check if the database was starred by the logged in user
	pageData.MyStar, err = com.CheckDBStarred(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve latest social stats")
		return
	}

	// Check if the database is being watched by the logged in user
	pageData.MyWatch, err = com.CheckDBWatched(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve database watch status")
		return
	}

	// Retrieve the list of discussions for this database
	pageData.DiscussionList, err = com.Discussions(owner, folder, fileName, com.DISCUSSION, pageData.SelectedID)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Retrieve the latest discussion and MR counts
	pageData.DB.Info.Discussions, pageData.DB.Info.MRs, err = com.GetDiscussionAndMRCount(owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Retrieve correctly capitalised username for the database owner
	usr, err := com.User(owner)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.Meta.Owner = usr.Username

	// Retrieve the details and status updates count for the logged in user
	if loggedInUser != "" {
		ur, err := com.User(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if ur.AvatarURL != "" {
			pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
		}
		pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Retrieve the "forked from" information
	frkOwn, frkFol, frkDB, frkDel, err := com.ForkedFrom(owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failure")
		return
	}
	pageData.Meta.ForkOwner = frkOwn
	pageData.Meta.ForkFolder = frkFol
	pageData.Meta.ForkDatabase = frkDB
	pageData.Meta.ForkDeleted = frkDel

	// Fill out the metadata
	pageData.Meta.Database = fileName
	pageData.Meta.Title = "Discussion List"

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

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
		pageData.CommentList, err = com.DiscussionComments(owner, folder, fileName, pageData.SelectedID, 0)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// If this discussion matches one of the user's status updates, remove the status update from the list
		if loggedInUser != "" {
			pageData.Meta.NumStatusUpdates, err = com.StatusUpdateCheck(owner, folder, fileName, pageData.SelectedID, loggedInUser)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}
		}

		// Render the discussion comments page
		pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
		t := tmpl.Lookup("discussCommentsPage")
		err = t.Execute(w, pageData)
		if err != nil {
			log.Printf("Error: %s", err)
		}
		return
	}

	// Render the main discussion list page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
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

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "3dhub-user")
		if err != nil {
			fmt.Fprintf(w, "An error occurred when calling errorPage(): %s", err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Retrieve the details and status updates count for the logged in user
	if loggedInUser != "" {
		ur, err := com.User(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if ur.AvatarURL != "" {
			pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
		}
		pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	w.WriteHeader(httpCode)
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
	t := tmpl.Lookup("errorPage")
	err := t.Execute(w, pageData)
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

	// Retrieve user and database name
	owner, fileName, err := com.GetOD(1, r) // 1 = Ignore "/forks/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	pageData.Meta.Database = fileName
	folder := "/"

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "3dhub-user")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Check if the database exists
	exists, err := com.CheckFileExists(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database failure when looking up database details")
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, "That database doesn't seem to exist")
		return
	}

	// Retrieve list of forks for the database
	pageData.Forks, err = com.ForkTree(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError,
			fmt.Sprintf("Error retrieving fork list for '%s%s%s': %v\n", owner, folder,
				fileName, err.Error()))
		return
	}

	// Retrieve correctly capitalised username for the database owner
	usr, err := com.User(owner)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.Meta.Owner = usr.Username

	// Retrieve the details and status updates count for the logged in user
	if loggedInUser != "" {
		ur, err := com.User(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if ur.AvatarURL != "" {
			pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
		}
		pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
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

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "3dhub-user")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
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
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Retrieve the details and status updates count for the logged in user
	if loggedInUser != "" {
		ur, err := com.User(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if ur.AvatarURL != "" {
			pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
		}
		pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Render the page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
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

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "3dhub-user")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Retrieve the database owner & name
	// TODO: Add folder support
	folder := "/"
	owner, fileName, err := com.GetOD(1, r) // 1 = Ignore "/discuss/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Check if an MR id was provided
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

	// Validate the supplied information
	if owner == "" || fileName == "" {
		errorPage(w, r, http.StatusBadRequest, "Missing database owner or database name")
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckFileExists(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", owner, folder,
			fileName))
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, loggedInUser, owner, folder, fileName, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Get latest star and fork count
	_, pageData.DB.Info.Stars, pageData.DB.Info.Forks, err = com.SocialStats(owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Check if the database was starred by the logged in user
	pageData.MyStar, err = com.CheckDBStarred(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve latest social stats")
		return
	}

	// Check if the database is being watched by the logged in user
	pageData.MyWatch, err = com.CheckDBWatched(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve database watch status")
		return
	}

	// Retrieve the list of MRs for this database
	pageData.MRList, err = com.Discussions(owner, folder, fileName, com.MERGE_REQUEST, pageData.SelectedID)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Retrieve the latest discussion and MR counts
	pageData.DB.Info.Discussions, pageData.DB.Info.MRs, err = com.GetDiscussionAndMRCount(owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Retrieve correctly capitalised username for the database owner
	usr, err := com.User(owner)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.Meta.Owner = usr.Username

	// Retrieve the details and status updates count for the logged in user
	if loggedInUser != "" {
		ur, err := com.User(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if ur.AvatarURL != "" {
			pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
		}
		pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Retrieve the "forked from" information
	frkOwn, frkFol, frkDB, frkDel, err := com.ForkedFrom(owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failure")
		return
	}
	pageData.Meta.ForkOwner = frkOwn
	pageData.Meta.ForkFolder = frkFol
	pageData.Meta.ForkDatabase = frkDB
	pageData.Meta.ForkDeleted = frkDel

	// Fill out the metadata
	pageData.Meta.Database = fileName
	pageData.Meta.Title = "Merge Requests"

	// Set the default status message colour
	pageData.StatusMessageColour = "green"

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

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
			pageData.SourceDBOK, mr.MRDetails.SourceFolder, mr.MRDetails.SourceDBName, err = com.CheckDBID(loggedInUser,
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
		destBranches, err := com.GetBranches(owner, folder, fileName)
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
				ancestorID, newCommitList, err, _ := com.GetCommonAncestorCommits(mr.MRDetails.SourceOwner,
					mr.MRDetails.SourceFolder, mr.MRDetails.SourceDBName, mr.MRDetails.SourceBranch, owner, folder,
					fileName, mr.MRDetails.DestBranch)
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
					err = com.UpdateMergeRequestCommits(owner, folder, fileName, pageData.SelectedID,
						mr.MRDetails.Commits)
					if err != nil {
						errorPage(w, r, http.StatusInternalServerError, err.Error())
						return
					}
				}
			}
		}

		// Retrieve the current licence for the destination branch
		commitList, err := com.GetCommitList(owner, folder, fileName)
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
		pageData.CommentList, err = com.DiscussionComments(owner, folder, fileName, pageData.SelectedID, 0)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// If this MR matches one of the user's status updates, remove the status update from the list
		if loggedInUser != "" {
			pageData.Meta.NumStatusUpdates, err = com.StatusUpdateCheck(owner, folder, fileName, pageData.SelectedID, loggedInUser)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}
		}

		// Render the MR comments page
		pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
		t := tmpl.Lookup("mergeRequestCommentsPage")
		err = t.Execute(w, pageData)
		if err != nil {
			log.Printf("Error: %s", err)
		}
		return
	}

	// Render the MR list page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
	t := tmpl.Lookup("mergeRequestListPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Renders the user Preferences page.
func prefPage(w http.ResponseWriter, r *http.Request, loggedInUser string) {
	var pageData struct {
		Auth0       com.Auth0Set
		DisplayName string
		Email       string
		MaxRows     int
		Meta        com.MetaInfo
	}
	pageData.Meta.Title = "Preferences"
	pageData.Meta.LoggedInUser = loggedInUser

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

	// Retrieve the details and status updates count for the logged in user
	ur, err := com.User(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if ur.AvatarURL != "" {
		pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
	}
	pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
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
	pageData.Meta.Server = com.Conf.Web.ServerName
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
	if usr.AvatarURL != "" {
		pageData.Meta.AvatarURL = usr.AvatarURL + "&s=48"
	}

	// Retrieve the details and status updates count for the logged in user
	ur, err := com.User(userName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if ur.AvatarURL != "" {
		pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
	}
	pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(userName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.Meta.Owner = usr.Username
	pageData.Meta.Title = usr.Username

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
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

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "3dhub-user")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Retrieve the database owner & name
	// TODO: Add folder support
	folder := "/"
	owner, fileName, err := com.GetOD(1, r) // 1 = Ignore "/releases/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Validate the supplied information
	if owner == "" || fileName == "" {
		errorPage(w, r, http.StatusBadRequest, "Missing database owner or database name")
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckFileExists(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", owner, folder,
			fileName))
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, loggedInUser, owner, folder, fileName, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Retrieve the release list for the database
	releases, err := com.GetReleases(owner, folder, fileName)
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

	// Retrieve correctly capitalised username for the user
	usr, err := com.User(owner)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.Meta.Owner = usr.Username

	// Fill out the metadata
	pageData.Meta.Database = fileName
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

	// Retrieve the details and status updates count for the logged in user
	if loggedInUser != "" {
		ur, err := com.User(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if ur.AvatarURL != "" {
			pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
		}
		pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
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
	validRegSession := false
	rip := sess.Values["registrationinprogress"]
	if rip == nil {
		// This isn't a valid username selection session, so abort
		errorPage(w, r, http.StatusBadRequest, "Invalid user creation session")
		return
	}
	validRegSession = rip.(bool)
	if validRegSession != true {
		// For some reason this isn't a valid username selection session, so abort
		errorPage(w, r, http.StatusBadRequest, "Invalid user creation session")
		return
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// If the Auth0 profile included a nickname, we use that to pre-fill the input field
	ni := sess.Values["nickname"]
	if ni != nil {
		pageData.Nick = ni.(string)
	}

	// Render the page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
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
	}
	pageData.Meta.Title = "Database settings"

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	validSession := false
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "3dhub-user")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Retrieve the database owner, database name
	// TODO: Add folder support
	folder := "/"
	owner, fileName, err := com.GetOD(1, r) // 1 = Ignore "/settings/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Validate the supplied information
	if owner == "" || fileName == "" {
		errorPage(w, r, http.StatusBadRequest, "Missing database owner or database name")
		return
	}
	if strings.ToLower(owner) != strings.ToLower(loggedInUser) {
		errorPage(w, r, http.StatusBadRequest,
			"You can only access the settings page for your own databases")
		return
	}

	// Check if the user has access to the requested database
	exists, err := com.CheckFileExists(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", owner, folder,
			fileName))
		return
	}

	// Retrieve the database details
	err = com.DBDetails(&pageData.DB, loggedInUser, owner, folder, fileName, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Get a handle from Minio for the database object
	bkt := pageData.DB.Info.DBEntry.Sha256[:com.MinioFolderChars]
	id := pageData.DB.Info.DBEntry.Sha256[com.MinioFolderChars:]
	sdb, err := com.OpenMinioObject(bkt, id)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Automatically close the SQLite database when this function finishes
	defer func() {
		sdb.Close()
	}()

	// Retrieve the list of tables in the database
	pageData.DB.Info.Tables, err = com.Tables(sdb, fmt.Sprintf("%s%s%s", owner, folder, fileName))
	defer sdb.Close()
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Retrieve the list of branches
	branchHeads, err := com.GetBranches(owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Retrieve all of the commits for the database
	commitList, err := com.GetCommitList(owner, folder, fileName)
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
				owner, folder, fileName))
			return
		}
		licSHA := c.Tree.Entries[0].LicenceSHA

		// If the licence SHA256 field isn't empty, look up the licence info corresponding to it
		var a string
		if licSHA != "" {
			a, _, err = com.GetLicenceInfoFromSha256(owner, licSHA)
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
	pageData.Licences, err = com.GetLicences(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Error when retrieving list of available licences")
		return
	}
	pageData.NumLicences = len(pageData.Licences)

	// Render the full description markdown
	pageData.FullDescRendered = string(gfm.Markdown([]byte(pageData.DB.Info.FullDesc)))

	// Retrieve correctly capitalised username for the database owner
	usr, err := com.User(owner)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.Meta.Owner = usr.Username

	// Retrieve the details and status updates count for the logged in user
	ur, err := com.User(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if ur.AvatarURL != "" {
		pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
	}
	pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Fill out the metadata
	pageData.Meta.Database = fileName

	// If the default table is blank, use the first one from the table list
	if pageData.DB.Info.DefaultTable == "" {
		pageData.DB.Info.DefaultTable = pageData.DB.Info.Tables[0]
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
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

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "3dhub-user")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Retrieve owner and database name
	owner, fileName, err := com.GetOD(1, r) // 1 = Ignore "/stars/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	pageData.Meta.Database = fileName

	// Check if the database exists
	// TODO: Add folder support
	folder := "/"
	exists, err := com.CheckFileExists(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database failure when looking up database details")
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, "That database doesn't seem to exist")
		return
	}

	// Retrieve list of users who starred the database
	pageData.Stars, err = com.UsersStarredDB(owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Retrieve correctly capitalised username for the database owner
	usr, err := com.User(owner)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.Meta.Owner = usr.Username

	// Retrieve the details and status updates count for the logged in user
	if loggedInUser != "" {
		ur, err := com.User(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if ur.AvatarURL != "" {
			pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
		}
		pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
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

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "3dhub-user")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Retrieve the database owner & name
	// TODO: Add folder support
	folder := "/"
	owner, fileName, err := com.GetOD(1, r) // 1 = Ignore "/tags/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Validate the supplied information
	if owner == "" || fileName == "" {
		errorPage(w, r, http.StatusBadRequest, "Missing database owner or database name")
		return
	}

	// Check if the requested database exists
	exists, err := com.CheckFileExists(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Database '%s%s%s' doesn't exist", owner, folder,
			fileName))
		return
	}

	// Check if the user has access to the requested database (and get it's details if available)
	err = com.DBDetails(&pageData.DB, loggedInUser, owner, folder, fileName, "")
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Retrieve the tag list for the database
	tags, err := com.GetTags(owner, folder, fileName)
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

	// Retrieve correctly capitalised username for the user
	usr, err := com.User(owner)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.Meta.Owner = usr.Username

	// Fill out the metadata
	pageData.Meta.Database = fileName
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

	// Retrieve the details and status updates count for the logged in user
	if loggedInUser != "" {
		ur, err := com.User(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if ur.AvatarURL != "" {
			pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
		}
		pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
	t := tmpl.Lookup("tagsPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

// Display the 3D model the user requested
func threeDModelPage(w http.ResponseWriter, r *http.Request, loggedInUser string, owner string, folder string, fileName string, commitID string, branchName string, tagName string, releaseName string) {
	pageName := "Display 3D model"

	var pageData struct {
		Auth0   com.Auth0Set
		Data    com.SQLiteRecordSet
		DB      com.SQLiteDBinfo
		Meta    com.MetaInfo
		MyStar  bool
		MyWatch bool
	}
	pageData.Meta.LoggedInUser = loggedInUser

	// Increment the view counter for the file (excluding people viewing their own files)
	var err error
	if strings.ToLower(loggedInUser) != strings.ToLower(owner) {
		err = com.IncrementViewCount(owner, folder, fileName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// If a specific commit was requested, make sure it exists in the commit history
	// TODO: These commitID/release/etc checks should probably be in contentPage(), so they're not duplicated (etc)
	if commitID != "" {
		commitList, err := com.GetCommitList(owner, folder, fileName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if _, ok := commitList[commitID]; !ok {
			// The requested commit isn't one in the commit history so error out
			errorPage(w, r, http.StatusNotFound, fmt.Sprintf("Unknown commit for '%s%s%s'", owner, folder,
				fileName))
			return
		}
	}

	// If a specific release was requested, and no commit ID was given, retrieve the commit ID matching the release
	if commitID == "" && releaseName != "" {
		releases, err := com.GetReleases(owner, folder, fileName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve releases for this file")
			return
		}
		rls, ok := releases[releaseName]
		if !ok {
			errorPage(w, r, http.StatusInternalServerError, "Unknown release requested for this file")
			return
		}
		commitID = rls.Commit
	}

	// Load the branch info for the file
	branchHeads, err := com.GetBranches(owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve branch information for file")
		return
	}

	// If a specific branch was requested and no commit ID was given, use the latest commit for the branch
	if commitID == "" && branchName != "" {
		c, ok := branchHeads[branchName]
		if !ok {
			errorPage(w, r, http.StatusInternalServerError, "Unknown branch requested for this file")
			return
		}
		commitID = c.Commit
	}

	// If a specific tag was requested, and no commit ID was given, retrieve the commit ID matching the tag
	// TODO: If we need to reduce database calls, we can probably make a function merging this, GetBranches(), and
	// TODO  GetCommitList() above.  Potentially also the DBDetails() call below too.
	if commitID == "" && tagName != "" {
		tags, err := com.GetTags(owner, folder, fileName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve tags for this file")
			return
		}
		tg, ok := tags[tagName]
		if !ok {
			errorPage(w, r, http.StatusInternalServerError, "Unknown tag requested for this file")
			return
		}
		commitID = tg.Commit
	}

	// If we still haven't determined the required commit ID, use the head commit of the default branch
	if commitID == "" {
		commitID, err = com.DefaultCommit(owner, folder, fileName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Retrieve the file details
	// TODO: May need to create a 3D model specific version of this function
	err = com.DBDetails(&pageData.DB, loggedInUser, owner, folder, fileName, commitID)
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}

	// Get the latest discussion and merge request count directly from PG, skipping the ones (incorrectly) stored in memcache
	// TODO: Fix this ugliness
	currentDisc, currentMRs, err := com.GetDiscussionAndMRCount(owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// If an sha256 was in the licence field, retrieve it's friendly name and url for displaying
	licSHA := pageData.DB.Info.DBEntry.LicenceSHA
	if licSHA != "" {
		pageData.DB.Info.Licence, pageData.DB.Info.LicenceURL, err = com.GetLicenceInfoFromSha256(owner, licSHA)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		pageData.DB.Info.Licence = "Not specified"
	}

	// Check if the file was starred by the logged in user
	myStar, err := com.CheckDBStarred(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve database star status")
		return
	}

	// Check if the file is being watched by the logged in user
	myWatch, err := com.CheckDBWatched(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Couldn't retrieve database watch status")
		return
	}

	// Retrieve the details for the logged in user
	var avatarURL string
	if loggedInUser != "" {
		ur, err := com.User(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if ur.AvatarURL != "" {
			avatarURL = ur.AvatarURL + "&s=48"
		}
	}

	// Generate predictable cache keys for the metadata
	mdataCacheKey := com.MetadataCacheKey("dwndb-meta", loggedInUser, owner, folder, fileName, commitID)

	// If a cached version of the page data exists, use it
	ok, err := com.GetCachedData(mdataCacheKey, &pageData)
	if err != nil {
		log.Printf("%s: Error retrieving page data from cache: %v\n", pageName, err)
	}
	if ok {
		// Restore the correct username
		pageData.Meta.LoggedInUser = loggedInUser

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
					"logged in user '%s'", j, owner, folder, fileName, loggedInUser)
			}
		}

		// Retrieve the "forked from" information
		frkOwn, frkFol, frkDB, frkDel, err := com.ForkedFrom(owner, folder, fileName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, "Database query failure")
			return
		}
		pageData.Meta.ForkOwner = frkOwn
		pageData.Meta.ForkFolder = frkFol
		pageData.Meta.ForkDatabase = frkDB
		pageData.Meta.ForkDeleted = frkDel

		// Get latest star and fork count
		_, pageData.DB.Info.Stars, pageData.DB.Info.Forks, err = com.SocialStats(owner, folder, fileName)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		// Retrieve the status updates count for the logged in user
		if loggedInUser != "" {
			pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
			if err != nil {
				errorPage(w, r, http.StatusInternalServerError, err.Error())
				return
			}
		}

		// Ensure the correct Avatar URL is displayed
		pageData.Meta.AvatarURL = avatarURL

		// Render the page (using the caches)
		pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
		t := tmpl.Lookup("threeDModelPage")
		err = t.Execute(w, pageData)
		if err != nil {
			log.Printf("Error: %s", err)
		}
		return
	}

	// Get a handle from Minio for the model file
	sdb, err := com.OpenMinioObject(pageData.DB.Info.DBEntry.Sha256[:com.MinioFolderChars],
		pageData.DB.Info.DBEntry.Sha256[com.MinioFolderChars:])
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// TODO: Figure out how to embed the data in the page, so it can be viewed by the wasm viewer
	sdb.Close()

	// Retrieve correctly capitalised username for the user
	usr, err := com.User(owner)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.Meta.Owner = usr.Username

	// Ensure the correct Avatar URL is displayed
	pageData.Meta.AvatarURL = avatarURL

	// Retrieve the status updates count for the logged in user
	if loggedInUser != "" {
		pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Fill out various metadata fields
	pageData.Meta.Database = fileName
	pageData.Meta.Server = com.Conf.Web.ServerName
	pageData.Meta.Title = fmt.Sprintf("%s %s %s", owner, folder, fileName)

	// Retrieve default branch name details
	if branchName == "" {
		branchName, err = com.GetDefaultBranchName(owner, folder, fileName)
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
				"logged in user '%s'", j, owner, folder, fileName, loggedInUser)
		}
	}

	pageData.DB.Info.Branch = branchName
	pageData.DB.Info.Commits = branchHeads[branchName].CommitCount

	// Retrieve the "forked from" information
	frkOwn, frkFol, frkDB, frkDel, err := com.ForkedFrom(owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failure")
		return
	}
	pageData.Meta.ForkOwner = frkOwn
	pageData.Meta.ForkFolder = frkFol
	pageData.Meta.ForkDatabase = frkDB
	pageData.Meta.ForkDeleted = frkDel

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Update file star and watch status for the logged in user
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

	// Render the page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
	t := tmpl.Lookup("threeDModelPage")
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

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	validSession := false
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "3dhub-user")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Retrieve the list of status updates for the user
	var err error
	pageData.Updates, err = com.StatusUpdates(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Retrieve the details for the logged in user
	ur, err := com.User(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if ur.AvatarURL != "" {
		pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
	}

	// Check if there are any status updates for the user
	pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Fill out page metadata
	pageData.Meta.Title = "Status updates"
	pageData.Meta.LoggedInUser = loggedInUser

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
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

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	validSession := false
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "3dhub-user")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
		validSession = true
	}

	// Ensure we have a valid logged in user
	if validSession != true {
		errorPage(w, r, http.StatusUnauthorized, "You need to be logged in")
		return
	}

	// Ensure the user has set their display name and email address
	usr, err := com.User(loggedInUser)
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
	pageData.Licences, err = com.GetLicences(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Error when retrieving list of available licences")
		return
	}
	pageData.NumLicences = len(pageData.Licences)

	// Retrieve the details for the logged in user
	ur, err := com.User(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	if ur.AvatarURL != "" {
		pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
	}

	// Check if there are any status updates for the user
	pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	// Fill out page metadata
	pageData.Meta.Title = "Upload database"
	pageData.Meta.LoggedInUser = loggedInUser

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
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
	pageData.Meta.Server = com.Conf.Web.ServerName

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "3dhub-user")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		if strings.ToLower(loggedInUser) == strings.ToLower(userName) {
			// The logged in user is looking at their own user page
			profilePage(w, r, loggedInUser)
			return
		}
		pageData.Meta.LoggedInUser = loggedInUser
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

	// Retrieve the details and status updates count for the logged in user
	if loggedInUser != "" {
		ur, err := com.User(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if ur.AvatarURL != "" {
			pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
		}
		pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
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
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
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

	// Retrieve session data (if any)
	var loggedInUser string
	var u interface{}
	if com.Conf.Environment.Environment != "docker" {
		sess, err := store.Get(r, "3dhub-user")
		if err != nil {
			errorPage(w, r, http.StatusBadRequest, err.Error())
			return
		}
		u = sess.Values["UserName"]
	} else {
		u = "default"
	}
	if u != nil {
		loggedInUser = u.(string)
		pageData.Meta.LoggedInUser = loggedInUser
	}

	// Retrieve owner and database name
	owner, fileName, err := com.GetOD(1, r) // 1 = Ignore "/watchers/" at the start of the URL
	if err != nil {
		errorPage(w, r, http.StatusBadRequest, err.Error())
		return
	}
	pageData.Meta.Database = fileName

	// Check if the database exists
	// TODO: Add folder support
	folder := "/"
	exists, err := com.CheckFileExists(loggedInUser, owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database failure when looking up database details")
		return
	}
	if !exists {
		errorPage(w, r, http.StatusNotFound, "That database doesn't seem to exist")
		return
	}

	// Retrieve list of users watching the database
	pageData.Watchers, err = com.UsersWatchingDB(owner, folder, fileName)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, "Database query failed")
		return
	}

	// Retrieve correctly capitalised username for the database owner
	usr, err := com.User(owner)
	if err != nil {
		errorPage(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	pageData.Meta.Owner = usr.Username

	// Retrieve the details and status updates count for the logged in user
	if loggedInUser != "" {
		ur, err := com.User(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if ur.AvatarURL != "" {
			pageData.Meta.AvatarURL = ur.AvatarURL + "&s=48"
		}
		pageData.Meta.NumStatusUpdates, err = com.UserStatusUpdates(loggedInUser)
		if err != nil {
			errorPage(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Add Auth0 info to the page data
	pageData.Auth0.CallbackURL = "https://" + com.Conf.Web.ServerName + "/x/callback"
	pageData.Auth0.ClientID = com.Conf.Auth0.ClientID
	pageData.Auth0.Domain = com.Conf.Auth0.Domain

	// Render the page
	pageData.Meta.WebsiteName = com.Conf.Web.WebsiteName
	t := tmpl.Lookup("watchersPage")
	err = t.Execute(w, pageData)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}
