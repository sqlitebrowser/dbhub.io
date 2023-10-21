package common

import (
	"context"
	"log"
)

type NumDatabases struct {
	UsageDate string `json:"usage_date"`
	NumStd  int `json:"num_std"`
	NumLive int `json:"num_live"`
}

// UsageDiskSpaceUserHistorical returns the historical and current amount of disk space used by a given user
func UsageDiskSpaceUserHistorical(username string) (usage []NumDatabases, err error) {
	// FIXME: Manually verify this is indeed returning the highest value recorded in any month, just to be super safe
	dbQuery := `
		WITH loggedIn AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		SELECT to_char(analysis_date, 'YYYY-MM') AS "Usage date", max(standard_databases_bytes) / (1024*1024) AS "Standard databases", max(live_databases_bytes) / (1024*1024) AS "Live databases"
		FROM analysis_space_used a, users u
		WHERE a.user_id = u.user_id
			AND u.user_id = (SELECT user_id FROM loggedIn)
			AND standard_databases_bytes > 0
		GROUP BY "Usage date"
		ORDER BY "Usage date" ASC`
	rows, err := pdb.Query(context.Background(), dbQuery, username)
	if err != nil {
		log.Printf("Database query failed in %s: %v", GetCurrentFunctionName(), err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var date string
		var numStd, numLive int
		err = rows.Scan(&date, &numStd, &numLive)
		if err != nil {
			log.Printf("Error in %s when retrieving the disk space usage for '%s': %v", GetCurrentFunctionName(), username, err)
			return nil, err
		}
		usage = append(usage, NumDatabases{
			UsageDate: date,
			NumStd:  numStd,
			NumLive: numLive,
		})
	}
	return
}

// UsageDiskSpaceUserRecent returns the amount of disk space used by a given user over the last 30 days
func UsageDiskSpaceUserRecent(username string) (usage []NumDatabases, err error) {
	dbQuery := `
		WITH loggedIn AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		SELECT to_char(analysis_date, 'YYYY-MM-DD') AS "Usage date", standard_databases_bytes / (1024*1024) AS "Standard databases", live_databases_bytes / (1024*1024) AS "Live databases"
		FROM analysis_space_used a, users u
		WHERE a.user_id = u.user_id
			AND u.user_id = (SELECT user_id FROM loggedIn)
			AND standard_databases_bytes > 0
			AND analysis_date > now() - interval '30 days'
		ORDER BY "Usage date" ASC`
	rows, err := pdb.Query(context.Background(), dbQuery, username)
	if err != nil {
		log.Printf("Database query failed in %s: %v", GetCurrentFunctionName(), err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var date string
		var numStd, numLive int
		err = rows.Scan(&date, &numStd, &numLive)
		if err != nil {
			log.Printf("Error in %s when retrieving the disk space usage for '%s': %v", GetCurrentFunctionName(), username, err)
			return nil, err
		}
		usage = append(usage, NumDatabases{
			UsageDate: date,
			NumStd:  numStd,
			NumLive: numLive,
		})
	}
	return
}
