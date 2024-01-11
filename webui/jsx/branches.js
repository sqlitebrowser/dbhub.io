const React = require("react");
const ReactDOM = require("react-dom");

import MarkdownEditor from "./markdown-editor";

function BranchesTableRow({name, commit, description, setStatus}) {
	// This is the branch name currently shown in the front end
	const [branchName, setName] = React.useState(name);

	// This is the branch name currently saved in the database on the server
	const [savedBranchName, setSavedBranchName] = React.useState(name);

	function updateBranch() {
		let newDesc = document.getElementById(name + "_desc").value;
		let newName = document.getElementById(name + "_name").value;
		fetch("/x/updatebranch/", {
			method: "post",
			headers: {
				"Content-Type": "application/x-www-form-urlencoded"
			},
			body: new URLSearchParams({
				"branch": savedBranchName,
				"dbname": meta.database,
				"username": meta.owner,
				"newdesc": newDesc,
				"newbranch": newName
			})
		}).then((response) => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			setSavedBranchName(newName);
			setStatus("green", "Branch updated");
		})
		.catch((error) => {
			// The delete failed, so display an error message
			setStatus("red", "Branch update failed");
		});
	}

	function setDefaultBranch() {
		fetch("/x/setdefaultbranch/", {
			method: "post",
	                headers: {
				"Content-Type": "application/x-www-form-urlencoded"
			},
			body: new URLSearchParams({
				"branch": name,
				"dbname": meta.database,
				"username": meta.owner
			})
		}).then(function (response) {
			// If successful, reload the page
			if (response.status === 200) {
				window.location = "/branches/" + meta.owner + "/" + meta.database;
			}
		});
	}

	function viewChanges() {
		window.location = "/diffs/" + meta.owner + "/" + meta.database + "?commit_a=" + branchData[defaultBranch].commit + "&commit_b=" + commit;
	}

	function deleteBranch() {
		fetch("/x/deletebranch/", {
			method: "post",
	                headers: {
				"Content-Type": "application/x-www-form-urlencoded"
			},
			body: new URLSearchParams({
				"branch": name,
				"dbname": meta.database,
				"username": meta.owner
			}),
		}).then((response) => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			window.location = "/branches/" + meta.owner + "/" + meta.database;
		})
		.catch((error) => {
			// The delete failed, so display an error message
			setStatus("red", "Error: Something went wrong when trying to delete the branch.");
		});
	}

	return (<>
		<tr>
			<td>
				{authInfo.loggedInUser === meta.owner ? <button className="btn btn-primary" onClick={() => {return updateBranch()}} data-cy="savebtn">Save Changes</button> : null}
			</td>
			<td>
				<div>
					{authInfo.loggedInUser === meta.owner && name !== defaultBranch ? <button className="btn btn-primary" onClick={() => {return setDefaultBranch()}} data-cy="setdefaultbtn">Set Default</button> : null }
					{name === defaultBranch ? <i>Default branch</i> : null}
				</div>
			</td>
			<td>
				{authInfo.loggedInUser === meta.owner ?
					<input className="form-control" name={name + "_name"} id={name + "_name"} size="20" maxlength="20" value={branchName} data-cy="nameinput" onChange={(e) => setName(e.target.value)} required />
				:
					<a href={"/" + meta.owner + "/" + meta.database + "?branch=" + name}>{name}</a>
				}
			</td>
			<td>
				<a href={"/" + meta.owner + "/" + meta.database + "?branch=" + name + "&commit=" + commit} data-cy="commitlnk">{commit.substring(0, 8)}</a>
			</td>
		</tr>
		<tr>
			<td>
				{name !== defaultBranch ? <button className="btn btn-info me-1" onClick={() => {return viewChanges()}} data-cy="comparebtn">{"Compare with " + defaultBranch}</button> : null}
				{authInfo.loggedInUser === meta.owner ? <button className="btn btn-danger" onClick={() => {return deleteBranch()}} data-cy="delbtn">Delete</button> : null}
			</td>
			<td colSpan={3}>
				<MarkdownEditor editorId={name + "_desc"} rows={10} placeholder="A description for this branch" defaultTab="preview" initialValue={description} viewOnly={meta.owner !== authInfo.loggedInUser} />
			</td>
		</tr>
	</>);
}

export default function BranchesTable() {
	const [statusMessage, setStatusMessage] = React.useState("");
	const [statusMessageColour, setStatusMessageColour] = React.useState("");

	// Reorder the branches with the default branch first, then the remainder alphabetically
	let rows = [];
        Object.keys(branchData)
            .sort(function (a, b) {
		if ((a > b && a !== defaultBranch) || b === defaultBranch) {
                    return 1;
		} else if ((a < b && b !== defaultBranch) || a === defaultBranch) {
                    return -1;
                } else {
                    return 0;
                }
            }).forEach(function(i, v) {
                let branch = branchData[i];
		rows.push(BranchesTableRow({
			name: i,
			commit: branch["commit"],
			description: branch["description"],
			setStatus: function(colour, text) {
				setStatusMessage(text);
				setStatusMessageColour(colour);
			}
		}));
            });

	return (
		<div>
			{statusMessage !== "" ? (
				<div className="row">
					<div className="col-md-12 text-center mb-2">
						<h6 style={{color: statusMessageColour}}>&nbsp;{statusMessage}</h6>
					</div>
				</div>
			) : null}
			<div className="row">
				<div className="col-md-12">
					<div className="border border-secondary rounded">
						<table id="contents" className="table table-striped table-responsive table-borderless m-0">
							<thead>
								<tr>
									<th colSpan={2}>Actions</th><th>Name</th><th>Head Commit ID</th>
								</tr>
							</thead>
							<tbody>
								{rows}
							</tbody>
						</table>
					</div>
				</div>
			</div>
		</div>
	);
}
