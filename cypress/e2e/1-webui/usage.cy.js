describe("usage page", () => {
	before(() => {
		// Seed data
		cy.request("/x/test/seed")
	})

	// Admins are allowed to access other people's usage site but non-admins aren't
	it("admin", () => {
		// As user default, an admin user, and for the default user
		cy.visit("usage")
		cy.get("h3").should("not.contain", "for user")

		// As user default, an admin user, for the first user
		cy.visit("usage?username=first")
		cy.get("h3").should("contain", "for user 'first'")

		// As user first, a non-admin user, for the first user
		cy.request("/x/test/switchfirst")
		cy.visit("usage")
		cy.get("h3").should("not.contain", "for user")

		// As user first, a non-admin user, for the second user
		cy.visit("usage?username=second")
		cy.get("h3").should("not.contain", "for user")
	})
})
