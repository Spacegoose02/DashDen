package dynacat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type denMock struct {
	mu     sync.Mutex
	calls  []string
	bodies []string
	status string
}

func newDenMock(t *testing.T) (*denMock, *httptest.Server) {
	m := &denMock{status: "pending"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		b, _ := io.ReadAll(r.Body)
		m.mu.Lock()
		m.calls = append(m.calls, r.Method+" "+r.URL.Path)
		m.bodies = append(m.bodies, string(b))
		if r.URL.Path == "/api/homes/h1/ha/complete" {
			m.status = "submitted"
		}
		status := m.status
		m.mu.Unlock()

		switch r.Method + " " + r.URL.Path {
		case "GET /api/homes/h1/ha/today":
			json.NewEncoder(w).Encode([]map[string]any{
				{"kid": "Mia", "kid_id": "k1", "chores": []map[string]any{{"id": "c1", "title": "Feed <the> cat", "icon": "🐱", "points": 5, "status": status}}},
				{"kid": "Leo", "kid_id": "k2", "chores": []map[string]any{}},
			})
		case "GET /api/homes/h1/shopping":
			json.NewEncoder(w).Encode([]map[string]any{
				{"id": "s1", "name": "milk", "quantity": 2, "unit": "l", "checked": false},
				{"id": "s2", "name": "eggs", "checked": true},
			})
		case "GET /api/ha/players":
			json.NewEncoder(w).Encode(map[string]any{"players": []map[string]any{
				{"id": "p1", "name": "Kitchen", "state": "playing", "title": "Song", "artist": "Band", "position": 30, "duration": 120},
			}})
		case "GET /api/admin/activity":
			json.NewEncoder(w).Encode(map[string]any{"nowPlaying": []map[string]any{
				{"title": "Bluey", "subtitle": "S1E1", "name": "Mia", "client": "Living room TV", "position": 1, "duration": 2, "paused": true},
			}})
		case "GET /api/family/now-playing":
			json.NewEncoder(w).Encode([]map[string]any{
				{"name": "Leo", "device": "Switch", "title": "Kart", "coverUrl": "/covers/kart.jpg", "href": "/games/1", "remainingSeconds": 1500},
			})
		default:
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)
	return m, srv
}

func denAppForTest(t *testing.T, srv *httptest.Server, app string) *denAppWidget {
	w := &denAppWidget{App: app, URL: srv.URL + "/", Token: "tok", HomeID: "h1"}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	w.update(context.Background())
	if w.Error != nil {
		t.Fatal(w.Error)
	}
	return w
}

func pressDenButton(w *denAppWidget, action, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/widgets/1/action/"+action, strings.NewReader(body))
	r.SetPathValue("action", action)
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	w.handleRequest(rec, r)
	return rec
}

func TestDenAppValidation(t *testing.T) {
	for _, w := range []*denAppWidget{
		{App: "nope", URL: "http://x", Token: "t"},
		{App: "den-duty", URL: "http://x", Token: "t"},
		{App: "wavelength", URL: "http://x"},
	} {
		if err := w.initialize(); err == nil {
			t.Fatalf("%+v should not initialize", w)
		}
	}
}

func TestDenDutyChores(t *testing.T) {
	m, srv := newDenMock(t)
	w := denAppForTest(t, srv, "den-duty")

	html := string(w.Render())
	if !strings.Contains(html, "Feed &lt;the&gt; cat") || !strings.Contains(html, `data-den-action="complete"`) {
		t.Fatalf("expected an escaped chore with a Done button:\n%s", html)
	}

	rec := pressDenButton(w, "complete", `{"kid":"Mia","chore":"c1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete: %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `data-den-action="approve"`) {
		t.Fatal("after Done the chore should wait for approval, and the fresh render should show it")
	}
	if got := m.bodies[len(m.bodies)-2]; got != `{"chore":"c1","kid":"Mia"}` {
		t.Fatalf("complete body was %s", got)
	}

	w.Kid = "leo"
	w.update(context.Background())
	if len(w.Kids) != 1 || w.Kids[0].Kid != "Leo" {
		t.Fatalf("kid filter should keep only Leo, got %+v", w.Kids)
	}
}

func TestDenDutyApproveNeedsGroup(t *testing.T) {
	_, srv := newDenMock(t)
	w := denAppForTest(t, srv, "den-duty")
	w.ApproveGroups = []string{"adult"}

	kid := &authenticatedUser{Username: "mia", Groups: []string{"kid"}}
	if _, _, _, err := w.planAction("approve", map[string]string{"kid": "Mia", "chore": "c1"}, kid); err == nil {
		t.Fatal("a kid should not approve chores")
	}
	if _, _, _, err := w.planAction("approve", map[string]string{"kid": "Mia", "chore": "c1"}, nil); err == nil {
		t.Fatal("an unknown visitor should not approve chores")
	}
	adult := &authenticatedUser{Username: "dad", Groups: []string{"adult"}}
	if _, path, _, err := w.planAction("approve", map[string]string{"kid": "Mia", "chore": "c1"}, adult); err != nil || path != "/api/homes/h1/ha/approve" {
		t.Fatalf("adult approve: %s %v", path, err)
	}
	if _, _, _, err := w.planAction("delete", map[string]string{}, adult); err == nil {
		t.Fatal("unknown actions must be refused")
	}
}

func TestDenAppActionGroupsAndContentType(t *testing.T) {
	m, srv := newDenMock(t)
	w := denAppForTest(t, srv, "nighttime-ninja")
	w.ActionGroups = []string{"adult"}

	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"text":"bread"}`))
	r.SetPathValue("action", "add")
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	w.handleRequest(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("without the group the button should be refused, got %d", rec.Code)
	}

	w.ActionGroups = nil
	r = httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"text":"bread"}`))
	r.SetPathValue("action", "add")
	r.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	w.handleRequest(rec, r)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("a form post should be refused, got %d", rec.Code)
	}
	for _, c := range m.calls {
		if strings.HasPrefix(c, "POST") {
			t.Fatalf("nothing should have reached the app, saw %s", c)
		}
	}
}

func TestNinjaShopping(t *testing.T) {
	m, srv := newDenMock(t)
	w := denAppForTest(t, srv, "nighttime-ninja")
	if len(w.Shopping) != 1 || w.Shopping[0].Label() != "2 l milk" {
		t.Fatalf("checked items should be hidden and quantities shown, got %+v", w.Shopping)
	}

	if rec := pressDenButton(w, "add", `{"text":"  bread "}`); rec.Code != http.StatusOK {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	if rec := pressDenButton(w, "check", `{"id":"s1"}`); rec.Code != http.StatusOK {
		t.Fatalf("check: %d %s", rec.Code, rec.Body)
	}
	joined := strings.Join(m.calls, "\n")
	if !strings.Contains(joined, "POST /api/homes/h1/shopping") || !strings.Contains(joined, "PATCH /api/homes/h1/shopping/s1") {
		t.Fatalf("unexpected calls:\n%s", joined)
	}
	if rec := pressDenButton(w, "add", `{"text":"   "}`); rec.Code != http.StatusBadRequest {
		t.Fatal("empty items should be refused")
	}
}

func TestWavelengthControls(t *testing.T) {
	m, srv := newDenMock(t)
	w := denAppForTest(t, srv, "wavelength")
	if len(w.Players) != 1 || w.Players[0].Percent() != 25 {
		t.Fatalf("got %+v", w.Players)
	}
	if rec := pressDenButton(w, "control", `{"id":"p1","op":"next"}`); rec.Code != http.StatusOK {
		t.Fatalf("next: %d %s", rec.Code, rec.Body)
	}
	if m.bodies[len(m.bodies)-2] != `{"action":"next"}` {
		t.Fatalf("body was %s", m.bodies[len(m.bodies)-2])
	}
	if rec := pressDenButton(w, "control", `{"id":"p1","op":"volume"}`); rec.Code != http.StatusBadRequest {
		t.Fatal("only toggle, next and prev are allowed")
	}
}

func TestDenNow(t *testing.T) {
	_, srv := newDenMock(t)
	w := &denNowWidget{Sources: []denNowSource{
		{App: "bingebandit", URL: srv.URL, Token: "tok"},
		{App: "lootden", URL: srv.URL, Token: "tok"},
		{App: "wavelength", URL: srv.URL, Token: "tok"},
		{App: "lootden", URL: srv.URL, Token: "wrong", Name: "Broken"},
	}}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	w.update(context.Background())

	if len(w.Items) != 3 {
		t.Fatalf("expected 3 items, got %+v", w.Items)
	}
	if !w.Items[2].Paused || w.Items[2].Title != "Bluey" {
		t.Fatal("paused items should sort last")
	}
	for _, it := range w.Items {
		if it.Title == "Kart" && (it.ImageURL != srv.URL+"/covers/kart.jpg" || it.Subtitle != "25 min of screen time left") {
			t.Fatalf("lootden item: %+v", it)
		}
	}
	if len(w.Failed) != 1 || w.Failed[0] != "Broken" {
		t.Fatalf("failed: %v", w.Failed)
	}
	html := string(w.Render())
	if !strings.Contains(html, "Leo is playing") || !strings.Contains(html, "Couldn't reach Broken") {
		t.Fatalf("render:\n%s", html)
	}
}
