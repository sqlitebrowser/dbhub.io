import React from "react";
import ReactDOM from "react-dom/client";

import Auth from "./auth";
import DbHeader from "./db-header";
import MarkdownEditor from "./markdown-editor";

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

{
	document.querySelectorAll('.markdown-editor').forEach((rootNode) => {
		const editorId = rootNode.dataset.id;
		const rows = rootNode.dataset.rows;
		const placeholder = rootNode.dataset.placeholder;
		const defaultIndex = rootNode.dataset.defaultIndex;
		const initialValue = rootNode.dataset.initialValue;

		const root = ReactDOM.createRoot(rootNode);
		root.render(React.createElement(MarkdownEditor, {
			editorId: editorId,
			rows: rows,
			placeholder: placeholder,
			defaultIndex: defaultIndex,
			initialValue: initialValue
		}));
	});
}
