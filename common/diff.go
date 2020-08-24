package common

import (
	"fmt"
	"io"
	"log"
	"sort"
	"strings"

	sqlite "github.com/gwenn/gosqlite"
)

// DiffType specifies the type of change in a row or object
type DiffType string

const (
	// ActionAdd is used for inserted rows and created objects
	ActionAdd DiffType = "add"

	// ActionDelete is used for deleted rows and dropped objects
	ActionDelete DiffType = "delete"

	// ActionModify is used for updated rows and altered objects
	ActionModify DiffType = "modify"
)

// MergeStrategy specifies the type of SQL statements included in the diff results.
// The SQL statements can be used for merging databases and depending on whether and
// how you want to merge you should choose your merge strategy.
type MergeStrategy int

const (
	// NoMerge removes any SQL statements for merging from the diff results
	NoMerge MergeStrategy = iota

	// PreservePkMerge produces SQL statements which preserve the values of the primary key columns.
	// Executing these statements on the first database produces a database similar to the second.
	PreservePkMerge

	// NewPkMerge produces SQL statements which generate new values for the primary key columns when
	// executed. This avoids a couple of possible conflicts and allows merging more distant databases.
	NewPkMerge
)

// SchemaDiff describes the changes to the schema of a database object, i.e. a created, dropped or altered object
type SchemaDiff struct {
	ActionType DiffType `json:"action_type"`
	Sql        string   `json:"sql,omitempty"`
}

// DataDiff stores a single change in the data of a table, i.e. a single new, deleted, or changed row
type DataDiff struct {
	ActionType DiffType      `json:"action_type"`
	Sql        string        `json:"sql,omitempty"`
	Pk         []DataValue   `json:"pk"`
	DataBefore []interface{} `json:"data_before,omitempty"`
	DataAfter  []interface{} `json:"data_after,omitempty"`
}

// DiffObjectChangeset stores all the differences between two objects in a database, for example two tables.
// Both Schema and Data are optional and can be nil if there are no respective changes in this object.
type DiffObjectChangeset struct {
	ObjectName string      `json:"object_name"`
	ObjectType string      `json:"object_type"`
	Schema     *SchemaDiff `json:"schema,omitempty"`
	Data       []DataDiff  `json:"data,omitempty"`
}

// Diffs is able to store all the differences between two databases.
type Diffs struct {
	Diff []DiffObjectChangeset `json:"diff"`
	// TODO Add PRAGMAs here
}

// Diff generates the differences between the two commits commitA and commitB of the two databases specified in the other parameters
func Diff(ownerA string, folderA string, nameA string, commitA string, ownerB string, folderB string, nameB string, commitB string, loggedInUser string, merge MergeStrategy, includeData bool) (Diffs, error) {
	// Check if the user has access to the requested databases
	bucketA, idA, _, err := MinioLocation(ownerA, folderA, nameA, commitA, loggedInUser)
	if err != nil {
		return Diffs{}, err
	}
	bucketB, idB, _, err := MinioLocation(ownerB, folderB, nameB, commitB, loggedInUser)
	if err != nil {
		return Diffs{}, err
	}

	// Sanity check
	if idA == "" {
		// The requested database wasn't found, or the user doesn't have permission to access it
		err = fmt.Errorf("Requested database not found")
		log.Printf("Requested database not found: '%s%s%s'", ownerA, folderA, nameA)
		return Diffs{}, err
	}
	if idB == "" {
		// The requested database wasn't found, or the user doesn't have permission to access it
		err = fmt.Errorf("Requested database not found")
		log.Printf("Requested database not found: '%s%s%s'", ownerB, folderB, nameB)
		return Diffs{}, err
	}

	// Retrieve database files from Minio, using locally cached version if it's already there
	dbA, err := RetrieveDatabaseFile(bucketA, idA)
	if err != nil {
		return Diffs{}, err
	}
	dbB, err := RetrieveDatabaseFile(bucketB, idB)
	if err != nil {
		return Diffs{}, err
	}

	// Call dbDiff which does the actual diffing of the database files
	return dbDiff(dbA, dbB, merge, includeData)
}

