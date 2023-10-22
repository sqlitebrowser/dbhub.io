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

type NumUploads struct {
	UploadDate string `json:"upload_date"`
	NumApi int `json:"num_api"`
	NumDB4S int `json:"num_db4s"`
	NumWebui  int `json:"num_webui"`
}

// UsageUserDiskSpaceHistorical returns the historical amount of disk space used by a given user
func UsageUserDiskSpaceHistorical(username string) (usage []NumDatabases, err error) {
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

// UsageUserDiskSpaceRecent returns the amount of disk space used by a given user over the last 30 days
func UsageUserDiskSpaceRecent(username string) (usage []NumDatabases, err error) {
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

// UsageUserUploadsHistorical returns the historical number of uploads by a given user
func UsageUserUploadsHistorical(username string) (usage []NumUploads, err error) {
	dbQuery := `
		WITH loggedIn AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		SELECT to_char(d.upload_date, 'YYYY-MM') AS "Upload date", d.server_sw, count(u.email)
		FROM database_uploads d, users u
		WHERE d.user_id = u.user_id
			AND u.user_id = (SELECT user_id FROM loggedIn)
		GROUP BY "Upload date", d.server_sw
		ORDER BY "Upload date"`
	rows, err := pdb.Query(context.Background(), dbQuery, username)
	if err != nil {
		log.Printf("Database query failed in %s: %v", GetCurrentFunctionName(), err)
		return
	}
	defer rows.Close()
	var tmpUploadCount NumUploads
	for rows.Next() {
		var date, serverSw string
		var uploadCount int
		err = rows.Scan(&date, &serverSw, &uploadCount)
		if err != nil {
			log.Printf("Error in %s when retrieving the historical upload data for for '%s': %v", GetCurrentFunctionName(), username, err)
			return nil, err
		}
		if tmpUploadCount.UploadDate != date {
			// We've moved on to a new date, so store the existing one we're processing in the output slice
			if tmpUploadCount.UploadDate != "" {
				usage = append(usage, tmpUploadCount)
			}

			// Clear the existing entries in the temporary upload data buffer
			tmpUploadCount.UploadDate = ""
			tmpUploadCount.NumApi = 0
			tmpUploadCount.NumDB4S = 0
			tmpUploadCount.NumWebui = 0
		}
		tmpUploadCount.UploadDate = date
		if serverSw == "api" && uploadCount != 0 {
			tmpUploadCount.NumApi = uploadCount
		}
		if serverSw == "db4s" && uploadCount != 0 {
			tmpUploadCount.NumDB4S = uploadCount
		}
		if serverSw == "webui" && uploadCount != 0 {
			tmpUploadCount.NumWebui = uploadCount
		}
	}

	// Add the final temporary entry to the output slice
	usage = append(usage, tmpUploadCount)
	return
}

// UsageUserUploadsRecent returns the number of uploads by a given user over the last 30 days
func UsageUserUploadsRecent(username string) (usage []NumUploads, err error) {
	dbQuery := `
		WITH loggedIn AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		SELECT to_char(d.upload_date, 'YYYY-MM') AS "Upload date", d.server_sw, count(u.email)
		FROM database_uploads d, users u
		WHERE d.user_id = u.user_id
			AND u.user_id = (SELECT user_id FROM loggedIn)
			AND d.upload_date > now() - interval '30 days'
		GROUP BY "Upload date", d.server_sw
		ORDER BY "Upload date"`
	rows, err := pdb.Query(context.Background(), dbQuery, username)
	if err != nil {
		log.Printf("Database query failed in %s: %v", GetCurrentFunctionName(), err)
		return
	}
	defer rows.Close()
	var tmpUploadCount NumUploads
	for rows.Next() {
		var date, serverSw string
		var uploadCount int
		err = rows.Scan(&date, &serverSw, &uploadCount)
		if err != nil {
			log.Printf("Error in %s when retrieving the historical upload data for for '%s': %v", GetCurrentFunctionName(), username, err)
			return nil, err
		}
		if tmpUploadCount.UploadDate != date {
			// We've moved on to a new date, so store the existing one we're processing in the output slice
			if tmpUploadCount.UploadDate != "" {
				usage = append(usage, tmpUploadCount)
			}

			// Clear the existing entries in the temporary upload data buffer
			tmpUploadCount.UploadDate = ""
			tmpUploadCount.NumApi = 0
			tmpUploadCount.NumDB4S = 0
			tmpUploadCount.NumWebui = 0
		}
		tmpUploadCount.UploadDate = date
		if serverSw == "api" && uploadCount != 0 {
			tmpUploadCount.NumApi = uploadCount
		}
		if serverSw == "db4s" && uploadCount != 0 {
			tmpUploadCount.NumDB4S = uploadCount
		}
		if serverSw == "webui" && uploadCount != 0 {
			tmpUploadCount.NumWebui = uploadCount
		}
	}

	// Add the final temporary entry to the output slice
	usage = append(usage, tmpUploadCount)
	return
}