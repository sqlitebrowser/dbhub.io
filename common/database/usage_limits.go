package database

import (
	"context"
	"log"

	"github.com/sqlitebrowser/dbhub.io/common/config"
)

type RateLimit struct {
	Limit    int    `json:"limit"`    // Maximum number of tokens
	Period   string `json:"period"`   // Period after which tokens are restored
	Increase int    `json:"increase"` // Number of tokens restored after that period
}

// Model type for the usage_limits table
type UsageLimit struct {
	ID          int         `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	RateLimits  []RateLimit `json:"rate_limits"`
}

// AddDefaultUsageLimits adds the default usage limits to the system so the the default value for users is valid
func AddDefaultUsageLimits() (err error) {
	// Insert default and unlimited usage limits
	sql := `INSERT INTO usage_limits (id, name, description, rate_limits) VALUES
		(1, 'default', 'Default limits for new users', '[{"limit": 10, "period": "s", "increase": 10}]'),
		(2, 'unlimited', 'No usage limits (intended for testing and developers)', NULL),
		(3, 'banned', 'No access to the API at all', '[{"limit": 0, "period": "M", "increase": 0}]')
		ON CONFLICT (id) DO NOTHING`
	_, err = DB.Exec(context.Background(), sql)
	if err != nil {
		log.Printf("%v: error when adding default usage limits to the database: %v", config.Conf.Live.Nodename, err)
		return err
	}

	// Reset sequence
	sql = `SELECT setval(pg_get_serial_sequence('usage_limits', 'id'), coalesce(max(id) + 1, 1), false) FROM usage_limits`
	_, err = DB.Exec(context.Background(), sql)
	if err != nil {
		log.Printf("%v: error when resetting usage limits sequence: %v", config.Conf.Live.Nodename, err)
		return err
	}

	log.Printf("%v: default usage limits added", config.Conf.Live.Nodename)
	return nil
}

// RateLimitsForUser retrieves the rate limits for a user based on their configured usage limits.
func RateLimitsForUser(user string) (limits []RateLimit, err error) {
	query := `
		WITH userData AS (
			SELECT usage_limits_id
			FROM users
			WHERE lower(user_name) = lower($1)
		)
		SELECT coalesce(rate_limits, '[]'::jsonb) FROM usage_limits
		WHERE id=(SELECT usage_limits_id FROM userData)`
	err = DB.QueryRow(context.Background(), query, user).Scan(&limits)
	if err != nil {
		log.Printf("Querying usage limits failed for user '%s': %v", user, err)
		return nil, err
	}

	return
}

// GetUsageLimits returns a list of all usage limits
func GetUsageLimits() (usageLimits []UsageLimit, err error) {
	query := `SELECT id, name, description, rate_limits FROM usage_limits`
	rows, err := DB.Query(context.Background(), query)
	if err != nil {
		log.Printf("Database query failed: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var u UsageLimit
		err = rows.Scan(&u.ID, &u.Name, &u.Description, &u.RateLimits)
		if err != nil {
			log.Printf("Error retrieving usage limits list: %v", err)
			return
		}
		usageLimits = append(usageLimits, u)
	}

	return
}
