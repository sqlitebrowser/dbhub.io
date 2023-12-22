const path = require('path')

describe('logged-in user preferences page', () => {
  before(() => {
    // Seed data
    cy.request('/x/test/seed')
  })

  // Automatically visit the logged-in user's preferences page for each test
  beforeEach(() => {
    cy.visit('pref')
  })

  // Change user full name
  it('change user full name', () => {
    cy.get('[data-cy="fullname"]').should('have.attr', 'value', 'Default system user')
    cy.get('[data-cy="fullname"]').type('{selectall}{backspace}').type('Some User').should('have.value', 'Some User')
    cy.get('[data-cy="updatebtn"]').click()
    cy.visit('pref')
    cy.get('[data-cy="fullname"]').should('have.attr', 'value', 'Some User')
  })

  // Change user email address
  it('change user email address', () => {
    cy.get('[data-cy="email"]').should('have.attr', 'value', 'default@docker-dev.dbhub.io')
    cy.get('[data-cy="email"]').type('{selectall}{backspace}').type('test@example.org').should('have.value', 'test@example.org')
    cy.get('[data-cy="updatebtn"]').click()
    cy.visit('pref')
    cy.get('[data-cy="email"]').should('have.attr', 'value', 'test@example.org')
  })

  // Change maximum # of database rows to display
  it('change maximum # of database rows to display', () => {
    cy.get('[data-cy="numrows"]').should('have.attr', 'value', '10')
    cy.get('[data-cy="numrows"]').type('{selectall}{backspace}').type('25').should('have.value', '25')
    cy.get('[data-cy="updatebtn"]').click()
    cy.visit('pref')
    cy.get('[data-cy="numrows"]').should('have.attr', 'value', '25')
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
    cy.readFile(cert, { timeout: 10000 }).should('have.length.gt', 512)
    cy.task('rmFile', { path: cert })
  })

  // Generate new API key
  it('generate new API key', () => {
    cy.get('[data-cy="genapibtn"]').click()
    cy.get('[data-cy="apistatus"]').should('contain', 'created')
  })
})
