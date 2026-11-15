package dynacat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func clearAppEnv(t *testing.T) {
	for _, e := range appCatalog {
		for _, v := range e.vars() {
			t.Setenv(v, "")
			os.Unsetenv(v)
		}
	}
}

func TestSettingsAddApps(t *testing.T) {
	a, path := newPeopleTestApp(t)
	clearAppEnv(t)

	add := func(id, body string) *httptest.ResponseRecorder {
		return settingsCall(a, a.handleSettingsAppAdd, "POST", "/x", body, "mom", map[string]string{"id": id})
	}

	if w := add("den-duty", `{"url":"https://chores.example.com/","token":"dnk_1","page":1,"column":0}`); w.Code != http.StatusBadRequest {
		t.Fatalf("den duty without a home id: %d %s", w.Code, w.Body)
	}
	if w := add("den-duty", `{"url":"ftp://x","token":"t","homeId":"h","page":1,"column":0}`); w.Code != http.StatusBadRequest {
		t.Fatalf("bad address: %d", w.Code)
	}
	if w := add("nope", `{}`); w.Code != http.StatusNotFound {
		t.Fatalf("unknown app: %d", w.Code)
	}
	if w := add("den-duty", `{"url":"https://chores.example.com/","token":"dnk_1","homeId":"h1","page":1,"column":0}`); w.Code != http.StatusOK {
		t.Fatalf("den duty: %d %s", w.Code, w.Body)
	}
	if os.Getenv("DENDUTY_URL") != "https://chores.example.com" || os.Getenv("DENDUTY_TOKEN") != "dnk_1" {
		t.Fatal("address and token should be saved as secrets")
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "dnk_1") || !strings.Contains(string(raw), "${DENDUTY_TOKEN}") {
		t.Fatalf("the token must not be in the config:\n%s", raw)
	}

	if w := add("lootden", `{"url":"https://games.example.com","token":"lt","page":1,"column":0}`); w.Code != http.StatusOK {
		t.Fatalf("lootden: %d %s", w.Code, w.Body)
	}
	// BingeBandit's token is already set elsewhere; only the address is typed.
	t.Setenv("BINGEBANDIT_TOKEN", "from-env-file")
	w := add("bingebandit", `{"url":"https://tv.example.com","page":1,"column":0}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"source"`) {
		t.Fatalf("bingebandit should join the existing Now in the Den: %d %s", w.Code, w.Body)
	}

	widgets := readYAMLRoot(t, path)["pages"].([]any)[1].(map[string]any)["columns"].([]any)[0].(map[string]any)["widgets"].([]any)
	if len(widgets) != 3 {
		t.Fatalf("expected clock, den-app and den-now, got %v", widgets)
	}
	duty := widgets[1].(map[string]any)
	if duty["type"] != "den-app" || duty["home-id"] != "${DENDUTY_HOME_ID}" || len(duty["approve-groups"].([]any)) != 1 {
		t.Fatalf("den-app = %v", duty)
	}
	if sources := widgets[2].(map[string]any)["sources"].([]any); len(sources) != 2 {
		t.Fatalf("den-now sources = %v", sources)
	}
	if secrets, _ := os.ReadFile(managedSecretsPath(path)); strings.Contains(string(secrets), "from-env-file") {
		t.Fatal("values set elsewhere shouldn't be copied into the secrets file")
	}

	if w := add("wavelength", `{"url":"https://music.example.com","token":"w","page":7,"column":0}`); w.Code != http.StatusBadRequest {
		t.Fatalf("missing page: %d", w.Code)
	}
}

func TestSettingsAppTest(t *testing.T) {
	a, _ := newPeopleTestApp(t)
	clearAppEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/homes/h1/shopping" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	try := func(token, home string) map[string]any {
		body := `{"url":"` + srv.URL + `","token":"` + token + `","homeId":"` + home + `"}`
		w := settingsCall(a, a.handleSettingsAppTest, "POST", "/x", body, "mom", map[string]string{"id": "nighttime-ninja"})
		var out map[string]any
		json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}
	if r := try("good", "h1"); r["ok"] != true {
		t.Fatalf("good: %v", r)
	}
	if r := try("bad", "h1"); r["ok"] != false || !strings.Contains(r["message"].(string), "token") {
		t.Fatalf("bad token: %v", r)
	}
	if r := try("good", "h2"); !strings.Contains(r["message"].(string), "home id") {
		t.Fatalf("bad home: %v", r)
	}
}

func TestSettingsServer(t *testing.T) {
	a, path := newPeopleTestApp(t)
	a.Config.Server.Port = 8080

	if w := settingsCall(a, a.handleSettingsServerSave, "PUT", "/x", `{"port":0,"proxied":false,"allowEditing":true,"disablePicker":false}`, "mom", nil); w.Code != http.StatusBadRequest {
		t.Fatalf("port 0: %d", w.Code)
	}
	if w := settingsCall(a, a.handleSettingsServerSave, "PUT", "/x", `{"port":9000,"proxied":true,"allowEditing":false,"disablePicker":true}`, "mom", nil); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"portChanged":true`) {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	root := readYAMLRoot(t, path)
	server := root["server"].(map[string]any)
	if server["port"] != 9000 || server["proxied"] != true || server["allow-editing"] != false || root["theme"].(map[string]any)["disable-picker"] != true {
		t.Fatalf("server = %v theme = %v", server, root["theme"])
	}

	// With no admin, turning off editing would close Settings.
	a.Config.Household.Members = nil
	if w := settingsCall(a, a.handleSettingsServerSave, "PUT", "/x", `{"port":9000,"proxied":true,"allowEditing":false,"disablePicker":true}`, "mom", nil); w.Code != http.StatusConflict {
		t.Fatalf("should refuse to turn off editing without an admin, got %d %s", w.Code, w.Body)
	}
}
