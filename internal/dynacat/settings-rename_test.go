package dynacat

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const renameTestConfig = `# Our dashboard
server:
  trusted-proxies: [10.0.0.0/24]
  editing-users: [mom, Mrsunderwod02]
auth:
  proxy:
    login-url: https://auth.example.com
  users:
    mrsunderwod02:
      password-hash: $2a$10$abcdefghijklmnopqrstuuJ2aYQ7b0tq8nZf0P5fJ0rHq8T6pY3aW
household:
  members:
    mom:
      name: Mom
      role: admin
    mrsunderwod02:
      name: Mrs U
      role: adult
pages:
  - name: Home
    slug: home
    allowed-users: [mom, mrsunderwod02]
    columns:
      - size: full
        widgets:
          - type: clock
  - $include: extra.yml
`

const renameTestInclude = `- name: Extra
  slug: extra
  columns:
    - size: full
      widgets:
        - type: clock
          allowed-users: [MRSUNDERWOD02]
        - type: notice-board
          id: fridge
          post-groups: [adult, mrsunderwod02]
`

func newRenameTestApp(t *testing.T) (*application, string, string) {
	t.Helper()
	a, path := newPeopleTestApp(t)
	dir := filepath.Dir(path)
	if err := os.WriteFile(path, []byte(renameTestConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	include := filepath.Join(dir, "extra.yml")
	if err := os.WriteFile(include, []byte(renameTestInclude), 0o644); err != nil {
		t.Fatal(err)
	}
	a.Config.Household.Members["mrsunderwod02"] = &householdMember{Name: "Mrs U", Role: householdRoleAdult}
	a.Config.Auth.Users = map[string]*user{"mrsunderwod02": {PasswordHashString: "x"}}
	a.Config.Server.DBPath = filepath.Join(dir, "dashden.db")
	return a, path, include
}

func TestRenamePersonEverywhere(t *testing.T) {
	a, path, include := newRenameTestApp(t)

	prefs := newUserPrefsStorage(a.Config.Server.DBPath)
	if err := prefs.set("mrsunderwod02", userPrefTheme, "den-night"); err != nil {
		t.Fatal(err)
	}
	notes := newNoticeStorage(a.Config.Server.DBPath)
	if err := notes.add(notice{Board: "fridge", Username: "mrsunderwod02", Author: "Mrs U", Text: "milk", Created: time.Now()}); err != nil {
		t.Fatal(err)
	}
	todos := newTodoStorage(a.Config.Server.DBPath)
	todos.saveTasks("chores\x1fmrsunderwod02", []todoTask{{Text: "laundry"}})
	todos.saveTasks("chores\x1fmom", []todoTask{{Text: "dishes"}})

	w := settingsCall(a, a.handleSettingsPersonRename, "POST", "/x", `{"newUsername":"mrsunderwood02"}`, "mom", map[string]string{"username": "MrsUnderwod02"})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"dataCarried":true`) {
		t.Fatalf("rename: %d %s", w.Code, w.Body)
	}

	raw, _ := os.ReadFile(path)
	cfg := string(raw)
	if strings.Contains(strings.ToLower(cfg), "mrsunderwod02") {
		t.Fatalf("the old name should be gone from the config:\n%s", cfg)
	}
	if strings.Count(cfg, "mrsunderwood02") != 4 || !strings.HasPrefix(cfg, "# Our dashboard") {
		t.Fatalf("the new name should replace the old one in members, users, editing-users and allowed-users:\n%s", cfg)
	}
	inc, _ := os.ReadFile(include)
	if strings.Contains(strings.ToLower(string(inc)), "mrsunderwod02") || strings.Count(string(inc), "mrsunderwood02") != 2 {
		t.Fatalf("the included file should be renamed too:\n%s", inc)
	}

	if v, ok, _ := prefs.get("mrsunderwood02", userPrefTheme); !ok || v != "den-night" {
		t.Fatal("the saved theme should follow the new name")
	}
	if list, _ := notes.list("fridge", time.Hour, 10); len(list) != 1 || list[0].Username != "mrsunderwood02" {
		t.Fatalf("notes should follow the new name: %+v", list)
	}
	if tasks, _ := todos.loadTasks("chores\x1fmrsunderwood02"); len(tasks) != 1 || tasks[0].Text != "laundry" {
		t.Fatalf("the to-do list should follow the new name: %+v", tasks)
	}
	if tasks, _ := todos.loadTasks("chores\x1fmom"); len(tasks) != 1 {
		t.Fatal("other people's lists stay put")
	}
	if snaps, _ := listConfigSnapshots(path); len(snaps) != 1 {
		t.Fatal("the rename should be in History")
	}
}

func TestRenamePersonRefusals(t *testing.T) {
	a, path, _ := newRenameTestApp(t)
	before, _ := os.ReadFile(path)

	cases := []struct {
		name, body, user, target string
		want                     int
	}{
		{"taken", `{"newUsername":"MOM"}`, "mom", "mrsunderwod02", http.StatusConflict},
		{"bad name", `{"newUsername":"no spaces"}`, "mom", "mrsunderwod02", http.StatusBadRequest},
		{"no one", `{"newUsername":"someone"}`, "mom", "ghost", http.StatusNotFound},
		{"not an admin", `{"newUsername":"someone"}`, "ava", "mrsunderwod02", http.StatusForbidden},
	}
	for _, c := range cases {
		if w := settingsCall(a, a.handleSettingsPersonRename, "POST", "/x", c.body, c.user, map[string]string{"username": c.target}); w.Code != c.want {
			t.Errorf("%s: got %d %s, want %d", c.name, w.Code, w.Body, c.want)
		}
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("refused renames must not touch the config")
	}
}

func TestRenameOnlyCapitals(t *testing.T) {
	a, path, _ := newRenameTestApp(t)
	w := settingsCall(a, a.handleSettingsPersonRename, "POST", "/x", `{"newUsername":"MrsUnderwod02"}`, "mom", map[string]string{"username": "mrsunderwod02"})
	if w.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", w.Code, w.Body)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "mrsunderwod02") || !strings.Contains(string(raw), "editing-users: [mom, MrsUnderwod02]") {
		t.Fatalf("capitals should change everywhere without duplicates:\n%s", raw)
	}
}

func TestRenameYourselfKeepsYouSignedIn(t *testing.T) {
	secret, err := makeAuthSecretKey(AUTH_SECRET_KEY_LENGTH)
	if err != nil {
		t.Fatal(err)
	}
	a := &application{}
	a.authSecretKey, err = resolveAuthSecretKey(secret)
	if err != nil {
		t.Fatal(err)
	}
	u := &user{credential: "$2a$10$hash"}
	oldHash, _ := computeUsernameHash("mrsunderwod02", u.credential, a.authSecretKey)
	oldToken, _ := generateSessionTokenFor(oldHash, a.authSecretKey, time.Now(), true)

	r := httptest.NewRequest("POST", "/x", nil)
	r.AddCookie(&http.Cookie{Name: AUTH_SESSION_COOKIE_NAME, Value: oldToken})
	w := httptest.NewRecorder()
	a.reissueSessionAfterRename(w, r, "mrsunderwood02", u)

	var token string
	for _, c := range w.Result().Cookies() {
		if c.Name == AUTH_SESSION_COOKIE_NAME {
			token = c.Value
		}
	}
	hash, _, remember, err := verifySessionTokenFor(token, a.authSecretKey, time.Now())
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	want, _ := computeUsernameHash("mrsunderwood02", u.credential, a.authSecretKey)
	if string(hash) != string(want) || !remember {
		t.Fatal("the new session should be for the new name and keep Stay signed in")
	}
}

func TestRenameMovesProviderSessions(t *testing.T) {
	s := newSessionStore()
	s.set("a", &oidcSession{Username: "MrsUnderwod02", CreatedAt: time.Now()})
	s.set("b", &oidcSession{Username: "mom", CreatedAt: time.Now()})
	s.renameUser("mrsunderwod02", "mrsunderwood02")
	if sess, _ := s.get("a"); sess.Username != "mrsunderwood02" {
		t.Fatal("the session should follow the new name")
	}
	if sess, _ := s.get("b"); sess.Username != "mom" {
		t.Fatal("other sessions stay put")
	}
}
