package dynacat

// DashDen: YouTube's RSS feeds have been unreliable since late 2025 (random or
// long-running 404s and 500s), so the videos widget can read uploads through
// the YouTube Data API instead. Each channel costs one unit of the free 10,000
// a day. Set api-key on the widget, or YOUTUBE_API_KEY for every videos widget.
// Anything the API can't answer falls back to the RSS feed, and the last good
// videos for each channel are kept on disk so a bad hour (or a restart during
// one) still shows something.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var youtubeAPIBaseURL = "https://www.googleapis.com/youtube/v3"

const youtubeAPIMaxResults = 15

type youtubePlaylistItemsResponse struct {
	Items []struct {
		Snippet struct {
			PublishedAt            string `json:"publishedAt"`
			ChannelID              string `json:"channelId"`
			ChannelTitle           string `json:"channelTitle"`
			Title                  string `json:"title"`
			VideoOwnerChannelID    string `json:"videoOwnerChannelId"`
			VideoOwnerChannelTitle string `json:"videoOwnerChannelTitle"`
			Thumbnails             map[string]struct {
				URL string `json:"url"`
			} `json:"thumbnails"`
			ResourceID struct {
				VideoID string `json:"videoId"`
			} `json:"resourceId"`
		} `json:"snippet"`
		ContentDetails struct {
			VideoPublishedAt string `json:"videoPublishedAt"`
		} `json:"contentDetails"`
	} `json:"items"`
}

type youtubeAPIStatusError struct {
	status int
	body   string
}

func (e *youtubeAPIStatusError) Error() string {
	return fmt.Sprintf("YouTube API answered %d: %s", e.status, e.body)
}

// The key goes in a header, not the URL, so it never shows up in an error
// message or a log line.
func fetchYoutubePlaylistItems(ctx context.Context, apiKey, playlistID string) (youtubePlaylistItemsResponse, error) {
	var out youtubePlaylistItemsResponse
	q := url.Values{}
	q.Set("part", "snippet,contentDetails")
	q.Set("maxResults", fmt.Sprint(youtubeAPIMaxResults))
	q.Set("playlistId", playlistID)
	req, err := http.NewRequestWithContext(ctx, "GET", youtubeAPIBaseURL+"/playlistItems?"+q.Encode(), nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("X-Goog-Api-Key", apiKey)
	req.Header.Set("Accept", "application/json")

	status, body, err := fetchRequestBody(defaultHTTPClient, req)
	if err != nil {
		return out, err
	}
	if status != http.StatusOK {
		msg := string(body)
		var apiErr struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &apiErr) == nil && apiErr.Error.Message != "" {
			msg = apiErr.Error.Message
		}
		msg, _ = limitStringLength(msg, 200)
		return out, &youtubeAPIStatusError{status: status, body: msg}
	}
	err = json.Unmarshal(body, &out)
	return out, err
}

func youtubeAPIPlaylistFor(id string, includeShorts bool) (string, bool) {
	if strings.HasPrefix(id, videosWidgetPlaylistPrefix) {
		return strings.TrimPrefix(id, videosWidgetPlaylistPrefix), true
	}
	if strings.HasPrefix(id, "UC") && len(id) > 2 {
		if includeShorts {
			return "UU" + id[2:], true
		}
		return "UULF" + id[2:], true
	}
	return "", false
}

func youtubeVideosFromPlaylistItems(resp youtubePlaylistItemsResponse, videoUrlTemplate string) videoList {
	videos := make(videoList, 0, len(resp.Items))
	for _, item := range resp.Items {
		s := item.Snippet
		videoID := s.ResourceID.VideoID
		if videoID == "" || len(s.Thumbnails) == 0 {
			// Private or deleted videos come back without thumbnails.
			continue
		}

		thumb := ""
		for _, size := range []string{"high", "medium", "standard", "default", "maxres"} {
			if t, ok := s.Thumbnails[size]; ok && t.URL != "" {
				thumb = t.URL
				break
			}
		}

		author, authorID := s.VideoOwnerChannelTitle, s.VideoOwnerChannelID
		if author == "" {
			author = s.ChannelTitle
		}
		if authorID == "" {
			authorID = s.ChannelID
		}

		videoUrl := "https://www.youtube.com/watch?v=" + videoID
		if videoUrlTemplate != "" {
			videoUrl = strings.ReplaceAll(videoUrlTemplate, "{VIDEO-ID}", videoID)
		}

		posted := item.ContentDetails.VideoPublishedAt
		if posted == "" {
			posted = s.PublishedAt
		}
		t, err := time.Parse(time.RFC3339, posted)
		if err != nil {
			t = time.Now()
		}

		videos = append(videos, video{
			ThumbnailUrl: thumb,
			Title:        s.Title,
			Url:          videoUrl,
			Author:       author,
			AuthorUrl:    "https://www.youtube.com/channel/" + authorID + "/videos",
			TimePosted:   t,
		})
	}
	return videos
}

