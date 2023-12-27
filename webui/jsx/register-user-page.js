const React = require("react");
const ReactDOM = require("react-dom");

export default function RegisterUserPage({username}) {
	const [statusMessage, setStatusMessage] = React.useState("");
	const [statusMessageColour, setStatusMessageColour] = React.useState("");

	const [name, setName] = React.useState(username);

	// Handler for the check button. Query the server for determining if the username is available
	function checkName() {
		fetch("/x/checkname?name=" + name, {
			method: "get",
		}).then(response => {
			if (!response.ok) {
				return Promise.reject(response);
			}

			response.text().then(data => {
				if (data === "y") {
					setStatusMessage("✔ Name is available");
					setStatusMessageColour("green");
				} else {
					setStatusMessage("✘ Name not available");
					setStatusMessageColour("red");
				}
			});
		})
		.catch(error => {
			// Checking availability failed, display the error message
			error.text().then(text => {
				setStatusMessageColour("red");
				setStatusMessage("Checking availability failed: " + text);
			});
		});
	}

	return (<>
		<h3 className="text-center">Select your preferred username</h3>
		{statusMessage !== "" ? (
			<div className="row">
				<div className="col-md-12 text-center mb-2">
					<h6 style={{color: statusMessageColour}}>{statusMessage}</h6>
				</div>
			</div>
		) : null}
		<form action="/register" method="post">
			<div className="mb-2">
				<label className="form-label" htmlFor="username">Username</label>
				<input type="text" className="form-control" id="username" name="username" maxlength={80} value={name} onChange={e => setName(e.target.value)} required />
			</div>

			<button type="button" className="btn btn-primary" onClick={() => checkName()}>Check</button>&nbsp;
			<input type="submit" className="btn btn-success" value="Continue" />
		</form>
	</>);
}
