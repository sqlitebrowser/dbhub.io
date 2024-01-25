import path from "path";

const roKey = "ReuYtI49nGGA6rEYaBPxS6qdK4mlYRvToucoxjw4ZDiOT9tJ6NxRXw";

describe("permissions", () => {
	before(() => {
		// Seed data
		cy.request("/x/test/seed")
	})

	// Columns
	it("columns", () => {
		cy.request({
			method: "POST",
			url: "https://localhost:9444/v1/columns",
			form: true,
			body: {
				apikey: roKey,
				dbowner: "default",
				dbname: "Join Testing with index.sqlite",
				table: "table1"
			},
		}).then(response => {
			expect(response.status).to.eq(200)
		})
	})

	// Upload database
	it("upload", () => {
		cy.readFile("cypress/test_data/Join Testing with index.sqlite", "binary").then(dbData => {
			const blob = Cypress.Blob.binaryStringToBlob(dbData)

			// Manually construct a form data object, as cy.request() doesn't yet have proper support
			// for form data
			const z = new FormData()
			z.set("apikey", roKey)
			z.set("dbname", "LIVE database upload testing.sqlite")
			z.set("live", "true")
			z.set("file", blob)

			// Send the request
			cy.request({
				method: "POST",
				url: "https://localhost:9444/v1/upload",
				body: z,
				failOnStatusCode: false,
			}).then(response => {
				expect(response.status).to.eq(401)
			})
		})
	})

	// Download
	it("download", () => {
		cy.request({
			method: "POST",
			url: "https://localhost:9444/v1/download",
			form: true,
			encoding: "binary",
			body: {
				apikey: roKey,
				dbowner: "default",
				dbname: "Join Testing with index.sqlite",
			},
		}).then(response => {
			expect(response.status).to.eq(200)
		})
	})

	// Databases
	it("databases", () => {
		cy.request({
			method: "POST",
			url: "https://localhost:9444/v1/databases",
			form: true,
			body: {
				apikey: roKey,
				live: "true"
			},
		}).then(response => {
			expect(response.status).to.eq(200)
		})
	})

	// Execute
	it("execute", () => {
		cy.request({
			method: "POST",
			url: "https://localhost:9444/v1/execute",
			form: true,
			body: {
				apikey: roKey,
				dbowner: "default",
				dbname: "Join Testing with index.sqlite",
				sql: btoa("UPDATE table1 SET Name = \"Testing 1\" WHERE id = 1")
			},
			failOnStatusCode: false,
		}).then(response => {
			expect(response.status).to.eq(401)
		})
	})

	// Indexes
	it("indexes", () => {
		cy.request({
			method: "POST",
			url: "https://localhost:9444/v1/indexes",
			form: true,
			body: {
				apikey: roKey,
				dbowner: "default",
				dbname: "Join Testing with index.sqlite"
			},
		}).then(response => {
			expect(response.status).to.eq(200)
		})
	})

	// Query
	it("query", () => {
		cy.request({
			method: "POST",
			url: "https://localhost:9444/v1/query",
			form: true,
			body: {
				apikey: roKey,
				dbowner: "default",
				dbname: "Join Testing with index.sqlite",
				sql: btoa("SELECT table1.Name, table2.value FROM table1 JOIN table2 USING (id) ORDER BY table1.id")
			},
		}).then(response => {
			expect(response.status).to.eq(200)
		})
	})

	// Tables
	it("tables", () => {
		cy.request({
			method: "POST",
			url: "https://localhost:9444/v1/tables",
			form: true,
			body: {
				apikey: roKey,
				dbowner: "default",
				dbname: "Join Testing with index.sqlite"
			},
		}).then(response => {
			expect(response.status).to.eq(200)
		})
	})

	// Delete
	it("delete", () => {
		cy.request({
			method: "POST",
			url: "https://localhost:9444/v1/delete",
			form: true,
			body: {
				apikey: roKey,
				dbname: "Join Testing with index.sqlite",
			},
			failOnStatusCode: false,
		}).then(response => {
			expect(response.status).to.eq(401)
		})
	})
})