// fetchYoutubeAPIUploads asks the Data API for each channel or playlist it can.
// It returns the videos it got, keyed by id, and the ids left for the RSS feed.
func fetchYoutubeAPIUploads(ctx context.Context, apiKey string, ids []string, videoUrlTemplate string, includeShorts bool) (map[string]videoList, []string) {
	got := make(map[string]videoList, len(ids))
	var rest []string
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)

	for _, id := range ids {
		playlistID, ok := youtubeAPIPlaylistFor(id, includeShorts)
		if !ok {
			rest = append(rest, id)
			continue
		}

		wg.Add(1)
		go func(id, playlistID string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			resp, err := fetchYoutubePlaylistItems(ctx, apiKey, playlistID)
			// A channel with no long-form uploads has no UULF playlist.
			var statusErr *youtubeAPIStatusError
			if errors.As(err, &statusErr) && statusErr.status == http.StatusNotFound && strings.HasPrefix(playlistID, "UULF") {
				resp, err = fetchYoutubePlaylistItems(ctx, apiKey, "UU"+strings.TrimPrefix(playlistID, "UULF"))
			}

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				slog.Warn("YouTube API failed, trying the RSS feed", "channel", id, "error", err)
				rest = append(rest, id)
				return
			}
			got[id] = youtubeVideosFromPlaylistItems(resp, videoUrlTemplate)
		}(id, playlistID)
	}

	wg.Wait()
	return got, rest
}

// fetchYoutubeVideos uses the API when there's a key, the RSS feed for the
// rest, and the cache for anything that failed both.
func fetchYoutubeVideos(ctx context.Context, apiKey string, ids []string, videoUrlTemplate string, includeShorts bool, cache map[string]videoList) (videoList, error) {
	if apiKey == "" {
		return fetchYoutubeChannelUploads(ctx, ids, videoUrlTemplate, includeShorts, cache)
	}

	got, rest := fetchYoutubeAPIUploads(ctx, apiKey, ids, videoUrlTemplate, includeShorts)
	videos := make(videoList, 0, len(ids)*youtubeAPIMaxResults)
	for id, v := range got {
		if cache != nil {
			cache[id] = v
		}
		videos = append(videos, v...)
	}

	if len(rest) == 0 {
		if len(videos) == 0 {
			return nil, errNoContent
		}
		return videos.sortByNewest(), nil
	}

	rssVideos, err := fetchYoutubeChannelUploads(ctx, rest, videoUrlTemplate, includeShorts, cache)
	videos = append(videos, rssVideos...)
	if len(videos) == 0 {
		if err == nil {
			err = errNoContent
		}
		return nil, err
	}
	videos.sortByNewest()
	if err != nil && !errors.Is(err, errPartialContent) {
		return videos, fmt.Errorf("%w: missing videos from %d channels", errPartialContent, len(rest))
	}
	return videos, err
}

// Saved videos live next to the config (the Unraid template maps that folder),
// so they survive a restart or a Force update.
const youtubeCacheDirName = ".dashden-cache/youtube"

var youtubeCacheFileUnsafe = regexp.MustCompile(`[^A-Za-z0-9_-]`)

func youtubeCacheFile(dir, id string) string {
	return filepath.Join(dir, youtubeCacheFileUnsafe.ReplaceAllString(id, "_")+".json")
}

func loadYoutubeCache(dir string, ids []string, cache map[string]videoList) {
	if dir == "" {
		return
	}
	for _, id := range ids {
		if _, ok := cache[id]; ok {
			continue
		}
		data, err := os.ReadFile(youtubeCacheFile(dir, id))
		if err != nil {
			continue
		}
		var v videoList
		if json.Unmarshal(data, &v) == nil && len(v) > 0 {
			cache[id] = v
		}
	}
}

func saveYoutubeCache(dir string, ids []string, cache map[string]videoList) {
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	for _, id := range ids {
		v, ok := cache[id]
		if !ok || len(v) == 0 {
			continue
		}
		data, err := json.Marshal(v)
		if err != nil {
			continue
		}
		path := youtubeCacheFile(dir, id)
		tmp := path + ".tmp"
		if os.WriteFile(tmp, data, 0o644) == nil {
			os.Rename(tmp, path)
		}
	}
}