// dbDiff generates the differences between the two database files in dbA and dbD
func dbDiff(dbA string, dbB string, merge MergeStrategy, includeData bool) (Diffs, error) {
	var diff Diffs

	// Check if this is the same database and exit early
	if dbA == dbB {
		return diff, nil
	}

	// Open the first SQLite database in read only mode
	var sdb *sqlite.Conn
	sdb, err := sqlite.Open(dbA, sqlite.OpenReadOnly)
	if err != nil {
		log.Printf("Couldn't open database in dbDiff(): %s", err)
		return Diffs{}, err
	}
	if err = sdb.EnableExtendedResultCodes(true); err != nil {
		log.Printf("Couldn't enable extended result codes in dbDiff(): %v\n", err.Error())
		return Diffs{}, err
	}

	// Attach the second database
	err = sdb.Exec("ATTACH '" + dbB + "' AS aux")
	if err != nil {
		log.Printf("Couldn't attach database in dbDiff(): %s", err)
		return Diffs{}, err
	}

	// Get list of all objects in both databases, excluding virtual tables because they tend to be unpredictable
	var stmt *sqlite.Stmt
	stmt, err = sdb.Prepare("SELECT name, type FROM main.sqlite_master WHERE name NOT LIKE 'sqlite_%' AND (type != 'table' OR (type = 'table' AND sql NOT LIKE 'CREATE VIRTUAL%%'))\n" +
		" UNION\n" +
		"SELECT name, type FROM aux.sqlite_master WHERE name NOT LIKE 'sqlite_%' AND (type != 'table' OR (type = 'table' AND sql NOT LIKE 'CREATE VIRTUAL%%'))\n" +
		" ORDER BY name")
	if err != nil {
		log.Printf("Error when preparing statement for object list in dbDiff(): %s\n", err)
		return Diffs{}, err
	}
	defer stmt.Finalize()
	err = stmt.Select(func(s *sqlite.Stmt) error {
		objectName, _ := s.ScanText(0)
		objectType, _ := s.ScanText(1)
		changed, objectDiff, err := diffSingleObject(sdb, objectName, objectType, merge, includeData)
		if err != nil {
			return err
		}
		if changed {
			diff.Diff = append(diff.Diff, objectDiff)
		}
		return nil
	})
	if err != nil {
		log.Printf("Error when diffing single object in dbDiff: %s\n", err)
		return Diffs{}, err
	}

	// Sort changes by object type to make sure it is possible to execute them in the returned order.
	// For this it should be enough to always put tables in the first and triggers in the last position
	sort.SliceStable(diff.Diff, func(i, j int) bool {
		if diff.Diff[i].ObjectType == "table" && diff.Diff[j].ObjectType != "table" {
			return true
		} else if diff.Diff[j].ObjectType == "trigger" && diff.Diff[i].ObjectType != "trigger" {
			return true
		}

		return false
	})

	// TODO Check for differences in the PRAGMAs of both databases

	// Return
	return diff, nil
}

