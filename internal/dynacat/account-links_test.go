package dynacat

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestLinkedAccountSignsInAsMember(t *testing.T) {
	a := newHouseholdTestApp(t)
	a.Config.Household.Members["mom"].LinkedAccounts = []string{"MomTiny", "mom@example.com"}

	u := a.getAuthenticatedUser(httptest.NewRecorder(), proxyRequest("10.0.0.5:1", map[string]string{"Remote-User": "momtiny"}))
	if u == nil || u.Username != "mom" || !u.IsAdmin() || u.SignInName != "momtiny" || u.LinkedBy != "MomTiny" {
		t.Fatalf("a linked Tinyauth name should sign in as mom, got %+v", u)
	}

	u = a.getAuthenticatedUser(httptest.NewRecorder(), proxyRequest("10.0.0.5:1", map[string]string{"Remote-User": "someone", "Remote-Email": "Mom@Example.com"}))
	if u == nil || u.Username != "mom" {
		t.Fatalf("a linked email should sign in as mom, got %+v", u)
	}

	u = a.getAuthenticatedUser(httptest.NewRecorder(), proxyRequest("10.0.0.5:1", map[string]string{"Remote-User": "ava"}))
	if u.Username != "ava" || u.LinkedBy != "" {
		t.Fatalf("a direct match is unchanged, got %+v", u)
	}
}

func TestUnlinkedSignInsAreRemembered(t *testing.T) {
	a := newHouseholdTestApp(t)
	recentUnlinkedSignIns = &unlinkedSignInStore{}
	a.getAuthenticatedUser(httptest.NewRecorder(), proxyRequest("10.0.0.5:1", map[string]string{"Remote-User": "newbie", "Remote-Email": "n@example.com"}))
	list := recentUnlinkedSignIns.list(a)
	if len(list) != 1 || list[0].Username != "newbie" || list[0].Email != "n@example.com" {
		t.Fatalf("unlinked = %+v", list)
	}
	a.Config.Household.Members["ava"].LinkedAccounts = []string{"newbie"}
	if list := recentUnlinkedSignIns.list(a); len(list) != 0 {
		t.Fatalf("once linked it should drop off, got %+v", list)
	}
}

