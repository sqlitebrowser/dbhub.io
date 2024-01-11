import path from "path";

// Sometimes we need a delay between making a change, and testing it, otherwise the AngularJS changes are missed
let waitTime = 250;

describe('live visualisation', () => {
  before(() => {
    // Seed data
    cy.request('/x/test/seed')
  })

  // Save a visualisation
  it('save a visualisation', () => {
    cy.visit('/vis/default/Join Testing with index.sqlite')
    cy.get('[data-cy="newvisbtn"]').click()
    cy.get('[data-rr-ui-event-key="sql"]').click()
    cy.get('[name="usersql"]').type(
      'SELECT table1.Name, table2.value\n' +
      'FROM table1 JOIN table2\n' +
      'ON table1.id = table2.id\n' +
      'ORDER BY table1.id')
    cy.get('[data-cy="runsqlbtn"]').click()
    cy.wait(150) // Needs a bit of a delay here, otherwise any error status message may be missed
    cy.get('[data-cy="renamebtn"]').click()
    cy.get('[data-cy="nameinput"]').type('{selectall}{backspace}').type('livetest1')
    cy.get('[data-cy="renameokbtn"]').click()
    cy.get('[data-cy="savebtn"]').click()

    // Verify the visualisation - it should be automatically selected in the drop down list as it's the only one
    cy.visit('/vis/default/Join Testing with index.sqlite')
    cy.get('[data-cy="savedvis"]').should('contain.text', 'livetest1')
  })

  // Save over an existing visualisation
  it('save over an existing visualisation', () => {
    cy.visit('/vis/default/Join Testing with index.sqlite')
    cy.get('[data-cy="newvisbtn"]').click()
    cy.get('[data-rr-ui-event-key="sql"]').click()
    cy.get('[name="usersql"]').type('{selectall}{backspace}').type(
      'SELECT table1.Name, table2.value\n' +
      'FROM table1, table2\n' +
      'WHERE table1.id = table2.id\n' +
      'ORDER BY table1.id;')
    cy.get('[data-cy="runsqlbtn"]').click()
    cy.wait(150) // Needs a bit of a delay here, otherwise any error status message may be missed
    cy.get('[data-cy="renamebtn"]').click()
    cy.get('[data-cy="nameinput"]').type('{selectall}{backspace}').type('livetest1')
    cy.get('[data-cy="renameokbtn"]').click()
    cy.wait(200)
    cy.get('.react-confirm-alert-body').should('contain.text', 'already exists')
  })

  // * Chart settings tab *

  // Chart type drop down
  it('chart type drop down', () => {
    // Start with the existing "livetest1" visualisation
    cy.visit('/vis/default/Join Testing with index.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('livetest1').click()

    // Switch to the chart settings tab
    cy.get('[data-rr-ui-event-key="settings"]').click()

    // Change the chart type
    cy.get('[name="charttype"]').parent().click()
    cy.get('[name="charttype"]').siblings('.react-dropdown-select-dropdown').find('.react-dropdown-select-item').contains('Pie chart').click({force: true})
    cy.get('[data-cy="savebtn"]').click()

    // Verify the change
    cy.visit('/vis/default/Join Testing with index.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('livetest1').click()
    cy.get('[data-rr-ui-event-key="settings"]').click()
    cy.get('[name="charttype"]').should('have.value', 'Pie chart')
    cy.get('[data-cy="xtruetoggle"]').should('not.exist')

    // Switch to a different chart type
    cy.get('[name="charttype"]').parent().click()
    cy.get('[name="charttype"]').siblings('.react-dropdown-select-dropdown').find('.react-dropdown-select-item').contains('Horizontal bar chart').click({force: true})
    cy.get('[data-cy="savebtn"]').click()

    // Verify the change
    cy.visit('/vis/default/Join Testing with index.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('livetest1').click()
    cy.get('[data-rr-ui-event-key="settings"]').click()
    cy.get('[name="charttype"]').should('have.value', 'Horizontal bar chart')
    cy.get('[data-cy="xtruetoggle"]').should('exist')
  })

  // X axis column drop down
  it('X axis column drop down', () => {
    // Start with the existing "livetest1" visualisation
    cy.visit('/vis/default/Join Testing with index.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('livetest1').click()

    // Switch to the chart settings tab
    cy.get('[data-rr-ui-event-key="settings"]').click()

    // Change the X axis column value
    cy.get('[name="xaxiscol"]').parent().click()
    cy.get('[name="xaxiscol"]').siblings('.react-dropdown-select-dropdown').find('.react-dropdown-select-item').contains('value').click({force: true})
    cy.get('[data-cy="savebtn"]').click()

    // Verify the change
    cy.visit('/vis/default/Join Testing with index.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('livetest1').click()
    cy.get('[data-rr-ui-event-key="settings"]').click()
    cy.get('[name="xaxiscol"]').should('have.value', 'value')

    // Switch to a different X axis column value
    cy.get('[name="xaxiscol"]').parent().click()
    cy.get('[name="xaxiscol"]').siblings('.react-dropdown-select-dropdown').find('.react-dropdown-select-item').contains('Name').click({force: true})
    cy.get('[data-cy="savebtn"]').click()

    // Verify the change
    cy.visit('/vis/default/Join Testing with index.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('livetest1').click()
    cy.get('[data-rr-ui-event-key="settings"]').click()
    cy.get('[name="xaxiscol"]').should('have.value', 'Name')
  })

  // Y axis column drop down
  it('Y axis column drop down', () => {
    // Add a third column to table2
    cy.visit('/exec/default/Join Testing with index.sqlite')
    cy.get('.sql-terminal-input').find('textarea').type(
        'ALTER TABLE table2 ADD COLUMN value2 INTEGER DEFAULT 8')
    cy.get('[data-cy="executebtn"]').click()

    // Create a visualisation with a third column
    cy.visit('/vis/default/Join Testing with index.sqlite')
    cy.get('[data-cy="newvisbtn"]').click()
    cy.get('[data-rr-ui-event-key="sql"]').click()
    cy.get('[name="usersql"]').type('{selectall}{backspace}').type(
        'SELECT table1.Name, table2.value, table2.value2\n' +
        'FROM table1, table2\n' +
        'WHERE table1.id = table2.id\n' +
        'ORDER BY table1.id')

    // Click the Run SQL button
    cy.get('[data-cy="runsqlbtn"]').click()
    cy.wait(150) // Needs a bit of a delay here, otherwise any error status message may be missed

    // Switch to the chart settings tab
    cy.get('[data-rr-ui-event-key="settings"]').click()

    // Change the Y axis column value
    cy.get('[name="yaxiscol"]').parent().click()
    cy.get('[name="yaxiscol"]').siblings('.react-dropdown-select-dropdown').find('.react-dropdown-select-item').contains('value2').click({force: true})
    cy.get('[data-cy="savebtn"]').click()

    // Verify the change
    cy.visit('/vis/default/Join Testing with index.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('new 1').click()
    cy.get('[data-rr-ui-event-key="settings"]').click()
    cy.get('[name="yaxiscol"]').should('have.value', 'value2')

    // Switch to a different Y axis column value
    cy.get('[name="yaxiscol"]').parent().click()
    cy.get('[name="yaxiscol"]').siblings('.react-dropdown-select-dropdown').find('.react-dropdown-select-item').contains('value').click({force: true})
    cy.get('[data-cy="savebtn"]').click()

    // Verify the change
    cy.visit('/vis/default/Join Testing with index.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('new 1').click()
    cy.get('[data-rr-ui-event-key="settings"]').click()
    cy.get('[name="yaxiscol"]').should('have.value', 'value')
  })

  // "Show result table" button works
  it('Shows results table button works', () => {
    // Start with the existing "livetest1" visualisation
    cy.visit('/vis/default/Join Testing with index.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('livetest1').click()

    // Verify the button starts closed
    cy.contains('button', 'Show data table').should('exist')

    // Open the results table
    cy.contains('button', 'Show data table').click()
    cy.contains('button', 'Hide data table').should('exist')
  })

  // "Download as CSV" button
  const downloadsFolder = Cypress.config('downloadsFolder')
  it('"Download as CSV" button', () => {
    // Start with the existing "livetest1" visualisation
    cy.visit('/vis/default/Join Testing with index.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('livetest1').click()

    // Click the download button
    cy.contains('button', 'Export to CSV').click()

    // Simple sanity check of the downloaded file
    // TODO - Implement a better check.   Maybe keep the "correct" csv in the repo as test data too, and compare against it?
    const csv = path.join(downloadsFolder, 'livetest1.csv')
    cy.readFile(csv, 'binary', { timeout: 5000 }).should('have.length', 90)
    cy.task('rmFile', { path: csv })
  })

  // "Format SQL" button
  it('"Format SQL" button', () => {
    // Start with the existing "livetest1" visualisation
    cy.visit('/vis/default/Join Testing with index.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('livetest1').click()

    // Click the format buttn
    cy.get('[data-rr-ui-event-key="sql"]').click()
    cy.get('[data-cy="formatsqlbtn"]').click()

    // Verify the changed text
    cy.wait(waitTime)
    cy.get('[name="usersql"]').should('have.value',
      'SELECT\n' +
        '  table1.Name,\n' +
        '  table2.value\n' +
        'FROM\n' +
        '  table1\n' +
        '  JOIN table2 ON table1.id = table2.id\n' +
        'ORDER BY\n' +
        '  table1.id')
  })

  // "Run SQL" button
  it('"Run SQL" button', () => {
    // Start with the existing "livetest1" visualisation
    cy.visit('/vis/default/Join Testing with index.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('livetest1').click()

    // Click the Run SQL button
    cy.get('[data-rr-ui-event-key="sql"]').click()
    cy.get('[data-cy="runsqlbtn"]').click()

    // Verify the result
    // TODO: Probably need to add cypress attributes to the rows and columns of the results table, then
    //       check against known good return values for the testing
  })

  // "Delete" button
  it('Delete button', () => {
    cy.visit('/vis/default/Join Testing with index.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('livetest1').click()

    // Click the Delete button
    cy.get('[data-cy="deletebtn"]').click()

    // Confirm dialog
    cy.wait(200)
    cy.get('button[label="Yes"]').click()

    // Verify the result
    cy.wait(waitTime)
    cy.get('[data-cy="savedvis"]').get('.list-group-item').should('not.contain', 'livetest1')
  })

  // Verify only the owner can see this visualisation
  it('Verify private visualisation is indeed private', () => {
    // Switch to a different user
    cy.request('/x/test/switchfirst')

    // Try accessing a private database's visualisation page
    cy.visit({url: '/vis/default/Join Testing with index.sqlite', failOnStatusCode: false})
    cy.get('[data-cy="errormsg"').should('contain', 'doesn\'t exist')

    // Switch back to the default user
    cy.request('/x/test/switchdefault')
  })
})
