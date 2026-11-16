package dynacat

import (
	"context"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

// Icons used to be downloaded one by one while the app was being built, so
// startup and every config reload waited on the network: with a few dozen
// icons and a slow CDN the dashboard took minutes to come up, or not at all.
// Now an icon already on disk is used straight away; one that isn't is shown
// from its original URL and downloaded in the background for next time.

var iconDownloadSlots = make(chan struct{}, 4)

// backgroundIconDownloads counts downloads started here, which
// IsBuildingCache leaves out.
var backgroundIconDownloads atomic.Int64

// cachedImageURL returns the local copy of rawURL if one is on disk,
// without touching the network.
func (c *imageCache) cachedImageURL(rawURL string) (string, bool) {
	if c == nil || rawURL == "" {
		return "", false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", false
	}
	existing, found := c.findExistingFile(hashString(rawURL), parsed.Path)
	if !found {
		return "", false
	}
	return c.publicURL(existing), true
}

// cacheIconInBackground fetches an icon, or its fallbacks, for next time.
func (c *imageCache) cacheIconInBackground(rawURL string) {
	if c == nil {
		return
	}
	go func() {
		iconDownloadSlots <- struct{}{}
		defer func() { <-iconDownloadSlots }()
		backgroundIconDownloads.Add(1)
		defer backgroundIconDownloads.Add(-1)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if u, err := c.CacheURL(ctx, rawURL); err == nil && u != "" {
			return
		}
		for _, fallback := range iconFallbackURLs(rawURL) {
			if u, err := c.CacheURL(ctx, fallback); err == nil && u != "" {
				return
			}
		}
	}()
}

// cachedIconURL is what an icon shows right now: the local copy of the icon
// or of one of its fallbacks, or "" to keep the original URL.
func (c *imageCache) cachedIconURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if u, ok := c.cachedImageURL(rawURL); ok {
		return u
	}
	for _, fallback := range iconFallbackURLs(rawURL) {
		if u, ok := c.cachedImageURL(fallback); ok {
			return u
		}
	}
	c.cacheIconInBackground(rawURL)
	return ""
}
