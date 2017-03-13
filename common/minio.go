package common

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"

	sqlite "github.com/gwenn/gosqlite"
	"github.com/minio/minio-go"
)

var (
	// Minio connection handle
	minioClient *minio.Client
)

func ConnectMinio() (err error) {
	// Connect to the Minio server
	minioClient, err = minio.New(MinioServer(), MinioAccessKey(), MinioSecret(), MinioHTTPS())
	if err != nil {
		return errors.New(fmt.Sprintf("Problem with Minio server configuration: %v\n", err))
	}

	// Log Minio server end point
	log.Printf("Minio server config ok. Address: %v\n", MinioServer())

	return nil
}

// Create a bucket in Minio
func CreateMinioBucket(bucket string) error {
	err := minioClient.MakeBucket(bucket, "us-east-1")
	if err != nil {
		log.Printf("Error creating new bucket: %v\n", err)
		return err
	}

	return nil
}

// Check if a given Minio bucket exists
func MinioBucketExists(bucket string) (bool, error) {
	found, err := minioClient.BucketExists(bucket)
	if err != nil {
		log.Printf("Error when checking if Minio bucket '%s' already exists: %v\n", bucket, err)
		return false, err
	}
	return found, nil
}

// Get a handle from Minio for a SQLite database object.
func MinioHandle(bucket string, id string) (*minio.Object, error) {
	userDB, err := minioClient.GetObject(bucket, id)
	if err != nil {
		log.Printf("Error retrieving DB from Minio: %v\n", err)
		return nil, errors.New("Error retrieving database from internal storage")
	}

	return userDB, nil
}

// Close a Minio object handle.  Probably most useful for calling with defer().
func MinioHandleClose(userDB *minio.Object) (err error) {
	err = userDB.Close()
	if err != nil {
		log.Printf("Error closing object handle: %v\n", err)
	}
	return
}

// Retrieves a SQLite database from Minio, opens it, returns the connection handle
func OpenMinioObject(bucket string, id string) (*sqlite.Conn, error) {
	// Get a handle from Minio for the database object
	userDB, err := MinioHandle(bucket, id)
	if err != nil {
		return nil, err
	}

	// Close the object handle when this function finishes
	defer func() {
		MinioHandleClose(userDB)
	}()

	// Save the database locally to a temporary file
	tempfileHandle, err := ioutil.TempFile("", "databaseViewHandler-")
	if err != nil {
		log.Printf("Error creating tempfile: %v\n", err)
		return nil, errors.New("Internal server error")
	}
	tempfile := tempfileHandle.Name()
	bytesWritten, err := io.Copy(tempfileHandle, userDB)
	if err != nil {
		log.Printf("Error writing database to temporary file: %v\n", err)
		return nil, errors.New("Internal server error")
	}
	if bytesWritten == 0 {
		log.Printf("0 bytes written to the SQLite temporary file. Minio object: %s/%s\n", bucket, id)
		return nil, errors.New("Internal server error")
	}
	tempfileHandle.Close()
	defer os.Remove(tempfile) // Delete the temporary file when this function finishes

	// Open database
	sdb, err := sqlite.Open(tempfile, sqlite.OpenReadOnly)
	if err != nil {
		log.Printf("Couldn't open database: %s", err)
		return nil, errors.New("Internal server error")
	}

	return sdb, nil
}

// Removes a Minio bucket, and all files inside it
func RemoveMinioBucket(bucket string) error {
	// Remove the users files
	doneCh := make(chan struct{})
	isRecursive := false
	objectCh := minioClient.ListObjects(bucket, "", isRecursive, doneCh)
	for object := range objectCh {
		if object.Err != nil {
			log.Printf("Error when listing objects: %v", object.Err)
			return object.Err
		}
		err := minioClient.RemoveObject(bucket, object.Key)
		if err != nil {
			log.Printf("Error when removing objects: %v", err)
			return err
		}
	}
	defer close(doneCh)

	// Remove the user's Minio bucket
	err := minioClient.RemoveBucket(bucket)
	if err != nil {
		log.Printf("Error deleting bucket: %v\n", err)
		return err
	}

	return nil
}

// Removes a file from Minio.
func RemoveMinioFile(bucket string, id string) error {
	err := minioClient.RemoveObject(bucket, id)
	if err != nil {
		fmt.Printf("Error when removing Minio file '%s/%s': %v", bucket, id, err)
		return err
	}

	return nil
}

// Store a file in Minio
func StoreMinioObject(bucket string, id string, reader io.Reader, contentType string) (int, error) {
	dbSize, err := minioClient.PutObject(bucket, id, reader, contentType)
	if err != nil {
		log.Printf("Storing file in Minio failed: %v\n", err)
		return -1, err
	}

	return int(dbSize), nil
}
