const React = require("react");
const ReactDOM = require("react-dom");

function DatabaseForkRow({data}) {
	// Display the appropriate fork icons for a database row
	function rowIcons(row) {
		let returnList = "";
		row.icon_list.forEach(item => {
			switch (item) {
				case 0:
					returnList += "  "; // Space
					break;
				case 1:
					returnList += "🜷"; // Root
					break;
				case 2:
					returnList += "┃ "; // Stem
					break;
				case 3:
					returnList += "┣ "; // Branch
					break;
				case 4:
					returnList += "┗"; // End
					break;
				default:
					returnList += "?"; // Unknown.  This shouldn't happen. ;)
			}
		});
		return returnList;
	}

	// Ensure private and deleted databases display appropriately
	function rowUrl(row) {
		// Simple placeholder for deleted databases
		if (row.deleted === true) {
			return "deleted database";
		}

		// Create appropriate link or placeholder for public/private databases
		if (row.public === true) {
			return <a href={"/" + row.database_owner + "/" + row.database_name}>{row.database_name}</a>;
		} else if (row.database_owner === authInfo.loggedInUser) {
			// The logged in user should see their own private databases. Make sure it's not mistaken as public though.
			return <><a href={"/" + row.database_owner + "/" + row.database_name}>{row.database_name}</a> (private database)</>;
		} else {
			return "private database";
		}
	}

	return (<p>
		<pre className="font-monospace d-inline fs-5">{rowIcons(data)}</pre>&nbsp;
		<a href={"/" + data.database_owner}>{data.database_owner}</a> /&nbsp;
		{rowUrl(data)}
	</p>);
}

export default function DatabaseForks() {
	// Render table rows
	let rows = [];
	for (const [index, data] of Object.entries(forksData)) {
		rows.push(<DatabaseForkRow data={data} />);
	}

	return (<>
		<h3 className="text-center">
			<span data-cy="forks">Forks of</span> <a href={"/" + meta.owner} data-cy="ownerlnk">{meta.owner}</a> /&nbsp;
			<a href={"/" + meta.owner + "/" + meta.database} data-cy="dblnk">{meta.database}</a>
		</h3>
		{rows}
	</>);
}
