package main

type Auth0Set struct {
	CallbackURL string
	ClientID    string
	Domain      string
}

type PageMetaInfo struct {
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

type UpdateDataRequestRow struct {
	Key    map[string]string `json:"key"`
	Values map[string]string `json:"values"`
}

type UpdateDataRequest struct {
	Table string                 `json:"table"`
	Data  []UpdateDataRequestRow `json:"data"`
}
