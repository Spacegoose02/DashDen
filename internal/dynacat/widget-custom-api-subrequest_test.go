package dynacat

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCustomAPIFailedSubrequestDoesNotFailWidget(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"name":"primary"}`))
	}))
	defer ok.Close()

	down := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	downURL := down.URL
	down.Close() // connection refused from here on

	primary := &CustomAPIRequest{URL: ok.URL}
	sub := &CustomAPIRequest{URL: downURL}
	for _, r := range []*CustomAPIRequest{primary, sub} {
		if err := r.initialize(); err != nil {
			t.Fatal(err)
		}
	}

	tmpl := template.Must(template.New("").Funcs(customAPITemplateFuncs(nil, 0)).Parse(
		`{{ .JSON.String "name" }}|{{ $s := .Subrequest "extra" }}{{ if $s }}has-extra{{ else }}no-extra{{ end }}`,
	))

	html, _, _, err := fetchAndRenderCustomAPIRequest(primary, map[string]*CustomAPIRequest{"extra": sub}, nil, tmpl)
	if err != nil {
		t.Fatalf("widget failed because of its subrequest: %v", err)
	}
	if got := strings.TrimSpace(string(html)); got != "primary|no-extra" {
		t.Fatalf("got %q", got)
	}
}

func TestCustomAPIFailedPrimaryStillFailsWidget(t *testing.T) {
	down := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	downURL := down.URL
	down.Close()

	primary := &CustomAPIRequest{URL: downURL}
	if err := primary.initialize(); err != nil {
		t.Fatal(err)
	}
	tmpl := template.Must(template.New("").Parse(`x`))
	if _, _, _, err := fetchAndRenderCustomAPIRequest(primary, map[string]*CustomAPIRequest{}, nil, tmpl); err == nil {
		t.Fatal("expected an error when the primary request fails")
	}
}
