package dynacat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func familyBoardApp(t *testing.T, extra string) *application {
	t.Helper()
	db := filepath.Join(t.TempDir(), "test.db")
	cfg := `
server:
  trusted-proxies: [10.0.0.0/24]
  db-path: ` + db + `
auth:
  proxy: {}
household:
  members:
    mom: {name: Mom, role: admin, avatar: "🦝"}
    dad: {name: Dad, role: adult}
    ava: {name: Ava, role: kid}
pages:
  - name: Home
    slug: home
    columns:
      - size: full
        widgets:
          - type: notice-board
            id: family
` + extra
	return newWidgetAccessApp(t, cfg)
}

func boardOf(a *application) *noticeBoardWidget {
	for _, w := range a.widgetByID {
		if b, ok := w.(*noticeBoardWidget); ok {
			return b
		}
	}
	return nil
}

func boardAction(b *noticeBoardWidget, user, action, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/x", strings.NewReader(body))
	r.RemoteAddr = "10.0.0.5:1"
	r.Header.Set("Remote-User", user)
	r.Header.Set("Content-Type", "application/json")
	r.SetPathValue("action", action)
	w := httptest.NewRecorder()
	b.handleRequest(w, r)
	return w
}

func TestNoticeBoard(t *testing.T) {
	a := familyBoardApp(t, "")
	b := boardOf(a)

	w := boardAction(b, "mom", "post", `{"text":"  Pizza night <3  "}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Pizza night &lt;3") || !strings.Contains(w.Body.String(), "Mom") {
		t.Fatalf("post: %d %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "🦝") {
		t.Fatal("the household avatar should show next to the note")
	}
	if w := boardAction(b, "mom", "post", `{"text":"`+strings.Repeat("x", 281)+`"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("too long: %d", w.Code)
	}
	if w := boardAction(b, "mom", "post", `{"text":"   "}`); w.Code != http.StatusBadRequest {
		t.Fatalf("empty: %d", w.Code)
	}

	id := strconv.FormatInt(b.Notes[0].ID, 10)
	if w := boardAction(b, "ava", "delete", `{"id":"`+id+`"}`); w.Code != http.StatusForbidden {
		t.Fatalf("a kid removing mom's note: %d", w.Code)
	}
	boardAction(b, "ava", "post", `{"text":"can we get a dog"}`)
	avaID := strconv.FormatInt(b.Notes[0].ID, 10)
	if w := boardAction(b, "ava", "delete", `{"id":"`+avaID+`"}`); w.Code != http.StatusOK {
		t.Fatalf("removing your own note: %d", w.Code)
	}
	if w := boardAction(b, "mom", "delete", `{"id":"`+id+`"}`); w.Code != http.StatusOK || len(b.Notes) != 0 {
		t.Fatalf("admin remove: %d, %d notes left", w.Code, len(b.Notes))
	}
}

func TestNoticeBoardPostGroupsAndExpiry(t *testing.T) {
	a := familyBoardApp(t, "            post-groups: [adult]\n            keep-for: 1h\n")
	b := boardOf(a)
	if w := boardAction(b, "ava", "post", `{"text":"hi"}`); w.Code != http.StatusForbidden {
		t.Fatalf("kid posting on an adults board: %d", w.Code)
	}
	if w := boardAction(b, "dad", "post", `{"text":"hi"}`); w.Code != http.StatusOK {
		t.Fatalf("adult: %d", w.Code)
	}
	a.noticeStorage.db.Exec(`UPDATE notices SET created = ?`, time.Now().Add(-2*time.Hour).Unix())
	b.update(t.Context())
	if len(b.Notes) != 0 {
		t.Fatal("notes older than keep-for should be gone")
	}
}

func TestPerPersonTodo(t *testing.T) {
	a := familyBoardApp(t, "          - type: to-do\n            id: chores\n            storage: server\n            per-person: true\n")

	call := func(method, user, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/todo/chores", strings.NewReader(body))
		r.RemoteAddr = "10.0.0.5:1"
		if user != "" {
			r.Header.Set("Remote-User", user)
		}
		r.SetPathValue("listID", "chores")
		w := httptest.NewRecorder()
		if method == "PUT" {
			a.handleTodoSave(w, r)
		} else {
			a.handleTodoLoad(w, r)
		}
		return w
	}

	if w := call("PUT", "ava", `[{"text":"feed the cat"}]`); w.Code != http.StatusNoContent {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	call("PUT", "dad", `[{"text":"fix the gate"},{"text":"call grandma"}]`)

	var ava, dad []todoTask
	json.Unmarshal(call("GET", "ava", "").Body.Bytes(), &ava)
	json.Unmarshal(call("GET", "dad", "").Body.Bytes(), &dad)
	if len(ava) != 1 || ava[0].Text != "feed the cat" || len(dad) != 2 {
		t.Fatalf("lists should be separate: ava=%v dad=%v", ava, dad)
	}
	if w := call("GET", "", ""); w.Code == http.StatusOK && strings.Contains(w.Body.String(), "feed") {
		t.Fatal("someone not signed in must not get a personal list")
	}

	cfg, err := newConfigFromYAML([]byte("pages:\n  - name: Home\n    columns:\n      - size: full\n        widgets:\n          - type: to-do\n            id: x\n            storage: server\n            per-person: true\n"))
	if err == nil {
		_, err = newApplication(cfg)
	}
	if err == nil || !strings.Contains(err.Error(), "sign-in") {
		t.Fatalf("per-person without sign-in: %v", err)
	}
}
