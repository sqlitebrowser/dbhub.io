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
	"strconv"
	"strings"

	"github.com/bradfitz/gomemcache/memcache"
)

var (
	// Connection handles
	memCache *memcache.Client
)

// Caches data in Memcached
func CacheData(cacheKey string, cacheData interface{}, cacheSeconds int) error {
	// Encode the data
	var encodedData bytes.Buffer
	enc := gob.NewEncoder(&encodedData)
	err := enc.Encode(cacheData)
	if err != nil {
		return err
	}

	// Send the data to memcached
	cachedData := memcache.Item{Key: cacheKey, Value: encodedData.Bytes(), Expiration: int32(cacheSeconds)}
	err = memCache.Set(&cachedData)
	if err != nil {
		return err
	}

	return nil
}

func ConnectCache() error {
	// Connect to memcached server
	memCache = memcache.New(Conf.Memcache.Server)

	// Test the memcached connection
	cacheTest := memcache.Item{Key: "connecttext", Value: []byte("1"), Expiration: 10}
	err := memCache.Set(&cacheTest)
	if err != nil {
		return errors.New(fmt.Sprintf("Couldn't connect to memcached server: %s", err))
	}

	// Log successful connection message for Memcached
	log.Printf("Connected to Memcached: %v\n", Conf.Memcache.Server)

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

// Retrieves the view count in memcached for a database
func GetViewCount(owner string, folder string, fileName string) (count int, err error) {
	// Generate the cache key
	cacheString := fmt.Sprintf("viewcount-%s-%s-%s", owner, folder, fileName)
	tempArr := md5.Sum([]byte(cacheString))
	cacheKey := hex.EncodeToString(tempArr[:])

	// Retrieve the view count
	data, err := memCache.Get(cacheKey)
	if err != nil {
		if err != memcache.ErrCacheMiss {
			// A real error occurred
			return -1, err
		}

		// There isn't a cached value for the database
		return -1, nil
	}

	// Convert the string value to int, and return it
	count, err = strconv.Atoi(string(data.Value))
	if err != nil {
		return -1, err
	}
	return count, nil
}

// Increments the view counter in memcached for a database
func IncrementViewCount(owner string, folder string, fileName string) error {
	// Generate the cache key
	cacheString := fmt.Sprintf("viewcount-%s-%s-%s", owner, folder, fileName)
	tempArr := md5.Sum([]byte(cacheString))
	cacheKey := hex.EncodeToString(tempArr[:])

	// Attempt to directly increment the counter
	_, err := memCache.Increment(cacheKey, 1)
	if err != nil {
		if err != memcache.ErrCacheMiss {
			// A real error occurred
			return err
		}

		// The cached value didn't exist, so we check if it has an entry in PostgreSQL already
		// NOTE: This function returns 0 if there's no existing entry, so we can just increment whatever it gives us
		cnt, err := ViewCount(owner, folder, fileName)
		if err != nil {
			return err
		}

		// It doesn't so we create a new memcached entry for it
		cachedData := memcache.Item{
			Key:        cacheKey,
			Value:      []byte(fmt.Sprintf("%d", cnt+1)),
			Expiration: int32(Conf.Memcache.DefaultCacheTime),
		}
		err = memCache.Set(&cachedData)
		if err != nil {
			return err
		}
	}
	return nil
}

// Invalidate memcache data for a database entry or entries
func InvalidateCacheEntry(loggedInUser string, owner string, folder string, fileName string, commitID string) error {
	// If commitID is "", that means "for all commits".  Otherwise, just invalidate the data for the requested one
	var commitList []string
	if commitID == "" {
		// Get the list of all commits for the given database
		var err error
		l, err := GetCommitList(owner, folder, fileName) // Get the full commit list
		if err != nil {
			return err
		}
		for i := range l {
			commitList = append(commitList, i)
		}
		commitList = append(commitList, "") // Add "" on the end, to indicate all entries
	} else {
		// Only one cached commit needs invalidation
		commitList = append(commitList, commitID)
	}

	// Loop around, invalidating the now outdated entries
	for _, c := range commitList {
		// Invalidate the meta info, for private database versions
		cacheKey := MetadataCacheKey("meta", loggedInUser, owner, folder, fileName, c)
		err := memCache.Delete(cacheKey)
		if err != nil {
			if err != memcache.ErrCacheMiss {
				// Cache miss is not an error we care about
				return err
			}
		}

		// Invalidate the meta info for public database versions
		cacheKey = MetadataCacheKey("meta", "", owner, folder, fileName, c)
		err = memCache.Delete(cacheKey)
		if err != nil {
			if err != memcache.ErrCacheMiss {
				// Cache miss is not an error we care about
				return err
			}
		}

		// Invalidate the download page data, for private database versions
		cacheKey = MetadataCacheKey("dwndb-meta", owner, owner, folder, fileName, c)
		err = memCache.Delete(cacheKey)
		if err != nil {
			if err != memcache.ErrCacheMiss {
				// Cache miss is not an error we care about
				return err
			}
		}

		// Invalidate the download page data for public database versions
		cacheKey = MetadataCacheKey("dwndb-meta", "", owner, folder, fileName, c)
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

// Returns the Memcached handle
func MemcacheHandle() *memcache.Client {
	return memCache
}

// Generate a predictable cache key for metadata information
func MetadataCacheKey(prefix string, loggedInUser string, owner string, folder string, fileName string, commitID string) string {
	var cacheString string
	if strings.ToLower(loggedInUser) == strings.ToLower(owner) {
		cacheString = fmt.Sprintf("%s/%s/%s/%s/%s", prefix, strings.ToLower(owner), folder, fileName, commitID)
	} else {
		// Requests for other users databases are cached separately from users own database requests
		cacheString = fmt.Sprintf("%s/pub/%s/%s/%s/%s", prefix, strings.ToLower(owner), folder, fileName, commitID)
	}
	tempArr := md5.Sum([]byte(cacheString))
	return hex.EncodeToString(tempArr[:])
}

// Increments the view counter in memcached for a database
func SetUserStatusUpdates(userName string, numUpdates int) error {
	// Generate the cache key
	cacheString := fmt.Sprintf("status-updates-%s", userName)
	tempArr := md5.Sum([]byte(cacheString))
	cacheKey := hex.EncodeToString(tempArr[:])

	// Create a memcached entry with the new user status updates count
	cachedData := memcache.Item{
		Key:        cacheKey,
		Value:      []byte(fmt.Sprintf("%d", numUpdates)),
		Expiration: int32(Conf.Memcache.DefaultCacheTime),
	}
	err := memCache.Set(&cachedData)
	if err != nil {
		return err
	}
	return nil
}

// Generate a predictable cache key for SQLite row data
func TableRowsCacheKey(prefix string, loggedInUser string, owner string, folder string, fileName string, commitID string, dbTable string, rows int) string {
	var cacheString string
	if strings.ToLower(loggedInUser) == strings.ToLower(owner) {
		cacheString = fmt.Sprintf("%s/%s/%s/%s/%s/%s/%d", prefix, strings.ToLower(owner), folder, fileName, commitID,
			dbTable, rows)
	} else {
		// Requests for other users databases are cached separately from users own database requests
		cacheString = fmt.Sprintf("%s/pub/%s/%s/%s/%s/%s/%d", prefix, strings.ToLower(owner), folder, fileName,
			commitID, dbTable, rows)
	}
	tempArr := md5.Sum([]byte(cacheString))
	return hex.EncodeToString(tempArr[:])
}

// Returns the number of status updates outstanding for a user
func UserStatusUpdates(userName string) (numUpdates int, err error) {
	// Generate the cache key
	cacheString := fmt.Sprintf("status-updates-%s", userName)
	tempArr := md5.Sum([]byte(cacheString))
	cacheKey := hex.EncodeToString(tempArr[:])

	// Retrieve the status updates counter
	data, err := memCache.Get(cacheKey)
	if err != nil {
		if err != memcache.ErrCacheMiss {
			// A real error occurred
			return 0, err
		}

		// There isn't a cached value for the user, so retrieve the list from PG and create an initial value
		lst, err := StatusUpdates(userName)
		if err != nil {
			return 0, err
		}
		for _, i := range lst {
			numUpdates += len(i)
		}

		// Set the initial number of updates
		cachedData := memcache.Item{
			Key:        cacheKey,
			Value:      []byte(fmt.Sprintf("%d", numUpdates)),
			Expiration: int32(Conf.Memcache.DefaultCacheTime),
		}
		err = memCache.Set(&cachedData)
		if err != nil {
			return 0, err
		}
		return numUpdates, nil
	}

	// Convert the string value to int, and return it
	numUpdates, err = strconv.Atoi(string(data.Value))
	if err != nil {
		return 0, err
	}
	return numUpdates, nil
}
