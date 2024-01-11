const React = require("react");
const ReactDOM = require("react-dom");
import Tab from "react-bootstrap/Tab";
import Tabs from "react-bootstrap/Tabs";

export default function MarkdownEditor({editorId, rows, placeholder, defaultTab, initialValue, viewOnly, onChange}) {
	const [previewHtml, setPreviewHtml] = React.useState("");

	if (rows === undefined) {
		rows = 10;
	}

	if (viewOnly === undefined) {
		viewOnly = false;
	} else if (viewOnly === true) {
		defaultTab = "preview";	// When in view-only mode, always change to the preview tab
	}

	function tabChanged(newTab) {
		// Preview tab selected?
		if (newTab === (editorId + "-preview-tab")) {
			// Retrieve latest markdown text from the text area
			const txt = document.getElementById(editorId).value;

			// Call the server, asking for a rendered version of the markdown
			fetch("/x/markdownpreview/", {
				method: "post",
				headers: {
					"Content-Type" : "application/x-www-form-urlencoded"
				},
				body: "mkdown=" + encodeURIComponent(txt)
			})
				.then((response) => response.text())
				.then((text) => {
					setPreviewHtml(text);
				});
		}
	}

	// After first rendering the component, make sure to get the markdown render from the
	// server
	React.useEffect(() => {
		if (defaultTab === "preview") {
			tabChanged(editorId + "-preview-tab");
		}
	}, []);

	// This is the editor and the preview area for the markdown.
	// The editor is set to invisible in view only mode
	let editor = (
		<textarea id={editorId} name={editorId} rows={rows} placeholder={placeholder} data-cy={editorId} style={{display: viewOnly ? "none" : null}} onChange={e => onChange !== undefined ? onChange(e.target.value) : null}>
			{initialValue}
		</textarea>
	);
	let view = <div className="minHeight" data-cy={editorId + "-preview"} dangerouslySetInnerHTML={{__html: previewHtml}} />;

	if (viewOnly) {
		return <>{view}{editor}</>;
	}

	return (
		<Tabs onSelect={(index) => tabChanged(index)} defaultActiveKey={defaultTab === "preview" ? (editorId + "-preview-tab") : (editorId + "-edit-tab")}>
			<Tab eventKey={editorId + "-edit-tab"} title="Edit">{editor}</Tab>
			<Tab eventKey={editorId + "-preview-tab"} title="Preview">{view}</Tab>
		</Tabs>
	);
}
