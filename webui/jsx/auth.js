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
			avatar = <img src={authInfo.avatarUrl} height="18" width="18" className="border border-secondary" />;
		}

		let updates = null;
		if (authInfo.numStatusUpdates === 0) {
			updates = <a href="/updates" className="align-middle"><i className="fa fa-inbox fa-fw fs-5"></i></a>;
		} else {
			updates = <a href="/updates" className="align-middle border-bottom border-info"><i className="fa fa-inbox fa-fw fs-5"></i>{authInfo.numStatusUpdates}</a>;
		}

		return (
			<>
				{avatar}
				&nbsp;
				{updates}
				&nbsp;
				<a href={"/" + authInfo.loggedInUser} className="align-middle">Home</a> | <a href="/pref" className="align-middle">Preferences</a> | <a href="/logout" className="align-middle">Log out</a>
			</>
		);
	} else {
		return <a onClick={() => {return login()}} data-cy="loginlnk" className="align-middle">Login / Register</a>;
	}
}
