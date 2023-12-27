const React = require("react");
const ReactDOM = require("react-dom");

import MarkdownEditor from "./markdown-editor";

export default function PreferencesPage() {
	const [statusMessage, setStatusMessage] = React.useState("");
	const [statusMessageColour, setStatusMessageColour] = React.useState("");

	const [fullName, setFullName] = React.useState(preferences.fullName);
	const [email, setEmail] = React.useState(preferences.email);
	const [maxRows, setMaxRows] = React.useState(preferences.maxRows);
	const [apiKeys, setApiKeys] = React.useState(preferences.apiKeys);

	// Handler for the cancel button.  Just bounces back to the profile page
	function cancel() {
		window.location = "/" + authInfo.loggedInUser;
	}

	// Send changed preferences to the server for saving
	function savePreferences() {
		// Send the preferences
		fetch("/pref", {
			method: "post",
			headers: {
				"Content-Type": "application/x-www-form-urlencoded"
			},
			body: new URLSearchParams({
				"fullname": encodeURIComponent(fullName),
				"email": encodeURIComponent(email),
				"maxrows": encodeURIComponent(maxRows),
			}),
		}).then(response => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			// Saving succeeded
			window.location = "/" + authInfo.loggedInUser;
		})
		.catch(error => {
			// Saving failed, display the error message
			error.text().then(text => {
				setStatusMessageColour("red");
				setStatusMessage("Saving failed: " + text);
			});
		});
	}

	// Generate a new client certificate
	function genCert() {
		window.location = "/x/gencert";
	}

	// Generate a new API key
	function genApiKey() {
		// Call the server to generate a new API key
		fetch("/x/apikeygen", {
			method: "get",
		}).then(response => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			response.json().then(data => {
				// Show success message
				setStatusMessageColour("green");
				setStatusMessage("New API key '" + data["key"] + "' created");

				// Append key to list
				let keys = apiKeys.slice();
				keys.push(data);
				setApiKeys(keys);
			});
		})
		.catch(error => {
			// Key creation failed, display the error message
			error.text().then(text => {
				setStatusMessageColour("red");
				setStatusMessage("Creating new API key failed: " + text);
			});
		});
	}

	// Render table of all API keys
	let apiKeysTable = <i>You don't have any API keys yet</i>;
	if (apiKeys) {
		apiKeysTable = (
			<table className="table table-sm table-striped table-responsive mb-2" data-cy="apikeystbl">
				<thead>
					<tr><th>Key</th><th>Generation date</th></tr>
				</thead>
				<tbody>
					{apiKeys.map(row => (
						<tr><td>{row.key}</td><td>{new Date(row.date_created).toLocaleString()}</td></tr>
					))}
				</tbody>
			</table>
		);
	}

	return (<>
		<h3 className="text-center">Preferences</h3>
		{statusMessage !== "" ? (
			<div className="row">
				<div className="col-md-12 text-center mb-2">
					<h6 data-cy="apistatus" style={{color: statusMessageColour}}>{statusMessage}</h6>
				</div>
			</div>
		) : null}
		<form>
			<h5>Used when uploading databases</h5>
			<div className="mb-2">
				<label className="form-label" htmlFor="fullname">Full Name</label>
				<input type="text" className="form-control" id="fullname" maxlength={80} data-cy="fullname" placeholder="Jane Doe" value={fullName} onChange={e => setFullName(e.target.value)} required />
			</div>
			<div className="mb-2">
				<label className="form-label" htmlFor="email">Email address</label>
				<input type="email" className="form-control" id="email" maxlength={80} data-cy="email" placeholder={authInfo.loggedInUser + "@" + preferences.server} value={email} onChange={e => setEmail(e.target.value)} required />
				<div className="form-text">{"If you don't want to use your real email address, use \"" + authInfo.loggedInUser + "@" + preferences.server + "\"."}</div>
			</div>

			<h5>Display options</h5>
			<div className="mb-2">
				<label className="form-label" htmlFor="maxrows">Maximum number of database rows to display</label>
				<input type="number" className="form-control" id="maxrows" data-cy="numrows" value={maxRows} onChange={e => setMaxRows(e.target.value)} min="1" max="500" required />
			</div>

			<button type="button" className="btn btn-success" data-cy="updatebtn" onClick={() => savePreferences()}>Save</button>&nbsp;
			<button type="button" className="btn btn-secondary" onClick={() => cancel()}>Cancel</button>
		</form>

		<hr />

		<h5><a href="https://sqlitebrowser.org/" target="_blank" rel="noopener noreferrer external">DB4S</a> Integration</h5>
		<div className="form-text">This is needed for easily making changes to your uploaded databases.</div>
		<button type="button" className="btn btn-primary" data-cy="gencertbtn" onClick={() => genCert()}>Generate new client certificate</button>

		<hr />

		<h5><a href="https://api.dbhub.io" target="_blank">API</a> keys</h5>
		{apiKeysTable}
		<button type="button" className="btn btn-primary" data-cy="genapibtn" onClick={() => genApiKey()}>Generate new API key</button>
	</>);
}
