package main

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	com "github.com/sqlitebrowser/dbhub.io/common"
	"github.com/sqlitebrowser/dbhub.io/common/database"
)

// Time in seconds for which the rate limit data is stored in the cache.
// 0 means never expire, otherwise the maximum is 30 days.
const cacheTime int = 0

// Interval for flushing the cached data of a user and reloading it from the database
// Reloading the data from database also reloads the assigned usage limits. So this is also the maximum
// time a user has to wait until a newly assigned limit is active unless the cache is cleared.
const reloadInterval time.Duration = 24 * time.Hour

type rateLimitCacheData struct {
	// These values reflect the applied settings from the usage_limits table
	Limit    int
	Period   time.Duration
	Increase int

	// These values maintain the current status for the user
	Remaining    int
	LastIncrease time.Time
}

type usageLimitCacheData struct {
	// Last time the data was reloaded from the database
	LastReload time.Time

	// State for rate limiting
	RateLimits []rateLimitCacheData
}

func limitPeriodToDuration(period string) time.Duration {
	if period == "s" { // 1 second
		return time.Second
	} else if period == "m" { // 1 minute
		return time.Minute
	} else if period == "h" { // 1 hour
		return time.Hour
	} else if period == "d" { // 1 day (= 24 hours)
		return time.Hour * 24
	} else if period == "M" { // 1 month (= 30 days)
		return time.Hour * 24 * 30
	} else { // Default is the maximum duration possible, i.e. practically never increasing
		return time.Duration(-1)
	}
}

func initialiseLimitDataFromDatabase(user string) (data usageLimitCacheData, err error) {
	// Retrieve limits for user
	limits, err := database.RateLimitsForUser(user)
	if err != nil {
		return
	}

	// Convert each rate limit to the cache format and figure out the correct number
	// of remaining tokens to start with
	for _, l := range limits {
		// Convert period string to duration
		period := limitPeriodToDuration(l.Period)

		// Get usage info for given period from database
		count, lastCall, errLimit := database.ApiUsageStatsLastPeriod(user, period)
		if err != nil {
			return data, errLimit
		}

		// If no calls were made in the period (i.e. maximum number of tokens remaining in this bucket) start counting tokens now
		if count == 0 {
			lastCall = time.Now()
		}

		data.RateLimits = append(data.RateLimits, rateLimitCacheData{
			// Store usage limits from database
			Limit:    l.Limit,
			Period:   period,
			Increase: l.Increase,

			// The number of remaining tokens in this bucket is the maximum number of tokens minus the number of API calls within the period
			Remaining: l.Limit - count,

			// The last increase of tokens must at least have happened on the last API call
			LastIncrease: lastCall,
		})
	}

	// Set last hit
	data.LastReload = time.Now()

	return
}

func limit(c *gin.Context) {
	// Get current user and build a cache key based on the user's name
	user := c.MustGet("user").(string)
	cacheKey := "limits-" + user

	// Try to retrieve usage limiting info for the current user from the cache
	var data usageLimitCacheData
	hit, err := com.GetCachedData(cacheKey, &data)
	if err != nil {
		log.Printf("Error retrieving usage limit data from cache for user '%s': %v", user, err)
		hit = false
	}

	// If no cached data could be found or it is too old, initialise the data from the database.
	// If cached data has been found, it needs to be updated.
	if !hit || time.Now().After(data.LastReload.Add(reloadInterval)) {
		// Get up-to-date values from the database
		data, err = initialiseLimitDataFromDatabase(user)
		if err != nil {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
	} else {
		// For information we got from the cache, the remaining number of tokens needs
		// to be increased first. This happens whenever the last increase time is more
		// time ago than the increase period.
		now := time.Now()
		for k, l := range data.RateLimits {
			if now.After(l.LastIncrease.Add(l.Period)) {
				data.RateLimits[k].Remaining += l.Increase * int(now.Sub(l.LastIncrease)/l.Period)
				if data.RateLimits[k].Remaining > l.Limit {
					data.RateLimits[k].Remaining = l.Limit
				}

				data.RateLimits[k].LastIncrease = now
			}
		}
	}

	// Check if any of the rate limits has no tokens remaining
	for _, l := range data.RateLimits {
		if l.Remaining <= 0 {
			c.AbortWithStatus(http.StatusTooManyRequests)
			return
		}
	}

	// Reduce remaining tokens
	for k := range data.RateLimits {
		data.RateLimits[k].Remaining -= 1
	}

	// Store updated data in cache
	err = com.CacheData(cacheKey, data, cacheTime)
	if err != nil {
		log.Printf("Error storing usage limit data to cache for user '%s': %v", user, err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	// No limits exceeded, so proceed with the API call
	c.Next()
}
