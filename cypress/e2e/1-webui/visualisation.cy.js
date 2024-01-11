import path from "path";

// Sometimes we need a delay between making a change, and testing it, otherwise the AngularJS changes are missed
let waitTime = 250;

// TODO: Test that when provided a commit id, we visualise the data for that commit rather than the latest commit

// TODO: Check the star and watch buttons + counts work properly on the standard / live db visualisations page

// TODO: Test the branch drop down works properly for data selection on the visualisation page

describe('visualisation', () => {
  before(() => {
    // Seed data
    cy.request('/x/test/seed')

    // Create new branch
    cy.visit('default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="commitslnk"]').click()
    cy.get('[data-cy="createbranchbtn"]').click()
    cy.get('[data-cy="nameinput"]').type('{selectall}{backspace}').type('firstBranch')
    cy.get('[data-cy="createbtn"]').click()
  })

  // Save a visualisation
  it('save a visualisation', () => {
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="newvisbtn"]').click()
    cy.get('[data-rr-ui-event-key="sql"]').click()
    cy.get('[name="usersql"]').type(
      'SELECT Constituency_Name, Constituency_Number, Turnout_pct\n' +
      'FROM Constituency_Turnout_Information\n' +
      'ORDER BY Constituency_Name DESC\n' +
      'LIMIT 5')
    cy.get('[data-cy="runsqlbtn"]').click()
    cy.wait(150) // Needs a bit of a delay here, otherwise any error status message may be missed
    cy.get('[data-cy="renamebtn"]').click()
    cy.get('[data-cy="nameinput"]').type('{selectall}{backspace}').type('test1')
    cy.get('[data-cy="renameokbtn"]').click()
    cy.get('[data-cy="savebtn"]').click()

    // Verify the visualisation - it should be automatically selected in the drop down list as it's the only one
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="savedvis"]').should('contain.text', 'test1')
  })

  // Save over an existing visualisation
  it('save over an existing visualisation', () => {
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="newvisbtn"]').click()
    cy.get('[data-rr-ui-event-key="sql"]').click()
    cy.get('[name="usersql"]').type('{selectall}{backspace}').type(
      'SELECT Constituency_Name, Constituency_Number\n' +
      'FROM Constituency_Turnout_Information\n' +
      'ORDER BY Constituency_Name DESC\n' +
      'LIMIT 10')
    cy.get('[data-cy="runsqlbtn"]').click()
    cy.wait(150) // Needs a bit of a delay here, otherwise any error status message may be missed
    cy.get('[data-cy="renamebtn"]').click()
    cy.get('[data-cy="nameinput"]').type('{selectall}{backspace}').type('test1')
    cy.get('[data-cy="renameokbtn"]').click()
    cy.wait(200)
    cy.get('.react-confirm-alert-body').should('contain.text', 'already exists')
  })

  // * Chart settings tab *

  // Chart type drop down
  it('chart type drop down', () => {
    // Start with the existing "test1" test
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('test1').click()

    // Switch to the chart settings tab
    cy.get('[data-rr-ui-event-key="settings"]').click()

    // Change the chart type
    cy.get('[name="charttype"]').parent().click()
    cy.get('[name="charttype"]').siblings('.react-dropdown-select-dropdown').find('.react-dropdown-select-item').contains('Pie chart').click({force: true})
    cy.get('[data-cy="savebtn"]').click()

    // Verify the change
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('test1').click()
    cy.get('[data-rr-ui-event-key="settings"]').click()
    cy.get('[name="charttype"]').should('have.value', 'Pie chart')
    cy.get('[data-cy="xtruetoggle"]').should('not.exist')

    // Switch to a different chart type
    cy.get('[name="charttype"]').parent().click()
    cy.get('[name="charttype"]').siblings('.react-dropdown-select-dropdown').find('.react-dropdown-select-item').contains('Horizontal bar chart').click({force: true})
    cy.get('[data-cy="savebtn"]').click()

    // Verify the change
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('test1').click()
    cy.get('[data-rr-ui-event-key="settings"]').click()
    cy.get('[name="charttype"]').should('have.value', 'Horizontal bar chart')
    cy.get('[data-cy="xtruetoggle"]').should('exist')
  })

  // X axis column drop down
  it('X axis column drop down', () => {
    // Start with the existing "test1" test
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('test1').click()

    // Switch to the chart settings tab
    cy.get('[data-rr-ui-event-key="settings"]').click()

    // Change the X axis column value
    cy.get('[name="xaxiscol"]').parent().click()
    cy.get('[name="xaxiscol"]').siblings('.react-dropdown-select-dropdown').find('.react-dropdown-select-item').contains('Constituency_Number').click({force: true})
    cy.get('[data-cy="savebtn"]').click()

    // Verify the change
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('test1').click()
    cy.get('[data-rr-ui-event-key="settings"]').click()
    cy.get('[name="xaxiscol"]').should('have.value', 'Constituency_Number')

    // Switch to a different X axis column value
    cy.get('[name="xaxiscol"]').parent().click()
    cy.get('[name="xaxiscol"]').siblings('.react-dropdown-select-dropdown').find('.react-dropdown-select-item').contains('Constituency_Name').click({force: true})
    cy.get('[data-cy="savebtn"]').click()

    // Verify the change
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('test1').click()
    cy.get('[data-rr-ui-event-key="settings"]').click()
    cy.get('[name="xaxiscol"]').should('have.value', 'Constituency_Name')
  })

  // Y axis column drop down
  it('Y axis column drop down', () => {
    // Start with the existing "test1" test
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('test1').click()

    // Switch to the chart settings tab
    cy.get('[data-rr-ui-event-key="settings"]').click()

    // Change the Y axis column value
    cy.get('[name="yaxiscol"]').parent().click()
    cy.get('[name="yaxiscol"]').siblings('.react-dropdown-select-dropdown').find('.react-dropdown-select-item').contains('Turnout_pct').click({force: true})
    cy.get('[data-cy="savebtn"]').click()

    // Verify the change
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('test1').click()
    cy.get('[data-rr-ui-event-key="settings"]').click()
    cy.get('[name="yaxiscol"]').should('have.value', 'Turnout_pct')

    // Switch to a different Y axis column value
    cy.get('[name="yaxiscol"]').parent().click()
    cy.get('[name="yaxiscol"]').siblings('.react-dropdown-select-dropdown').find('.react-dropdown-select-item').contains('Constituency_Number').click({force: true})
    cy.get('[data-cy="savebtn"]').click()

    // Verify the change
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('test1').click()
    cy.get('[data-rr-ui-event-key="settings"]').click()
    cy.get('[name="yaxiscol"]').should('have.value', 'Constituency_Number')
  })

  // "Show result table" button works
  it('Shows results table button works', () => {
    // Start with the existing "test1" test
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('test1').click()

    // Verify the button starts closed
    cy.contains('button', 'Show data table').should('exist')

    // Open the results table
    cy.contains('button', 'Show data table').click()
    cy.contains('button', 'Hide data table').should('exist')
  })

  // "Download as CSV" button
  const downloadsFolder = Cypress.config('downloadsFolder')
  it('"Download as CSV" button', () => {
    // Start with the existing "test1" test
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('test1').click()

    // Click the download button
    cy.contains('button', 'Export to CSV').click()

    // Simple sanity check of the downloaded file
    // TODO - Implement a better check.   Maybe keep the "correct" csv in the repo as test data too, and compare against it?
    const csv = path.join(downloadsFolder, 'test1.csv')
    cy.readFile(csv, 'binary', { timeout: 5000 }).should('have.length', 188)
    cy.task('rmFile', { path: csv })
  })

  // "Format SQL" button
  it('"Format SQL" button', () => {
    // Start with the existing "test1" test
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('test1').click()

    // Click the format button
    cy.get('[data-rr-ui-event-key="sql"]').click()
    cy.get('[data-cy="formatsqlbtn"]').click()
    cy.wait(waitTime)

    // Verify the changed text
    cy.get('[name="usersql"]').should('have.value',
      'SELECT\n' +
        '  Constituency_Name,\n' +
        '  Constituency_Number,\n' +
        '  Turnout_pct\n' +
        'FROM\n' +
        '  Constituency_Turnout_Information\n' +
        'ORDER BY\n' +
        '  Constituency_Name DESC\n' +
        'LIMIT\n' +
        '  5')
  })

  // "Run SQL" button
  it('"Run SQL" button', () => {
    // Start with the existing "test1" test
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('test1').click()

    // Click the Run SQL button
    cy.get('[data-rr-ui-event-key="sql"]').click()
    cy.get('[data-cy="runsqlbtn"]').click()

    // Verify the result
    // TODO: Probably need to add cypress attributes to the rows and columns of the results table, then
    //       check against known good return values for the testing
  })

  // For a private database, verify only the owner and shared users can see it
  it('Verify private databases are private', () => {
    // Switch to a different user
    cy.request('/x/test/switchfirst')

    // Try accessing a private database's visualisation page
    cy.visit({url: '/vis/default/Assembly Election 2017 with view.sqlite', failOnStatusCode: false})
    cy.get('[data-cy="errormsg"').should('contain', 'doesn\'t exist')

    // Switch back to the default user
    cy.request('/x/test/switchdefault')

    // TODO: Add some shared users, and test with them
  })

  // For a public database, verify all users can see its visualisations
  it('Verify public databases are accessible by others', () => {
    // Switch the database to public
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="settingslink"]').click()
    cy.get('[data-cy="public"]').click()
    cy.get('[data-cy="savebtn"]').click()

    // Switch to a different user
    cy.request('/x/test/switchfirst')

    // Try accessing a private database's visualisation page
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    //cy.get('[data-cy="errormsg"').should('contain', 'doesn\'t seem to exist')

    // TODO: Test if the visualisations created by the default user can be loaded

    // Switch back to the default user
    cy.request('/x/test/switchdefault')
  })

  // Ensure the save and delete buttons are only shown to the database owner
  it('Ensure save/delete buttons are only shown to database owner', () => {
    // Switch to a different user
    cy.request('/x/test/switchfirst')

    // Load a public page
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')

    // Test if the save button is visible. It shouldn't be
    cy.get('[data-cy="newvisbtn"]').click()
    cy.get('[data-cy="savebtn"').should('not.exist')

    // Log out
    cy.request('/x/test/logout')

    // Reload the page
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')

    // Test if the save button is visible. It shouldn't be
    cy.get('[data-cy="newvisbtn"]').click()
    cy.get('[data-cy="savebtn"').should('not.exist')

    // Switch back to the default user
    cy.request('/x/test/switchdefault')

    // Reload the page again
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')

    // Ensure the save button is visible. It should be this time
    cy.get('[data-cy="newvisbtn"]').click()
    cy.get('[data-cy="savebtn"').should('exist')
  })

  // Ensure a logged-out user can still change visualisations
  it('Ensure a logged-out user can still change visualisations', () => {
    // Log out
    cy.request('/x/test/logout')

    // Load the database again
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')

    // Ensure the visualisation drop down works ok
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('test1').click()
    cy.get('[data-rr-ui-event-key="sql"]').click()
    cy.get('[name="usersql"]').should('contain',
      'SELECT Constituency_Name, Constituency_Number, Turnout_pct\n' +
      'FROM Constituency_Turnout_Information\n' +
      'ORDER BY Constituency_Name DESC\n' +
      'LIMIT 5')

    // Switch back to the default user
    cy.request('/x/test/switchdefault')
  })

  // "Delete" button
  it('Delete button', () => {
    // Start with the existing "test1" test
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="savedvis"]').get('.list-group-item').contains('test1').click()

    // Click the Delete button
    cy.get('[data-cy="deletebtn"]').click()

    // Confirm dialog
    cy.wait(200)
    cy.get('button[label="Yes"]').click()

    // Verify the result
    cy.wait(waitTime)
    cy.get('[data-cy="savedvis"]').get('.list-group-item').should('not.exist')
  })
})
