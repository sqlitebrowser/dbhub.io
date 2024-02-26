package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	gsm "github.com/bradleypeabody/gorilla-sessions-memcache"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	com "github.com/sqlitebrowser/dbhub.io/common"
	"github.com/sqlitebrowser/dbhub.io/common/config"
	"github.com/sqlitebrowser/dbhub.io/common/database"
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
	if err = config.ReadConfig(); err != nil {
		log.Fatalf("Configuration file problem: '%s'", err)
	}

	// Set the node name used in various logging strings
	config.Conf.Live.Nodename = "API server"

	// Open the request log for writing
	reqLog, err = os.OpenFile(config.Conf.Api.RequestLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY|os.O_SYNC, 0750)
	if err != nil {
		log.Fatalf("Error when opening request log: %s", err)
	}
	defer reqLog.Close()
	log.Printf("%s: request log opened: %s", config.Conf.Live.Nodename, config.Conf.Api.RequestLog)

	// Connect to Minio server
	err = com.ConnectMinio()
	if err != nil {
		log.Fatal(err)
	}

	// Connect to database
	err = database.Connect()
	if err != nil {
		log.Fatal(err)
	}

	// Connect to the Memcached server
	err = com.ConnectCache()
	if err != nil {
		log.Fatal(err)
	}

	// Setup session storage
	sessionStore := gsm.NewMemcacheStore(com.MemcacheHandle(), "dbhub_", []byte(config.Conf.Web.SessionStorePassword))

	// Start background goroutines to handle job queue responses
	com.ResponseQueue = com.NewResponseQueue()
	com.CheckResponsesQueue = make(chan struct{})
	com.SubmitterInstance = com.RandomString(3)
	go com.ResponseQueueCheck()
	go com.ResponseQueueListen()

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
		Addr:           config.Conf.Api.BindAddress,
		ErrorLog:       com.HttpErrorLog(),
		Handler:        router,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
		TLSConfig:      tlsConfig,
		TLSNextProto:   make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0),
	}

	// Add gzip middleware
	router.Use(gzip.Gzip(gzip.DefaultCompression))

	// Add CORS middlewares. These allow all origins but only allow sending credentials for the DBHub.io web UI.
	// For this we are using two middlewares here. The first one does the majority of the CORS handling but does
	// not support setting the allow credentials header depending on the provided origin header. Because of this
	// the second one is just adding that header if required.
	router.Use(cors.New(cors.Config{
		// Allow all origins but avoid using the "*" specifier which would disallow sending credentials
		AllowOriginFunc: func(origin string) bool { return true },

		// Allow common REST methods
		AllowMethods: []string{"GET", "POST", "PATCH", "DELETE"},
	}))

	router.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if len(origin) == 0 {
			return
		}

		// This allows sending user credentials from the web UI
		if origin == "https://"+config.Conf.Web.ServerName {
			c.Header("Access-Control-Allow-Credentials", "true")
		}
	})

	// Parse our template files
	router.Delims("[[", "]]")
	router.LoadHTMLGlob(filepath.Join(config.Conf.Web.BaseDir, "api", "templates", "*.html"))

	// Register API v1 handlers. There is three middlewares which apply to all of them:
	// 1) authentication is required
	// 2) usage limits are applied; because these are applied per user this needs to happen after authentication
	// 3) authenticated and permitted calls are logged
	v1 := router.Group("/v1", authenticateV1, limit, callLog)
	{
		v1.POST("/branches", branchesHandler)
		v1.POST("/columns", columnsHandler)
		v1.POST("/commits", commitsHandler)
		v1.POST("/databases", databasesHandler)
		v1.POST("/delete", authRequireWritePermission, deleteHandler)
		v1.POST("/diff", diffHandler)
		v1.POST("/download", downloadHandler)
		v1.POST("/execute", authRequireWritePermission, executeHandler)
		v1.POST("/indexes", indexesHandler)
		v1.POST("/metadata", metadataHandler)
		v1.POST("/query", queryHandler)
		v1.POST("/releases", releasesHandler)
		v1.POST("/tables", tablesHandler)
		v1.POST("/tags", tagsHandler)
		v1.POST("/upload", authRequireWritePermission, uploadHandler)
		v1.POST("/views", viewsHandler)
		v1.POST("/webpage", webpageHandler)
	}

	// Register API v2 handlers. There is three middlewares which apply to all of them:
	// 1) authentication is required
	// 2) usage limits are applied; because these are applied per user this needs to happen after authentication
	// 3) authenticated and permitted calls are logged
	v2 := router.Group("/v2", authenticateV2(sessionStore), limit, callLog)
	{
		v2.GET("/status", statusHandler)
	}

	// Register web routes
	router.GET("/", rootHandler)
	router.GET("/changelog", changeLogHandler)
	router.GET("/changelog.html", changeLogHandler)
	router.StaticFile("/favicon.ico", filepath.Join(config.Conf.Web.BaseDir, "webui", "favicon.ico"))

	// Generate the formatted server string
	server = fmt.Sprintf("https://%s", config.Conf.Api.ServerName)

	// Start API server
	log.Printf("%s: listening on %s", config.Conf.Live.Nodename, server)
	go s.ListenAndServeTLS(config.Conf.Api.Certificate, config.Conf.Api.CertificateKey)

	// Wait for exit signal
	<-exitSignal
}

