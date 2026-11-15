package dynacat

import (
	"net/http/httptest"
	"testing"
)

func newHouseholdTestApp(t *testing.T) *application {
	t.Helper()
	a := newProxyTestApp(t, &proxyAuthConfig{}, "10.0.0.0/24")
	a.Config.Household.Members = map[string]*householdMember{
		"mom": {Name: "Mom", Role: householdRoleAdmin, Home: "mom", Avatar: "🦝"},
		"ava": {Name: "Ava", Role: householdRoleKid, Home: "ava", Groups: []string{"gamers"}},
	}
	a.Config.Pages = []page{{Title: "Home", Slug: "home"}, {Title: "Mom", Slug: "mom"}, {Title: "Ava", Slug: "ava", AllowedGroups: []string{"kid"}}, {Title: "Grown-ups", Slug: "grown-ups", AllowedGroups: []string{"adult"}}}
	a.slugToPage = map[string]*page{"": &a.Config.Pages[0]}
	for i := range a.Config.Pages {
		a.slugToPage[a.Config.Pages[i].Slug] = &a.Config.Pages[i]
	}
	return a
}

func TestHouseholdAddsNameRoleAndGroups(t *testing.T) {
	a := newHouseholdTestApp(t)

	u := a.getAuthenticatedUser(httptest.NewRecorder(), proxyRequest("10.0.0.5:1", map[string]string{"Remote-User": "ava", "Remote-Groups": "family"}))
	if u == nil || u.DisplayName() != "Ava" || u.Member() == nil {
		t.Fatalf("expected Ava's profile, got %+v", u)
	}
	want := []string{"family", "kid", "gamers"}
	if len(u.Groups) != len(want) {
		t.Fatalf("groups = %v, want %v", u.Groups, want)
	}
	for i := range want {
		if u.Groups[i] != want[i] {
			t.Fatalf("groups = %v, want %v", u.Groups, want)
		}
	}
	if u.IsAdmin() {
		t.Fatal("a kid is not an admin")
	}
}

func TestHouseholdAdminCountsAsAdult(t *testing.T) {
	a := newHouseholdTestApp(t)
	u := a.getAuthenticatedUser(httptest.NewRecorder(), proxyRequest("10.0.0.5:1", map[string]string{"Remote-User": "mom"}))
	if !u.IsAdmin() || !a.canUserAccessPage(u, a.slugToPage["grown-ups"]) {
		t.Fatalf("admin should be an adult too, groups %v", u.Groups)
	}
	kid := a.getAuthenticatedUser(httptest.NewRecorder(), proxyRequest("10.0.0.5:1", map[string]string{"Remote-User": "ava"}))
	if a.canUserAccessPage(kid, a.slugToPage["grown-ups"]) {
		t.Fatal("a kid must not see the grown-ups page")
	}
}

func TestUnknownUserIsUnchanged(t *testing.T) {
	a := newHouseholdTestApp(t)
	u := a.getAuthenticatedUser(httptest.NewRecorder(), proxyRequest("10.0.0.5:1", map[string]string{"Remote-User": "visitor"}))
	if u == nil || u.Member() != nil || u.DisplayName() != "visitor" || len(u.Groups) != 0 {
		t.Fatalf("unexpected %+v", u)
	}
}

func TestRootRedirectsToHomePage(t *testing.T) {
	a := newHouseholdTestApp(t)
	u := a.getAuthenticatedUser(httptest.NewRecorder(), proxyRequest("10.0.0.5:1", map[string]string{"Remote-User": "ava"}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/?x=1", nil)
	if !a.redirectToHomePage(w, r, u) || w.Header().Get("Location") != "/ava?x=1" {
		t.Fatalf("expected redirect to /ava?x=1, got %q", w.Header().Get("Location"))
	}

	// Asking for a page by name never redirects.
	r = httptest.NewRequest("GET", "/home", nil)
	r.SetPathValue("page", "home")
	if a.redirectToHomePage(httptest.NewRecorder(), r, u) {
		t.Fatal("explicit pages must not redirect")
	}

	if a.redirectToHomePage(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil), nil) {
		t.Fatal("anonymous visitors stay on the first page")
	}
}

func TestHouseholdValidation(t *testing.T) {
	cfg := &config{}
	cfg.Household.Members = map[string]*householdMember{"sam": {Role: "boss"}}
	if err := validateHousehold(cfg); err == nil {
		t.Fatal("expected an error for an unknown role")
	}
	cfg.Household.Members = map[string]*householdMember{"sam": {Role: "adult", Theme: "nope"}}
	if err := validateHousehold(cfg); err == nil {
		t.Fatal("expected an error for an unknown theme")
	}

	a := newHouseholdTestApp(t)
	a.Config.Household.Members["ava"].Home = "missing"
	if err := a.checkHouseholdHomePages(); err == nil {
		t.Fatal("expected an error for an unknown home page")
	}
}
