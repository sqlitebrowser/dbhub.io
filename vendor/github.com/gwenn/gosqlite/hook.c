// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <sqlite3.h>
// warning: incompatible pointer types passing
//#include "_cgo_export.h"

extern int goXCommitHook(void *udp);

void* goSqlite3CommitHook(sqlite3 *db, void *udp) {
	return sqlite3_commit_hook(db, goXCommitHook, udp);
}

extern void goXRollbackHook(void *udp);

void* goSqlite3RollbackHook(sqlite3 *db, void *udp) {
	return sqlite3_rollback_hook(db, goXRollbackHook, udp);
}

extern void goXUpdateHook(void *udp, int action, char const *dbName, char const *tableName, sqlite3_int64 rowID);

void* goSqlite3UpdateHook(sqlite3 *db, void *udp) {
	return sqlite3_update_hook(db, goXUpdateHook, udp);
}

/*
extern int goXWalHook(void *udp, sqlite3* db, const char *dbName, int nEntry);

void* goSqlite3WalHook(sqlite3 *db, void *udp) {
	return sqlite3_wal_hook(db, goXWalHook, udp);
}
*/