const React = require("react");
const ReactDOM = require("react-dom");

export default function UpdatesPage() {
	let rows = <h5 className="text-center">No new status updates</h5>;

	if (updates !== null) {
		rows = [];
		for (const [dbname, entries] of Object.entries(updates)) {
			let dbEntries = [];
			entries.forEach(e => {
				dbEntries.push(<li className="list-group-item"><a href={e.event_url}>{e.title}</a></li>);
			});

			rows.push(
				<div className="card mb-2">
					<div className="card-header fw-bold"><a href={"/" + dbname}>{dbname}</a></div>
					<ul className="list-group list-group-flush">
						{dbEntries}
					</ul>
				</div>
			);
		}
	}

	return (<>
		<h3 className="text-center" data-cy="updates">Status updates</h3>
		{rows}
	</>);
}
