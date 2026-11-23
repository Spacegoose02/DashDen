package dynacat

import (
	"path/filepath"
	"testing"
	"time"
)

func TestProviderSessionsSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dynacat.db")
	now := time.Now()

	before := newSessionStore()
	before.persistTo(path)
	before.set("kept", &oidcSession{Username: "underwood02", Groups: []string{"admins"}, IDToken: "id", CreatedAt: now, Remember: true})
	before.set("plain", &oidcSession{Username: "mom", CreatedAt: now})
	before.set("gone", &oidcSession{Username: "dad", CreatedAt: now})
	before.delete("gone")
	before.set("old", &oidcSession{Username: "kid", CreatedAt: now.Add(-15 * 24 * time.Hour)})

	after := newSessionStore() // a restart
	after.persistTo(path)

	sess, ok := after.get("kept")
	if !ok || sess.Username != "underwood02" || !sess.Remember || sess.IDToken != "id" || len(sess.Groups) != 1 {
		t.Fatalf("a Stay signed in session should come back after a restart, got %+v", sess)
	}
	if _, ok := after.get("plain"); !ok {
		t.Fatal("a plain session still within 14 days should come back too")
	}
	if _, ok := after.get("gone"); ok {
		t.Fatal("a signed-out session should stay signed out")
	}
	if _, ok := after.get("old"); ok {
		t.Fatal("an expired session should not be loaded")
	}
}

func TestStaySignedInProviderSessions(t *testing.T) {
	now := time.Now()
	plain := &oidcSession{CreatedAt: now.Add(-20 * 24 * time.Hour), LastSeen: now}
	if !plain.expired(now) {
		t.Fatal("without Stay signed in a session ends 14 days after sign-in")
	}
	if !plain.cookieExpiry(now).IsZero() {
		t.Fatal("without Stay signed in the cookie should end with the browser")
	}

	kept := &oidcSession{CreatedAt: now.Add(-200 * 24 * time.Hour), LastSeen: now.Add(-85 * 24 * time.Hour), Remember: true}
	if kept.expired(now) {
		t.Fatal("a Stay signed in session used in the last 90 days should still work")
	}
	if kept.cookieExpiry(now).Before(now.Add(80 * 24 * time.Hour)) {
		t.Fatal("Stay signed in should keep the cookie for months")
	}
	if !(&oidcSession{LastSeen: now.Add(-91 * 24 * time.Hour), Remember: true}).expired(now) {
		t.Fatal("a Stay signed in session unused for 90 days should end")
	}

	s := newSessionStore()
	s.set("a", &oidcSession{Username: "underwood02", CreatedAt: now.Add(-2 * 24 * time.Hour), Remember: true})
	if !s.touch("a", now) {
		t.Fatal("using a remembered session should push it back")
	}
	if s.touch("a", now.Add(time.Hour)) {
		t.Fatal("it should only be pushed back once a day")
	}
	if sess, _ := s.get("a"); !sess.LastSeen.Equal(now) {
		t.Fatal("last use should be recorded")
	}
}
