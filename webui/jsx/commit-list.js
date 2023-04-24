const React = require("react");
const ReactDOM = require("react-dom");

export default function CommitList({commits, owner, database}) {
	// Prepare rendered rows for commit table
	const commitRows = (commits === null ? null : commits.map(row => (
		<tr>
			<td>
				<a href={"/" + row.author_username} className="blackLink">
					{row.author_avatar !== "" ? <img src={row.author_avatar} height="18" width="18" style={{border: "1px solid #8c8c8c"}} /> : null}&nbsp;
					{row.author_name}
				</a>
			</td>
			<td>
				<a className="blackLink" href={"/diffs/" + owner + "/" + database + "?commit_a=" + row.parent + "&commit_b=" + row.id}>
					{row.id.substring(0, 8)}
				</a>
			</td>
			<td>
				{row.message === "" ? <span className="text-muted">This commit has no commit message</span> : row.message}
				{row.licence_change !== "" ? <span className="text-danger">{row.licence_change}</span> : null}
			</td>
			<td>
				<span title={new Date(row.timestamp).toLocaleString()}>{getTimePeriod(row.timestamp, true)}</span>
			</td>
		</tr>
	)));

	return (
		<table className="table">
			<thead>
				<tr><th>Author</th><th>Commit ID</th><th>Commit message</th><th>Date</th></tr>
			</thead>
			<tbody>
				{commitRows}
			</tbody>
		</table>
	);
}
