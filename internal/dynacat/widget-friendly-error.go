package dynacat

import (
	"errors"
	"regexp"
	"strings"
)

// friendlyWidgetError turns a widget's raw error into a short line a person
// can act on. The full error stays one click away.
func friendlyWidgetError(err error) string {
	if err == nil {
		return "This widget couldn't load."
	}
	msg := strings.ToLower(err.Error())
	if errors.Is(err, errPartialContent) {
		return "Some of this widget's data couldn't load."
	}

	switch {
	case strings.Contains(msg, "connection refused"):
		return "The service isn't answering. It may be stopped."
	case strings.Contains(msg, "no such host"), strings.Contains(msg, "server misbehaving"):
		return "The address can't be found. Check the URL."
	case strings.Contains(msg, "deadline exceeded"), strings.Contains(msg, "timeout"), strings.Contains(msg, "timed out"):
		return "The service took too long to answer."
	case strings.Contains(msg, "x509"), strings.Contains(msg, "certificate"):
		return "The service's certificate isn't trusted. Try allow-insecure: true."
	case strings.HasSuffix(msg, ": forbidden"), strings.HasSuffix(msg, ": unauthorized"):
		return "The service refused the request. Check the API key or token."
	case friendlyStatus(msg) != "":
		switch friendlyStatus(msg) {
		case "401", "403":
			return "The service refused the request. Check the API key or token."
		case "404":
			return "The service says this address doesn't exist (404)."
		case "429":
			return "The service is rate limiting requests. It will retry."
		default:
			return "The service returned an error."
		}
	case strings.Contains(msg, "no content"), strings.Contains(msg, "failed to retrieve any content"):
		return "Nothing came back from the service."
	}
	return "This widget couldn't load."
}

var (
	friendlyGotStatusPattern = regexp.MustCompile(`\bgot ([1-5]\d\d)\b`)
	friendlyStatusPattern    = regexp.MustCompile(`\b(?:status(?: code)?:?\s*"?|returned\s*)([1-5]\d\d)\b`)
)

// friendlyStatus finds the HTTP status in an error, preferring the one a
// check got over the one it expected.
func friendlyStatus(msg string) string {
	if m := friendlyGotStatusPattern.FindStringSubmatch(msg); m != nil {
		return m[1]
	}
	if m := friendlyStatusPattern.FindStringSubmatch(msg); m != nil && m[1] != "200" {
		return m[1]
	}
	return ""
}
