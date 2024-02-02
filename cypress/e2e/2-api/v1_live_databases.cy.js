import path from "path";

describe('live databases', () => {
  before(() => {
    // Seed data
    cy.request('/x/test/seed')
  })

  // Columns
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" \
  //       -F dbowner="default" \
  //       -F dbname="Join Testing with index.sqlite" \
  //       -F table="table1" https://localhost:9444/v1/columns
  it('columns', () => {
    cy.request({
      method: 'POST',
      url: 'https://localhost:9444/v1/columns',
      form: true,
      body: {
        apikey: '2MXwA5jGZkIQ3UNEcKsuDNSPMlx',
        dbowner: 'default',
        dbname: 'Join Testing with index.sqlite',
        table: 'table1'
      },
    }).then(
      (response) => {
        expect(response.status).to.eq(200)
        let jsonBody = response.body
        expect(jsonBody[0]).to.include.keys(['column_id', 'default_value', 'name', 'not_null'])
      }
    )
  })

  // Columns (wrong table name)
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" \
  //       -F dbowner="default" \
  //       -F dbname="Join Testing with index.sqlite" \
  //       -F table="some_table_that_doesnt_exist" https://localhost:9444/v1/columns
  it('columns (wrong table name)', () => {
    cy.request({
      method: 'POST',
      url: 'https://localhost:9444/v1/columns',
      form: true,
      body: {
        apikey: '2MXwA5jGZkIQ3UNEcKsuDNSPMlx',
        dbowner: 'default',
        dbname: 'Join Testing with index.sqlite',
        table: 'some_table_that_doesnt_exist'
      },
      failOnStatusCode: false,
    }).then(
      (response) => {
        expect(response.status).to.eq(400)
      }
    )
  })

  // Upload live database
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" -F dbname="LIVE database upload testing.sqlite" \
  //       -F file=@"cypress/test_data/Join Testing with index.sqlite" -F live="true" https://localhost:9444/v1/upload
  it('upload', () => {
    cy.readFile('cypress/test_data/Join Testing with index.sqlite', 'binary').then((dbData) => {
      const blob = Cypress.Blob.binaryStringToBlob(dbData)

      // Manually construct a form data object, as cy.request() doesn't yet have proper support
      // for form data
      const z = new FormData()
      z.set('apikey', '2MXwA5jGZkIQ3UNEcKsuDNSPMlx')
      z.set('dbname', 'LIVE database upload testing.sqlite')
      z.set('live', 'true')
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
          expect(jsonBody.url).to.equal('https://docker-dev.dbhub.io:9444/default/LIVE database upload testing.sqlite')
        }
      )
    })
  })

  // Upload over existing live database
  //   eg: re-run the same upload command above
  it('upload to existing database name', () => {
    cy.readFile('cypress/test_data/Join Testing with index.sqlite', 'binary').then((dbData) => {
      const blob = Cypress.Blob.binaryStringToBlob(dbData)

      // Manually construct a form data object, as cy.request() doesn't yet have proper support
      // for form data
      const z = new FormData()
      z.set('apikey', '2MXwA5jGZkIQ3UNEcKsuDNSPMlx')
      z.set('dbname', 'LIVE database upload testing.sqlite')
      z.set('live', 'true')
      z.set('file', blob)

      // Send the request
      cy.request({
        method: 'POST',
        url: 'https://localhost:9444/v1/upload',
        failOnStatusCode: false,
        body: z
      }).then(
        (response) => {
          expect(response.status).to.eq(409)
        }
      )
    })
  })

  // Download
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" \
  //       -F dbowner="default"  -F dbname="Join Testing with index.sqlite" \
  //       https://localhost:9444/v1/download
  const downloadsFolder = Cypress.config('downloadsFolder')
  it('download', () => {
    cy.request({
      method: 'POST',
      url: 'https://localhost:9444/v1/download',
      form: true,
      encoding: "binary",
      body: {
        apikey: '2MXwA5jGZkIQ3UNEcKsuDNSPMlx',
        dbowner: 'default',
        dbname: 'Join Testing with index.sqlite',
      },
    }).then(
      (response) => {
        expect(response.status).to.eq(200)

        // Save the database to local disk
        const db = path.join(downloadsFolder, 'Join Testing with index.sqlite')
        cy.writeFile(db, response.body, 'binary')

        // Verify the downloaded file is ok
        cy.readFile(db, 'binary', { timeout: 5000 }).should('have.length', 16384)

        // Remove the downloaded file
        cy.task('rmFile', { path: db })
      }
    )
  })

  // Databases
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" \
  //       -F live="true" https://localhost:9444/v1/databases
  it('databases', () => {
    cy.request({
      method: 'POST',
      url: 'https://localhost:9444/v1/databases',
      form: true,
      body: {
        apikey: '2MXwA5jGZkIQ3UNEcKsuDNSPMlx',
        live: 'true'
      },
    }).then(
      (response) => {
        expect(response.status).to.eq(200)
        let jsonBody = response.body
        expect(jsonBody).to.include.members(["LIVE database upload testing.sqlite", 'Join Testing with index.sqlite'])
      }
    )
  })

  // Execute
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" \
  //       -F dbowner="default" -F dbname="Join Testing with index.sqlite" \
  //       -F sql="VVBEQVRFIHRhYmxlMSBTRVQgTmFtZSA9ICdUZXN0aW5nIDEnIFdIRVJFIGlkID0gMQ==" \
  //       https://localhost:9444/v1/execute
  //     Note, the base64 encoded SQL query above is: 'UPDATE table1 SET Name = 'Testing 1' WHERE id = 1'
  it('execute', () => {
    cy.request({
      method: 'POST',
      url: 'https://localhost:9444/v1/execute',
      form: true,
      body: {
        apikey: '2MXwA5jGZkIQ3UNEcKsuDNSPMlx',
        dbowner: 'default',
        dbname: 'Join Testing with index.sqlite',
        sql: btoa('UPDATE table1 SET Name = \'Testing 1\' WHERE id = 1')
      },
    }).then(
      (response) => {
        expect(response.status).to.eq(200)
        let jsonBody = response.body
        expect(jsonBody).to.have.property('rows_changed', 1)
        expect(jsonBody).to.have.property('status', 'OK')

        // Run a follow up query on the database, to verify the change definitely took effect
        cy.request({
          method: 'POST',
          url: 'https://localhost:9444/v1/query',
          form: true,
          body: {
            apikey: '2MXwA5jGZkIQ3UNEcKsuDNSPMlx',
            dbowner: 'default',
            dbname: 'Join Testing with index.sqlite',
            sql: btoa('SELECT Name FROM table1 WHERE id = 1')
          },
        }).then(
          (response) => {
            expect(response.status).to.eq(200)
            let jsonBody = response.body
            expect(jsonBody[0][0]).to.have.property('Name', 'Name')
            expect(jsonBody[0][0]).to.have.property('Type', 3)
            expect(jsonBody[0][0]).to.have.property('Value', 'Testing 1')
          }
        )
      }
    )
  })

  // Indexes
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" \
  //       -F dbowner="default"  -F dbname="Join Testing with index.sqlite" \
  //       https://localhost:9444/v1/indexes
  it('indexes', () => {
    cy.request({
      method: 'POST',
      url: 'https://localhost:9444/v1/indexes',
      form: true,
      body: {
        apikey: '2MXwA5jGZkIQ3UNEcKsuDNSPMlx',
        dbowner: 'default',
        dbname: 'Join Testing with index.sqlite'
      },
    }).then(
      (response) => {
        expect(response.status).to.eq(200)

        // Needs an extra step, due to the structure of the returned JSON
        let temp = response.body

        let jsonBody = temp[0]
        expect(jsonBody).to.have.property('name', 'stuff')
        expect(jsonBody).to.have.property('table', 'table1')

        let columns = jsonBody.columns[0]
        expect(columns).to.have.property('id', 0)
        expect(columns).to.have.property('name', 'id')
      }
    )
  })

  // Query
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" \
  //       -F dbowner="default"  -F dbname="Join Testing with index.sqlite" \
  //       -F sql="U0VMRUNUIHRhYmxlMS5OYW1lLCB0YWJsZTIudmFsdWUgRlJPTSB0YWJsZTEgSk9JTiB0YWJsZTIgVVNJTkcgKGlkKSBPUkRFUiBCWSB0YWJsZTEuaWQ=" \
  //       https://localhost:9444/v1/query
  //     Note, the base64 encoded SQL query above is:
  //       'SELECT table1.Name, table2.value FROM table1 JOIN table2 USING (id) ORDER BY table1.id'
  it('query', () => {
    cy.request({
      method: 'POST',
      url: 'https://localhost:9444/v1/query',
      form: true,
      body: {
        apikey: '2MXwA5jGZkIQ3UNEcKsuDNSPMlx',
        dbowner: 'default',
        dbname: 'Join Testing with index.sqlite',
        sql: btoa('SELECT table1.Name, table2.value FROM table1 JOIN table2 USING (id) ORDER BY table1.id')
      },
    }).then(
      (response) => {
        expect(response.status).to.eq(200)
        let jsonBody = response.body
        expect(jsonBody[2][0]).to.have.property('Name', 'Name')
        expect(jsonBody[2][0]).to.have.property('Type', 3)
        expect(jsonBody[2][0]).to.have.property('Value', 'Baz')
        expect(jsonBody[2][1]).to.have.property('Name', 'value')
        expect(jsonBody[2][1]).to.have.property('Type', 4)
        expect(jsonBody[2][1]).to.have.property('Value', '15')
      }
    )
  })

  // Tables
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" \
  //       -F dbowner="default" -F dbname="Join Testing with index.sqlite" \
  //       https://localhost:9444/v1/tables
  it('tables', () => {
    cy.request({
      method: 'POST',
      url: 'https://localhost:9444/v1/tables',
      form: true,
      body: {
        apikey: '2MXwA5jGZkIQ3UNEcKsuDNSPMlx',
        dbowner: 'default',
        dbname: 'Join Testing with index.sqlite'
      },
    }).then(
      (response) => {
        expect(response.status).to.eq(200)
        let jsonBody = response.body
        expect(jsonBody).to.have.members([
            "table1",
            "table2"
          ]
        )
      }
    )
  })

  // Delete
  //   Equivalent curl command:
  //     curl -k -F apikey="2MXwA5jGZkIQ3UNEcKsuDNSPMlx" \
  //       -F dbname="LIVE database upload testing.sqlite" \
  //       https://localhost:9444/v1/delete
  it('delete', () => {
    cy.request({
      method: 'POST',
      url: 'https://localhost:9444/v1/delete',
      form: true,
      body: {
        apikey: '2MXwA5jGZkIQ3UNEcKsuDNSPMlx',
        dbname: 'LIVE database upload testing.sqlite',
      },
    }).then(
      (response) => {
        expect(response.status).to.eq(200)
        let jsonBody = response.body
        expect(jsonBody).to.have.property('status', 'OK')

        // Verify the database is no longer present
        cy.request({
          method: 'POST',
          url: 'https://localhost:9444/v1/databases',
          form: true,
          body: {
            apikey: '2MXwA5jGZkIQ3UNEcKsuDNSPMlx',
            live: 'true'
          },
        }).then(
          (response) => {
            expect(response.status).to.eq(200)
            let jsonBody = response.body
            expect(jsonBody).to.not.include.members(['LIVE database upload testing.sqlite'])
            expect(jsonBody).to.include.members(['Join Testing with index.sqlite'])
          }
        )
      }
    )
  })
})
