package database

import (
	"time"
)

type CommitEntry struct {
	AuthorEmail    string    `json:"author_email"`
	AuthorName     string    `json:"author_name"`
	CommitterEmail string    `json:"committer_email"`
	CommitterName  string    `json:"committer_name"`
	ID             string    `json:"id"`
	Message        string    `json:"message"`
	OtherParents   []string  `json:"other_parents"`
	Parent         string    `json:"parent"`
	Timestamp      time.Time `json:"timestamp"`
	Tree           DBTree    `json:"tree"`
}

type DBEntry struct {
	DateEntry        time.Time
	DBName           string
	Owner            string
	OwnerDisplayName string `json:"display_name"`
}

type DBTreeEntryType string

const (
	TREE     DBTreeEntryType = "tree"
	DATABASE                 = "db"
	LICENCE                  = "licence"
)

type DBTree struct {
	ID      string        `json:"id"`
	Entries []DBTreeEntry `json:"entries"`
}
type DBTreeEntry struct {
	EntryType    DBTreeEntryType `json:"entry_type"`
	LastModified time.Time       `json:"last_modified"`
	LicenceSHA   string          `json:"licence"`
	Name         string          `json:"name"`
	Sha256       string          `json:"sha256"`
	Size         int64           `json:"size"`
}
