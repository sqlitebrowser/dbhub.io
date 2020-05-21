package common

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	sqlite "github.com/gwenn/gosqlite"
)

// SQLite Functions
type function string

const (
	// Core functions: https://sqlite.org/lang_corefunc.html
	//fnLoadExtension           function = "load_extension" // Loading extensions is definitely not allowed
	fnAbs                     function = "abs"
	fnChanges                 function = "changes"
	fnChar                    function = "char"
	fnCoalesce                function = "coalesce"
	fnGlob                    function = "glob"
	fnHex                     function = "hex"
	fnIfNull                  function = "ifnull"
	fnInstr                   function = "instr"
	fnLastInsertRowID         function = "last_insert_rowid"
	fnLength                  function = "length"
	fnLike                    function = "like"
	fnLikelihood              function = "likelihood"
	fnLikely                  function = "likely"
	fnLower                   function = "lower"
	fnLTrim                   function = "ltrim"
	fnMax                     function = "max"
	fnMin                     function = "min"
	fnNullIf                  function = "nullif"
	fnPrintF                  function = "printf"
	fnQuot                    function = "quote"
	fnRandom                  function = "random"
	fnRandomBlob              function = "randomblob"
	fnReplace                 function = "replace"
	fnRound                   function = "round"
	fnRTrim                   function = "rtrim"
	fnSoundEx                 function = "soundex"
	fnSQLiteCompileOptionGet  function = "sqlite_compileoption_get"
	fnSQLiteCompileOptionUsed function = "sqlite_compileoption_used"
	fnSQLiteOffset            function = "sqlite_offset"
	fnSQLiteSourceID          function = "sqlite_source_id"
	fnSQLiteVersion           function = "sqlite_version"
	fnSubstr                  function = "substr"
	fnTotalChanges            function = "total_changes"
	fnTrim                    function = "trim"
	fnTypeOf                  function = "typeof"
	fnUnicode                 function = "unicode"
	fnUnlikely                function = "unlikely"
	fnUpper                   function = "upper"
	fnZeroBlob                function = "zeroblob"

	// Date and Time functions: https://sqlite.org/lang_datefunc.html
	fnDate      function = "date"
	fnTime      function = "time"
	fnDateTime  function = "datetime"
	fnJulianDay function = "julianday"
	fnStrfTime  function = "strftime"

	// Aggregate functions: https://sqlite.org/lang_aggfunc.html
	fnAvg         function = "avg"
	fnCount       function = "count"
	fnGroupConcat function = "group_concat"
	fnSum         function = "sum"
	fnTotal       function = "total"

	// Window functions: https://sqlite.org/windowfunctions.html
	fnRowNumber   function = "row_number"
	fnRank        function = "rank"
	fnDenseRank   function = "dense_rank"
	fnPercentRank function = "percent_rank"
	fnCumeDist    function = "cume_dist"
	fnNTile       function = "ntile"
	fnLag         function = "lag"
	fnLead        function = "lead"
	fnFirstValue  function = "first_value"
	fnLastValue   function = "last_value"
	fnNthValue    function = "nth_value"

	// JSON1 functions: https://sqlite.org/json1.html
	fnJson            function = "json"
	fnJsonArray       function = "json_array"
	fnJsonArrayLength function = "json_array_length"
	fnJsonExtract     function = "json_extract"
	fnJsonInsert      function = "json_insert"
	fnJsonObject      function = "json_object"
	fnJsonPatch       function = "json_patch"
	fnJsonRemove      function = "json_remove"
	fnJsonReplace     function = "json_replace"
	fnJsonSet         function = "json_set"
	fnJsonType        function = "json_type"
	fnJsonValid       function = "json_valid"
	fnJsonQuote       function = "json_quote"
	fnJsonGroupArray  function = "json_group_array"
	fnJsonGroupObject function = "json_group_object"
	fnJsonEach        function = "json_each"
	fnJsonTree        function = "json_tree"
)

