const React = require("react");
const ReactDOM = require("react-dom");
import { Tab, Tabs, TabList, TabPanel } from "react-tabs";

export default function MarkdownEditor({editorId, rows, placeholder, defaultIndex, initialValue}) {
	const [previewHtml, setPreviewHtml] = React.useState("");

	if (rows === undefined) {
		rows = 10;
	}

	if (defaultIndex === undefined) {
		defaultIndex = 0;
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

	return (
		<>
		<Tabs onSelect={(index) => tabChanged(index)} forceRenderTabPanel={true} defaultIndex={defaultIndex}>
			<TabList>
				<Tab data-cy={editorId + "-edit-tab"}>Edit</Tab>
				<Tab data-cy={editorId + "-preview-tab"}>Preview</Tab>
			</TabList>
			<TabPanel>
				<textarea id={editorId} name={editorId} rows={rows} placeholder={placeholder} data-cy={editorId}>
					{initialValue}
				</textarea>
			</TabPanel>
			<TabPanel>
				<div class="rendered minHeight" data-cy={editorId + "-preview"} dangerouslySetInnerHTML={{__html: previewHtml}} />
			</TabPanel>
		</Tabs>
		</>
	);
}
