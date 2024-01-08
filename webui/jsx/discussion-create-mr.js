const React = require("react");
const ReactDOM = require("react-dom");

import MarkdownEditor from "./markdown-editor";
import Select from "react-dropdown-select";
import CommitList from "./commit-list";

export default function DiscussionCreateMr() {
	const [statusMessage, setStatusMessage] = React.useState("");
	const [statusMessageColour, setStatusMessageColour] = React.useState("");

	const [title, setTitle] = React.useState("");
	const [sourceDbName, setSourceDbName] = React.useState(createMrData.sourceDbName);
	const [sourceDbOwner, setSourceDbOwner] = React.useState(createMrData.sourceDbOwner);
	const [sourceBranch, setSourceBranch] = React.useState(createMrData.sourceDbDefaultBranch);
	const [sourceBranches, setSourceBranches] = React.useState(createMrData.sourceBranches);
	const [destDbName, setDestDbName] = React.useState(createMrData.destDbName);
	const [destDbOwner, setDestDbOwner] = React.useState(createMrData.destDbOwner);
	const [destBranch, setDestBranch] = React.useState(createMrData.destDbDefaultBranch);
	const [destBranches, setDestBranches] = React.useState(createMrData.destBranches);
	const [commitList, setCommitList] = React.useState(createMrData.commitList);

	// Updates the commit list showing the difference between source and destination databases
	function updateCommitList() {
		fetch("/x/diffcommitlist/", {
			method: "post",
			headers: {
				"Content-Type": "application/x-www-form-urlencoded"
			},
			body: new URLSearchParams({
				"destbranch": encodeURIComponent(destBranch),
				"destdbname": encodeURIComponent(destDbName),
				"destowner": encodeURIComponent(destDbOwner),
				"sourcebranch": encodeURIComponent(sourceBranch),
				"sourcedbname": encodeURIComponent(sourceDbName),
				"sourceowner": encodeURIComponent(sourceDbOwner),
			}),
		}).then((response) => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			response.json().then(data => {
				// Retrieving the commit list succeeded, so update the displayed commit list
				setCommitList(data.commit_list);

				setStatusMessageColour("green");
				setStatusMessage("");
			});
		})
		.catch((error) => {
			// Retrieving the commit list failed, so clear out the existing displayed list and display a message
			// about it
			setCommitList([]);
			setStatusMessageColour("orange");
			setStatusMessage("The selected source and destination can't be merged.  Please choose a different source and destination.");
		});
	}

	// Update name of the source or destination branch in the drop down selector
	function changeBranch(sourceDest, newBranch) {
		if (sourceDest === "source") {
			setSourceBranch(newBranch);
		} else if(sourceDest === "dest") {
			setDestBranch(newBranch);
		}
	}

	// Updates the source or destination database name and branch list in the drop down selectors
	function changeDb(sourceDest, newRow) {
		// Retrieve the branch list for the newly selected database
		fetch("/x/branchnames/", {
			method: "post",
			headers: {
				"Content-Type": "application/x-www-form-urlencoded"
			},
			body: new URLSearchParams({
				"dbname": newRow.database_name,
				"username": newRow.database_owner,
			}),
		}).then((response) => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			response.json().then(data => {
				// Clear any previous error message
				setStatusMessageColour("green");
				setStatusMessage("");

				// Update the values used when sending the creation request and update the branch list
				if (sourceDest === "source") {
					setSourceDbName(newRow.database_name);
					setSourceDbOwner(newRow.database_owner);

					setSourceBranches(data.branches);
					setSourceBranch(data.default_branch);
				} else if(sourceDest === "dest") {
					setDestDbName(newRow.database_name);
					setDestDbOwner(newRow.database_owner);

					setDestBranches(data.branches);
					setDestBranch(data.default_branch);
				}
			});
		})
		.catch((error) => {
			// Retrieving the branch names failed, so display an error message
			setStatusMessageColour("red");
			setStatusMessage("Retrieving branch names for the database failed.");
		});
	}

	// Handler for the cancel button.  Just bounces back to the database page
	function cancelCreate() {
		window.location = "/" + meta.owner + "/" + meta.database;
	}

	// Sends the merge request creation details, and if successful then bounces to the newly created MR for it
	function createMR() {
		if (authInfo.loggedInUser === "") {
			// User needs to be logged in
			lock.show();
			return;
		}

		// Send the MR creation request
		fetch("/x/createmerge/", {
			method: "post",
			headers: {
				"Content-Type": "application/x-www-form-urlencoded"
			},
			body: new URLSearchParams({
				"desc": encodeURIComponent(document.getElementById("desc").value),
				"destbranch": encodeURIComponent(destBranch),
				"destdbname": encodeURIComponent(destDbName),
				"destowner": encodeURIComponent(destDbOwner),
				"sourcebranch": encodeURIComponent(sourceBranch),
				"sourcedbname": encodeURIComponent(sourceDbName),
				"sourceowner": encodeURIComponent(sourceDbOwner),
				"title": encodeURIComponent(title),
			}),
		}).then((response) => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			response.json().then(data => {
				// MR creation succeeded.  The response should include the MR # we'll bounce to
				window.location = "/merge/" + destDbOwner + "/" + destDbName + "?id=" + data.mr_id;
			});
		})
		.catch((error) => {
			// Creating the MR failed, so display an error message
			error.text().then(text => {
				setStatusMessageColour("red");
				setStatusMessage("Merge Request creation failed: " + text);
			});
		});
	};

	// Update commit list when branches change
	React.useEffect(() => {
		updateCommitList();
	}, [sourceBranch, destBranch]);

	const dbListData = createMrData.forkList.map(f => new Object({name: f.database_owner + "/" + f.database_name, database_owner: f.database_owner, database_name: f.database_name}));
	const sourceBranchListData = sourceBranches.map(b => new Object({name: b}));
	const destBranchListData = destBranches.map(b => new Object({name: b}));

	return (<>
		<h3 className="text-center">Create a Merge Request</h3>
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
				<input type="text" className="form-control" id="title" placeholder="Please fill in a title for the new merge request" maxlength={80} value={title} onChange={e => setTitle(e.target.value)} required />
			</div>
			<div className="mb-2">
				<label className="form-label" htmlFor="sourcedb">Source database</label>
				<Select name="sourcedb" required={true} labelField="name" valueField="name" onChange={(values) => changeDb("source", values[0])} options={dbListData} values={[{name: sourceDbOwner + "/" + sourceDbName}]} />
				<div className="form-text">Where the new data is coming from</div>
			</div>
			<div className="mb-2">
				<label className="form-label" htmlFor="sourcebranch">Source branch</label>
				<Select name="sourcebranch" required={true} labelField="name" valueField="name" onChange={(values) => setSourceBranch(values[0].name)} options={sourceBranchListData} values={[{name: sourceBranch}]} />
				<div className="form-text">The branch in the source database to use</div>
			</div>
			<div className="mb-2">
				<label className="form-label" htmlFor="destdb">Destination database</label>
				<Select name="destdb" required={true} labelField="name" valueField="name" onChange={(values) => changeDb("dest", values[0])} options={dbListData} values={[{name: destDbOwner + "/" + destDbName}]} />
				<div className="form-text">Where you'd like the data merged into</div>
			</div>
			<div className="mb-2">
				<label className="form-label" htmlFor="destbranch">Destination branch</label>
				<Select name="destbranch" required={true} labelField="name" valueField="name" onChange={(values) => setDestBranch(values[0].name)} options={destBranchListData} values={[{name: destBranch}]} />
				<div className="form-text">The target branch in the destination database</div>
			</div>
			<div className="mb-2">
				<label className="form-label" htmlFor="desc">Description</label>
				<MarkdownEditor editorId="desc" rows={10} placeholder="Please add a summary for this merge request, describing what the new or changed data is for" />
				<div className="form-text">The purpose of this merge request</div>
			</div>
			<button type="button" className="btn btn-success" onClick={() => createMR()}>Create</button>&nbsp;
			<button type="button" className="btn btn-secondary" onClick={() => cancelCreate()}>Cancel</button>
		</form>
		<div className="card mt-3">
			<div className="card-header">
				Changes between the source and destination
			</div>
			<div className="card-body">
				<CommitList commits={commitList} owner={sourceDbOwner} database={sourceDbName} />
			</div>
		</div>
	</>);
}
