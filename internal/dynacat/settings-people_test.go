package dynacat

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

const peopleTestConfig = `# Our dashboard
server:
  trusted-proxies: [10.0.0.0/24]
auth:
  proxy:
    login-url: https://auth.example.com
household:
  members:
    mom:
      name: Mom
      role: admin
      home: home
pages:
  - name: Home
    slug: home
    columns:
      - size: full
        widgets:
          - type: clock
  - name: Kids
    slug: kids
    columns:
      - size: full
        widgets:
          - type: clock
`

func newPeopleTestApp(t *testing.T) (*application, string) {
	t.Helper()
	a := newHouseholdTestApp(t)
	a.EditorEnabled = true
	a.Config.Server.AllowEditing = true
	a.Config.Server.TrustedProxies = []string{"10.0.0.0/24"}
	a.Config.Pages = []page{{Title: "Home", Slug: "home"}, {Title: "Kids", Slug: "kids"}}
	a.configPath = filepath.Join(t.TempDir(), "dynacat.yml")
	if err := os.WriteFile(a.configPath, []byte(peopleTestConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	return a, a.configPath
}

func settingsCall(a *application, h http.HandlerFunc, method, target, body, user string, pathValues map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.RemoteAddr = "10.0.0.5:1"
	if user != "" {
		r.Header.Set("Remote-User", user)
	}
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	for k, v := range pathValues {
		r.SetPathValue(k, v)
	}
	w := httptest.NewRecorder()
	a.settingsJSON(h)(w, r)
	return w
}

func readYAMLRoot(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSettingsAddPersonWithPassword(t *testing.T) {
	a, path := newPeopleTestApp(t)
	t.Setenv("DASHDEN_AUTH_SECRET", "")
	os.Unsetenv("DASHDEN_AUTH_SECRET")

	w := settingsCall(a, a.handleSettingsPersonSave, "PUT", "/x",
		`{"name":"Ava","avatar":"🦊","role":"kid","home":"kids","groups":["gamers"," gamers ",""],"password":"correct horse"}`,
		"mom", map[string]string{"username": "ava"})
	if w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}

	raw, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(raw), "# Our dashboard") {
		t.Fatal("comments in the config should survive")
	}
	root := readYAMLRoot(t, path)
	ava := root["household"].(map[string]any)["members"].(map[string]any)["ava"].(map[string]any)
	if ava["role"] != "kid" || ava["home"] != "kids" || len(ava["groups"].([]any)) != 1 {
		t.Fatalf("ava = %v", ava)
	}
	hash := root["auth"].(map[string]any)["users"].(map[string]any)["ava"].(map[string]any)["password-hash"].(string)
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("correct horse")) != nil {
		t.Fatal("the stored hash should match the password")
	}
	if strings.Contains(string(raw), "correct horse") {
		t.Fatal("the plain password must never be written")
	}
	if root["auth"].(map[string]any)["secret-key"] != "${DASHDEN_AUTH_SECRET}" || os.Getenv("DASHDEN_AUTH_SECRET") == "" {
		t.Fatal("password sign-in needs a signing key, kept in the secrets file")
	}
	if snaps, _ := listConfigSnapshots(path); len(snaps) != 1 {
		t.Fatal("the change should be in History")
	}
}

func TestSettingsPeopleRefusals(t *testing.T) {
	a, path := newPeopleTestApp(t)
	before, _ := os.ReadFile(path)

	cases := []struct {
		name, method, body, user, target string
		want                             int
	}{
		{"bad username", "PUT", `{"role":"kid"}`, "mom", "bad name!", http.StatusBadRequest},
		{"bad role", "PUT", `{"role":"boss"}`, "mom", "ava", http.StatusBadRequest},
		{"short password", "PUT", `{"role":"kid","password":"short"}`, "mom", "ava", http.StatusBadRequest},
		{"unknown field", "PUT", `{"rol":"kid"}`, "mom", "ava", http.StatusBadRequest},
		{"demote yourself", "PUT", `{"role":"adult"}`, "mom", "mom", http.StatusConflict},
		{"remove yourself", "DELETE", ``, "mom", "mom", http.StatusConflict},
		{"kid can't", "PUT", `{"role":"admin"}`, "ava", "ava", http.StatusForbidden},
		{"unknown home page", "PUT", `{"role":"kid","home":"nowhere"}`, "mom", "ava", http.StatusBadRequest},
		{"theme that isn't a preset", "PUT", `{"role":"kid","theme":"nope"}`, "mom", "ava", http.StatusUnprocessableEntity},
	}
	for _, c := range cases {
		h := a.handleSettingsPersonSave
		if c.method == "DELETE" {
			h = a.handleSettingsPersonDelete
		}
		if w := settingsCall(a, h, c.method, "/x", c.body, c.user, map[string]string{"username": c.target}); w.Code != c.want {
			t.Errorf("%s: got %d %s, want %d", c.name, w.Code, w.Body, c.want)
		}
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatalf("refused changes must not touch the config:\n%s", after)
	}
}

