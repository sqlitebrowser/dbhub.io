const React = require("react");
const ReactDOM = require("react-dom");

import DataGrid from "react-data-grid";
import "react-data-grid/lib/styles.css";
import Select from "react-dropdown-select";

export function DatabaseDescription({oneLineDescription, sourceUrl}) {
	if (oneLineDescription === false && sourceUrl === false) {
		return;
	}

	return (
		<div className="row">
			<div className="col-md-12">
				<div className="well well-sm" style={{marginBottom: "10px", border: "1px solid #DDD", borderRadius: "7px"}}>
					{oneLineDescription ? <label id="viewdesc" data-cy="onelinedesc">{oneLineDescription}</label> : null}
					{sourceUrl ? <div><label>Source:</label> <a href={sourceUrl} data-cy="srcurl">{sourceUrl}</a></div> : null}
				</div>
			</div>
		</div>
	);
}

export function DatabaseFullDescription({description}) {
	return (
		<div className="row" style={{border: "none"}}>
			<div className="col-md-12" style={{border: "none"}}>
				<div style={{border: "1px solid #DDD", borderRadius: "7px", padding: "1px"}}>
					<table className="table table-striped table-responsive" style={{margin: 0}}><tbody>
						<tr style={{borderBottom: "1px solid #DDD"}}>
							<td className="page-header" style={{border: "none"}}><h4>DESCRIPTION</h4></td>
						</tr>
						<tr>
							<td className="rendered" id="viewreadme" data-cy="repodescrip" dangerouslySetInnerHTML={{__html: description}}></td>
						</tr>
					</tbody></table>
				</div>
			</div>
		</div>
	);
}

export function DatabaseSubMenu() {
	// The database sub menu shows links to the commits, branches, etc. pages. These do not exist (yet) for live databases
	if (meta.isLive) {
		return;
	}

	return (
		<div className="row">
			<div className="col-md-12">
				<div style={{border: "1px solid #DDD", borderRadius: "7px", marginBottom: "10px"}}>
					<table width="100%" className="table" style={{marginBottom: 0, border: "none"}}><tbody>
						<tr style={{border: "none"}}>
							<td style={{border: "none", borderRight: "1px solid #DDD"}}>
								<div style={{textAlign: "center"}}>
									<a href={"/commits/" + meta.owner + "/" + meta.database + "?branch=" + meta.branch} className="blackLink" style={{fontWeight: "bold"}} data-cy="commitslnk">Commits: <span data-cy="commitscnt">{meta.numCommits}</span></a>
								</div>
							</td>
							<td style={{border: "none", borderRight: "1px solid #DDD"}}>
								<div style={{textAlign: "center"}}>
									<a href={"/branches/" + meta.owner + "/" + meta.database} className="blackLink" style={{fontWeight: "bold"}} data-cy="brancheslnk">Branches: <span data-cy="branchescnt">{meta.numBranches}</span></a>
								</div>
							</td>
							<td style={{border: "none", borderRight: "1px solid #DDD"}}>
								<div style={{textAlign: "center"}}>
									<a href={"/tags/" + meta.owner + "/" + meta.database} className="blackLink" style={{fontWeight: "bold"}} data-cy="tagslnk">Tags: <span data-cy="tagscnt">{meta.numTags}</span></a>
								</div>
							</td>
							<td style={{border: "none", borderRight: "1px solid #DDD"}}>
								<div style={{textAlign: "center"}}>
									<a href={"/releases/" + meta.owner + "/" + meta.database} className="blackLink" style={{fontWeight: "bold"}} data-cy="rlslnk">Releases: <span data-cy="rlscnt">{meta.numReleases}</span></a>
								</div>
							</td>
							<td style={{border: "none"}}>
								<div style={{textAlign: "center"}}>
									<a href={"/contributors/" + meta.owner + "/" + meta.database} className="blackLink" style={{fontWeight: "bold"}} data-cy="contlnk">Contributors: <span data-cy="contcnt">{meta.numContributors}</span></a>
								</div>
							</td>
						</tr>
					</tbody></table>
				</div>
			</div>
		</div>
	);
}

