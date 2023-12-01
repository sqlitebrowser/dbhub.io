package common

import (
	"context"
	"log"
)

// NumAPIOps contains the breakdown of API operations (per operation type) for a date period
type NumAPIOps struct {
	OpDate           string `json:"op_date"`
	NumCommits       int    `json:"num_commits"`
	NumDatabases     int    `json:"num_databases"`
	NumDownload      int    `json:"num_download"`
	NumExecute       int    `json:"num_execute"`
	NumIndexes       int    `json:"num_indexes"`
	NumLiveDatabases int    `json:"num_live_databases"`
	NumMetadata      int    `json:"num_metadata"`
	NumQuery         int    `json:"num_query"`
	NumTables        int    `json:"num_tables"`
	NumTags          int    `json:"num_tags"`
	NumUpload        int    `json:"num_upload"`
}

type NumDatabases struct {
	UsageDate string `json:"usage_date"`
	NumStd    int    `json:"num_std"`
	NumLive   int    `json:"num_live"`
}

type NumTransfers struct {
	TransferDate string `json:"transfer_date"`
	NumApi       int    `json:"num_api"`
	NumDB4S      int    `json:"num_db4s"`
	NumWebui     int    `json:"num_webui"`
}

// UsageUserApiOps returns the number of API operations by a given user for the desired time period
func UsageUserApiOps(username string, recentOnly bool) (usage []NumAPIOps, err error) {
	dbQuery := `
		WITH loggedIn AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		SELECT to_char(api_call_date, 'YYYY-MM') AS "Date", api_operation AS "Operation", count(*)
		FROM api_call_log a
		WHERE a.caller_id = (SELECT user_id FROM loggedIn)`
	if recentOnly {
		dbQuery += `
		AND api_call_date > now() - interval '30 days'`
	}
	dbQuery += `
		GROUP BY "Date", "Operation"
		ORDER BY "Date"`
	rows, err := pdb.Query(context.Background(), dbQuery, username)
	if err != nil {
		log.Printf("Database query failed in %s: %v", GetCurrentFunctionName(), err)
		return
	}
	defer rows.Close()
	var tmpNumOps NumAPIOps
	for rows.Next() {
		var date, opName string
		var opCount int
		err = rows.Scan(&date, &opName, &opCount)
		if err != nil {
			log.Printf("Error in %s when retrieving the historical API operations data for '%s': %v", GetCurrentFunctionName(), username, err)
			return nil, err
		}
		if tmpNumOps.OpDate != date {
			// We've moved on to a new date, so store the existing one we're processing in the output slice
			if tmpNumOps.OpDate != "" {
				usage = append(usage, tmpNumOps)
			}

			// Clear the existing entries in the temporary upload data buffer
			tmpNumOps.OpDate = ""
			tmpNumOps.NumCommits = 0
			tmpNumOps.NumDatabases = 0
			tmpNumOps.NumDownload = 0
			tmpNumOps.NumExecute = 0
			tmpNumOps.NumIndexes = 0
			tmpNumOps.NumLiveDatabases = 0
			tmpNumOps.NumMetadata = 0
			tmpNumOps.NumQuery = 0
			tmpNumOps.NumTables = 0
			tmpNumOps.NumTags = 0
			tmpNumOps.NumUpload = 0
		}
		tmpNumOps.OpDate = date
		if opName == "commits" && opCount != 0 {
			tmpNumOps.NumCommits = opCount
		}
		if opName == "databases" && opCount != 0 {
			tmpNumOps.NumDatabases = opCount
		}
		if opName == "download" && opCount != 0 {
			tmpNumOps.NumDownload = opCount
		}
		if opName == "execute" && opCount != 0 {
			tmpNumOps.NumExecute = opCount
		}
		if opName == "indexes" && opCount != 0 {
			tmpNumOps.NumIndexes = opCount
		}
		if opName == "LIVE databases" && opCount != 0 {
			tmpNumOps.NumLiveDatabases = opCount
		}
		if opName == "metadata" && opCount != 0 {
			tmpNumOps.NumMetadata = opCount
		}
		if opName == "query" && opCount != 0 {
			tmpNumOps.NumQuery = opCount
		}
		if opName == "tables" && opCount != 0 {
			tmpNumOps.NumTables = opCount
		}
		if opName == "tags" && opCount != 0 {
			tmpNumOps.NumTags = opCount
		}
		if opName == "upload" && opCount != 0 {
			tmpNumOps.NumUpload = opCount
		}
	}

	// Add the final temporary entry to the output slice
	usage = append(usage, tmpNumOps)
	return
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
			NumStd:    numStd,
			NumLive:   numLive,
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
			NumStd:    numStd,
			NumLive:   numLive,
		})
	}
	return
}


// UsageUserNumDatabasesHistorical returns the historical number of databases in a given users' account
// NOTE - We haven't recorded this information historically, so will likely need to add a database table for the info first
func UsageUserNumDatabasesHistorical(username string) (usage []NumDatabases, err error) {
	dbQuery := `
		WITH loggedIn AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)

		SELECT count(*) AS "# of databases"
		FROM users u, sqlite_databases db
		WHERE u.user_id = db.user_id
			AND u.user_name NOT IN ('mkleusberg', 'justinclift', 'chrisjlocke')
		GROUP BY "Username"
		ORDER BY "# of databases" desc

		SELECT to_char(analysis_date, 'YYYY-MM') AS "Usage date", max(standard_databases_bytes) / (1024*1024) AS "Standard databases", max(live_databases_bytes) / (1024*1024) AS "Live databases"
		FROM analysis_space_used a, users u
		WHERE a.user_id = u.user_id
			AND u.user_id = (SELECT user_id FROM loggedIn)
			AND standard_databases_bytes > 0
		GROUP BY "Usage date"
		ORDER BY "Usage date" ASC


`
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
			NumStd:    numStd,
			NumLive:   numLive,
		})
	}
	return
}





