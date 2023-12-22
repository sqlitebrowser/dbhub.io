const React = require("react");
const ReactDOM = require("react-dom");

import {DatabasePanelGroup} from "./user-page";
import {getTimePeriod} from "./format";

function WatchPanel({data, dateText}) {
	const [isExpanded, setExpanded] = React.useState(false);

	return (
		<div className="card text-bg-light mb-1">
			<div className="card-header">
				<a href={"/" + data.Owner}>{data.Owner}</a>&nbsp;/&nbsp;<a href={"/" + data.Owner + "/" + data.DBName}>{data.DBName}</a>
				<span className="pull-right">
					<a href="#/" onClick={() => setExpanded(!isExpanded)}><i className={isExpanded ? "fa fa-minus" : "fa fa-plus"}></i></a>
				</span>
			</div>
			{isExpanded ? (<>
				<div className="card-body">
					<p>
						<strong>{dateText}: </strong><span className="text-info" title={new Date(data.DateEntry).toLocaleString()}>{getTimePeriod(data.DateEntry, false)}</span>
					</p>
				</div>
			</>) : null}
		</div>
	);
}

function WatchPanelGroup({title, noDatabasesMessage, databases, dateText}) {
	const databaseRows = databases === null ? null : databases.map(d => WatchPanel({data: d, dateText: dateText}));

	return (<>
		<h4>{title}</h4>
		{databaseRows ? databaseRows : (<em>{noDatabasesMessage}</em>)}
	</>);
}

function SharedWithYouPanel({data}) {
	return (
		<div className="card text-bg-light mb-1">
			<div className="card-header">
				<a href={"/" + data.owner_name + "/" + data.database_name}>{data.owner_name} / {data.database_name}</a>: {data.permission === "rw" ? "Read Write" : "Read Only"}
			</div>
		</div>
	);
}

function SharedWithYouPanelGroup({databases}) {
	const databaseRows = databases === null ? null : databases.map(d => SharedWithYouPanel({data: d}));

	return (<>
		<h4>Databases shared with you</h4>
		{databaseRows ? databaseRows : (<em>No databases shared with you yet</em>)}
	</>);
}

function SharedWithOthersPanel({data}) {
	const [isExpanded, setExpanded] = React.useState(false);

	let permissionRows = [];
	for (const [user, perm] of Object.entries(data.user_permissions)) {
		permissionRows.push(<tr><td><a href={"/" + user}>{user}</a></td><td>{perm === "rw" ? "Read Write" : "Read Only"}</td></tr>);
	}

	return (
		<div className="card text-bg-light mb-1">
			<div className="card-header">
				<a href={"/settings/" + authInfo.loggedInUser + "/" + data.database_name}><i className="fa fa-cog"></i></a>&nbsp;
				<a href={"/" + authInfo.loggedInUser + "/" + data.database_name}>{data.database_name}</a>
				<span className="pull-right">
					<a href="#/" onClick={() => setExpanded(!isExpanded)}><i className={isExpanded ? "fa fa-minus" : "fa fa-plus"}></i></a>
				</span>
			</div>
			{isExpanded ? (<>
				<table className="table">
					<thead>
						<tr><th>User</th><th>Permission</th></tr>
					</thead>
					<tbody>
						{permissionRows}
					</tbody>
				</table>
			</>) : null}
		</div>
	);
}

function SharedWithOthersPanelGroup({databases}) {
	const databaseRows = databases === null ? null : databases.map(d => SharedWithOthersPanel({data: d}));

	return (<>
		<h4>Databases shared with others</h4>
		{databaseRows ? databaseRows : (<em>No databases shared with others yet</em>)}
	</>);
}

export default function ProfilePage() {
	return (<>
		<h3>
			{authInfo.avatarUrl ? <img src={authInfo.avatarUrl} height="48" width="48" className="border border-secondary" /> : null}&nbsp;Your page
		</h3>
		<div className="row mb-2">
			<div className="col-md-12">
				<a className="btn btn-success" href="/upload/" data-cy="uploadbtn">Upload database</a>&nbsp;
				<a className="btn btn-primary" href="/x/gencert" role="button" data-cy="gencertbtn">Generate client certificate</a>
			</div>
		</div>
		<div className="row mb-2">
			<div className="col-md-6" data-cy="pubdbs">
				<DatabasePanelGroup title="Public standard databases" noDatabasesMessage="No public standard databases yet" databases={userData.publicDbs} username={authInfo.loggedInUser} />
			</div>
			<div className="col-md-6" data-cy="privdbs">
				<DatabasePanelGroup title="Private standard databases" noDatabasesMessage="No private standard databases yet" databases={userData.privateDbs} username={authInfo.loggedInUser} />
			</div>
		</div>
		<div className="row mb-2">
			<div className="col-md-6">
				<DatabasePanelGroup title="Public live databases" noDatabasesMessage="No public live databases yet" databases={userData.publicLiveDbs} username={authInfo.loggedInUser} />
			</div>
			<div className="col-md-6">
				<DatabasePanelGroup title="Private live databases" noDatabasesMessage="No private live databases yet" databases={userData.privateLiveDbs} username={authInfo.loggedInUser} />
			</div>
		</div>
		<div className="row mb-2">
			<div className="col-md-6" data-cy="stars">
				<WatchPanelGroup title="Databases you've starred" noDatabasesMessage="No starred databases yet" databases={userData.starredDbs} dateText="Starred" />
			</div>
			<div className="col-md-6" data-cy="watches">
				<WatchPanelGroup title="Databases you're watching" noDatabasesMessage="Not watching any databases yet" databases={userData.watchedDbs} dateText="Started watching" />
			</div>
		</div>
		<div className="row mb-2">
			<div className="col-md-6" data-cy="sharedwithyou">
				<SharedWithYouPanelGroup databases={userData.sharedWithYouDbs} />
			</div>
			<div className="col-md-6" data-cy="sharedwithothers">
				<SharedWithOthersPanelGroup databases={userData.sharedWithOthersDbs} />
			</div>
		</div>
	</>);
}
