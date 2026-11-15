package dynacat

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedSecretsRoundTrip(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "dynacat.yml")
	v := `tri"cky\value`
	t.Cleanup(func() { os.Unsetenv("DD_TEST_TOKEN") })

	if err := setManagedSecret(configPath, "DD_TEST_TOKEN", &v); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("DD_TEST_TOKEN") != v {
		t.Fatal("secret should be applied to the running process")
	}
	info, err := os.Stat(managedSecretsPath(configPath))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("secrets file should be 0600, got %v %v", info.Mode().Perm(), err)
	}

	os.Unsetenv("DD_TEST_TOKEN")
	if err := loadManagedSecrets(configPath); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("DD_TEST_TOKEN") != v {
		t.Fatalf("reloading the file gave %q", os.Getenv("DD_TEST_TOKEN"))
	}

	if err := setManagedSecret(configPath, "DD_TEST_TOKEN", nil); err != nil {
		t.Fatal(err)
	}
	if _, set := os.LookupEnv("DD_TEST_TOKEN"); set {
		t.Fatal("removing should unset it")
	}
}

func TestSecretValidation(t *testing.T) {
	for _, bad := range [][2]string{{"1ABC", "x"}, {"A-B", "x"}, {"OK", "two\nlines"}} {
		if validateSecret(bad[0], bad[1]) == nil {
			t.Errorf("expected %q=%q to be refused", bad[0], bad[1])
		}
	}
}

func TestSecretStatuses(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "dynacat.yml")
	v := "x"
	t.Cleanup(func() { os.Unsetenv("DD_SAVED"); os.Unsetenv("DD_ENV_ONLY") })
	if err := setManagedSecret(configPath, "DD_SAVED", &v); err != nil {
		t.Fatal(err)
	}
	os.Setenv("DD_ENV_ONLY", "y")

	raw := []byte("a: ${DD_SAVED}\nb: ${env:DD_ENV_ONLY}\nc: ${DD_MISSING}\nd: ${secret:ignored}\n")
	got, err := listSecretStatuses(configPath, raw)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"DD_MISSING": "missing", "DD_SAVED": "settings", "DD_ENV_ONLY": "environment"}
	if len(got) != len(want) || got[0].Name != "DD_MISSING" {
		t.Fatalf("unexpected statuses %+v", got)
	}
	for _, s := range got {
		if want[s.Name] != s.Source || !s.UsedByConf {
			t.Errorf("%s: got %q", s.Name, s.Source)
		}
	}
}

const settingsTestConfig = "pages:\n  - name: Home\n    columns:\n      - size: full\n        widgets:\n          - type: clock\n"

func TestConfigHistorySnapshotAndRestore(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "dynacat.yml")
	if err := os.WriteFile(configPath, []byte(settingsTestConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &application{configPath: configPath}

	edited := strings.Replace(settingsTestConfig, "Home", "Den", 1)
	if err := a.writeConfigCandidates(map[string][]byte{configPath: []byte(edited)}); err != nil {
		t.Fatal(err)
	}

	snaps, err := listConfigSnapshots(configPath)
	if err != nil || len(snaps) != 1 || snaps[0].Files[0].Path != "dynacat.yml" {
		t.Fatalf("expected one snapshot of dynacat.yml, got %+v %v", snaps, err)
	}

	if err := a.restoreConfigSnapshot(snaps[0].ID); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(configPath)
	if string(data) != settingsTestConfig {
		t.Fatalf("restore gave %q", data)
	}

	// The restore is itself undoable.
	if snaps, _ := listConfigSnapshots(configPath); len(snaps) != 2 {
		t.Fatalf("expected the restore to add a snapshot, got %d", len(snaps))
	}

	// A save that fails validation leaves no snapshot behind.
	if err := a.writeConfigCandidates(map[string][]byte{configPath: []byte("pages: []\n")}); err == nil {
		t.Fatal("expected an invalid config to be refused")
	}
	if snaps, _ := listConfigSnapshots(configPath); len(snaps) != 2 {
		t.Fatalf("a refused save must not keep a snapshot, got %d", len(snaps))
	}

	if err := a.restoreConfigSnapshot("../../etc"); err == nil {
		t.Fatal("expected a bad id to be refused")
	}
}

func TestConfigHistoryPrunes(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "dynacat.yml")
	for i := 0; i < 5; i++ {
		if _, err := snapshotConfigFiles(configPath, map[string][]byte{configPath: []byte("x")}, nil); err != nil {
			t.Fatal(err)
		}
	}
	pruneConfigHistory(configPath, 2)
	if snaps, _ := listConfigSnapshots(configPath); len(snaps) != 2 {
		t.Fatalf("expected 2 snapshots after pruning, got %d", len(snaps))
	}
}

