let waitTime = 250;

const resizeObserverLoopErrRe = /^[^(ResizeObserver loop limit exceeded)]/
Cypress.on('uncaught:exception', (err) => {
  /* returning false here prevents Cypress from failing the test */
  if (resizeObserverLoopErrRe.test(err.message)) {
    return false
  }
})

describe('database sharing', () => {
  before(() => {
    // Seed data
    cy.request('/x/test/seed')

    // Setup some shares for this public database
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="settingslink"]').click()
    cy.get('[data-cy="usernameinput"]').type('second')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.get('input[name="shareperm-second"]').parents('.react-dropdown-select').click()
    cy.get('input[name="shareperm-second"]').parents('.react-dropdown-select').find('.react-dropdown-select-item').contains('Read only').click({force: true})

    cy.get('[data-cy="usernameinput"]').type('third')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.get('input[name="shareperm-third"]').parents('.react-dropdown-select').click()
    cy.get('input[name="shareperm-third"]').parents('.react-dropdown-select').find('.react-dropdown-select-item').contains('Read and write').click({force: true})
    cy.get('[data-cy="savebtn"]').click()
    cy.wait(waitTime)

    // Set up some shares for the live database too
    cy.visit('settings/default/Join Testing with index.sqlite')
    cy.get('[data-cy="settingslink"]').click()
    cy.get('[data-cy="usernameinput"]').type('second')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.get('input[name="shareperm-second"]').parents('.react-dropdown-select').click()
    cy.get('input[name="shareperm-second"]').parents('.react-dropdown-select').find('.react-dropdown-select-item').contains('Read only').click({force: true})

    cy.get('[data-cy="usernameinput"]').type('third')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.get('input[name="shareperm-third"]').parents('.react-dropdown-select').click()
    cy.get('input[name="shareperm-third"]').parents('.react-dropdown-select').find('.react-dropdown-select-item').contains('Read and write').click({force: true})
    cy.get('[data-cy="savebtn"]').click()
    cy.wait(waitTime)

    // Upload a standard database to the user "first", plus setup some useful database sharing
    cy.request("/x/test/switchfirst")
    cy.visit('upload')
    cy.get('input[type=file]').selectFile('cypress/test_data/Assembly Election 2017.sqlite')
    cy.get('[data-cy="uploadbtn"]').click()
    cy.get('[data-cy="settingslink"]').click()
    cy.get('[data-cy="private"]').click()

    cy.get('[data-cy="usernameinput"]').type('second')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.get('input[name="shareperm-second"]').parents('.react-dropdown-select').click()
    cy.get('input[name="shareperm-second"]').parents('.react-dropdown-select').find('.react-dropdown-select-item').contains('Read and write').click({force: true})

    cy.get('[data-cy="usernameinput"]').type('third')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.get('input[name="shareperm-third"]').parents('.react-dropdown-select').click()
    cy.get('input[name="shareperm-third"]').parents('.react-dropdown-select').find('.react-dropdown-select-item').contains('Read only').click({force: true})
    cy.get('[data-cy="savebtn"]').click()
    cy.wait(waitTime)

    // Upload a live database to the user "first", plus setup some useful database sharing
    cy.visit('upload')
    cy.get('input[type=file]').selectFile('cypress/test_data/Join Testing with index.sqlite')
    cy.get('[data-cy="livebtn"]').click()
    cy.get('[data-cy="uploadbtn"]').click()
    cy.get('[data-cy="settingslink"]').click()

    cy.get('[data-cy="usernameinput"]').type('default')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.get('input[name="shareperm-default"]').parents('.react-dropdown-select').click()
    cy.get('input[name="shareperm-default"]').parents('.react-dropdown-select').find('.react-dropdown-select-item').contains('Read and write').click({force: true})

    cy.get('[data-cy="usernameinput"]').type('third')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.get('input[name="shareperm-third"]').parents('.react-dropdown-select').click()
    cy.get('input[name="shareperm-third"]').parents('.react-dropdown-select').find('.react-dropdown-select-item').contains('Read only').click({force: true})
    cy.get('[data-cy="savebtn"]').click()
    cy.wait(waitTime)

    // Upload a test database to the user "second", plus setup some useful database sharing
    cy.request("/x/test/switchsecond")
    cy.visit('upload')
    cy.get('input[type=file]').selectFile('cypress/test_data/Assembly Election 2017.sqlite')
    cy.get('[data-cy="uploadbtn"]').click()
    cy.wait(waitTime)
    cy.get('[data-cy="settingslink"]').click()
    cy.get('[data-cy="private"]').click()

    cy.get('[data-cy="usernameinput"]').type('default')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.get('input[name="shareperm-default"]').parents('.react-dropdown-select').click()
    cy.get('input[name="shareperm-default"]').parents('.react-dropdown-select').find('.react-dropdown-select-item').contains('Read and write').click({force: true})

    cy.get('[data-cy="usernameinput"]').type('first')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.get('input[name="shareperm-first"]').parents('.react-dropdown-select').click()
    cy.get('input[name="shareperm-first"]').parents('.react-dropdown-select').find('.react-dropdown-select-item').contains('Read only').click({force: true})
    cy.get('[data-cy="savebtn"]').click()
    cy.wait(waitTime)

    // Upload a test database to the user "third", plus setup some useful database sharing
    cy.request("/x/test/switchthird")
    cy.visit('upload')
    cy.get('input[type=file]').selectFile('cypress/test_data/Assembly Election 2017.sqlite')
    cy.get('[data-cy="uploadbtn"]').click()
    cy.wait(waitTime)
    cy.get('[data-cy="settingslink"]').click()
    cy.get('[data-cy="private"]').click()

    cy.get('[data-cy="usernameinput"]').type('first')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.get('input[name="shareperm-first"]').parents('.react-dropdown-select').click()
    cy.get('input[name="shareperm-first"]').parents('.react-dropdown-select').find('.react-dropdown-select-item').contains('Read and write').click({force: true})

    cy.get('[data-cy="usernameinput"]').type('second')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.get('input[name="shareperm-second"]').parents('.react-dropdown-select').click()
    cy.get('input[name="shareperm-second"]').parents('.react-dropdown-select').find('.react-dropdown-select-item').contains('Read only').click({force: true})
    cy.get('[data-cy="savebtn"]').click()
    cy.wait(waitTime)

    // Switch back to the default user
    cy.request("/x/test/switchdefault")
  })

  // Public databases should remain viewable to users they're shared with
  it('public databases remain viewable to users they\'re shared with', () => {
    cy.visit("default/Assembly Election 2017.sqlite")
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/default/Assembly Election 2017.sqlite')
    cy.request("/x/test/switchfirst")
    cy.visit("default/Assembly Election 2017.sqlite")
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/default/Assembly Election 2017.sqlite')
    cy.request("/x/test/switchsecond")
    cy.visit("default/Assembly Election 2017.sqlite")
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/default/Assembly Election 2017.sqlite')
    cy.request("/x/test/switchthird")
    cy.visit("default/Assembly Election 2017.sqlite")
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/default/Assembly Election 2017.sqlite')
    cy.request("/x/test/switchdefault")
  })

  // Profile page - default user
  it('profile page shows expected shared databases - default user', () => {
    // Verify the right entries are showing up for the default user
    cy.visit("default")

    // Ensure the standard test databases are listed on the profile page where appropriate
    cy.get('[data-cy="sharedwithyou"]').should('not.contain', 'first / Assembly Election 2017.sqlite')
    cy.get('[data-cy="sharedwithyou"]').should('contain', 'second / Assembly Election 2017.sqlite')
    cy.get('[data-cy="sharedwithyou"]').should('not.contain', 'third / Assembly Election 2017.sqlite')

    // Ensure the live test database is listed correctly in the "Databases shared with you" section
    cy.get('[data-cy="sharedwithyou"]').should('contain', 'first / Join Testing with index.sqlite')
    cy.get('[data-cy="sharedwithyou"]').should('contain', 'Read Write')

    // Ensure the live test database is listed correctly in the "Databases shared with others" section
    cy.get('[data-cy="sharedwithothers"]').should('contain', 'Join Testing with index.sqlite')

    // Ensure trying to load the test databases only works where appropriate
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/default/Assembly Election 2017.sqlite')
    cy.visit({url: 'first/Assembly%20Election%202017.sqlite', failOnStatusCode: false})
    cy.get('[data-cy="errormsg"').should('contain', 'doesn\'t exist')
    cy.visit('second/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/second/Assembly Election 2017.sqlite')
    cy.visit({url: 'third/Assembly%20Election%202017.sqlite', failOnStatusCode: false})
    cy.get('[data-cy="errormsg"').should('contain', 'doesn\'t exist')
  })

  // Profile page - user "first"
  it('profile page shows expected shared databases - user "first"', () => {
    cy.request("/x/test/switchfirst")
    cy.visit("first")

    // Ensure the other test databases are only listed on the profile page where appropriate
    cy.get('[data-cy="sharedwithyou"]').should('not.contain', 'default / Assembly Election 2017.sqlite')
    cy.get('[data-cy="sharedwithyou"]').should('contain', 'second / Assembly Election 2017.sqlite')
    cy.get('[data-cy="sharedwithyou"]').should('contain', 'third / Assembly Election 2017.sqlite')

    // Ensure trying to load the other test databases only works where appropriate
    cy.visit('second/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/second/Assembly Election 2017.sqlite')
    cy.visit('third/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/third/Assembly Election 2017.sqlite')
    cy.request("/x/test/switchdefault")
  })

  // Profile page - user "second"
  it('profile page shows expected shared databases - user "second"', () => {
    cy.request("/x/test/switchsecond")
    cy.visit("second")

    // Ensure the other test databases are only listed on the profile page where appropriate
    cy.get('[data-cy="sharedwithyou"]').should('contain', 'first / Assembly Election 2017.sqlite')
    cy.get('[data-cy="sharedwithyou"]').should('contain', 'third / Assembly Election 2017.sqlite')

    // Ensure trying to load the other test databases only works where appropriate
    cy.visit('first/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/first/Assembly Election 2017.sqlite')
    cy.visit('third/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/third/Assembly Election 2017.sqlite')
    cy.request("/x/test/switchdefault")
  })

  // Profile page - user "third"
  it('profile page shows expected shared databases - user "third"', () => {
    cy.request("/x/test/switchthird")
    cy.visit("third")

    // Ensure the other test databases are only listed on the profile page where appropriate
    cy.get('[data-cy="sharedwithyou"]').should('contain', 'first / Assembly Election 2017.sqlite')
    cy.get('[data-cy="sharedwithyou"]').should('not.contain', 'second / Assembly Election 2017.sqlite')

    // Ensure trying to load the other test databases only works where appropriate
    cy.visit('first/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/first/Assembly Election 2017.sqlite')
    cy.visit({url: 'second/Assembly%20Election%202017.sqlite', failOnStatusCode: false})
    cy.get('[data-cy="errormsg"').should('contain', 'doesn\'t exist')
    cy.request("/x/test/switchdefault")
  })

  // Upload to shared read-only private database (should fail)
  it('Upload denied to shared read-only private databases', () => {
    cy.request("/x/test/switchfirst")
    cy.visit('second/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="uploadbtn"').click()
    cy.get('[data-cy="errormsg"').should('contain', 'don\'t have write access')

    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="uploadbtn"').click()
    cy.get('[data-cy="errormsg"').should('contain', 'don\'t have write access')

    cy.request("/x/test/switchsecond")
    cy.visit('third/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="uploadbtn"').click()
    cy.get('[data-cy="errormsg"').should('contain', 'don\'t have write access')

    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="uploadbtn"').click()
    cy.get('[data-cy="errormsg"').should('contain', 'don\'t have write access')

    cy.request("/x/test/switchthird")
    cy.visit('first/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="uploadbtn"').click()
    cy.get('[data-cy="errormsg"').should('contain', 'don\'t have write access')
  })

  // Upload to shared read-only public database (should fail)
  it('Upload denied to shared read-only public database', () => {
    cy.request("/x/test/switchsecond")
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="uploadbtn"').click()
    cy.get('[data-cy="errormsg"').should('contain', 'don\'t have write access')
  })

  // Upload to shared read-write private standard database (should succeed)
  it('Upload to shared read-write private standard database succeeds (part 1)', () => {
      cy.request("/x/test/switchdefault")
      cy.visit('second/Assembly%20Election%202017.sqlite')
      cy.get('[data-cy="uploadbtn"').click()
      cy.get('input[type=file]').selectFile('cypress/test_data/Assembly Election 2017.sqlite')
      cy.get('[data-cy="uploadbtn"').click()
      cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/second/Assembly Election 2017.sqlite')
  })
  it('Upload to shared read-write private standard database succeeds (part 2)', () => {
      cy.request("/x/test/switchfirst")
      cy.visit('third/Assembly%20Election%202017.sqlite')
      cy.get('[data-cy="uploadbtn"').click()
      cy.get('input[type=file]').selectFile('cypress/test_data/Assembly Election 2017.sqlite')
      cy.get('[data-cy="uploadbtn"').click()
      cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/third/Assembly Election 2017.sqlite')
  })
  it('Upload to shared read-write private standard database succeeds (part 3)', () => {
    cy.request("/x/test/switchsecond")
    cy.visit('first/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="uploadbtn"').click()
    cy.get('input[type=file]').selectFile('cypress/test_data/Assembly Election 2017.sqlite')
    cy.get('[data-cy="uploadbtn"').click()
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/first/Assembly Election 2017.sqlite')
  })

  // Upload to shared read-write public standard database (should succeed)
  it('Upload to shared read-write public standard database succeeds', () => {
    cy.request("/x/test/switchthird")
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="uploadbtn"').click()
    cy.get('input[type=file]').selectFile('cypress/test_data/Assembly Election 2017.sqlite')
    cy.get('[data-cy="uploadbtn"').click()
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/default/Assembly Election 2017.sqlite')
    cy.get('[data-cy="vis"]').should('have.text', 'Public')
    cy.request("/x/test/switchdefault")
  })
})
