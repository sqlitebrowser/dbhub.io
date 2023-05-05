describe('002-visualisation-base64url-padding-bug', () => {
  before(() => {
    // Seed data
    cy.request('/x/test/seed')
  })

  // Run a SQL visualisation which used to trigger a base64url encoding padding bug
  it('run a visualisation query whose base64url encoding does not have a padding character', () => {
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="newvisbtn"]').click()
    cy.get('[data-cy="sqltab"]').click()
    cy.get('[name="usersql"]').type('{selectall}{backspace}').type(
      'SELECT Constituency_Name, Constituency_Number\n' +
      'FROM Constituency_Turnout_Information\n' +
      'WHERE Constituency_Number > 0\n' +
      'ORDER BY Constituency_Name ASC\n' +
      'LIMIT 10')
    cy.get('[data-cy="runsqlbtn"]').click()
    cy.wait(150) // Needs a bit of a delay here, otherwise any error status message may be missed
    cy.get('[data-cy="statusmsg"]').should('contain.text', 'successfully')
  })
})
