// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <sqlite3.h>
//#include "_cgo_export.h"

int goSqlite3ConfigThreadMode(int mode) {
	return sqlite3_config(mode);
}

int goSqlite3Config(int op, int mode) {
	return sqlite3_config(op, mode);
}
