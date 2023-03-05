import path from "path";

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

  // Databases
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" \
  //       https://localhost:9444/v1/databases
  it('databases', () => {
    cy.request({
      method: 'POST',
      url: 'https://localhost:9444/v1/databases',
      form: true,
      body: {
        apikey: '2MXwA5jGZkIQ3UNEcKsuDNSPMlx'
      },
    }).then(
      (response) => {
        expect(response.status).to.eq(200)
        let jsonBody = JSON.parse(response.body)
        expect(jsonBody).to.include.members(["Assembly Election 2017.sqlite"])
      }
    )
  })

  // Delete
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" \
  //       -F dbname="Assembly Election 2017.sqlite" \
  //       https://localhost:9444/v1/delete
  it('delete', () => {
    cy.request({
      method: 'POST',
      url: 'https://localhost:9444/v1/delete',
      form: true,
      body: {
        apikey: '2MXwA5jGZkIQ3UNEcKsuDNSPMlx',
        dbname: 'Assembly Election 2017.sqlite',
      },
    }).then(
      (response) => {
        expect(response.status).to.eq(200)
        let jsonBody = JSON.parse(response.body)
        expect(jsonBody).to.have.property('status', 'OK')

        // Verify the database is no longer present
        cy.request({
          method: 'POST',
          url: 'https://localhost:9444/v1/databases',
          form: true,
          body: {
            apikey: '2MXwA5jGZkIQ3UNEcKsuDNSPMlx'
          },
        }).then(
          (response) => {
            expect(response.status).to.eq(200)
            expect(response.body).to.eq('null')

            // Restore the contents of the database
            cy.request('/x/test/seed')
          }
        )
      }
    )
  })

  // Download
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" \
  //       -F dbowner="default"  -F dbname="Assembly Election 2017.sqlite" \
  //       https://localhost:9444/v1/download
  const downloadsFolder = Cypress.config('downloadsFolder')
  it('download', () => {
    cy.request({
      method: 'POST',
      url: 'https://localhost:9444/v1/download',
      form: true,
      body: {
        apikey: '2MXwA5jGZkIQ3UNEcKsuDNSPMlx',
        dbowner: 'default',
        dbname: 'Assembly Election 2017.sqlite',
      },
    }).then(
      (response) => {
        expect(response.status).to.eq(200)

        // Save the database to local disk
        const db = path.join(downloadsFolder, 'Assembly Election 2017.sqlite')
        // FIXME: cy.writeFile() isn't writing the full file out to disk, even though the server
        //        is definitely sending it (as evidenced by curl having no issues).  It would be
        //        good to figure out wtf is causing this problem, then fix it and write a more
        //        thorough cy.readFile() test.
        cy.writeFile(db, response.body, 'binary')

        // Verify the downloaded file is ok
        cy.readFile(db, 'binary', { timeout: 5000 }).should('have.length.gt', 512)

        // Remove the downloaded file
        cy.task('rmFile', { path: db })
      }
    )
  })

  // Indexes
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" \
  //       -F dbowner="default"  -F dbname="Assembly Election 2017.sqlite" \
  //       https://localhost:9444/v1/indexes
  it('indexes', () => {
    cy.request({
      method: 'POST',
      url: 'https://localhost:9444/v1/indexes',
      form: true,
      body: {
        apikey: '2MXwA5jGZkIQ3UNEcKsuDNSPMlx',
        dbowner: 'default',
        dbname: 'Assembly Election 2017.sqlite'
      },
    }).then(
      (response) => {
        expect(response.status).to.eq(200)

        // Needs an extra step, due to the structure of the returned JSON
        let temp = JSON.parse(response.body)

        let jsonBody = temp[0]
        expect(jsonBody).to.have.property('name')
        expect(jsonBody).to.have.property('table')

        let columns = jsonBody.columns[0]
        expect(columns).to.have.property('id')
        expect(columns).to.have.property('name')
      }
    )
  })

  // Metadata
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" \
  //       -F dbowner="default"  -F dbname="Assembly Election 2017.sqlite" \
  //       https://localhost:9444/v1/metadata
  it('metadata', () => {
    cy.request({
      method: 'POST',
      url: 'https://localhost:9444/v1/metadata',
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
        expect(jsonBody).to.have.property('default_branch', 'main')

        // Test the "branches" structure
        let branchesMain = jsonBody.branches.main
        expect(branchesMain).to.have.property('commit_count', 1)
        expect(branchesMain).to.have.property('description', '')

        // Test the "commits" structure
        let commitID = Object.keys(jsonBody.commits)
        let commitData = jsonBody.commits[commitID]
        expect(commitData).to.have.property('author_email', 'default@dbhub.io')
        expect(commitData).to.have.property('author_name', 'Default system user')
        expect(commitData).to.have.property('committer_email', '')
        expect(commitData).to.have.property('committer_name', '')
        expect(commitData).to.have.property('message', 'Initial commit')
        expect(commitData).to.have.property('other_parents', null)
        expect(commitData).to.have.property('parent', '')
        expect(commitData).to.include.keys(['id', 'timestamp', 'tree'])

        // Test the "tree" structure
        let entries = commitData.tree.entries[0]
        expect(entries).to.have.property('entry_type', 'db')
        expect(entries).to.have.property('licence', '9348ddfd44da5a127c59141981954746a860ec8e03e0412cf3af7134af0f97e2')
        expect(entries).to.have.property('name', 'Assembly Election 2017.sqlite')
        expect(entries).to.have.property('sha256', '4244d55013359c6476d06c045700139629ecfd2752ffad141984ba14ecafd17e')
        expect(entries).to.have.property('size', 57344)
        expect(entries).to.include.keys(['last_modified'])
      }
    )
  })

  // Tables
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" \
  //       -F dbowner="default"  -F dbname="Assembly Election 2017.sqlite" \
  //       https://localhost:9444/v1/tables
  it('tables', () => {
    cy.request({
      method: 'POST',
      url: 'https://localhost:9444/v1/tables',
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
        expect(jsonBody).to.have.members([
            "Candidate_Information",
            "Constituency_Turnout_Information",
            "Elected_Candidates"
          ]
        )
      }
    )
  })

  // Webpage
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" \
  //       -F dbowner="default"  -F dbname="Assembly Election 2017.sqlite" \
  //       https://localhost:9444/v1/webpage
  it('webpage', () => {
    cy.request({
      method: 'POST',
      url: 'https://localhost:9444/v1/webpage',
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
        expect(jsonBody).to.have.property('web_page')
        expect(jsonBody.web_page).to.match(/.*\/default\/Assembly\ Election\ 2017\.sqlite$/)
      }
    )
  })
})