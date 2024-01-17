package common

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sqlitebrowser/dbhub.io/common/config"
	"github.com/sqlitebrowser/dbhub.io/common/database"

	sqlite "github.com/gwenn/gosqlite"
)

// SqliteDebug displays some SQLite related debugging information (on the backend), when set to a non-0 value
const SqliteDebug = 0

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
	fnIif                     function = "iif"
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
	fnSign                    function = "sign"
	fnSoundEx                 function = "soundex"
	fnSQLiteCompileOptionGet  function = "sqlite_compileoption_get"
	fnSQLiteCompileOptionUsed function = "sqlite_compileoption_used"
	fnSQLiteOffset            function = "sqlite_offset"
	fnSQLiteSourceID          function = "sqlite_source_id"
	fnSQLiteVersion           function = "sqlite_version"
	fnSubstr                  function = "substr"
	fnSubstring               function = "substring"
	fnTotalChanges            function = "total_changes"
	fnTrim                    function = "trim"
	fnTypeOf                  function = "typeof"
	fnUnhex                   function = "unhex"
	fnUnicode                 function = "unicode"
	fnUnlikely                function = "unlikely"
	fnUpper                   function = "upper"
	fnZeroBlob                function = "zeroblob"

	// Date and Time functions: https://sqlite.org/lang_datefunc.html
	fnDate      function = "date"
	fnTime      function = "time"
	fnDateTime  function = "datetime"
	fnJulianDay function = "julianday"
	fnUnixEpoch function = "unixepoch"
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

	// FTS3 & FTS4 functions: https://www.sqlite.org/fts3.html
	fnFts3         function = "fts3"
	fnFts3Tokenize function = "fts3tokenize"
	fnFts4         function = "fts4"
	fnFts4Aux      function = "fts4aux"
	fnMatchinfo    function = "matchinfo"
	fnOffsets      function = "offsets"

	// FTS5 functions: https://www.sqlite.org/fts5.html
	fnBm25      function = "bm25"
	fnFts5      function = "fts5"
	fnFts5Vocab function = "fts5vocab"
	fnHighlight function = "highlight"
	fnSnippet   function = "snippet"

	// R*Tree functions: https://www.sqlite.org/rtree.html
	fnRTree      function = "rtree"
	fnRTreei32   function = "rtree_i32"
	fnRTreeCheck function = "rtreecheck"

	// Geopoly functions: https://www.sqlite.org/geopoly.html
	fnGeopoly              function = "geopoly"
	fnGeopolyArea          function = "geopoly_area"
	fnGeopolyBBox          function = "geopoly_bbox"
	fnGeopolyBlob          function = "geopoly_blob"
	fnGeopolyCCW           function = "geopoly_ccw"
	fnGeopolyContainsPoint function = "geopoly_contains_point"
	fnGeopolyGroupBBox     function = "geopoly_group_bbox"
	fnGeopolyJson          function = "geopoly_json"
	fnGeopolyOverlap       function = "geopoly_overlap"
	fnGeopolyRegular       function = "geopoly_regular"
	fnGeopolySvg           function = "geopoly_svg"
	fnGeopolyWithin        function = "geopoly_within"
	fnGeopolyXForm         function = "geopoly_xform"

	// Math functions: https://www.sqlite.org/lang_mathfunc.html
	fnACos     function = "acos"
	fnACosH    function = "acosh"
	fnASin     function = "asin"
	fnASinH    function = "asinh"
	fnATan     function = "atan"
	fnATan2    function = "atan2"
	fnATanh    function = "atanh"
	fnACeil    function = "ceil"
	fnACeiling function = "ceiling"
	fnCos      function = "cos"
	fnCosH     function = "cosh"
	fnDegrees  function = "degrees"
	fnExp      function = "exp"
	fnFloor    function = "floor"
	fnLn       function = "ln"
	fnLog      function = "log"
	fnLog10    function = "log10"
	fnLog2     function = "log2"
	fnMod      function = "mod"
	fnPi       function = "pi"
	fnPow      function = "pow"
	fnPower    function = "power"
	fnRadians  function = "radians"
	fnSin      function = "sin"
	fnSinH     function = "sinh"
	fnSqrt     function = "sqrt"
	fnTan      function = "tan"
	fnTanH     function = "tanh"
	fnTrunc    function = "trunc"

	// Other functions we should allow
	fnVersion function = "sqlite_version"
)

// SQLiteFunctions lists the function we allow SQL queries to run
var SQLiteFunctions = []function{
	fnAbs,
	fnChanges,
	fnChar,
	fnCoalesce,
	fnGlob,
	fnHex,
	fnIfNull,
	fnIif,
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
	fnSign,
	fnSoundEx,
	fnSQLiteCompileOptionGet,
	fnSQLiteCompileOptionUsed,
	fnSQLiteOffset,
	fnSQLiteSourceID,
	fnSQLiteVersion,
	fnSubstr,
	fnSubstring,
	fnTotalChanges,
	fnTrim,
	fnTypeOf,
	fnUnhex,
	fnUnicode,
	fnUnlikely,
	fnUpper,
	fnZeroBlob,
	fnDate,
	fnTime,
	fnDateTime,
	fnJulianDay,
	fnUnixEpoch,
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
	fnFts3,
	fnFts3Tokenize,
	fnFts4,
	fnFts4Aux,
	fnMatchinfo,
	fnOffsets,
	fnBm25,
	fnFts5,
	fnFts5Vocab,
	fnHighlight,
	fnSnippet,
	fnRTree,
	fnRTreei32,
	fnRTreeCheck,
	fnGeopoly,
	fnGeopolyArea,
	fnGeopolyBBox,
	fnGeopolyBlob,
	fnGeopolyCCW,
	fnGeopolyContainsPoint,
	fnGeopolyGroupBBox,
	fnGeopolyJson,
	fnGeopolyOverlap,
	fnGeopolyRegular,
	fnGeopolySvg,
	fnGeopolyWithin,
	fnGeopolyXForm,
	fnACos,
	fnACosH,
	fnASin,
	fnASinH,
	fnATan,
	fnATan2,
	fnATanh,
	fnACeil,
	fnACeiling,
	fnCos,
	fnCosH,
	fnDegrees,
	fnExp,
	fnFloor,
	fnLn,
	fnLog,
	fnLog10,
	fnLog2,
	fnMod,
	fnPi,
	fnPow,
	fnPower,
	fnRadians,
	fnSin,
	fnSinH,
	fnSqrt,
	fnTan,
	fnTanH,
	fnTrunc,
	fnVersion,
}

