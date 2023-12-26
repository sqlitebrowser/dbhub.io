const React = require("react");
const ReactDOM = require("react-dom");

function DatabaseContributorRow({data, index}) {
	const [contributorIndex, setContributorIndex] = React.useState(Number(index));

	return (
		<tr>
			<td>
				{data.avatar_url !== "" ? <img src={data.avatar_url} height="30" width="30" className="border border-secondary" /> : null}&nbsp;
				<a href={"/" + data.author_user_name}>{data.author_name}</a>
			</td>
			<td>
				{data.num_commits}
			</td>
		</tr>
	);
}

export default function DatabaseContributors() {
	// Render table rows
	let rows = [];
	for (const [index, data] of Object.entries(contributorData)) {
		rows.push(<DatabaseContributorRow data={data} index={index} />);
	}

	return (
		<div className="border border-secondary rounded">
			<table className="table table-striped table-responsive m-0">
				<thead>
					<tr><th>Contributor</th><th># of Commits</th></tr>
				</thead>
				<tbody>
					{rows}
				</tbody>
			</table>
		</div>
	);
}
