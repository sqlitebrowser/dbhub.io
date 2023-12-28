const React = require("react");
const ReactDOM = require("react-dom");

import MarkdownEditor from "./markdown-editor";

export default function DatabaseCreateBranch({commit}) {
	const [statusMessage, setStatusMessage] = React.useState("");
	const [statusMessageColour, setStatusMessageColour] = React.useState("");

	const [name, setName] = React.useState("");

	// Handler for the cancel button.  Just bounces back to the commits page
	function cancelCreate() {
		window.location = "/commits/" + meta.owner + "/" + meta.database;
	}

	// Sends the branch creation details
	function createBranch() {
		if (authInfo.loggedInUser === "") {
			// User needs to be logged in
			lock.show();
			return;
		}

		// Send the branch creation request
		fetch("/x/createbranch", {
			method: "post",
			headers: {
				"Content-Type": "application/x-www-form-urlencoded"
			},
			body: new URLSearchParams({
				"commit": encodeURIComponent(commit),
				"dbname": encodeURIComponent(meta.database),
				"username": encodeURIComponent(meta.owner),
				"branch": encodeURIComponent(name),
				"branchdesc": encodeURIComponent(document.getElementById("branchdesc").value),
			}),
		}).then(response => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			// Branch creation succeeded.  Bounce to the branch page
			window.location = "/branches/" + meta.owner + "/" + meta.database;
		})
		.catch(error => {
			// Creating the branch failed, so display an error message
			error.text().then(text => {
				setStatusMessageColour("red");
				setStatusMessage("Branch creation failed: " + text);
			});
		});
	}

	return (<>
		<h3 className="text-center" data-cy="createbranch">Create new branch</h3>
		<h5 className="text-center"><small>from commit {commit.substring(0, 8)}</small></h5>
		{statusMessage !== "" ? (
			<div className="row">
				<div className="col-md-12 text-center mb-2">
					<h6 style={{color: statusMessageColour}}>{statusMessage}</h6>
				</div>
			</div>
		) : null}
		<form>
			<div className="mb-2">
				<label htmlFor="branch" className="form-label">Name for the new branch</label>
				<input type="text" className="form-control" id="branch" maxlength={80} data-cy="nameinput" value={name} onChange={e => setName(e.target.value)} required />
			</div>
			<div className="mb-2">
				<label htmlFor="branchdesc" className="form-label">Branch description<div className="form-text">Markdown (<a href="https://commonmark.org" target="_blank" rel="noopener noreferrer external">CommonMark</a> format) is supported</div></label>
				<MarkdownEditor editorId="branchdesc" rows={10} placeholder="A description for this branch" />
			</div>
			<button type="button" className="btn btn-success" onClick={() => createBranch()} data-cy="createbtn">Create</button>&nbsp;
			<button type="button" className="btn btn-secondary" onClick={() => cancelCreate()} data-cy="cancelbtn">Cancel</button>
		</form>
	</>);
}
