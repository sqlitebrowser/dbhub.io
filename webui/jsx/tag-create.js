const React = require("react");
const ReactDOM = require("react-dom");

import MarkdownEditor from "./markdown-editor";

export default function TagCreate({commit}) {
	const [statusMessage, setStatusMessage] = React.useState("");
	const [statusMessageColour, setStatusMessageColour] = React.useState("");

	const [type, setType] = React.useState("tag");
	const [name, setName] = React.useState("");

	// Handler for the cancel button.  Just bounces back to the commits page
	function cancelCreate() {
		window.location = "/commits/" + meta.owner + "/" + meta.database;
	}

	// Sends the tag creation details
	function createTag() {
		if (authInfo.loggedInUser === "") {
			// User needs to be logged in
			lock.show();
			return;
		}

		// Send the tag creation request
		fetch("/x/createtag", {
			method: "post",
			headers: {
				"Content-Type": "application/x-www-form-urlencoded"
			},
			body: new URLSearchParams({
				"commit": encodeURIComponent(commit),
				"dbname": encodeURIComponent(meta.database),
				"username": encodeURIComponent(meta.owner),
				"tagtype": encodeURIComponent(type),
				"tag": encodeURIComponent(name),
				"tagdesc": encodeURIComponent(document.getElementById("tagdesc").value),
			}),
		}).then(response => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			// Tag creation succeeded.  Bounce to the tags page
			window.location = "/tags/" + meta.owner + "/" + meta.database;
		})
		.catch(error => {
			// Creating the tag failed, so display an error message
			error.text().then(text => {
				setStatusMessageColour("red");
				setStatusMessage("Tag creation failed: " + text);
			});
		});
	}

	return (<>
		<h3 className="text-center">Create new tag or release</h3>
		<h5 className="text-center"><small>on commit {commit.substring(0, 8)}</small></h5>
		{statusMessage !== "" ? (
			<div className="row">
				<div className="col-md-12 text-center mb-2">
					<h6 style={{color: statusMessageColour}}>{statusMessage}</h6>
				</div>
			</div>
		) : null}
		<form>
			<div className="mb-2">
				<label htmlFor="typeselect" className="form-label">Tag or release?</label>
				<div>
					<div className="btn-group" role="group">
						<input type="radio" className="btn-check" name="typeselect" autocomplete="off" checked={type === "tag"} />
						<label className="btn btn-outline-secondary" htmlFor="typeselect" onClick={() => setType("tag")} data-cy="tagradio">Tag</label>
						<input type="radio" className="btn-check" name="typeselect" autocomplete="off" checked={type === "release"} />
						<label className="btn btn-outline-secondary" htmlFor="typeselect" onClick={() => setType("release")} data-cy="relradio">Release</label>
					</div>
					&nbsp;<span>This will be a new <b>{type}</b>.</span>
				</div>
			</div>
			<div className="mb-2">
				<label htmlFor="tag" className="form-label">Name</label>
				<input type="text" className="form-control" id="tag" maxlength={80} data-cy="nameinput" value={name} onChange={e => setName(e.target.value)} required />
			</div>
			<div className="mb-2">
				<label htmlFor="tagdesc" className="form-label">Description</label>
				<MarkdownEditor editorId="tagdesc" rows={10} placeholder="A description for this tag or release" />
			</div>
			<button type="button" className="btn btn-success" onClick={() => createTag()} data-cy="createbtn">Create</button>&nbsp;
			<button type="button" className="btn btn-secondary" onClick={() => cancelCreate()} data-cy="cancelbtn">Cancel</button>
		</form>
	</>);
}
