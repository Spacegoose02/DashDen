package dynacat

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestYoutubeFeedRetriesFlakyAnswers(t *testing.T) {
	old := youtubeFeedRetryDelays
	youtubeFeedRetryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { youtubeFeedRetryDelays = old })

	req, _ := http.NewRequest("GET", "https://www.youtube.com/feeds/videos.xml?channel_id=UCx", nil)

	calls := 0
	flakyThenOK := func(*http.Request) (youtubeFeedResponseXml, error) {
		calls++
		if calls < 3 {
			return youtubeFeedResponseXml{}, errors.New("unexpected status code 404 from https://www.youtube.com/feeds/videos.xml")
		}
		return youtubeFeedResponseXml{}, nil
	}
	if _, err := retryYoutubeFeedTask(flakyThenOK)(req); err != nil || calls != 3 {
		t.Fatalf("should succeed on the third try: calls=%d err=%v", calls, err)
	}

	calls = 0
	badXML := func(*http.Request) (youtubeFeedResponseXml, error) {
		calls++
		return youtubeFeedResponseXml{}, errors.New("XML syntax error")
	}
	if _, err := retryYoutubeFeedTask(badXML)(req); err == nil || calls != 1 {
		t.Fatalf("other errors shouldn't be retried: calls=%d", calls)
	}

	calls = 0
	alwaysDown := func(*http.Request) (youtubeFeedResponseXml, error) {
		calls++
		return youtubeFeedResponseXml{}, errors.New("unexpected status code 500 from youtube")
	}
	if _, err := retryYoutubeFeedTask(alwaysDown)(req); err == nil || calls != 3 {
		t.Fatalf("gives up after three tries: calls=%d", calls)
	}
}
