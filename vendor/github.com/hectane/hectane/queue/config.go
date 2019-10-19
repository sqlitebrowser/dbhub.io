package queue

// Application configuration.
type Config struct {
	Directory              string `json:"directory"`
	DisableSSLVerification bool   `json:"disable-ssl-verification"`
}
