const React = require("react");
const ReactDOM = require("react-dom");

import MarkdownEditor from "./markdown-editor";
import Select from "react-dropdown-select";
import { confirmAlert } from "react-confirm-alert";
import "react-confirm-alert/src/react-confirm-alert.css";

function LicenceEdit() {
	const [branchLicences, setBranchLicences] = React.useState(settingsData.branchLicences);

	// Called when a licence is changed
	function changeLicence(branch, licence) {
		let update = {};
		update[branch] = licence;
		setBranchLicences(branchLicences => ({
			...branchLicences,
			...update
		}));
	}

	// Prepare licence list
	let licences = [];
	for (const [name, data] of Object.entries(settingsData.licences)) {
		licences.push({
			value: name,
			label: name,
			url: data.url,
			order: data.order,
		});
	}
	licences.sort(function(a, b) {
		if (a.order < b.order) {
			return -1;
		} else if(a.order > b.order) {
			return 1;
		} else {
			return 0;
		}
	});

	// Render a table rows per branch
	let tableRows = [];
	for (const [branch, licence] of Object.entries(branchLicences)) {
		tableRows.push(
			<tr>
				<td>
					<div>{branch}</div>
				</td>
				<td>
					<Select
						name={branch + "-licence"}
						required={true}
						onChange={(values) => changeLicence(branch, values[0].value)}
						options={licences}
						values={[{value: licence, label: licence}]}
						backspaceDelete={false}
						itemRenderer={({item, props, state, methods}) => {
							return (
								<div>
									<span onClick={() => methods.addItem(item)}>{item.label}</span>
									{item.url !== '' ? (<><span> - </span><a href={item.url} target="_blank" rel="noopener noreferrer external">info</a></>) : null}
								</div>
							);
						}}
					/>
				</td>
			</tr>
		);
	}

	return (
		<div className="row mb-2">
			<label className="col-sm-2 col-form-label">Licence<div className="form-text">Can be set per branch</div></label>
			<div className="col-sm-10">
				<table className="table table-striped">
					<thead>
						<tr>
							<th>Branch</th><th>Licence</th>
						</tr>
					</thead>
					<tbody>
						{tableRows}
					</tbody>
				</table>
				<input type="hidden" name="licences" value={JSON.stringify(branchLicences)} />
			</div>
		</div>
	);
}

function ShareEdit() {
	const [shares, setShares] = React.useState(settingsData.shares);

	// Update the chosen permissions for the given user
	function changeShare(user, access) {
		let update = {};
		update[user] = access;
		setShares(shares => ({
			...shares,
			...update
		}));
	}

	// Handler for the Add User to shares button
	function addShare() {
		let user_field = document.getElementById("addShareUserName");
		let user = user_field.value;

		// No user name is allowed to appear twice
		if(shares[user] !== undefined)
			return;

		// Only allow adding existing user names
		fetch("/x/checkuserexists?name=" + user)
			.then((response) => response.text())
			.then((text) => {
				if (text === "y") {
					changeShare(user, "r");
					user_field.value = "";
				}
			});
	}

	// Removes a user from the list of shares
	function removeShare(user) {
		let newData = { ...shares };
		delete newData[user];
		setShares(newData);
	}

	// Render a table row per user
	let tableRows = [];
	for (const [user, access] of Object.entries(shares)) {
		tableRows.push(
			<tr>
				<td data-cy={"shareuser-" + user}>
					{user}
				</td>
				<td>
					<Select
						name={"shareperm-" + user}
						required={true}
						onChange={(values) => changeShare(user, values[0].value)}
						options={[{value: "r", label: "Read only"}, {value: "rw", label: "Read and write"}]}
						values={[{value: access, label: (access === "r" ? "Read only" : "Read and write")}]}
						backspaceDelete={false}
					/>
				</td>
				<td>
					<input type="button" className="btn btn-danger btn-sm" onClick={() => removeShare(user)} value="Remove" data-cy={"shareremovebtn-" + user} />
				</td>
			</tr>
		);
	}

	return (
		<div className="row mb-2">
			<label className="col-sm-2 col-form-label">Share Database<div className="form-text">Make private databases visible to other users or give them write access to your databases</div></label>
			<div className="col-sm-10">
				<table className="table table-striped">
					<thead>
						<tr>
							<th>User</th>
							<th>Permissions</th>
							<th>
								<div className="input-group">
									<span className="input-group-text"><i className="fa fa-user"></i></span>
									<input id="addShareUserName" type="text" className="form-control" name="addShareUserName" placeholder="Username" data-cy="usernameinput" />
									<button className="btn btn-light" type="button" onClick={() => addShare()} title="Add User" data-cy="adduserbtn">
										<i className="fa fa-plus"></i>
									</button>
								</div>
							</th>
						</tr>
					</thead>
					<tbody>
						{tableRows}
					</tbody>
				</table>
				<input type="hidden" name="shares" value={JSON.stringify(shares)} />
			</div>
		</div>
	);
}

