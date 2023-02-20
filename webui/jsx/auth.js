function Auth() {
	function login() {
		lock.show();
	}

	if (authInfo.loggedInUser) {
		let avatar = null;
		if (authInfo.avatarUrl) {
			avatar = <img src={authInfo.avatarUrl} height="18" width="18" style={{border: "1px solid #8c8c8c"}}/>;
		}

		let updates = null;
		if (authInfo.numStatusUpdates === 0) {
			updates = <a href="/updates" class="inBox" style={{verticalAlign: "middle"}}><i class="fa fa-inbox fa-fw" style={{fontSize: "large"}}></i></a>;
		} else {
			updates = <a href="/updates" class="inBox" style={{verticalAlign: "middle", borderBottom: "1px grey dotted"}}><i class="fa fa-inbox fa-fw" style={{fontSize: "large"}}></i>{authInfo.numStatusUpdates}</a>;
		}

		return (
			<>
			{avatar}
			&nbsp;
			{updates}
			&nbsp;
			<a href="/pref" style={{color: "black", verticalAlign: "middle"}}>Settings</a> | <a href={"/" + authInfo.loggedInUser} style={{color: "black", verticalAlign: "middle"}}>Home</a> | <a href="/logout" style={{color: "black", verticalAlign: "middle"}}>Log out</a>
			</>
		);
	} else {
		return <a href="" onClick={login} style={{color: "black"}} data-cy="loginlnk">Login / Register</a>;
	}
}

const rootNode = document.getElementById('authcontrol');
const root = ReactDOM.createRoot(rootNode);
root.render(React.createElement(Auth));