var SQLiteFunctions = []function{
	fnAbs,
	fnChanges,
	fnChar,
	fnCoalesce,
	fnGlob,
	fnHex,
	fnIfNull,
	fnInstr,
	fnLastInsertRowID,
	fnLength,
	fnLike,
	fnLikelihood,
	fnLikely,
	fnLower,
	fnLTrim,
	fnMax,
	fnMin,
	fnNullIf,
	fnPrintF,
	fnQuot,
	fnRandom,
	fnRandomBlob,
	fnReplace,
	fnRound,
	fnRTrim,
	fnSoundEx,
	fnSQLiteCompileOptionGet,
	fnSQLiteCompileOptionUsed,
	fnSQLiteOffset,
	fnSQLiteSourceID,
	fnSQLiteVersion,
	fnSubstr,
	fnTotalChanges,
	fnTrim,
	fnTypeOf,
	fnUnicode,
	fnUnlikely,
	fnUpper,
	fnZeroBlob,
	fnDate,
	fnTime,
	fnDateTime,
	fnJulianDay,
	fnStrfTime,
	fnAvg,
	fnCount,
	fnGroupConcat,
	fnSum,
	fnTotal,
	fnRowNumber,
	fnRank,
	fnDenseRank,
	fnPercentRank,
	fnCumeDist,
	fnNTile,
	fnLag,
	fnLead,
	fnFirstValue,
	fnLastValue,
	fnNthValue,
	fnJson,
	fnJsonArray,
	fnJsonArrayLength,
	fnJsonExtract,
	fnJsonInsert,
	fnJsonObject,
	fnJsonPatch,
	fnJsonRemove,
	fnJsonReplace,
	fnJsonSet,
	fnJsonType,
	fnJsonValid,
	fnJsonQuote,
	fnJsonGroupArray,
	fnJsonGroupObject,
	fnJsonEach,
	fnJsonTree,
}

// A SQLite authorizer callback which only allows SELECT queries and their needed sub-operations to run
func AuthorizerSelect(d interface{}, action sqlite.Action, tableName, funcName, dbName, triggerName string) sqlite.Auth {
	// We make sure the "action" code is either SELECT, READ (needed for reading data), or one of the in-built/allowed
	// functions
	switch action {
	case sqlite.Select:
		return sqlite.AuthOk
	case sqlite.Read:
		return sqlite.AuthOk
	case sqlite.Function:
		// Check if the function name is known (eg allowed), or unknown (eg disallowed)
		knownFunction := false
		for _, f := range SQLiteFunctions {
			if funcName == string(f) { // funcName holds the name of the function being executed
				knownFunction = true
			}
		}
		if knownFunction {
			// Only known functions are allowed
			return sqlite.AuthOk
		}
	}

	// All other action types, functions, etc are denied
	return sqlite.AuthDeny
}

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

// Retrieves a SQLite database from Minio, opens it, then returns the connection handle.
func OpenSQLiteDatabase(bucket, id string) (*sqlite.Conn, error) {
	// Retrieve database file from Minio, using cached version if it's already there
	newDB, err := RetrieveDatabaseFile(bucket, id)
	if err != nil {
		return nil, err
	}

	// Open database
	// NOTE - OpenFullMutex seems like the right thing for ensuring multiple connections to a database file don't
	// screw things up, but it wouldn't be a bad idea to keep it in mind if weirdness shows up
	sdb, err := sqlite.Open(newDB, sqlite.OpenReadWrite|sqlite.OpenFullMutex)
	if err != nil {
		log.Printf("Couldn't open database: %s", err)
		return nil, errors.New("Internal server error")
	}
	err = sdb.EnableExtendedResultCodes(true)
	if err != nil {
		log.Printf("Couldn't enable extended result codes! Error: %v\n", err.Error())
		return nil, err
	}
	return sdb, nil
}

