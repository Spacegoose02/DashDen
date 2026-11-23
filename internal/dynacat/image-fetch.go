package dynacat

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Feed thumbnails are often full-size photos of a few megabytes. They used to be
// fetched with the 5 second client meant for JSON APIs, and that limit covers
// reading the body too. A slow image failed to cache, fell back to the image
// proxy, and the proxy had already sent "200 OK" with a 30 day "immutable"
// header when the 5 seconds ran out, so the browser kept a cut-off picture
// (the top of the photo, then grey) for a month. Images now get longer, and
// the proxy reads the whole image before it answers.

const (
	imageCacheFetchTimeout = 15 * time.Second
	imageProxyFetchTimeout = 30 * time.Second
)

// imageProxyURLVersion is added to proxied image addresses so browsers fetch
// them again instead of reusing a cut-off copy cached by an older version.
const imageProxyURLVersion = "?v=2"

var (
	imageCacheHTTPClient = &http.Client{
		Transport: defaultHTTPClient.Transport,
		Timeout:   imageCacheFetchTimeout,
	}
	imageCacheInsecureHTTPClient = &http.Client{
		Transport: defaultInsecureHTTPClient.Transport,
		Timeout:   imageCacheFetchTimeout,
	}
	imageProxyHTTPClient = &http.Client{
		Transport:     defaultHTTPClient.Transport,
		Timeout:       imageProxyFetchTimeout,
		CheckRedirect: checkPublicRedirect,
	}
	imageProxyInsecureHTTPClient = &http.Client{
		Transport:     defaultInsecureHTTPClient.Transport,
		Timeout:       imageProxyFetchTimeout,
		CheckRedirect: checkPublicRedirect,
	}
)

func imageCacheClient(allowInsecure bool) *http.Client {
	return ternary(allowInsecure, imageCacheInsecureHTTPClient, imageCacheHTTPClient)
}

var errImageNotAvailable = errors.New("image not available")

// fetchWholeImage downloads an image completely, or returns an error. It never
// hands back part of an image.
func fetchWholeImage(r *http.Request, client *http.Client, rawURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "image/*")
	setBrowserUserAgentHeader(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("%w: status %d", errImageNotAvailable, resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if extensionFromContentType(contentType) == "" {
		return nil, "", fmt.Errorf("%w: content type %q", errImageNotAvailable, contentType)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) > maxResponseBytes {
		return nil, "", errResponseTooLarge
	}
	if len(body) == 0 {
		return nil, "", fmt.Errorf("%w: empty body", errImageNotAvailable)
	}
	if resp.ContentLength > 0 && int64(len(body)) != resp.ContentLength {
		return nil, "", io.ErrUnexpectedEOF
	}

	return body, contentType, nil
}

// serveProxiedImage answers an image proxy request. A failure is sent as an
// error the browser won't keep, so the next page load tries again.
func (a *application) serveProxiedImage(w http.ResponseWriter, r *http.Request, info imageProxyInfo) {
	client := ternary(info.AllowInsecure, imageProxyInsecureHTTPClient, imageProxyHTTPClient)
	body, contentType, err := fetchWholeImage(r, client, info.URL)
	if err != nil {
		w.Header().Set("Cache-Control", "no-store")
		switch {
		case errors.Is(err, errImageNotAvailable):
			http.Error(w, "Image not available", http.StatusNotFound)
		default:
			http.Error(w, "Failed to fetch image", http.StatusBadGateway)
		}
		return
	}

	// Keep a copy, so the widget's next update links the cached file.
	a.imageCache.storeImage(info.URL, contentType, body)

	// The proxied URL comes from remote widget content, so reflecting its content type would
	// let an upstream serve HTML from this origin; fetchWholeImage only accepts image types.
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("Content-Security-Policy", "sandbox")
	w.Header().Set("Cache-Control", "public, max-age=2592000, immutable")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

// storeImage saves an image fetched elsewhere under the name CacheURL would
// give it. It does nothing if the file is already there.
func (c *imageCache) storeImage(rawURL string, contentType string, body []byte) {
	if c == nil || c.dir == "" || len(body) == 0 {
		return
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	hashHex := hashString(rawURL)
	if _, found := c.findExistingFile(hashHex, parsed.Path); found {
		return
	}

	ext := extensionFromPath(parsed.Path)
	if ext == "" {
		ext = extensionFromContentType(contentType)
	}
	if ext == "" {
		return
	}

	tmp, err := os.CreateTemp(c.dir, hashHex+"-*.tmp")
	if err != nil {
		return
	}
	_, writeErr := tmp.Write(body)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(tmp.Name())
		return
	}
	if err := os.Rename(tmp.Name(), filepath.Join(c.dir, hashHex+ext)); err != nil {
		os.Remove(tmp.Name())
	}
}
