const React = require("react");
const ReactDOM = require("react-dom");

import "../jsx/sql-terminal.css";

import Editor from "react-simple-code-editor";
import {highlight, languages} from "prismjs/components/prism-core";
import "prismjs/components/prism-sql";
import "prismjs/themes/prism-solarizedlight.css";
import {format} from "sql-formatter";

/********************
 * The general structure here is this:
 * SqlTerminal
 *  - SqlTerminalCommand
 *     - SqlTerminalCommandInput
 *     - SqlTerminalCommandOutput
 *  - Editor, Buttons
 * The SqlTerminalCommand components holds an SQL command and its corresponding result and output.
 * For doing so it knows several states:
 *  - new: Used for newly typed in commands. Indicates that the command still needs to be sent to the server.
 *  - loading: When the command has been sent to the server but no response has been received yet, the command is marked as loading.
 *  - error: The server responded with an error message.
 *  - executed: The server successfully executed the command and it did not return any data.
 *  - queried: The server successfully executed the command and it did return data.
 * The state is handled and set in SqlTerminalCommand. The value is also passed into SqlTerminalCommandOutput where a different style
 * is applied depending on the state.
 *******************/

function SqlTerminalCommandInput({data}) {
	return (
		<p className="sql-terminal-command-input">
			<strong className="text-muted">sql> </strong><span dangerouslySetInnerHTML={{__html: Prism.highlight(data, languages.sql, "sql")}} />
		</p>
	);
}

function SqlTerminalCommandOutput({state, data}) {
	let output = null;
	if (state === "loading") {
		output = <span className="text-muted">loading...</span>;
	} else if (state === "error") {
		output = <span className="text-danger"><strong>error: </strong>{data}</span>;
	} else if (state === "executed") {
		output = <span className="text-info"><strong>done: </strong>{data.rows_changed + " row" + (data.rows_changed === 1 ? "" : "s") + " changed"}</span>;
	} else if (state === "queried") {
		output = (<>
			<span className="text-info"><strong>done: </strong>{data.RowCount + " row" + (data.RowCount === 1 ? "" : "s") + " returned"}</span>
			<div className="table-responsive">
				<table className="table table-hover table-condensed">
					<thead>
						<tr>{data.ColNames.map(n => <th>{n}</th>)}</tr>
					</thead>
					<tbody>
						{data.Records.map(r => <tr>{r.map(c => <td>{c.Value}</td>)}</tr>)}
					</tbody>
				</table>
			</div>
		</>);
	}

	return (
		<p className="sql-terminal-command-output">
			{output}
		</p>
	);
}

function SqlTerminalCommand({command}) {
	const [state, setState] = React.useState(command.state);
	const [output, setOutput] = React.useState(command.output);

	// When first rendering this component, check if the query needs to be executed
	React.useEffect(() => {
		if (state === "new") {
			setState("loading");

			fetch("/x/execlivesql/" + meta.owner + "/" + meta.database, {
				method: "post",
				body: JSON.stringify({sql: command.input}),
				headers: {"Content-Type": "application/json"},
			}).then(response => {
				if (!response.ok) {
					return Promise.reject(response);
				}

				response.json().then(data => {
					// Did this return any data?
					if ("rows_changed" in data) {
						// This query was modifying the database

						setState("executed");
						setOutput(data);
					} else {
						// This query returned some data

						setState("queried");
						setOutput(data);
					}
				});
			})
			.catch(error => {
				error.text().then(text => {
					setState("error");
					setOutput(text);
				});
			});
		}
	}, []);

	return (
		<div className="sql-terminal-command">
			<SqlTerminalCommandInput data={command.input} />
			<SqlTerminalCommandOutput state={state} data={output} />
		</div>
	);
}

