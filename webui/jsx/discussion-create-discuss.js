const React = require("react");
const ReactDOM = require("react-dom");

import MarkdownEditor from "./markdown-editor";

export default function DiscussionCreateDiscuss() {
	const [statusMessage, setStatusMessage] = React.useState("");
	const [statusMessageColour, setStatusMessageColour] = React.useState("");

	const [title, setTitle] = React.useState("");

	// Handler for the cancel button.  Just bounces back to the discussions page
	function cancelCreate() {
		window.location = "/discuss/" + meta.owner + "/" + meta.database;
	}

	// Sends the discussion creation details, and if successful then bounces to the newly created discussion for it
	function createDiscuss() {
		if (authInfo.loggedInUser === "") {
			// User needs to be logged in
			lock.show();
			return;
		}

		// Send the MR creation request
		fetch("/x/creatediscuss/", {
			method: "post",
			headers: {
				"Content-Type": "application/x-www-form-urlencoded"
			},
			body: new URLSearchParams({
				"title": encodeURIComponent(title),
				"disctxt": encodeURIComponent(document.getElementById("disctxt").value),
				"dbname": encodeURIComponent(meta.database),
				"username": encodeURIComponent(meta.owner),
			}),
		}).then(response => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			response.json().then(data => {
				// Discussion creation succeeded.  The response should include the discussion # we'll bounce to
				window.location = "/discuss/" + meta.owner + "/" + meta.database + "?id=" + data.discuss_id;
			});
		})
		.catch(error => {
			// Creating the discussion failed, so display an error message
			error.text().then(text => {
				setStatusMessageColour("red");
				setStatusMessage("Creating the discussion failed: " + text);
			});
		});
	};

	return (<>
		<h3 className="text-center">Create a new discussion</h3>
		{statusMessage !== "" ? (
			<div className="row">
				<div className="col-md-12 text-center mb-2">
					<h6 style={{color: statusMessageColour}}>{statusMessage}</h6>
				</div>
			</div>
		) : null}
		<form>
			<div className="mb-2">
				<label className="form-label" htmlFor="title">Title</label>
				<input type="text" className="form-control" id="title" maxlength={80} value={title} onChange={e => setTitle(e.target.value)} required />
			</div>
			<div className="mb-2">
				<label htmlFor="disctxt" className="form-label">Description<div className="form-text">Markdown (<a href="https://commonmark.org" target="_blank" rel="noopener noreferrer external">CommonMark</a> format) is supported</div></label>
				<MarkdownEditor editorId="disctxt" rows={10} placeholder="Type in the discussion details then click the Create button" />
			</div>
			<button type="button" className="btn btn-success" onClick={() => createDiscuss()}>Create</button>&nbsp;
			<button type="button" className="btn btn-secondary" onClick={() => cancelCreate()}>Cancel</button>
		</form>
	</>);
}
