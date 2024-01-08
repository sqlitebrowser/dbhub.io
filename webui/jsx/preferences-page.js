const React = require("react");
const ReactDOM = require("react-dom");

import Button from "react-bootstrap/Button";
import Modal from "react-bootstrap/Modal";
import { confirmAlert } from "react-confirm-alert";

import MarkdownEditor from "./markdown-editor";
import { copyToClipboard } from "./clipboard";
import { userPrefTheme, setUserPrefTheme } from "./theme";

export default function PreferencesPage() {
	const [statusMessage, setStatusMessage] = React.useState("");
	const [statusMessageColour, setStatusMessageColour] = React.useState("");

	const [showCreateApiKeyDialog, setShowCreateApiKeyDialog] = React.useState(false);
	const [showNewApiKeyDialog, setShowNewApiKeyDialog] = React.useState(false);
	const [lastNewApiKey, setLastNewApiKey] = React.useState("");
	const [lastNewApiKeyId, setLastNewApiKeyId] = React.useState("");
	const [createApiKeyDialogExpiryEnabled, setCreateApiKeyDialogExpiryEnabled] = React.useState(true);
	const [createApiKeyDialogExpiryDate, setCreateApiKeyDialogExpiryDate] = React.useState((new Date((new Date()).valueOf() + 1000*3600*24*365)).toISOString().split("T")[0]); // 365 days
	const [createApiKeyDialogComment, setCreateApiKeyDialogComment] = React.useState("");

	const [fullName, setFullName] = React.useState(preferences.fullName);
	const [email, setEmail] = React.useState(preferences.email);
	const [maxRows, setMaxRows] = React.useState(preferences.maxRows);
	const [colourTheme, setColourTheme] = React.useState(userPrefTheme());
	const [apiKeys, setApiKeys] = React.useState(preferences.apiKeys || []);

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

			// Save locally stored settings
			setUserPrefTheme(colourTheme);

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
			method: "post",
			headers: {
				"Content-Type": "application/x-www-form-urlencoded"
			},
			body: new URLSearchParams({
				"expiry": createApiKeyDialogExpiryEnabled ? encodeURIComponent(createApiKeyDialogExpiryDate) : "",
				"comment": createApiKeyDialogComment,
			}),
		}).then(response => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			response.json().then(data => {
				// Close create dialog
				setShowCreateApiKeyDialog(false);

				// Store new key
				setLastNewApiKey(data["key"]);
				setLastNewApiKeyId(data["uuid"]);

				// Append key to list
				let keys = apiKeys.slice();
				keys.push(data);
				setApiKeys(keys);

				// Show success dialog
				setShowNewApiKeyDialog(true);
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

	// Deletes an API key
	function deleteApiKey(uuid) {
		confirmAlert({
			title: "Confirm delete",
			message: "Are you sure you want to delete the API key with the ID \"" + uuid + "\"? Access with it will no longer be possible.",
			buttons: [
				{
					label: 'Yes',
					onClick: () => {
						// Send request to server
						fetch("/x/apikeydel", {
							method: "post",
							headers: {
								"Content-Type": "application/x-www-form-urlencoded"
							},
							body: new URLSearchParams({
								"uuid": encodeURIComponent(uuid),
							}),
						}).then(response => {
							if (!response.ok) {
								return Promise.reject(response);
							}

							response.text().then(data => {
								// Remove key from list
								let keys = apiKeys.slice();
								keys.find((o, i) => {
									if (o.uuid === uuid) {
										keys.splice(i, 1);
										return true;
									}
								});
								setApiKeys(keys);
							});
						})
						.catch(error => {
							// Key deletion failed, display the error message
							error.text().then(text => {
								setStatusMessageColour("red");
								setStatusMessage("Deleting the API key failed: " + text);
							});
						});
					},
				},
				{
					label: 'No'
				}
			]
		});
	}

	// Render table of all API keys
	let apiKeysTable = <p><i>You don't have any API keys yet</i></p>;
	if (apiKeys) {
		apiKeysTable = (
			<table className="table table-sm table-hover table-responsive" data-cy="apikeystbl">
				<thead>
					<tr><th>ID</th><th>Generation date</th><th>Expiry date</th><th>Description</th><th></th></tr>
				</thead>
				<tbody>
					{apiKeys.map(row => (
						<tr>
							<td>{row.uuid}</td>
							<td>{new Date(row.date_created).toLocaleString()}</td>
							<td className={row.expiry_date && (new Date() >= new Date(row.expiry_date)) ? "table-warning" : ""}>{row.expiry_date ? Intl.DateTimeFormat().format(new Date(row.expiry_date)) : <i>never</i>}</td>
							<td>{row.comment}</td>
							<td><button type="button" className="btn btn-outline-danger" title="Delete this API key" onClick={() => deleteApiKey(row.uuid)}><span className="fa fa-trash"></span></button></td>
						</tr>
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
			<div className="mb-2">
				<label className="form-label" htmlFor="theme">Colour theme</label>
				<select className="form-select" id="theme" value={colourTheme} onChange={e => setColourTheme(e.target.value)}>
					<option value="light">Light (default)</option>
					<option value="dark">Dark (experimental)</option>
				</select>
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
		<button type="button" className="btn btn-primary mb-2" data-cy="genapibtn" onClick={() => setShowCreateApiKeyDialog(true)}>Generate new API key</button>
		{apiKeysTable}

		<Modal show={showCreateApiKeyDialog} onHide={() => setShowCreateApiKeyDialog(false)}>
			<Modal.Header closeButton>
				<Modal.Title>Generate new API key</Modal.Title>
			</Modal.Header>
			<Modal.Body>
				<p>
					This will create a new API key for your user account. Clicking OK will show the API key. Make sure nobody else sees it. Please save it in a safe and secure location. You won't be able to see or retrieve it at a later time here. You can identify your keys using the value in the ID column.
				</p>
				<form>
					<div className="mb-3">
						<label htmlFor="createapikeyexpiry" className="form-label">Expiry date</label>
						<div className="input-group">
							<div className="input-group-text">
								<input className="form-check-input mt-0" type="checkbox" checked={createApiKeyDialogExpiryEnabled} onChange={() => setCreateApiKeyDialogExpiryEnabled(!createApiKeyDialogExpiryEnabled)} />
							</div>
							<input type="date" className="form-control" id="createapikeyexpiry" required={createApiKeyDialogExpiryEnabled} disabled={!createApiKeyDialogExpiryEnabled} value={createApiKeyDialogExpiryDate} min={(new Date((new Date()).valueOf() + 1000*3600*24)).toISOString().split("T")[0]} onChange={e => setCreateApiKeyDialogExpiryDate(e.target.value)} />
						</div>
						<div className="form-text">We recommend setting an expiry date. The API key will no longer work after that date. This way it provides only limited access to attackers if stolen.</div>
					</div>
					<div className="mb-3">
						<label htmlFor="createapikeydescr" className="form-label">Description</label>
						<input type="text" className="form-control" id="createapikeydescr" value={createApiKeyDialogComment} onChange={e => setCreateApiKeyDialogComment(e.target.value)} />
						<div className="form-text">Optionally you can provide a description of this API key. This can help identifying keys if you have multiple.</div>
					</div>
				</form>
			</Modal.Body>
			<Modal.Footer>
				<Button variant="primary" onClick={() => genApiKey()} data-cy="apiokbtn">OK</Button>
				<Button variant="secondary" onClick={() => setShowCreateApiKeyDialog(false)}>Cancel</Button>
			</Modal.Footer>
		</Modal>
		<Modal show={showNewApiKeyDialog} onHide={() => setShowNewApiKeyDialog(false)}>
			<Modal.Header closeButton>
				<Modal.Title>New API key</Modal.Title>
			</Modal.Header>
			<Modal.Body>
				{"Your API key with the id \"" + lastNewApiKeyId + "\" has been generated. It is:"}
				<div className="input-group">
					<input type="text" className="form-control" value={lastNewApiKey} id="api-key" />
					<button className="btn btn-outline-secondary" type="button" title="Copy key to clipboard" onClick={() => copyToClipboard('api-key')}><span className="fa fa-clipboard"></span></button>
				</div>
			</Modal.Body>
			<Modal.Footer>
				<Button variant="primary" onClick={() => setShowNewApiKeyDialog(false)}>Close</Button>
			</Modal.Footer>
		</Modal>
	</>);
}
