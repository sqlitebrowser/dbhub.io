package database

import (
	"context"
	"log"
	"time"
)

// AnalysisRecordUserStorage adds a record to the backend database containing the amount of storage space used by a user
func AnalysisRecordUserStorage(userName string, recordDate time.Time, spaceUsedStandard, spaceUsedLive int64) (err error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		INSERT INTO analysis_space_used (user_id, analysis_date, standard_databases_bytes, live_databases_bytes)
		VALUES ((SELECT user_id FROM u), $2, $3, $4)`
	commandTag, err := DB.Exec(context.Background(), dbQuery, userName, recordDate, spaceUsedStandard, spaceUsedLive)
	if err != nil {
		log.Printf("Adding record of storage space used by '%s' failed: %s", userName, err)
		return
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected when recording the storage space used by '%s'", numRows, userName)
	}
	return
}