// diffSingleObject compares the object with name objectName and of type objectType in the main and aux schemata of the connection sdb
// and returns three values: a boolean to indicate whether there are differences, a DiffObjectChangeset object containing all the differences, and an optional error object
func diffSingleObject(sdb *sqlite.Conn, objectName string, objectType string, merge MergeStrategy, includeData bool) (bool, DiffObjectChangeset, error) {
	// Prepare diff object to return
	var diff DiffObjectChangeset
	diff.ObjectName = objectName
	diff.ObjectType = objectType

	// Check for object's existence in both databases
	var sqlInMain, sqlInAux string
	err := sdb.OneValue("SELECT sql FROM main.sqlite_master WHERE name = ? AND type = ?", &sqlInMain, objectName, objectType)
	if err != nil && err != io.EOF { // io.EOF is okay. It is returned when the object does not exist in the main database
		return false, DiffObjectChangeset{}, err
	}
	err = sdb.OneValue("SELECT sql FROM aux.sqlite_master WHERE name = ? AND type = ?", &sqlInAux, objectName, objectType)
	if err != nil && err != io.EOF { // io.EOF is okay. It is returned when the object does not exist in the aux database
		return false, DiffObjectChangeset{}, err
	}

	// Check for dropped object
	if sqlInMain != "" && sqlInAux == "" {
		diff.Schema = &SchemaDiff{ActionType: ActionDelete}
		if merge != NoMerge {
			diff.Schema.Sql = "DROP " + strings.ToUpper(objectType) + " " + EscapeId(objectName) + ";"
		}

		// If this is a table, also add all the deleted data to the diff
		if objectType == "table" {
			// We never include the SQL statements because there is no need to delete all the rows when we DROP the table anyway
			diff.Data, err = dataDiffForAllTableRows(sdb, "main", objectName, ActionDelete, false, includeData)
			if err != nil {
				return false, DiffObjectChangeset{}, err
			}
		}

		// No further changes for dropped objects. So we can return here
		return true, diff, nil
	}

	// Check for added object
	if sqlInMain == "" && sqlInAux != "" {
		diff.Schema = &SchemaDiff{ActionType: ActionAdd}
		if merge != NoMerge {
			diff.Schema.Sql = sqlInAux + ";"
		}

		// If this is a table, also add all the added data to the diff
		if objectType == "table" {
			diff.Data, err = dataDiffForAllTableRows(sdb, "aux", objectName, ActionAdd, merge != NoMerge, includeData)
			if err != nil {
				return false, DiffObjectChangeset{}, err
			}
		}

		// No further changes for created objects. So we can return here
		return true, diff, nil
	}

	// Check for modified object
	if sqlInMain != "" && sqlInAux != "" && sqlInMain != sqlInAux {
		diff.Schema = &SchemaDiff{ActionType: ActionModify}
		if merge != NoMerge {
			diff.Schema.Sql = "DROP " + strings.ToUpper(objectType) + " " + EscapeId(objectName) + ";" + sqlInAux + ";"
		}

		// TODO If this is a table, be more clever and try to get away with ALTER TABLE instead of DROP and CREATE

		// If this is a table, also add all the data to the diff
		if objectType == "table" {
			deleteData, err := dataDiffForAllTableRows(sdb, "main", objectName, ActionDelete, false, includeData)
			if err != nil {
				return false, DiffObjectChangeset{}, err
			}
			addData, err := dataDiffForAllTableRows(sdb, "aux", objectName, ActionAdd, merge != NoMerge, includeData)
			if err != nil {
				return false, DiffObjectChangeset{}, err
			}
			diff.Data = append(deleteData, addData...)
		}

		// No further changes for modified objects. So we can return here
		return true, diff, nil
	}

	// If this is a table, check for modified data
	if objectType == "table" {
		diff.Data, err = dataDiffForModifiedTableRows(sdb, objectName, merge, includeData)
		if err != nil {
			return false, DiffObjectChangeset{}, err
		}

		// When there are data changes, fill in the rest of the diff information and return the diff
		if diff.Data != nil {
			diff.ObjectName = objectName
			diff.ObjectType = objectType

			return true, diff, nil
		}
	}

	// Nothing has changed
	return false, DiffObjectChangeset{}, nil
}

