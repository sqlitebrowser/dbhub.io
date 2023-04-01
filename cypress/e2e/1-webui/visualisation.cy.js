import path from "path";

// Sometimes we need a delay between making a change, and testing it, otherwise the AngularJS changes are missed
let waitTime = 150;

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
  it('save a visualisation using non-default name', () => {
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="sqltab"]').click()
    cy.get('[data-cy="usersqltext"]').type(
      'SELECT Constituency_Name, Constituency_Number\n' +
      'FROM Constituency_Turnout_Information\n' +
      'ORDER BY Constituency_Name DESC\n' +
      'LIMIT 5')
    cy.get('[data-cy="nameinput"]').type('{selectall}{backspace}').type('test1')
    cy.get('[data-cy="savebtn"]').click()
    cy.get('[data-cy="statusmsg"]').should('contain.text', 'Visualisation \'test1\' saved')

    // Verify the visualisation - it should be automatically selected in the drop down list as it's the only one
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="selectedvis"]').should('contain.text', 'test1')
    cy.get('[data-cy="visdropdown"]').click()
    cy.get('[data-cy="vis-test1"]').should('contain.text', 'test1')
  })

  // Check if 'default' visualisation is still chosen by default even when created after non-default ones
  it('save a visualisation using default name', () => {
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="sqltab"]').click()
    cy.get('[data-cy="usersqltext"]').type('{selectall}{backspace}').type(
      'SELECT Constituency_Name, Constituency_Number\n' +
      'FROM Constituency_Turnout_Information\n' +
      'ORDER BY Constituency_Name ASC\n' +
      'LIMIT 5')
    cy.get('[data-cy="nameinput"]').type('{selectall}{backspace}').type('default')
    cy.get('[data-cy="savebtn"]').click()
    cy.get('[data-cy="statusmsg"]').should('contain.text', 'Visualisation \'default\' saved')

    // Verify the visualisation - it should be automatically selected in the drop down list as it's the default
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="selectedvis"]').should('contain.text', 'default')
    cy.get('[data-cy="visdropdown"]').click()
    cy.get('[data-cy="vis-default"]').should('contain.text', 'default')
    cy.get('[data-cy="usersqltext"]').should('contain.text',
      'SELECT Constituency_Name, Constituency_Number\n' +
      'FROM Constituency_Turnout_Information\n' +
      'ORDER BY Constituency_Name ASC\n' +
      'LIMIT 5')
  })

  // Save a visualisation
  it('save a visualisation with name alphabetically lower than \'default\'', () => {
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="sqltab"]').click()
    cy.get('[data-cy="usersqltext"]').type('{selectall}{backspace}').type(
      'SELECT Constituency_Name, Constituency_Number\n' +
      'FROM Constituency_Turnout_Information\n' +
      'ORDER BY Constituency_Name DESC\n' +
      'LIMIT 5')
    cy.get('[data-cy="nameinput"]').type('{selectall}{backspace}').type('abc')
    cy.get('[data-cy="savebtn"]').click()
    cy.get('[data-cy="statusmsg"]').should('contain.text', 'Visualisation \'abc\' saved')

    // Check that the visualisation is present, but not automatically selected when the page loads
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="selectedvis"]').should('not.contain.text', 'abc')
    cy.get('[data-cy="visdropdown"]').click()
    cy.get('[data-cy="vis-abc"]').should('contain.text', 'abc')
  })

  // Save over an existing visualisation
  it('save over an existing visualisation', () => {
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="sqltab"]').click()
    cy.get('[data-cy="usersqltext"]').type('{selectall}{backspace}').type(
      'SELECT Constituency_Name, Constituency_Number, Turnout_pct\n' +
      'FROM Constituency_Turnout_Information\n' +
      'ORDER BY Constituency_Name DESC\n' +
      'LIMIT 10')
    cy.get('[data-cy="nameinput"]').type('{selectall}{backspace}').type('test1')
    cy.get('[data-cy="savebtn"]').click()
    cy.get('[data-cy="statusmsg"]').should('contain.text', 'Visualisation \'test1\' saved')

    // Verify the new visualisation text
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="visdropdown"]').click()
    cy.get('[data-cy="vis-test1"]').click()
    cy.get('[data-cy="usersqltext"]').should('contain',
      'SELECT Constituency_Name, Constituency_Number, Turnout_pct\n' +
      'FROM Constituency_Turnout_Information\n' +
      'ORDER BY Constituency_Name DESC\n' +
      'LIMIT 10')
  })

  // * Chart settings tab *

  // Chart type drop down
  it('chart type drop down', () => {
    // Start with the existing "test1" test
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="visdropdown"]').click()
    cy.get('[data-cy="vis-test1"]').click()

    // Switch to the chart settings tab
    cy.get('[data-cy="charttab"]').click()

    // Change the chart type
    cy.get('[data-cy="chartdropdown"]').click()
    cy.get('[data-cy="chartpie"]').click()

    // Verify the change
    cy.wait(waitTime)
    cy.get('[data-cy="charttype"]').should('contain', 'Pie chart')
    cy.get('[data-cy="showxaxis"]').should('not.exist')

    // Switch to a different chart type
    cy.get('[data-cy="chartdropdown"]').click()
    cy.get('[data-cy="charthbc"]').click()

    // Verify the change
    cy.wait(waitTime)
    cy.get('[data-cy="charttype"]').should('contain', 'Horizontal bar chart')
    cy.get('[data-cy="showxaxis"]').should('exist')
  })

  // X axis column drop down
  it('X axis column drop down', () => {
    // Start with the existing "test1" test
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="visdropdown"]').click()
    cy.get('[data-cy="vis-test1"]').click()

    // Switch to the chart settings tab
    cy.get('[data-cy="charttab"]').click()

    // Change the X axis column value
    cy.get('[data-cy="xaxisdropdown"]').click()
    cy.get('[data-cy="xcol-Constituency_Number"]').click()

    // Verify the change
    cy.wait(waitTime)
    cy.get('[data-cy="xaxiscol"]').should('contain', 'Constituency_Number')
    cy.get('[data-cy="yaxiscol"]').should('contain', 'Constituency_Name')

    // Switch to a different X axis column value
    cy.get('[data-cy="xaxisdropdown"]').click()
    cy.get('[data-cy="xcol-Constituency_Name"]').click()

    // Verify the change
    cy.wait(waitTime)
    cy.get('[data-cy="xaxiscol"]').should('contain', 'Constituency_Name')
    cy.get('[data-cy="yaxiscol"]').should('contain', 'Constituency_Number')
  })

  // Y axis column drop down
  it('Y axis column drop down', () => {
    // Start with the existing "test1" test
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="visdropdown"]').click()
    cy.get('[data-cy="vis-test1"]').click()

    // Switch to the chart settings tab
    cy.get('[data-cy="charttab"]').click()

    // Change the Y axis column value
    cy.get('[data-cy="yaxisdropdown"]').click()
    cy.get('[data-cy="ycol-Turnout_pct"]').click()

    // Verify the change
    cy.wait(waitTime)
    cy.get('[data-cy="yaxiscol"]').should('contain', 'Turnout_pct')
    //cy.get('[data-cy="xaxiscol"]').should('contain', 'Constituency_Name')

    // Switch to a different Y axis column value
    cy.get('[data-cy="yaxisdropdown"]').click()
    cy.get('[data-cy="ycol-Constituency_Number"]').click()

    // Verify the change
    cy.wait(waitTime)
    cy.get('[data-cy="yaxiscol"]').should('contain', 'Constituency_Number')
  })

  // "Show result table" button works
  it('Shows results table button works', () => {
    // Start with the existing "test1" test
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="visdropdown"]').click()
    cy.get('[data-cy="vis-test1"]').click()

    // Verify the button starts closed
    cy.get('[data-cy="resultsbtn"]').should('contain', 'Show result table')

    // Open the results table
    cy.get('[data-cy="resultsbtn"]').click()
    cy.get('[data-cy="resultsbtn"]').should('contain', 'Hide result table')
  })

  // "Download as CSV" button
  const downloadsFolder = Cypress.config('downloadsFolder')
  it('"Download as CSV" button', () => {
    // Start with the existing "test1" test
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="visdropdown"]').click()
    cy.get('[data-cy="vis-test1"]').click()

    // Click the download button
    cy.get('[data-cy="downcsvbtn"]').click()

    // Simple sanity check of the downloaded file
    // TODO - Implement a better check.   Maybe keep the "correct" csv in the repo as test data too, and compare against it?
    const csv = path.join(downloadsFolder, 'results.csv')
    cy.readFile(csv, 'binary', { timeout: 5000 }).should('have.length', 213)
    cy.task('rmFile', { path: csv })
  })

  // "Format SQL" button
  it('"Format SQL" button', () => {
    // Start with the existing "test1" test
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="visdropdown"]').click()
    cy.get('[data-cy="vis-test1"]').click()

    // Click the format button
    cy.get('[data-cy="formatsqlbtn"]').click()

    // Verify the changed text
    cy.get('[data-cy="usersqltext"]').should('contain',
      'SELECT Constituency_Name,\n' +
      '       Constituency_Number,\n' +
      '       Turnout_pct\n' +
      'FROM Constituency_Turnout_Information\n' +
      'ORDER BY Constituency_Name DESC\n' +
      'LIMIT 10')
  })

  // "Run SQL" button
  it('"Run SQL" button', () => {
    // Start with the existing "test1" test
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="visdropdown"]').click()
    cy.get('[data-cy="vis-test1"]').click()

    // Click the Run SQL button
    cy.get('[data-cy="runsqlbtn"]').click()

    // Verify the result
    // TODO: Probably need to add cypress to the rows and columns of the results table, then
    //       check against known good return values
  })

  // "Delete" button
  it('Delete button', () => {
    // Start with the existing "test1" test
    cy.visit('/vis/default/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="visdropdown"]').click()
    cy.get('[data-cy="vis-abc"]').click()

    // Click the Delete button
    cy.get('[data-cy="delvisbtn"]').click()

    // Verify the result
    cy.wait(waitTime)
    cy.get('[data-cy="visdropdown"]').click()
    cy.get('[data-cy="vis-abc"]').should('not.exist')
  })

  // For a private database, verify only the owner and shared users can see it
  it('Verify private databases are private', () => {
    // Switch to a different user
    cy.request('/x/test/switchfirst')

    // Try accessing a private database's visualisation page
    cy.visit({url: '/vis/default/Assembly Election 2017 with view.sqlite', failOnStatusCode: false})
    cy.get('[data-cy="errormsg"').should('contain', 'doesn\'t seem to exist')

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
})