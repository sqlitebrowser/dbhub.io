const React = require("react");
const ReactDOM = require("react-dom");

import {getTimePeriod} from "./format";

export default function DatabaseWatchers({stars}) {
	let data = stars ? starsData : watchersData;

	if (data === null) {
		if (stars) {
			return <h3 style={{textAlign: "center"}}>No-one has starred '{meta.owner + "/" + meta.database}' yet</h3>;
		} else {
			return <h3 style={{textAlign: "center"}}>No-one is watching '{meta.owner + "/" + meta.database}' yet</h3>;
		}
	}

	let rows = [];
	data.forEach(function(v) {
		rows.push(
			<li className="list-group-item">
				<h4>• <a className="blackLink" href={"/" + v.Owner}>{v.display_name}</a></h4>
				{stars ? "Starred" : "Started watching"} <span title={new Date(v.DateEntry).toLocaleString()}>{getTimePeriod(v.DateEntry, true)}</span>
			</li>
		);
	});

	return (
		<ul className="list-group">
			{rows}
		</ul>
	);
}