// authenticateV1 authenticates incoming requests for the API v1 endpoints
func authenticateV1(c *gin.Context) {
	// Extract the API key from the request
	apiKey := c.PostForm("apikey")

	// Look up the details of the API key
	user, key, err := database.GetAPIKeyBySecret(apiKey)

	// Check for any errors
	if err != nil || user == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Unauthorised.  Either no API key was provided, or the provided key doesn't have access.",
		})
		c.Abort()
		return
	}

	// Save username and key
	c.Set("user", user)
	c.Set("key", key)
}

// authenticateV2 authenticates incoming requests for the API v2 endpoints
func authenticateV2(store *gsm.MemcacheStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		// First try getting the authorization header value
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			// Extract the API key from it
			if !strings.HasPrefix(strings.ToLower(authHeader), "apikey ") {
				// Not sending any response back on purpose here. This keeps the amount of traffic we create for
				// possibly large numbers of unauthenticated calls low.
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}
			apiKey := authHeader[7:len(authHeader)] // 7 is the length of "apikey "

			// Look up the details of the API key
			user, key, err := database.GetAPIKeyBySecret(apiKey)

			// Check for any errors
			if err != nil || user == "" {
				// Again, not responding here
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}

			// Save username and key
			c.Set("user", user)
			c.Set("key", key)
		} else {
			// If the authorization header has not been set, check for a session cookie
			sess, err := store.Get(c.Request, "dbhub-user")
			if err != nil {
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}

			u := sess.Values["UserName"]
			if u == nil {
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}

			c.Set("user", u.(string))
			c.Set("key", database.APIKey{
				ID:          0,                        // The ID 0 is translated into NULL when inserting into api_call_log
				Permissions: database.MayReadAndWrite, // Calls from the web UI may read and write
			})
		}
	}
}

// authRequireWritePermission is a middleware which denies requests when the API key used does not provide write permissions
func authRequireWritePermission(c *gin.Context) {
	key := c.MustGet("key").(database.APIKey)
	if key.Permissions != database.MayReadAndWrite {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "This function requires an API key with Write access.  The API key provided doesn't have it.",
		})
		c.Abort()
		return
	}
}

// callLog is a middleware to log authenticated calls to API endpoints to the database
func callLog(c *gin.Context) {
	// Time at the start of the request
	t := time.Now()

	// Process request
	c.Next()

	// Calculate runtime of the request and retrieve other information
	runtime := time.Since(t)
	loggedInUser := c.MustGet("user").(string)
	key := c.MustGet("key").(database.APIKey)
	endpoint := c.Request.URL.Path
	userAgent := c.Request.UserAgent()
	method := c.Request.Method
	statusCode := c.Writer.Status()
	requestSize := c.Request.ContentLength
	responseSize := c.Writer.Size()
	dbOwner := c.GetString("owner")
	dbName := c.GetString("database")

	database.ApiCallLog(key, loggedInUser, dbOwner, dbName, endpoint, userAgent, method, statusCode, runtime, requestSize, responseSize)
}

// changeLogHandler handles requests for the Changelog (a html page)
func changeLogHandler(c *gin.Context) {
	var pageData struct {
		ServerName string
	}

	// Pass through some variables, useful for the generated docs
	pageData.ServerName = config.Conf.Web.ServerName

	// Display our API changelog
	c.HTML(http.StatusOK, "changelog", pageData)
}

// rootHandler handles requests for "/" and all unknown paths
func rootHandler(c *gin.Context) {
	var pageData struct {
		ServerName string
	}

	// If the incoming request is for anything other than the index page, return a 404
	if c.Request.URL.Path != "/" {
		c.JSON(http.StatusNotFound, gin.H{"error": "page not found"})
		return
	}

	// Pass through some variables, useful for the generated docs
	pageData.ServerName = config.Conf.Web.ServerName

	// Display our API documentation
	c.HTML(http.StatusOK, "docs", pageData)
}
