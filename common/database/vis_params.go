package database

import (
	"context"
	"errors"
	"log"

	pgx "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type VisParamsV2 struct {
	ChartType   string `json:"chart_type"`
	ShowXLabel  bool   `json:"show_x_label"`
	ShowYLabel  bool   `json:"show_y_label"`
	SQL         string `json:"sql"`
	XAXisColumn string `json:"x_axis_label"`
	YAXisColumn string `json:"y_axis_label"`
}

// GetVisualisations returns the saved visualisations for a given database
func GetVisualisations(dbOwner, dbName string) (visualisations map[string]VisParamsV2, err error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
				AND db_name = $2
		)
		SELECT name, parameters
		FROM vis_params as vis, u, d
		WHERE vis.db_id = d.db_id
			AND vis.user_id = u.user_id
		ORDER BY name`
	rows, e := DB.Query(context.Background(), dbQuery, dbOwner, dbName)
	if e != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// There weren't any saved visualisations for this database
			return
		}

		// A real database error occurred
		err = e
		log.Printf("Retrieving visualisation list for '%s/%s' failed: %v", dbOwner, dbName, e)
		return
	}
	defer rows.Close()

	visualisations = make(map[string]VisParamsV2)
	for rows.Next() {
		var n string
		var p VisParamsV2
		err = rows.Scan(&n, &p)
		if err != nil {
			log.Printf("Error retrieving visualisation list: %v", err.Error())
			return
		}

		visualisations[n] = p
	}
	return
}

// VisualisationDeleteParams deletes a set of visualisation parameters
func VisualisationDeleteParams(dbOwner, dbName, visName string) (err error) {
	var commandTag pgconn.CommandTag
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
				AND db_name = $2
		)
		DELETE FROM vis_params WHERE user_id = (SELECT user_id FROM u) AND db_id = (SELECT db_id FROM d) AND name = $3`
	commandTag, err = DB.Exec(context.Background(), dbQuery, dbOwner, dbName, visName)
	if err != nil {
		log.Printf("Deleting visualisation '%s' for database '%s/%s' failed: %v", visName,
			dbOwner, dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected while deleting visualisation '%s' for database '%s/%s'",
			numRows, visName, dbOwner, dbName)
	}
	return
}

// VisualisationRename renames an existing saved visualisation
func VisualisationRename(dbOwner, dbName, visName, visNewName string) (err error) {
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
				AND db_name = $2
		)
		UPDATE vis_params SET name = $4 WHERE user_id = (SELECT user_id FROM u) AND db_id = (SELECT db_id FROM d) AND name = $3`
	commandTag, err := DB.Exec(context.Background(), dbQuery, dbOwner, dbName, visName, visNewName)
	if err != nil {
		log.Printf("Renaming visualisation '%s' for database '%s/%s' failed: %v", visName,
			dbOwner, dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected while renaming visualisation '%s' for database '%s/%s'",
			numRows, visName, dbOwner, dbName)
	}
	return
}

// VisualisationSaveParams saves a set of visualisation parameters for later retrieval
func VisualisationSaveParams(dbOwner, dbName, visName string, visParams VisParamsV2) (err error) {
	var commandTag pgconn.CommandTag
	dbQuery := `
		WITH u AS (
			SELECT user_id
			FROM users
			WHERE lower(user_name) = lower($1)
		), d AS (
			SELECT db.db_id
			FROM sqlite_databases AS db, u
			WHERE db.user_id = u.user_id
				AND db_name = $2
		)
		INSERT INTO vis_params (user_id, db_id, name, parameters)
		SELECT (SELECT user_id FROM u), (SELECT db_id FROM d), $3, $4
		ON CONFLICT (db_id, user_id, name)
			DO UPDATE
			SET parameters = $4`
	commandTag, err = DB.Exec(context.Background(), dbQuery, dbOwner, dbName, visName, visParams)
	if err != nil {
		log.Printf("Saving visualisation '%s' for database '%s/%s' failed: %v", visName,
			dbOwner, dbName, err)
		return err
	}
	if numRows := commandTag.RowsAffected(); numRows != 1 {
		log.Printf("Wrong number of rows (%d) affected while saving visualisation '%s' for database '%s/%s'",
			numRows, visName, dbOwner, dbName)
	}
	return
}
