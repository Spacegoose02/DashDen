package dynacat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
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
		if a := r.Header.Get("Authorization"); a != "Bearer tok" && !(a == "Bot tok" && strings.HasPrefix(r.URL.Path, "/api/bot/")) {
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
				{"id": 7, "name": "Kitchen", "state": "playing", "title": "Song", "artist": "Band", "position": 30, "duration": 120},
			}})
		case "GET /api/admin/activity":
			json.NewEncoder(w).Encode(map[string]any{"nowPlaying": []map[string]any{
				{"title": "Bluey", "subtitle": "S1E1", "name": "Mia", "client": "Living room TV", "position": 1, "duration": 2, "paused": true,
					"itemId": 42, "poster": "/api/images/tmdb/bluey.jpg"},
			}})
		case "GET /api/family/now-playing":
			json.NewEncoder(w).Encode([]map[string]any{
				{"name": "Leo", "device": "Switch", "title": "Kart", "coverUrl": "/covers/kart.jpg", "href": "/games/1", "remainingSeconds": 1500},
			})
		case "GET /api/v1/homes/h1/parent-duty":
			json.NewEncoder(w).Encode([]map[string]any{
				{"id": "d1", "title": "Fix sink", "status": "doing", "dueDate": nil},
				{"id": "d2", "title": "Call <dentist>", "status": "todo", "dueDate": "2000-01-01"},
				{"id": "d3", "title": "Finished", "status": "done"},
			})
		case "GET /api/homes/h1/items":
			json.NewEncoder(w).Encode([]map[string]any{
				{"id": "i1", "name": "Flour", "quantity": 0.5, "unit": "kg"},
				{"id": "i2", "name": "Milk", "onShoppingList": true},
			})
		case "GET /api/time-requests":
			json.NewEncoder(w).Encode([]map[string]any{
				{"id": 7, "name": "Leo", "minutes": 30, "note": "one more race", "state": "pending"},
				{"id": 6, "name": "Mia", "minutes": 15, "state": "declined"},
			})
		case "GET /api/bot/v1/servers":
			json.NewEncoder(w).Encode([]map[string]any{
				{"name": "Family", "channels": []map[string]any{{"id": 42, "kind": "text", "name": "general"}, {"id": 43, "kind": "voice", "name": "Lounge"}}},
				{"name": "Gaming", "channels": []map[string]any{{"id": 77, "kind": "text", "name": "general"}, {"id": 78, "kind": "text", "name": "clips"}}},
			})
		case "GET /api/bot/v1/channels/42/messages":
			json.NewEncoder(w).Encode([]map[string]any{
				{"body": "On my way", "createdAt": "2026-11-16T01:00:00.000Z", "author": map[string]any{"displayName": "Mom"}},
				{"body": "secret", "keyVersion": 2, "createdAt": "2026-11-16T01:01:00.000Z", "author": map[string]any{"displayName": "Leo"}},
				{"body": "", "deleted": true, "createdAt": "2026-11-16T01:02:00.000Z"},
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
	if rec := pressDenButton(w, "control", `{"id":"7","op":"next"}`); rec.Code != http.StatusOK {
		t.Fatalf("next: %d %s", rec.Code, rec.Body)
	}
	if m.bodies[len(m.bodies)-2] != `{"action":"next"}` {
		t.Fatalf("body was %s", m.bodies[len(m.bodies)-2])
	}
	if rec := pressDenButton(w, "control", `{"id":"7","op":"volume"}`); rec.Code != http.StatusBadRequest {
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
	if bluey := w.Items[2]; bluey.ImageURL != srv.URL+"/api/images/tmdb/bluey.jpg?token=tok&w=w185" || bluey.Link != srv.URL+"/item/42" {
		t.Fatalf("bingebandit poster and link: %+v", bluey)
	}
	if len(w.Failed) != 1 || w.Failed[0] != "Broken" {
		t.Fatalf("failed: %v", w.Failed)
	}
	html := string(w.Render())
	if !strings.Contains(html, "Leo is playing") || !strings.Contains(html, "Couldn't reach Broken") {
		t.Fatalf("render:\n%s", html)
	}
}

func TestDenAppViewsValidate(t *testing.T) {
	for _, w := range []*denAppWidget{
		{App: "wavelength", URL: "http://x", Token: "t", View: "low"},
		{App: "den-duty", URL: "http://x", Token: "t", HomeID: "h", View: "low"},
		{App: "dendrop", URL: "http://x", Token: "t"},
	} {
		if err := w.initialize(); err == nil {
			t.Fatalf("%s with view %q should be refused", w.App, w.View)
		}
	}
}

func TestDenDutyParentDuty(t *testing.T) {
	m, srv := newDenMock(t)
	w := &denAppWidget{App: "den-duty", View: "parent-duty", URL: srv.URL, Token: "tok", HomeID: "h1"}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	w.update(context.Background())
	if w.Title != "Parent to-dos" || len(w.Duties) != 2 || w.Duties[0].ID != "d2" || w.Duties[0].Due() != "Overdue" {
		t.Fatalf("done cards should be hidden and dated ones first: %+v", w.Duties)
	}
	if html := string(w.Render()); !strings.Contains(html, "Call &lt;dentist&gt;") {
		t.Fatalf("render:\n%s", html)
	}
	if rec := pressDenButton(w, "done", `{"id":"d2"}`); rec.Code != http.StatusOK {
		t.Fatalf("done: %d %s", rec.Code, rec.Body)
	}
	if !slices.Contains(m.calls, "PATCH /api/v1/homes/h1/parent-duty/d2") || !slices.Contains(m.bodies, `{"status":"done"}`) {
		t.Fatalf("calls: %v %v", m.calls, m.bodies)
	}
}

func TestNinjaLow(t *testing.T) {
	m, srv := newDenMock(t)
	w := &denAppWidget{App: "nighttime-ninja", View: "low", URL: srv.URL, Token: "tok", HomeID: "h1"}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	w.update(context.Background())
	if len(w.Low) != 2 || w.Low[0].Left() != "0.5 kg left" || w.Low[1].Left() != "low" {
		t.Fatalf("low: %+v", w.Low)
	}
	if rec := pressDenButton(w, "add", `{"id":"i1"}`); rec.Code != http.StatusOK {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	if !slices.Contains(m.bodies, `{"text":"Flour"}`) {
		t.Fatalf("the item's own name should be added: %v", m.bodies)
	}
	if rec := pressDenButton(w, "add", `{"id":"nope"}`); rec.Code != http.StatusBadRequest {
		t.Fatal("unknown items should be refused")
	}
}

func TestLootDenTimeAsks(t *testing.T) {
	m, srv := newDenMock(t)
	w := &denAppWidget{App: "lootden", URL: srv.URL, Token: "tok", ApproveGroups: []string{"adult"}}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	w.update(context.Background())
	if len(w.TimeAsks) != 1 || w.TimeAsks[0].ID != 7 {
		t.Fatalf("only pending asks: %+v", w.TimeAsks)
	}
	// Nobody is signed in here, so the approve-groups check refuses it.
	if rec := pressDenButton(w, "approve", `{"id":"7"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("approve without a grown-up: %d", rec.Code)
	}
	w.ApproveGroups = nil
	if rec := pressDenButton(w, "approve", `{"id":"7"}`); rec.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body)
	}
	if !slices.Contains(m.calls, "POST /api/time-requests/7/approve") {
		t.Fatalf("calls: %v", m.calls)
	}
}

func TestDendropQuickMessages(t *testing.T) {
	m, srv := newDenMock(t)
	w := &denAppWidget{App: "dendrop", URL: srv.URL, Token: "tok", Channel: "42", Messages: []string{"Dinner's ready!", " "}}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	w.update(context.Background())
	if w.Error != nil || len(w.Chat) != 2 || w.Chat[1].Text() != "🔒 Encrypted message" || len(w.Messages) != 1 {
		t.Fatalf("chat: %+v %v %v", w.Chat, w.Messages, w.Error)
	}
	if rec := pressDenButton(w, "post", `{"id":"0"}`); rec.Code != http.StatusOK {
		t.Fatalf("post: %d %s", rec.Code, rec.Body)
	}
	if !slices.Contains(m.calls, "POST /api/bot/v1/channels/42/messages") || !slices.Contains(m.bodies, `{"content":"Dinner's ready!"}`) {
		t.Fatalf("calls: %v %v", m.calls, m.bodies)
	}
	if rec := pressDenButton(w, "post", `{"id":"1"}`); rec.Code != http.StatusBadRequest {
		t.Fatal("only configured messages can be sent")
	}
}

func TestDendropChannelByName(t *testing.T) {
	_, srv := newDenMock(t)
	for channel, want := range map[string]string{"42": "42", "Family/#general": "42", "family/General": "42", "#clips": "78"} {
		w := &denAppWidget{App: "dendrop", URL: srv.URL, Token: "tok", Channel: channel}
		if err := w.initialize(); err != nil {
			t.Fatal(err)
		}
		if err := w.resolveDendropChannel(context.Background()); err != nil || w.channelID != want {
			t.Fatalf("%q: got %q, %v", channel, w.channelID, err)
		}
	}
	for _, channel := range []string{"general", "Lounge", "nope"} {
		w := &denAppWidget{App: "dendrop", URL: srv.URL, Token: "tok", Channel: channel}
		w.initialize()
		if err := w.resolveDendropChannel(context.Background()); err == nil {
			t.Fatalf("%q should be refused (two servers, voice channel, or missing)", channel)
		}
	}
}
