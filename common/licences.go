package common

import (
	"io/ioutil"
	"log"
	"path/filepath"
)

// AddDefaultLicences adds the default licences to the PostgreSQL database.  Generally useful for populating a new
// database, or adding new entries to an existing one
func AddDefaultLicences() (err error) {
	// The default licences to load into the system
	type licenceInfo struct {
		DisplayOrder int
		FileFormat   string
		FullName     string
		Path         string
		URL          string
	}
	licences := map[string]licenceInfo{
		"Not specified": {
			DisplayOrder: 100,
			FileFormat:   "text",
			FullName:     "No licence specified",
			Path:         "",
			URL:          ""},
		"CC0": {
			DisplayOrder: 200,
			FileFormat:   "text",
			FullName:     "Creative Commons Zero 1.0",
			Path:         "CC0-1.0.txt",
			URL:          "https://creativecommons.org/publicdomain/zero/1.0/"},
		"CC-BY-4.0": {
			DisplayOrder: 300,
			FileFormat:   "text",
			FullName:     "Creative Commons Attribution 4.0 International",
			Path:         "CC-BY-4.0.txt",
			URL:          "https://creativecommons.org/licenses/by/4.0/"},
		"CC-BY-SA-4.0": {
			DisplayOrder: 400,
			FileFormat:   "text",
			FullName:     "Creative Commons Attribution-ShareAlike 4.0 International",
			Path:         "CC-BY-SA-4.0.txt",
			URL:          "https://creativecommons.org/licenses/by-sa/4.0/"},
		"CC-BY-NC-4.0": {
			DisplayOrder: 500,
			FileFormat:   "text",
			FullName:     "Creative Commons Attribution-NonCommercial 4.0 International",
			Path:         "CC-BY-NC-4.0.txt",
			URL:          "https://creativecommons.org/licenses/by-nc/4.0/"},
		"CC-BY-IGO-3.0": {
			DisplayOrder: 600,
			FileFormat:   "html",
			FullName:     "Creative Commons Attribution 3.0 IGO",
			Path:         "CC-BY-IGO-3.0.html",
			URL:          "https://creativecommons.org/licenses/by/3.0/igo/"},
		"ODbL-1.0": {
			DisplayOrder: 700,
			FileFormat:   "text",
			FullName:     "Open Data Commons Open Database License 1.0",
			Path:         "ODbL-1.0.txt",
			URL:          "https://opendatacommons.org/licenses/odbl/1.0/"},
		"UK-OGL-3": {
			DisplayOrder: 800,
			FileFormat:   "html",
			FullName:     "United Kingdom Open Government Licence 3",
			Path:         "UK-OGL3.html",
			URL:          "https://www.nationalarchives.gov.uk/doc/open-government-licence/version/3/"},
	}

	// Add the default licences to PostgreSQL
	for lName, l := range licences {
		txt := []byte{}
		if l.Path != "" {
			// Read the file contents
			txt, err = ioutil.ReadFile(filepath.Join(Conf.Licence.LicenceDir, l.Path))
			if err != nil {
				return err
			}
		}

		// Save the licence text, sha256, and friendly name in the database
		err = StoreLicence("default", lName, txt, l.URL, l.DisplayOrder, l.FullName, l.FileFormat)
		if err != nil {
			return err
		}
	}
	log.Println("Default licences added")
	return nil
}