func init() {
	// Enable the collection of memory allocation statistics
	if err := sqlite.ConfigMemStatus(true); err != nil {
		log.Fatalf("Cannot enable memory allocation statistics: '%s'", err)
	}
}

// AuthorizerLive is a SQLite authorizer callback intended to allow almost anything.  Except for loading extensions,
// and running pragmas.
func AuthorizerLive(d interface{}, action sqlite.Action, tableName, funcName, dbName, triggerName string) sqlite.Auth {
	if SqliteDebug > 0 {
		log.Printf("AuthorizerLive - action: '%s', table: '%s', function: '%s'", action, tableName, funcName)
	}

	switch action {
	case sqlite.Pragma:
		// The "index_info"  and "table_info" Pragmas are allowed, as they're used by SQLite internally for things we need
		if tableName == "index_info" || tableName == "table_info" {
			return sqlite.AuthOk
		}

		// The "data_version" Pragma is needed when creating FTS5 virtual tables
		if tableName == "data_version" {
			return sqlite.AuthOk
		}

		// The "page_size" Pragma is needed when creating R*Tree virtual tables.  Ironically, SQLite (3.41.2) ignores
		// any "Deny" of the "page_size" pragma when creating FTS tables.  No idea why this difference in behaviour
		// exists, but it doesn't seem to hurt anything.
		if tableName == "page_size" {
			return sqlite.AuthOk
		}

		if SqliteDebug > 0 {
			log.Printf("Denying pragma: '%s'", tableName)
		}
		return sqlite.AuthDeny
	case sqlite.Function:
		if funcName == "load_extension" {
			// Extension loading is disabled (at least for now)
			return sqlite.AuthDeny
		}
	}

	// All other action types, functions, etc are allowed
	return sqlite.AuthOk
}

// AuthorizerSelect is a SQLite authorizer callback which only allows SELECT queries and their needed
// sub-operations to run.
func AuthorizerSelect(d interface{}, action sqlite.Action, tableName, funcName, dbName, triggerName string) sqlite.Auth {
	// We make sure the "action" code is either SELECT, READ (needed for reading data), or one of the in-built/allowed
	// functions
	switch action {
	case sqlite.Pragma:
		// (Only) the "table_info" Pragma is allowed.  It's used by SQLite internally for querying table structure
		if tableName == "table_info" {
			return sqlite.AuthOk
		}
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
	case sqlite.Update:
		if tableName == "sqlite_master" {
			return sqlite.AuthOk
		}
	}

	// All other action types, functions, etc are denied
	return sqlite.AuthDeny
}

// GetSQLiteRowCount returns the number of rows in a SQLite table.
func GetSQLiteRowCount(sdb *sqlite.Conn, dbTable string) (rowCount int, err error) {
	dbQuery := `SELECT count(*) FROM "` + dbTable + `"`
	err = sdb.OneValue(dbQuery, &rowCount)
	if err != nil {
		log.Printf("Error occurred when counting total rows for table '%s'.  Error: %s", SanitiseLogString(dbTable), err)
		return 0, errors.New("Database query failure")
	}
	return
}

// OpenSQLiteDatabase retrieves a SQLite database from Minio, opens it, then returns the connection handle.
func OpenSQLiteDatabase(bucket, id string) (sdb *sqlite.Conn, err error) {
	// Retrieve database file from Minio, using cached version if it's already there
	var newDB string
	newDB, err = RetrieveDatabaseFile(bucket, id)
	if err != nil {
		return
	}

	// Open database
	// NOTE - OpenFullMutex seems like the right thing for ensuring multiple connections to a database file don't
	// screw things up, but it wouldn't be a bad idea to keep it in mind if weirdness shows up
	sdb, err = sqlite.Open(newDB, sqlite.OpenReadWrite|sqlite.OpenFullMutex)
	if err != nil {
		log.Printf("Couldn't open database: %s", err)
		return nil, errors.New("Internal server error")
	}
	err = sdb.EnableExtendedResultCodes(true)
	if err != nil {
		log.Printf("Couldn't enable extended result codes! Error: %v", err.Error())
		return
	}
	return
}

