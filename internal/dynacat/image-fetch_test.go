package dynacat

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestServeProxiedImageSendsWholeImage(t *testing.T) {
	img := bytes.Repeat([]byte{0xFF, 0xD8}, 4096)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(img)
	}))
	defer upstream.Close()

	dir := t.TempDir()
	app := &application{imageCache: newImageCache("", dir)}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/image-proxy/x", nil)
	imageURL := upstream.URL + "/photo"
	app.serveProxiedImage(rec, req, imageProxyInfo{URL: imageURL})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), img) {
		t.Fatalf("body has %d bytes, want %d", rec.Body.Len(), len(img))
	}
	if got := rec.Header().Get("Content-Length"); got != "8192" {
		t.Errorf("Content-Length = %q", got)
	}

	// The proxy keeps a copy where the image cache looks for it.
	if _, err := os.Stat(filepath.Join(dir, hashString(imageURL)+".jpg")); err != nil {
		t.Errorf("image not stored in cache: %v", err)
	}
	if u, ok := app.imageCache.cachedImageURL(imageURL); !ok || u == "" {
		t.Errorf("cachedImageURL did not find the stored image")
	}
}

func TestServeProxiedImageCutOffIsNotCached(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Content-Length", "100000")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte{0xFF, 0xD8, 0xFF})
		// Drop the connection mid-image, as a stalled download does.
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			conn.Close()
		}
	}))
	defer upstream.Close()

	dir := t.TempDir()
	app := &application{imageCache: newImageCache("", dir)}
	rec := httptest.NewRecorder()
	app.serveProxiedImage(rec, httptest.NewRequest(http.MethodGet, "/", nil), imageProxyInfo{URL: upstream.URL + "/a.jpg"})

	if rec.Code == http.StatusOK {
		t.Fatalf("a cut-off image was answered with 200")
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("cache dir has %d files, want none", len(entries))
	}
}

func TestServeProxiedImageRejectsNonImages(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html>sign in</html>"))
	}))
	defer upstream.Close()

	app := &application{}
	rec := httptest.NewRecorder()
	app.serveProxiedImage(rec, httptest.NewRequest(http.MethodGet, "/", nil), imageProxyInfo{URL: upstream.URL})

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if rec.Header().Get("Content-Type") == "text/html" {
		t.Error("upstream content type was reflected")
	}
}

func TestSecureImageURLProxyAddressIsVersioned(t *testing.T) {
	p := &widgetProviders{baseURL: "http://dash"}
	got := p.SecureImageURL(t.Context(), "https://example.com/x.jpg", false)
	want := "http://dash/api/image-proxy/" + hashString("https://example.com/x.jpg") + imageProxyURLVersion
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
