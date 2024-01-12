describe('releases', () => {
  before(() => {
    // Seed data
    cy.request('/x/test/seed')
  })

  // Create release
  it('create release', () => {
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="rlscnt"]').should('contain', '2')
    cy.get('[data-cy="commitslnk"]').click()
    cy.get('[data-cy="createtagrelbtn"]').click()
    cy.get('[data-cy="nameinput"]').type('{selectall}{backspace}').type('Some release name').should('have.value', 'Some release name')
    cy.get('[data-cy="relradio"]').click()
    cy.get('[data-cy="tagdesc"]').type('{selectall}{backspace}').type('Some release description').should('have.value', 'Some release description')
    cy.get('[data-cy="tagdesc-preview-tab"]').click()
    cy.get('[data-cy="tagdesc-preview"]').should('contain', 'Some release description')
    cy.get('[data-cy="createbtn"]').click()
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="rlscnt"]').should('contain', '3')
  })

  // Release details are ok
  it('check release details', () => {
    cy.visit('releases/default/Assembly%20Election%202017.sqlite')

    // Name
    cy.get('[data-cy="nameinput"]').first().should('have.value', 'Some release name')

    // Description
    cy.get('[data-cy="Some release name_desc-preview"]').should('contain', 'Some release description')

    // Edit description field
    cy.get('[data-cy="Some release name_desc"]').should('have.value', 'Some release description')

    // URL for tag creator
    cy.get('[data-cy="taggerlnk"]').first().click()
    cy.location('pathname').should('equal', '/default')

    // URL for commit id
    cy.visit('releases/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="commitlnk"]').first().click()
    cy.location().should((loc) => {
      expect(loc.pathname).to.eq('/default/Assembly%20Election%202017.sqlite')
      expect(loc.search).to.match(/^\?commit=.*$/)
    })
  })

  // Rename release
  it('rename release', () => {
    cy.visit('releases/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="nameinput"]').should('have.value', 'Some release name')
    cy.get('[data-cy="nameinput"]').first().type('{selectall}{backspace}').type('Some other name').should('have.value', 'Some other name')
    cy.get('[data-cy="updatebtn"]').first().click()
    cy.reload()
    cy.get('[data-cy="nameinput"]').should('have.value', 'Some other name')
  })

  // Change description text
  it('change release description', () => {
    cy.visit('releases/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="Some other name_desc-preview"]').should('contain', 'Some release description')
    cy.get('[data-cy="Some other name_desc-edit-tab"]').click()
    cy.get('[data-cy="Some other name_desc"]').type('{selectall}{backspace}').type('A new description').should('have.value', 'A new description')
    cy.get('[data-cy="updatebtn"]').first().click()
    cy.reload()
    cy.get('[data-cy="Some other name_desc-preview"]').should('contain', 'A new description')
  })

  // Delete releases
  it('delete release 1', () => {
    cy.visit('releases/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="delbtn"]').first().click()
  })
  it('delete release 2', () => {
    cy.visit('releases/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="delbtn"]').first().click()
  })
  it('delete release 3', () => {
    cy.visit('releases/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="delbtn"]').first().click()

    cy.visit('releases/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="notagstxt"]').should('not.have.attr', 'hidden')
  })
})
