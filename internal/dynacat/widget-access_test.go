package dynacat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const widgetAccessConfig = `
server:
  trusted-proxies: [10.0.0.0/24]
auth:
  proxy: {}
household:
  members:
    mom: {role: admin}
    ava: {role: kid}
pages:
  - name: Home
    slug: home
    columns:
      - size: full
        widgets:
          - type: html
            source: <p>for-everyone</p>
          - type: html
            allowed-groups: [adult]
            source: <p>grown-ups-only</p>
          - type: group
            allowed-users: [ava]
            widgets:
              - type: html
                source: <p>ava-in-a-group</p>
`

func newWidgetAccessApp(t *testing.T, cfgText string) *application {
	t.Helper()
	cfg, err := newConfigFromYAML([]byte(cfgText))
	if err != nil {
		t.Fatal(err)
	}
	a, err := newApplication(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func pageContentAs(a *application, user string) string {
	r := httptest.NewRequest("GET", "/api/pages/home/content/", nil)
	r.RemoteAddr = "10.0.0.5:1"
	r.Header.Set("Remote-User", user)
	r.SetPathValue("page", "home")
	w := httptest.NewRecorder()
	a.handlePageContentRequest(w, r)
	return w.Body.String()
}

func TestWidgetVisibility(t *testing.T) {
	a := newWidgetAccessApp(t, widgetAccessConfig)

	mom := pageContentAs(a, "mom")
	if !strings.Contains(mom, "for-everyone") || !strings.Contains(mom, "grown-ups-only") || strings.Contains(mom, "ava-in-a-group") {
		t.Fatalf("mom sees the wrong widgets:\n%s", mom)
	}
	ava := pageContentAs(a, "ava")
	if !strings.Contains(ava, "for-everyone") || strings.Contains(ava, "grown-ups-only") || !strings.Contains(ava, "ava-in-a-group") {
		t.Fatalf("ava sees the wrong widgets:\n%s", ava)
	}

	// The single-widget endpoint and actions hide it too, including a
	// widget that only inherits the rule from its group.
	var hidden, nested uint64
	for id, w := range a.widgetByID {
		if h, ok := w.(*htmlWidget); ok {
			switch {
			case strings.Contains(string(h.Source), "grown-ups-only"):
				hidden = id
			case strings.Contains(string(h.Source), "ava-in-a-group"):
				nested = id
			}
		}
	}
	for _, c := range []struct {
		id   uint64
		user string
		want int
	}{{hidden, "ava", 404}, {hidden, "mom", 200}, {nested, "mom", 404}, {nested, "ava", 200}} {
		r := httptest.NewRequest("GET", "/x", nil)
		r.RemoteAddr = "10.0.0.5:1"
		r.Header.Set("Remote-User", c.user)
		r.SetPathValue("widget", jsonNumber(c.id))
		w := httptest.NewRecorder()
		a.handleWidgetContentRequest(w, r)
		if w.Code != c.want {
			t.Errorf("widget %d as %s: got %d, want %d", c.id, c.user, w.Code, c.want)
		}
	}
}

func jsonNumber(n uint64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestWidgetVisibilityRules(t *testing.T) {
	nested := strings.Replace(widgetAccessConfig, "              - type: html\n                source: <p>ava-in-a-group</p>",
		"              - type: html\n                allowed-users: [mom]\n                source: <p>x</p>", 1)
	cfg, err := newConfigFromYAML([]byte(nested))
	if err == nil {
		_, err = newApplication(cfg)
	}
	if err == nil || !strings.Contains(err.Error(), "directly in a column") {
		t.Fatalf("rules inside a group should be refused, got %v", err)
	}

	noAuth := strings.Replace(strings.Replace(widgetAccessConfig, "auth:\n  proxy: {}\n", "", 1), "household:\n  members:\n    mom: {role: admin}\n    ava: {role: kid}\n", "", 1)
	if _, err := newConfigFromYAML([]byte(noAuth)); err == nil || !strings.Contains(err.Error(), "no auth method") {
		t.Fatalf("rules without sign-in should be refused, got %v", err)
	}
}

func TestDynawidgetsOwner(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Spacegoose02/dynawidgets/refs/heads/main/database/list-w.json":
			json.NewEncoder(w).Encode([]dynawidgetsListEntry{{
				Title: "Weather", Slug: "weather",
				Template: "https://" + r.Host + "/Panonim/dynawidgets/refs/heads/main/widgets/weather.txt",
			}})
		case "/Spacegoose02/dynawidgets/refs/heads/main/widgets/weather.txt":
			w.Write([]byte("from the fork"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	oldHost, oldClient := dynawidgetsTemplateHost, defaultHTTPClient
	dynawidgetsTemplateHost = strings.TrimPrefix(srv.URL, "https://")
	defaultHTTPClient = srv.Client()
	t.Cleanup(func() { dynawidgetsTemplateHost, defaultHTTPClient = oldHost, oldClient })
	dir := t.TempDir()
	setDynawidgetsAssetsDir(dir)
	t.Cleanup(func() { setDynawidgetsAssetsDir("/app/assets/dynawidgets") })

	if dynawidgetsRepoKey("Panonim", "main") != "main" || dynawidgetsRepoKey("", "testing") != "testing" {
		t.Fatal("the default owner keeps plain branch keys")
	}
	key := dynawidgetsRepoKey("Spacegoose02", "main")
	raw, title, err := dynawidgetsRawTemplate("weather", key)
	if err != nil || raw != "from the fork" || title != "Weather" {
		t.Fatalf("got %q %q %v", raw, title, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "weather@Spacegoose02~main.txt")); err != nil {
		t.Fatal("the fork's template should be cached under its own name")
	}

	w := &dynawidgetsWidget{Widget: "weather", Owner: "not/valid"}
	if err := w.initialize(); err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("bad owner: %v", err)
	}
}
