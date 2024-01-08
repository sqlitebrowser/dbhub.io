package main

// TODO: API functions that still need updating for Live databases
//         * diff - already updated to just return an error for live databases.  needs testing though

// FIXME: After the API and webui pieces are done, figure out how the DB4S end
//        point and dio should be updated to use live databases too

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	com "github.com/sqlitebrowser/dbhub.io/common"
)

var (
	// Log file for incoming HTTPS requests
	reqLog *os.File

	// Address of our server, formatted for display
	server string
)

func main() {
	// Read server configuration
	var err error
	if err = com.ReadConfig(); err != nil {
		log.Fatalf("Configuration file problem: '%s'", err)
	}

	// Set the node name used in various logging strings
	com.Conf.Live.Nodename = "API server"

	// Open the request log for writing
	reqLog, err = os.OpenFile(com.Conf.Api.RequestLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY|os.O_SYNC, 0750)
	if err != nil {
		log.Fatalf("Error when opening request log: %s", err)
	}
	defer reqLog.Close()
	log.Printf("%s: request log opened: %s", com.Conf.Live.Nodename, com.Conf.Api.RequestLog)

	// Connect to Minio server
	err = com.ConnectMinio()
	if err != nil {
		log.Fatal(err)
	}

	// Connect to PostgreSQL server
	err = com.ConnectPostgreSQL()
	if err != nil {
		log.Fatal(err)
	}

	// Connect to job queue server
	com.AmqpChan, err = com.ConnectQueue()
	if err != nil {
		log.Fatal(err)
	}

	// Connect to the Memcached server
	err = com.ConnectCache()
	if err != nil {
		log.Fatal(err)
	}

	// Add the default user to the system
	err = com.AddDefaultUser()
	if err != nil {
		log.Fatal(err)
	}

	// Add the default licences to the system
	err = com.AddDefaultLicences()
	if err != nil {
		log.Fatal(err)
	}

	// Start background goroutines to handle job queue responses
	if !com.UseAMQP {
		com.ResponseQueue = com.NewResponseQueue()
		com.CheckResponsesQueue = make(chan struct{})
		com.SubmitterInstance = com.RandomString(3)
		go com.ResponseQueueCheck()
		go com.ResponseQueueListen()
	}

	// Start background signal handler
	exitSignal := make(chan struct{}, 1)
	go com.SignalHandler(&exitSignal)

	// Register log file
	gin.DisableConsoleColor()
	gin.DefaultWriter = io.MultiWriter(reqLog)

	// Create Gin router object
	router := gin.New()

	// Add logging middleware
	router.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return fmt.Sprintf("%v - %s [%s] \"%s %s %s\" \"-\" \"-\" \"%s\" \"%s\"\n",
			param.ClientIP,
			param.Keys["user"],
			time.Now().Format(time.RFC3339Nano),
			param.Method,
			param.Path,
			param.Request.Proto,
			param.Request.Referer(),
			param.Request.UserAgent(),
		)
	}))

	// Add recovery middleware
	router.Use(gin.Recovery())

	// Create TLS and HTTP server configurations
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	s := &http.Server{
		Addr:           com.Conf.Api.BindAddress,
		Handler:        router,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
		TLSConfig:      tlsConfig,
		TLSNextProto:   make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0),
	}

	// Add gzip middleware
	router.Use(gzip.Gzip(gzip.DefaultCompression))

	// Add CORS middleware
	// The default configuration allows all origins
	router.Use(cors.Default())

	// Parse our template files
	router.Delims("[[", "]]")
	router.LoadHTMLGlob(filepath.Join(com.Conf.Web.BaseDir, "api", "templates", "*.html"))

	// Register API v1 handlers. All of them require authentication which is done by the checkAuth middleware
	v1 := router.Group("/v1", checkAuth)
	{
		v1.POST("/branches", branchesHandler)
		v1.POST("/columns", columnsHandler)
		v1.POST("/commits", commitsHandler)
		v1.POST("/databases", databasesHandler)
		v1.POST("/delete", deleteHandler)
		v1.POST("/diff", diffHandler)
		v1.POST("/download", downloadHandler)
		v1.POST("/execute", executeHandler)
		v1.POST("/indexes", indexesHandler)
		v1.POST("/metadata", metadataHandler)
		v1.POST("/query", queryHandler)
		v1.POST("/releases", releasesHandler)
		v1.POST("/tables", tablesHandler)
		v1.POST("/tags", tagsHandler)
		v1.POST("/upload", uploadHandler)
		v1.POST("/views", viewsHandler)
		v1.POST("/webpage", webpageHandler)
	}

	// Register web routes
	router.GET("/", rootHandler)
	router.GET("/changelog", changeLogHandler)
	router.GET("/changelog.html", changeLogHandler)
	router.StaticFile("/favicon.ico", filepath.Join(com.Conf.Web.BaseDir, "webui", "favicon.ico"))

	// Generate the formatted server string
	server = fmt.Sprintf("https://%s", com.Conf.Api.ServerName)

	// Start API server
	log.Printf("%s: listening on %s", com.Conf.Live.Nodename, server)
	go s.ListenAndServeTLS(com.Conf.Api.Certificate, com.Conf.Api.CertificateKey)

	// Wait for exit signal
	<-exitSignal
}

// checkAuth authenticates incoming requests
func checkAuth(c *gin.Context) {
	// Extract the API key from the request
	apiKey := c.PostForm("apikey")

	// Look up the owner of the API key
	user, err := com.GetAPIKeyUser(apiKey)

	// Check for any errors
	if err != nil || user == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	// Save username
	c.Set("user", user)

	return
}

// collectInfo is an internal function which xtracts the database owner, name, and commit ID from the request
// and checks the permissions
func collectInfo(c *gin.Context) (loggedInUser, dbOwner, dbName, commitID string, httpStatus int, err error) {
	// Get user name
	loggedInUser = c.MustGet("user").(string)

	// Extract the database owner name, database name, and (optional) commit ID for the database from the request
	dbOwner, dbName, commitID, err = com.GetFormODC(c.Request)
	if err != nil {
		httpStatus = http.StatusInternalServerError
		return
	}

	// Check if the user has access to the requested database
	// Check if the requested database exists
	exists, err := com.CheckDBPermissions(loggedInUser, dbOwner, dbName, false)
	if err != nil {
		httpStatus = http.StatusInternalServerError
		return
	}
	if !exists {
		httpStatus = http.StatusNotFound
		err = fmt.Errorf("Database does not exist, or user isn't authorised to access it")
		return
	}
	return
}
