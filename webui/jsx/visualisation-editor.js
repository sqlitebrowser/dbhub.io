const React = require("react");
const ReactDOM = require("react-dom");

// Import the Plotly basic bundle for reduced size
import Plotly from "plotly.js-basic-dist";
import createPlotlyComponent from "react-plotly.js/factory";
const Plot = createPlotlyComponent(Plotly);

import Tab from "react-bootstrap/Tab";
import Tabs from "react-bootstrap/Tabs";
import Editor from "react-simple-code-editor";
import {highlight, languages} from "prismjs/components/prism-core";
import "prismjs/components/prism-sql";
import "prismjs/themes/prism-solarizedlight.css";
import {format} from "sql-formatter";
import Select from "react-dropdown-select";
import { confirmAlert } from "react-confirm-alert";
import "react-confirm-alert/src/react-confirm-alert.css";
import { convertResultsetToCsv, downloadData } from "./export-data";

function SavedVisualisationItem({name, visStatus, active, onSelectItem, onSaveItem, onDeleteItem, onRenameItem}) {
	const [savedName, setSavedName] = React.useState(name);
	const [editingName, setEditingName] = React.useState(false);
	const [editedName, setEditedName] = React.useState(name);

	// When the name for this item is updated, refresh the saved state. This is important when the items are reordered due to renaming
	React.useEffect(() => {
		setEditedName(name);
		setSavedName(name);
	}, [name]);

	// Is this the selected item? For the selected item we show a couple more options
	if (active) {
		// For the active item it is also possible to edit the name. In this case show a text input
		if (editingName) {
			return (
				<div key={name} className="list-group-item active">
					<form onSubmit={event => {
						event.preventDefault();
						if (savedName === editedName) {
							setEditingName(false);
							return;
						}
						if (onRenameItem(savedName, editedName) !== false) {
							setEditingName(false);
							setSavedName(editedName);
						}
					}}>
						<div className="input-group">
							<input type="text" className="form-control form-control-sm" data-cy="nameinput" value={editedName} onChange={e => setEditedName(e.target.value)} />
							<button type="submit" className="btn btn-secondary btn-sm" disabled={editedName.trim() === "" ? "disabled" : null} data-cy="renameokbtn">Save</button>
							<button type="button" className="btn btn-secondary btn-sm" onClick={() => {setEditedName(savedName); setEditingName(false);}}>Cancel</button>
						</div>
					</form>
				</div>
			);
		} else {
			return (
				<a key={name} className="list-group-item list-group-item-action active" data-cy="selectedvis">
					<span onClick={e => {e.detail === 2 && setEditingName(true)}}>{visStatus.dirty || visStatus.newlyCreated ? <i>{savedName + " [" + (visStatus.newlyCreated ? "unsaved" : "modified") + "]"}</i> : savedName}</span>
					<div className="pull-right">
						{authInfo.loggedInUser === meta.owner && (visStatus.dirty || visStatus.newlyCreated) ? <button type="button" className="btn btn-primary btn-sm" title="Save visualisation" data-cy="savebtn" onClick={() => onSaveItem(savedName)}><i className="fa fa-save" /></button> : null}
						{authInfo.loggedInUser === meta.owner || visStatus.newlyCreated ? <button type="button" className="btn btn-primary btn-sm" title="Rename visualisation" data-cy="renamebtn" onClick={() => setEditingName(true)}><i className="fa fa-edit" /></button> : null}
						{authInfo.loggedInUser === meta.owner || visStatus.newlyCreated ? <button type="button" className="btn btn-primary btn-sm" title="Delete visualisation" data-cy="deletebtn" onClick={() => onDeleteItem(savedName)}><i className="fa fa-trash" /></button> : null}
					</div>
				</a>
			);
		}
	} else {
		return <a href="#" key={name} className="list-group-item list-group-item-action" onClick={() => onSelectItem(savedName)}>{visStatus.dirty || visStatus.newlyCreated ? <i>{savedName + " [" + (visStatus.newlyCreated ? "unsaved" : "modified") + "]"}</i> : savedName}</a>;
	}
}

