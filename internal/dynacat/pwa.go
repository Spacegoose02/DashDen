package dynacat

import (
	"io"
	"net/http"
	"strings"
)

// Installable app: a service worker served from the site root so it can
// cover every page. See static/pwa/sw.js.

func (a *application) registerPWARoutes(mux *http.ServeMux) {
	raw, err := staticFS.Open("pwa/sw.js")
	if err != nil {
		return
	}
	defer raw.Close()
	contents, err := io.ReadAll(raw)
	if err != nil {
		return
	}
	script := strings.NewReplacer(
		"__VERSION__", getStaticFSHash(),
		"__BASE__", a.Config.Server.BaseURL,
	).Replace(string(contents))

	mux.HandleFunc("GET /sw.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Service-Worker-Allowed", a.Config.Server.BaseURL+"/")
		w.Write([]byte(script))
	})
}
