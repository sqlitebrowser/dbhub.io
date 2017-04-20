package common

import (
	"bytes"
	"crypto/md5"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"

	"github.com/bradfitz/gomemcache/memcache"
)

var (
	// Connection handles
	memCache *memcache.Client
)

// Caches data in Memcached
func CacheData(cacheKey string, cacheData interface{}, cacheSeconds int32) error {
	// Encode the data
	var encodedData bytes.Buffer
	enc := gob.NewEncoder(&encodedData)
	err := enc.Encode(cacheData)
	if err != nil {
		return err
	}

	// Send the data to memcached
	cachedData := memcache.Item{Key: cacheKey, Value: encodedData.Bytes(), Expiration: cacheSeconds}
	err = memCache.Set(&cachedData)
	if err != nil {
		return err
	}

	return nil
}

func ConnectCache() error {
	// Connect to memcached server
	memCache = memcache.New(conf.Cache.Server)

	// Test the memcached connection
	cacheTest := memcache.Item{Key: "connecttext", Value: []byte("1"), Expiration: 10}
	err := memCache.Set(&cacheTest)
	if err != nil {
		return errors.New(fmt.Sprintf("Couldn't connect to memcached server: %s", err))
	}

	// Log successful connection message for Memcached
	log.Printf("Connected to Memcached: %v\n", conf.Cache.Server)

	return nil
}

// Retrieves cached data from Memcached
func GetCachedData(cacheKey string, cacheData interface{}) (bool, error) {
	cacheItem, err := memCache.Get(cacheKey)
	if err != nil {
		if err == memcache.ErrCacheMiss {
			return false, nil
		}
		return false, err
	}

	// If a value was retrieved, return it
	if cacheItem != nil {
		// Decode the serialised data
		var decBuf bytes.Buffer
		io.Copy(&decBuf, bytes.NewReader(cacheItem.Value))
		dec := gob.NewDecoder(&decBuf)
		dec.Decode(cacheData)
		return true, nil
	}

	return false, nil
}

// Invalidate memcache data for a database version or versions
func InvalidateCacheEntry(loggedInUser string, dbOwner string, dbFolder string, dbName string, dbVersion int) error {

	// If dbVersion is 0, that means "for all versions".  Otherwise, just invalidate the data for the requested one
	var versionList []int
	if dbVersion == 0 {
		// Get the list of all versions for the given database
		var err error
		versionList, err = DBVersions(loggedInUser, dbOwner, dbFolder, dbName)
		versionList = append(versionList, 0) // Need to clear "0" version entries too
		if err != nil {
			return err
		}
	} else {
		// Only one version needs invalidation
		versionList = append(versionList, dbVersion)
	}

	// Loop around, invalidating the now outdated entries
	for _, ver := range versionList {
		// Invalidate the meta info
		cacheKey := MetadataCacheKey("meta", loggedInUser, dbOwner, dbFolder, dbName, ver)
		err := memCache.Delete(cacheKey)
		if err != nil {
			if err != memcache.ErrCacheMiss {
				// Cache miss is not an error we care about
				return err
			}
		}

		// Invalidate the download page data, for private database versions
		cacheKey = MetadataCacheKey("dwndb-meta", dbOwner, dbOwner, dbFolder, dbName, ver)
		err = memCache.Delete(cacheKey)
		if err != nil {
			if err != memcache.ErrCacheMiss {
				// Cache miss is not an error we care about
				return err
			}
		}

		// Invalidate the download page data for public database versions
		cacheKey = MetadataCacheKey("dwndb-meta", "", dbOwner, dbFolder, dbName, ver)
		err = memCache.Delete(cacheKey)
		if err != nil {
			if err != memcache.ErrCacheMiss {
				// Cache miss is not an error we care about
				return err
			}
		}
	}

	return nil
}

// Generate a predictable cache key for metadata information
func MetadataCacheKey(prefix string, loggedInUser string, dbOwner string, dbFolder string, dbName string, dbVersion int) string {
	var cacheString string
	if loggedInUser == dbOwner {
		cacheString = fmt.Sprintf("%s/%s/%s/%s/%d", prefix, dbOwner, dbFolder, dbName, dbVersion)
	} else {
		// Requests for other users databases are cached separately from users own database requests
		cacheString = fmt.Sprintf("%s/pub/%s/%s/%s/%d", prefix, dbOwner, dbFolder, dbName, dbVersion)
	}

	tempArr := md5.Sum([]byte(cacheString))
	return hex.EncodeToString(tempArr[:])
}

// Generate a predictable cache key for SQLite row data
func TableRowsCacheKey(prefix string, loggedInUser string, dbOwner string, dbFolder string, dbName string, dbVersion int, dbTable string, rows int) string {
	var cacheString string
	if loggedInUser == dbOwner {
		cacheString = fmt.Sprintf("%s/%s/%s/%s/%d/%s/%d", prefix, dbOwner, dbFolder, dbName, dbVersion,
			dbTable, rows)
	} else {
		// Requests for other users databases are cached separately from users own database requests
		cacheString = fmt.Sprintf("%s/pub/%s/%s/%s/%d/%s/%d", prefix, dbOwner, dbFolder, dbName,
			dbVersion, dbTable, rows)
	}

	tempArr := md5.Sum([]byte(cacheString))
	return hex.EncodeToString(tempArr[:])
}
