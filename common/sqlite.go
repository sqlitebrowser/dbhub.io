package common

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strconv"

	"github.com/gwenn/gosqlite"
)

// Returns the number of rows in a SQLite table.
func GetSQLiteRowCount(sdb *sqlite.Conn, dbTable string) (int, error) {
	dbQuery := `SELECT count(*) FROM "` + dbTable + `"`
	var rowCount int
	err := sdb.OneValue(dbQuery, &rowCount)
	if err != nil {
		log.Printf("Error occurred when counting total rows for table '%s'.  Error: %s\n", dbTable, err)
		return 0, errors.New("Database query failure")
	}
	return rowCount, nil
}

// Reads up to maxRows number of rows from a given SQLite database table.  If maxRows < 0 (eg -1), then read all rows.
func ReadSQLiteDB(sdb *sqlite.Conn, dbTable string, maxRows int, sortCol string, sortDir string, rowOffset int) (SQLiteRecordSet, error) {
	return ReadSQLiteDBCols(sdb, dbTable, false, false, maxRows, sortCol, sortDir, rowOffset)
}

// Reads up to maxRows # of rows from a SQLite database.  Only returns the requested columns.
func ReadSQLiteDBCols(sdb *sqlite.Conn, dbTable string, ignoreBinary bool, ignoreNull bool, maxRows int,
	sortCol string, sortDir string, rowOffset int) (SQLiteRecordSet, error) {
	// Ugh, have to use string smashing for this, even though the SQL spec doesn't seem to say table names
	// shouldn't be parametrised.  Limitation from SQLite's implementation? :(
	var dataRows SQLiteRecordSet
	var err error
	var stmt *sqlite.Stmt

	// Make sure we don't try to index non-tables
	isTable := false
	tb, err := sdb.Tables("")
	if err != nil {
		return SQLiteRecordSet{}, err
	}
	for _, j := range tb {
		if dbTable == j {
			isTable = true
		}
	}

	// If a sort column was given, we check if the database (in the local cache) has an index on that column.  If it
	// doesn't, we create one
	// TODO: If no sortCol was given, but a rowOffset was, it's likely useful having an index (on any column?) anyway
	// TODO  It'd probably be good to check if that's the case, and add an index (on say the first column) if none are
	// TODO  already present
	if sortCol != "" && isTable == true {
		// Grab the list of indexes in the database
		idxList, err := sdb.Indexes("")
		if err != nil {
			return SQLiteRecordSet{}, err
		}

		// Look for indexes on the table we'll be querying
		idxFound := false
		for idx, tbl := range idxList {
			if tbl == dbTable {
				idxCol, err := sdb.IndexColumns("", idx)
				if err != nil {
					return SQLiteRecordSet{}, err
				}

				// Is the index on the column we're using
				if idxCol[0].Name == sortCol {
					idxFound = true
					break
				}
			}
		}

		// If no matching index was found, create one
		if !idxFound {
			// TODO: Index creation locks the database while it's happening, which I've seen cause further query
			// TODO  failures with "database is locked" errors (for a few seconds) in tableViewHandler().
			// TODO  eg when clicking two or three times quickly on a column heading in the database view.  If the
			// TODO  first click triggers index creation, then (on larger sized databases) index creation won't be
			// TODO  finished by the time the next click comes in and triggers queries.  We'll probably need to add
			// TODO  some detection/retry thing to the places where the failure shows up.
			dbQuery := sqlite.Mprintf("CREATE INDEX `%w_", dbTable)
			dbQuery += sqlite.Mprintf("%s_idx`", sortCol)
			dbQuery += sqlite.Mprintf(" ON `%s`", dbTable)
			dbQuery += sqlite.Mprintf(" (`%s`)", sortCol)
			err = sdb.Exec(dbQuery)
			if err != nil {
				log.Printf("Error occurred when creating index: %s\n", err.Error())
				return SQLiteRecordSet{}, err
			}
			sdb.Commit()
		}
	}

	// Set the table name
	dataRows.Tablename = dbTable

	// Construct the main SQL query
	dbQuery := sqlite.Mprintf(`SELECT * FROM "%w"`, dbTable)

	// If a sort column was given, include it
	if sortCol != "" {
		dbQuery += ` ORDER BY "%w"`
		dbQuery = sqlite.Mprintf(dbQuery, sortCol)
	}

	// If a sort direction was given, include it
	switch sortDir {
	case "ASC":
		dbQuery += " ASC"
	case "DESC":
		dbQuery += " DESC"
	}

	// If a row limit was given, add it
	if maxRows >= 0 {
		dbQuery = fmt.Sprintf("%s LIMIT %d", dbQuery, maxRows)
	}

	// If an offset was given, add it
	if rowOffset >= 0 {
		dbQuery = fmt.Sprintf("%s OFFSET %d", dbQuery, rowOffset)
	}

	// Use the sort column as needed
	stmt, err = sdb.Prepare(dbQuery)
	if err != nil {
		log.Printf("Error when preparing statement for database: %s\n", err)
		return dataRows, errors.New("Error when reading data from the SQLite database")
	}

	// Retrieve the field names
	dataRows.ColNames = stmt.ColumnNames()
	dataRows.ColCount = len(dataRows.ColNames)

	// Process each row
	fieldCount := -1
	err = stmt.Select(func(s *sqlite.Stmt) error {

		// Get the number of fields in the result
		if fieldCount == -1 {
			fieldCount = stmt.DataCount()
		}

		// Retrieve the data for each row
		var row []DataValue
		addRow := true
		for i := 0; i < fieldCount; i++ {
			// Retrieve the data type for the field
			fieldType := stmt.ColumnType(i)

			isNull := false
			switch fieldType {
			case sqlite.Integer:
				var val int
				val, isNull, err = s.ScanInt(i)
				if err != nil {
					log.Printf("Something went wrong with ScanInt(): %v\n", err)
					break
				}
				if !isNull {
					stringVal := fmt.Sprintf("%d", val)
					row = append(row, DataValue{Name: dataRows.ColNames[i], Type: Integer,
						Value: stringVal})
				}
			case sqlite.Float:
				var val float64
				val, isNull, err = s.ScanDouble(i)
				if err != nil {
					log.Printf("Something went wrong with ScanDouble(): %v\n", err)
					break
				}
				if !isNull {
					stringVal := strconv.FormatFloat(val, 'f', 4, 64)
					row = append(row, DataValue{Name: dataRows.ColNames[i], Type: Float,
						Value: stringVal})
				}
			case sqlite.Text:
				var val string
				val, isNull = s.ScanText(i)
				if !isNull {
					row = append(row, DataValue{Name: dataRows.ColNames[i], Type: Text,
						Value: val})
				}
			case sqlite.Blob:
				// BLOBs can be ignored (via flag to this function) for situations like the vis data
				if !ignoreBinary {
					_, isNull = s.ScanBlob(i)
					if !isNull {
						row = append(row, DataValue{Name: dataRows.ColNames[i], Type: Binary,
							Value: "<i>BINARY DATA</i>"})
					}
				} else {
					addRow = false
				}
			case sqlite.Null:
				isNull = true
			}
			if isNull && !ignoreNull {
				// NULLS can be ignored (via flag to this function) for situations like the vis data
				row = append(row, DataValue{Name: dataRows.ColNames[i], Type: Null,
					Value: "<i>NULL</i>"})
			}
			if isNull && ignoreNull {
				addRow = false
			}
		}
		if addRow == true {
			dataRows.Records = append(dataRows.Records, row)
			dataRows.RowCount++
		}

		return nil
	})
	if err != nil {
		log.Printf("Error when retrieving select data from database: %s\n", err)
		return dataRows, errors.New("Error when reading data from the SQLite database")
	}
	defer stmt.Finalize()

	// Add count of total rows to returned data
	tmpCount, err := GetSQLiteRowCount(sdb, dbTable)
	if err != nil {
		return dataRows, err
	}
	dataRows.RowCount = tmpCount

	// Fill out the sort column, direction, and row offset
	dataRows.SortCol = sortCol
	dataRows.SortDir = sortDir
	dataRows.Offset = rowOffset

	return dataRows, nil
}

