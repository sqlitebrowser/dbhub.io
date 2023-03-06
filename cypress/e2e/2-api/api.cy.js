import path from "path";

// TODO: Looks like we don't have API functions yet to create branches, tags, or releases
//       Those could be useful to add

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
        expect(entries).to.have.property('sha256', '32e0815554a6fe4e3c17bda3c4abcddc47c0fa3e9291bdefd18effef08a16db8')
        expect(entries).to.have.property('size', 73728)
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
        expect(entries).to.have.property('sha256', '32e0815554a6fe4e3c17bda3c4abcddc47c0fa3e9291bdefd18effef08a16db8')
        expect(entries).to.have.property('size', 73728)
        expect(entries).to.include.keys(['last_modified'])
      }
    )
  })

  // Query
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" \
  //       -F dbowner="default"  -F dbname="Assembly Election 2017.sqlite" \
  //       -F sql="U0VMRUNUIGNvdW50KCopIEZST00gQ2FuZGlkYXRlX0luZm9ybWF0aW9u" \
  //       https://localhost:9444/v1/query
  //     Note: the sql argument above is the base64 encoding of "SELECT count(*) FROM Candidate_Information"
  //     Another useful sql is:
  //       text: 'SELECT Firstname, Surname FROM Candidate_Information ORDER BY Surname, Firstname LIMIT 1'
  //       base64: U0VMRUNUIEZpcnN0bmFtZSwgU3VybmFtZSBGUk9NIENhbmRpZGF0ZV9JbmZvcm1hdGlvbiBPUkRFUiBCWSBTdXJuYW1lLCBGaXJzdG5hbWUgTElNSVQgMQ==
  it('query', () => {
    cy.request({
      method: 'POST',
      url: 'https://localhost:9444/v1/query',
      form: true,
      body: {
        apikey: '2MXwA5jGZkIQ3UNEcKsuDNSPMlx',
        dbowner: 'default',
        dbname: 'Assembly Election 2017.sqlite',
        sql: btoa('SELECT Firstname, Surname FROM Candidate_Information ORDER BY Surname, Firstname LIMIT 1')
      },
    }).then(
      (response) => {
        expect(response.status).to.eq(200)

        let jsonBody = JSON.parse(response.body)
        expect(jsonBody[0][0]).to.have.property('Name', 'Firstname')
        expect(jsonBody[0][0]).to.have.property('Type', 3)
        expect(jsonBody[0][0]).to.have.property('Value', 'Steven')
        expect(jsonBody[0][1]).to.have.property('Name', 'Surname')
        expect(jsonBody[0][1]).to.have.property('Type', 3)
        expect(jsonBody[0][1]).to.have.property('Value', 'Agnew')
      }
    )
  })

  // Releases
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" \
  //       -F dbowner="default"  -F dbname="Assembly Election 2017.sqlite" \
  //       https://localhost:9444/v1/releases
  it('releases', () => {
    // Create a release, so we can test it using the API
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="rlscnt"]').should('contain', '0')
    cy.get('[data-cy="commitslnk"]').click()
    cy.get('[data-cy="createtagrelbtn"]').click()
    cy.get('[data-cy="nameinput"]').type('{selectall}{backspace}').type('Some release name').should('have.value', 'Some release name')
    cy.get('[data-cy="relradio"]').click()
    cy.get('[data-cy="desctext"]').type('{selectall}{backspace}').type('Some release description').should('have.value', 'Some release description')
    cy.get('[data-cy="previewtab"]').click()
    cy.get('[data-cy="previewdiv"]').should('contain', 'Some release description')
    cy.get('[data-cy="createbtn"]').click()
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="rlscnt"]').should('contain', '1')

    // Test the release details via the api
    cy.request({
      method: 'POST',
      url: 'https://localhost:9444/v1/releases',
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
        expect(jsonBody).to.have.property('Some release name')
        expect(jsonBody['Some release name']).to.include.keys(['commit', 'date'])
        expect(jsonBody['Some release name']).to.have.property('description', 'Some release description')
        expect(jsonBody['Some release name']).to.have.property('email', 'default@dbhub.io')
        expect(jsonBody['Some release name']).to.have.property('name', 'Default system user')
        expect(jsonBody['Some release name']).to.have.property('size', 73728)
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

  // Tags
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" \
  //       -F dbowner="default"  -F dbname="Assembly Election 2017.sqlite" \
  //       https://localhost:9444/v1/tags
  it('tags', () => {
    // Create a tag for us to test with
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="tagscnt"]').should('contain', '0')
    cy.get('[data-cy="commitslnk"]').click()
    cy.get('[data-cy="createtagrelbtn"]').click()
    cy.get('[data-cy="nameinput"]').type('{selectall}{backspace}').type('Some tag name').should('have.value', 'Some tag name')
    cy.get('[data-cy="desctext"]').type('{selectall}{backspace}').type('Some tag description').should('have.value', 'Some tag description')
    cy.get('[data-cy="previewtab"]').click()
    cy.get('[data-cy="previewdiv"]').should('contain', 'Some tag description')
    cy.get('[data-cy="createbtn"]').click()
    cy.visit('default/Assembly%20Election%202017.sqlite')
    cy.get('[data-cy="tagscnt"]').should('contain', '1')

    // Test the tag details using the API
    cy.request({
      method: 'POST',
      url: 'https://localhost:9444/v1/tags',
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
        expect(jsonBody).to.have.property('Some tag name')
        expect(jsonBody['Some tag name']).to.include.keys(['commit', 'date'])
        expect(jsonBody['Some tag name']).to.have.property('description', 'Some tag description')
        expect(jsonBody['Some tag name']).to.have.property('email', 'default@dbhub.io')
        expect(jsonBody['Some tag name']).to.have.property('name', 'Default system user')
      }
    )
  })

  // Upload
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" \
  //       -F dbname="Assembly Election 2017v2.sqlite" \
  //       -F file=@../../test_data/Assembly\ Election\ 2017.sqlite \
  //       https://localhost:9444/v1/upload
  it('upload', () => {
    cy.readFile('cypress/test_data/Assembly Election 2017.sqlite', 'binary').then((dbData) => {
      const blob = Cypress.Blob.binaryStringToBlob(dbData)

      // Manually construct a form data object, as cy.request() doesn't yet have proper support
      // for form data
      const z = new FormData()
      z.set('apikey', '2MXwA5jGZkIQ3UNEcKsuDNSPMlx')
      z.set('dbname', 'Assembly Election 2017v2.sqlite')
      z.set('file', blob)

      // Send the request
      cy.request({
        method: 'POST',
        url: 'https://localhost:9444/v1/upload',
        body: z
      }).then(
        (response) => {
          expect(response.status).to.eq(201)

          // For some unknown reason Cypress thinks the response.body is an ArrayBuffer (wtf?), when it's just standard
          // json.  It's *probably* some side effect of using Cypress.Blob.binaryStringToBlob() above, but that seems
          // pretty silly.
          // Anyway, we manually convert it to something that JSON.parse() can operate on, then proceed as per normal
          let fixedBody = Cypress.Blob.arrayBufferToBinaryString(response.body)
          let jsonBody = JSON.parse(fixedBody)

          expect(jsonBody).to.have.keys(['commit', 'url'])
          expect(jsonBody.url).to.match(/.*\/default\/Assembly\ Election\ 2017v2\.sqlite/)
        }
      )
    })
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