func TestValidateLinkedAccounts(t *testing.T) {
	cases := map[string]map[string]*householdMember{
		"linked twice":      {"a": {LinkedAccounts: []string{"x"}}, "b": {LinkedAccounts: []string{"X"}}},
		"linked to self":    {"a": {LinkedAccounts: []string{"A"}}},
		"has a profile too": {"a": {LinkedAccounts: []string{"b"}}, "b": {}},
		"empty":             {"a": {LinkedAccounts: []string{" "}}},
	}
	for name, members := range cases {
		cfg := &config{}
		cfg.Household.Members = members
		if validateHousehold(cfg) == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	cfg := &config{}
	cfg.Household.Members = map[string]*householdMember{"a": {LinkedAccounts: []string{"x", "a@example.com"}}, "b": {LinkedAccounts: []string{"y"}}}
	if err := validateHousehold(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestSettingsLinkAccounts(t *testing.T) {
	a, path := newPeopleTestApp(t)
	a.Config.Household.Members["mom"].LinkedAccounts = []string{"mom-tiny"}

	// Linking a name with its own profile asks first, then merges it.
	w := settingsCall(a, a.handleSettingsPersonSave, "PUT", "/x", `{"name":"Mom","role":"admin","linkedAccounts":["mom-tiny","ava"]}`, "mom", map[string]string{"username": "mom"})
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `"replaceProfiles":["ava"]`) {
		t.Fatalf("expected a question about ava's profile, got %d %s", w.Code, w.Body)
	}
	w = settingsCall(a, a.handleSettingsPersonSave, "PUT", "/x", `{"name":"Mom","role":"admin","linkedAccounts":["mom-tiny","ava"],"replaceProfiles":true}`, "mom", map[string]string{"username": "mom"})
	if w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	members := readYAMLRoot(t, path)["household"].(map[string]any)["members"].(map[string]any)
	if _, ok := members["ava"]; ok {
		t.Fatal("ava's own profile should be gone")
	}
	if links := members["mom"].(map[string]any)["linked-accounts"].([]any); len(links) != 2 || links[1] != "ava" {
		t.Fatalf("links = %v", links)
	}

	// Leaving linkedAccounts out keeps them.
	a2, path2 := newPeopleTestApp(t)
	os.WriteFile(path2, []byte(strings.Replace(peopleTestConfig, "      home: home\n", "      home: home\n      linked-accounts: [mom-tiny]\n", 1)), 0o644)
	if w := settingsCall(a2, a2.handleSettingsPersonSave, "PUT", "/x", `{"role":"admin"}`, "mom", map[string]string{"username": "mom"}); w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	if !strings.Contains(readFile(t, path2), "mom-tiny") {
		t.Fatal("linked accounts should survive a save that doesn't mention them")
	}
}

func TestSettingsLinkRefusals(t *testing.T) {
	a, path := newPeopleTestApp(t)
	a.Config.Household.Members["ava"].LinkedAccounts = []string{"ava-tiny"}
	a.Config.Household.Members["mom"].LinkedAccounts = []string{"mom-tiny"}
	before := readFile(t, path)

	cases := []struct {
		name, body, user, target string
		want                     int
	}{
		{"already linked elsewhere", `{"role":"admin","linkedAccounts":["ava-tiny"]}`, "mom", "mom", http.StatusConflict},
		{"own username", `{"role":"admin","linkedAccounts":["mom"]}`, "mom", "mom", http.StatusBadRequest},
		{"bad name", `{"role":"admin","linkedAccounts":["two words"]}`, "mom", "mom", http.StatusBadRequest},
		{"merge yourself away", `{"role":"kid","linkedAccounts":["mom"],"replaceProfiles":true}`, "mom", "ava", http.StatusConflict},
		{"unlink the sign-in in use", `{"role":"admin","linkedAccounts":[]}`, "mom-tiny", "mom", http.StatusConflict},
	}
	for _, c := range cases {
		if w := settingsCall(a, a.handleSettingsPersonSave, "PUT", "/x", c.body, c.user, map[string]string{"username": c.target}); w.Code != c.want {
			t.Errorf("%s: got %d %s, want %d", c.name, w.Code, w.Body, c.want)
		}
	}
	if readFile(t, path) != before {
		t.Fatal("refused changes must not touch the config")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func accountPost(a *application, h http.HandlerFunc, user string, form url.Values, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "http://dash.example.com/account/link", strings.NewReader(form.Encode()))
	r.RemoteAddr = "10.0.0.5:1"
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Remote-User", user)
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h(w, r)
	return w
}

func TestAccountSelfLink(t *testing.T) {
	a, path := newPeopleTestApp(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct horse"), bcrypt.MinCost)
	a.Config.Auth.Users = map[string]*user{"Ava": {PasswordHash: hash}, "tiny-ava": {PasswordHash: hash}}
	a.PasswordEnabled = true
	a.failedAuthAttempts = map[string]*failedAuthAttempt{}
	good := url.Values{"username": {"ava"}, "password": {"correct horse"}}

	if w := accountPost(a, a.handleAccountLink, "ava-tiny", good, "https://evil.example.com"); w.Code != http.StatusForbidden {
		t.Fatalf("cross-site post: %d", w.Code)
	}
	if w := accountPost(a, a.handleAccountLink, "ava-tiny", good, ""); w.Code != http.StatusForbidden {
		t.Fatalf("post without an origin: %d", w.Code)
	}
	if w := accountPost(a, a.handleAccountLink, "ava-tiny", url.Values{"username": {"ava"}, "password": {"wrong"}}, "http://dash.example.com"); w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", w.Code)
	}
	if w := accountPost(a, a.handleAccountLink, "mom", good, "http://dash.example.com"); w.Code != http.StatusConflict {
		t.Fatalf("someone with a profile has nothing to link: %d", w.Code)
	}
	// A Tinyauth name that is also another local account is left to admins.
	if w := accountPost(a, a.handleAccountLink, "tiny-ava", good, "http://dash.example.com"); w.Code != http.StatusConflict {
		t.Fatalf("name clash: %d", w.Code)
	}

	w := accountPost(a, a.handleAccountLink, "ava-tiny", good, "http://dash.example.com")
	if w.Code != http.StatusSeeOther {
		t.Fatalf("link: %d %s", w.Code, w.Body)
	}
	members := readYAMLRoot(t, path)["household"].(map[string]any)["members"].(map[string]any)
	ava, ok := members["ava"].(map[string]any)
	if !ok {
		t.Fatalf("ava's profile should hold the link, got %v", members)
	}
	if links := ava["linked-accounts"].([]any); len(links) != 1 || links[0] != "ava-tiny" {
		t.Fatalf("links = %v", links)
	}
}

func TestAccountUnlink(t *testing.T) {
	a, path := newPeopleTestApp(t)
	os.WriteFile(path, []byte(strings.Replace(peopleTestConfig, "      home: home\n", "      home: home\n      linked-accounts: [mom-tiny, mom-old]\n", 1)), 0o644)
	a.Config.Household.Members["mom"].LinkedAccounts = []string{"mom-tiny", "mom-old"}
	origin := "http://dash.example.com"

	if w := accountPost(a, a.handleAccountUnlink, "mom-tiny", url.Values{"account": {"mom-tiny"}}, origin); w.Code != http.StatusConflict {
		t.Fatalf("unlinking the sign-in in use: %d", w.Code)
	}
	if w := accountPost(a, a.handleAccountUnlink, "mom-tiny", url.Values{"account": {"ava"}}, origin); w.Code != http.StatusNotFound {
		t.Fatalf("unlinking someone else: %d", w.Code)
	}
	if w := accountPost(a, a.handleAccountUnlink, "mom-tiny", url.Values{"account": {"MOM-OLD"}}, origin); w.Code != http.StatusSeeOther {
		t.Fatalf("unlink: %d %s", w.Code, w.Body)
	}
	if s := readFile(t, path); strings.Contains(s, "mom-old") || !strings.Contains(s, "mom-tiny") {
		t.Fatalf("config after unlink:\n%s", s)
	}
}

func TestAccountPageRenders(t *testing.T) {
	a, _ := newPeopleTestApp(t)
	a.Config.Household.Members["mom"].LinkedAccounts = []string{"mom-tiny"}
	a.PasswordEnabled = true
	for _, who := range []string{"mom-tiny", "mom", "stranger"} {
		r := proxyRequest("10.0.0.5:1", map[string]string{"Remote-User": who})
		w := httptest.NewRecorder()
		a.handleAccountPage(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", who, w.Code, w.Body)
		}
		body := w.Body.String()
		switch who {
		case "mom-tiny":
			if !strings.Contains(body, "using this one now") {
				t.Errorf("mom-tiny should see which link is in use: %s", body)
			}
		case "stranger":
			if !strings.Contains(body, "Link accounts") {
				t.Errorf("an unlinked sign-in should be offered the link form")
			}
		}
	}
}
