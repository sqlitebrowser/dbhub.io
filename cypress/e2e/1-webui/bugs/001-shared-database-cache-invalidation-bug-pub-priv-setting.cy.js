describe('001 - shared database cache invalidation bug - pub/priv setting', () => {
  before(() => {
    // Seed data
    cy.request('/x/test/seed')
  })

  // Trigger cache invalidation bug (pub/priv setting)
  it('Trigger pub/priv setting cache invalidation bug', () => {
    // 1. Share the user "default"'s test database with the user "first", and ensure they load the database page for it afterwards while its public
    cy.visit("default/Assembly Election 2017.sqlite")
    cy.get('[data-cy="settingslink"]').click()
    cy.get('[data-cy="usernameinput"]').type('first')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.get('[data-cy="savebtn"]').click()
    cy.request("/x/test/switchfirst")
    cy.visit("default/Assembly Election 2017.sqlite")

    // 2. This will now have the database details in our metadata cache, with visibility set to Public

    // 3. Switch the database to being private
    cy.request("/x/test/switchdefault")
    cy.visit("default/Assembly Election 2017.sqlite")
    cy.get('[data-cy="settingslink"]').click()
    cy.get('[data-cy="private"]').click()
    cy.get('[data-cy="savebtn"]').click()

    // 4. The bug is that when the user "first" now visits the page, they are still shown the database setting of "public"
    //    even though the database has actually been set to private
    cy.request("/x/test/switchfirst")
    cy.visit("default/Assembly Election 2017.sqlite")
    cy.get('[data-cy="vis"]').should('contain.text', 'Private')
  })
})
