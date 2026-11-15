package dynacat

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestUserPrefsRoundTrip(t *testing.T) {
	s := newUserPrefsStorage(filepath.Join(t.TempDir(), "test.db"))

	if _, ok, err := s.get("ava", userPrefTheme); err != nil || ok {
		t.Fatalf("expected nothing stored, got ok=%v err=%v", ok, err)
	}
	if err := s.set("ava", userPrefTheme, "forest"); err != nil {
		t.Fatal(err)
	}
	if err := s.set("ava", userPrefTheme, "night"); err != nil {
		t.Fatal(err)
	}
	if v, ok, err := s.get("ava", userPrefTheme); err != nil || !ok || v != "night" {
		t.Fatalf("got %q ok=%v err=%v", v, ok, err)
	}
}

func TestThemeFollowsThePerson(t *testing.T) {
	a := newHouseholdTestApp(t)
	night := &themeProperties{Key: "night"}
	forest := &themeProperties{Key: "forest"}
	presets, err := newOrderedYAMLMap([]string{"night", "forest"}, []*themeProperties{night, forest})
	if err != nil {
		t.Fatal(err)
	}
	a.Config.Theme.Presets = *presets
	a.Config.Household.Members["ava"].Theme = "forest"
	a.userPrefs = newUserPrefsStorage(filepath.Join(t.TempDir(), "test.db"))

	ava := a.getAuthenticatedUser(httptest.NewRecorder(), proxyRequest("10.0.0.5:1", map[string]string{"Remote-User": "ava"}))

	// Household default wins over the device's cookie.
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: "theme", Value: "night"})
	var data templateRequestData
	a.populateTemplateRequestData(&data, r, ava)
	if data.Theme != forest || !data.ThemeFromProfile {
		t.Fatalf("expected household default, got %+v", data.Theme)
	}

	// A saved choice wins over the household default.
	if err := a.userPrefs.set("ava", userPrefTheme, "night"); err != nil {
		t.Fatal(err)
	}
	data = templateRequestData{}
	a.populateTemplateRequestData(&data, httptest.NewRequest("GET", "/", nil), ava)
	if data.Theme != night {
		t.Fatalf("expected saved choice, got %+v", data.Theme)
	}

	// Anonymous visitors still use the cookie.
	data = templateRequestData{}
	a.populateTemplateRequestData(&data, r, nil)
	if data.Theme != night || data.ThemeFromProfile {
		t.Fatalf("expected cookie theme for anonymous, got %+v", data.Theme)
	}
}
