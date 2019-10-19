// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include <sqlite3.h>
#include <stdlib.h>
#include "_cgo_export.h"

void goSqlite3SetAuxdata(sqlite3_context *ctx, int N, void *ad) {
	sqlite3_set_auxdata(ctx, N, ad, goXAuxDataDestroy);
}

static inline void cXFunc(sqlite3_context *ctx, int argc, sqlite3_value **argv) {
	void *udf = sqlite3_user_data(ctx);
	void *goctx = sqlite3_get_auxdata(ctx, 0);
	goXFunc(ctx, udf, goctx, argc, argv);
}

static inline void cXStep(sqlite3_context *ctx, int argc, sqlite3_value **argv) {
	void *udf = sqlite3_user_data(ctx);
	goXStep(ctx, udf, argc, argv);
}

static inline void cXFinal(sqlite3_context *ctx) {
	void *udf = sqlite3_user_data(ctx);
	goXFinal(ctx, udf);
}

int goSqlite3CreateScalarFunction(sqlite3 *db, const char *zFunctionName, int nArg, int eTextRep, void *pApp) {
	return sqlite3_create_function_v2(db, zFunctionName, nArg, eTextRep, pApp, cXFunc, 0, 0, goXDestroy);
}
int goSqlite3CreateAggregateFunction(sqlite3 *db, const char *zFunctionName, int nArg, int eTextRep, void *pApp) {
	return sqlite3_create_function_v2(db, zFunctionName, nArg, eTextRep, pApp, 0, cXStep, cXFinal, goXDestroy);
}
