const unlimitedKey = "Rh3fPl6cl84XEw2FeWtj-FlUsn9OrxKz9oSJfe6kho7jT_1l5hizqw"; // Key of user 'default'
const limitedKey = "R4btZIUCGfLeIPJN1qDtBRuz7I6YWhiM2F0EOh3-neoLxqd9h7J8uw"; // Key of user 'limited'
const waitTime = 150;

describe("usage_limits", () => {
	before(() => {
		// Seed data
		cy.request("/x/test/seed")

		// Set up required share for limited user
		cy.visit("settings/default/Join Testing with index.sqlite")
		cy.get('[data-cy="settingslink"]').click()
		cy.get('[data-cy="usernameinput"]').type("limited")
		cy.get('[data-cy="adduserbtn"]').click()
		cy.get('input[name="shareperm-limited"]').parents(".react-dropdown-select").click()
		cy.get('input[name="shareperm-limited"]').parents(".react-dropdown-select").find(".react-dropdown-select-item").contains("Read only").click({force: true})
		cy.get('[data-cy="savebtn"]').click()
		cy.wait(waitTime)
	})

	// Using an unlimited user a large number of requests should all work
	it("unlimited", () => {
		for (let i = 0; i < 3; i++) {
			cy.request({
				method: "POST",
				url: "https://localhost:9444/v1/webpage",
				form: true,
				body: {
					apikey: unlimitedKey,
					dbowner: "default",
					dbname: "Join Testing with index.sqlite",
				},
			}).then(response => {
				expect(response.status).to.eq(200)
			})
		}
	})

	// Using a limited user requests should fail at some point but then succeed after a waiting period
	it("limited", () => {
		// Info: the user has a token limit of 1 and a restoration rate of one token per second

		// First request should be successful
		cy.request({
			method: "POST",
			url: "https://localhost:9444/v1/webpage",
			failOnStatusCode: false,
			form: true,
			body: {
				apikey: limitedKey,
				dbowner: "default",
				dbname: "Join Testing with index.sqlite",
			},
		}).then(response => {
			expect(response.status).to.eq(200)
		})

		// Second request should fail
		cy.request({
			method: "POST",
			url: "https://localhost:9444/v1/webpage",
			failOnStatusCode: false,
			form: true,
			body: {
				apikey: limitedKey,
				dbowner: "default",
				dbname: "Join Testing with index.sqlite",
			},
		}).then(response => {
			expect(response.status).to.eq(429)
		})

		// Other users should be unaffected (using an unlimited user here)
		cy.request({
			method: "POST",
			url: "https://localhost:9444/v1/webpage",
			failOnStatusCode: false,
			form: true,
			body: {
				apikey: unlimitedKey,
				dbowner: "default",
				dbname: "Join Testing with index.sqlite",
			},
		}).then(response => {
			expect(response.status).to.eq(200)
		})

		// After waiting for some time requests should be allowed again
		cy.wait(2000) // waiting two seconds, just in case
		cy.request({
			method: "POST",
			url: "https://localhost:9444/v1/webpage",
			failOnStatusCode: false,
			form: true,
			body: {
				apikey: limitedKey,
				dbowner: "default",
				dbname: "Join Testing with index.sqlite",
			},
		}).then(response => {
			expect(response.status).to.eq(200)
		})

		// Next call should fail again because there is a limit of one token
		cy.request({
			method: "POST",
			url: "https://localhost:9444/v1/webpage",
			failOnStatusCode: false,
			form: true,
			body: {
				apikey: limitedKey,
				dbowner: "default",
				dbname: "Join Testing with index.sqlite",
			},
		}).then(response => {
			expect(response.status).to.eq(429)
		})

		// Even after more waiting the next calls should fail because the user exceeded the hourly limit
		cy.wait(2000) // waiting two seconds, just in case
		cy.request({
			method: "POST",
			url: "https://localhost:9444/v1/webpage",
			failOnStatusCode: false,
			form: true,
			body: {
				apikey: limitedKey,
				dbowner: "default",
				dbname: "Join Testing with index.sqlite",
			},
		}).then(response => {
			expect(response.status).to.eq(429)
		})
	})
})
