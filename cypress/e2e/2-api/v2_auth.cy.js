import path from "path";

describe("api v2 tests", () => {
	before(() => {
		// Seed data
		cy.request("/x/test/seed")
	})

	// status
	//   Equivalent curl command:
	//     curl -k -H "Authorization: Apikey Rh3fPl6cl84XEw2FeWtj-FlUsn9OrxKz9oSJfe6kho7jT_1l5hizqw" https://localhost:9444/v2/status
	it("status", () => {
		cy.request({
			method: "GET",
			url: "https://localhost:9444/v2/status",
			headers: {
				"Authorization": "Apikey Rh3fPl6cl84XEw2FeWtj-FlUsn9OrxKz9oSJfe6kho7jT_1l5hizqw",
			},
		}).then(
			response => {
				expect(response.status).to.eq(200)
				const body = response.body
				expect(body).to.have.property("status", "ok")
			}
		)
	})
})
