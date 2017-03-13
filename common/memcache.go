package common

import (
	"bytes"
	"encoding/gob"
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
