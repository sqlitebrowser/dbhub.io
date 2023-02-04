import path from "path";

describe('logged-in user profile page', () => {
  before(() => {
    // Seed data
    cy.request('/x/test/seed')
  })

  // Automatically visit the logged-in user's profile page for each test
  beforeEach(() => {
    cy.visit('default')
  })

  // Confirm the test database is present
  it('assembly election 2017 database is present', () => {
    cy.get('[data-cy="pubdbs"]').should('contain', 'Assembly Election 2017.sqlite')
  })

  // Confirm test database has the expected details
  it('assembly election 2017 database has expected details', () => {
    cy.get('[data-cy="pubexpand"]').click()
    cy.get('[data-cy="pubdbs"]').contains('Source:').next().should('contain', 'http://data.nicva.org/dataset/assembly-election-2017')
    cy.get('[data-cy="pubdbs"]').contains('Size:').next().should('contain', '56 KB')
    cy.get('[data-cy="pubdbs"]').contains('Contributors:').next().should('contain', '1')
    cy.get('[data-cy="pubdbs"]').contains('Discussions:').next().should('contain', '0')
    cy.get('[data-cy="pubdbs"]').contains('Licence:').next().should('contain', 'CC-BY-SA-4.0')
    cy.get('[data-cy="pubdbs"]').contains('Watchers:').next().should('contain', '1')
    cy.get('[data-cy="pubdbs"]').contains('Stars:').next().should('contain', '0')
    cy.get('[data-cy="pubdbs"]').contains('Forks:').next().should('contain', '0')
    cy.get('[data-cy="pubdbs"]').contains('MRs:').next().should('contain', '0')
    cy.get('[data-cy="pubdbs"]').contains('Branches:').next().should('contain', '1')
    cy.get('[data-cy="pubdbs"]').contains('Releases:').next().should('contain', '0')
    cy.get('[data-cy="pubdbs"]').contains('Tags:').next().should('contain', '0')
    cy.get('[data-cy="pubdbs"]').contains('Downloads:').next().should('contain', '0')
  })

  // Switch the test database to be private
  it ('switch test database to be private', () => {
    cy.visit('default/Assembly Election 2017.sqlite')
    cy.get('#settings').click()
    cy.get('[data-cy="private"]').click()
    cy.get('[data-cy="savebtn"]').click()
  })

  // Confirm the test database is present in the private database list
  it('assembly election 2017 database is present in private databases', () => {
    cy.get('[data-cy="privdbs"]').should('contain', 'Assembly Election 2017.sqlite')
  })

  // Confirm the test database is no longer present in the public databases list
  if ('test database no longer in public db list', () => {
    cy.get('[data-cy="pubdbs"]').should('not.contain', 'Assembly Election 2017.sqlite')
  })

  // Confirm the details for the test database are now showing up correctly in the private list
  it('private database has expected details', () => {
    cy.get('[data-cy="privexpand"]').click()
    cy.get('[data-cy="privdbs"]').contains('Source:').next().should('contain', 'http://data.nicva.org/dataset/assembly-election-2017')
    cy.get('[data-cy="privdbs"]').contains('Size:').next().should('contain', '56 KB')
    cy.get('[data-cy="privdbs"]').contains('Contributors:').next().should('contain', '1')
    cy.get('[data-cy="privdbs"]').contains('Discussions:').next().should('contain', '0')
    cy.get('[data-cy="privdbs"]').contains('Licence:').next().should('contain', 'CC-BY-SA-4.0')
    cy.get('[data-cy="privdbs"]').contains('Watchers:').next().should('contain', '1')
    cy.get('[data-cy="privdbs"]').contains('Stars:').next().should('contain', '0')
    cy.get('[data-cy="privdbs"]').contains('Forks:').next().should('contain', '0')
    cy.get('[data-cy="privdbs"]').contains('MRs:').next().should('contain', '0')
    cy.get('[data-cy="privdbs"]').contains('Branches:').next().should('contain', '1')
    cy.get('[data-cy="privdbs"]').contains('Releases:').next().should('contain', '0')
    cy.get('[data-cy="privdbs"]').contains('Tags:').next().should('contain', '0')
    cy.get('[data-cy="privdbs"]').contains('Downloads:').next().should('contain', '0')
  })

  // Switch the test database back to public
  it ('switch test database back to being public', () => {
    cy.visit('default/Assembly Election 2017.sqlite')
    cy.get('#settings').click()
    cy.get('[data-cy="public"]').click()
    cy.get('[data-cy="savebtn"]').click()
    cy.visit('default')
    cy.get('[data-cy="pubdbs"]').should('contain', 'Assembly Election 2017.sqlite')
  })

  // Star the database
  it ('starring a database works', () => {
    cy.get('[data-cy="stars"]').should('not.contain', 'Assembly Election 2017.sqlite')
    cy.visit('default/Assembly Election 2017.sqlite')
    cy.get('[data-cy="starstogglebtn"]').click()
    cy.visit('default')
    cy.get('[data-cy="stars"]').should('contain', 'Assembly Election 2017.sqlite')
  })

  // Unstar the database
  it ('unstarring a database works', () => {
    cy.get('[data-cy="stars"]').should('contain', 'Assembly Election 2017.sqlite')
    cy.visit('default/Assembly Election 2017.sqlite')
    cy.get('[data-cy="starstogglebtn"]').click()
    cy.visit('default')
    cy.get('[data-cy="stars"]').should('not.contain', 'Assembly Election 2017.sqlite')
  })

  // Unwatch the database
  it ('unwatching a database works', () => {
    cy.get('[data-cy="watches"]').should('contain', 'Assembly Election 2017.sqlite')
    cy.visit('default/Assembly Election 2017.sqlite')
    cy.get('[data-cy="watcherstogglebtn"]').click()
    cy.visit('default')
    cy.get('[data-cy="watches"]').should('not.contain', 'Assembly Election 2017.sqlite')
  })

  // Watch the database
  it ('watching a database works', () => {
    cy.get('[data-cy="watches"]').should('not.contain', 'Assembly Election 2017.sqlite')
    cy.visit('default/Assembly Election 2017.sqlite')
    cy.get('[data-cy="watcherstogglebtn"]').click()
    cy.visit('default')
    cy.get('[data-cy="watches"]').should('contain', 'Assembly Election 2017.sqlite')
  })

  // Generate a new client certificate
  const downloadsFolder = Cypress.config('downloadsFolder')
  it('generate a new client certificate', () => {
    // Ugh.  Cypress doesn't properly handle downloads which don't generate a new page load event afterwards. :(
    // There are several issues for this problem reported in their repo, but no good solution.  Fortunately, some
    // people have created workarounds that get it working "enough" to be usable.
    // Workaround code: https://github.com/cypress-io/cypress/issues/14857#issuecomment-790765826
    cy.window().document().then(function (doc) {
      // Wait 2 seconds, then trigger a page reload.  This page reload provides the "page load" event which Cypress
      // needs in order to continue after the download has finished
      doc.addEventListener('click', () => {
        setTimeout(function () { doc.location.reload() }, 2000)
      })

      // Ensure the server responds with an acceptable response code (eg 200)
      cy.intercept('/x/gencert', (req) => {
        req.reply((res) => {
          expect(res.statusCode).to.equal(200);
        });
      });

      // Trigger the certificate generation and download event
      cy.get('[data-cy="gencertbtn"]').click()
    })

    // Simple sanity check of the downloaded cert
    const cert = path.join(downloadsFolder, 'default.cert.pem')
    cy.readFile(cert, { timeout: 5000 }).should('have.length.gt', 512)
    cy.task('rmFile', { path: cert })
  })
})