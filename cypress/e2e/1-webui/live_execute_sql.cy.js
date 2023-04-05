// Sometimes we need a delay between making a change, and testing it, otherwise the AngularJS changes are missed
let waitTime = 250;

describe('live execute', () => {
  before(() => {
    // Seed data
    cy.request('/x/test/seed')
  })

  // Save a SQL statement
  it('save a SQL statement using non-default name', () => {
    cy.visit('/exec/default/Join Testing with index.sqlite')
    cy.get('[data-cy="sqltab"]').click()
    cy.get('[data-cy="usersqltext"]').type(
      'CREATE TABLE livetest1(\n' +
        'field1 INTEGER, field2 TEXT, field3 INTEGER\n' +
        ')')
    cy.get('[data-cy="nameinput"]').type('{selectall}{backspace}').type('livetest1')
    cy.get('[data-cy="savebtn"]').click()
    cy.get('[data-cy="statusmsg"]').should('contain.text', 'SQL statement \'livetest1\' saved')

    // Verify the SQL statement - it should be automatically selected in the drop down list as it's the only one
    cy.visit('/exec/default/Join Testing with index.sqlite')
    cy.get('[data-cy="selectedexec"]').should('contain.text', 'livetest1')
  })

  // Check if 'default' SQL statement is still chosen by default even when created after non-default ones
  it('save a SQL statement using default name', () => {
    cy.visit('/exec/default/Join Testing with index.sqlite')
    cy.get('[data-cy="sqltab"]').click()
    cy.get('[data-cy="usersqltext"]').type('{selectall}{backspace}').type(
      'CREATE TABLE defaulttest (field2 INTEGER)')
    cy.get('[data-cy="nameinput"]').type('{selectall}{backspace}').type('default')
    cy.get('[data-cy="savebtn"]').click()
    cy.get('[data-cy="statusmsg"]').should('contain.text', 'SQL statement \'default\' saved')

    // Verify the SQL statement - it should be automatically selected in the drop down list as it's the default
    cy.visit('/exec/default/Join Testing with index.sqlite')
    cy.get('[data-cy="selectedexec"]').should('contain.text', 'default')
    cy.get('[data-cy="usersqltext"]').should('contain.text',
      'CREATE TABLE defaulttest (field2 INTEGER)')
  })

  // Save a SQL statement
  it('save a SQL statement with name alphabetically lower than \'default\'', () => {
    cy.visit('/exec/default/Join Testing with index.sqlite')
    cy.get('[data-cy="sqltab"]').click()
    cy.get('[data-cy="usersqltext"]').type('{selectall}{backspace}').type(
      'CREATE TABLE stuff (field3 INTEGER)')
    cy.get('[data-cy="nameinput"]').type('{selectall}{backspace}').type('abc')
    cy.get('[data-cy="savebtn"]').click()
    cy.get('[data-cy="statusmsg"]').should('contain.text', 'SQL statement \'abc\' saved')

    // Check that the SQL statement is present, but not automatically selected when the page loads
    cy.visit('/exec/default/Join Testing with index.sqlite')
    cy.get('[data-cy="selectedexec"]').should('not.contain.text', 'abc')
  })

  // Save over an existing SQL statement
  it('save over an existing SQL statement', () => {
    cy.visit('/exec/default/Join Testing with index.sqlite')
    cy.get('[data-cy="sqltab"]').click()
    cy.get('[data-cy="usersqltext"]').type('{selectall}{backspace}').type(
      'DROP TABLE table1')
    cy.get('[data-cy="nameinput"]').type('{selectall}{backspace}').type('default')
    cy.get('[data-cy="savebtn"]').click()
    cy.get('[data-cy="statusmsg"]').should('contain.text', 'SQL statement \'default\' saved')

    // Verify the new SQL statement text
    cy.visit('/exec/default/Join Testing with index.sqlite')
    cy.get('[data-cy="execdropdown"]').click()
    cy.get('[data-cy="exec-default"]').click()
    cy.get('[data-cy="usersqltext"]').should('contain',
      'DROP TABLE table1')
  })

  // "Format SQL" button
  it('"Format SQL" button', () => {
    // Start with the existing "livetest1" test
    cy.visit('/exec/default/Join Testing with index.sqlite')
    cy.get('[data-cy="execdropdown"]').click()
    cy.get('[data-cy="exec-livetest1"]').click()

    // Click the format button
    cy.get('[data-cy="formatsqlbtn"]').click()

    // Verify the changed text
    cy.get('[data-cy="usersqltext"]').should('contain',
      'CREATE TABLE\n' +
        '  livetest1 (field1 INTEGER, field2 TEXT, field3 INTEGER)')
  })

  // "Execute SQL" button
  it('"Execute SQL" button', () => {
    // Load the data view page, and ensure the view we're about to remove is present
    cy.visit('/default/Join Testing with index.sqlite')
    cy.get('[data-cy="tabledropdown"]').click()
    cy.get('[data-cy="row-joinedView"]').should('exist')

    // Remove the view using the Execute SQL button
    cy.visit('/exec/default/Join Testing with index.sqlite')
    cy.get('[data-cy="sqltab"]').click()
    cy.get('[data-cy="usersqltext"]').type('{selectall}{backspace}').type(
        'DROP VIEW joinedView')
    cy.get('[data-cy="execsqlbtn"]').click()

    // Verify the view has been removed
    cy.visit('/default/Join Testing with index.sqlite')
    cy.get('[data-cy="tabledropdown"]').click()
    cy.get('[data-cy="row-joinedView"]').should('not.exist')
  })

  // "Delete" button
  it('Delete button', () => {
    cy.visit('/exec/default/Join Testing with index.sqlite')
    cy.get('[data-cy="execdropdown"]').click()
    cy.get('[data-cy="exec-abc"]').click()

    // Click the Delete button
    cy.get('[data-cy="delexecbtn"]').click()

    // Verify the result
    cy.wait(waitTime)
    cy.get('[data-cy="execdropdown"]').click()
    cy.get('[data-cy="exec-abc"]').should('not.exist')
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