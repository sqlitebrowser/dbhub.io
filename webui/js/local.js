// Returns a string describing how long ago the given date was.  eg "3 seconds ago", "2 weeks ago", etc
function getTimePeriod(date1, includeOn) {
    var d1 = new Date(date1);
    var secondsElapsed = (new Date() - d1) / 1000;

    // Check if the time elapsed is more then 1 minute
    if (secondsElapsed >= 60) {
        var minutesElapsed = secondsElapsed / 60;

        // Check if the time elapsed is more then 1 hour
        if (minutesElapsed >= 60) {
            var hoursElapsed = minutesElapsed / 60;

            // Check if the time elapsed is more then 1 day
            if (hoursElapsed >= 24) {
                var daysElapsed = hoursElapsed / 24;
                daysElapsed = Math.round(daysElapsed);

                // Check if the time elapsed is more then 1 week
                if (daysElapsed >= 7) {
                    var weeksElapsed = daysElapsed / 7;
                    weeksElapsed = Math.round(weeksElapsed);

                    // If the time elapsed is more then 4 weeks ago, we return the date, nicely formatted
                    if (weeksElapsed > 4) {
                        var str = includeOn ? "on " : "";
                        return str + d1.toLocaleTimeString(undefined, { year: "numeric", month: "short",
                            day: "numeric", weekday: "short", hour: "2-digit", minute: "2-digit" });
                    } else {
                        var p0 = (weeksElapsed === 1) ? "" : "s";
                        return weeksElapsed + " week" + p0 + " ago"
                    }
                } else {
                    var p1 = (daysElapsed === 1) ? "" : "s";
                    return daysElapsed + " day" + p1 + " ago"
                }
            } else {
                hoursElapsed = Math.round(hoursElapsed);
                var p2 = (hoursElapsed === 1 ) ? "" : "s";
                return hoursElapsed + " hour" + p2 + " ago";
            }
        } else {
            minutesElapsed = Math.round(minutesElapsed);
            var p3 = (minutesElapsed === 1) ? "" : "s";
            return minutesElapsed + " minute" + p3 + " ago";
        }
    } else {
        secondsElapsed=Math.round(secondsElapsed);
        if (secondsElapsed === 0) {
            return "now"
        } else {
            var p4 = (secondsElapsed === 1) ? "" : "s";
            return secondsElapsed+" second" + p4 + " ago";
        }
    }
}