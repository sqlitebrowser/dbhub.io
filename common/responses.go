package common

// BranchListResponseContainer holds the response to a client request for the database branch list. It's a temporary
// structure, mainly so the JSON created for it is consistent between our various daemons
type BranchListResponseContainer struct {
	Default string                 `json:"default_branch"`
	Entries map[string]BranchEntry `json:"branches"`
}

// MetadataResponse returns the branch list for a database.  It's used by both the DB4S and API daemons, to ensure
// they return exactly the same data
func BranchListResponse(dbOwner, dbFolder, dbName string) (list BranchListResponseContainer, err error) {
	// Retrieve the branch list for the database
	list.Entries, err = GetBranches(dbOwner, dbFolder, dbName)
	if err != nil {
		return
	}

	// Retrieve the default branch for the database
	list.Default, err = GetDefaultBranchName(dbOwner, dbFolder, dbName)
	if err != nil {
		return
	}
	return
}

// MetadataResponseContainer holds the response to a client request for database metadata. It's a temporary structure,
// mainly so the JSON created for it is consistent between our various daemons
type MetadataResponseContainer struct {
	Branches  map[string]BranchEntry  `json:"branches"`
	Commits   map[string]CommitEntry  `json:"commits"`
	DefBranch string                  `json:"default_branch"`
	Releases  map[string]ReleaseEntry `json:"releases"`
	Tags      map[string]TagEntry     `json:"tags"`
	WebPage   string                  `json:"web_page"`
}

// MetadataResponse returns the metadata for a database.  It's used by both the DB4S and API daemons, to ensure they
// return exactly the same data
func MetadataResponse(dbOwner, dbFolder, dbName string) (meta MetadataResponseContainer, err error) {
	// Get the branch heads list for the database
	meta.Branches, err = GetBranches(dbOwner, dbFolder, dbName)
	if err != nil {
		return
	}

	// Get the default branch for the database
	meta.DefBranch, err = GetDefaultBranchName(dbOwner, dbFolder, dbName)
	if err != nil {
		return
	}

	// Get the complete commit list for the database
	meta.Commits, err = GetCommitList(dbOwner, dbFolder, dbName)
	if err != nil {
		return
	}

	// Get the releases for the database
	meta.Releases, err = GetReleases(dbOwner, dbFolder, dbName)
	if err != nil {
		return
	}

	// Get the tags for the database
	meta.Tags, err = GetTags(dbOwner, dbFolder, dbName)
	if err != nil {
		return
	}

	// Generate the link to the web page of this database in the webUI module
	meta.WebPage = "https://" + Conf.Web.ServerName + "/" + dbOwner + "/" + dbName
	return
}
