const React = require("react");
const ReactDOM = require("react-dom");

import MarkdownEditor from "./markdown-editor";
import Select from "react-dropdown-select";

export default function UploadForm({branch}) {
	const [statusMessage, setStatusMessage] = React.useState("");
	const [statusMessageColour, setStatusMessageColour] = React.useState("");

	const [isExpanded, setExpanded] = React.useState(false);

	const [live, setLive] = React.useState(false);
	const [isPublic, setPublic] = React.useState(meta.publicDb);
	const [licence, setLicence] = React.useState("Not specified");
	const [branchName, setBranchName] = React.useState(branch);
	const [sourceUrl, setSourceUrl] = React.useState("");
	const [commitMsg, setCommitMsg] = React.useState("");

	// Upload the selected database with the specified settings
	function uploadDatabase() {
		if (authInfo.loggedInUser === "") {
			// User needs to be logged in
			lock.show();
			return;
		}

		// Send the upload request
		const formData = new FormData();
		formData.append("database", document.getElementById("database").files[0]);
		formData.append("username", meta.owner);
		formData.append("dbname", meta.database);
		formData.append("live", live);
		formData.append("public", isPublic);
		formData.append("licence", licence);
		formData.append("commitmsg", commitMsg);
		formData.append("sourceurl", sourceUrl);
		formData.append("branch", branchName);

		fetch("/x/uploaddata/", {
			method: "post",
			body: formData,
		}).then(response => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			// Upload succeeded.  Bounce to the database page
			window.location = "/" + (meta.owner !== "" ? meta.owner : authInfo.loggedInUser) + "/" + (meta.database !== "" ? meta.database : document.getElementById("database").files[0].name);
		})
		.catch(error => {
			// Uploading the database failed, so display an error message
			error.text().then(text => {
				setStatusMessageColour("red");
				setStatusMessage("Upload failed: " + text);
			});
		});
	}

	// Prepare licence list
	let licences = [];
	for (const [name, data] of Object.entries(uploadFormData.licences)) {
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

	return (<>
		<h3 data-cy="uptitle" className="text-center">{meta.owner !== "" && meta.database !== "" ? "Upload new commit" : "Upload a new database"}</h3>
		{statusMessage !== "" ? (
			<div className="row">
				<div className="col-md-12 text-center mb-2">
					<h6 style={{color: statusMessageColour}}>{statusMessage}</h6>
				</div>
			</div>
		) : null}
		<form>
			<div className="mb-2">
				<label className="form-label" htmlFor="database">Database file</label>
				<input className="form-control" type="file" id="database" name="database" data-cy="dbfile" />
			</div>

			{meta.owner !== "" && meta.database !== "" ? <p><b>
				As a new commit into the <a href={"/" + meta.owner} data-cy="ownerlabel">{meta.owner}</a> /&nbsp;
				<a href={"/" + meta.owner + "/" + meta.database} data-cy="dbnamelabel">{meta.database}</a> database,
				in the <a href={"/" + meta.owner + "/" + meta.database + "?branch=" + branchName} data-cy="branchlabel">{branchName}</a> branch.
			</b></p> : <>
				<div className="mb-2">
					<label htmlFor="liveselect" className="form-label">Database type</label>
					<div>
						<div className="btn-group" role="group">
							<input type="radio" className="btn-check" name="liveselect" autocomplete="off" checked={!live} />
							<label className="btn btn-outline-secondary" htmlFor="liveselect" onClick={() => setLive(false)} data-cy="stdbtn">Standard</label>
							<input type="radio" className="btn-check" name="liveselect" autocomplete="off" checked={live} />
							<label className="btn btn-outline-secondary" htmlFor="liveselect" onClick={() => setLive(true)} data-cy="livebtn">Live</label>
						</div>&nbsp;
						{live ? <span><b>Live</b> means a traditional SQLite database.  As with our Standard databases, these can be made public (or kept private), and
							collaborated upon with others. These <b>are</b> able to run INSERT/UPDATE/DELETE statements and other SQL queries.  A good choice if you're
							not publishing static data.</span>
						: <span><b>Standard</b> uses a git like system of read-only snapshots, suitable for publishing data, and collaborating upon with others. These
							databases <b>cannot</b> have INSERT/UPDATE/DELETE statements run on them directly.</span>
						}
					</div>
				</div>
				<div className="mb-2">
					<label htmlFor="publicselect" className="form-label">Visibility</label>
					<div>
						<div className="btn-group" role="group">
							<input type="radio" className="btn-check" name="publicselect" autocomplete="off" checked={isPublic} />
							<label className="btn btn-outline-secondary" htmlFor="publicselect" onClick={() => setPublic(true)} data-cy="pubbtn">Public</label>
							<input type="radio" className="btn-check" name="publicselect" autocomplete="off" checked={!isPublic} />
							<label className="btn btn-outline-secondary" htmlFor="publicselect" onClick={() => setPublic(false)} data-cy="privbtn">Private</label>
						</div>&nbsp;
						{isPublic ? <span>Database will be <b>public</b>. Everyone has read access to it.</span> : <span>Database will be <b>private</b>. Only you have access to it.</span>}
					</div>
				</div>
				{live === false ?
					<div className="mb-2">
						<label htmlFor="licence" className="form-label">Licence</label>
						<Select
							name={"licence"}
							required={true}
							onChange={(values) => setLicence(values[0].value)}
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
					</div>
				: null}
			</>}

			{live === false ?
				<div className="card text-bg-light mt-2 mb-2">
					<div className="card-header">
						<a href="#/" onClick={() => setExpanded(!isExpanded)}>Advanced</a>
						<span className="pull-right">
							<a href="#/" onClick={() => setExpanded(!isExpanded)}><i className={isExpanded ? "fa fa-minus" : "fa fa-plus"}></i></a>
						</span>
					</div>
					{isExpanded ? (<>
						<div className="card-body">
							<div className="mb-2">
								<label htmlFor="commitmsg" className="form-label">Commit Message</label>
								<MarkdownEditor editorId="commitmsg" rows={10} placeholder="A description for this specific commit" onChange={e => setCommitMsg(e)} />
							</div>
							{meta.owner === "" && meta.database === "" ?
								<div className="mb-2">
									<label htmlFor="sourceurl" className="form-label">Source URL</label>
									<input type="text" className="form-control" id="sourceurl" maxlength={240} data-cy="srcurlinput" value={sourceUrl} onChange={e => setSourceUrl(e.target.value)} />
								</div>
							: null}
							<div className="mb-2">
								<label htmlFor="branch" className="form-label">Branch</label>
								<input type="text" className="form-control" id="branch" maxlength={60} data-cy="branchinput" value={branchName} onChange={e => setBranchName(e.target.value)} />
							</div>
						</div>
					</>) : null}
				</div>
			: null }
		</form>

		<button type="button" className="btn btn-success" onClick={() => uploadDatabase()} data-cy="uploadbtn">Upload</button>
	</>);
}
