// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <sqlite3.h>
#include <stdlib.h>
//#include "_cgo_export.h"

extern void goXTrace(void *udp, const char *sql);

void goSqlite3Trace(sqlite3 *db, void *udp) {
	sqlite3_trace(db, goXTrace, udp);
}

extern void goXProfile(void *udp, const char *sql, sqlite3_uint64 nanoseconds);

void goSqlite3Profile(sqlite3 *db, void *udp) {
	sqlite3_profile(db, goXProfile, udp);
}

extern int goXAuth(void *udp, int action, const char *arg1, const char *arg2, const char *dbName, const char *triggerName);

int goSqlite3SetAuthorizer(sqlite3 *db, void *udp) {
	return sqlite3_set_authorizer(db, goXAuth, udp);
}

extern int goXBusy(void *udp, int count);

int goSqlite3BusyHandler(sqlite3 *db, void *udp) {
	return sqlite3_busy_handler(db, goXBusy, udp);
}

extern int goXProgress(void *udp);

void goSqlite3ProgressHandler(sqlite3 *db, int numOps, void *udp) {
	sqlite3_progress_handler(db, numOps, goXProgress, udp);
}

extern void goXLog(void *udp, int err, const char *msg);

int goSqlite3ConfigLog(void *udp) {
	if (udp) {
		return sqlite3_config(SQLITE_CONFIG_LOG, goXLog, udp);
	} else {
		return sqlite3_config(SQLITE_CONFIG_LOG, 0, 0);
	}
}
