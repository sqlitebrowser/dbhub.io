// Sometimes we need a delay between making a change, and testing it, otherwise the AngularJS changes are missed
let waitTime = 250;

describe('live execute', () => {
  before(() => {
    // Seed data
    cy.request('/x/test/seed')
  })

  // "Format SQL" button
  it('"Format SQL" button', () => {
    cy.visit('/exec/default/Join Testing with index.sqlite')

    // Type in some SQL statement
    cy.get('.sql-terminal-input').find('textarea').type(
      'CREATE TABLE livetest1(\n' +
        'field1 INTEGER, field2 TEXT, field3 INTEGER\n' +
        ')')

    // Click the format button
    cy.get('[data-cy="dropdownbtn"]').click()
    cy.get('[data-cy="formatbtn"]').click()

    // Verify the changed text
    cy.wait(waitTime)
    cy.get('.sql-terminal-input').find('textarea').should('contain',
      'CREATE TABLE\n' +
        '  livetest1 (field1 INTEGER, field2 TEXT, field3 INTEGER)')
  })

  // "Execute SQL" button
  it('"Execute SQL" button', () => {
    // Load the data view page, and ensure the view we're about to remove is present
    cy.visit('/default/Join Testing with index.sqlite')
    cy.get('[name="viewtable"]').parent('.react-dropdown-select').click()
    cy.get('[name="viewtable"]').siblings('.react-dropdown-select-dropdown').find('.react-dropdown-select-item').contains('joinedView').should('exist')

    // Remove the view using the Execute SQL button
    cy.visit('/exec/default/Join Testing with index.sqlite')
    cy.get('.sql-terminal-input').find('textarea').type(
        'DROP VIEW joinedView')
    cy.get('[data-cy="executebtn"]').click()

    // Verify the view has been removed
    cy.visit('/default/Join Testing with index.sqlite')
    cy.get('[name="viewtable"]').parent('.react-dropdown-select').click()
    cy.get('[name="viewtable"]').siblings('.react-dropdown-select-dropdown').find('.react-dropdown-select-item').contains('joinedView').should('not.exist')
  })

  // Verify only the owner can see this Execute SQL page
  it('Verify private database Execute SQL page is indeed private', () => {
    // Switch to a different user
    cy.request('/x/test/switchfirst')

    // Try accessing a private database's Execute SQL page
    cy.visit({url: '/exec/default/Join Testing with index.sqlite', failOnStatusCode: false})
    cy.get('[data-cy="errormsg"').should('contain', 'doesn\'t exist')

    // Switch back to the default user
    cy.request('/x/test/switchdefault')
  })
})
