package main

// Validate the provided user and database name
func validateUserDB(user string, db string) error {

	errs := validate.Var(user, "required,alphanum,min=3,max=63")
	if errs != nil {
		return errs
	}

	errs = validate.Var(db, "required,alphanum|contains=.,min=1,max=1024")
	if errs != nil {
		return errs
	}

	return nil
}

// Validate the provided user, database, and table name
func validateUserDBTable(user string, db string, table string) error {

	errs := validateUserDB(user, db)
	if errs != nil {
		return errs
	}

	// TODO: Improve this to work with all valid SQLite identifiers
	// TODO  Not seeing a definitive reference page for SQLite yet, so using the PostgreSQL one is
	// TODO  probably ok as a fallback:
	// TODO      https://www.postgresql.org/docs/current/static/sql-syntax-lexical.html#SQL-SYNTAX-IDENTIFIERS
	// TODO: Should we exclude SQLite internal tables too? (eg "sqlite_*" https://sqlite.org/lang_createtable.html)
	errs = validate.Var(table, "required,alphanum|contains=-|contains=_|contains=.,max=63")
	if errs != nil {
		return errs
	}

	return nil
}