func dataDiffForAllTableRows(sdb *sqlite.Conn, schemaName string, tableName string, action DiffType, includeSql bool, includeData bool) (diff []DataDiff, err error) {
	// Retrieve a list of all primary key columns and other columns in this table
	pk, implicitPk, otherColumns, err := GetPrimaryKeyAndOtherColumns(sdb, schemaName, tableName)
	if err != nil {
		return nil, err
	}

	// Escape all the column names
	pkEscaped := EscapeIds(pk)
	otherEscaped := EscapeIds(otherColumns)

	// Prepare query for the primary keys of all rows in this table. Only include the rest of the data
	// in the rows if required
	query := "SELECT " + strings.Join(pkEscaped, ",")
	if (includeSql && action == ActionAdd) || includeData {
		if len(otherEscaped) > 0 {
			query += "," + strings.Join(otherEscaped, ",")
		}
	}
	query += " FROM " + EscapeId(schemaName) + "." + EscapeId(tableName)

	// Retrieve data and add it to the data diff object
	_, _, data, err := SQLiteRunQuery(sdb, Internal, query, false, false)
	if err != nil {
		log.Printf("Error getting rows in dataDiffForAllTableRows(): %s\n", err)
		return nil, err
	}
	for _, row := range data.Records {
		var d DataDiff
		d.ActionType = action

		// Prepare SQL statement when needed
		if includeSql {
			if action == ActionDelete {
				d.Sql = "DELETE FROM " + EscapeId(tableName) + " WHERE "
			} else if action == ActionAdd {
				var insertColumns []string
				// Don't include rowid column, only regular PK
				if !implicitPk {
					insertColumns = append(insertColumns, pkEscaped...)
				}
				insertColumns = append(insertColumns, otherEscaped...)

				d.Sql = "INSERT INTO " + EscapeId(tableName) + "(" + strings.Join(insertColumns, ",") + ") VALUES("
			}
		}

		// Get primary key data
		for i := 0; i < data.ColCount; i++ {
			// If this column is still part of the primary key, add it to the data diff
			if i < len(pk) {
				d.Pk = append(d.Pk, row[i])
			}

			// If we want to include a SQL statement for deleting data and this is still
			// part of the primary key, add this to the prepared DELETE statement
			if includeSql && action == ActionDelete && i < len(pk) {
				d.Sql += pkEscaped[i]
				if row[i].Type == Null {
					d.Sql += " IS NULL"
				} else {
					d.Sql += "=" + EscapeValue(row[i])
				}
				d.Sql += " AND "
			}

			// If we want to include a SQL statement for adding data and this is the regular
			// data part, add this to the prepared INSERT statement
			if includeSql && action == ActionAdd && i >= len(pk) {
				d.Sql += EscapeValue(row[i]) + ","
			}

			// If we want to include all data, add this to the row data
			if includeData {
				if action == ActionAdd {
					d.DataAfter = append(d.DataAfter, row[i].Value)
				} else if action == ActionDelete {
					d.DataBefore = append(d.DataBefore, row[i].Value)
				}
			}
		}

		// Remove the last " AND " of the SQL query for DELETE statements and the last "," for INSERT statements
		// and add a semicolon instead
		if includeSql {
			if action == ActionDelete {
				d.Sql = strings.TrimSuffix(d.Sql, " AND ") + ";"
			} else if action == ActionAdd {
				d.Sql = strings.TrimSuffix(d.Sql, ",") + ");"
			}
		}

		// Add row to data diff set
		diff = append(diff, d)
	}

	return diff, nil
}

