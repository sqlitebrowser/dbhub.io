import React from "react";
import ReactDOM from "react-dom/client";

import ModalImage from "react-modal-image";

import Auth from "./auth";
import BranchesTable from "./branches";
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
	const rootNode = document.getElementById("branches");
	if (rootNode) {
		const root = ReactDOM.createRoot(rootNode);
		root.render(React.createElement(BranchesTable));
	}
}

{
	document.querySelectorAll(".markdown-editor").forEach((rootNode) => {
		const editorId = rootNode.dataset.id;
		const rows = rootNode.dataset.rows;
		const placeholder = rootNode.dataset.placeholder;
		const defaultIndex = rootNode.dataset.defaultIndex;
		const initialValue = rootNode.dataset.initialValue;
		const viewOnly = rootNode.dataset.viewOnly;

		const root = ReactDOM.createRoot(rootNode);
		root.render(React.createElement(MarkdownEditor, {
			editorId: editorId,
			rows: rows,
			placeholder: placeholder,
			defaultIndex: defaultIndex,
			initialValue: initialValue,
			viewOnly: viewOnly
		}));
	});
}

{
	document.querySelectorAll(".lightbox-image").forEach((rootNode) => {
		const small = rootNode.dataset.small;
		const large = rootNode.dataset.large;
		const alt = rootNode.dataset.alt;

		const root = ReactDOM.createRoot(rootNode);
		root.render(React.createElement(ModalImage, {
			small: small,
			large: large,
			alt: alt
		}));
	});

}
