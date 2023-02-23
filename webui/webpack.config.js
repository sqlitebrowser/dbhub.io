const path = require("path");

module.exports = {
	mode: "production",		// Change to "development" for debugging purposes
	entry: "./js/app.js",
	output: {
		filename: "dbhub.js",
		path: path.resolve(__dirname, "js")
	}
};
