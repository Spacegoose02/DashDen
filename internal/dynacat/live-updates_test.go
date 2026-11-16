package dynacat

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPushTodoChangeReachesOnlyTheOwnerOfAPersonalList(t *testing.T) {
	pg := &page{}
	a := &application{
		DynamicUpdateEnabled: true,
		RequiresAuth:         true,
		todoListIDToPage:     map[string]*page{"chores": pg},
		sseClients:           map[*sseClient]struct{}{},
	}
	alice := &sseClient{ch: make(chan string, 1), user: &authenticatedUser{Username: "alice"}}
	bob := &sseClient{ch: make(chan string, 1), user: &authenticatedUser{Username: "bob"}}
	a.sseClients[alice] = struct{}{}
	a.sseClients[bob] = struct{}{}

	r := httptest.NewRequest("PUT", "/api/todo/chores", nil)
	r.Header.Set("X-DashDen-Tab", "tab1")
	a.pushTodoChange(r, "chores\x1falice")

	select {
	case msg := <-alice.ch:
		if !strings.Contains(msg, `"todoId":"chores"`) || !strings.Contains(msg, `"from":"tab1"`) {
			t.Fatalf("unexpected message %s", msg)
		}
	default:
		t.Fatal("alice didn't hear about her own list")
	}
	select {
	case msg := <-bob.ch:
		t.Fatalf("bob heard about alice's list: %s", msg)
	default:
	}

	a.pushTodoChange(r, "chores")
	if len(alice.ch) != 1 || len(bob.ch) != 1 {
		t.Fatal("a shared list should reach everyone")
	}
}

func TestStatusRecorder(t *testing.T) {
	rec := &statusRecorder{ResponseWriter: httptest.NewRecorder()}
	rec.Write([]byte("ok"))
	if !rec.succeeded() {
		t.Fatal("a plain write is a success")
	}
	rec = &statusRecorder{ResponseWriter: httptest.NewRecorder()}
	rec.WriteHeader(403)
	if rec.succeeded() {
		t.Fatal("403 isn't a success")
	}
}