func TestSettingsRemovePerson(t *testing.T) {
	a, path := newPeopleTestApp(t)
	t.Setenv("DASHDEN_AUTH_SECRET", "")
	os.Unsetenv("DASHDEN_AUTH_SECRET")
	settingsCall(a, a.handleSettingsPersonSave, "PUT", "/x", `{"role":"kid","password":"correct horse"}`, "mom", map[string]string{"username": "ava"})

	if w := settingsCall(a, a.handleSettingsPersonDelete, "DELETE", "/x?password=remove", "", "mom", map[string]string{"username": "ava"}); w.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}
	root := readYAMLRoot(t, path)
	if _, ok := root["household"].(map[string]any)["members"].(map[string]any)["ava"]; ok {
		t.Fatal("ava should be gone")
	}
	if _, ok := root["auth"].(map[string]any)["users"]; ok {
		t.Fatal("an emptied users section should be removed")
	}
}

func TestSettingsSignIn(t *testing.T) {
	a, path := newPeopleTestApp(t)

	save := func(body string) *httptest.ResponseRecorder {
		return settingsCall(a, a.handleSettingsSignInSave, "PUT", "/x", body, "mom", nil)
	}

	if w := save(`{"trustedProxies":[],"proxy":{"enabled":true}}`); w.Code != http.StatusBadRequest {
		t.Fatalf("proxy without trusted proxies: %d", w.Code)
	}
	if w := save(`{"trustedProxies":["nonsense"],"proxy":{"enabled":true}}`); w.Code != http.StatusBadRequest {
		t.Fatalf("bad address: %d", w.Code)
	}
	if w := save(`{"trustedProxies":["10.0.0.0/24"],"proxy":{"enabled":true,"allowedUsers":["dad"]}}`); w.Code != http.StatusConflict {
		t.Fatalf("locking yourself out by user list: %d %s", w.Code, w.Body)
	}
	if w := save(`{"trustedProxies":["172.18.0.0/16"],"proxy":{"enabled":true}}`); w.Code != http.StatusConflict {
		t.Fatalf("dropping your own proxy address: %d %s", w.Code, w.Body)
	}
	if w := save(`{"trustedProxies":["10.0.0.0/24"],"proxy":{"enabled":false}}`); w.Code != http.StatusConflict {
		t.Fatalf("turning off the only sign-in: %d %s", w.Code, w.Body)
	}

	w := save(`{"trustedProxies":["10.0.0.0/24","172.18.0.1"],"proxy":{"enabled":true,"loginURL":"https://auth.example.com","logoutURL":"https://auth.example.com/logout","allowedGroups":["family"],"allowedUsers":["mom"]}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	root := readYAMLRoot(t, path)
	proxy := root["auth"].(map[string]any)["proxy"].(map[string]any)
	if proxy["logout-url"] != "https://auth.example.com/logout" || len(proxy["allowed-groups"].([]any)) != 1 {
		t.Fatalf("proxy = %v", proxy)
	}
	if tp := root["server"].(map[string]any)["trusted-proxies"].([]any); len(tp) != 2 {
		t.Fatalf("trusted-proxies = %v", tp)
	}
}

func TestSettingsPageAccess(t *testing.T) {
	a, path := newPeopleTestApp(t)

	list := settingsCall(a, a.handleSettingsAccessList, "GET", "/x", "", "mom", nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"title":"Kids"`) {
		t.Fatalf("list: %d %s", list.Code, list.Body)
	}

	if w := settingsCall(a, a.handleSettingsAccessSave, "PUT", "/x", `{"allowedUsers":[],"allowedGroups":["kid"]}`, "mom", map[string]string{"index": "0"}); w.Code != http.StatusConflict {
		t.Fatalf("hiding a page from yourself should be refused, got %d", w.Code)
	}
	if w := settingsCall(a, a.handleSettingsAccessSave, "PUT", "/x", `{"allowedUsers":[],"allowedGroups":["kid","adult"]}`, "mom", map[string]string{"index": "1"}); w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	pages := readYAMLRoot(t, path)["pages"].([]any)
	if g := pages[1].(map[string]any)["allowed-groups"].([]any); len(g) != 2 {
		t.Fatalf("allowed-groups = %v", g)
	}
	if w := settingsCall(a, a.handleSettingsAccessSave, "PUT", "/x", `{"allowedUsers":[],"allowedGroups":[]}`, "mom", map[string]string{"index": "1"}); w.Code != http.StatusOK {
		t.Fatalf("clear: %d", w.Code)
	}
	if _, ok := readYAMLRoot(t, path)["pages"].([]any)[1].(map[string]any)["allowed-groups"]; ok {
		t.Fatal("clearing should remove the key")
	}
	if w := settingsCall(a, a.handleSettingsAccessSave, "PUT", "/x", `{}`, "mom", map[string]string{"index": "9"}); w.Code != http.StatusNotFound {
		t.Fatalf("unknown page: %d", w.Code)
	}
}

