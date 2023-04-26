// base64url encode a string
function base64url(input) {
    let encoded = window.btoa(input);
    encoded = encoded.replace(/=+$/, '');
    encoded = encoded.replace(/\+/g, '-');
    encoded = encoded.replace(/\//g, '_');
    return encoded
}

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