export function DatabaseActions({table, setTable, setBranch}) {
	// This function generates custom render function for rendering the dropdown field for table and branch selection.
	// It prints the currently selected item name as usual but adds a label as a prefix
	const dropdownContentRendererWithLabel = function(label) {
		return function({props, state}) {
			return (
				<div style={{cursor: "pointer"}}>
					<span style={{fontWeight: "bold"}}>{label}:</span> {state.values.length ? state.values[0].name : null}
				</div>
			);
		};
	};

	// Copy value of an input element to the system clipboard
	function copyToClipboard(element_id) {
		let e = document.getElementById(element_id);
		e.select();
		e.setSelectionRange(0, 99999);
		document.execCommand("copy");
	}

	// Dropdown input for selecting the current table
	let tables = [];
	meta.tableList.forEach(function(v) {
		tables.push({name: v});
	});
	const tableSelection = (
		<div style={{display: "inline-block"}}>
			<Select name="viewtable" required={true} labelField="name" valueField="name" onChange={(values) => setTable(values[0].name)} options={tables} values={[{name: table}]} contentRenderer={dropdownContentRendererWithLabel("Table/view")} />
		</div>
	);

	// Dropdown input for selecting the current branch (not available for live databases)
	let branchSelection = null;
	if (meta.isLive === false) {
		let branches = [];
		meta.branchList.forEach(function(v) {
			branches.push({name: v});
		});
		branchSelection = (
			<div style={{display: "inline-block"}}>
				<Select name="viewbranch" required={true} labelField="name" valueField="name" onChange={(values) => setBranch(values[0].name)} options={branches} values={[{name: meta.branch}]} contentRenderer={dropdownContentRendererWithLabel("Branch")} />
			</div>
		);
	}

	return (
		<div className="row" style={{paddingBottom: "10px"}}>
			<div className="col-md-8">
				<span className="pull-left" style={{whiteSpace: "nowrap"}}>
					{tableSelection}&nbsp;
					{branchSelection}&nbsp;
					{authInfo.loggedInUser && !meta.isLive ? <a href={"/compare/" + meta.owner + "/" + meta.database} className="btn btn-primary" data-cy="newmrbtn">New Merge Request</a> : null}&nbsp;
					{authInfo.loggedInUser && !meta.isLive ? <a href={"/upload/?username=" + meta.owner + "&dbname=" + meta.database + "&branch=" + meta.branch} className="btn btn-primary" data-cy="uploadbtn">Upload database</a> : null}
				</span>
			</div>
			{meta.isLive === false ? (
				<div className="col-md-4">
					<span className="pull-right">
						<div className="btn-group">
							<button type="button" className="btn btn-success dropdown-toggle" data-toggle="dropdown" aria-haspopup="true" aria-expanded="false">
								Clone database in DB4S <span className="caret"></span>
							</button>
							<ul className="dropdown-menu">
								<li><input type="text" value={"https://" + db4s.server + (db4s.port === 443 ? null : (":" + db4s.port)) + "/" + meta.owner + "/" + meta.database + "?commit=" + meta.commitID + "&branch=" + meta.branch} id="db4sCloneLink" readOnly /></li>
								<li><a href="#" onClick={() => copyToClipboard('db4sCloneLink')}>Copy link <span className="glyphicon glyphicon-link"></span></a></li>
							</ul>
						</div>&nbsp;
						<div className="btn-group" data-cy="dldropdown">
							<button type="button" className="btn btn-success dropdown-toggle" data-toggle="dropdown" aria-haspopup="true" aria-expanded="false">
								Download database <span className="caret"></span>
							</button>
							<ul className="dropdown-menu">
								<li><a href={"/x/download/" + meta.owner + "/" + meta.database + "?commit=" + meta.commitID} data-cy="dldb">Entire database ({Math.round(meta.size / 1024).toLocaleString()} KB)</a></li>
								{meta.size <= 100000000 ? <li><a href={"/x/downloadcsv/" + meta.owner + "/" + meta.database + "?commit=" + meta.commitID + "&table=" + table} data-cy="dlcsv">Selected table as CSV</a></li> : null}
							</ul>
						</div>
					</span>
				</div>
			) : null}
		</div>
	);
}

function DatabasePageControls({offset, maxRows, rowCount, setOffset}) {
	// Returns a text string with row count information for the table
	function totalRowCountText(offset, maxRows, rowCount) {
		// Update the end value if it's pointing past the last row
		let end = offset + maxRows;
		if (end > rowCount) {
			end = rowCount;
		}

		return offset.toLocaleString() + "-" + end.toLocaleString() + " of " + rowCount.toLocaleString() + " total rows";
	}

	return (
		<div className="row">
			<div className="col-md-12">
				<div style={{maxWidth: "100%", overflow: "auto", border: "1px solid #DDD", borderRadius: "0 0 7px 7px"}}>
					<table className="table table-responsive" style={{margin: 0}}>
						<thead>
							<tr>
								<th style={{textAlign: "center", padding: 0}}>
									{offset > 0 ? (<>
										<span style={{fontSize: "x-large", verticalAlign: "middle", marginBottom: "10px"}}>
											<a href="#" style={{color: "black", textDecoration: "none"}} onClick={() => setOffset(0)} data-cy="firstpgbtn">⏮</a>
										</span>
										<span style={{fontSize: "x-large", verticalAlign: "middle", marginBottom: "10px"}}>
											<a href="#" style={{color: "black", textDecoration: "none"}} onClick={() => setOffset(offset - maxRows)} data-cy="pgupbtn">⏴</a>
										</span>
									</>) : null}
									<span style={{verticalAlign: "middle"}}>{totalRowCountText(offset, maxRows, rowCount)}</span>
									{offset + maxRows < rowCount ? (<>
										<span style={{fontSize: "x-large", verticalAlign: "middle", marginBottom: "10px"}}>
											<a href="#" style={{color: "black", textDecoration: "none"}} onClick={() => setOffset(offset + maxRows)} data-cy="pgdnbtn">⏵️</a>
										</span>
										<span style={{fontSize: "x-large", verticalAlign: "middle", marginBottom: "10px"}}>
											<a href="#" style={{color: "black", textDecoration: "none"}} onClick={() => setOffset(rowCount - maxRows)} data-cy="lastpgbtn">⏭</a>
										</span>
									</>) : null}
								</th>
							</tr>
						</thead>
					</table>
				</div>
			</div>
		</div>
	);
}

