const React = require("react");
const ReactDOM = require("react-dom");

import {getTimePeriod} from "./format";

function DiscussionListRow({data, mergeRequests}) {
	return (
		<div className="card text-bg-light mt-1">
			<div className="card-body">
				<h5 className="card-title"># {data.disc_id} {data.open === true ? <i className="fa fa-minus-square-o fa-lg text-success fs-4" /> : <i className="fa fa fa-check-square-o fa-lg text-danger fs-4" />} <a href={"/" + (mergeRequests ? "merge" : "discuss") + "/" + meta.owner + "/" + meta.database + "?id=" + data.disc_id}>{data.title}</a></h5>
				<h6 className="card-subtitle">Created <span className="text-info" title={new Date(data.creation_date).toLocaleString()}>{getTimePeriod(data.creation_date, true)}</span> by <a href={"/" + data.creator}>{data.avatar_url !== "" ? <img src={data.avatar_url} height="18" width="18" className="border border-secondary" /> : null} {data.creator}</a>. Last modified <span className="text-info" title={new Date(data.last_modified).toLocaleString()}>{getTimePeriod(data.last_modified, true)}</span></h6>
				{data.comment_count > 0 ? <p className="card-text"> <i className="fa fa-comment-o"></i> <a href={"/" + (mergeRequests ? "merge" : "discuss") + "/" + meta.owner + "/" + meta.database + "?id=" + data.disc_id}>{data.comment_count} comment{data.comment_count > 1 ? "s" : ""}</a></p> : null}
			</div>
		</div>
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
					<div className="btn-group">
						<label className={"btn btn-light " + (showOpen ? "active" : null)} onClick={() => setShowOpen(true)}>Open</label>
						<label className={"btn btn-light " + (showOpen ? null : "active")} onClick={() => setShowOpen(false)}>Closed</label>
					</div>
				</div>
			</div>
		</div>
	);

	// Special case of no discussions at all
	if (discussionData === null) {
		return <>{buttonRow}<h5 data-cy="nodisc" className="text-center mt-2">This database does not have any {mergeRequests ? "merge requests" : "discussions"} yet</h5></>;
	}

	// Render discussion items
	const rows = discussionData
		.filter(item => item.open === showOpen)
		.map(item => DiscussionListRow({mergeRequests: mergeRequests, data: item}));

	// If no discussions are visible in the current selection print a message
	if (rows.length === 0) {
		return <>{buttonRow}<h5 data-cy="nodisc" className="text-center mt-2">This database does not have any {showOpen ? "open" : "closed"} {mergeRequests ? "merge requests" : "discussions"} yet</h5></>;
	}

	return (<>
		{buttonRow}
		{rows}
	</>);
}