func TestSettingsPermissions(t *testing.T) {
	a := newHouseholdTestApp(t)
	a.EditorEnabled = true
	a.configPath = "/tmp/dynacat.yml"
	a.Config.Server.AllowEditing = true

	mom := a.getAuthenticatedUser(httptest.NewRecorder(), proxyRequest("10.0.0.5:1", map[string]string{"Remote-User": "mom"}))
	ava := a.getAuthenticatedUser(httptest.NewRecorder(), proxyRequest("10.0.0.5:1", map[string]string{"Remote-User": "ava"}))
	if !a.CanUseSettings(mom) || a.CanUseSettings(ava) || a.CanUseSettings(nil) {
		t.Fatal("only household admins may use Settings when there is one")
	}

	a.EditorEnabled = false
	if a.CanUseSettings(mom) {
		t.Fatal("Settings follows ENABLE_EDITOR")
	}
}

func TestSettingsRefusesCrossSiteWrites(t *testing.T) {
	a := newHouseholdTestApp(t)
	a.EditorEnabled = true
	a.configPath = "/tmp/dynacat.yml"
	called := false
	h := a.settingsJSON(func(w http.ResponseWriter, r *http.Request) { called = true })

	send := func(contentType, origin string, method ...string) int {
		r := proxyRequest("10.0.0.5:1", map[string]string{"Remote-User": "mom"})
		r.Method = "PUT"
		if len(method) > 0 {
			r.Method = method[0]
		}
		r.Host = "dash.example.com"
		if contentType != "" {
			r.Header.Set("Content-Type", contentType)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		called = false
		h(w, r)
		return w.Code
	}

	if code := send("text/plain", ""); code != http.StatusUnsupportedMediaType || called {
		t.Fatalf("a form-style post must be refused, got %d", code)
	}
	if code := send("application/json", "https://evil.example"); code != http.StatusForbidden || called {
		t.Fatalf("a cross-origin request must be refused, got %d", code)
	}
	if send("application/json", "https://dash.example.com"); !called {
		t.Fatal("a same-origin JSON request should go through")
	}
	if send("", "https://dash.example.com", "DELETE"); !called {
		t.Fatal("a same-origin DELETE has no body and should go through")
	}
	if code := send("", "https://evil.example", "DELETE"); code != http.StatusForbidden || called {
		t.Fatalf("a cross-origin DELETE must be refused, got %d", code)
	}
}

func TestSecretsLiveInConfigDotEnv(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "dynacat.yml")
	envPath := filepath.Join(dir, ".env")
	t.Cleanup(func() {
		for _, n := range []string{"DD_A", "DD_B", "DD_C", "DD_OLD"} {
			os.Unsetenv(n)
		}
	})

	original := "# Weather\nDD_A=one\n\nexport DD_B=\"two\"\n"
	os.WriteFile(envPath, []byte(original), 0o640)
	os.WriteFile(filepath.Join(dir, "dashden-secrets.env"), []byte("DD_OLD=\"legacy\"\n"), 0o600)

	// The container may have been started with an older copy of the file.
	os.Setenv("DD_A", "stale")
	if err := loadManagedSecrets(configPath); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("DD_A") != "one" || os.Getenv("DD_B") != "two" || os.Getenv("DD_OLD") != "legacy" {
		t.Fatalf("config/.env should be loaded at start-up and win: A=%q B=%q OLD=%q", os.Getenv("DD_A"), os.Getenv("DD_B"), os.Getenv("DD_OLD"))
	}

	v := "three"
	if err := setManagedSecret(configPath, "DD_B", &v); err != nil {
		t.Fatal(err)
	}
	if err := setManagedSecret(configPath, "DD_C", &v); err != nil {
		t.Fatal(err)
	}
	got := string(mustRead(t, envPath))
	want := "# Weather\nDD_A=one\n\nDD_B=\"three\"\nDD_OLD=\"legacy\"\nDD_C=\"three\"\n"
	if got != want {
		t.Fatalf("the file should be edited in place:\n%s\nwant:\n%s", got, want)
	}
	if info, _ := os.Stat(envPath); info.Mode().Perm() != 0o640 {
		t.Fatalf("permissions changed to %v", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(dir, "dashden-secrets.env")); !os.IsNotExist(err) {
		t.Fatal("the old secrets file should be folded into .env")
	}

	if err := setManagedSecret(configPath, "DD_A", nil); err != nil {
		t.Fatal(err)
	}
	if got := string(mustRead(t, envPath)); strings.Contains(got, "DD_A") || !strings.HasPrefix(got, "# Weather\n\nDD_B") {
		t.Fatalf("removing should drop just that line: %q", got)
	}
}
