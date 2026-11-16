package dynacat

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const loginTestConfig = `
auth:
  users:
    Underwood02:
      password: correct-horse
pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: clock
`

func newLoginTestApp(t *testing.T, cfgText string) *application {
	t.Helper()
	t.Setenv(generatedSecretKeyName, "")
	os.Unsetenv(generatedSecretKeyName)
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

func loginAttempt(a *application, username, password string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	r := httptest.NewRequest("POST", "/api/authenticate", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	r.RemoteAddr = "192.0.2.7:1234"
	w := httptest.NewRecorder()
	a.handleAuthenticationAttempt(w, r)
	return w
}

func TestLoginIgnoresCapitalLettersAndMakesASecret(t *testing.T) {
	a := newLoginTestApp(t, loginTestConfig)
	if os.Getenv(generatedSecretKeyName) == "" {
		t.Fatal("a missing secret-key should be generated")
	}

	w := loginAttempt(a, "underwood02", "nope-nope-nope")
	var res struct{ TriesLeft int }
	json.Unmarshal(w.Body.Bytes(), &res)
	if w.Code != http.StatusUnauthorized || res.TriesLeft != AUTH_RATE_LIMIT_MAX_ATTEMPTS-1 {
		t.Fatalf("wrong password: %d %s", w.Code, w.Body)
	}

	if w := loginAttempt(a, "  UNDERWOOD02 ", "correct-horse"); w.Code != http.StatusOK {
		t.Fatalf("a different case should still sign in, got %d", w.Code)
	}
	if u := a.verifyUserPassword("underwood02", "correct-horse"); u == nil || u.Username != "Underwood02" {
		t.Fatalf("the config's spelling should be used, got %+v", u)
	}
}

func TestLoginConfigChecks(t *testing.T) {
	dup := strings.Replace(loginTestConfig, "  users:\n", "  users:\n    underwood02:\n      password: another-one\n", 1)
	if _, err := newConfigFromYAML([]byte(dup)); err == nil || !strings.Contains(err.Error(), "capital letters") {
		t.Fatalf("names that differ only in case: %v", err)
	}
	short := strings.Replace(loginTestConfig, "correct-horse", "short", 1)
	if _, err := newConfigFromYAML([]byte(short)); err == nil || !strings.Contains(err.Error(), "8 characters") {
		t.Fatalf("short password: %v", err)
	}
	if _, err := newConfigFromYAML([]byte(strings.Replace(loginTestConfig, "correct-horse", "eight888", 1))); err != nil {
		t.Fatalf("8 characters is enough: %v", err)
	}
}

func TestResolveAuthSecretKey(t *testing.T) {
	raw := make([]byte, AUTH_SECRET_KEY_LENGTH)
	raw[0] = 7
	if b, err := resolveAuthSecretKey(base64.StdEncoding.EncodeToString(raw)); err != nil || b[0] != 7 {
		t.Fatal("an upstream-style key should be used as is")
	}
	a, err1 := resolveAuthSecretKey("any long random text here")
	b, err2 := resolveAuthSecretKey("any long random text here")
	if err1 != nil || err2 != nil || len(a) != AUTH_SECRET_KEY_LENGTH || string(a) != string(b) {
		t.Fatal("other text should be stretched into a stable key")
	}
	if _, err := resolveAuthSecretKey("short"); err == nil {
		t.Fatal("a too-short key should be refused with advice")
	}

	t.Setenv(generatedSecretKeyName, "")
	var saved string
	saveGeneratedSecret = func(name, value string) error { saved = name + "=" + value; return nil }
	t.Cleanup(func() { saveGeneratedSecret = nil })
	if _, err := resolveAuthSecretKey(""); err != nil || !strings.HasPrefix(saved, generatedSecretKeyName+"=") {
		t.Fatalf("a generated key should be saved: %v %q", err, saved)
	}
}

func TestHouseholdIgnoresCapitalLetters(t *testing.T) {
	a := newHouseholdTestApp(t)
	u := a.getAuthenticatedUser(httptest.NewRecorder(), proxyRequest("10.0.0.5:1", map[string]string{"Remote-User": "Mom"}))
	if u == nil || u.Member() == nil || u.Username != "mom" {
		t.Fatalf("expected Mom's profile under the household spelling, got %+v", u)
	}
}

func TestWatchEnvFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "dynacat.yml")
	var calls atomic.Int32
	stop := watchEnvFile(configPath, func() { calls.Add(1) })
	defer stop()

	os.WriteFile(filepath.Join(dir, ".env"), []byte("A=1\n"), 0o600)
	deadline := time.Now().Add(5 * time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if calls.Load() == 0 {
		t.Fatal("creating config/.env should trigger a reload")
	}
}

func TestStaySignedIn(t *testing.T) {
	a := newLoginTestApp(t, loginTestConfig)
	login := func(remember bool) *http.Cookie {
		body, _ := json.Marshal(map[string]any{"username": "underwood02", "password": "correct-horse", "remember": remember})
		r := httptest.NewRequest("POST", "/api/authenticate", strings.NewReader(string(body)))
		r.Header.Set("Content-Type", "application/json")
		r.RemoteAddr = "192.0.2.8:1"
		w := httptest.NewRecorder()
		a.handleAuthenticationAttempt(w, r)
		for _, c := range w.Result().Cookies() {
			if c.Name == AUTH_SESSION_COOKIE_NAME {
				return c
			}
		}
		t.Fatalf("no session cookie: %d", w.Code)
		return nil
	}

	short := login(false)
	if !short.Expires.IsZero() {
		t.Fatal("without Stay signed in the cookie should end with the browser")
	}
	long := login(true)
	if time.Until(long.Expires) < 80*24*time.Hour {
		t.Fatalf("Stay signed in should keep the cookie for months, got %v", long.Expires)
	}

	_, _, remember, err := verifySessionTokenFor(long.Value, a.authSecretKey, time.Now().Add(85*24*time.Hour))
	if err != nil || !remember {
		t.Fatalf("a remembered token should still work after 85 days: %v", err)
	}
	if _, _, _, err := verifySessionTokenFor(short.Value, a.authSecretKey, time.Now().Add(15*24*time.Hour)); err == nil {
		t.Fatal("a normal token should still expire after 14 days")
	}

	// Renewing a remembered session keeps it remembered.
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: AUTH_SESSION_COOKIE_NAME, Value: long.Value})
	_, regen, _, _ := verifySessionTokenFor(long.Value, a.authSecretKey, time.Now().Add(40*24*time.Hour))
	if !regen {
		t.Fatal("a remembered token should be renewed once it's a month old")
	}
}