// This is a specialised variation of the ReadSQLiteDB() function, just for our CSV exporting code. It'll probably
// need to be merged with the above function at some point.
func ReadSQLiteDBCSV(sdb *sqlite.Conn, dbTable string) ([][]string, error) {
	// Retrieve all of the data from the selected database table
	stmt, err := sdb.Prepare(`SELECT * FROM "` + dbTable + `"`)
	if err != nil {
		log.Printf("Error when preparing statement for database: %s\n", err)
		return nil, err
	}

	// Process each row
	fieldCount := -1
	var resultSet [][]string
	err = stmt.Select(func(s *sqlite.Stmt) error {

		// Get the number of fields in the result
		if fieldCount == -1 {
			fieldCount = stmt.DataCount()
		}

		// Retrieve the data for each row
		var row []string
		for i := 0; i < fieldCount; i++ {
			// Retrieve the data type for the field
			fieldType := stmt.ColumnType(i)

			isNull := false
			switch fieldType {
			case sqlite.Integer:
				var val int
				val, isNull, err = s.ScanInt(i)
				if err != nil {
					log.Printf("Something went wrong with ScanInt(): %v\n", err)
					break
				}
				if !isNull {
					row = append(row, fmt.Sprintf("%d", val))
				}
			case sqlite.Float:
				var val float64
				val, isNull, err = s.ScanDouble(i)
				if err != nil {
					log.Printf("Something went wrong with ScanDouble(): %v\n", err)
					break
				}
				if !isNull {
					row = append(row, strconv.FormatFloat(val, 'f', 4, 64))
				}
			case sqlite.Text:
				var val string
				val, isNull = s.ScanText(i)
				if !isNull {
					row = append(row, val)
				}
			case sqlite.Blob:
				var val []byte
				val, isNull = s.ScanBlob(i)
				if !isNull {
					// Base64 encode the value
					row = append(row, base64.StdEncoding.EncodeToString(val))
				}
			case sqlite.Null:
				isNull = true
			}
			if isNull {
				row = append(row, "NULL")
			}
		}
		resultSet = append(resultSet, row)

		return nil
	})
	if err != nil {
		log.Printf("Error when reading data from database: %s\n", err)
		return nil, err
	}
	defer stmt.Finalize()

	// Return the results
	return resultSet, nil
}

