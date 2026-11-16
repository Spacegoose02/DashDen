package dynacat

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServiceWorkerIsServedWithThisBuildAndBase(t *testing.T) {
	a := &application{}
	a.Config.Server.BaseURL = "/den"
	mux := http.NewServeMux()
	a.registerPWARoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/sw.js", nil))
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "__VERSION__") || strings.Contains(body, "__BASE__") {
		t.Fatal("placeholders left in the service worker")
	}
	if !strings.Contains(body, getStaticFSHash()) || !strings.Contains(body, "/den") {
		t.Fatal("service worker doesn't carry the build hash and base URL")
	}
	if rec.Header().Get("Service-Worker-Allowed") != "/den/" {
		t.Fatalf("Service-Worker-Allowed = %q", rec.Header().Get("Service-Worker-Allowed"))
	}
}
