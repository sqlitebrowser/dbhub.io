const React = require("react");
const ReactDOM = require("react-dom");

export default function Auth() {
	function login() {
		lock.show();
		return false;
	}

	if (authInfo.loggedInUser) {
		let avatar = null;
		if (authInfo.avatarUrl) {
			avatar = <img src={authInfo.avatarUrl} height="18" width="18" style={{border: "1px solid #8c8c8c"}}/>;
		}

		let updates = null;
		if (authInfo.numStatusUpdates === 0) {
			updates = <a href="/updates" className="inBox" style={{verticalAlign: "middle"}}><i className="fa fa-inbox fa-fw" style={{fontSize: "large"}}></i></a>;
		} else {
			updates = <a href="/updates" className="inBox" style={{verticalAlign: "middle", borderBottom: "1px grey dotted"}}><i className="fa fa-inbox fa-fw" style={{fontSize: "large"}}></i>{authInfo.numStatusUpdates}</a>;
		}

		return (
			<>
			{avatar}
			&nbsp;
			{updates}
			&nbsp;
				<a href={"/" + authInfo.loggedInUser} style={{color: "black", verticalAlign: "middle"}}>Home</a> | <a href={"/usage"} style={{color: "black", verticalAlign: "middle"}}>Usage</a> | <a href="/pref" style={{color: "black", verticalAlign: "middle"}}>Preferences</a> | <a href="/logout" style={{color: "black", verticalAlign: "middle"}}>Log out</a>
			</>
		);
	} else {
		return <a onClick={() => {return login()}} className="blackLink" data-cy="loginlnk">Login / Register</a>;
	}
}
