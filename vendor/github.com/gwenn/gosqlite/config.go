// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlite

/*
#include <sqlite3.h>
#include <stdlib.h>

// cgo doesn't support varargs
static inline int my_db_config(sqlite3 *db, int op, int v, int *ok) {
	return sqlite3_db_config(db, op, v, ok);
}

int goSqlite3ConfigThreadMode(int mode);
int goSqlite3Config(int op, int mode);
*/
import "C"

import "unsafe"

// ThreadingMode enumerates SQLite threading mode
// See ConfigThreadingMode
type ThreadingMode int32

// SQLite threading modes
const (
	SingleThread ThreadingMode = C.SQLITE_CONFIG_SINGLETHREAD
	MultiThread  ThreadingMode = C.SQLITE_CONFIG_MULTITHREAD
	Serialized   ThreadingMode = C.SQLITE_CONFIG_SERIALIZED
)

// ConfigThreadingMode alters threading mode.
// (See sqlite3_config(SQLITE_CONFIG_SINGLETHREAD|SQLITE_CONFIG_MULTITHREAD|SQLITE_CONFIG_SERIALIZED): http://sqlite.org/c3ref/config.html)
func ConfigThreadingMode(mode ThreadingMode) error {
	rv := C.goSqlite3ConfigThreadMode(C.int(mode))
	if rv == C.SQLITE_OK {
		return nil
	}
	return Errno(rv)
}

// ConfigMemStatus enables or disables the collection of memory allocation statistics.
// (See sqlite3_config(SQLITE_CONFIG_MEMSTATUS): http://sqlite.org/c3ref/config.html)
func ConfigMemStatus(b bool) error {
	rv := C.goSqlite3Config(C.SQLITE_CONFIG_MEMSTATUS, btocint(b))
	if rv == C.SQLITE_OK {
		return nil
	}
	return Errno(rv)
}

// ConfigURI enables or disables URI handling.
// (See sqlite3_config(SQLITE_CONFIG_URI): http://sqlite.org/c3ref/config.html)
func ConfigURI(b bool) error {
	rv := C.goSqlite3Config(C.SQLITE_CONFIG_URI, btocint(b))
	if rv == C.SQLITE_OK {
		return nil
	}
	return Errno(rv)
}

// EnableSharedCache enables or disables shared pager cache
// (See http://sqlite.org/c3ref/enable_shared_cache.html)
func EnableSharedCache(b bool) error {
	rv := C.sqlite3_enable_shared_cache(btocint(b))
	if rv == C.SQLITE_OK {
		return nil
	}
	return Errno(rv)
}

// EnableFKey enables or disables the enforcement of foreign key constraints.
// Calls sqlite3_db_config(db, SQLITE_DBCONFIG_ENABLE_FKEY, b).
// Another way is PRAGMA foreign_keys = boolean;
//
// (See http://sqlite.org/c3ref/c_dbconfig_enable_fkey.html)
func (c *Conn) EnableFKey(b bool) (bool, error) {
	return c.queryOrSetEnableDbConfig(C.SQLITE_DBCONFIG_ENABLE_FKEY, btocint(b))
}

// IsFKeyEnabled reports if the enforcement of foreign key constraints is enabled or not.
// Calls sqlite3_db_config(db, SQLITE_DBCONFIG_ENABLE_FKEY, -1).
// Another way is PRAGMA foreign_keys;
//
// (See http://sqlite.org/c3ref/c_dbconfig_enable_fkey.html)
func (c *Conn) IsFKeyEnabled() (bool, error) {
	return c.queryOrSetEnableDbConfig(C.SQLITE_DBCONFIG_ENABLE_FKEY, -1)
}

// EnableTriggers enables or disables triggers.
// Calls sqlite3_db_config(db, SQLITE_DBCONFIG_ENABLE_TRIGGER, b).
//
// (See http://sqlite.org/c3ref/c_dbconfig_enable_fkey.html)
func (c *Conn) EnableTriggers(b bool) (bool, error) {
	return c.queryOrSetEnableDbConfig(C.SQLITE_DBCONFIG_ENABLE_TRIGGER, btocint(b))
}

// AreTriggersEnabled checks if triggers are enabled.
// Calls sqlite3_db_config(db, SQLITE_DBCONFIG_ENABLE_TRIGGER, -1)
//
// (See http://sqlite.org/c3ref/c_dbconfig_enable_fkey.html)
func (c *Conn) AreTriggersEnabled() (bool, error) {
	return c.queryOrSetEnableDbConfig(C.SQLITE_DBCONFIG_ENABLE_TRIGGER, -1)
}

func (c *Conn) queryOrSetEnableDbConfig(key, i C.int) (bool, error) {
	var ok C.int
	rv := C.my_db_config(c.db, key, i, &ok)
	if rv == C.SQLITE_OK {
		return (ok == 1), nil
	}
	return false, c.error(rv)
}

// EnableExtendedResultCodes enables or disables the extended result codes feature of SQLite.
// (See http://sqlite.org/c3ref/extended_result_codes.html)
func (c *Conn) EnableExtendedResultCodes(b bool) error {
	return c.error(C.sqlite3_extended_result_codes(c.db, btocint(b)), "Conn.EnableExtendedResultCodes")
}

// CompileOptionUsed returns false or true indicating whether the specified option was defined at compile time.
// (See http://sqlite.org/c3ref/compileoption_get.html)
func CompileOptionUsed(optName string) bool {
	cOptName := C.CString(optName)
	defer C.free(unsafe.Pointer(cOptName))
	return C.sqlite3_compileoption_used(cOptName) == 1
}
