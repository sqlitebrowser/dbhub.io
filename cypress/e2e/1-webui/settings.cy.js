let waitTime = 250;

const resizeObserverLoopErrRe = /^[^(ResizeObserver loop limit exceeded)]/
Cypress.on('uncaught:exception', (err) => {
  /* returning false here prevents Cypress from failing the test */
  if (resizeObserverLoopErrRe.test(err.message)) {
    return false
  }
})

describe('settings', () => {
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

   // Rename database
   it('name', () => {
     cy.visit('settings/default/Assembly%20Election%202017.sqlite')
     cy.get('[data-cy="nameinput"]').should('have.value', 'Assembly Election 2017.sqlite')
     cy.get('[data-cy="nameinput"]').type('{selectall}{backspace}New database name')
     cy.get('[data-cy="savebtn"]').click()
     cy.location('pathname').should('equal', '/default/New%20database%20name')
     cy.get('[data-cy="settingslink"]').click()
     cy.get('[data-cy="nameinput"]').type('{selectall}{backspace}Assembly Election 2017.sqlite')
     cy.get('[data-cy="savebtn"]').click()
     cy.location('pathname').should('equal', '/default/Assembly%20Election%202017.sqlite')
   })

   // One line description
   it('one line description', () => {
     cy.visit('settings/default/Assembly%20Election%202017.sqlite')
     cy.get('[data-cy="onelinedescinput"]').should('have.value', '')
     cy.get('[data-cy="onelinedescinput"]').type('{selectall}{backspace}Some new description')
     cy.get('[data-cy="savebtn"]').click()
     cy.get('[data-cy="settingslink"]').click()
     cy.get('[data-cy="onelinedescinput"]').should('have.value', 'Some new description')
   })

   // Public/private toggle
   it('public/private', () => {
     cy.visit('settings/default/Assembly%20Election%202017.sqlite')
     cy.get('[data-cy="private"]').click()
     cy.get('[data-cy="savebtn"]').click()
     cy.get('[data-cy="vis"]').should('contain', 'Private')
     cy.get('[data-cy="settingslink"]').click()
     cy.get('[data-cy="public"]').click()
     cy.get('[data-cy="savebtn"]').click()
     cy.get('[data-cy="vis"]').should('contain', 'Public')
   })

   // Default table or view
   // Note - The default table or view is set "per branch", so when changing the default branch (below) this will
   //        appear to revert to the default setting.  Not sure if this is gotcha we should change or not (?)
   it('default table or view', () => {
     cy.visit('settings/default/Assembly%20Election%202017.sqlite')
     cy.get('[name="selectdefaulttable"]').should('have.value', 'Candidate_Information')
     cy.get('[name="selectdefaulttable"]').parents('.react-dropdown-select').click()
     cy.get('[name="selectdefaulttable"]').parents('.react-dropdown-select').find('.react-dropdown-select-item').contains('Constituency_Turnout_Information').click({force: true})
     cy.get('[data-cy="savebtn"]').click()
     cy.get('[data-cy="settingslink"]').click()
     cy.get('[name="selectdefaulttable"]').should('have.value', 'Constituency_Turnout_Information')
   })

   // Default branch
   it('default branch', () => {
     cy.visit('settings/default/Assembly%20Election%202017.sqlite')
     cy.get('[name="selectbranch"]').should('have.value', 'main')
     cy.get('[name="selectbranch"]').parents('.react-dropdown-select').click()
     cy.get('[name="selectbranch"]').parents('.react-dropdown-select').find('.react-dropdown-select-item').contains('stuff').click({force: true})
     cy.get('[data-cy="savebtn"]').click()
     cy.get('[data-cy="settingslink"]').click()
     cy.get('[name="selectbranch"]').should('have.value', 'stuff')
   })

   // Source URL
   it('source url', () => {
     cy.visit('settings/default/Assembly%20Election%202017.sqlite')
     cy.get('[data-cy="sourceurl"]').should('have.value', 'http://data.nicva.org/dataset/assembly-election-2017')
     cy.get('[data-cy="sourceurl"]').type('{selectall}{backspace}https://example.org')
     cy.get('[data-cy="savebtn"]').click()
     cy.get('[data-cy="settingslink"]').click()
     cy.get('[data-cy="sourceurl"]').should('have.value', 'https://example.org')
     cy.get('[data-cy="sourceurl"]').type('{selectall}{backspace}http://data.nicva.org/dataset/assembly-election-2017')
     cy.get('[data-cy="savebtn"]').click()
   })

   // Licence
   it('licence', () => {
     // Test the main branch
     cy.visit('settings/default/Assembly%20Election%202017.sqlite')
     cy.get('[name="main-licence"]').should('have.value', 'CC-BY-SA-4.0')
     cy.get('[name="main-licence"]').parents('.react-dropdown-select').click()
     cy.get('[name="main-licence"]').parents('.react-dropdown-select').find('.react-dropdown-select-dropdown').find('span').contains('Not specified').click({force: true})
     cy.get('[data-cy="savebtn"]').click()
     cy.get('[data-cy="settingslink"]').click()
     cy.get('[name="main-licence"]').should('have.value', 'Not specified')

     // Test the 2nd branch
     cy.get('[name="stuff-licence"]').should('have.value', 'CC-BY-SA-4.0')
     cy.get('[name="stuff-licence"]').parents('.react-dropdown-select').click()
     // Note - scrollIntoView() seems like it should work instead of forcing this click on Firefox.  But it doesn't.
     cy.get('[name="stuff-licence"]').parents('.react-dropdown-select').find('.react-dropdown-select-dropdown').find('span').contains('CC0').click({force: true}) // Firefox sizing seems to have this *slightly* clipped, but otherwise usable
     cy.get('[data-cy="savebtn"]').click()
     cy.get('[data-cy="settingslink"]').click()
     cy.get('[name="stuff-licence"]').should('have.value', 'CC0')
   })

  // Share Database
  it('share database', () => {
    cy.visit('settings/default/Assembly%20Election%202017.sqlite')
    // TODO - Ensure the user CAN NOT add themselves to this list (seems to be currently possible)

    // Set the database to Private
    cy.get('[data-cy="private"]').click()
    cy.get('[data-cy="savebtn"]').click()
    cy.wait(waitTime)

    // Ensure the database cannot be seen by the other users
    cy.request("/x/test/switchfirst")
    cy.visit({url: 'default/Assembly%20Election%202017.sqlite', failOnStatusCode: false})
    cy.get('[data-cy="errormsg"').should('contain.text', 'Database \'default/Assembly Election 2017.sqlite\' doesn\'t exist')
    cy.request("/x/test/switchsecond")
    cy.visit({url: 'default/Assembly%20Election%202017.sqlite', failOnStatusCode: false})
    cy.get('[data-cy="errormsg"').should('contain.text', 'Database \'default/Assembly Election 2017.sqlite\' doesn\'t exist')
    cy.request("/x/test/switchthird")
    cy.visit({url: 'default/Assembly%20Election%202017.sqlite', failOnStatusCode: false})
    cy.get('[data-cy="errormsg"').should('contain.text', 'Database \'default/Assembly Election 2017.sqlite\' doesn\'t exist')

    // Add users to the share list
    cy.request("/x/test/switchdefault")
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="settingslink"]').click()
    cy.get('[data-cy="usernameinput"]').type('first')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.wait(waitTime)

    cy.get('[name="shareperm-first"]').parents('.react-dropdown-select').click()
    cy.get('[name="shareperm-first"]').parents('.react-dropdown-select').find('.react-dropdown-select-item').contains('Read and write').click({force: true})

    cy.get('[data-cy="usernameinput"]').type('second')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.wait(waitTime)

    cy.get('[data-cy="usernameinput"]').type('third')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.wait(waitTime)

    cy.get('[name="shareperm-third"]').parents('.react-dropdown-select').click()
    cy.get('[name="shareperm-third"]').parents('.react-dropdown-select').find('.react-dropdown-select-item').contains('Read and write').click({force: true})

    cy.get('[data-cy="savebtn"]').click()
    cy.get('[data-cy="settingslink"]').click()

    cy.get('[data-cy="shareuser-first"]').should('contain.text', 'first')
    cy.get('[name="shareperm-first"]').should('have.value', 'Read and write')

    cy.get('[data-cy="shareuser-second"]').should('contain.text', 'second')
    cy.get('[name="shareperm-second"]').should('have.value', 'Read only')

    cy.get('[data-cy="shareuser-third"]').should('contain.text', 'third')
    cy.get('[name="shareperm-third"]').should('have.value', 'Read and write')

    // Switch to the different users and verify they have read access to the database
    cy.request("/x/test/switchfirst")
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="srcurl"]').should('contain', 'http://data.nicva.org/dataset/assembly-election-2017')
    cy.request("/x/test/switchsecond")
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="srcurl"]').should('contain', 'http://data.nicva.org/dataset/assembly-election-2017')
    cy.request("/x/test/switchthird")
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="srcurl"]').should('contain', 'http://data.nicva.org/dataset/assembly-election-2017')

    // Upload a database
    cy.visit('upload')
    cy.get('input[type=file]').selectFile('cypress/test_data/Assembly Election 2017.sqlite')
    cy.get('[data-cy="uploadbtn"]').click()

    cy.request("/x/test/switchdefault")
  })

   // Full description
   it('full description', () => {
     cy.visit('settings/default/Assembly%20Election%202017.sqlite')
     cy.get('[data-cy="fulldesc-preview"]').should('contain.text', 'No full description')
     cy.get('[data-rr-ui-event-key="fulldesc-edit-tab"]').click()
     cy.get('[data-cy="fulldesc"]').should('contain.text', 'No full description')
     cy.get('[data-cy="fulldesc"]').type('{selectall}{backspace}Some new description')
     cy.get('[data-rr-ui-event-key="fulldesc-preview-tab"]').click()
     cy.get('[data-cy="fulldesc-preview"]').should('contain.text', 'Some new description')
     cy.get('[data-cy="savebtn"]').click()
     cy.get('[data-cy="settingslink"]').click()
     cy.get('[data-cy="fulldesc-preview"]').should('contain.text', 'Some new description')
   })

   // Delete database
   it('delete database', () => {
     cy.visit('settings/default/Assembly%20Election%202017.sqlite')
     cy.get('[data-cy="delbtn"]').click()
     cy.wait(waitTime)	// Some animation played here
     cy.get('button[label="Yes, delete it"]').click()
     cy.get('[data-cy="pubdbs"]').should('not.contain', 'Assembly Election 2017.sqlite')
     cy.get('[data-cy="privdbs"]').should('not.contain', 'Assembly Election 2017.sqlite')
   })
})
