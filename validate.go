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

	// TODO: Improve this to work with all valid PostgreSQL identifiers
	// https://www.postgresql.org/docs/current/static/sql-syntax-lexical.html#SQL-SYNTAX-IDENTIFIERS
	errs = validate.Var(table, "required,alphanum,max=63")
	if errs != nil {
		return errs
	}

	return nil
}