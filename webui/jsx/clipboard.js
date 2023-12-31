// Copy value of an input element to the system clipboard
export function copyToClipboard(element_id) {
	let e = document.getElementById(element_id);
	e.select();
	e.setSelectionRange(0, 99999);
	document.execCommand("copy");
}
