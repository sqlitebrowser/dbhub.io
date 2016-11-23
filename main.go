package main

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"html/template"
	"log"
	"net/http"
)

type ValType int
const (
	Binary ValType = iota
	Image
	Null
	Text
)
type dataValue struct {
	Name string
	Type ValType
	Value interface{}
}
type dataRow []dataValue
type dbInfo struct {
	TableHeaders []string
	Records []dataRow
	Tables []string
	Username string
	Database string
	Tablename string
	Watchers int
	Stars int
	Forks int
	Discussions int
	PRs int
	Description string
	Updates int
	Branches int
	Releases int
	Contributors int
	Readme string
}

func main() {
	http.HandleFunc("/", mainHandler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}


func mainHandler(w http.ResponseWriter, _ *http.Request) {

	// Parse the template, but use "[[" and "]]" as delimiters.  This is because both Go and AngularJS use
	// "{{" "}}" by default, so one needs to be changed ;)
	t := template.New("index.html")
	t.Delims("[[", "]]")
	t, err := t.ParseFiles("templates/index.html")
	if err != nil {
		log.Printf("Error: %s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Open database
	db, err := sql.Open("sqlite3", "/Users/jc/tmp/devcrapdb1.sqlite")
	if err != nil {
		log.Fatalf("Couldn't open database: %s", err)
	}
	if err = db.Ping(); err != nil {
		log.Fatal(err)
	}

	// Retrieve the list of tables in the database
	var dataRows dbInfo
	var rowCount int
	tableRows, err := db.Query(
		"SELECT name FROM sqlite_master WHERE type='table' ORDER BY name ASC",
	)
	defer tableRows.Close()
	for tableRows.Next() {
		var name string
		if err := tableRows.Scan(&name); err != nil {
			log.Fatal(err)
		}
		dataRows.Tables = append(dataRows.Tables, name)
		rowCount += 1
	}
	if rowCount == 0 {
		// No table names were returned, so abort
		log.Fatal("The database doesn't seem to have any tables???.  Aborting.")
	}

	// Select the first table
	selectedTable := dataRows.Tables[0]

	// Retrieve (up to) x rows from the selected database
	// Ugh, have to use string smashing for this, even though the SQL spec doesn't seem to say table names
	// shouldn't be parameterised.  Limitation from SQLite's implementation? :(
	queryRows, err := db.Query("SELECT * FROM " + selectedTable + " LIMIT 8")
	defer queryRows.Close()

	// Ready the data for template binding
	if err != nil {
		log.Fatal(err)
	}
	dataRows.TableHeaders, err = queryRows.Columns()
	if err != nil {
		log.Fatal(err)
	}

	rowCount = 0
	numColumns := len(dataRows.TableHeaders)
	for queryRows.Next() {
		colValues := make([]string, numColumns)
		p := make([]interface{}, numColumns)
		for i := range colValues {
			p[i] = &colValues[i]
		}
		if err := queryRows.Scan(p...); err != nil {
			log.Fatal(err)
		}
		var newRecord []dataValue
		for idx, val := range colValues {
			newRecord = append(newRecord, dataValue{ Name: dataRows.TableHeaders[idx], Type: Text,
				Value: val })
		}
		dataRows.Records = append(dataRows.Records, newRecord)
		rowCount += 1
	}

	dataRows.Username = "foo"
	dataRows.Database = "devcrapdb1"
	dataRows.Watchers = 0
	dataRows.Tablename = selectedTable
	dataRows.Stars = 0
	dataRows.Forks = 0
	dataRows.Discussions = 0
	dataRows.PRs = 0
	dataRows.Description =  "Short description of the database goes here..."
	dataRows.Updates = 1
	dataRows.Branches = 1
	dataRows.Releases = 0
	dataRows.Contributors = 1
	dataRows.Readme ="Longer project description goes here..."

	t.Execute(w, dataRows)
}