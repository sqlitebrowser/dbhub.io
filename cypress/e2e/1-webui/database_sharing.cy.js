describe('database sharing', () => {
  before(() => {
    // Seed data
    cy.request('/x/test/seed')

    // Setup some shares to this public database
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="settingslink"]').click()
    cy.get('[data-cy="usernameinput"]').type('second')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.get('[data-cy="sharedropdown-second"]').click()
    cy.get('[data-cy="sharero-second"]').click()

    cy.get('[data-cy="usernameinput"]').type('third')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.get('[data-cy="sharedropdown-third"]').click()
    cy.get('[data-cy="sharerw-third"]').click({force: true})
    cy.get('[data-cy="savebtn"]').click()

    // Upload the test database to the user "first", plus setup some useful database sharing
    cy.request("/x/test/switchfirst")
    cy.visit('upload')
    cy.get('input[type=file]').selectFile('cypress/test_data/Assembly Election 2017.sqlite')
    cy.get('[data-cy="uploadbtn"]').click()
    cy.get('[data-cy="settingslink"]').click()
    cy.get('[data-cy="private"]').click()

    cy.get('[data-cy="usernameinput"]').type('second')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.get('[data-cy="sharedropdown-second"]').click()
    cy.get('[data-cy="sharerw-second"]').click({force: true})

    cy.get('[data-cy="usernameinput"]').type('third')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.get('[data-cy="sharedropdown-third"]').click()
    cy.get('[data-cy="sharero-third"]').click()
    cy.get('[data-cy="savebtn"]').click()

    // Upload the test database to the user "second", plus setup some useful database sharing
    cy.request("/x/test/switchsecond")
    cy.visit('upload')
    cy.get('input[type=file]').selectFile('cypress/test_data/Assembly Election 2017.sqlite')
    cy.get('[data-cy="uploadbtn"]').click()
    cy.get('[data-cy="settingslink"]').click()
    cy.get('[data-cy="private"]').click()

    cy.get('[data-cy="usernameinput"]').type('default')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.get('[data-cy="sharedropdown-default"]').click()
    cy.get('[data-cy="sharerw-default"]').click({force: true})

    cy.get('[data-cy="usernameinput"]').type('first')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.get('[data-cy="sharedropdown-first"]').click()
    cy.get('[data-cy="sharero-first"]').click()
    cy.get('[data-cy="savebtn"]').click()

    // Upload the test database to the user "third", plus setup some useful database sharing
    cy.request("/x/test/switchthird")
    cy.visit('upload')
    cy.get('input[type=file]').selectFile('cypress/test_data/Assembly Election 2017.sqlite')
    cy.get('[data-cy="uploadbtn"]').click()
    cy.get('[data-cy="settingslink"]').click()
    cy.get('[data-cy="private"]').click()

    cy.get('[data-cy="usernameinput"]').type('first')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.get('[data-cy="sharedropdown-first"]').click()
    cy.get('[data-cy="sharerw-first"]').click({force: true})

    cy.get('[data-cy="usernameinput"]').type('second')
    cy.get('[data-cy="adduserbtn"]').click()
    cy.get('[data-cy="sharedropdown-second"]').click()
    cy.get('[data-cy="sharero-second"]').click()
    cy.get('[data-cy="savebtn"]').click()

    // Switch back to the default user
    cy.request("/x/test/switchdefault")
  })

  // Public databases should remain viewable to users they're shared with
  it('public databases remain viewable to users they\'re shared with', () => {
    cy.visit("default/Assembly Election 2017.sqlite")
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/default/Assembly%20Election%202017.sqlite')
    cy.request("/x/test/switchfirst")
    cy.visit("default/Assembly Election 2017.sqlite")
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/default/Assembly%20Election%202017.sqlite')
    cy.request("/x/test/switchsecond")
    cy.visit("default/Assembly Election 2017.sqlite")
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/default/Assembly%20Election%202017.sqlite')
    cy.request("/x/test/switchthird")
    cy.visit("default/Assembly Election 2017.sqlite")
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/default/Assembly%20Election%202017.sqlite')
    cy.request("/x/test/switchdefault")
  })

  // Profile page - default user
  it('profile page shows expected shared databases - default user', () => {
    // Verify the right entries are showing up for the default user
    cy.visit("default")

    // Ensure the other test databases are only listed on the profile page where appropriate
    cy.get('[data-cy="sharedwithyoutbl"').should('not.contain', 'first/Assembly Election 2017.sqlite')
    cy.get('[data-cy="sharedwithyoutbl"').should('contain', 'second/Assembly Election 2017.sqlite')
    cy.get('[data-cy="sharedwithyoutbl"').should('not.contain', 'third/Assembly Election 2017.sqlite')

    // Ensure trying to load the test databases only works where appropriate
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/default/Assembly%20Election%202017.sqlite')
    cy.visit({url: 'first/Assembly%20Election%202017.sqlite', failOnStatusCode: false})
    cy.get('[data-cy="errormsg"').should('contain', 'doesn\'t exist')
    cy.visit('second/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/second/Assembly%20Election%202017.sqlite')
    cy.visit({url: 'third/Assembly%20Election%202017.sqlite', failOnStatusCode: false})
    cy.get('[data-cy="errormsg"').should('contain', 'doesn\'t exist')
  })

  // Profile page - user "first"
  it('profile page shows expected shared databases - user "first"', () => {
    cy.request("/x/test/switchfirst")
    cy.visit("first")

    // Ensure the other test databases are only listed on the profile page where appropriate
    cy.get('[data-cy="sharedwithyoutbl"').should('not.contain', 'default/Assembly Election 2017.sqlite')
    cy.get('[data-cy="sharedwithyoutbl"').should('contain', 'second/Assembly Election 2017.sqlite')
    cy.get('[data-cy="sharedwithyoutbl"').should('contain', 'third/Assembly Election 2017.sqlite')

    // Ensure trying to load the other test databases only works where appropriate
    cy.visit('second/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/second/Assembly%20Election%202017.sqlite')
    cy.visit('third/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/third/Assembly%20Election%202017.sqlite')
    cy.request("/x/test/switchdefault")
  })

  // Profile page - user "second"
  it('profile page shows expected shared databases - user "second"', () => {
    cy.request("/x/test/switchsecond")
    cy.visit("second")

    // Ensure the other test databases are only listed on the profile page where appropriate
    cy.get('[data-cy="sharedwithyoutbl"').should('contain', 'first/Assembly Election 2017.sqlite')
    cy.get('[data-cy="sharedwithyoutbl"').should('contain', 'third/Assembly Election 2017.sqlite')

    // Ensure trying to load the other test databases only works where appropriate
    cy.visit('first/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/first/Assembly%20Election%202017.sqlite')
    cy.visit('third/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/third/Assembly%20Election%202017.sqlite')
    cy.request("/x/test/switchdefault")
  })

  // Profile page - user "third"
  it('profile page shows expected shared databases - user "third"', () => {
    cy.request("/x/test/switchthird")
    cy.visit("third")

    // Ensure the other test databases are only listed on the profile page where appropriate
    cy.get('[data-cy="sharedwithyoutbl"').should('contain', 'first/Assembly Election 2017.sqlite')
    cy.get('[data-cy="sharedwithyoutbl"').should('not.contain', 'second/Assembly Election 2017.sqlite')

    // Ensure trying to load the other test databases only works where appropriate
    cy.visit('first/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/first/Assembly%20Election%202017.sqlite')
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

  // Upload to shared read-write private database (should succeed)
  it('Upload to shared read-write private database succeeds', () => {
    cy.request("/x/test/switchdefault")
    cy.visit('second/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="uploadbtn"').click()
    cy.get('input[type=file]').selectFile('cypress/test_data/Assembly Election 2017.sqlite')
    cy.get('[data-cy="uploadbtn"').click()
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/second/Assembly%20Election%202017.sqlite')

    cy.request("/x/test/switchfirst")
    cy.visit('third/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="uploadbtn"').click()
    cy.get('input[type=file]').selectFile('cypress/test_data/Assembly Election 2017.sqlite')
    cy.get('[data-cy="uploadbtn"').click()
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/third/Assembly%20Election%202017.sqlite')

    cy.request("/x/test/switchsecond")
    cy.visit('first/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="uploadbtn"').click()
    cy.get('input[type=file]').selectFile('cypress/test_data/Assembly Election 2017.sqlite')
    cy.get('[data-cy="uploadbtn"').click()
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/first/Assembly%20Election%202017.sqlite')
  })

  // Upload to shared read-write public database (should succeed)
  it('Upload to shared read-write public database succeeds', () => {
    cy.request("/x/test/switchthird")
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="uploadbtn"').click()
    cy.get('input[type=file]').selectFile('cypress/test_data/Assembly Election 2017.sqlite')
    cy.get('[data-cy="uploadbtn"').click()
    cy.get('[data-cy="headerdblnk"').should('have.attr', 'href').and('equal', '/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="vis"]').should('have.text', 'Public')
  })
})