// Similar to OpenSQLiteDatabase(), but opens the database Read Only and implements recommended defensive precautions
// for potentially malicious user provided SQL queries
func OpenSQLiteDatabaseDefensive(w http.ResponseWriter, r *http.Request, dbOwner, dbFolder, dbName, commitID, loggedInUser string) (sdb *sqlite.Conn, err error) {
	// Check if the user has access to the requested database
	var bucket, id string
	bucket, id, _, err = MinioLocation(dbOwner, dbFolder, dbName, commitID, loggedInUser)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Sanity check
	if id == "" {
		// The requested database wasn't found, or the user doesn't have permission to access it
		err = fmt.Errorf("Requested database not found")
		log.Printf("Requested database not found. Owner: '%s%s%s'", dbOwner, dbFolder, dbName)
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "%s", err.Error())
		return
	}

	// Retrieve database file from Minio, using locally cached version if it's already there
	newDB, err := RetrieveDatabaseFile(bucket, id)
	if err != nil {
		return nil, err
	}

	// Open the SQLite database super carefully: https://www.sqlite.org/security.html
	sdb, err = sqlite.Open(newDB, sqlite.OpenReadOnly)
	if err != nil {
		log.Printf("Couldn't open database: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return nil, err
	}
	if err = sdb.EnableExtendedResultCodes(true); err != nil {
		log.Printf("Couldn't enable extended result codes! Error: %v\n", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return nil, err
	}

	// Enable the defensive flag
	var enabled, ok bool
	if ok, err = sdb.EnableDefensive(true); !ok || err != nil {
		log.Printf("Couldn't enable the defensive flag: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return nil, err
	}

	// Verify the defensive flag was enabled
	if enabled, err = sdb.IsDefensiveEnabled(); !enabled || err != nil {
		log.Printf("The defensive flag wasn't enabled after all: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return nil, err
	}

	// Turn off the trusted schema flag
	if ok, err = sdb.EnableTrustedSchema(false); !ok || err != nil {
		log.Printf("Couldn't disable the trusted schema flag: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return nil, err
	}

	// Verify the trusted schema flag was turned off
	if enabled, err = sdb.IsTrustedSchema(); enabled || err != nil {
		log.Printf("The trusted schema flag wasn't disabled after all: %v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return nil, err
	}

	// Adjust limits for the SQLite connection
	var newLimits = []struct {
		name sqlite.Limit
		val  int32
	}{
		{
			name: sqlite.LimitLength,
			val:  1000000, // 1,000,000
		},
		{
			name: sqlite.LimitSQLLength,
			val:  100000, // 100,000
		},
		{
			name: sqlite.LimitColumn,
			val:  100,
		},
		{
			name: sqlite.LimitExprDepth,
			val:  10,
		},
		{
			name: sqlite.LimitCompoundSelect,
			val:  3,
		},
		{
			name: sqlite.LimitVdbeOp,
			val:  25000,
		},
		{
			name: sqlite.LimitFunctionArg,
			val:  8,
		},
		{
			name: sqlite.LimitAttached,
			val:  0,
		},
		{
			name: sqlite.LimitLikePatternLength,
			val:  50,
		},
		{
			name: sqlite.LimitVariableNumber,
			val:  10,
		},
		{
			name: sqlite.LimitTriggerLength,
			val:  10,
		},
	}
	for _, j := range newLimits {
		sdb.SetLimit(j.name, j.val)
		if sdb.Limit(j.name) != j.val {
			err = fmt.Errorf("Was not able to set SQLite limit '%v' to desired value", j.name)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "%s", err.Error())
			return nil, err
		}
	}

	// Set a SQLite authorizer which only allows SELECT statements to run
	err = sdb.SetAuthorizer(AuthorizerSelect, "SELECT authorizer")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return nil, err
	}

	// TODO: Set up a progress handler and timer (or something) to abort statements which run too long
	//       https://www.sqlite.org/c3ref/interrupt.html

	// TODO: Limit the maximum amount of memory SQLite will allocate (sqlite3_hard_heap_limit64())
	//       https://www.sqlite.org/c3ref/hard_heap_limit64.html
	//       This may need adding to gwenn/gosqlite.  It'd probably also be useful to measure the usage on DBHub.io
	//       too, to get an idea of a reasonable starting value. eg: https://www.sqlite.org/c3ref/memory_highwater.html

	// TODO: Disable creation and/or redefinition of user defined functions
	//       https://www.sqlite.org/c3ref/create_function.html

	// TODO: Disable creation of table-valued functions
	//       https://www.sqlite.org/vtab.html#tabfunc2

	// TODO: Should we add some of the commonly used extra functions?
	//       eg: https://github.com/sqlitebrowser/sqlitebrowser/blob/master/src/extensions/extension-functions.c

	// TODO: It could be interesting to add the Spatialite functions when we have the country (choropleth) chart
	//       operational

	return sdb, nil
}

// Reads up to maxRows number of rows from a given SQLite database table.  If maxRows < 0 (eg -1), then read all rows.
func ReadSQLiteDB(sdb *sqlite.Conn, dbTable, sortCol, sortDir string, maxRows, rowOffset int) (SQLiteRecordSet, error) {
	return ReadSQLiteDBCols(sdb, dbTable, sortCol, sortDir, false, false, maxRows, rowOffset)
}

// Reads up to maxRows # of rows from a SQLite database.  Only returns the requested columns.
func ReadSQLiteDBCols(sdb *sqlite.Conn, dbTable, sortCol, sortDir string, ignoreBinary, ignoreNull bool, maxRows, rowOffset int) (SQLiteRecordSet, error) {
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

// This is a specialised variation of the ReadSQLiteDB() function, just for our Redash JSON exporting code. It'll probably
// need to be merged with the above function at some point.
func ReadSQLiteDBRedash(sdb *sqlite.Conn, dbTable string) (dash RedashTableData, err error) {
	// Retrieve all of the data from the selected database table
	stmt, err := sdb.Prepare(`SELECT * FROM "` + dbTable + `"`)
	if err != nil {
		log.Printf("Error when preparing statement for database: %s\n", err)
		return RedashTableData{}, err
	}

	// Process each row
	fieldCount := -1
	err = stmt.Select(func(s *sqlite.Stmt) error {

		// Get the number of fields in the result
		if fieldCount == -1 {
			fieldCount = stmt.DataCount()
		}

		// Retrieve the (table level) type declaration for the result fields
		if dash.Columns == nil {
			for i := 0; i < fieldCount; i++ {
				var c RedashColumnMeta
				c.Name = stmt.ColumnName(i)
				c.FriendlyName = stmt.ColumnName(i)

				// Map common SQLite data types to Redash JSON acceptable equivalent
				t := strings.ToLower(stmt.ColumnDeclaredType(i))
				switch t {
				case "numeric":
					c.Type = "float"
				case "real":
					c.Type = "float"
				case "text":
					c.Type = "string"
				default:
					c.Type = t
				}

				dash.Columns = append(dash.Columns, c)
			}
		}

		// Retrieve the data for each row
		row := make(map[string]interface{})
		for i := 0; i < fieldCount; i++ {

			// Retrieve the name of the field
			fieldName := stmt.ColumnName(i)

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
					row[fieldName] = val
				}
			case sqlite.Float:
				var val float64
				val, isNull, err = s.ScanDouble(i)
				if err != nil {
					log.Printf("Something went wrong with ScanDouble(): %v\n", err)
					break
				}
				if !isNull {
					row[fieldName] = val
				}
			case sqlite.Text:
				var val string
				val, isNull = s.ScanText(i)
				if !isNull {
					row[fieldName] = val
				}
			case sqlite.Blob:
				var val []byte
				val, isNull = s.ScanBlob(i)
				if !isNull {
					// Base64 encode the value
					row[fieldName] = base64.StdEncoding.EncodeToString(val)
				}
			case sqlite.Null:
				isNull = true
			}
		}
		dash.Rows = append(dash.Rows, row)
		return nil
	})
	if err != nil {
		log.Printf("Error when reading data from database: %s\n", err)
		return RedashTableData{}, err
	}
	defer stmt.Finalize()

	// Return the results
	return dash, nil
}

// Runs a SQLite database query, for the visualisation tab.
func RunSQLiteVisQuery(sdb *sqlite.Conn, params VisParamsV1) ([]VisRowV1, error) {
	xTable := params.XAxisTable
	xAxis := params.XAXisColumn
	yTable := params.YAxisTable
	yAxis := params.YAXisColumn

	// Construct the SQLite visualisation query
	aggText := ""
	switch params.AggType {
	case 0:
		aggText = ""
	case 1:
		aggText = "avg"
	case 2:
		aggText = "count"
	case 3:
		aggText = "group_concat"
	case 4:
		aggText = "max"
	case 5:
		aggText = "min"
	case 6:
		aggText = "sum"
	case 7:
		aggText = "total"
	default:
		return []VisRowV1{}, errors.New("Unknown aggregate type")
	}

	joinText := ""
	switch params.JoinType {
	case 0:
		joinText = ""
	case 1:
		joinText = "INNER JOIN"
	case 2:
		joinText = "LEFT OUTER JOIN"
	case 3:
		joinText = "CROSS JOIN"
	default:
		return []VisRowV1{}, errors.New("Unknown join type")
	}

	// * Construct the SQL query using sqlite.Mprintf() for safety *
	var dbQuery string

	// Check if we're joining tables
	if xTable == yTable {
		// Simple query, no join needed
		if aggText != "" {
			dbQuery = sqlite.Mprintf(`SELECT "%s",`, xAxis)
			dbQuery += sqlite.Mprintf(` %s(`, aggText)
			dbQuery += sqlite.Mprintf(`"%s")`, yAxis)
			dbQuery += sqlite.Mprintf(` FROM "%s"`, xTable)
			dbQuery += sqlite.Mprintf(` GROUP BY "%s"`, xAxis)
		} else {
			dbQuery = sqlite.Mprintf(`SELECT "%s",`, xAxis)
			dbQuery += sqlite.Mprintf(` "%s"`, yAxis)
			dbQuery += sqlite.Mprintf(` FROM "%s"`, xTable)
		}
	} else {
		// We're joining tables
		dbQuery = sqlite.Mprintf(`SELECT "%s"`, xTable)
		dbQuery += sqlite.Mprintf(`."%s",`, xAxis)
		dbQuery += sqlite.Mprintf(` "%s"`, yTable)
		dbQuery += sqlite.Mprintf(`."%s"`, yAxis)
		dbQuery += sqlite.Mprintf(` FROM "%s"`, xTable)
		dbQuery += sqlite.Mprintf(` %s`, joinText)
		dbQuery += sqlite.Mprintf(` "%s"`, yTable)
		if params.JoinType == 1 || params.JoinType == 2 { // INNER JOIN and LEFT OUTER JOIN
			dbQuery += sqlite.Mprintf(` ON "%s"`, xTable)
			dbQuery += sqlite.Mprintf(`."%s"`, params.JoinXCol)
			dbQuery += sqlite.Mprintf(` = "%s"`, yTable)
			dbQuery += sqlite.Mprintf(`."%s"`, params.JoinYCol)
		}
	}
	var visRows []VisRowV1
	stmt, err := sdb.Prepare(dbQuery)
	if err != nil {
		log.Printf("Error when preparing statement for database: %s\n", err)
		return visRows, errors.New("Error when preparing the SQLite visualisation statement")
	}

	// Process each row
	err = stmt.Select(func(s *sqlite.Stmt) error {
		// Retrieve the data for each row
		var name string
		var val int
		if err = s.Scan(&name, &val); err != nil {
			_ = errors.New("Error when running the SQLite visualisation statement")
		}
		visRows = append(visRows, VisRowV1{Name: name, Value: val})
		return nil
	})
	if err != nil {
		log.Printf("Error when retrieving select data from database: %s\n", err)
		return visRows, err
	}
	defer stmt.Finalize()

	return visRows, nil
}

// Runs user provided SQL query for visualisation
func RunUserVisQuery(sdb *sqlite.Conn, dbQuery string) (visRows []VisRowV1, err error) {
	stmt, err := sdb.Prepare(dbQuery)
	if err != nil {
		log.Printf("Error when preparing statement for database: %s\n", err)
		return visRows, errors.New("Error when preparing the SQLite query")
	}
	// Process each row
	err = stmt.Select(func(s *sqlite.Stmt) error {
		// Retrieve the data for each row
		// TODO: This will very likely need something that checks the # of returned fields, type of values, etc.
		var name string
		var val int
		if err = s.Scan(&name, &val); err != nil {
			_ = errors.New("Error when running the SQLite visualisation statement")
		}
		visRows = append(visRows, VisRowV1{Name: name, Value: val})
		return nil
	})
	if err != nil {
		log.Printf("Error when retrieving select data from database: %s\n", err)
		return visRows, err
	}
	defer stmt.Finalize()

	return
}

// Performs basic sanity checks of an uploaded database.
func SanityCheck(fileName string) (tables []string, err error) {
	// Perform a read on the database, as a basic sanity check to ensure it's really a SQLite database
	sqliteDB, err := sqlite.Open(fileName, sqlite.OpenReadOnly)
	if err != nil {
		log.Printf("Couldn't open database when sanity checking upload: %s", err)
		err = fmt.Errorf("Internal error when uploading database")
		return
	}
	defer sqliteDB.Close()

	// Run an integrity check on the uploaded database
	var ok bool
	var results []string
	err = sqliteDB.Select("PRAGMA integrity_check", func(s *sqlite.Stmt) error {
		// Retrieve a row from the integrity check result
		var a string
		if err = s.Scan(&a); err != nil {
			// Error where reading the row, so ensure the integrity check returns a failure result
			ok = false
			return err
		}

		// If the returned row was the text string "ok", then we mark the integrity check as passed.  Any other
		// string or set of strings means the check failed
		switch a {
		case "ok":
			ok = true
		default:
			ok = false
			results = append(results, a)
		}
		return nil
	})

	// Check for a failure
	if !ok || err != nil {
		log.Printf("Error when running an integrity check on the database: %s\n", err)
		if len(results) > 0 {
			for _, b := range results {
				log.Printf("  * %v\n", b)
			}
		}
		return
	}

	// Ensure the uploaded database has tables.  An empty database serves no useful purpose.
	tables, err = sqliteDB.Tables("")
	if err != nil {
		log.Printf("Error retrieving table names when sanity checking upload: %s", err)
		err = fmt.Errorf("Error when sanity checking file.  Possibly encrypted or not a database?")
		return
	}
	if len(tables) == 0 {
		// No table names were returned, so abort
		log.Print("The attempted upload failed, as it doesn't seem to have any tables.")
		err = fmt.Errorf("Database has no tables?")
		return
	}
	return
}

// Returns the list of tables and view in the SQLite database.
func Tables(sdb *sqlite.Conn, dbName string) ([]string, error) {
	// TODO: It might be useful to cache this info in PG or memcached
	// Retrieve the list of tables in the database
	tables, err := sdb.Tables("")
	if err != nil {
		// An error occurred, so get the extended error code
		if cerr, ok := err.(sqlite.ConnError); ok {
			// Check if the error was due to the table being locked
			extCode := cerr.ExtendedCode()
			if extCode == 5 { // Magic number which (in this case) means "database is locked"
				// Wait 3 seconds then try again
				time.Sleep(3 * time.Second)
				tables, err = sdb.Tables("")
				if err != nil {
					log.Printf("Error retrieving table names: %s", err)
					return nil, err
				}
			} else {
				log.Printf("Error retrieving table names: %s", err)
				return nil, err
			}
		} else {
			log.Printf("Error retrieving table names: %s", err)
			return nil, err
		}
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
