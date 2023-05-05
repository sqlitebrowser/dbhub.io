export function convertResultsetToCsv(set, separator = ",", quote = "\"") {
	// This helper function adds quote characters to a field and escapes any existing occurrences of the quote character
	function addQuotes(s) {
		return quote + s.replace(new RegExp(quote, "g"), quote + quote) + quote;
	}

	// Convert data
	let rows = [
		// Header
		set.ColNames.map(c => addQuotes(c)).join(separator),

		// Data
		...set.Records.map(r => r.map(c => addQuotes(c.Value)).join(separator))
	];

	// Return CSV string including a final line break
	return rows.join("\n") + "\n";
}

export function downloadData(data, filename, type) {
	// Create file object
	const file = new Blob([data], {type: type});

	// Create a temporary invisible link to open the browser's save as dialog
	let link = document.createElement("a");
	link.download = filename;
	link.href = window.URL.createObjectURL(file);
	link.style.display = "none";
	document.body.appendChild(link);

	// Click on the link
	link.click();

	// Remove the temporary link
	document.body.removeChild(link);
}
