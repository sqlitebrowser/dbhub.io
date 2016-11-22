package main

import (
	"html/template"
	"log"
	"net/http"
)

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

	t.Execute(w, nil)
}