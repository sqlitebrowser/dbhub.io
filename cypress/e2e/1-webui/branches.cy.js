describe('branches', () => {
  before(() => {
    // Seed data
    cy.request('/x/test/seed')
  })

  // Create branch
  it('create branch', () => {
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="branchescnt"]').should('contain', '1')
    cy.get('[data-cy="commitslnk"]').click()
    cy.get('[data-cy="createbranchbtn"]').click()
    cy.get('[data-cy="nameinput"]').type('{selectall}{backspace}').type('Some branch name').should('have.value', 'Some branch name')
    cy.get('[data-cy="branchdesc"]').type('{selectall}{backspace}').type('Some branch description').should('have.value', 'Some branch description')
    cy.get('[data-cy="branchdesc-preview-tab"]').click()
    cy.get('[data-cy="branchdesc-preview"]').should('contain', 'Some branch description')
    cy.get('[data-cy="createbtn"]').click()
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="branchescnt"]').should('contain', '2')
  })

  // Branch details are ok
  it('check branch details', () => {
    cy.visit('branches/default/Assembly%20Election%202017.sqlite')

    // Name
    cy.get('[data-cy="nameinput"]').first().should('have.value', 'main')

    // Description
    cy.get('[data-cy="main_desc-preview"]').first().should('contain', 'No description')

    // Editable description tag
    cy.get('[data-cy="edittab"]').first().click()
    cy.get('[data-cy="desctext"]').first().should('be.empty')

    // URL for commit id
    cy.get('[data-cy="commitlnk"]').first().should('have.attr', 'href').and('match', /^\/default\/Assembly%20Election%202017.sqlite\?branch=main&commit=.*$/)
  })

  // Rename branch
  it('rename branch', () => {
    cy.visit('branches/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="nameinput"]').first().should('have.value', 'main')
    cy.get('[data-cy="nameinput"]').first().type('{selectall}{backspace}').type('Some other name').should('have.value', 'Some other name')
    cy.get('[data-cy="savebtn"]').first().click()
    cy.reload()
    cy.get('[data-cy="nameinput"]').first().should('have.value', 'Some other name')
  })

  // Change description text
  it('change branch description', () => {
    cy.visit('branches/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="rendereddiv"]').first().should('contain', 'No description')
    cy.get('[data-cy="edittab"]').first().click()
    cy.get('[data-cy="desctext"]').first().type('{selectall}{backspace}').type('A new description').should('have.value', 'A new description')
    cy.get('[data-cy="savebtn"]').first().click()
    cy.reload()
    cy.get('[data-cy="rendereddiv"]').first().should('contain', 'A new description')
  })

  // Delete branch
  it('delete branch', () => {
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="branchescnt"]').should('contain', '2')
    cy.visit('branches/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="delbtn"]').click()
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="branchescnt"]').should('contain', '1')
  })
})