func TestSettingsSectionInIncludedFile(t *testing.T) {
	a, path := newPeopleTestApp(t)
	dir := filepath.Dir(path)
	os.WriteFile(filepath.Join(dir, "household.yml"), []byte("members:\n  mom:\n    role: admin\n"), 0o644)
	cfg := strings.Replace(peopleTestConfig, "household:\n  members:\n    mom:\n      name: Mom\n      role: admin\n      home: home\n", "household:\n  $include: household.yml\n", 1)
	os.WriteFile(path, []byte(cfg), 0o644)

	w := settingsCall(a, a.handleSettingsPersonSave, "PUT", "/x", `{"role":"kid"}`, "mom", map[string]string{"username": "ava"})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "included file") {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
}

func TestRecoveryCode(t *testing.T) {
	a, _ := newPeopleTestApp(t)

	t.Setenv(recoveryEnvVar, "short")
	if recoveryCode() != "" {
		t.Fatal("short codes are ignored")
	}

	code := "let-me-back-in-please"
	t.Setenv(recoveryEnvVar, code)

	post := func(given string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/settings/recover", strings.NewReader("code="+given))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		a.handleRecoveryPage(w, r)
		return w
	}
	if w := post("wrong-code-entirely"); w.Code != http.StatusUnauthorized || len(w.Result().Cookies()) != 0 {
		t.Fatalf("wrong code: %d", w.Code)
	}
	w := post(code)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("right code: %d", w.Code)
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("the recovery cookie must be HttpOnly and SameSite=Strict")
	}

	// Not signed in at all, but the cookie opens Settings.
	r := httptest.NewRequest("GET", "/api/settings/people", nil)
	r.AddCookie(cookie)
	if !inRecovery(r) {
		t.Fatal("cookie should be accepted")
	}
	rec := httptest.NewRecorder()
	a.settingsJSON(a.handleSettingsPeopleList)(rec, r)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"recovery":true`) {
		t.Fatalf("recovery list: %d %s", rec.Code, rec.Body)
	}

	// A forged or expired cookie is not.
	old := time.Now().Add(-2 * time.Hour).Unix()
	for _, v := range []string{"123.abc", strings.Replace(cookie.Value, cookie.Value[:3], "999", 1), formatRecoveryCookie(code, old)} {
		r := httptest.NewRequest("GET", "/", nil)
		r.AddCookie(&http.Cookie{Name: recoveryCookieName, Value: v})
		if inRecovery(r) {
			t.Fatalf("cookie %q should be refused", v)
		}
	}

	// Changing the code ends every recovery session.
	t.Setenv(recoveryEnvVar, "a-completely-new-code")
	r = httptest.NewRequest("GET", "/", nil)
	r.AddCookie(cookie)
	if inRecovery(r) {
		t.Fatal("a new code should invalidate old cookies")
	}
}

func TestMarshalKeepsEmoji(t *testing.T) {
	var doc yaml.Node
	yaml.Unmarshal([]byte("a: 🦝\nb: 'back\\U0001F99D slash'\nc: \"x\\\\U0001F99D\"\n"), &doc)
	out, err := marshalDocument(&doc)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := yaml.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got["a"] != "🦝" || got["b"] != `back\U0001F99D slash` || got["c"] != `x\U0001F99D` {
		t.Fatalf("values changed: %q", got)
	}
	if !strings.Contains(string(out), `a: "🦝"`) {
		t.Fatalf("emoji should be written as-is:\n%s", out)
	}
}