// This helper function gets the differences between the two tables named tableName in the main and in the aux schema.
// It then builds an array of DataDiff objects which represents these differences. This function assumes that the table
// schemas match.
func dataDiffForModifiedTableRows(sdb *sqlite.Conn, tableName string, merge MergeStrategy, includeData bool) (diff []DataDiff, err error) {
	// Retrieve a list of all primary key columns and other columns in this table
	pk, implicitPk, otherColumns, err := GetPrimaryKeyAndOtherColumns(sdb, "aux", tableName)
	if err != nil {
		return nil, err
	}

	// If we need to produce merge statements using the NewPkMerge strategy we need to know if we can rely on SQLite
	// to generate new primary keys or if we must generate them on our own.
	var incrementingPk bool
	if merge == NewPkMerge {
		incrementingPk, err = hasIncrementingIntPk(sdb, "aux", tableName)
		if err != nil {
			return nil, err
		}
	}

	// Escape all column names
	pkEscaped := EscapeIds(pk)
	otherEscaped := EscapeIds(otherColumns)

	// Build query for getting differences. This is based on the query produced by the sqldiff utility for SQLite.
	// The resulting query returns n+1+m*2 number of rows where n is the number of columns in the primary key and
	// m is the number of columns in the table which are not part of the primary key. The extra column between the
	// primary key columns and the other columns contains the diff type as specified by the DiffType constants.
	// We generate two columns for each table column. The first item of each pair is the old value and and the
	// second item is the new value.

	var query string

	// Updated rows
	// There can only be updated rows in tables with more columns than the primary key columns
	if len(otherColumns) > 0 {
		query = "SELECT "
		for _, c := range pkEscaped { // Primary key columns first
			query += "B." + c + ","
		}
		query += "'" + string(ActionModify) + "'" // Updated row
		for _, c := range otherEscaped {          // Other columns last
			query += ",A." + c + ",B." + c
		}

		query += " FROM main." + EscapeId(tableName) + " A, aux." + EscapeId(tableName) + " B WHERE "

		for _, c := range pkEscaped { // Where all primary key columns equal
			query += "A." + c + "=B." + c + " AND "
		}

		query += "(" // And at least one of the other columns differs
		for _, c := range otherEscaped {
			query += "A." + c + " IS NOT B." + c + " OR "
		}
		query = strings.TrimSuffix(query, " OR ") + ")"

		query += " UNION ALL "
	}

	// Deleted rows
	query += "SELECT "
	for _, c := range pkEscaped { // Primary key columns first. This needs to be from the first table for deleted rows
		query += "A." + c + ","
	}
	query += "'" + string(ActionDelete) + "'" // Deleted row
	for _, c := range otherEscaped {          // Other columns last
		query += ",A." + c + ",NULL"
	}

	query += " FROM main." + EscapeId(tableName) + " A WHERE "

	query += "NOT EXISTS(SELECT 1 FROM aux." + EscapeId(tableName) + " B WHERE " // Where a row with the same primary key doesn't exist in the second table
	for _, c := range pkEscaped {
		query += "A." + c + " IS B." + c + " AND "
	}
	query = strings.TrimSuffix(query, " AND ") + ") UNION ALL "

	// Inserted rows
	query += "SELECT "
	for _, c := range pkEscaped { // Primary key columns first. This needs to be from the second table for inserted rows
		query += "B." + c + ","
	}
	query += "'" + string(ActionAdd) + "'" // Inserted row
	for _, c := range otherEscaped {       // Other columns last
		query += ",NULL,B." + c
	}

	query += " FROM aux." + EscapeId(tableName) + " B WHERE "

	query += "NOT EXISTS(SELECT 1 FROM main." + EscapeId(tableName) + " A WHERE " // Where a row with the same primary key doesn't exist in the first table
	for _, c := range pkEscaped {
		query += "A." + c + " IS B." + c + " AND "
	}
	query = strings.TrimSuffix(query, " AND ") + ")"

	// Finish query
	query += " ORDER BY 1;" // Order by first primary key column

	// Run the query and retrieve the data. Each row in the result set is a difference between the two tables.
	// The column after the primary key columns and before the other columns specifies the type of change, i.e.
	// update, insert or delete. While the primary key bit of the DataDiff object we create for each row can
	// be taken directly from the first couple of columns and the action type of the DataDiff object can be
	// deduced from the type column in a straightforward way, the generated SQL statements for merging highly
	// depend on the diff type. Additionally, we need to respect the merge strategy when producing the SQL in
	// the DataDiff object.

	// Retrieve data and generate a new DataDiff object for each row
	_, _, data, err := SQLiteRunQuery(sdb, Internal, query, false, false)
	if err != nil {
		log.Printf("Error getting rows in dataDiffForModifiedTableRows(): %s\n", err)
		return nil, err
	}
	for _, row := range data.Records {
		var d DataDiff

		// Get the diff type
		d.ActionType = DiffType(row[len(pk)].Value.(string))

		// Fill in the primary key columns
		for i := 0; i < len(pk); i++ {
			d.Pk = append(d.Pk, row[i])
		}

		// If we want to include all data, add this to the row data
		if includeData {
			if d.ActionType == ActionAdd {
				for i := 0; i < len(pk); i++ {
					d.DataAfter = append(d.DataAfter, row[i].Value)
				}
				for i := len(pk) + 1; i < len(row); i += 2 {
					d.DataAfter = append(d.DataAfter, row[i+1].Value)
				}
			} else if d.ActionType == ActionDelete {
				for i := 0; i < len(pk); i++ {
					d.DataBefore = append(d.DataBefore, row[i].Value)
				}
				for i := len(pk) + 1; i < len(row); i += 2 {
					d.DataBefore = append(d.DataBefore, row[i].Value)
				}
			} else if d.ActionType == ActionModify {
				for i := 0; i < len(pk); i++ {
					d.DataBefore = append(d.DataBefore, row[i].Value)
					d.DataAfter = append(d.DataAfter, row[i].Value)
				}
				for i := len(pk) + 1; i < len(row); i += 2 {
					d.DataBefore = append(d.DataBefore, row[i].Value)
					d.DataAfter = append(d.DataAfter, row[i+1].Value)
				}
			}
		}

		// Produce the SQL statement for merging
		if merge != NoMerge {
			if d.ActionType == ActionModify || d.ActionType == ActionDelete {
				// For updated and deleted rows the merge strategy doesn't matter

				// The first part of the UPDATE and DELETE statements is different
				if d.ActionType == ActionModify {
					d.Sql = "UPDATE " + EscapeId(tableName) + " SET "

					// For figuring out which values to set, start with the first column after the diff type column.
					// It specifies whether the value of the first data column has changed. If it has, we set that
					// column to the new value which is stored in the following column of the row. Because each
					// comparison takes two fields (one for marking differences and one for the new value), we move
					// forward in steps of two columns.
					for i := len(pk) + 1; i < len(row); i += 2 {
						// Only include field when it was updated
						if row[i].Value != row[i+1].Value {
							// From the column number in the results of the difference query we calculate the
							// corresponding array index in the array of non-primary key columns. The new
							// value is stored in the next column of the result set.
							d.Sql += otherEscaped[(i-len(pk)-1)/2] + "=" + EscapeValue(row[i+1]) + ","
						}
					}
					d.Sql = strings.TrimSuffix(d.Sql, ",")
				} else {
					d.Sql = "DELETE FROM " + EscapeId(tableName)
				}

				d.Sql += " WHERE "

				// The last part of the UPDATE and DELETE statements is the same
				for _, p := range d.Pk {
					if p.Type == Null {
						d.Sql += EscapeId(p.Name) + " IS NULL"
					} else {
						d.Sql += EscapeId(p.Name) + "=" + EscapeValue(p)
					}
					d.Sql += " AND "
				}
				d.Sql = strings.TrimSuffix(d.Sql, " AND ") + ";"
			} else if d.ActionType == ActionAdd {
				// For inserted rows the merge strategy actually does matter. The PreservePkMerge strategy is simple:
				// We just include all columns, no matter whether primary key or not, in the INSERT statement as-is.
				// For tables which don't have a primary key the same applies even when using the NewPkMerge strategy.
				// Finally for tables with an incrementing primary key we must omit the primary key columns too to make
				// SQLite generate a new value for us.

				d.Sql = "INSERT INTO " + EscapeId(tableName) + "("

				if merge == PreservePkMerge || implicitPk {
					// Include all data we have in the INSERT statement but don't include the rowid column, the
					// implicit primary key

					// Add the explicit primary key columns first if any, then the other fields
					if !implicitPk {
						d.Sql += strings.Join(pkEscaped, ",") + ","
					}
					d.Sql += strings.Join(otherEscaped, ",") + ") VALUES ("

					// If there is an explicit primary key, add the values of that first
					if !implicitPk {
						for i := 0; i < len(pk); i++ {
							d.Sql += EscapeValue(row[i]) + ","
						}
					}

					// For the other columns start at the first data column after the diff type column and the first
					// modified flag column and skip to the next data columns.
					for i := len(pk) + 2; i < len(row); i += 2 {
						d.Sql += EscapeValue(row[i]) + ","
					}
				} else {
					// For the NewPkMerge strategy for tables with an explicit primary key the generated INSERT
					// statement depends on whether we can rely on SQLite to generate a new primary key value
					// or whether we must generate a new value on our own.

					if incrementingPk {
						// SQLite can generate a new key for us if we omit the primary key columns from the
						// INSERT statement.

						d.Sql += strings.Join(otherEscaped, ",") + ") VALUES ("

						// For the other columns start at the first data column after the diff type column and the first
						// modified flag column and skip to the next data columns.
						for i := len(pk) + 2; i < len(row); i += 2 {
							d.Sql += EscapeValue(row[i]) + ","
						}
					} else {
						// We need to generate a new key by ourselves by including the primary key columns in
						// the INSERT statement and producing a new value.

						// Add the (explicit) primary key columns first, then the other fields
						d.Sql += strings.Join(pkEscaped, ",") + ","
						d.Sql += strings.Join(otherEscaped, ",") + ") VALUES ("

						// Add the (explicit) primary key values first using a SELECT statement which generates a
						// new value for the first key column
						d.Sql += "(SELECT max(" + pkEscaped[0] + ")+1 FROM " + EscapeId(tableName) + "),"
						for i := 1; i < len(pk); i++ {
							d.Sql += EscapeValue(row[i]) + ","
						}

						// For the other columns start at the first data column after the diff type column and the first
						// modified flag column and skip to the next data columns.
						for i := len(pk) + 2; i < len(row); i += 2 {
							d.Sql += EscapeValue(row[i]) + ","
						}
					}
				}

				d.Sql = strings.TrimSuffix(d.Sql, ",") + ");"
			}
		}

		diff = append(diff, d)
	}

	return diff, nil
}

