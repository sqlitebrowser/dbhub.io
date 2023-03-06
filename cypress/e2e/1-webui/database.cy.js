import path from "path";

describe('database page', () => {
  before(() => {
    // Seed data
    cy.request('/x/test/seed')

    // Create a new branch for testing with
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="commitslnk"]').click()
    cy.get('[data-cy="createbranchbtn"]').click()
    cy.get('[data-cy="nameinput"]').type('{selectall}{backspace}stuff')
    cy.get('[data-cy="createbtn"]').click()
  })

  // For each test, automatically visit the test database page
  beforeEach(() => {
    cy.visit('default/Assembly%20Election%202017.sqlite')
  })

  // Watch count
  it('Watch count', () => {
    cy.get('[data-cy="watcherspagebtn"]').should('contain', '1')
    cy.get('[data-cy="watcherstogglebtn"]').click()
    cy.get('[data-cy="watcherspagebtn"]').should('contain', '0')
    cy.get('[data-cy="watcherstogglebtn"]').click()
    cy.get('[data-cy="watcherspagebtn"]').should('contain', '1')
  })

  // Star count
  it('Star count', () => {
    cy.get('[data-cy="starspagebtn"]').should('contain', '0')
    cy.get('[data-cy="starstogglebtn"]').click()
    cy.get('[data-cy="starspagebtn"]').should('contain', '1')
    cy.get('[data-cy="starstogglebtn"]').click()
    cy.get('[data-cy="starspagebtn"]').should('contain', '0')
  })

  // Fork count
  it('Fork count', () => {
    // Switch to a different user
    cy.request('/x/test/switchfirst')

    // Fork the database
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="forksbtn"]').click()
    cy.location('pathname').should('equal', '/first/Assembly%20Election%202017.sqlite')

    // Switch back to the default user
    cy.request('/x/test/switchdefault')

    // Ensure the fork count shows the new fork
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="forkspagebtn"]').should('contain', '1')
  })

  // Change tabs
  it('Change tabs', () => {
    cy.get('[data-cy="vislink"]').click()
    cy.location('pathname').should('equal', '/vis/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="discusslink"]').click()
    cy.location('pathname').should('equal', '/discuss/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="mrlink"]').click()
    cy.location('pathname').should('equal', '/merge/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="settingslink"]').click()
    cy.location('pathname').should('equal', '/settings/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="datalink"]').click()
    cy.location('pathname').should('equal', '/default/Assembly%20Election%202017.sqlite')
  })

  // Visibility setting
  it('Visibility setting', () => {
    cy.get('[data-cy="vis"]').should('contain', 'Public')
    cy.get('[data-cy="vis"]').click()
    cy.get('[data-cy="private"]').click()
    cy.get('[data-cy="savebtn"]').click()
    cy.get('[data-cy="vis"]').should('contain', 'Private')
    cy.get('[data-cy="vis"]').click()
    cy.get('[data-cy="public"]').click()
    cy.get('[data-cy="savebtn"]').click()
    cy.get('[data-cy="vis"]').should('contain', 'Public')
  })

  // Licence field
  it('License field', () => {
    cy.get('[data-cy="lic"]').should('contain', 'CC-BY-SA-4.0')
  })

  // Size field
  it('Size field', () => {
    cy.get('[data-cy="size"]').should('contain', '72 KB')
  })

  // Source URL
  it('Source URL', () => {
    cy.get('[data-cy="srcurl"]').should('contain', 'http://data.nicva.org/dataset/assembly-election-2017')
  })

  // Commits
  it('Commits count', () => {
    cy.get('[data-cy="commitscnt"]').should('contain', '1')
  })

  // Branches
  it('Branches count', () => {
    cy.get('[data-cy="branchescnt"]').should('contain', '2')
  })

  // Tags
  it('Tags count', () => {
    cy.get('[data-cy="tagscnt"]').should('contain', '0')
  })

  // Releases
  it('Releases count', () => {
    cy.get('[data-cy="rlscnt"]').should('contain', '0')
  })

  // Contributors
  it('Contributors', () => {
    cy.get('[data-cy="contcnt"]').should('contain', '1')
  })

  // Table/view dropdown
  it('Table/view dropdown', () => {
    cy.get('[data-cy="tabledropdown"]').click()
    cy.get('[data-cy="row-Constituency_Turnout_Information"]').click()
    cy.get('[data-cy="col-Constituency_Name"]').should('contain', 'Constituency_Name')
  })

  // Branch dropdown
  it('Branch dropdown', () => {
    cy.get('[data-cy="branchname"]').should('contain.text', 'main')
    cy.get('[data-cy="branchdropdown"]').click()
    cy.get('[data-cy="branch-stuff"]').click()
    cy.get('[data-cy="branchname"]').should('contain.text', 'stuff')
  })

  // New Merge Request button
  it('New Merge Request button', () => {
    cy.get('[data-cy="newmrbtn"]').click()
    cy.location('pathname').should('equal', '/compare/default/Assembly%20Election%202017.sqlite')
  })

  // Download database button (entire database)
  const downloadsFolder = Cypress.config('downloadsFolder')
  it('Download database - entire database', () => {
    // Open the drop down, unhiding the element we want to trigger
    cy.get('[data-cy="dldropdown"]').click()

    // Setup the dodgy page reload to workaround Cypress issue #14857
    cy.window().document().then(function (doc) {
      // Wait 2 seconds, then trigger a page reload.  This page reload provides the "page load" event which Cypress
      // needs in order to continue after the download has finished
      doc.addEventListener('click', () => {
        setTimeout(function () { doc.location.reload() }, 2000)
      })

      // Ensure the server responds with an acceptable response code (eg 200)
      cy.intercept('/x/download/', (req) => {
        req.reply((res) => {
          expect(res.statusCode).to.equal(200);
        });
      });

      // Trigger the download
      cy.get('[data-cy="dldb"]').click()
    })

    // Simple sanity check of the downloaded file
    // TODO - Implement a better check.   Maybe create a task that diffs the database to the original test data file?
    const db = path.join(downloadsFolder, 'Assembly Election 2017.sqlite')
    cy.readFile(db, 'binary', { timeout: 5000 }).should('have.length', 73728)
    cy.task('rmFile', { path: db })
  })

  // Download database button (selected table as csv)
  it('Download database - selected table as csv', () => {
    // Open the drop down, unhiding the element we want to trigger
    cy.get('[data-cy="dldropdown"]').click()

    // Setup the dodgy page reload to workaround Cypress issue #14857
    cy.window().document().then(function (doc) {
      // Wait 2 seconds, then trigger a page reload.  This page reload provides the "page load" event which Cypress
      // needs in order to continue after the download has finished
      doc.addEventListener('click', () => {
        setTimeout(function () { doc.location.reload() }, 2000)
      })

      // Ensure the server responds with an acceptable response code (eg 200)
      cy.intercept('/x/downloadcsv/', (req) => {
        req.reply((res) => {
          expect(res.statusCode).to.equal(200);
        });
      });

      // Trigger the download
      cy.get('[data-cy="dlcsv"]').click()
    })

    // Simple sanity check of the downloaded file
    // TODO - Implement a better check.   Maybe keep the "correct" csv in the repo as test data too, and compare against it?
    const csv = path.join(downloadsFolder, 'Candidate_Information.csv')
    cy.readFile(csv, 'binary', { timeout: 5000 }).should('have.length', 30773)
    cy.task('rmFile', { path: csv })
  })

  // Click on column header (sort ascending)
  it('Click on column header - 1st time', () => {
    cy.get('[data-cy="col-Candidate_First_Pref_Votes"]').click()
    cy.get('[data-cy="datarow-Candidate_First_Pref_Votes"]').should('contain', '27')
  })

  // Click on column header twice (sort descending)
  it('Click on column header - 2nd time', () => {
    cy.get('[data-cy="col-Candidate_First_Pref_Votes"]').click()
    cy.get('[data-cy="col-Candidate_First_Pref_Votes"]').click()
    cy.get('[data-cy="datarow-Candidate_First_Pref_Votes"]').should('contain', '10258')
  })

  // Click on page down button
  it('Page down button', () => {
    // Initial sort to guarantee a stable order
    cy.get('[data-cy="col-Candidate_First_Pref_Votes"]').click()

    // Click the button we're testing
    cy.get('[data-cy="pgdnbtn"]').click()
    cy.get('[data-cy="datarow-Candidate_First_Pref_Votes"]').should('contain', '85')
  })

  // Click on "go to the last page" button
  it('Last page button', () => {
    // Initial sort to guarantee a stable order
    cy.get('[data-cy="col-Candidate_First_Pref_Votes"]').click()

    // Click the button we're testing
    cy.get('[data-cy="lastpgbtn"]').click()
    cy.get('[data-cy="datarow-Candidate_First_Pref_Votes"]').should('contain', '8881')
  })

  // Click on page up button
  it('Page up button', () => {
    // Initial sort to guarantee a stable order
    cy.get('[data-cy="col-Candidate_First_Pref_Votes"]').click()

    // Click the button we're testing
    cy.get('[data-cy="lastpgbtn"]').click()
    cy.get('[data-cy="pgupbtn"]').click()
    cy.get('[data-cy="datarow-Candidate_First_Pref_Votes"]').should('contain', '7786')
  })

  // Click on "go to the first page" button
  it('First page button', () => {
    // Initial sort to guarantee a stable order
    cy.get('[data-cy="col-Candidate_First_Pref_Votes"]').click()

    // Click the button we're testing
    cy.get('[data-cy="lastpgbtn"]').click()
    cy.get('[data-cy="firstpgbtn"]').click()
    cy.get('[data-cy="datarow-Candidate_First_Pref_Votes"]').should('contain', '27')
  })

  // Readme contents
  it('Repo description', () => {
    cy.get('[data-cy="repodescrip"]').should('contain', 'No full description')
  })
})