// UsageUserUploadsHistorical returns the historical number of uploads by a given user
func UsageUserUploadsHistorical(username string) (usage []NumTransfers, err error) {
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
	var tmpUploadCount NumTransfers
	for rows.Next() {
		var date, serverSw string
		var uploadCount int
		err = rows.Scan(&date, &serverSw, &uploadCount)
		if err != nil {
			log.Printf("Error in %s when retrieving the historical upload data for '%s': %v", GetCurrentFunctionName(), username, err)
			return nil, err
		}
		if tmpUploadCount.TransferDate != date {
			// We've moved on to a new date, so store the existing one we're processing in the output slice
			if tmpUploadCount.TransferDate != "" {
				usage = append(usage, tmpUploadCount)
			}

			// Clear the existing entries in the temporary upload data buffer
			tmpUploadCount.TransferDate = ""
			tmpUploadCount.NumApi = 0
			tmpUploadCount.NumDB4S = 0
			tmpUploadCount.NumWebui = 0
		}
		tmpUploadCount.TransferDate = date
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
func UsageUserUploadsRecent(username string) (usage []NumTransfers, err error) {
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
	var tmpUploadCount NumTransfers
	for rows.Next() {
		var date, serverSw string
		var uploadCount int
		err = rows.Scan(&date, &serverSw, &uploadCount)
		if err != nil {
			log.Printf("Error in %s when retrieving the historical upload data for '%s': %v", GetCurrentFunctionName(), username, err)
			return nil, err
		}
		if tmpUploadCount.TransferDate != date {
			// We've moved on to a new date, so store the existing one we're processing in the output slice
			if tmpUploadCount.TransferDate != "" {
				usage = append(usage, tmpUploadCount)
			}

			// Clear the existing entries in the temporary upload data buffer
			tmpUploadCount.TransferDate = ""
			tmpUploadCount.NumApi = 0
			tmpUploadCount.NumDB4S = 0
			tmpUploadCount.NumWebui = 0
		}
		tmpUploadCount.TransferDate = date
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

// UsageUserDownloadsHistorical returns the historical number of downloads by a given user
func UsageUserDownloadsHistorical(username string) (usage []NumTransfers, err error) {
	dbQuery := `
		WITH loggedIn AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		SELECT to_char(d.download_date, 'YYYY-MM') AS "Download date", d.server_sw, count(u.email)
		FROM database_downloads d, users u
		WHERE d.user_id = u.user_id
			AND u.user_id = (SELECT user_id FROM loggedIn)
		GROUP BY "Download date", d.server_sw
		ORDER BY "Download date"`
	rows, err := pdb.Query(context.Background(), dbQuery, username)
	if err != nil {
		log.Printf("Database query failed in %s: %v", GetCurrentFunctionName(), err)
		return
	}
	defer rows.Close()
	var tmpUploadCount NumTransfers
	for rows.Next() {
		var date, serverSw string
		var uploadCount int
		err = rows.Scan(&date, &serverSw, &uploadCount)
		if err != nil {
			log.Printf("Error in %s when retrieving the historical upload data for '%s': %v", GetCurrentFunctionName(), username, err)
			return nil, err
		}
		if tmpUploadCount.TransferDate != date {
			// We've moved on to a new date, so store the existing one we're processing in the output slice
			if tmpUploadCount.TransferDate != "" {
				usage = append(usage, tmpUploadCount)
			}

			// Clear the existing entries in the temporary upload data buffer
			tmpUploadCount.TransferDate = ""
			tmpUploadCount.NumApi = 0
			tmpUploadCount.NumDB4S = 0
			tmpUploadCount.NumWebui = 0
		}
		tmpUploadCount.TransferDate = date
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

// UsageUserDownloadsRecent returns the number of downloads by a given user over the last 30 days
func UsageUserDownloadsRecent(username string) (usage []NumTransfers, err error) {
	dbQuery := `
		WITH loggedIn AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		SELECT to_char(d.download_date, 'YYYY-MM') AS "Download date", d.server_sw, count(u.email)
		FROM database_downloads d, users u
		WHERE d.user_id = u.user_id
			AND u.user_id = (SELECT user_id FROM loggedIn)
			AND d.download_date > now() - interval '30 days'
		GROUP BY "Download date", d.server_sw
		ORDER BY "Download date"`
	rows, err := pdb.Query(context.Background(), dbQuery, username)
	if err != nil {
		log.Printf("Database query failed in %s: %v", GetCurrentFunctionName(), err)
		return
	}
	defer rows.Close()
	var tmpUploadCount NumTransfers
	for rows.Next() {
		var date, serverSw string
		var uploadCount int
		err = rows.Scan(&date, &serverSw, &uploadCount)
		if err != nil {
			log.Printf("Error in %s when retrieving the historical upload data for '%s': %v", GetCurrentFunctionName(), username, err)
			return nil, err
		}
		if tmpUploadCount.TransferDate != date {
			// We've moved on to a new date, so store the existing one we're processing in the output slice
			if tmpUploadCount.TransferDate != "" {
				usage = append(usage, tmpUploadCount)
			}

			// Clear the existing entries in the temporary upload data buffer
			tmpUploadCount.TransferDate = ""
			tmpUploadCount.NumApi = 0
			tmpUploadCount.NumDB4S = 0
			tmpUploadCount.NumWebui = 0
		}
		tmpUploadCount.TransferDate = date
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
