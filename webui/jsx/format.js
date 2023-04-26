// Returns a string describing how long ago the given date was.  eg "3 seconds ago", "2 weeks ago", etc
export function getTimePeriod(date1, includeOn) {
	const d1 = new Date(date1);
	let secondsElapsed = (new Date() - d1) / 1000;

	// Check if the time elapsed is more then 1 minute
	if (secondsElapsed >= 60) {
		let minutesElapsed = secondsElapsed / 60;

		// Check if the time elapsed is more then 1 hour
		if (minutesElapsed >= 60) {
			let hoursElapsed = minutesElapsed / 60;

			// Check if the time elapsed is more then 1 day
			if (hoursElapsed >= 24) {
				const daysElapsed = Math.round(hoursElapsed / 24);

				// Check if the time elapsed is more then 1 week
				if (daysElapsed >= 7) {
					const weeksElapsed = Math.round(daysElapsed / 7);

					// If the time elapsed is more then 4 weeks ago, we return the date, nicely formatted
					if (weeksElapsed > 4) {
						const str = includeOn ? "on " : "";
						return str + d1.toLocaleTimeString(undefined, {year: "numeric", month: "short",
							day: "numeric", weekday: "short", hour: "2-digit", minute: "2-digit"});
					} else {
						const p0 = (weeksElapsed === 1) ? "" : "s";
						return weeksElapsed + " week" + p0 + " ago";
					}
				} else {
					const p1 = (daysElapsed === 1) ? "" : "s";
					return daysElapsed + " day" + p1 + " ago";
				}
			} else {
				hoursElapsed = Math.round(hoursElapsed);
				const p2 = (hoursElapsed === 1 ) ? "" : "s";
				return hoursElapsed + " hour" + p2 + " ago";
			}
		} else {
			minutesElapsed = Math.round(minutesElapsed);
			const p3 = (minutesElapsed === 1) ? "" : "s";
			return minutesElapsed + " minute" + p3 + " ago";
		}
	} else {
		secondsElapsed = Math.round(secondsElapsed);
		if (secondsElapsed === 0) {
			return "now";
		} else {
			const p4 = (secondsElapsed === 1) ? "" : "s";
			return secondsElapsed + " second" + p4 + " ago";
		}
	}
}
