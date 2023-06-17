package common

import (
	"context"
	"log"
	"time"
)

type NumDatabases struct {
	NumStd  int `json:"num_std"`
	NumLive int `json:"num_live"`
}

// UsageDiskSpaceUser returns the historical and current amount of disk space used by a given user
func UsageDiskSpaceUser(username string) (usage map[time.Time]NumDatabases, err error) {
	// FIXME: Adjust this query to return the highest value from *any* date in a month, rather than just sample the 1st of the month
	dbQuery := `
		WITH loggedIn AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		SELECT to_char(analysis_date, 'YYYY-MM') AS "Usage date", standard_databases_bytes / (1024*1024) AS "Standard databases", live_databases_bytes / (1024*1024) AS "Live databases"
		FROM analysis_space_used a, users u
		WHERE a.user_id = u.user_id
			AND u.user_id = (SELECT user_id FROM loggedIn)
			AND standard_databases_bytes > 0
			AND to_char(analysis_date, 'DD') = '01'`
	rows, err := pdb.Query(context.Background(), dbQuery, username)
	if err != nil {
		log.Printf("Database query failed in %s: %v", GetCurrentFunctionName(), err)
		return
	}
	defer rows.Close()
	if len(usage) == 0 {
		usage = make(map[time.Time]NumDatabases)
	}
	for rows.Next() {
		var date time.Time
		var numStd, numLive int
		err = rows.Scan(&date, &numStd, &numLive)
		if err != nil {
			log.Printf("Error in %s when retrieving the disk space usage for '%s': %v", GetCurrentFunctionName(), username, err)
			return nil, err
		}
		usage[date] = NumDatabases{
			NumStd:  numStd,
			NumLive: numLive,
		}
	}
	return
}