// Performs basic sanity checks of an uploaded database.
func SanityCheck(fileName string) error {
	// Perform a read on the database, as a basic sanity check to ensure it's really a SQLite database
	sqliteDB, err := sqlite.Open(fileName, sqlite.OpenReadOnly)
	if err != nil {
		log.Printf("Couldn't open database when sanity checking upload: %s", err)
		return errors.New("Internal error when uploading database")
	}
	defer sqliteDB.Close()
	tables, err := sqliteDB.Tables("")
	if err != nil {
		log.Printf("Error retrieving table names when sanity checking upload: %s", err)
		return errors.New("Error when sanity checking file.  Possibly encrypted or not a database?")
	}
	if len(tables) == 0 {
		// No table names were returned, so abort
		log.Print("The attemped upload failed, as it doesn't seem to have any tables.")
		return errors.New("Database has no tables?")
	}
	return nil
}

// Returns the list of tables and view in the SQLite database.
func Tables(sdb *sqlite.Conn, dbName string) ([]string, error) {
	// TODO: It might be useful to cache this info in PG or memcached
	// Retrieve the list of tables in the database
	tables, err := sdb.Tables("")
	if err != nil {
		log.Printf("Error retrieving table names: %v\n", err)
		if cerr, ok := err.(sqlite.ConnError); ok {
			log.Printf("Error code: %v\n", cerr.Code())
			log.Printf("Extended error code: %v\n", cerr.ExtendedCode())
			log.Printf("Extended error message: %v\n", cerr.Error())
			log.Printf("Extended error filename: %v\n", cerr.Filename())
		} else {
			log.Printf("Expected a connection error, but got a '%v'\n", reflect.TypeOf(cerr))
		}
		return nil, err
	}
	if len(tables) == 0 {
		// No table names were returned, so abort
		log.Printf("The database '%s' doesn't seem to have any tables. Aborting.", dbName)
		return nil, err
	}

	// Retrieve the list of views in the database
	vw, err := sdb.Views("")
	if err != nil {
		log.Printf("Error retrieving view names: %v\n", err)
		if cerr, ok := err.(sqlite.ConnError); ok {
			log.Printf("Error code: %v\n", cerr.Code())
			log.Printf("Extended error code: %v\n", cerr.ExtendedCode())
			log.Printf("Extended error message: %v\n", cerr.Error())
			log.Printf("Extended error filename: %v\n", cerr.Filename())
		} else {
			log.Printf("Expected a connection error, but got a '%v'\n", reflect.TypeOf(cerr))
		}
		return nil, err
	}

	// Merge the table and view arrays
	tables = append(tables, vw...)
	return tables, nil
}
