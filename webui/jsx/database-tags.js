const React = require("react");
const ReactDOM = require("react-dom");

import MarkdownEditor from "./markdown-editor";
import {getTimePeriod} from "./format";

function DatabaseTagRow({name, data, releases, setStatusMessage, setStatusMessageColour}) {
	// This is the tag name currently shown in the front end
	const [tagName, setTagName] = React.useState(name);

	// This is the tag name currently saved in the database on the server
	const [savedTagName, setSavedTagName] = React.useState(name);

	// Delete the tag
        function deleteTag() {
		fetch(releases ? "/x/deleterelease/" : "/x/deletetag/" , {
			method: "post",
	                headers: {
				"Content-Type": "application/x-www-form-urlencoded"
			},
			body: new URLSearchParams({
				"tag": savedTagName,
				"dbname": meta.database,
				"username": meta.owner
			}),
		}).then((response) => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			window.location = "/" + (releases ? "releases" : "tags") + "/" + meta.owner + "/" + meta.database;
		})
		.catch((error) => {
			// The delete failed, so display an error message
			setStatusMessageColour("red");
			setStatusMessage("Error: Something went wrong when trying to delete.");
		});
        }

	// Send the update details to the server
        function updateTag() {
		let newDesc = document.getElementById(savedTagName + "_desc").value;

		fetch(releases ? "/x/updaterelease/" : "/x/updatetag/" , {
			method: "post",
	                headers: {
				"Content-Type": "application/x-www-form-urlencoded"
			},
			body: new URLSearchParams({
				"tag": savedTagName,
				"dbname": meta.database,
				"username": meta.owner,
				"newmsg": newDesc,
				"newtag": tagName,
			}),
		}).then((response) => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			setSavedTagName(tagName);
			setStatusMessageColour("green");
			setStatusMessage(releases ? "Release updated" : "Tag updated");
		})
		.catch((error) => {
			// The delete failed, so display an error message
			setStatusMessageColour("red");
			setStatusMessage(releases ? "Release update failed" : "Tag update failed");
		});
        }

	let actionsCol = null;
	if (meta.owner === authInfo.loggedInUser) {
		actionsCol = (
			<td>
				<p><button className="btn btn-primary" onClick={() => updateTag()} data-cy="updatebtn">Update</button></p>
				<p><button className="btn btn-danger" onClick={() => deleteTag()} data-cy="delbtn">Delete</button></p>
			</td>
		);
	}

	let nameCol = null;
	if (meta.owner === authInfo.loggedInUser) {
		nameCol = (
			<td>
				<input name={savedTagName + "_name"} className="form-control" id={savedTagName + "_name"} size="20" maxlength="20" value={tagName} onChange={(e) => setTagName(e.target.value)} data-cy="nameinput" />
			</td>
		);
	} else {
		nameCol = (
			<td>
				<a href={"/" + meta.owner + "/" + meta.database + (releases ? "?release=" : "?tag=") + savedTagName}>{tagName}</a>
			</td>
		);
	}

	return (
		<tr>
			<td>
				{releases ? <>
					<a href={"/x/download/" + meta.owner + "/" + meta.database + "?commit=" + data.commit} className="btn btn-success">Download</a>
					<p>{Math.round(data.size / 1024).toLocaleString()} KB</p>
				</> : null}
			</td>
			{actionsCol}
			{nameCol}
			<td>
				<MarkdownEditor editorId={savedTagName + "_desc"} rows={10} placeholder={"A description for this " + (releases ? "release" : "tag")} defaultTab="preview" initialValue={data.description} viewOnly={meta.owner !== authInfo.loggedInUser} />
			</td>
			<td>
				{data.avatar_url !== "" ? <img src={data.avatar_url} height="28" width="28" className="border border-secondary" /> : null}&nbsp;
				<a href={"/" + data.tagger_user_name} data-cy="taggerlnk">{data.tagger_display_name}</a>
			</td>
			<td>
				<span title={new Date(data.date).toLocaleString()}>{getTimePeriod(data.date, false)}</span>
			</td>
			<td>
				<a href={"/" + meta.owner + "/" + meta.database + "?commit=" + data.commit} data-cy="commitlnk">{data.commit.substring(0, 8)}</a>
			</td>
		</tr>
	);
}

export default function DatabaseTags({releases}) {
	const [statusMessage, setStatusMessage] = React.useState("");
	const [statusMessageColour, setStatusMessageColour] = React.useState("");

	let rows = [];
	for (const [name, data] of Object.entries(tagsData)) {
		rows.push(<DatabaseTagRow name={name} data={data} releases={releases} setStatusMessage={setStatusMessage} setStatusMessageColour={setStatusMessageColour} />);
	}

	if (rows.length === 0) {
		return <h4 data-cy="notagstxt" className="text-center">This database does not have any {releases ? "releases" : "tags"} yet</h4>;
	}

	return (<>
		{statusMessage !== "" ? (
			<div className="row">
				<div className="col-md-12 text-center mb-2">
					<h6 style={{color: statusMessageColour}}>{statusMessage}</h6>
				</div>
			</div>
		) : null}
		<table id="contents" className="table table-striped table-responsive">
			<thead>
				<tr>
					<th></th>
					{meta.owner === authInfo.loggedInUser ? <th>Actions</th> : null}
					<th>Name</th><th>Description</th><th>Creator</th><th>Creation date</th><th>Commit ID</th>
				</tr>
			</thead>
			<tbody>
				{rows}
			</tbody>
		</table>
	</>);
}
