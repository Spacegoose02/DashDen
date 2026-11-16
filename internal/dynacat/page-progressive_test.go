package dynacat

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSlowWidgetGoesOutAsPlaceholder(t *testing.T) {
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.Write([]byte(`<rss><channel><item><title>hi</title><link>https://x</link></item></channel></rss>`))
	}))
	defer slow.Close()

	cfg, err := newConfigFromYAML([]byte(`pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: clock
          - type: rss
            title: Slow feed
            feeds:
              - url: ` + slow.URL + `
`))
	if err != nil {
		t.Fatal(err)
	}
	app, err := newApplication(cfg)
	if err != nil {
		t.Fatal(err)
	}
	p := &app.Config.Pages[0]
	rss := p.Columns[0].Widgets[1]

	p.mu.Lock()
	done, late := p.updateWidgetsWithin(100 * time.Millisecond)
	if !late[rss.GetID()] {
		t.Fatalf("slow widget should be late, got %v", late)
	}
	html := string(templateData{lateWidgets: late}.RenderWidget(rss))
	if !strings.Contains(html, "widget-still-loading") || !strings.Contains(html, "Slow feed") {
		t.Errorf("placeholder: %s", html)
	}

	close(release)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("update never finished")
	}
	p.mu.Unlock()

	if !takeStillLoading(rss.GetID()) || takeStillLoading(rss.GetID()) {
		t.Error("placeholder should be remembered exactly once")
	}
	if !strings.Contains(string(rss.Render()), "hi") {
		t.Error("widget should have its data after the update")
	}
}
