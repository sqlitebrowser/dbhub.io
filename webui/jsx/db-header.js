const React = require("react");
const ReactDOM = require("react-dom");

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
			<button type="button" className="btn btn-default" onClick={toggleState} data-cy={cyToggle} disabled={disabled}><i className={"fa " + icon}></i> {state ? textSet : textUnset}</button>
			<button type="button" className="btn btn-default" onClick={gotoPage} data-cy={cyPage}>{number}</button>
		</div>
	);
}

export default function DbHeader() {
	let forkedFrom = null;
	if (meta.forkOwner) {
		forkedFrom = (
			<div style={{fontSize: "small"}}>
				forked from <a href={"/" + meta.forkOwner}>{meta.forkOwner}</a> /&nbsp;
				{meta.forkDeleted ? "deleted database" : <a href={"/" + meta.forkOwner + "/" + meta.forkDatabase}>{meta.forkDatabase}</a>}
			</div>
		);
	}

	let settings = null;
	if (authInfo.loggedIn) {
		settings = <label id="settings" className={meta.pageSection == "db_settings" ? "dbMenuLinkActive" : "dbMenuLink"}><a href={"/settings/" + meta.owner + "/" + meta.database} className="blackLink" title="Settings" data-cy="settingslink"><i className="fa fa-cog"></i> Settings</a></label>;
	}

	let publicString = "Private";
	if (meta.publicDb) {
		publicString = "Public";
	}

	let visibility = null;
	if (meta.owner == authInfo.loggedInUser) {
		visibility = <><b>Visibility:</b> <a className="blackLink" href={"/settings/" + meta.owner + "/" + meta.database} data-cy="vis">{publicString}</a></>;
	} else {
		visibility = <><b>Visibility:</b> <span data-cy="vis">{publicString}</span></>;
	}

	let licence = null;
	if (meta.owner == authInfo.loggedInUser) {
		licence = <><b>Licence:</b> <a className="blackLink" href={"/settings/" + meta.owner + "/" + meta.database} data-cy="lic">{ meta.licence }</a></>;
	} else {
		if (meta.licenceUrl != "") {
			licence = <><b>Licence:</b> <a className="blackLink" href={ meta.licenceURL } data-cy="licurl">{ meta.licence }</a></>;
		} else {
			licence = <><b>Licence:</b> <span data-cy="licurl">{ meta.licence }</span></>;
		}
	}

	return (
	<>
		<div className="row">
			<div className="col-md-12">
				<h2 id="viewdb" style={{marginTop: "10px"}}>
					<div className="pull-left">
						<div>
							<a className="blackLink" href={"/" + meta.owner} data-cy="headerownerlnk">{meta.owner}</a> /&nbsp;
							<a className="blackLink" href={"/" + meta.owner + "/" + meta.database} data-cy="headerdblnk">{meta.database}</a>
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
							disabled={meta.owner == authInfo.loggedInUser}
						/>
					</div>
				</h2>
			</div>
		</div>
		<div className="row" style={{paddingBottom: "5px", paddingTop: "10px"}}>
		    <div className="col-md-6">
			<label id="viewdata" className={meta.pageSection == "db_data" ? "dbMenuLinkActive" : "dbMenuLink"}><a href={"/" + meta.owner + "/" + meta.database} className="blackLink" title="Data" data-cy="datalink"><i className="fa fa-database"></i> Data</a></label>

			&nbsp; &nbsp; &nbsp;

			<label id="viewvis" className={meta.pageSection == "db_vis" ? "dbMenuLinkActive" : "dbMenuLink"}><a href={"/vis/" + meta.owner + "/" + meta.database} className="blackLink" title="Visualise" data-cy="vislink"><i className="fa fa-bar-chart"></i> Visualise</a></label>

			&nbsp; &nbsp; &nbsp;

			<label id="viewdiscuss" className={meta.pageSection == "db_disc" ? "dbMenuLinkActive" : "dbMenuLink"}><a href={"/discuss/" + meta.owner + "/" + meta.database} className="blackLink" title="Discussions" data-cy="discusslink"><i className="fa fa-commenting"></i> Discussions:</a> {meta.numDiscussions}</label>

			&nbsp; &nbsp; &nbsp;

			<label id="viewmrs" className={meta.pageSection == "db_merge" ? "dbMenuLinkActive" : "dbMenuLink"}><a href={"/merge/" + meta.owner + "/" + meta.database} className="blackLink" title="Merge Requests" data-cy="mrlink"><i className="fa fa-clone"></i> Merge Requests:</a> {meta.numMRs}</label>

			&nbsp; &nbsp; &nbsp;

			{settings}
		    </div>
		    <div className="col-md-6">
			<div className="pull-right">
				{visibility} &nbsp;
				<b>Last Commit:</b> {meta.commitID.substring(0, 8)} ({getTimePeriod(meta.repoModified, false)}) &nbsp;
				{licence} &nbsp;
				<b>Size:</b> <span data-cy="size">{Math.round(meta.size / 1024).toLocaleString()} KB</span>
			</div>
		    </div>
		</div>
		</>
	)
}
