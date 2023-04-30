describe('diff databases', () => {
  before(() => {
    // Seed data
    cy.request('/x/test/seed')

    // Create new branch
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="commitslnk"]').click()
    cy.get('[data-cy="createbranchbtn"]').click()
    cy.get('[data-cy="nameinput"]').type('{selectall}{backspace}').type('firstBranch')
    cy.get('[data-cy="createbtn"]').click()

    // Upload database to the new branch
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="uploadbtn"]').click()
    cy.get('input[type=file]').selectFile('cypress/test_data/Assembly Election 2017 with view.sqlite')
    cy.get('[data-cy="uploadbtn"]').click()
  })

  // Diff between two databases with just a simple schema change (view creation)
  it('schema change only diff', () => {
    cy.visit('/branches/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="comparebtn"]').click()
    cy.get('[data-cy="objname"]').should('have.text', 'Candidate_Names')
    cy.get('[data-cy="objtype"]').should('have.text', 'view')
    cy.get('[data-cy="droptype"]').should('have.text', 'Dropped view')
    cy.get('[data-cy="dropdetail"]').should('have.text', 'CREATE VIEW "Candidate_Names" AS\n' +
      '  SELECT Firstname, Surname\n' +
      '  FROM "Candidate_Information"\n' +
      '  ORDER BY Surname, Firstname\n' +
      '  DESC')
  })
})