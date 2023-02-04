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
    cy.get('[data-cy="onelinedescinput"]').should('have.value', 'No description')
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
    cy.get('[data-cy="deftblname"]').should('contain.text', 'Candidate_Information')
    cy.get('[data-cy="deftbldropdown"]').click()
    cy.get('[data-cy="tbl-Constituency_Turnout_Information"]').click()
    cy.get('[data-cy="savebtn"]').click()
    cy.get('[data-cy="settingslink"]').click()
    cy.get('[data-cy="deftblname"]').should('contain.text', 'Constituency_Turnout_Information')
  })

  // Default branch
  it('default branch', () => {
    cy.visit('settings/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="defbranchname"]').should('contain.text', 'master')
    cy.get('[data-cy="defbranchdropdown"]').click()
    cy.get('[data-cy="branch-stuff"]').click()
    cy.get('[data-cy="savebtn"]').click()
    cy.get('[data-cy="settingslink"]').click()
    cy.get('[data-cy="defbranchname"]').should('contain.text', 'stuff')
  })

  // Source URL
  it('source url', () => {
    cy.visit('settings/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="srcurlinput"]').should('have.value', 'http://data.nicva.org/dataset/assembly-election-2017')
    cy.get('[data-cy="srcurlinput"]').type('{selectall}{backspace}https://example.org')
    cy.get('[data-cy="savebtn"]').click()
    cy.get('[data-cy="settingslink"]').click()
    cy.get('[data-cy="srcurlinput"]').should('have.value', 'https://example.org')
  })

  // Licence
  it('licence', () => {
    // Test the master branch
    cy.visit('settings/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="master-licname"]').should('contain.text', 'CC-BY-SA-4.0')
    cy.get('[data-cy="master-licdropdown"]').click()
    cy.get('[data-cy="lic-master-Not specified"]').click()
    cy.get('[data-cy="savebtn"]').click()
    cy.get('[data-cy="settingslink"]').click()
    cy.get('[data-cy="master-licname"]').should('contain.text', 'Not specified')

    // Test the 2nd branch
    cy.get('[data-cy="stuff-licname"]').should('contain.text', 'CC-BY-SA-4.0')
    cy.get('[data-cy="stuff-licdropdown"]').click()
    cy.get('[data-cy="lic-stuff-CC0"]').click({force: true}) // Firefox sizing seems to have this *slightly* clipped, but otherwise usable
    cy.get('[data-cy="savebtn"]').click()
    cy.get('[data-cy="settingslink"]').click()
    cy.get('[data-cy="stuff-licname"]').should('contain.text', 'CC0')
  })

  // Share Database
  // it('check tag details', () => {
  //   cy.visit('settings/default/Assembly%20Election%202017.sqlite')
  //   // TODO
  //   * Also ensure the user CAN NOT add themselves to this list (seems to be currently possible)
  // })

  // Full description
  it('full description', () => {
    cy.visit('settings/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="rendereddiv"]').should('contain.text', 'No full description')
    cy.get('[data-cy="edittab"]').click()
    cy.get('[data-cy="desctext"]').should('contain.text', 'No full description')
    cy.get('[data-cy="desctext"]').type('{selectall}{backspace}Some new description')
    cy.get('[data-cy="renderedtab"]').click()
    cy.get('[data-cy="rendereddiv"]').should('contain.text', 'Some new description')
    cy.get('[data-cy="savebtn"]').click()
    cy.get('[data-cy="settingslink"]').click()
    cy.get('[data-cy="rendereddiv"]').should('contain.text', 'Some new description')
  })

  // Delete database
  it('delete database', () => {
    cy.visit('settings/default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="delbtn"]').click()
    cy.get('[data-cy="confirmbtn"]').click()
    cy.get('[data-cy="pubdbs"]').should('not.contain', 'Assembly Election 2017.sqlite')
    cy.get('[data-cy="privdbs"]').should('not.contain', 'Assembly Election 2017.sqlite')
  })
})