const React = require("react");
const ReactDOM = require("react-dom");

import {getTimePeriod} from "./format";

export default function DatabaseWatchers({stars}) {
	let data = stars ? starsData : watchersData;

	if (data === null) {
		if (stars) {
			return <h4 className="text-center">No-one has starred '{meta.owner + "/" + meta.database}' yet</h4>;
		} else {
			return <h4 className="text-center">No-one is watching '{meta.owner + "/" + meta.database}' yet</h4>;
		}
	}

	let rows = [];
	data.forEach(function(v) {
		rows.push(
			<li className="list-group-item">
				<div className="d-flex w-100 justify-content-between">
					<h5><a href={"/" + v.Owner}>{v.display_name}</a></h5>
					<small>{stars ? "Starred" : "Started watching"} <span title={new Date(v.DateEntry).toLocaleString()}>{getTimePeriod(v.DateEntry, true)}</span></small>
				</div>
			</li>
		);
	});

	return (
		<ul className="list-group w-50">
			{rows}
		</ul>
	);
}