export default function SqlTerminal() {
	const [code, setCode] = React.useState("");
	const [recentCommands, setRecentCommands] = React.useState([]);
	const [executeOnEnter, setExecuteOnEnter] = React.useState(false);
	const [isDragging, setDragging] = React.useState(false);		// We use this to distinguish simple clicks on the component from drag & drop movements. The latter are ignored to not interfere with selecting text for copy & paste.

	const editorRef = React.useRef(null);
	const doubleClickTimer = React.useRef();

	// This execute the currently typed in SQL statement by adding it to the command history with new state
	function execute() {
		// Do nothing for empty code
		if (code.trim() === "") {
			return;
		}

		// Add the current SQL text to the list of recent commands
		let commands = recentCommands.slice();
		commands.push({
			input: code,
			state: "new",
		});
		setRecentCommands(commands);

		// Clear input field
		setCode("");
	}

	// This is called when a key is pressed in the SQL editor component
	function checkKeyPressExecute(e) {
		// Are we in "Enter to execute" or in normal mode?
		if (executeOnEnter) {
			// Enter executes the current code
			if (e.key === "Enter" && e.ctrlKey === false) {
				execute();
			}
		} else {
			// Control + Enter executes the current code
			if (e.key === "Enter" && e.ctrlKey === true) {
				execute();
			}
		}
	}

	// Format the current SQL code
	function formatSql() {
		// For parse errors this throws an exception
		try {
			setCode(format(code, {
				language: "sqlite",
			}));
		} catch(e) {
			alert(e);
		}
	}

	// This effect makes sure we scroll to the bottom of the command history whenever a new entry is added
	const performScrolldown = React.useRef(false);
	React.useEffect(() => {
		if (performScrolldown.current) {	// skip scrolldown when the component first loads
			setTimeout(() => editorRef?.current?.scrollIntoView({behavior: "auto", block: "nearest"}), 100);
		}
		performScrolldown.current = true;
	}, [recentCommands]);

	// Handle all mouse click events, used for setting focus to the SQL code editor component even if another
	// part of the component has been clicked.
	function handleClick(e) {
		// The idea here is to start a timer whenever the user makes a click on the terminal component.
		// Only when the timer expires, the actual actions are executed. This is necessary because
		// double click events also trigger a single click event and we only want to set the focus for
		// single clicks because double clicks are often used for selecting text for copy & paste.
		clearTimeout(doubleClickTimer.current);

		// The detail property here contains the click count
		if (e.detail === 1) {
			doubleClickTimer.current = setTimeout(() => {
				editorRef.current.querySelector("textarea").focus();
			}, 200);
		}
	}

	return (
		<div className="sql-terminal" onMouseDown={() => setDragging(false)} onMouseMove={() => setDragging(true)} onMouseUp={(e) => (isDragging ? null : handleClick(e))} >
			{recentCommands.map(c => <SqlTerminalCommand command={c} />)}
			<div className="sql-terminal-input">
				<div className="input-group" ref={editorRef}>
					<Editor
						value={code}
						onValueChange={text => setCode(text)}
						highlight={text => highlight(text, languages.sql)}
						autoFocus={true}
						placeholder={"Type in your SQL command here and click the Execute button or press " + (executeOnEnter ? "Enter" : "Ctrl+Enter")}
						onKeyPress={checkKeyPressExecute}
						style={{
							backgroundColor: "rgba(0, 0, 0, 0)",
							fontFamily: "monospace",
							fontSize: "14px",
							minHeight: "42px",
						}}
					/>
					<div className="input-group-btn dropup">
						<button type="button" className="btn btn-primary" disabled={code.trim() === "" ? "disabled" : null} onClick={() => execute()} data-cy="executebtn"><i className="fa fa-play" /> Execute</button>
						<button type="button" className="btn btn-primary dropdown-toggle" data-toggle="dropdown" aria-haspopup="true" aria-expanded="false" data-cy="dropdownbtn"><span className="caret"></span></button>
						<ul className="dropdown-menu dropdown-menu-right">
							<li><a href="#" onClick={() => setExecuteOnEnter(!executeOnEnter)}><input type="checkbox" checked={executeOnEnter ? "checked" : null} /> Execute on Enter</a></li>
							<li role="separator" className="divider"></li>
							<li><a href="#" onClick={() => formatSql()} data-cy="formatbtn">Format SQL</a></li>
						</ul>
					</div>
				</div>
			</div>
		</div>
	);
}
