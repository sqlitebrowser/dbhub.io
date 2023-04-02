const React = require("react");
const ReactDOM = require("react-dom");
import { Tab, Tabs, TabList, TabPanel } from "react-tabs";

export default function MarkdownEditor({editorId, rows, placeholder, defaultIndex, initialValue, viewOnly}) {
	const [previewHtml, setPreviewHtml] = React.useState("");

	if (rows === undefined) {
		rows = 10;
	}

	if (defaultIndex === undefined) {
		defaultIndex = 0;
	}

	if (viewOnly === undefined) {
		viewOnly = false;
		defaultIndex = 1;	// When in view-only mode, always change to the preview tab
	}

	function tabChanged(index) {
		// Preview tab selected?
		if (index === 1) {
			// Retrieve latest markdown text from the text area
			let txt = document.getElementById(editorId).value;

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
		if (defaultIndex == 1) {
			tabChanged(1);
		}
	}, []);

	// This is the editor and the preview area for the markdown.
	// The editor is set to invisible in view only mode
	let editor = (
		<textarea id={editorId} name={editorId} rows={rows} placeholder={placeholder} data-cy={editorId} style={{display: viewOnly ? "none" : null}}>
			{initialValue}
		</textarea>
	);
	let view = <div class="rendered minHeight" data-cy={editorId + "-preview"} dangerouslySetInnerHTML={{__html: previewHtml}} />;

	if (viewOnly) {
		return <>{view}{editor}</>;
	}

	return (
		<Tabs onSelect={(index) => tabChanged(index)} forceRenderTabPanel={true} defaultIndex={defaultIndex}>
			<TabList>
				<Tab data-cy={editorId + "-edit-tab"}>Edit</Tab>
				<Tab data-cy={editorId + "-preview-tab"}>Preview</Tab>
			</TabList>
			<TabPanel>{editor}</TabPanel>
			<TabPanel>{view}</TabPanel>
		</Tabs>
	);
}
