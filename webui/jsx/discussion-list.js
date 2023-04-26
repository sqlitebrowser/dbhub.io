const React = require("react");
const ReactDOM = require("react-dom");

import {getTimePeriod} from "./format";

function DiscussionListRow({data, mergeRequests}) {
	return (
		<tr>
			<td width="80px">
				<div className="pull-right" style={{paddingTop: "6px"}}>
					{data.open === true ? <i className="fa fa-minus-square-o fa-lg text-success" /> : <i className="fa fa fa-check-square-o fa-lg text-danger" />}
				</div>
				<div style={{paddingTop: "6px"}}># {data.disc_id}</div>
			</td>
			<td>
				<span className="lead"><a className="blackLink" href={"/" + (mergeRequests ? "merge" : "discuss") + "/" + meta.owner + "/" + meta.database + "?id=" + data.disc_id}>{data.title}</a></span>
				<div>
					Created <span className="text-info" title={new Date(data.creation_date).toLocaleString()}>{getTimePeriod(data.creation_date, true)}</span> by <a className="blackLink" href={"/" + data.creator}>{data.avatar_url !== "" ? <img src={data.avatar_url} style={{verticalAlign: "top", border: "1px solid #8c8c8c"}} height="18" width="18" /> : null} {data.creator}</a>. Last modified <span className="text-info" title={new Date(data.last_modified).toLocaleString()}>{getTimePeriod(data.last_modified, true)}</span>

					{data.comment_count > 0 ? <span> <i className="fa fa-comment-o"></i> <a className="blackLink" href={"/" + (mergeRequests ? "merge" : "discuss") + "/" + meta.owner + "/" + meta.database + "?id=" + data.disc_id}>{data.comment_count} comment{data.comment_count > 1 ? "s" : ""}</a></span> : null}
				</div>
			</td>
		</tr>
	);
}

export default function DiscussionList({mergeRequests}) {
	const [showOpen, setShowOpen] = React.useState(true);

	// Switch to the create discussion page
	function createDiscussion() {
		if (authInfo.loggedInUser) {
			window.location = "/" + (mergeRequests ? "compare" : "creatediscuss") + "/" + meta.owner + "/" + meta.database;
		} else {
			// User needs to be logged in
			lock.show();
		}
	}

	// Button row at the top
	const buttonRow = (
		<div className="row">
			<div className="col-md-12">
				<div className="text-center">
					<button className="btn btn-success" onClick={() => createDiscussion()}>{mergeRequests ? "New Merge Request" : "Start a new discussion"}</button>
					&nbsp;
					<div className="btn-group" data-toggle="buttons">
						<label className={"btn btn-default " + (showOpen ? "active" : null)} onClick={() => setShowOpen(true)}>Open</label>
						<label className={"btn btn-default " + (showOpen ? null : "active")} onClick={() => setShowOpen(false)}>Closed</label>
					</div>
				</div>
			</div>
		</div>
	);

	// Special case of no discussions at all
	if (discussionData === null) {
		return <>{buttonRow}<h4 data-cy="nodisc" className="text-center">This database does not have any {mergeRequests ? "merge requests" : "discussions"} yet</h4></>;
	}

	// Render discussion items
	const rows = discussionData
		.filter(item => item.open === showOpen)
		.map(item => DiscussionListRow({mergeRequests: mergeRequests, data: item}));

	// If no discussions are visible in the current selection print a message
	if (rows.length === 0) {
		return <>{buttonRow}<h4 data-cy="nodisc" className="text-center">This database does not have any {showOpen ? "open" : "closed"} {mergeRequests ? "merge requests" : "discussions"} yet</h4></>;
	}

	return (<>
		{buttonRow}
		<table className="table table-striped table-responsive" style={{marginTop: "1em"}}>
			<tbody>
				{rows}
			</tbody>
		</table>
	</>);
}
