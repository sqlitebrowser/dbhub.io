const { defineConfig } = require('cypress');
const fs = require('fs');

module.exports = defineConfig({
  e2e: {
    setupNodeEvents(on, config) {
      on('task', {
        rmFile({path}) {
          if (fs.existsSync(path)) {
            fs.rmSync(path)
          }
          return null
        },
      })
    },
    baseUrl: 'https://localhost:9443',
  },
});
