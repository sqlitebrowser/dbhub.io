export function userPrefTheme() {
	const saved = localStorage.getItem("userPref_colourMode");
	if (saved && (saved === "light" || saved === "dark")) {
		return saved;
	}

	// TODO When the dark theme is finished, change the default from light to system preference as shown in the second line
	return "light";
	return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

export function setUserPrefTheme(theme) {
	localStorage.setItem("userPref_colourMode", theme);
}

export function applyUserPrefTheme() {
	document.documentElement.setAttribute("data-bs-theme", userPrefTheme());
}
