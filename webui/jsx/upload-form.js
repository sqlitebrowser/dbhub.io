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
				<div className="col-md-12 text-center">
					<div style={{paddingBottom: "1em"}}>
						<h4 style={{color: statusMessageColour}}>{statusMessage}</h4>
					</div>
				</div>
			</div>
		) : null}
		<form>
			<div className="form-group">
				<label htmlFor="database">Database file</label>
				<input type="file" id="database" name="database" data-cy="dbfile" />
			</div>

			{meta.owner !== "" && meta.database !== "" ? <p><b>
				As a new commit into the <a className="blackLink" href={"/" + meta.owner} data-cy="ownerlabel">{meta.owner}</a> /&nbsp;
				<a className="blackLink" href={"/" + meta.owner + "/" + meta.database} data-cy="dbnamelabel">{meta.database}</a> database,
				in the <a className="blackLink" href={"/" + meta.owner + "/" + meta.database + "?branch=" + branchName} data-cy="branchlabel">{branchName}</a> branch.
			</b></p> : <>
				<div className="form-group">
					<label htmlFor="liveselect" className="control-label">Database type</label>
					<div>
						<div className="btn-group" data-toggle="buttons">
							<label className={"btn btn-default " + (live ? null : "active")} onClick={() => setLive(false)} data-cy="stdbtn">
								<input type="radio" name="liveselect" checked={!live} /> Standard
							</label>
							<label className={"btn btn-default " + (live ? "active" : null)} onClick={() => setLive(true)} data-cy="livebtn">
								<input type="radio" name="liveselect" checked={live} /> Live
							</label>
						</div>&nbsp;
						{live ? <span><b>Live</b> means a traditional SQLite database.  As with our Standard databases, these can be made public (or kept private), and
							collaborated upon with others. These <b>are</b> able to run INSERT/UPDATE/DELETE statements and other SQL queries.  A good choice if you're
							not publishing static data.</span>
						: <span><b>Standard</b> uses a git like system of read-only snapshots, suitable for publishing data, and collaborating upon with others. These
							databases <b>cannot</b> have INSERT/UPDATE/DELETE statements run on them directly.</span>
						}
					</div>
				</div>
				<div className="form-group">
					<label htmlFor="publicselect" className="control-label">Visibility</label>
					<div>
						<div className="btn-group" data-toggle="buttons">
							<label className={"btn btn-default " + (isPublic ? "active" : null)} onClick={() => setPublic(true)} data-cy="pubbtn">
								<input type="radio" name="publicselect" checked={isPublic} /> Public
							</label>
							<label className={"btn btn-default " + (isPublic ? null : "active")} onClick={() => setPublic(false)} data-cy="privbtn">
								<input type="radio" name="publicselect" checked={!isPublic} /> Private
							</label>
						</div>&nbsp;
						{isPublic ? <span>Database will be <b>public</b>. Everyone has read access to it.</span> : <span>Database will be <b>private</b>. Only you have access to it.</span>}
					</div>
				</div>
				{live === false ?
					<div className="form-group">
						<label htmlFor="licence" className="control-label">Licence</label>
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
										{item.url !== '' ? (<><span> - </span><a href={item.url} target="_blank" rel="noopener noreferrer">info</a></>) : null}
									</div>
								);
							}}
						/>
					</div>
				: null}
			</>}

			{live === false ?
				<div className="panel panel-default">
					<div className="panel-heading">
						<h3 className="panel-title">
							<a href="#/" className="blackLink" onClick={() => setExpanded(!isExpanded)}>Advanced</a>
							<span className="pull-right">
								<a href="#/" className="blackLink" onClick={() => setExpanded(!isExpanded)}><i className={isExpanded ? "fa fa-minus" : "fa fa-plus"}></i></a>
							</span>
						</h3>
					</div>
					{isExpanded ? (<>
						<div className="panel-body">
							<div className="form-group">
								<label htmlFor="commitmsg">Commit Message</label>
								<MarkdownEditor editorId="commitmsg" rows={10} placeholder="A description for this specific commit" onChange={e => setCommitMsg(e)} />
							</div>
							{meta.owner === "" && meta.database === "" ?
								<div className="form-group">
									<label htmlFor="sourceurl">Source URL</label>
									<input type="text" className="form-control" id="sourceurl" maxlength={240} data-cy="srcurlinput" value={sourceUrl} onChange={e => setSourceUrl(e.target.value)} />
								</div>
							: null}
							<div className="form-group">
								<label htmlFor="branch">Branch</label>
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
