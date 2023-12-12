const React = require("react");
const ReactDOM = require("react-dom");

import {getTimePeriod} from "./format";

function DatabasePanel({data, username}) {
	const [isExpanded, setExpanded] = React.useState(false);

	return (
		<div className="panel panel-default">
			<div className="panel-heading">
				<h3 className="panel-title">
					{username === authInfo.loggedInUser ? (<a className="blackLink" href={"/settings/" + username + "/" + data.Database}><i className="fa fa-cog"></i></a>) : null}
					&nbsp;
					<a className="blackLink" href={"/" + username + "/" + data.Database}>{data.Database}</a>
					<span className="pull-right">
						<a href="#/" className="blackLink" onClick={() => setExpanded(!isExpanded)}><i className={isExpanded ? "fa fa-minus" : "fa fa-plus"}></i></a>
					</span>
				</h3>
			</div>
			<div className="panel-body">
				{data.OneLineDesc !== "" ? <p>{data.OneLineDesc}</p> : null}
				<p>
					<strong>Updated: </strong><span title={new Date(data.RepoModified).toLocaleString()} className="text-info">{getTimePeriod(data.RepoModified, false)}</span>&nbsp;&nbsp;
                    {data.IsLive ? null : <><strong>Licence: </strong><span className="text-info">{data.LicenceURL === "" ? data.Licence : <a href={data.LicenceURL}>{data.Licence}</a>}</span>&nbsp;&nbsp;</>}
					<strong>Size: </strong><span className="text-info">{Math.floor(data.Size / 1024).toLocaleString()} KB</span>
				</p>
				{isExpanded ? (<>
					<p>
						{data.IsLive ? null : <><strong>Commit ID: </strong><span className="text-info">{data.CommitID.substring(0, 8)}</span>&nbsp;&nbsp;</>}
						<strong>Contributors: </strong><span className="text-info"><a className="blackLink" href={"/contributors/" + userData.name + "/" + data.Database}>{data.Contributors}</a></span>&nbsp;&nbsp;
						<strong>Watchers: </strong><span className="text-info"><a className="blackLink" href={"/watchers/" + userData.name + "/" + data.Database}>{data.Watchers}</a></span>&nbsp;&nbsp;
						<strong>Stars: </strong><span className="text-info"><a className="blackLink" href={"/stars/" + userData.name + "/" + data.Database}>{data.Stars}</a></span>&nbsp;&nbsp;
						{data.IsLive ? null : <><strong>Forks: </strong><span className="text-info"><a className="blackLink" href={"/forks/" + userData.name + "/" + data.Database}>{data.Forks}</a></span>&nbsp;&nbsp;</>}
						<strong>Discussions: </strong><span className="text-info"><a className="blackLink" href={"/discuss/" + userData.name + "/" + data.Database}>{data.Discussions}</a></span>&nbsp;&nbsp;
						{data.IsLive ? null : <><strong>MRs: </strong><span className="text-info"><a className="blackLink" href={"/merge/" + userData.name + "/" + data.Database}>{data.MRs}</a></span>&nbsp;&nbsp;</>}
						{data.IsLive ? null : <><strong>Branches: </strong><span className="text-info"><a className="blackLink" href={"/branches/" + userData.name + "/" + data.Database}>{data.Branches}</a></span>&nbsp;&nbsp;</>}
						{data.IsLive ? null : <><strong>Releases: </strong><span className="text-info"><a className="blackLink" href={"/releases/" + userData.name + "/" + data.Database}>{data.Releases}</a></span>&nbsp;&nbsp;</>}
						{data.IsLive ? null : <><strong>Tags: </strong><span className="text-info"><a className="blackLink" href={"/tags/" + userData.name + "/" + data.Database}>{data.Tags}</a></span>&nbsp;</>}
						{data.Downloads === undefined ? null : <><strong>Downloads: </strong><span className="text-info">{data.Downloads}</span>&nbsp;</>}
						{data.Views === undefined ? null : <><strong>Views: </strong><span className="text-info">{data.Views}</span>&nbsp;</>}
					</p>
					{data.SourceURL === "" ? null : <p><strong>Source: </strong><span className="text-info"><a className="blackLink" href={data.SourceURL}>{data.SourceURL}</a></span></p>}
				</>) : null}
			</div>
		</div>
	);
}

export function DatabasePanelGroup({title, noDatabasesMessage, databases, username}) {
	const databaseRows = databases === null ? null : databases.map(d => DatabasePanel({data: d, username: username}));

	return (<>
		<h3>{title}</h3>
		{databaseRows ? databaseRows : (<h4><em>{noDatabasesMessage}</em></h4>)}
	</>);
}

export default function UserPage() {
	return (<>
		<h2>
			{userData.avatarUrl ? <img src={userData.avatarUrl} height="48" width="48" style={{border: "1px solid #8c8c8c"}} /> : null}&nbsp;
			{userData.name + (userData.fullName ? ": " + userData.fullName : "")}'s <span data-cy="userpg">public projects</span>
		</h2>
		<div className="row">
			<div className="col-md-6">
				<DatabasePanelGroup title="Public standard databases" noDatabasesMessage="No public standard databases yet" databases={userData.databases} username={userData.name} />
			</div>
			<div className="col-md-6">
				<DatabasePanelGroup title="Public live databases" noDatabasesMessage="No public live databases yet" databases={userData.liveDatabases} username={userData.name} />
			</div>
		</div>
	</>);
}
