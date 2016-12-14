package main

import (
	"bytes"
	"encoding/gob"
	"io"

	"github.com/bradfitz/gomemcache/memcache"
)

// Caches data in Memcached
func cacheData(cacheKey string, cacheData interface{}, cacheSeconds int32) error {
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

// Retrieves cached data from Memcached
func getCachedData(cacheKey string, cacheData interface{}) (bool, error) {
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
