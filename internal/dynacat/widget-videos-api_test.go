package dynacat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestYoutubeAPIUploads(t *testing.T) {
	var sawKeyInURL bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "secret") {
			sawKeyInURL = true
		}
		if r.Header.Get("X-Goog-Api-Key") != "secret" {
			w.WriteHeader(403)
			return
		}
		pl := r.URL.Query().Get("playlistId")
		switch pl {
		case "UULFaaa":
			w.Write([]byte(`{"items":[
				{"snippet":{"title":"Newer","channelTitle":"A","channelId":"UCaaa","videoOwnerChannelTitle":"A","videoOwnerChannelId":"UCaaa",
				 "thumbnails":{"high":{"url":"https://i.ytimg.com/vi/v2/hqdefault.jpg"}},"resourceId":{"videoId":"v2"}},
				 "contentDetails":{"videoPublishedAt":"2026-11-15T10:00:00Z"}},
				{"snippet":{"title":"Private video","resourceId":{"videoId":"gone"}}}
			]}`))
		case "UULFbbb":
			w.WriteHeader(404)
			w.Write([]byte(`{"error":{"message":"The playlist identified with the request's playlistId parameter cannot be found."}}`))
		case "UUbbb":
			w.Write([]byte(`{"items":[{"snippet":{"title":"Older","channelTitle":"B","channelId":"UCbbb",
				"thumbnails":{"default":{"url":"https://i.ytimg.com/vi/v1/default.jpg"}},"resourceId":{"videoId":"v1"}},
				"contentDetails":{"videoPublishedAt":"2026-11-01T10:00:00Z"}}]}`))
		default:
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()
	old := youtubeAPIBaseURL
	youtubeAPIBaseURL = srv.URL
	defer func() { youtubeAPIBaseURL = old }()

	got, rest := fetchYoutubeAPIUploads(context.Background(), "secret", []string{"UCaaa", "UCbbb", "@handle"}, "", false)
	if sawKeyInURL {
		t.Fatal("API key leaked into the URL")
	}
	if len(rest) != 1 || rest[0] != "@handle" {
		t.Fatalf("rest = %v, want only the handle left for RSS", rest)
	}
	if len(got["UCaaa"]) != 1 || got["UCaaa"][0].Url != "https://www.youtube.com/watch?v=v2" || got["UCaaa"][0].AuthorUrl != "https://www.youtube.com/channel/UCaaa/videos" {
		t.Fatalf("UCaaa = %+v", got["UCaaa"])
	}
	if len(got["UCbbb"]) != 1 || got["UCbbb"][0].ThumbnailUrl != "https://i.ytimg.com/vi/v1/default.jpg" {
		t.Fatalf("UCbbb should fall back to the UU playlist, got %+v", got["UCbbb"])
	}

	got, rest = fetchYoutubeAPIUploads(context.Background(), "wrong", []string{"UCaaa"}, "https://x/{VIDEO-ID}", false)
	if len(got) != 0 || len(rest) != 1 {
		t.Fatalf("a refused key should leave the channel for RSS, got %v %v", got, rest)
	}
}

func TestYoutubeCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cache := map[string]videoList{"playlist:PL1": {{Title: "Kept"}}}
	saveYoutubeCache(dir, []string{"playlist:PL1", "UCnone"}, cache)

	loaded := map[string]videoList{}
	loadYoutubeCache(dir, []string{"playlist:PL1", "UCnone"}, loaded)
	if len(loaded["playlist:PL1"]) != 1 || loaded["playlist:PL1"][0].Title != "Kept" {
		t.Fatalf("loaded = %+v", loaded)
	}
	if _, ok := loaded["UCnone"]; ok {
		t.Fatal("nothing was saved for UCnone")
	}
}
