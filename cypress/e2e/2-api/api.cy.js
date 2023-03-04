describe('api tests', () => {
  before(() => {
    // Seed data
    cy.request('/x/test/seed')
  })

  // Branches
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" -F dbowner="default" \
  //       -F dbname="Assembly Election 2017.sqlite" https://localhost:9444/v1/branches
  it('branches', () => {
    cy.request({
      method: 'POST',
      url: 'https://localhost:9444/v1/branches',
      form: true,
      body: {
        apikey: '2MXwA5jGZkIQ3UNEcKsuDNSPMlx',
        dbowner: 'default',
        dbname: 'Assembly Election 2017.sqlite'
      },
    }).then(
      (response) => {
        expect(response.status).to.eq(200)
        let jsonBody = JSON.parse(response.body)
        expect(jsonBody).to.include.keys(['branches', 'default_branch'])
        expect(jsonBody).to.have.property('default_branch', 'main')
        expect(jsonBody.branches.main).to.have.property('commit')
        expect(jsonBody.branches.main).to.have.property('commit_count', 1)
        expect(jsonBody.branches.main).to.have.property('description', '')
      }
    )
  })

  // Columns
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" \
  //       -F dbowner="default"  -F dbname="Assembly Election 2017.sqlite" \
  //       -F table="Candidate_Information" \
  //       https://localhost:9444/v1/columns
  it('columns', () => {
    cy.request({
      method: 'POST',
      url: 'https://localhost:9444/v1/columns',
      form: true,
      body: {
        apikey: '2MXwA5jGZkIQ3UNEcKsuDNSPMlx',
        dbowner: 'default',
        dbname: 'Assembly Election 2017.sqlite',
        table: 'Candidate_Information'
      },
    }).then(
      (response) => {
        expect(response.status).to.eq(200)
        let jsonBody = JSON.parse(response.body)
        expect(jsonBody[0]).to.include.keys(['column_id', 'default_value', 'name', 'not_null'])
      }
    )
  })

  // Commits
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" \
  //       -F dbowner="default"  -F dbname="Assembly Election 2017.sqlite" \
  //       https://localhost:9444/v1/commits
  it('commits', () => {
    cy.request({
      method: 'POST',
      url: 'https://localhost:9444/v1/commits',
      form: true,
      body: {
        apikey: '2MXwA5jGZkIQ3UNEcKsuDNSPMlx',
        dbowner: 'default',
        dbname: 'Assembly Election 2017.sqlite',
      },
    }).then(
      (response) => {
        expect(response.status).to.eq(200)

        // Needs an extra step, due to the structure of the returned JSON
        let temp = JSON.parse(response.body)
        let jsonBody = temp[Object.keys(temp)[0]]

        expect(jsonBody).to.have.property('author_email', 'default@dbhub.io')
        expect(jsonBody).to.have.property('author_name', 'Default system user')
        expect(jsonBody).to.have.property('committer_email', '')
        expect(jsonBody).to.have.property('committer_name', '')
        expect(jsonBody).to.have.property('message', 'Initial commit')
        expect(jsonBody).to.have.property('other_parents', null)
        expect(jsonBody).to.have.property('parent', '')
        expect(jsonBody).to.include.keys(['id', 'timestamp', 'tree'])

        // Test the "tree" entries
        let entries = jsonBody.tree.entries[0]
        expect(entries).to.have.property('entry_type', 'db')
        expect(entries).to.have.property('licence', '9348ddfd44da5a127c59141981954746a860ec8e03e0412cf3af7134af0f97e2')
        expect(entries).to.have.property('name', 'Assembly Election 2017.sqlite')
        expect(entries).to.have.property('sha256', '4244d55013359c6476d06c045700139629ecfd2752ffad141984ba14ecafd17e')
        expect(entries).to.have.property('size', 57344)
        expect(entries).to.include.keys(['last_modified'])
      }
    )
  })
})