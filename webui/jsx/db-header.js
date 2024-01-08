const React = require("react");
const ReactDOM = require("react-dom");

import {getTimePeriod} from "./format";

function ToggleButton({icon, textSet, textUnset, redirectUrl, updateUrl, pageUrl, isSet, count, cyToggle, cyPage, disabled}) {
	const [state, setState] = React.useState(isSet);
	const [number, setNumber] = React.useState(count);

	function gotoPage () {
		window.location = pageUrl;
	}

	function toggleState() {
		if (authInfo.loggedIn !== true) {
			// User needs to be logged in
			lock.show();
			return;
		}

		if (redirectUrl !== undefined) {
			window.location = redirectUrl;
			return;
		}

		// Retrieve the branch list for the newly selected database
		fetch(updateUrl)
			.then((response) => response.text())
			.then((text) => {
				// Update button text
				setState(!state);

				// Update displayed count
				setNumber(text);
			});
	}

	return (
		<div className="btn-group">
			<button type="button" className="btn btn-outline-secondary text-reset" onClick={toggleState} data-cy={cyToggle} disabled={disabled}><i className={"fa " + icon}></i> {state ? textSet : textUnset}</button>
			<button type="button" className="btn btn-outline-secondary text-reset" onClick={gotoPage} data-cy={cyPage}>{number}</button>
		</div>
	);
}

export default function DbHeader() {
	// Fork and commit information and actions are only shown for non-live databases
	let forkedFrom = null;
	let forkButton = null;
	let lastCommit = null;
	if (meta.isLive === false) {
		forkButton = (
			<ToggleButton
				icon="fa-sitemap"
				textSet="Fork"
				textUnset="Fork"
				redirectUrl={"/x/forkdb/" + meta.owner + "/" + meta.database + "?commit=" + meta.commitID}
				pageUrl={"/forks/" + meta.owner + "/" + meta.database}
				isSet={false}
				count={meta.numForks}
				cyToggle="forksbtn"
				cyPage="forkspagebtn"
				disabled={meta.owner === authInfo.loggedInUser}
			/>
		);

		if (meta.forkOwner) {
			forkedFrom = (
				<p className="fs-6 mb-0">
					forked from <a href={"/" + meta.forkOwner}>{meta.forkOwner}</a> /&nbsp;
					{meta.forkDeleted ? "deleted database" : <a href={"/" + meta.forkOwner + "/" + meta.forkDatabase}>{meta.forkDatabase}</a>}
				</p>
			);
		}

		lastCommit = (<><b>Last Commit:</b> {meta.commitID.substring(0, 8)} ({getTimePeriod(meta.repoModified, false)}) &nbsp;</>);
	}

	let settings = null;
	if (authInfo.loggedIn && (meta.owner === authInfo.loggedInUser)) {
		settings = <a id="settings" className={meta.pageSection === "db_settings" ? "nav-link active" : "nav-link"} href={"/settings/" + meta.owner + "/" + meta.database} title="Settings" data-cy="settingslink"><i className="fa fa-cog"></i> Settings</a>;
	}

	let publicString = "Private";
	if (meta.publicDb) {
		publicString = "Public";
	}

	let visibility = null;
	if (meta.owner === authInfo.loggedInUser) {
		visibility = <><b>Visibility:</b> <a href={"/settings/" + meta.owner + "/" + meta.database} data-cy="vis">{publicString}</a></>;
	} else {
		visibility = <><b>Visibility:</b> <span data-cy="vis">{publicString}</span></>;
	}

	let licence = null;
	if (meta.owner === authInfo.loggedInUser) {
		licence = <><b>Licence:</b> <a href={"/settings/" + meta.owner + "/" + meta.database} data-cy="lic">{ meta.licence }</a></>;
	} else {
		if (meta.licenceUrl !== "") {
			licence = <><b>Licence:</b> <a href={ meta.licenceURL } data-cy="licurl" rel="noopener noreferrer external">{ meta.licence }</a></>;
		} else {
			licence = <><b>Licence:</b> <span data-cy="licurl">{ meta.licence }</span></>;
		}
	}

	return (
	<div className="container-fluid">
		<div className="row">
			<div className="col-md-12">
				<h3 id="viewdb">
					<div className="pull-left">
						<div>
							<a href={"/" + meta.owner} data-cy="headerownerlnk">{meta.owner}</a> /&nbsp;
							<a href={"/" + meta.owner + "/" + meta.database} data-cy="headerdblnk">{meta.database}</a>
						</div>
						{forkedFrom}
					</div>
					<div className="pull-right">
						<ToggleButton
							icon="fa-eye"
							textSet="Unwatch"
							textUnset="Watch"
							updateUrl={"/x/watch/" + meta.owner + "/" + meta.database}
							pageUrl={"/watchers/" + meta.owner + "/" + meta.database}
							isSet={meta.isWatching}
							count={meta.numWatchers}
							cyToggle="watcherstogglebtn"
							cyPage="watcherspagebtn"
						/>
						&nbsp;
						<ToggleButton
							icon="fa-star"
							textSet="Unstar"
							textUnset="Star"
							updateUrl={"/x/star/" + meta.owner + "/" + meta.database}
							pageUrl={"/stars/" + meta.owner + "/" + meta.database}
							isSet={meta.isStarred}
							count={meta.numStars}
							cyToggle="starstogglebtn"
							cyPage="starspagebtn"
						/>
						&nbsp;
						{forkButton}
					</div>
				</h3>
			</div>
		</div>
		<div className="row mb-1 mt-2">
		    <div className="col-md-6">
			<nav className="nav nav-underline">
				<a id="viewdata" className={meta.pageSection === "db_data" ? "nav-link active" : "nav-link"} href={"/" + meta.owner + "/" + meta.database} title="Data" data-cy="datalink"><i className="fa fa-database"></i> Data</a>
				<a id="viewvis" className={meta.pageSection === "db_vis" ? "nav-link active" : "nav-link"} href={"/vis/" + meta.owner + "/" + meta.database} title="Visualise" data-cy="vislink"><i className="fa fa-bar-chart"></i> Visualise</a>
				{meta.isLive && (meta.owner === authInfo.loggedInUser) ? <a id="viewexec" className={meta.pageSection === "db_exec" ? "nav-link active" : "nav-link"} href={"/exec/" + meta.owner + "/" + meta.database} title="Execute SQL" data-cy="execlink"><i className="fa fa-wrench"></i> Execute SQL</a> : null }
				<a id="viewdiscuss" className={meta.pageSection === "db_disc" ? "nav-link active" : "nav-link"} href={"/discuss/" + meta.owner + "/" + meta.database} title="Discussions" data-cy="discusslink"><i className="fa fa-commenting"></i> Discussions: {meta.numDiscussions}</a>
				{meta.isLive ? null : <a id="viewmrs" className={meta.pageSection === "db_merge" ? "nav-link active" : "nav-link"} href={"/merge/" + meta.owner + "/" + meta.database} title="Merge Requests" data-cy="mrlink"><i className="fa fa-clone"></i> Merge Requests: {meta.numMRs}</a>}
				{settings}
			</nav>
		    </div>
		    <div className="col-md-6">
			<div className="pull-right">
				{visibility} &nbsp;
				{lastCommit}
				{meta.isLive ? null : licence} &nbsp;
				<b>Size:</b> <span data-cy="size">{Math.round(meta.size / 1024).toLocaleString()} KB</span>
			</div>
		    </div>
		</div>
	</div>);
}
