package dynacat

import (
	"errors"
	"fmt"
	"testing"
)

func TestFriendlyWidgetError(t *testing.T) {
	cases := map[string]string{
		`Get "http://x:1/": dial tcp 1.2.3.4:1: connect: connection refused`:       "The service isn't answering. It may be stopped.",
		`Get "https://x/": context deadline exceeded (Client.Timeout exceeded)`:    "The service took too long to answer.",
		`Get "https://nope.invalid/": dial tcp: lookup nope.invalid: no such host`: "The address can't be found. Check the URL.",
		`unexpected status code 401 from https://x`:                                "The service refused the request. Check the API key or token.",
		`expected status code 200, got 404`:                                        "The service says this address doesn't exist (404).",
		`docker API returned status 403`:                                           "The service refused the request. Check the API key or token.",
		`401 Unauthorized`:                                                         "The service refused the request. Check the API key or token.",
		`502 Bad Gateway`:                                                          "The service returned an error.",
		`something else entirely`:                                                  "This widget couldn't load.",
	}
	for in, want := range cases {
		if got := friendlyWidgetError(errors.New(in)); got != want {
			t.Errorf("%q: got %q", in, got)
		}
	}
	if got := friendlyWidgetError(fmt.Errorf("x: %w", errPartialContent)); got != "Some of this widget's data couldn't load." {
		t.Errorf("partial: %q", got)
	}
}
