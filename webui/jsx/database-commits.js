const React = require("react");
const ReactDOM = require("react-dom");

import Select from "react-dropdown-select";
import {getTimePeriod} from "./format";

function DatabaseCommitRow({data, index, branch, setStatusMessage, setStatusMessageColour}) {
	const [commitIndex, setCommitIndex] = React.useState(Number(index));

	// Bounce to the page for creating branches
	function createBranch() {
		window.location = "/createbranch/" + meta.owner + "/" + meta.database + "?commit=" + data.id;
	}

	// Bounce to the page for creating tags
	function createTag() {
		window.location = "/createtag/" + meta.owner + "/" + meta.database + "?commit=" + data.id;
	}

	// Bounce to the page for viewing changes
	function viewChanges() {
		window.location = "/diffs/" + meta.owner + "/" + meta.database + "?commit_a=" + commitData[commitIndex + 1].id + "&commit_b=" + data.id;
	}

	// Delete a commit from the viewed branch
	function deleteCommit() {
		fetch("/x/deletecommit/", {
			method: "post",
			headers: {
				"Content-Type": "application/x-www-form-urlencoded"
			},
			body: new URLSearchParams({
				"branch": branch,
				"commit": data.id,
				"dbname": meta.database,
				"username": meta.owner
			}),
		}).then((response) => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			window.location = "/commits/" + meta.owner + "/" + meta.database + "?branch=" + branch;
		})
		.catch((error) => {
			// The delete failed, so display the returned error message
			setStatusMessageColour("red");
			setStatusMessage("Error: " + error.text());
		});
	}

	// Is this the last and/or head commit?
	const isHeadCommit = data.id == commitData[0].id;
	const isLastCommit = data.id == commitData[commitData.length - 1].id;

	return (
		<tr>
			<td>
				{meta.owner === authInfo.loggedInUser ? <p><button className="btn btn-primary" onClick={() => createBranch()} data-cy="createbranchbtn">Create Branch</button></p> : null}
				{isLastCommit === false ? <p><button className="btn btn-primary" onClick={() => viewChanges()} data-cy="viewchangesbtn">View Changes</button></p> : null}
				{meta.owner === authInfo.loggedInUser ? <p><button className="btn btn-primary" onClick={() => createTag()} data-cy="createtagrelbtn">Create Tag or Release</button></p> : null}
				{meta.owner === authInfo.loggedInUser && isHeadCommit && !isLastCommit ? <p><button className="btn btn-danger" onClick={() => deleteCommit()} data-cy="delcommitbtn">Delete Commit</button></p> : null}
			</td>
			<td dangerouslySetInnerHTML={{__html: data.message}}>
			</td>
			<td>
				{data.avatar_url !== "" ? <img src={data.avatar_url} height="30" width="30" className="border border-secondary" /> : null}&nbsp;
				<a href={"/" + data.author_user_name}>{data.author_name}</a>
			</td>
			<td>
				<span title={new Date(data.timestamp).toLocaleString()}>{getTimePeriod(data.timestamp, false)}</span>
			</td>
			<td>
				<a href={"/" + meta.owner + "/" + meta.database + "?branch=" + branch + "&commit=" + data.id} data-cy="commitlnk">{data.id.substring(0, 8)}</a>
			</td>
		</tr>
	);
}

export default function DatabaseCommits() {
	const urlParams = new URL(window.location.href).searchParams;

	const [statusMessage, setStatusMessage] = React.useState("");
	const [statusMessageColour, setStatusMessageColour] = React.useState("");
	const [branch, setBranch] = React.useState(urlParams.get("branch") ? urlParams.get("branch") : meta.defaultBranch);

	// Change the branch being viewed
	function changeBranch(branchName) {
		window.location = "/commits/" + meta.owner + "/" + meta.database + "?branch=" + branchName;
	}

	// Prepare branch names
	let branches = [];
	for (const [name, data] of Object.entries(branchData)) {
		branches.push({name: name});
	}

	// Render commit rows
	let rows = [];
	for (const [index, data] of Object.entries(commitData)) {
		rows.push(<DatabaseCommitRow data={data} index={index} branch={branch} />);
	}

	return (<>
		<div className="row">
			<div className="col-md-12 text-center">
				<span data-cy="commithist">Commit history for branch</span>&nbsp;
				<div className="d-inline-block">
					<Select name="branchname" required={true} labelField="name" valueField="name" onChange={(values) => changeBranch(values[0].name)} options={branches} values={[{name: branch}]} />
				</div>
			</div>
		</div>
		{statusMessage !== "" ? (
			<div className="row">
				<div className="col-md-12 text-center mb-2">
					<h6 style={{color: statusMessageColour}}>{statusMessage}</h6>
				</div>
			</div>
		) : null}
		<div className="border border-secondary rounded">
			<table id="contents" className="table table-striped table-responsive m-0">
				<thead>
					<tr>
						<th>Actions</th><th>Message</th><th>Author</th><th>Date</th><th>Commit ID</th>
					</tr>
				</thead>
				<tbody>
					{rows}
				</tbody>
			</table>
		</div>
	</>);
}
