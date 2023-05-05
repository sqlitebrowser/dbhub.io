describe('ensure auth0 dialog is available on all pages', () => {

  /*
    TODO - These pages still need to be considered, and/or have tests added for
    discussioncomments.html
    mergerequestcomments.html
    mergerequestlist.html
  */

  before(() => {
    // Seed data
    cy.request('/x/test/seed')

    // Pretend-switch to the "production" environment
    cy.request('/x/test/envprod')
  })

  beforeEach(() => {
    cy.on('uncaught:exception', (err, runnable) => { return false }) // Ignore Chatwoot error
  })

  it('about page', () => {
    cy.visit('about')
    cy.get('[data-cy="aboutus"]').should('contain.text', 'About Us')
    cy.get('[data-cy="loginlnk"]').click()
    cy.get('.auth0-lock-name').should('contain.text', 'Auth0')
  })

  it('branches page', () => {
    cy.visit('branches/default/Assembly Election 2017.sqlite')
    //cy.get('[data-cy="branchesfor"]').should('contain.text', 'Branches for')
    cy.get('[data-cy="loginlnk"]').click()
    cy.get('.auth0-lock-name').should('contain.text', 'Auth0')
  })

  it('commits page', () => {
    cy.visit('commits/default/Assembly Election 2017.sqlite')
    cy.get('[data-cy="commithist"]').should('contain.text', 'Commit history for branch')
    cy.get('[data-cy="loginlnk"]').click()
    cy.get('.auth0-lock-name').should('contain.text', 'Auth0')
  })

  it('contributors page', () => {
    cy.visit('contributors/default/Assembly Election 2017.sqlite')
    //cy.get('[data-cy="contribs"]').should('contain.text', 'Contributors to')
    cy.get('[data-cy="loginlnk"]').click()
    cy.get('.auth0-lock-name').should('contain.text', 'Auth0')
  })

  it('database page', () => {
    cy.visit('default/Assembly Election 2017.sqlite')
    cy.get('[data-cy="tagscnt"]').should('contain.text', '0')
    cy.get('[data-cy="loginlnk"]').click()
    cy.get('.auth0-lock-name').should('contain.text', 'Auth0')
  })

  it('discussion page', () => {
    cy.visit('discuss/default/Assembly Election 2017.sqlite')
    cy.get('[data-cy="nodisc"]').should('contain.text', 'This database does not have any discussions yet')
    cy.get('[data-cy="loginlnk"]').click()
    cy.get('.auth0-lock-name').should('contain.text', 'Auth0')
  })

  it('forks page', () => {
    cy.visit('forks/default/Assembly Election 2017.sqlite')
    cy.get('[data-cy="forks"]').should('contain.text', 'Forks of')
    cy.get('[data-cy="loginlnk"]').click()
    cy.get('.auth0-lock-name').should('contain.text', 'Auth0')
  })

  it('front page', () => {
    cy.visit('/')
    cy.get('[data-cy="features"]').should('contain.text', 'Features')
    cy.get('[data-cy="loginlnk"]').click()
    cy.get('.auth0-lock-name').should('contain.text', 'Auth0')
  })

  it('logged-out user profile page', () => {
    cy.visit('default')
    cy.get('[data-cy="userpg"]').should('contain.text', 'public projects')
    cy.get('[data-cy="loginlnk"]').click()
    cy.get('.auth0-lock-name').should('contain.text', 'Auth0')
  })

  it('releases page', () => {
    cy.visit('releases/default/Assembly Election 2017.sqlite')
    //cy.get('[data-cy="relsfor"]').should('contain.text', 'Releases for')
    cy.get('[data-cy="loginlnk"]').click()
    cy.get('.auth0-lock-name').should('contain.text', 'Auth0')
  })

  it('stars page', () => {
    cy.visit('stars/default/Assembly Election 2017.sqlite')
    cy.title().should('include', 'Stars')
    cy.get('[data-cy="loginlnk"]').click()
    cy.get('.auth0-lock-name').should('contain.text', 'Auth0')
  })

  it('tags page', () => {
    cy.visit('tags/default/Assembly Election 2017.sqlite')
    //cy.get('[data-cy="tagsfor"]').should('contain.text', 'Tags for')
    cy.get('[data-cy="loginlnk"]').click()
    cy.get('.auth0-lock-name').should('contain.text', 'Auth0')
  })

  it('visualise page', () => {
    cy.visit('vis/default/Assembly Election 2017.sqlite')
    cy.title().should('include', 'Visualisations')
    cy.get('[data-cy="loginlnk"]').click()
    cy.get('.auth0-lock-name').should('contain.text', 'Auth0')
  })

  it('watchers page', () => {
    cy.visit('watchers/default/Assembly Election 2017.sqlite')
    cy.title().should('include', 'Watchers')
    cy.get('[data-cy="loginlnk"]').click()
    cy.get('.auth0-lock-name').should('contain.text', 'Auth0')
  })

  after(() => {
    // Pretend-switch back to the "test" environment
    cy.request('/x/test/envtest')
  })
})