// hasIncrementingIntPk returns true if the table with name tableName has a primary key of integer type which increments
// automatically. Note that in SQLite this does not require the AUTOINCREMENT specifier. It merely requires a column of
// type INTEGER which is used as the primary key of the table. The only other constraint is that the table must not be a
// WITHOUT ROWID table
func hasIncrementingIntPk(sdb *sqlite.Conn, schemaName string, tableName string) (bool, error) {
	// Get column list
	columns, err := sdb.Columns(schemaName, tableName)
	if err != nil {
		return false, err
	}

	// Check if there is an INTEGER column used as primary key
	var numIntPks int
	var hasColumnRowid, hasColumn_Rowid_, hasColumnOid bool
	for _, c := range columns {
		if c.DataType == "INTEGER" && c.Pk > 0 {
			// If the column has also the AUTOINCREMENT specifier set, we don't need any extra checks and
			// can return early
			if c.Autoinc {
				return true, nil
			}

			numIntPks += 1
		}

		// While here check if there are any columns called rowid or similar in this table
		if c.Name == "rowid" {
			hasColumnRowid = true
		} else if c.Name == "_rowid_" {
			hasColumn_Rowid_ = true
		} else if c.Name == "oid" {
			hasColumnOid = true
		}
	}

	// Only exactly one integer primary key column works
	if numIntPks != 1 {
		return false, nil
	}

	// Check if this is a WITHOUT ROWID table. We do this by selecting the rowid column. If this produces an error
	// this probably means there is no rowid column
	var rowid string
	if !hasColumnRowid {
		rowid = "rowid"
	} else if !hasColumn_Rowid_ {
		rowid = "_rowid_"
	} else if !hasColumnOid {
		rowid = "oid"
	} else {
		return false, nil
	}
	err = sdb.OneValue("SELECT "+rowid+" FROM "+EscapeId(schemaName)+"."+EscapeId(tableName)+" LIMIT 1;", nil)

	// An error other than io.EOF (which just means there is no row in the table) means that there is no rowid column.
	// So this would be a WITHOUT ROWID table which doesn't increment its primary key automatically. Otherwise this is
	// a table with an incrementing primary key.
	if err != nil && err != io.EOF {
		return false, nil
	}
	return true, nil
}
