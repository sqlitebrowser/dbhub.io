package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/sqlitebrowser/dbhub.io/common/config"
)

// LogDB4SConnect creates a DB4S default browse list entry
func LogDB4SConnect(userAcc, ipAddr, userAgent string, downloadDate time.Time) error {
	if config.Conf.DB4S.Debug {
		log.Printf("User '%s' just connected with '%s' and generated the default browse list", userAcc, userAgent)
	}

	// If the user account isn't "public", then we look up the user id and store the info with the request
	userID := 0
	if userAcc != "public" {
		dbQuery := `
			SELECT user_id
			FROM users
			WHERE user_name = $1`

		err := DB.QueryRow(context.Background(), dbQuery, userAcc).Scan(&userID)
		if err != nil {
			log.Printf("Looking up the user ID failed: %v", err)
			return err
		}
		if userID == 0 {
			// The username wasn't found in our system
			return fmt.Errorf("The user wasn't found in our system!")
		}
	}

	// Store the high level connection info, so we can check for growth over time
	dbQuery := `
		INSERT INTO db4s_connects (user_id, ip_addr, user_agent, connect_date)
		VALUES ($1, $2, $3, $4)`
	commandTag, err := DB.Exec(context.Background(), dbQuery, userID, ipAddr, userAgent, downloadDate)
	if err != nil {
		log.Printf("Storing record of DB4S connection failed: %v", err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected while storing DB4S connection record for user '%s'", numRows, userAcc)
	}
	return nil
}
