package dynacat

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const OIDC_SESSION_COOKIE_NAME = "oidc_session"
const OIDC_SESSION_VALID_PERIOD = 14 * 24 * time.Hour

// "Stay signed in" provider sessions last this long without a visit, like
// password ones, and are pushed back at most once a day while they're used.
const OIDC_REMEMBER_VALID_PERIOD = AUTH_REMEMBER_VALID_PERIOD
const OIDC_REMEMBER_TOUCH_EVERY = 24 * time.Hour

type oidcSession struct {
	Username  string
	Groups    []string
	IDToken   string // for id_token_hint when signing out at the provider
	CreatedAt time.Time
	LastSeen  time.Time
	Remember  bool
}

// expired: a plain session ends 14 days after sign-in; a "Stay signed in"
// one ends 90 days after it was last used.
func (s *oidcSession) expired(now time.Time) bool {
	if s.Remember {
		last := s.LastSeen
		if last.IsZero() {
			last = s.CreatedAt
		}
		return now.Sub(last) > OIDC_REMEMBER_VALID_PERIOD
	}
	return now.Sub(s.CreatedAt) > OIDC_SESSION_VALID_PERIOD
}

// cookieExpiry is when the browser should drop the cookie: never set (ends
// with the browser) for a plain session, 90 days on for "Stay signed in".
func (s *oidcSession) cookieExpiry(now time.Time) time.Time {
	if s.Remember {
		return now.Add(OIDC_REMEMBER_VALID_PERIOD)
	}
	return time.Time{}
}

// sessionStore keeps provider sign-ins. They live in memory and, once
// persistTo has been given the database, in the oidc_sessions table too, so
// restarting or updating DashDen doesn't sign everyone out. Only a hash of
// each session ID is written to disk.
type sessionStore struct {
	sessions sync.Map

	mu   sync.Mutex
	path string
	db   *sql.DB
}

// Shared across config reloads so a reload does not log OIDC users out.
var sharedOIDCSessions = newSessionStore()

func newSessionStore() *sessionStore {
	return &sessionStore{}
}

func sessionKey(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])
}

// persistTo saves sessions in the database at path and loads the ones saved
// there before. Calling it again with the same path does nothing. If the
// database can't be used, sessions stay in memory only.
func (s *sessionStore) persistTo(path string) {
	if path == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == path {
		return
	}

	db, err := openSQLite(path)
	if err == nil {
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS oidc_sessions (
			id_hash    TEXT    PRIMARY KEY,
			username   TEXT    NOT NULL,
			groups     TEXT    NOT NULL DEFAULT '[]',
			id_token   TEXT    NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			last_seen  INTEGER NOT NULL,
			remember   INTEGER NOT NULL DEFAULT 0
		)`)
	}
	if err != nil {
		if db != nil {
			db.Close()
		}
		slog.Error("Could not save single sign-on sessions, people will be signed out when DashDen restarts", "path", path, "error", err)
		return
	}

	if s.db != nil {
		s.db.Close()
	}
	s.db, s.path = db, path

	// Sessions made before this (none on a fresh start) are kept and saved.
	s.sessions.Range(func(k, v any) bool {
		if sess, ok := v.(*oidcSession); ok {
			s.saveLocked(k.(string), sess)
		}
		return true
	})

	now := time.Now()
	rows, err := db.Query(`SELECT id_hash, username, groups, id_token, created_at, last_seen, remember FROM oidc_sessions`)
	if err != nil {
		slog.Error("Could not load saved single sign-on sessions", "error", err)
		return
	}
	defer rows.Close()
	var stale []string
	for rows.Next() {
		var key, groups string
		var created, seen int64
		var remember int
		sess := &oidcSession{}
		if err := rows.Scan(&key, &sess.Username, &groups, &sess.IDToken, &created, &seen, &remember); err != nil {
			continue
		}
		json.Unmarshal([]byte(groups), &sess.Groups)
		sess.CreatedAt, sess.LastSeen, sess.Remember = time.Unix(created, 0), time.Unix(seen, 0), remember != 0
		if sess.expired(now) {
			stale = append(stale, key)
			continue
		}
		if _, exists := s.sessions.Load(key); !exists {
			s.sessions.Store(key, sess)
		}
	}
	rows.Close()
	for _, key := range stale {
		db.Exec(`DELETE FROM oidc_sessions WHERE id_hash = ?`, key)
	}
}

func (s *sessionStore) saveLocked(key string, sess *oidcSession) {
	if s.db == nil {
		return
	}
	groups, _ := json.Marshal(sess.Groups)
	if sess.Groups == nil {
		groups = []byte("[]")
	}
	remember := 0
	if sess.Remember {
		remember = 1
	}
	_, err := s.db.Exec(`INSERT OR REPLACE INTO oidc_sessions
		(id_hash, username, groups, id_token, created_at, last_seen, remember)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		key, sess.Username, string(groups), sess.IDToken, sess.CreatedAt.Unix(), sess.LastSeen.Unix(), remember)
	if err != nil {
		slog.Error("Could not save a single sign-on session", "error", err)
	}
}

