const React = require("react");
const ReactDOM = require("react-dom");

function DatabaseDiffRow({data}) {
	// Schema changes
	let schema_change = null;
	if (data.schema !== null && data.schema.action_type === "add") {
		schema_change = <>
			<strong data-cy="addtype">{"Created " + data.object_type}</strong><br />
			<span style={{backgroundColor: "#e6ffed"}} data-cy="adddetail">{data.schema.after}</span><br />
		</>;
	} else if (data.schema !== null && data.schema.action_type === "delete") {
		schema_change = <>
			<strong data-cy="droptype">{"Dropped " + data.object_type}</strong><br />
			<span style={{backgroundColor: "#fdb8c0"}} data-cy="dropdetail">{data.schema.before}</span><br />
		</>;
	} else if (data.schema !== null && data.schema.action_type === "modify") {
		schema_change = <>
			<strong data-cy="mod">Schema changed</strong><br />
			<span style={{backgroundColor: "#fdb8c0"}} data-cy="modbefore">{data.schema.before}</span><br />
			<span style={{backgroundColor: "#e6ffed"}} data-cy="modafter">{data.schema.after}</span><br />
		</>;
	}

	// Data changes
	let data_change = null;
	let data_change_table = null;
	if (data.data !== undefined && data.data.length) {
		data_change = <span>{data.data.length + " rows changed:"}</span>;

		const data_change_table_rows = data.data.map(d => (
			<tr>
				{d.action_type !== "add" && d.data_before ? d.data_before.map((c, index) => (
						<td style={{backgroundColor: "#ffeef0"}}>
							{!d.data_after || c !== d.data_after[index] ?
								<span style={{backgroundColor: "#fdb8c0"}}>{c}</span>
							:
								<span>{c}</span>
							}
						</td>
					))
				: null}

				{d.action_type === "add" && columnNamesBefore[data.object_name] ? columnNamesBefore[data.object_name].map(c => (<td></td>)) : null}

				{d.action_type !== "delete" && d.data_after ? d.data_after.map((c, index) => (
						<td style={{backgroundColor: "#e6ffed"}}>
							{!d.data_before || c !== d.data_before[index] ?
								<span style={{backgroundColor: "#acf2bd"}}>{c}</span>
							:
								<span>{c}</span>
							}
						</td>
					))
				: null}
			</tr>
		));

		data_change_table = (<table className="table table-sm">
			<thead>
				<tr>
					{columnNamesBefore[data.object_name] === null ? null : columnNamesBefore[data.object_name].map((c, i) => (<td>{c}</td>))}
					{columnNamesAfter[data.object_name] === null ? null : columnNamesAfter[data.object_name].map((c, i) => (<td>{c}</td>))}
				</tr>
			</thead>
			<tbody>
				{data_change_table_rows}
			</tbody>
		</table>);
	}

	return (
		<tr>
			<td data-cy="objhdr">
				<span className="fs-5" data-cy="objname">{data.object_name}</span><br />
				<span data-cy="objtype">{data.object_type}</span>
			</td>
			<td>
				<span data-cy="objdetail">
					{schema_change}
					{data_change}
				</span>
				{data_change_table}
			</td>
		</tr>
	);
}

export default function DatabaseDiff() {
	// Special case if there are no changes
	if (!diffs) {
		return <h4>No changes</h4>;
	}

	// Render table rows
	let rows = [];
	for (const [index, data] of Object.entries(diffs)) {
		rows.push(<DatabaseDiffRow data={data} />);
	}

	return (
		<table className="table table-striped table-borderless table-responsive" data-cy="difftbl">
			<tbody>
				{rows}
			</tbody>
		</table>
	);
}