// OpenSQLiteDatabaseDefensive is similar to OpenSQLiteDatabase(), but opens the database Read Only and implements
// the recommended defensive precautions for potentially malicious user provided SQL
// queries: https://www.sqlite.org/security.html
func OpenSQLiteDatabaseDefensive(w http.ResponseWriter, r *http.Request, dbOwner, dbName, commitID, loggedInUser string) (sdb *sqlite.Conn, err error) {
	// Check if the user has access to the requested database
	var bucket, id string
	bucket, id, _, err = MinioLocation(dbOwner, dbName, commitID, loggedInUser)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Sanity check
	if id == "" {
		// The requested database wasn't found, or the user doesn't have permission to access it
		err = fmt.Errorf("Requested database not found")
		log.Printf("Requested database not found. Owner: '%s/%s'", SanitiseLogString(dbOwner), SanitiseLogString(dbName))
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "%s", err.Error())
		return
	}

	// Retrieve database file from Minio, using locally cached version if it's already there
	newDB, err := RetrieveDatabaseFile(bucket, id)
	if err != nil {
		return nil, err
	}

	// Open the SQLite database in read only mode
	sdb, err = sqlite.Open(newDB, sqlite.OpenReadOnly)
	if err != nil {
		log.Printf("Couldn't open database: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return nil, err
	}
	if err = sdb.EnableExtendedResultCodes(true); err != nil {
		log.Printf("Couldn't enable extended result codes! Error: %v", err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return nil, err
	}

	// Enable the defensive flag
	var enabled bool
	if enabled, err = sdb.EnableDefensive(true); !enabled || err != nil {
		log.Printf("Couldn't enable the defensive flag: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return nil, err
	}

	// Verify the defensive flag was enabled
	if enabled, err = sdb.IsDefensiveEnabled(); !enabled || err != nil {
		log.Printf("The defensive flag wasn't enabled after all: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return nil, err
	}

	// Turn off the trusted schema flag
	if enabled, err = sdb.EnableTrustedSchema(false); enabled || err != nil {
		log.Printf("Couldn't disable the trusted schema flag: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s", err.Error())
		return nil, err
	}

	// Verify the trusted schema flag was turned off
	if enabled, err = sdb.IsTrustedSchema(); enabled || err != nil {
		log.Printf("The trusted schema flag wasn't disabled after all: %v", err)
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
			val:  400,
		},
		{
			name: sqlite.LimitExprDepth,
			val:  20,
		},
		{
			name: sqlite.LimitCompoundSelect,
			val:  30,
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
			val:  15,
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

	// Turn on cell size checking
	err = sdb.Exec("PRAGMA cell_size_check=ON")
	if err != nil {
		log.Printf("Error when running 'PRAGMA cell_size_check=on' on the LIVE database: %s", err)
		return
	}

	// Turn off mmap
	var ok bool
	var results []string
	err = sdb.Select("PRAGMA mmap_size=0", func(s *sqlite.Stmt) error {
		// Retrieve a row from the pragma result
		var a string
		if err = s.Scan(&a); err != nil {
			// Error when reading the row, so return failure
			ok = false
			return err
		}

		// If the returned row was the value 0, then we regard the change as successfully applied.  Any other return
		// value is a failure
		switch a {
		case "0":
			ok = true
		default:
			ok = false
			results = append(results, a)
		}
		return nil
	})

	// Check for a failure
	if !ok || err != nil {
		log.Printf("Error when disabling mmap on the LIVE database: %s", err)
		return
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
	//         * Not sure if it's really needed though, as we've already reduced the resources SQLite can allocate

	// TODO: Potentially limit the maximum amount of memory SQLite will allocate (sqlite3_hard_heap_limit64())
	//       https://www.sqlite.org/c3ref/hard_heap_limit64.html
	//         * We're now measuring the amount of memory each user supplied SQLite query uses, so we may
	//           have the needed info for this

	// TODO: Should we add some of the commonly used extra functions?
	//       eg: https://github.com/sqlitebrowser/sqlitebrowser/blob/master/src/extensions/extension-functions.c

	// TODO: It could be interesting to add the Spatialite functions when we have the country (choropleth) chart
	//       operational

	return sdb, nil
}

// OpenSQLiteDatabaseLive is similar to OpenSQLiteDatabase(), but opens the a live SQLite database and implements
// the recommended defensive precautions for potentially malicious user provided SQL
// queries: https://www.sqlite.org/security.html
// TODO: De-duplicate/refactor the common code in this function and OpenSQLiteDatabaseDefensive() above, as they're
// TODO  mostly the same
func OpenSQLiteDatabaseLive(baseDir, dbOwner, dbName string) (sdb *sqlite.Conn, err error) {
	dbPath := filepath.Join(baseDir, dbOwner, dbName, "live.sqlite")
	if _, err = os.Stat(dbPath); err != nil {
		return
	}

	// Open database
	// NOTE - OpenFullMutex seems like the right thing for ensuring multiple connections to a database file don't
	// screw things up, but it wouldn't be a bad idea to keep it in mind if weirdness shows up
	sdb, err = sqlite.Open(dbPath, sqlite.OpenReadWrite|sqlite.OpenFullMutex)
	if err != nil {
		log.Printf("Couldn't open LIVE database: %s", err)
		return
	}
	if err = sdb.EnableExtendedResultCodes(true); err != nil {
		log.Printf("Couldn't enable extended result codes for LIVE database query! Error: %v", err.Error())
		return
	}

	// Enable the defensive flag
	var enabled bool
	if enabled, err = sdb.EnableDefensive(true); !enabled || err != nil {
		log.Printf("Couldn't enable the defensive flag for LIVE database query: %v", err)
		return
	}

	// Verify the defensive flag was enabled
	if enabled, err = sdb.IsDefensiveEnabled(); !enabled || err != nil {
		log.Printf("The defensive flag wasn't enabled for LIVE database query after all: %v", err)
		return
	}

	// Turn off the trusted schema flag
	if enabled, err = sdb.EnableTrustedSchema(false); enabled || err != nil {
		log.Printf("Couldn't disable the trusted schema flag for LIVE database query: %v", err)
		return
	}

	// Verify the trusted schema flag was turned off
	if enabled, err = sdb.IsTrustedSchema(); enabled || err != nil {
		log.Printf("The trusted schema flag wasn't disabled for LIVE database query after all: %v", err)
		return
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
			val:  400,
		},
		{
			name: sqlite.LimitExprDepth,
			val:  20,
		},
		{
			name: sqlite.LimitCompoundSelect,
			val:  30,
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
			val:  15,
		},
		{
			name: sqlite.LimitTriggerLength,
			val:  10,
		},
	}
	for _, j := range newLimits {
		sdb.SetLimit(j.name, j.val)
		if sdb.Limit(j.name) != j.val {
			err = fmt.Errorf("Was not able to set SQLite limit '%v' to desired value for LIVE database query", j.name)
			return
		}
	}

	// Turn on cell size checking
	err = sdb.Exec("PRAGMA cell_size_check=ON")
	if err != nil {
		log.Printf("Error when running 'PRAGMA cell_size_check=on' on the LIVE database: %s", err)
		return
	}

	// Turn off mmap
	var ok bool
	var results []string
	err = sdb.Select("PRAGMA mmap_size=0", func(s *sqlite.Stmt) error {
		// Retrieve a row from the pragma result
		var a string
		if err = s.Scan(&a); err != nil {
			// Error when reading the row, so return failure
			ok = false
			return err
		}

		// If the returned row was the value 0, then we regard the change as successfully applied.  Any other return
		// value is a failure
		switch a {
		case "0":
			ok = true
		default:
			ok = false
			results = append(results, a)
		}
		return nil
	})

	// Check for a failure
	if !ok || err != nil {
		log.Printf("Error when disabling mmap on the LIVE database: %s", err)
		return
	}

	// Set a SQLite authorizer which only disallows pragma statements and the "load_extension" function
	err = sdb.SetAuthorizer(AuthorizerLive, "SELECT authorizer")
	if err != nil {
		return
	}
	return sdb, nil
}

// ReadSQLiteDB reads up to maxRows number of rows from a given SQLite database table.  If maxRows < 0 (eg -1), then
// read all rows.
func ReadSQLiteDB(sdb *sqlite.Conn, dbTable, sortCol, sortDir string, maxRows, rowOffset int) (SQLiteRecordSet, error) {
	return ReadSQLiteDBCols(sdb, dbTable, sortCol, sortDir, false, false, maxRows, rowOffset)
}

// ReadSQLiteDBCols reads up to maxRows # of rows from a SQLite database.  Only returns the requested columns.
func ReadSQLiteDBCols(sdb *sqlite.Conn, dbTable, sortCol, sortDir string, ignoreBinary, ignoreNull bool, maxRows, rowOffset int) (SQLiteRecordSet, error) {
	// Ugh, have to use string smashing for this, even though the SQL spec doesn't seem to say table names
	// shouldn't be parametrised.  Limitation from SQLite's implementation? :(
	var dataRows SQLiteRecordSet
	var err error

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
				log.Printf("Error occurred when creating index: %s", err.Error())
				return SQLiteRecordSet{}, err
			}
			sdb.Commit()
		}
	}

	// Get information on the primary key of the table
	pk, _, _, err := GetPrimaryKeyAndOtherColumns(sdb, "main", dbTable)
	if err != nil {
		log.Printf("error retrieving primary key columns: %s", err.Error())
		return SQLiteRecordSet{}, err
	}

	// If there is no primary key the rowid column serves as an implicit primary key. In this case
	// also get the rowid column
	dbQuery := "SELECT "
	if len(pk) == 1 && pk[0] == "rowid" {
		dbQuery += "rowid,"
	}

	// Construct the main SQL query
	dbQuery += sqlite.Mprintf(`* FROM "%w"`, dbTable)

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

	// Execute the query and retrieve the data
	_, _, dataRows, err = SQLiteRunQuery(sdb, QuerySourceAPI, dbQuery, ignoreBinary, ignoreNull)
	if err != nil {
		return dataRows, err
	}

	// Add count of total rows to returned data
	tmpCount, err := GetSQLiteRowCount(sdb, dbTable)
	if err != nil {
		return dataRows, err
	}
	dataRows.RowCount = tmpCount

	// Fill out other data fields
	dataRows.PrimaryKeyColumns = pk
	dataRows.Tablename = dbTable
	dataRows.SortCol = sortCol
	dataRows.SortDir = sortDir
	dataRows.Offset = rowOffset
	return dataRows, nil
}

// ReadSQLiteDBCSV is a specialised variation of the ReadSQLiteDB() function, just for our CSV exporting code.  It may
// be merged with that in future.
func ReadSQLiteDBCSV(sdb *sqlite.Conn, dbTable string) ([][]string, error) {
	// Retrieve all the data from the selected database table
	stmt, err := sdb.Prepare(`SELECT * FROM "` + dbTable + `"`)
	if err != nil {
		log.Printf("Error when preparing statement for database: %s", err)
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
					log.Printf("Something went wrong with ScanInt(): %v", err)
					break
				}
				if !isNull {
					row = append(row, fmt.Sprintf("%d", val))
				}
			case sqlite.Float:
				var val float64
				val, isNull, err = s.ScanDouble(i)
				if err != nil {
					log.Printf("Something went wrong with ScanDouble(): %v", err)
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
		log.Printf("Error when reading data from database: %s", err)
		return nil, err
	}
	defer stmt.Finalize()

	// Return the results
	return resultSet, nil
}

// SQLiteBackupLive is used by our job queue backend nodes to refresh a live SQLite database back into Minio
func SQLiteBackupLive(baseDir, dbOwner, dbName string) (err error) {
	dbPath := filepath.Join(baseDir, dbOwner, dbName, "live.sqlite")
	if _, err = os.Stat(dbPath); err != nil {
		return
	}

	// Open the database on the local node
	// NOTE - OpenFullMutex seems like the right thing for ensuring multiple connections to a database file don't
	// screw things up, but it wouldn't be a bad idea to keep it in mind if weirdness shows up
	var sdb *sqlite.Conn
	sdb, err = sqlite.Open(dbPath, sqlite.OpenReadWrite|sqlite.OpenFullMutex)
	if err != nil {
		log.Printf("Couldn't open LIVE database: %s", err)
		return
	}
	if err = sdb.EnableExtendedResultCodes(true); err != nil {
		log.Printf("Couldn't enable extended result codes for LIVE database query! Error: %v", err.Error())
		return
	}
	defer sdb.Close()

	// Generate unique temporary file name
	var f *os.File
	f, err = os.CreateTemp("", "sqliteBackup*.sqlite")
	if err != nil {
		return
	}
	tmpName := f.Name()
	err = f.Close()
	if err != nil {
		return
	}

	// Generate the backup file
	err = sdb.Exec(fmt.Sprintf("VACUUM INTO '%s'", tmpName))
	if err != nil {
		return
	}

	// Ensure the backup file was generated, and isn't 0 bytes (just in case)
	var fileInfo os.FileInfo
	fileInfo, err = os.Stat(tmpName)
	if err != nil {
		return
	}
	if fileInfo.Size() == 0 {
		err = errors.New("Generating backup live SQLite database failed.  File size is 0")
		return
	}

	// Sanity check the backup file
	var sdb2 *sqlite.Conn
	sdb2, err = sqlite.Open(tmpName, sqlite.OpenReadOnly)
	if err != nil {
		log.Printf("Couldn't open live database backup to sanity check it: %s", err)
		return
	}
	if err = sdb2.EnableExtendedResultCodes(true); err != nil {
		log.Printf("Couldn't enable extended result codes for live database backup integrity check! Error: %v", err.Error())
		sdb2.Close()
		return
	}
	// Pretty sure the '1' parameter below isn't needed.  The SQLite docs mention "O(N)" (etc.) which is just Big O
	// notation, rather than trying to communicate a need for a number in the parameters
	err = sdb2.IntegrityCheck("main", 1, true)
	if err != nil {
		sdb2.Close()
		return
	}
	sdb2.Close()

	// Copy the local backup file into Minio
	var z *os.File
	z, err = os.Open(tmpName)
	_, err = LiveStoreDatabaseMinio(z, dbOwner, dbName, fileInfo.Size())
	if err != nil {
		z.Close()
		return
	}
	z.Close()

	// Delete the backup file from local disk
	os.Remove(tmpName)
	return
}

// SQLiteExecuteQueryLive is used by our job queue backend infrastructure to execute a user provided SQLite statement
func SQLiteExecuteQueryLive(baseDir, dbOwner, dbName, loggedInUser, query string) (rowsChanged int, err error) {
	// Open the Live database on the local node
	var sdb *sqlite.Conn
	sdb, err = OpenSQLiteDatabaseLive(baseDir, dbOwner, dbName)
	if err != nil {
		return
	}
	defer sdb.Close()

	// TODO: Probably add in the before and after logging info at some point (as per query function),
	//       so we can analyse query execution times, memory use, etc

	// Execute the statement
	rowsChanged, err = sdb.ExecDml(query)
	if err != nil {
		if !strings.HasPrefix(err.Error(), "don't use exec with") {
			log.Printf("Error when executing query by '%s' for LIVE database (%s/%s): '%s'",
				SanitiseLogString(loggedInUser), SanitiseLogString(dbOwner), SanitiseLogString(dbName),
				SanitiseLogString(err.Error()))
		}
		return
	}
	return
}

// SQLiteGetColumnsLive is used by our job queue backend nodes to retrieve the list of columns from a SQLite database
func SQLiteGetColumnsLive(baseDir, dbOwner, dbName, table string) (columns []sqlite.Column, pk []string, err error, errCode JobQueueErrorCode) {
	// Open the database on the local node
	var sdb *sqlite.Conn
	sdb, err = OpenSQLiteDatabaseLive(baseDir, dbOwner, dbName)
	if err != nil {
		return
	}
	defer sdb.Close()

	// Verify the requested table or view we're about to query does exist
	var tablesViews []string
	tablesViews, err = TablesAndViews(sdb, dbName)
	if err != nil {
		return
	}
	tableOrViewFound := false
	for _, t := range tablesViews {
		if t == table {
			tableOrViewFound = true
		}
	}
	if !tableOrViewFound {
		err = errors.New("Provided table or view name doesn't exist in this database")
		errCode = JobQueueRequestedTableNotPresent
		return
	}

	// Retrieve the list of columns for the table
	columns, err = sdb.Columns("", table)

	// Retrieve a list of primary key columns
	pk, _, _, err = GetPrimaryKeyAndOtherColumns(sdb, "main", table)
	if err != nil {
		return
	}

	return
}

// SQLiteGetIndexesLive is used by our job queue backend nodes to retrieve the list of indexes from a SQLite database
func SQLiteGetIndexesLive(baseDir, dbOwner, dbName string) (indexes []APIJSONIndex, err error) {
	// Open the database on the local node
	var sdb *sqlite.Conn
	sdb, err = OpenSQLiteDatabaseLive(baseDir, dbOwner, dbName)
	if err != nil {
		return
	}
	defer sdb.Close()

	// Retrieve the list of indexes
	var idx map[string]string
	idx, err = sdb.Indexes("")
	if err != nil {
		return
	}

	// Retrieve the details for each index
	for nam, tab := range idx {
		oneIndex := APIJSONIndex{
			Name:    nam,
			Table:   tab,
			Columns: []APIJSONIndexColumn{},
		}
		var cols []sqlite.Column
		cols, err = sdb.IndexColumns("", nam)
		if err != nil {
			return
		}
		for _, k := range cols {
			oneIndex.Columns = append(oneIndex.Columns, APIJSONIndexColumn{
				CID:  k.Cid,
				Name: k.Name,
			})
		}
		indexes = append(indexes, oneIndex)
	}
	return
}

// SQLiteGetTablesLive is used by our job queue backend nodes to retrieve the list of tables in a SQLite database
func SQLiteGetTablesLive(baseDir, dbOwner, dbName string) (tables []string, err error) {
	// Open the database on the local node
	var sdb *sqlite.Conn
	sdb, err = OpenSQLiteDatabaseLive(baseDir, dbOwner, dbName)
	if err != nil {
		return
	}
	defer sdb.Close()

	// Retrieve the list of tables
	tables, err = Tables(sdb)
	if err != nil {
		return
	}
	return
}

// SQLiteGetViewsLive is used by our job queue backend nodes to retrieve the list of views in a SQLite database
func SQLiteGetViewsLive(baseDir, dbOwner, dbName string) (views []string, err error) {
	// Open the database on the local node
	var sdb *sqlite.Conn
	sdb, err = OpenSQLiteDatabaseLive(baseDir, dbOwner, dbName)
	if err != nil {
		return
	}
	defer sdb.Close()

	// Retrieve the list of views
	views, err = Views(sdb)
	if err != nil {
		return
	}
	return
}

// SQLiteSanityCheck performs basic sanity checks of an uploaded database.
func SQLiteSanityCheck(fileName string) (tables []string, err error) {
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
			// Error when reading the row, so ensure the integrity check returns a failure result
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
		if err != nil {
			log.Printf("Error when running a SQLite integrity check on the file '%s': %s", fileName, err)
		} else {
			err = errors.New("SQLite integrity check failed on file")
		}
		if len(results) > 0 {
			log.Printf("SQLite integrity check failed on file: '%s'.  Results below:", fileName)
			for _, b := range results {
				log.Printf("  * %v", b)
			}
			log.Printf("End of SQLite integrity check results for '%s'", fileName)
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

// SQLiteReadDatabasePage opens a SQLite database (locally) and returns a "page" of rows from it, for display in the
// database view page.  Note that the dbSize return value is only set for live databases.
func SQLiteReadDatabasePage(bucket, id, loggedInUser, dbOwner, dbName, dbTable, sortCol, sortDir, commitID string, rowOffset, maxRows int, isLive bool) (tables []string, defaultTable string, rowData SQLiteRecordSet, dbSize int64, err error) {
	// Get a handle from Minio for the database object
	var sdb *sqlite.Conn
	if isLive {
		// Open live database file
		sdb, err = OpenSQLiteDatabaseLive(config.Conf.Live.StorageDir, dbOwner, dbName)
		if err != nil {
			return
		}

		// We also return the file size for live database files
		var z os.FileInfo
		z, err = os.Stat(filepath.Join(config.Conf.Live.StorageDir, dbOwner, dbName, "live.sqlite"))
		if err != nil {
			return
		}
		dbSize = z.Size()
	} else {
		// Open standard database
		sdb, err = OpenSQLiteDatabase(bucket, id)
		if err != nil {
			return
		}
	}
	defer sdb.Close()

	// Retrieve the list of tables and views in the database
	tables, err = TablesAndViews(sdb, dbName)
	if err != nil {
		return
	}

	// If a specific table or view was requested, check that it's present
	if dbTable != "" {
		tablePresent := false
		for _, tbl := range tables {
			if tbl == dbTable {
				tablePresent = true
			}
		}
		if tablePresent == false {
			// The requested table or view doesn't exist in the database, so pick one of the tables that does
			for _, t := range tables {
				err = ValidatePGTable(t)
				if err == nil {
					// Validation passed, so use this table or view
					dbTable = t
					defaultTable = t
					break
				}
			}
		}
	}

	// If a specific table wasn't requested, use the first table in the database that passes validation
	if dbTable == "" {
		for _, i := range tables {
			if i != "" {
				err = ValidatePGTable(i)
				if err == nil {
					// The database table name is acceptable, so use it
					dbTable = i
					break
				}
			}
		}
	}

	// If a sort column was requested, verify it exists
	if sortCol != "" {
		var colList []sqlite.Column
		colList, err = sdb.Columns("", dbTable)
		if err != nil {
			log.Printf("Error when reading column names for table '%s': %v", SanitiseLogString(dbTable), err.Error())
			return
		}
		colExists := false
		for _, j := range colList {
			if j.Name == sortCol {
				colExists = true
			}
		}
		if colExists == false {
			// The requested sort column doesn't exist, so we fall back to no sorting
			sortCol = ""
		}
	}

	// Validate the table name, just to be careful
	if dbTable != "" {
		err = ValidatePGTable(dbTable)
		if err != nil {
			// Validation failed, so don't pass on the table name

			// If the failed table name is "{{ db.Tablename }}", don't bother logging it.  It's just a search
			// bot picking up AngularJS in a string and doing a request with it
			if dbTable != "{{ db.Tablename }}" {
				log.Printf("Validation failed for table name: '%s': %s", SanitiseLogString(dbTable), err)
			}
			return
		}
	}

	var okCache bool
	if !isLive {
		rowCacheKey := TableRowsCacheKey(fmt.Sprintf("tablejson/%s/%s/%d", sortCol, sortDir, rowOffset),
			loggedInUser, dbOwner, dbName, commitID, dbTable, maxRows)
		okCache, err = GetCachedData(rowCacheKey, &rowData)
		if err != nil {
			log.Printf("Error retrieving page data from cache: %v", err)
		}
	}

	// If the row data wasn't in cache, read it from the database
	if !okCache {
		rowData, err = ReadSQLiteDB(sdb, dbTable, sortCol, sortDir, maxRows, rowOffset)
		if err != nil {
			// Some kind of error when reading the database data
			return
		}
		rowData.Tablename = dbTable
	}
	return
}

// SQLiteRunQuery runs a SQLite query.  DO NOT use this for user provided SQL queries.  For those,
// use SQLiteRunQueryDefensive().
func SQLiteRunQuery(sdb *sqlite.Conn, querySource QuerySource, dbQuery string, ignoreBinary, ignoreNull bool) (memUsed, memHighWater int64, dataRows SQLiteRecordSet, err error) {
	// Use the sort column as needed
	var stmt *sqlite.Stmt
	stmt, err = sdb.Prepare(dbQuery)
	if err != nil {
		return 0, 0, dataRows, err
	}
	defer stmt.Finalize()

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
				var val int64
				val, isNull, err = s.ScanInt64(i)
				if err != nil {
					log.Printf("Something went wrong with ScanInt64(): %v", err)
					break
				}
				if !isNull {
					stringVal := fmt.Sprintf("%d", val)
					row = append(row, DataValue{Name: dataRows.ColNames[i], Type: Integer, Value: stringVal})
				}
			case sqlite.Float:
				var val float64
				val, isNull, err = s.ScanDouble(i)
				if err != nil {
					log.Printf("Something went wrong with ScanDouble(): %v", err)
					break
				}
				if !isNull {
					stringVal := strconv.FormatFloat(val, 'f', -1, 64)
					row = append(row, DataValue{Name: dataRows.ColNames[i], Type: Float, Value: stringVal})
				}
			case sqlite.Text:
				var val string
				val, isNull = s.ScanText(i)
				if !isNull {
					row = append(row, DataValue{Name: dataRows.ColNames[i], Type: Text, Value: val})
				}
			case sqlite.Blob:
				// BLOBs can be ignored (via flag to this function) for situations like the vis data
				if !ignoreBinary {
					var b []byte
					b, isNull = s.ScanBlob(i)
					if !isNull {
						switch querySource {
						case QuerySourceAPI:
							row = append(row, DataValue{Name: dataRows.ColNames[i], Type: Binary,
								Value: base64.StdEncoding.EncodeToString(b)})
						case QuerySourceInternal:
							stringVal := "x'"
							for _, c := range b {
								stringVal += fmt.Sprintf("%02x", c)
							}
							stringVal += "'"
							row = append(row, DataValue{Name: dataRows.ColNames[i], Type: Binary,
								Value: stringVal})
						default:
							row = append(row, DataValue{Name: dataRows.ColNames[i], Type: Binary,
								Value: "<i>BINARY DATA</i>"})
						}
					}
				} else {
					addRow = false
				}
			case sqlite.Null:
				isNull = true
			}

			// NULLS can be ignored (via flag to this function) for situations like the vis data
			if isNull && !ignoreNull {
				// Different sources of the query have different requirements for the output
				switch querySource {
				case QuerySourceAPI, QuerySourceInternal:
					row = append(row, DataValue{Name: dataRows.ColNames[i], Type: Null})
				default:
					row = append(row, DataValue{Name: dataRows.ColNames[i], Type: Null, Value: "<i>NULL</i>"})
				}
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
		log.Printf("Error when retrieving select data from database: %s", err)
		return 0, 0, dataRows, errors.New("Error when reading data from the SQLite database")
	}

	// Gather memory usage stats for the execution run: https://www.sqlite.org/c3ref/memory_highwater.html
	memUsed = sqlite.MemoryUsed()
	memHighWater = sqlite.MemoryHighwater(false)
	return
}

// SQLiteRunQueryDefensive runs a user provided SQLite query, using our "defensive" mode.  eg with limits placed on
// what it's allowed to do.
func SQLiteRunQueryDefensive(w http.ResponseWriter, r *http.Request, querySource QuerySource, dbOwner, dbName, commitID, loggedInUser, query string) (SQLiteRecordSet, error) {
	// Retrieve the SQLite database from Minio (also doing appropriate permission/access checking)
	sdb, err := OpenSQLiteDatabaseDefensive(w, r, dbOwner, dbName, commitID, loggedInUser)
	if err != nil {
		// The return handling was already done in OpenSQLiteDatabaseDefensive()
		return SQLiteRecordSet{}, err
	}

	// Automatically close the SQLite database when this function finishes
	defer sdb.Close()

	// Was a user agent part of the request?
	var userAgent string
	if ua, ok := r.Header["User-Agent"]; ok {
		userAgent = ua[0]
	}

	// Log the SQL query (prior to executing it)
	var logID int64
	var source string
	switch querySource {
	case QuerySourceAPI:
		source = "api"
	case QuerySourceVisualisation:
		source = "vis"
	default:
		return SQLiteRecordSet{}, fmt.Errorf("Unknown source in SQLiteRunQueryDefensive()")
	}
	logID, err = database.LogSQLiteQueryBefore(source, dbOwner, dbName, loggedInUser, r.RemoteAddr, userAgent, query)
	if err != nil {
		return SQLiteRecordSet{}, err
	}

	// Execute the SQLite select query (or queries)
	var dataRows SQLiteRecordSet
	var memUsed, memHighWater int64
	memUsed, memHighWater, dataRows, err = SQLiteRunQuery(sdb, querySource, query, false, false)
	if err != nil {
		e := err.Error()
		if strings.HasPrefix(e, "not authorized") {
			err = errors.New("SQL that modifies a database can only be used on Live databases")
		} else {
			log.Printf("Error when running query by '%s' for database (%s/%s): '%s'", SanitiseLogString(loggedInUser),
				SanitiseLogString(dbOwner), SanitiseLogString(dbName), SanitiseLogString(e))
		}
		return SQLiteRecordSet{}, err
	}

	// Add the SQLite execution stats to the log record
	err = database.LogSQLiteQueryAfter(logID, memUsed, memHighWater)
	if err != nil {
		return SQLiteRecordSet{}, err
	}
	return dataRows, err
}

// SQLiteRunQueryLive is used by our job queue backend infrastructure to run a user provided SQLite query
func SQLiteRunQueryLive(baseDir, dbOwner, dbName, loggedInUser, query string) (records SQLiteRecordSet, err error) {
	// Open the database on the local node
	var sdb *sqlite.Conn
	sdb, err = OpenSQLiteDatabaseLive(baseDir, dbOwner, dbName)
	if err != nil {
		return
	}
	defer sdb.Close()

	// Log the SQL query (prior to executing it)
	logID, err := database.LogSQLiteQueryBefore("LIVE api", dbOwner, dbName, loggedInUser, "-", "-", query)
	if err != nil {
		return SQLiteRecordSet{}, err
	}

	// Execute the SQLite select query (or queries)
	memUsed, memHighWater, records, err := SQLiteRunQuery(sdb, QuerySourceAPI, query, false, false)
	if err != nil {
		log.Printf("Error when running LIVE query by '%s' for LIVE database (%s/%s): '%s'", SanitiseLogString(loggedInUser),
			SanitiseLogString(dbOwner), SanitiseLogString(dbName), SanitiseLogString(err.Error()))
		return SQLiteRecordSet{}, err
	}

	// Add the SQLite execution stats to the log record
	err = database.LogSQLiteQueryAfter(logID, memUsed, memHighWater)
	if err != nil {
		return SQLiteRecordSet{}, err
	}
	return
}

// SQLiteVersionNumber returns the version number of the available SQLite library, in 300X00Y format.
func SQLiteVersionNumber() int32 {
	return sqlite.VersionNumber()
}

// Tables returns the list of tables in the SQLite database.
func Tables(sdb *sqlite.Conn) (tbl []string, err error) {
	// Retrieve the list of tables in the database
	tbl, err = sdb.Tables("")
	if err != nil {
		// An error occurred, so get the extended error code
		if cerr, ok := err.(sqlite.ConnError); ok {
			// Check if the error was due to the table being locked
			extCode := cerr.ExtendedCode()
			if extCode == 5 { // Magic number which (in this case) means "database is locked"
				// Wait 3 seconds then try again
				time.Sleep(3 * time.Second)
				tbl, err = sdb.Tables("")
				if err != nil {
					log.Printf("Error retrieving table names: %s", err)
					return
				}
			} else {
				log.Printf("Error retrieving table names: %s", err)
				return
			}
		} else {
			log.Printf("Error retrieving table names: %s", err)
			return
		}
	}
	return
}

// TablesAndViews returns the list of tables and views in the SQLite database.
func TablesAndViews(sdb *sqlite.Conn, dbName string) (list []string, err error) {
	// TODO: It might be useful to cache this info in PG or memcached
	// Retrieve the list of tables in the database
	list, err = Tables(sdb)
	if err != nil {
		return
	}
	if len(list) == 0 {
		// No table names were returned, so abort
		log.Printf("The database '%s' doesn't seem to have any tables. Aborting.", SanitiseLogString(dbName))
		return
	}

	// Retrieve the list of views in the database
	var vw []string
	vw, err = Views(sdb)
	if err != nil {
		return
	}

	// Merge the table and view lists
	list = append(list, vw...)
	sort.Strings(list)
	return
}

// Views returns the list of views in the SQLite database.
func Views(sdb *sqlite.Conn) (vw []string, err error) {
	// Retrieve the list of views in the database
	vw, err = sdb.Views("")
	if err != nil {
		log.Printf("Error retrieving view names: %v", err)
		if cerr, ok := err.(sqlite.ConnError); ok {
			log.Printf("Error code: %v", cerr.Code())
			log.Printf("Extended error code: %v", cerr.ExtendedCode())
			log.Printf("Extended error message: %v", cerr.Error())
			log.Printf("Extended error filename: %v", cerr.Filename())
		} else {
			log.Printf("Expected a connection error, but got a '%v'", reflect.TypeOf(cerr))
		}
		return
	}
	return
}

// EscapeId puts an SQL identifier in quote characters and escapes any quote characters it contains, making it safe for use in SQL queries
func EscapeId(id string) string {
	return sqlite.Mprintf("\"%w\"", id)
}

// EscapeIds does the same as EscapeId but for a slice of identifiers
func EscapeIds(ids []string) (escaped []string) {
	for _, i := range ids {
		escaped = append(escaped, EscapeId(i))
	}
	return escaped
}

// EscapeValue formats, quotes and escapes a DataValue for use in SQL queries
func EscapeValue(val DataValue) string {
	if val.Type == Null {
		return "NULL"
	} else if val.Type == Integer || val.Type == Float {
		return val.Value.(string)
	} else if val.Type == Text {
		return sqlite.Mprintf("%Q", val.Value.(string))
	} else { // BLOB and similar
		return val.Value.(string)
	}
}

// GetPrimaryKeyAndOtherColumns figures out the primary key columns and the other columns of a table.
// The schema and table parameters specify the schema and table names to use.
// This function returns two arrays: One containing the list of primary key columns in the same order as they
// are used in the primary key. The other array contains a list of all the other, non-primary key columns.
// Generated columns are ignored completely. If the primary key exists only implicitly, i.e. it's the rowid
// column, the implicitPk flag is set to true.
func GetPrimaryKeyAndOtherColumns(sdb *sqlite.Conn, schema, table string) (pks []string, implicitPk bool, other []string, err error) {
	// Prepare query
	var stmt *sqlite.Stmt
	stmt, err = sdb.Prepare("PRAGMA " + EscapeId(schema) + ".table_info(" + EscapeId(table) + ")")
	if err != nil {
		log.Printf("Error when preparing statement in GetPrimaryKeyAndOtherColumns(): %s", err)
		return nil, false, nil, err
	}
	defer stmt.Finalize()

	// Execute query and retrieve all columns
	primaryKeyColumns := make(map[int]string)
	var hasColumnRowid, hasColumn_Rowid_, hasColumnOid bool
	err = stmt.Select(func(s *sqlite.Stmt) error {
		// Get name and primary key order
		columnName, _ := s.ScanText(1)
		pkOrder, _, _ := s.ScanInt(5)

		// Is this column part of the primary key?
		if pkOrder == 0 {
			// It's not
			other = append(other, columnName)
		} else {
			// It is
			primaryKeyColumns[pkOrder] = columnName
		}

		// While here check if there are any columns called rowid or similar in this table
		if columnName == "rowid" {
			hasColumnRowid = true
		} else if columnName == "_rowid_" {
			hasColumn_Rowid_ = true
		} else if columnName == "oid" {
			hasColumnOid = true
		}
		return nil
	})
	if err != nil {
		log.Printf("Error when retrieving rows in GetPrimaryKeyAndOtherColumns(): %s", err)
		return nil, false, nil, err
	}

	// When there are no rows at all, exit here
	if len(primaryKeyColumns) == 0 && len(other) == 0 {
		return
	}

	// Did we get any primary key columns? If not, this table has only an implicit primary key which
	// is accessible by the name rowid, _rowid_, or oid
	if len(primaryKeyColumns) > 0 {
		// Explicit primary key

		implicitPk = false

		// Sort the columns by their order in the PK
		keys := make([]int, 0, len(primaryKeyColumns))
		for k := range primaryKeyColumns {
			keys = append(keys, k)
		}
		sort.Ints(keys)

		// Return columns in order
		for _, k := range keys {
			pks = append(pks, primaryKeyColumns[k])
		}
	} else {
		// Implicit primary key

		// Views do not even have implicit keys
		vws, err := Views(sdb)
		if err != nil {
			log.Printf("Error retrieving views: %s", err)
			return nil, false, nil, err
		}

		isView := false
		for _, vw := range vws {
			if vw == table {
				isView = true
				break
			}
		}

		if isView == false {
			implicitPk = true

			if !hasColumnRowid {
				pks = append(pks, "rowid")
			} else if !hasColumn_Rowid_ {
				pks = append(pks, "_rowid_")
			} else if !hasColumnOid {
				pks = append(pks, "oid")
			} else {
				log.Println("Unreachable rowid column in GetPrimaryKey()")
				return nil, false, nil, errors.New("Unreachable rowid column")
			}
		}
	}

	return pks, implicitPk, other, nil
}
