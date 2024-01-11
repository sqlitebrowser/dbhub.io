describe('tags', () => {
  before(() => {
    // Seed data
    cy.request('/x/test/seed')
  })

  // Create tag
  it('create tag', () => {
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="tagscnt"]').should('contain', '2')
    cy.get('[data-cy="commitslnk"]').click()
    cy.get('[data-cy="createtagrelbtn"]').click()
    cy.get('[data-cy="nameinput"]').type('{selectall}{backspace}').type('Some tag name').should('have.value', 'Some tag name')
    cy.get('[data-cy="tagdesc"]').type('{selectall}{backspace}').type('Some tag description').should('have.value', 'Some tag description')
    cy.get('[data-rr-ui-event-key="tagdesc-preview-tab"]').click()
    cy.get('[data-cy="tagdesc-preview"]').should('contain', 'Some tag description')
    cy.get('[data-cy="createbtn"]').click()
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="tagscnt"]').should('contain', '3')
  })

  // Tag details are ok
  it('check tag details', () => {
    cy.visit('tags/default/Assembly%20Election%202017.sqlite')

    // Name
    cy.get('[data-cy="nameinput"]').should('have.value', 'Some tag name')

    // Description
    cy.get('[data-cy="Some tag name_desc-preview"]').should('contain', 'Some tag description')

    // Edit description field
    cy.get('[data-cy="Some tag name_desc"]').should('have.value', 'Some tag description')

    // URL for tag creator
    cy.get('[data-cy="taggerlnk"]').first().click()
    cy.location('pathname').should('equal', '/default')

    // URL for commit id
    cy.visit('tags/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="commitlnk"]').first().click()
    cy.location().should((loc) => {
      expect(loc.pathname).to.eq('/default/Assembly%20Election%202017.sqlite')
      expect(loc.search).to.match(/^\?commit=.*$/)
    })
  })

  // Rename tag
  it('rename tag', () => {
    cy.visit('tags/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="nameinput"]').first().should('have.value', 'Some tag name')
    cy.get('[data-cy="nameinput"]').first().type('{selectall}{backspace}').type('Some other name').should('have.value', 'Some other name')
    cy.get('[data-cy="updatebtn"]').first().click()
    cy.reload()
    cy.get('[data-cy="nameinput"]').first().should('have.value', 'Some other name')
  })

  // Change description text
  it('change tag description', () => {
    cy.visit('tags/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="Some other name_desc-preview"]').should('contain', 'Some tag description')
    cy.get('[data-rr-ui-event-key="Some other name_desc-edit-tab"]').click()
    cy.get('[data-cy="Some other name_desc"]').type('{selectall}{backspace}').type('A new description').should('have.value', 'A new description')
    cy.get('[data-cy="updatebtn"]').first().click()
    cy.reload()
    cy.get('[data-cy="Some other name_desc-preview"]').should('contain', 'A new description')
  })

  // Delete tags
  it('delete tag 1', () => {
    cy.visit('tags/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="delbtn"]').first().click()
  })
  it('delete tag 2', () => {
    cy.visit('tags/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="delbtn"]').first().click()
  })
  it('delete tag 3', () => {
    cy.visit('tags/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="delbtn"]').first().click()

    cy.visit('tags/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="notagstxt"]').should('not.have.attr', 'hidden')
  })
})
