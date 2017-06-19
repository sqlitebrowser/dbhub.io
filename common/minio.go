package common

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"

	sqlite "github.com/gwenn/gosqlite"
	"github.com/minio/minio-go"
)

var (
	// Minio connection handle
	minioClient *minio.Client
)

// Parse the Minio configuration, to ensure it seems workable.
// Note - this doesn't actually open a connection to the Minio server.
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

// Create a bucket in Minio.
func CreateMinioBucket(bucket string) error {
	err := minioClient.MakeBucket(bucket, "us-east-1")
	if err != nil {
		log.Printf("Error creating new bucket: %v\n", err)
		return err
	}

	return nil
}

// Check if a given Minio bucket exists.
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

func MinioObjCopy(sourceBucket string, sourceID string, destBucket string) (string, error) {
	// Generate a new name for the destination object
	destID := RandomString(8) + ".db"

	// Copy the SQLite database to the destination bucket
	cpCond := minio.CopyConditions{}
	err := minioClient.CopyObject(destBucket, destID, sourceBucket+"/"+sourceID, cpCond)
	if err != nil {
		return "", err
	}

	return destID, nil
}

// Retrieves a SQLite database from Minio, opens it, returns the connection handle.
// Also returns the name of the temp file created, which the caller needs to delete (os.Remove()) when finished with it
func OpenMinioObject(bucket string, id string) (*sqlite.Conn, string, error) {
	// Get a handle from Minio for the database object
	userDB, err := MinioHandle(bucket, id)
	if err != nil {
		return nil, "", err
	}

	// Close the object handle when this function finishes
	defer func() {
		MinioHandleClose(userDB)
	}()

	// Save the database locally to a temporary file
	tempFileHandle, err := ioutil.TempFile("", "databaseViewHandler-")
	if err != nil {
		log.Printf("Error creating tempfile: %v\n", err)
		return nil, "", errors.New("Internal server error")
	}
	tempFile := tempFileHandle.Name()
	bytesWritten, err := io.Copy(tempFileHandle, userDB)
	if err != nil {
		log.Printf("Error writing database to temporary file: %v\n", err)
		return nil, "", errors.New("Internal server error")
	}
	if bytesWritten == 0 {
		log.Printf("0 bytes written to the SQLite temporary file. Minio object: %s/%s\n", bucket, id)
		return nil, "", errors.New("Internal server error")
	}
	tempFileHandle.Close()

	// Open database
	sdb, err := sqlite.Open(tempFile, sqlite.OpenReadOnly)
	if err != nil {
		log.Printf("Couldn't open database: %s", err)
		return nil, "", errors.New("Internal server error")
	}
	err = sdb.EnableExtendedResultCodes(true)
	if err != nil {
		log.Printf("Couldn't enable extended result codes! Error: %v\n", err.Error())
	}
	return sdb, tempFile, nil
}

// Removes a Minio bucket, and all files inside it.
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

// Store a file in Minio.
func StoreMinioObject(bucket string, id string, reader io.Reader, contentType string) (int, error) {
	dbSize, err := minioClient.PutObject(bucket, id, reader, contentType)
	if err != nil {
		log.Printf("Storing file in Minio failed: %v\n", err)
		return -1, err
	}

	return int(dbSize), nil
}

// Store a database file in Minio.
func StoreDatabaseFile(db []byte, sha string) error {
	bkt := sha[:MinioFolderChars]
	id := sha[MinioFolderChars:]

	// If a Minio bucket with the desired name doesn't already exist, create it
	found, err := minioClient.BucketExists(bkt)
	if err != nil {
		log.Printf("Error when checking if Minio bucket '%s' already exists: %v\n", bkt, err)
		return err
	}
	if !found {
		err := minioClient.MakeBucket(bkt, "us-east-1")
		if err != nil {
			log.Printf("Error creating Minio bucket '%v': %v\n", bkt, err)
			return err
		}
	}

	// Store the SQLite database file in Minio
	dbSize, err := minioClient.PutObject(bkt, id, bytes.NewReader(db), "application/x-sqlite3")
	if err != nil {
		log.Printf("Storing file in Minio failed: %v\n", err)
		return err
	}
	// Sanity check.  Make sure the # of bytes written is equal to the size of the buffer we were given
	if len(db) != int(dbSize) {
		log.Printf("Something went wrong storing the database file: %v\n", err.Error())
		return err
	}
	return nil
}
