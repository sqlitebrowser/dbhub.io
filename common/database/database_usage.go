package database

type DBUsage struct {
	Date        string `json:"date"`
	NumLive     int64  `json:"num_live"`
	NumStandard int64  `json:"num_standard"`
}
