package main

import (
	"time"

	"github.com/sqlitebrowser/dbhub.io/common/database"
)

type ActivityRange string

const (
	TODAY      ActivityRange = "today"
	THIS_WEEK                = "week"
	THIS_MONTH               = "month"
	ALL_TIME                 = "all"
)

type Auth0Set struct {
	CallbackURL string
	ClientID    string
	Domain      string
}

type CommitData struct {
	AuthorAvatar   string    `json:"author_avatar"`
	AuthorEmail    string    `json:"author_email"`
	AuthorName     string    `json:"author_name"`
	AuthorUsername string    `json:"author_username"`
	ID             string    `json:"id"`
	Parent         string    `json:"parent"`
	LicenceChange  string    `json:"licence_change"`
	Message        string    `json:"message"`
	Timestamp      time.Time `json:"timestamp"`
}

type PageMetaInfo struct {
	ApiUrl           string
	Auth0            Auth0Set
	AvatarURL        string
	Environment      string
	LoggedInUser     string
	NumStatusUpdates int
	PageSection      string
	Protocol         string
	Server           string
	Title            string
}

// ShareDatabasePermissionsOthers contains a list of user permissions for a given database
type ShareDatabasePermissionsOthers struct {
	DBName string                                       `json:"database_name"`
	IsLive bool                                         `json:"is_live"`
	Perms  map[string]database.ShareDatabasePermissions `json:"user_permissions"`
}

type UpdateDataRequestRow struct {
	Key    map[string]string `json:"key"`
	Values map[string]string `json:"values,omitempty"`
}

type UpdateDataRequest struct {
	Table string                 `json:"table"`
	Data  []UpdateDataRequestRow `json:"data"`
}

type ExecuteSqlRequest struct {
	Sql string `json:"sql"`
}

type SaveSqlRequest struct {
	Sql     string `json:"sql"`
	SqlName string `json:"sql_name"`
}
