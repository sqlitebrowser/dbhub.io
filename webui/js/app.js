import React from "react";
import ReactDOM from "react-dom/client";

import Auth from "./auth";
import DbHeader from "./db-header";

{
	const rootNode = document.getElementById("db-header-root");
	if (rootNode) {
		const root = ReactDOM.createRoot(rootNode);
		root.render(React.createElement(DbHeader));
	}
}

{
	const rootNode = document.getElementById("authcontrol");
	if (rootNode) {
		const root = ReactDOM.createRoot(rootNode);
		root.render(React.createElement(Auth));
	}
}