function SavedVisualisations({visualisations, visualisationsStatus, preselectedVisualisation, onSelectVisualisation, onNewVisualisation, onSaveVisualisation, onDeleteVisualisation, onRenameVisualisation}) {
	const [selectedVisualisation, setSelectedVisualisation] = React.useState(preselectedVisualisation);

	// When the selected visualisation is changed by an external event, update the local status accordingly
	React.useEffect(() => {
		setSelectedVisualisation(preselectedVisualisation);
	}, [preselectedVisualisation]);

	// When the selected visualisation is changed, call the callback handler
	React.useEffect(() => {
		if (onSelectVisualisation !== undefined) {
			onSelectVisualisation(selectedVisualisation);
		}
	}, [selectedVisualisation]);

	return (
		<div>
			<h4>
				Saved visualisations&nbsp;
				<button type="button" className="btn btn-success btn-sm pull-right" data-cy="newvisbtn" onClick={() => onNewVisualisation()}><i className="fa fa-plus" />&nbsp;New</button>
			</h4>
			<div className="list-group" data-cy="savedvis">
				{
					Object.keys(visualisations).length === 0 ? <i>None yet</i> :
					Object.keys(visualisations).map(name => {
						return <SavedVisualisationItem name={name} visStatus={visualisationsStatus[name]} active={name === selectedVisualisation} onSelectItem={setSelectedVisualisation} onSaveItem={onSaveVisualisation} onDeleteItem={onDeleteVisualisation} onRenameItem={onRenameVisualisation} />;
					})
				}
			</div>
		</div>
	);
}

export function Visualisation({name, plotConfig, branch, setRawData, setLastRunResultMessage}) {
	const [state, setState] = React.useState("new");
	const [data, setData] = React.useState(null);

	// Retrieve data from the server whenever the plot config changes
	React.useEffect(() => {
		// Check if there is any SQL to send
		if (!plotConfig || plotConfig.sql.trim() === "") {
			setState("nosql");
			if (setLastRunResultMessage !== undefined) {
				setLastRunResultMessage(null);
			}
			return;
		}

		// We're loading data
		setState("loading");
		if (setLastRunResultMessage !== undefined) {
			setLastRunResultMessage("loading...");
		}

		// Send the SQL string to the backend
		fetch("/x/execsql/" + meta.owner + "/" + meta.database + (branchData && branch in branchData ? ("?commit=" + branchData[branch].commit) : ""), {
			method: "post",
			headers: {"Content-Type": "application/json"},
			body: JSON.stringify({sql: plotConfig.sql}),
		}).then(response => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			// Data has been successfully retrieved, so convert it, store it and update the state
			response.json().then(newData => {
				setState("done");

				if (setRawData !== undefined) {
					setRawData(newData);
				}

				if (setLastRunResultMessage !== undefined) {
					setLastRunResultMessage("successfully executed at " + (new Date().toLocaleTimeString()) + ", returning " + newData.Records.length + " row" + (newData.Records.length !== 1 ? "s" : ""));
				}

				// Convert data returned by server to the format expected by Plotly
				let plotData = {};
				if (newData.Records !== null) {
					// Figure out the column indexes in newData.Records for the X and Y columns
					const xColumnIndex = newData.ColNames.findIndex(e => e === plotConfig.x_axis_label);
					const yColumnIndex = newData.ColNames.findIndex(e => e === plotConfig.y_axis_label);
					if (xColumnIndex === -1 || yColumnIndex === -1) {
						setState("error");
						setData("unknown column selected for plot");
						return;
					}

					// Organise chart data to suit the selected chart type
					switch (plotConfig.chart_type) {
						case "vbc":	// Vertical bar chart
						case "lc":	// Line chart
							plotData.x = newData.Records.map(r => r[xColumnIndex].Value);
							plotData.y = newData.Records.map(r => r[yColumnIndex].Value);
							plotData.type = plotConfig.chart_type === "lc" ? "scatter" : "bar";
							plotData.orientation = "v";
							break;
						case "hbc":	// Horizontal bar chart
							plotData.y = newData.Records.map(r => r[xColumnIndex].Value);
							plotData.x = newData.Records.map(r => r[yColumnIndex].Value);
							plotData.type = "bar";
							plotData.orientation = "h";
							break;
						case "pie":	// Pie chart
							plotData.labels = newData.Records.map(r => r[xColumnIndex].Value);
							plotData.values = newData.Records.map(r => r[yColumnIndex].Value);
							plotData.type = "pie";
							plotData.orientation = undefined;
							break;
					}
				} else {
					setState("nodata");
				}
				setData(plotData);
			});
		}).catch(error => {
			error.text().then(text => {
				setState("error");
				setData(text);
				setRawData(null);

				if (setLastRunResultMessage !== undefined) {
					setLastRunResultMessage("error executing query at " + (new Date().toLocaleTimeString()) + ": " + text);
				}
			});
		});
	}, [plotConfig, branch]);

	// Depending on the current state of the component a different type of output is rendered
	if (state === "nosql") {
		// There is no SQL statement to execute, so display an info box
		return <div className="alert alert-info" role="alert">Please type in a SQL query and execute it to show a chart here.</div>;
	} else if (state === "new" || state === "loading") {
		// Data is still being retrieved, so display a loading indicator
		return <div className="alert alert-info" role="alert">Retrieving data...</div>;
	} else if (state === "error") {
		// Retrieving data failed, so display the returned error message and no graph
		return <div className="alert alert-danger" role="alert">Retrieving data failed: {data}</div>;
	} else if(state === "nodata") {
		// The server responded but the query did not return any records to plot
		return <div className="alert alert-info" role="alert">SQL query ran without error, but returned no records</div>;
	} else if(state === "done") {
		// The server responsed and the query returned some records to plot
		return (
			<Plot
				data={[data]}
				layout={{
					autosize: true,
					title: name,
					xaxis: {visible: plotConfig?.show_x_label, title: plotConfig?.chart_type === "hbc" ? plotConfig?.y_axis_label : plotConfig?.x_axis_label},
					yaxis: {visible: plotConfig?.show_y_label, title: plotConfig?.chart_type === "hbc" ? plotConfig?.x_axis_label : plotConfig?.y_axis_label},
				}}
				config={{
					displaylogo: false,
					watermark: false,
				}}
				useResizeHandler={true}
				style={{width: "100%"}}
			/>
		);
	}
}

