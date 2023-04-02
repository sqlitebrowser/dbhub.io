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
