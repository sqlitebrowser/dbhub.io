// Construct a timestamp string for use in user messages
function nowString() {
    // Construct a timestamp for the success message
    let now = new Date();
    let m = now.getMinutes();
    let s = now.getSeconds()
    let mins, seconds;
    mins = m;
    seconds = s;
    if (m < 10) {
        mins = '0' + m;
    }
    if (s < 10) {
        seconds = '0' + s;
    }
    return "[" + now.getHours() + ":" + mins + ":" + seconds + "] ";
}
