package dynacat

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSeasonalCovers(t *testing.T) {
	mk := func(from, to string) *seasonalTheme {
		s := &seasonalTheme{}
		s.from, _ = parseMonthDay(from)
		s.to, _ = parseMonthDay(to)
		return s
	}
	cases := []struct {
		from, to, day string
		want          bool
	}{
		{"10-15", "10-31", "10-15", true},
		{"10-15", "10-31", "10-31", true},
		{"10-15", "10-31", "11-01", false},
		{"12-20", "01-06", "12-31", true},
		{"12-20", "01-06", "01-03", true},
		{"12-20", "01-06", "06-01", false},
	}
	for _, c := range cases {
		d, _ := parseMonthDay(c.day)
		if got := mk(c.from, c.to).covers(d); got != c.want {
			t.Errorf("%s..%s on %s: got %v", c.from, c.to, c.day, got)
		}
	}
	for _, bad := range []string{"13-01", "02-30", "1201", "xx-yy", ""} {
		if _, err := parseMonthDay(bad); err == nil {
			t.Errorf("%q should be refused", bad)
		}
	}
	if _, err := parseMonthDay("02-29"); err != nil {
		t.Error("Feb 29 is a real date")
	}
}

const seasonalConfig = `
theme:
  presets:
    spooky:
      background-color: 270 20 8
      primary-color: 28 95 55
  seasonal:
    - preset: spooky
      from: "10-15"
      to: "10-31"
pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: clock
`

func TestSeasonalThemeApplies(t *testing.T) {
	a := newWidgetAccessApp(t, seasonalConfig)
	old := timeNow
	t.Cleanup(func() { timeNow = old })

	themeAt := func(day string, cookie string) string {
		timeNow = func() time.Time { tm, _ := time.Parse("2006-01-02", day); return tm }
		r := httptest.NewRequest("GET", "/", nil)
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: "theme", Value: cookie})
		}
		var data templateRequestData
		a.populateTemplateRequestData(&data, r, nil)
		return data.Theme.Key
	}

	if k := themeAt("2026-10-20", ""); k != "spooky" {
		t.Fatalf("in season: %q", k)
	}
	if k := themeAt("2026-11-02", ""); k != "default" {
		t.Fatalf("out of season: %q", k)
	}
	if k := themeAt("2026-10-20", "default"); k != "default" {
		t.Fatalf("an explicit choice of default should beat the season: %q", k)
	}

	bad := strings.Replace(seasonalConfig, "preset: spooky", "preset: nope", 1)
	cfg, err := newConfigFromYAML([]byte(bad))
	if err == nil {
		_, err = newApplication(cfg)
	}
	if err == nil || !strings.Contains(err.Error(), "not a theme preset") {
		t.Fatalf("unknown seasonal preset: %v", err)
	}
}

func TestSettingsThemePackAndSeasons(t *testing.T) {
	a, path := newPeopleTestApp(t)

	w := settingsCall(a, a.handleSettingsThemePackAdd, "POST", "/x", `{"keys":["campfire","spooky","not-in-pack"]}`, "mom", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"added":["campfire","spooky"]`) {
		t.Fatalf("pack add: %d %s", w.Code, w.Body)
	}
	presets := readYAMLRoot(t, path)["theme"].(map[string]any)["presets"].(map[string]any)
	if presets["campfire"].(map[string]any)["primary-color"] != "28 90 60" {
		t.Fatalf("campfire = %v", presets["campfire"])
	}
	// Adding again leaves the existing one alone.
	w = settingsCall(a, a.handleSettingsThemePackAdd, "POST", "/x", `{"keys":["campfire"]}`, "mom", nil)
	if !strings.Contains(w.Body.String(), `"added":[]`) {
		t.Fatalf("re-add: %s", w.Body)
	}

	// Seasons are checked against the running config's presets.
	if w := settingsCall(a, a.handleSettingsSeasonalSave, "PUT", "/x", `{"seasonal":[{"preset":"spooky","from":"10-15","to":"10-31"}]}`, "mom", nil); w.Code != http.StatusBadRequest {
		t.Fatalf("preset not loaded yet: %d", w.Code)
	}
	cfg, err := newConfigFromYAML(mustRead(t, path))
	if err != nil {
		t.Fatal(err)
	}
	a.Config.Theme.Presets = cfg.Theme.Presets
	if w := settingsCall(a, a.handleSettingsSeasonalSave, "PUT", "/x", `{"seasonal":[{"preset":"spooky","from":"10-32","to":"10-31"}]}`, "mom", nil); w.Code != http.StatusBadRequest {
		t.Fatalf("bad date: %d", w.Code)
	}
	if w := settingsCall(a, a.handleSettingsSeasonalSave, "PUT", "/x", `{"seasonal":[{"preset":"spooky","from":"10-15","to":"10-31"}]}`, "mom", nil); w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	raw := string(mustRead(t, path))
	if !strings.Contains(raw, `from: "10-15"`) {
		t.Fatalf("dates should be quoted so YAML keeps them as text:\n%s", raw)
	}
	cfg, err = newConfigFromYAML([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newApplication(cfg); err != nil {
		t.Fatalf("the saved config should load: %v", err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