func (s *sessionStore) deleteLocked(key string) {
	if s.db == nil {
		return
	}
	if _, err := s.db.Exec(`DELETE FROM oidc_sessions WHERE id_hash = ?`, key); err != nil {
		slog.Error("Could not remove a saved single sign-on session", "error", err)
	}
}

func (s *sessionStore) set(id string, session *oidcSession) {
	if session.LastSeen.IsZero() {
		session.LastSeen = session.CreatedAt
	}
	key := sessionKey(id)
	s.sessions.Store(key, session)
	s.mu.Lock()
	s.saveLocked(key, session)
	s.mu.Unlock()
}

func (s *sessionStore) get(id string) (*oidcSession, bool) {
	v, ok := s.sessions.Load(sessionKey(id))
	if !ok {
		return nil, false
	}
	sess, ok := v.(*oidcSession)
	return sess, ok
}

// touch notes that a "Stay signed in" session was just used, and reports
// whether it did (so the cookie's expiry should be pushed back too). It only
// writes once a day per session.
func (s *sessionStore) touch(id string, now time.Time) bool {
	key := sessionKey(id)
	v, ok := s.sessions.Load(key)
	if !ok {
		return false
	}
	sess, ok := v.(*oidcSession)
	if !ok || !sess.Remember || now.Sub(sess.LastSeen) < OIDC_REMEMBER_TOUCH_EVERY {
		return false
	}
	moved := *sess
	moved.LastSeen = now
	s.sessions.Store(key, &moved)
	s.mu.Lock()
	s.saveLocked(key, &moved)
	s.mu.Unlock()
	return true
}

func (s *sessionStore) delete(id string) {
	key := sessionKey(id)
	s.sessions.Delete(key)
	s.mu.Lock()
	s.deleteLocked(key)
	s.mu.Unlock()
}

func (s *sessionStore) sweepExpired() {
	now := time.Now()
	s.sessions.Range(func(k, v any) bool {
		sess, ok := v.(*oidcSession)
		if !ok || sess.expired(now) {
			s.sessions.Delete(k)
			s.mu.Lock()
			s.deleteLocked(k.(string))
			s.mu.Unlock()
		}
		return true
	})
}

func (s *sessionStore) runSweeper(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sweepExpired()
		}
	}
}

// renameUser moves signed-in provider sessions to the new name.
func (s *sessionStore) renameUser(oldName, newName string) {
	s.sessions.Range(func(k, v any) bool {
		if sess, ok := v.(*oidcSession); ok && strings.EqualFold(sess.Username, oldName) {
			moved := *sess
			moved.Username = newName
			s.sessions.Store(k, &moved)
			s.mu.Lock()
			s.saveLocked(k.(string), &moved)
			s.mu.Unlock()
		}
		return true
	})
}