export default function DatabaseSettings() {
	const [statusMessage, setStatusMessage] = React.useState("");
	const [statusMessageColour, setStatusMessageColour] = React.useState("");

	const [name, setName] = React.useState(meta.database);
	const [oneLineDescription, setOneLineDescription] = React.useState(meta.oneLineDescription);
	const [fullDescription, setFullDescription] = React.useState(meta.fullDescription);
	const [isPublic, setPublic] = React.useState(meta.publicDb);
	const [tableList, setTableList] = React.useState(meta.tableList);
	const [defaultTable, setDefaultTable] = React.useState(meta.defaultTable);
	const [defaultBranch, setDefaultBranch] = React.useState(meta.defaultBranch);
	const [sourceUrl, setSourceUrl] = React.useState(meta.sourceUrl);

	// Handler for the cancel button.  Just bounces back to the database page
	function cancelSettings() {
		window.location = "/" + meta.owner + "/" + meta.database;
	}

	// This function is called when the default branch is changed. It reloads the list of tables in this branch.
	function switchDefaultBranch(newBranch) {
		fetch("/x/tablenames/", {
			method: "post",
			headers: {
				"Content-Type": "application/x-www-form-urlencoded"
			},
			body: new URLSearchParams({
				"branch": newBranch,
				"dbname": meta.database,
				"username": meta.owner,
			})
		}).then((response) => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			return response.json();
		})
		.then((data) => {
			// Update displayed branch value
			setDefaultBranch(newBranch);

			// Update displayed default table values
			setTableList(data.tables);
			setDefaultTable(data.default_table);

			// Reset any displayed error message
			setStatusMessage("");
		})
		.catch((error) => {
			// Retrieving the table names failed, so display an error message
			setStatusMessageColor("red");
			setStatusMessage("Retrieving table names for the branch failed");
		});
	}

	// Delete the database
	function deleteDatabase() {
		fetch("/x/deletedatabase/", {
			method: "post",
			headers: {
				"Content-Type": "application/x-www-form-urlencoded"
			},
			body: new URLSearchParams({
				"dbname": meta.database,
				"username": meta.owner,
			})
		}).then((response) => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			window.location = "/" + meta.owner
		})
		.catch((error) => {
			// The delete failed, so display an error message
			setStatusMessageColor("red");
			setStatusMessage("Could not delete database");
		});
	}

	// Ask user to confirm deleting the database
	function confirmDelete() {
		confirmAlert({
			title: "Confirm delete",
			message: "Are you sure you want to delete '" + meta.owner + "/" + meta.database + "'? There is no \"undo\" if you proceed.",
			buttons: [
				{
					label: "Yes, delete it",
					onClick: () => deleteDatabase()
				},
				{
					label: "Cancel"
				}
			]
		});
	}

	// Convert branch data to format suited for Select component
	let branches = [];
	if (meta.isLive === false) {
		meta.branchList.forEach(function(v) {
			branches.push({name: v});
		});
	}

	// Convert table data to format suited for Select component
	let tables = [];
	tableList.forEach(function(v) {
		tables.push({name: v});
	});

	return (<>
		{statusMessage !== "" ? (
			<div className="row mb-2">
				<div className="col-md-12 text-center mb-2">
					<h6 style={{color: statusMessageColour}}>{statusMessage}</h6>
				</div>
			</div>
		) : null}
		<form action="/x/savesettings" method="post">
			<input type="hidden" name="username" value={meta.owner} />
			<input type="hidden" name="dbname" value={meta.database} />
			<div className="row mb-2">
				<label htmlFor="newname" className="col-sm-2 col-form-label">Name</label>
				<div className="col-sm-10">
					<input id="newname" name="newname" value={name} onChange={(e) => setName(e.target.value)} data-cy="nameinput" className="form-control" required />
				</div>
			</div>
			<div className="row mb-2">
				<label htmlFor="onelinedesc" className="col-sm-2 col-form-label">One line description</label>
				<div className="col-sm-10">
					<input id="onelinedesc" name="onelinedesc" value={oneLineDescription} onChange={(e) => setOneLineDescription(e.target.value)} data-cy="onelinedescinput" className="form-control" />
				</div>
			</div>
			<div className="row mb-2">
				<label className="col-sm-2 col-form-label">Public?</label>
				<div className="col-sm-10">
					<div className="btn-group" role="group">
						<input type="radio" className="btn-check" name="public" autocomplete="off" checked={!isPublic} value="false" />
						<label className="btn btn-outline-secondary" htmlFor="public" onClick={() => setPublic(false)} data-cy="private">Private</label>
						<input type="radio" className="btn-check" name="public" autocomplete="off" checked={isPublic} value="true" />
						<label className="btn btn-outline-secondary" htmlFor="public" onClick={() => setPublic(true)} data-cy="public">Public</label>
					</div>&nbsp;
					{isPublic ? <span>Database will be <b>public</b>. Everyone has read access to it.</span> : <span>Database will be <b>private</b>. Only you have access to it.</span>}
				</div>
			</div>
			<div className="row mb-2">
				<label htmlFor="selectdefaulttable" className="col-sm-2 col-form-label">Default table or view</label>
				<div className="col-sm-10">
					<Select name="selectdefaulttable" required={true} labelField="name" valueField="name" onChange={(values) => setDefaultTable(values[0].name)} options={tables} values={[{name: defaultTable}]} backspaceDelete={false} />
					<input type="hidden" name="defaulttable" value={defaultTable} />
				</div>
			</div>
			{meta.isLive === false ?
				<div className="row mb-2">
					<label htmlFor="selectbranch" className="col-sm-2 col-form-label">Default branch</label>
					<div className="col-sm-10">
						<Select name="selectbranch" required={true} labelField="name" valueField="name" onChange={(values) => switchDefaultBranch(values[0].name)} options={branches} values={[{name: defaultBranch}]} backspaceDelete={false} />
						<input type="hidden" name="branch" value={defaultBranch} />
					</div>
				</div>
			: null}
			<div className="row mb-2">
				<label htmlFor="sourceurl" className="col-sm-2 col-form-label">Source URL</label>
				<div className="col-sm-10">
					<input id="sourceurl" name="sourceurl" value={sourceUrl} onChange={(e) => setSourceUrl(e.target.value)} data-cy="sourceurl" className="form-control" />
				</div>
			</div>
			{meta.isLive === false ? <LicenceEdit /> : null}
			<ShareEdit />
			<div className="row mb-2">
				<label htmlFor="fulldesc" className="col-sm-2 col-form-label">Full length description<div className="form-text">Markdown (<a href="https://commonmark.org" target="_blank" rel="noopener noreferrer external">CommonMark</a> format) is supported</div></label>
				<div className="col-sm-10">
					<MarkdownEditor editorId="fulldesc" rows={18} defaultTab="preview" initialValue={fullDescription} />
				</div>
			</div>
			<div className="row mb-2">
				<div className="col-sm-offset-2 col-sm-10">
					<button type="submit" className="btn btn-success" data-cy="savebtn">Save</button>&nbsp;
					<button type="button" className="btn btn-secondary" data-cy="cancelbtn" onClick={() => cancelSettings()}>Cancel</button>
				</div>
			</div>
		</form>

		<div className="card mt-3">
			<div className="card-header border-danger text-bg-danger bg-opacity-50">
				Destructive options
			</div>
			<div className="card-body">
				<button type="button" className="btn btn-danger" onClick={() => confirmDelete()} data-cy="delbtn">Delete database</button>
			</div>
		</div>
	</>);
}