export function VisualisationEditor() {
	const [selectedBranch, setSelectedBranch] = React.useState(meta.branch);
	const [visualisations, setVisualisations] = React.useState(visualisationsData);
	const [visualisationsStatus, setVisualisationsStatus] = React.useState(Object.fromEntries(Object.keys(visualisationsData).map(k => [k, {dirty: false, newlyCreated: false, code: visualisationsData[k].sql}])));
	const [selectedVisualisation, setSelectedVisualisation] = React.useState("");
	const [rawData, setRawData] = React.useState(null);
	const [showDataTable, setShowDataTable] = React.useState(false);
	const [showEmbedHtml, setShowEmbedHtml] = React.useState(false);
	const [lastRunResultMessage, setLastRunResultMessage] = React.useState(null);

	// When the selected saved visualisation is changed, update the chart settings controls
	React.useEffect(() => {
		setRawData(null);
		setShowEmbedHtml(false);
	}, [selectedVisualisation]);

	// When the data is updated, check if the currently selected plot columns still exist
	React.useEffect(() => {
		// Do nothing if there is no data, if no visualisation is selected, or if the query did not return and columns which we could
		// automatically select here,
		if (rawData === null || visualisations[selectedVisualisation] === undefined || rawData.ColNames.length === 0) {
			return;
		}

		// Check x axis column
		if (rawData.ColNames.indexOf(visualisations[selectedVisualisation].x_axis_label) === -1) {
			updatePlotConfig({x_axis_label: rawData.ColNames[0]});
		}

		// Check y axis column
		if (rawData.ColNames.indexOf(visualisations[selectedVisualisation].y_axis_label) === -1) {
			updatePlotConfig({y_axis_label: rawData.ColNames[0]});
		}
	}, [rawData]);

	// Register a handler for the beforeunload event of the page. This allows us to show a warning before the user
	// is leaving this page in case there are unsaved changes.
	React.useEffect(() => {
		const handler = (e) => {
			// Check if there are any unsaved changes. If not, do nothing
			if (Object.values(visualisationsStatus).find(v => v.dirty || v.newlyCreated) === undefined) {
				return;
			}

			// Return a string to show a warning message box to the user. Since most browsers do not show this
			// string to the user anyway we do not bother with a specific message here.
			return (e.returnValue = "x");
		};

		window.addEventListener("beforeunload", handler);
		return () => window.removeEventListener("beforeunload", handler);
	}, [visualisationsStatus]);

	// Format the current SQL code
	function formatSql() {
		// For parse errors this throws an exception
		try {
			updateVisualisationStatus({
				code: format(visualisationsStatus[selectedVisualisation].code, {
					language: "sqlite",
				}),
				dirty: true,
			});
		} catch(e) {
			alert(e);
		}
	}

	// Modify the status attributes of the currently selected visualisation
	function updateVisualisationStatus(update) {
		const vis = {[selectedVisualisation]: {...visualisationsStatus[selectedVisualisation], ...update}};
		setVisualisationsStatus({
			...visualisationsStatus,
			...vis
		});
	}

	// This updates the current plot config triggering a redraw of the plot
	function updatePlotConfig(update, force) {
		update = update || {};

		// Check for modifications
		let modified = false;
		for (const [attr, val] of Object.entries(update)) {
			if (visualisations[selectedVisualisation][attr] !== val) {
				modified = true;
				break;
			}
		}

		 // Do nothing if the attribute values have not changed
		if (force !== true && modified === false) {
			return;
		}

		// Set the unsaved attribute when this update is modifying the configuration to indicate that there are unsaved changes now
		if (modified === true) {
			updateVisualisationStatus({dirty: true});
		}

		// Update the current plot config state
		const vis = {[selectedVisualisation]: {...visualisations[selectedVisualisation], ...update}};
		setVisualisations({
			...visualisations,
			...vis
		});
	}

	// Create a new empty visualisation
	function newVisualisation() {
		// Find an unused visualisation name
		let counter = 0;
		do {
			counter += 1;
		} while(("new " + counter) in visualisations);
		const name = "new " + counter;

		// Prepare new visualisation config
		const newConfig = {
			[name]: {
				sql: "",
				chart_type: "vbc",
				show_x_label: true,
				show_y_label: true,
			}
		};

		// Add config
		setVisualisations(visualisations => ({
			...visualisations,
			...newConfig
		}));

		// Prepare new status object
		const newStatus = {
			[name]: {
				code: "",
				dirty: true,		// This visualisation does not exist on the server (yet), so mark
				newlyCreated: true,	// it as unsaved and as never saved
			}
		};

		// Add status
		setVisualisationsStatus(visualisationsStatus => ({
			...visualisationsStatus,
			...newStatus
		}));

		// Select new visualisation
		setSelectedVisualisation(name);
	}

	// Save a visualisation in the database
	function saveVisualisation(name) {
		// Save visualisation on server
		fetch("/x/vissave/" + meta.owner + "/" + meta.database + "?visname=" + name, {
			method: "post",
			headers: {"Content-Type": "application/json"},
			body: JSON.stringify(visualisations[name]),
		}).then(response => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			// Visualisation has been successfully saved, so unset the dirty and/or the newlyCreated status attributes
			updateVisualisationStatus({dirty: false, newlyCreated: false});
		}).catch(error => {
			confirmAlert({
				title: "Error",
				message: "Saving the visualisation failed. Please check all information is valid.",
				buttons: [{label: "OK"}],
			});
		});
	}

	// Delete a visualisation
	function deleteVisualisation(name) {
		const deleteVisLocal = (vis) => {
			// Unselect it if it is the currently selected visualisation
			if (selectedVisualisation === name) {
				setSelectedVisualisation("");
			}

			// Delete visualisation object
			const newVis = {...visualisations};
			delete newVis[name];
			setVisualisations(newVis);

			// Delete status object
			const newStat = {...visualisationsStatus};
			delete newStat[name];
			setVisualisationsStatus(newStat);
		};

		// Has this been saved to the server yet?
		if (visualisationsStatus[name].newlyCreated) {
			// No, so just delete it locally
			deleteVisLocal(name);
		} else {
			// Yes, so delete it on the server first
			fetch("/x/visdel/" + meta.owner + "/" + meta.database + "?visname=" + name, {
				method: "post",
				headers: {"Content-Type": "application/json"},
			}).then(response => {
				if (!response.ok) {
					return Promise.reject(response);
				}

				deleteVisLocal(name);
			}).catch(error => {
				confirmAlert({
					title: "Error",
					message: "Deleting the visualisation failed.",
					buttons: [{label: "OK"}],
				});
			});
		}
	}

	// Ask for confirmation to delete a saved visualisation
	function askDeleteVisualisation(name) {
		confirmAlert({
			title: "Confirm delete",
			message: "Are you sure you want to delete the visualisation '" + name + "'?",
			buttons: [
				{
					label: "Yes",
					onClick: () => {
						deleteVisualisation(name);
					}
				},
				{
					label: "No"
				}
			]
		});
	}

	// Rename an existing visualisation
	function renameVisualisation(oldName, newName) {
		// Empty names are not allowed
		if (newName === "") {
			return false;
		}

		// Check if a visualisation of the new name already exists
		if (newName in visualisations) {
			confirmAlert({
				title: "Error",
				message: "A visualisation with that name already exists.",
				buttons: [{label: "OK"}],
			});
			return false;
		}

		// Helper function for renaming the visualisation on the client side
		const renameVisLocal = (oldVisName, newVisName) => {
			// Rename visualisation object
			const newVis = {...visualisations, [newVisName]: visualisations[oldVisName]};
			delete newVis[oldVisName];
			setVisualisations(newVis);

			// Rename visualisation status
			const newStat = {...visualisationsStatus, [newVisName]: visualisationsStatus[oldVisName]};
			delete newStat[oldVisName];
			setVisualisationsStatus(newStat);

			// Unselect it if it is the currently selected visualisation
			if (selectedVisualisation === oldVisName) {
				setSelectedVisualisation(newVisName);
			}

		};

		// Does this visusalisation exist on the server yet?
		if (visualisationsStatus[oldName].newlyCreated) {
			renameVisLocal(oldName, newName);
		} else {
			// Rename the visualisation on the server
			fetch("/x/visrename/" + meta.owner + "/" + meta.database + "?visname=" + oldName + "&visnewname=" + newName, {
				method: "post",
			}).then(response => {
				if (!response.ok) {
					return Promise.reject(response);
				}

				renameVisLocal(oldName, newName);
			}).catch(error => {
				confirmAlert({
					title: "Error",
					message: "Renaming the visualisation failed.",
					buttons: [{label: "OK"}],
				});
			});
		}
	}

	// Exports the current data table to CSV
	function exportDataTable() {
		const data = convertResultsetToCsv(rawData);
		downloadData(data, selectedVisualisation + ".csv", "text/csv");
	}

	// Prepare branch and visualisations list data
	let branches = [];
	if (branchData !== null) {
		for (const [name, data] of Object.entries(branchData)) {
			branches.push({name: name});
		}
	}

	// List of chart types for the chart type dropdown element
	const chartTypes = [
		{value: "hbc", label: "Horizontal bar chart"},
		{value: "vbc", label: "Vertical bar chart"},
		{value: "lc", label: "Line chart"},
		{value: "pie", label: "Pie chart"},
	];
	const selectedChartType = chartTypes.find(t => {return t.value === (visualisations[selectedVisualisation]?.chart_type || "vbc")});

	// List of data columns for the chart axis dropdown elements
	const columnList = rawData === null ? [] : rawData.ColNames.map(c => new Object({name: String(c)}));

	return (<>
		{meta.isLive === false ? (
			<div className="row">
				<div className="col-md-12">
					<div className="mb-2">
						<label htmlFor="viewbranch">Branch:</label>&nbsp;
						<div className="d-inline-block">
							<Select name="viewbranch" required={true} labelField="name" valueField="name" onChange={(values) => setSelectedBranch(values[0].name)} options={branches} values={[{name: selectedBranch}]} />
						</div>
					</div>
				</div>
			</div>
		) : null}
		<div className="row">
			<div className="col-md-2">
				<SavedVisualisations
					visualisations={visualisations}
					visualisationsStatus={visualisationsStatus}
					preselectedVisualisation={selectedVisualisation}
					onSelectVisualisation={setSelectedVisualisation}
					onNewVisualisation={newVisualisation}
					onSaveVisualisation={saveVisualisation}
					onDeleteVisualisation={askDeleteVisualisation}
					onRenameVisualisation={renameVisualisation}
				/>
			</div>
			<div className="col-md-10">
				{selectedVisualisation === "" ? (
					<div className="alert alert-info" role="alert">Select one of the saved visualisations or create a new visualisation.</div>
				) : (<>
					<Tabs>
						<Tab eventKey="chart" title="Chart">
							<Visualisation name={selectedVisualisation} plotConfig={visualisations[selectedVisualisation]} branch={selectedBranch} setRawData={setRawData} setLastRunResultMessage={setLastRunResultMessage} />
							{rawData !== null ? (<>
								<button type="button" className={"btn btn-secondary" + (showDataTable ? " active" : "")} onClick={() => setShowDataTable(!showDataTable)}>{showDataTable ? "Hide data table" : "Show data table"}</button>&nbsp;
								<button type="button" className="btn btn-secondary" onClick={() => exportDataTable()}>Export to CSV</button>
								{visualisationsStatus[selectedVisualisation].newlyCreated ? null :
									<>&nbsp;<button type="button" className={"btn btn-secondary" + (showEmbedHtml ? " active" : "")} onClick={() => setShowEmbedHtml(!showEmbedHtml)}>{showEmbedHtml ? "Hide embedding" : "Embed chart"}</button></>
								}
							</>) : null}
						</Tab>
						<Tab eventKey="sql" title="SQL">
							<Editor
								name="usersql"
								value={visualisationsStatus[selectedVisualisation].code}
								onValueChange={text => updateVisualisationStatus({code: text, dirty: true})}
								highlight={text => highlight(text, languages.sql)}
								placeholder="Your SQL query here..."
								style={{
									fontFamily: "monospace",
									fontSize: "14px",
									height: "200px",
									border: "1px solid grey",
								}}
							/>
							<div className="mt-2">
								<button type="button" className="btn btn-success" onClick={() => updatePlotConfig({sql: visualisationsStatus[selectedVisualisation].code}, true)} data-cy="runsqlbtn" disabled={visualisationsStatus[selectedVisualisation].code.trim() === "" ? "disabled" : null}>Run SQL</button>&nbsp;
								<button type="button" className="btn btn-light" value="Format SQL" onClick={formatSql} data-cy="formatsqlbtn">Format SQL</button>&nbsp;
								<button type="button" className={"btn btn-light" + (showDataTable ? " active" : "")} onClick={() => setShowDataTable(!showDataTable)}>{showDataTable ? "Hide data table" : "Show data table"}</button>&nbsp;
								<span data-cy="statusmsg">{lastRunResultMessage}</span>
							</div>
						</Tab>
						<Tab eventKey="settings" title="Chart settings">
							<form>
								<div className="row mt-1 mb-2">
									<label htmlFor="charttype" className="col-sm-2 col-form-label">Chart type</label>
									<div className="col-sm-10">
										<Select name="charttype" required={true} onChange={values => updatePlotConfig({chart_type: values[0].value})} options={chartTypes} values={[selectedChartType]} />
									</div>
								</div>
								<div className="row mb-2">
									<label htmlFor="xaxiscol" className="col-sm-2 col-form-label">X axis column</label>
									<div className="col-sm-10">
										<Select name="xaxiscol" required={true} labelField="name" valueField="name" onChange={values => updatePlotConfig({x_axis_label: values[0].name})} options={columnList} values={[{name: visualisations[selectedVisualisation]?.x_axis_label}]} />
									</div>
								</div>
								<div className="row mb-2">
									<label htmlFor="yaxiscol" className="col-sm-2 col-form-label">Y axis column</label>
									<div className="col-sm-10">
										<Select name="yaxiscol" required={true} labelField="name" valueField="name" onChange={values => updatePlotConfig({y_axis_label: values[0].name})} options={columnList} values={[{name: visualisations[selectedVisualisation]?.y_axis_label}]} />
									</div>
								</div>
								{visualisations[selectedVisualisation]?.chart_type !== "pie" ? (<>
									<div className="row mb-2">
										<label htmlFor="showxaxis" className="col-sm-2 col-form-label">Show X axis</label>
										<div className="col-sm-10">
											<div className="btn-group" role="group">
												<input type="radio" className="btn-check" name="showxaxis" autocomplete="off" checked={visualisations[selectedVisualisation]?.show_x_label} value="true" />
												<label className="btn btn-outline-secondary" htmlFor="showxaxis" onClick={() => updatePlotConfig({show_x_label: true})} data-cy="xtruetoggle">Yes</label>
												<input type="radio" className="btn-check" name="showxaxis" autocomplete="off" checked={!visualisations[selectedVisualisation]?.show_x_label} value="false" />
												<label className="btn btn-outline-secondary" htmlFor="showxaxis" onClick={() => updatePlotConfig({show_x_label: false})} data-cy="xfalsetoggle">No</label>
											</div>
										</div>
									</div>
									<div className="row mb-2">
										<label htmlhtmlFor="showyaxis" className="col-sm-2 col-form-label">Show Y axis</label>
										<div className="col-sm-10">
											<div className="btn-group" role="group">
												<input type="radio" className="btn-check" name="showyaxis" autocomplete="off" checked={visualisations[selectedVisualisation]?.show_y_label} value="true" />
												<label className="btn btn-outline-secondary" htmlFor="showyaxis" onClick={() => updatePlotConfig({show_y_label: true})} data-cy="ytruetoggle">Yes</label>
												<input type="radio" className="btn-check" name="showyaxis" autocomplete="off" checked={!visualisations[selectedVisualisation]?.show_y_label} value="false" />
												<label className="btn btn-outline-secondary" htmlFor="showyaxis" onClick={() => updatePlotConfig({show_y_label: false})} data-cy="yfalsetoggle">No</label>
											</div>
										</div>
									</div>
								</>) : null}
							</form>
						</Tab>
					</Tabs>
					{rawData !== null ? (
						<div className="mt-2">
							{showDataTable ? (<>
								<h5>{rawData.Records.length + " row" + (rawData.Records.length !== 1 ? "s" : "")}</h5>
								<div className="table-responsive">
									<table className="table table-striped table-sm table-bordered">
										<thead>
											<tr>{rawData.ColNames.map(n => <th>{n}</th>)}</tr>
										</thead>
										<tbody>
											{rawData.Records === null ? null : rawData.Records.map(r => <tr>{r.map(c => <td>{c.Value}</td>)}</tr>)}
										</tbody>
									</table>
								</div>
							</>) : null}
						</div>
					) : null}
					{showEmbedHtml ? (
						<div className="mt-2">
							<h6>You can embed the chart in other web pages by using this HTML code. Please keep in mind that renaming or deleting your visualisation is going to break the embedding.</h6>
							<code>
								&lt;iframe width="425" height="350" src={"\"" + window.location.origin + "/visembed/" + meta.owner + "/" + meta.database + "?visname=" + selectedVisualisation + "\""} title={"\"" + selectedVisualisation + " - DBHub.io visualisation\""} style="border: 1px solid black"&gt;&lt;/iframe&gt;
							</code>
						</div>
					) : null}
				</>)}
			</div>
		</div>
	</>);
}