function DataGridNoRowsRender() {
	return <div style={{textAlign: "center", gridColumn: "1/-1"}}><i>This table is empty</i></div>;
}

export default function DatabaseView() {
	const [table, setTable] = React.useState("");
	const [columns, setColumns] = React.useState([]);
	const [records, setRecords] = React.useState([]);
	const [offset, setOffset] = React.useState(0);
	const [maxRows, setMaxRows] = React.useState(meta.maxRows);
	const [rowCount, setRowCount] = React.useState(0);
	const [sortColumns, setSortColumns] = React.useState([]);

	// Retrieves the branch being viewed
	function changeBranch(newbranch) {
		window.location = "/" + meta.owner + "/" + meta.database + "?branch=" + newbranch;
	}

	// Retrieves table data for a different table or offset
	function changeView(newTable, newOffset, newSortCol, newSortDir) {
		// If neither table nor offset have changed do nothing
		if (table === newTable && offset === newOffset && sortColumns.length && sortColumns[0].columnKey === newSortCol && sortColumns[0].direction === newSortDir) {
			return;
		}

		// We do not need to check the value in newOffset here. It is checked on the server-side application
		// and the corrected offset is reported back by the server

		fetch("/x/table/" + meta.owner + "/" + meta.database + "?commit=" + meta.commitID + "&table=" + newTable + "&sort=" + (newSortCol ? newSortCol : "") + "&dir=" + (newSortDir ? newSortDir : "") + "&offset=" + newOffset)
			.then((response) => response.json())
			.then(function (data) {
				// Convert data to format required by grid view
				// TODO Just deliver the data in the right format to begin with
				let cols = [];
				data.ColNames.forEach(function(c) {
					// Remove the rowid column if it was added by the server
					if (c !== "rowid") {
						// Add the column
						cols.push({
							key: c,
							name: c,
							formatter: (props) => {
								if (props.row[c] === null) {
									return <i>NULL</i>;
								} else {
									return props.row[c];
								}
							},
						});
					}
				});

				let rows = [];
				if  (data.Records !== null) {
					data.Records.forEach(function(r) {
						let row = {};
						r.forEach(function(c) {
							row[c.Name] = c.Value;
						});
						rows.push(row);
					});
				}
				setRecords(rows);
				setColumns(cols);

				// Update table information
				setTable(data.Tablename);
				setOffset(data.Offset);
				setRowCount(data.RowCount);
				setSortColumns([{columnKey: data.SortCol, direction: data.SortDir}]);
			});
	}

	// Initial load of the first table when first rendering the component
	React.useEffect(() => {
		// If provided, we use the values from the URL as default parameters
		let urlParams = new URL(window.location.href).searchParams;
		let urlTable = urlParams.get("table");

		if (urlTable === null) {
			// If no table parameter has been provided, show the default table without any specific search order
			changeView(meta.defaultTable, 0);
		} else {
			let urlOffset = parseInt(urlParams.get("offset"));
			let urlSort = urlParams.get("sort");
			let urlDir = urlParams.get("dir");
			changeView(urlTable, urlOffset ? urlOffset : 0, urlSort, urlDir);
		}
	}, []);

	// Convert data to format required by grid view
	// TODO Just deliver the data in the right format to begin with
	let cols = [];
	columns.forEach(function(c) {
		cols.push({
			key: c,
			name: c
		});
	});

	let rows = [];
	records.forEach(function(r) {
		let row = {};
		r.forEach(function(c) {
			row[c.Name] = c.Value;
		});
		rows.push(row);
	});

	return (<>
		<DatabaseDescription oneLineDescription={meta.oneLineDescription} sourceUrl={meta.sourceUrl} />
		<DatabaseSubMenu />
		<DatabaseActions
			table={table}
			setBranch={changeBranch}
			setTable={(newTable) => {if (newTable !== table) { changeView(newTable, 0); }}}
		/>
		<DataGrid
			className="rdg-light"
			renderers={{noRowsFallback: <DataGridNoRowsRender />}}
			columns={cols}
			rows={rows}
			sortColumns={sortColumns}
			onSortColumnsChange={(s) => changeView(table, offset, s.length ? s[0].columnKey : null, s.length ? s[0].direction : null)}
			defaultColumnOptions={{
				sortable: true,
				resizable: true
			}}
		/>
		<DatabasePageControls offset={offset} maxRows={maxRows} rowCount={rowCount} setOffset={(newOffset) => changeView(table, newOffset, sortColumns.length ? sortColumns[0].columnKey : null, sortColumns.length ? sortColumns[0].direction : null)} />
		<div className="row" style={{border: "none"}}>&nbsp;</div>
		<DatabaseFullDescription description={meta.fullDescription} />
		<div className="row" style={{border: "none"}}>&nbsp;</div>
	</>);
}
