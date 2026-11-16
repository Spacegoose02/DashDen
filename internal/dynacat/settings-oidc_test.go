package dynacat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func fakeIssuer(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{
			"issuer": srv.URL, "authorization_endpoint": srv.URL + "/authorize",
			"token_endpoint": srv.URL + "/token", "jwks_uri": srv.URL + "/jwks",
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSettingsOIDCSave(t *testing.T) {
	a, path := newPeopleTestApp(t)
	issuer := fakeIssuer(t)
	t.Cleanup(func() { os.Unsetenv(oidcSecretName) })

	body := `{"buttonName":"Tinyauth","issuerURL":"` + issuer.URL + `","clientID":"abc","clientSecret":"s3cret","extraScopes":["groups"],"createAccounts":true,"newAccountRole":"kid","showOnLogin":false}`
	if w := settingsCall(a, a.handleSettingsOIDCSave, "PUT", "/api/settings/oidc", body, "mom", nil); w.Code != http.StatusOK {
		t.Fatalf("save: %d %s", w.Code, w.Body)
	}
	oidc := readYAMLRoot(t, path)["auth"].(map[string]any)["oidc"].(map[string]any)
	if oidc["client-secret"] != "${"+oidcSecretName+"}" || oidc["button-name"] != "Tinyauth" || oidc["hide-button"] != true ||
		oidc["create-accounts"] != true || oidc["new-account-role"] != "kid" || len(oidc["scopes"].([]any)) != 4 {
		t.Fatalf("oidc = %v", oidc)
	}
	if _, has := oidc["redirect-url"]; has {
		t.Fatal("the callback address should be worked out, not written")
	}
	if env := string(mustRead(t, managedSecretsPath(path))); !strings.Contains(env, oidcSecretName+`="s3cret"`) {
		t.Fatalf("the secret belongs in config/.env, got %q", env)
	}

	// An empty secret keeps the saved one.
	a.Config.Auth.OIDC = &oidcConfig{ClientSecret: "s3cret", IssuerURL: issuer.URL}
	body = `{"issuerURL":"` + issuer.URL + `","clientID":"abc","clientSecret":"","showOnLogin":true}`
	if w := settingsCall(a, a.handleSettingsOIDCSave, "PUT", "/api/settings/oidc", body, "mom", nil); w.Code != http.StatusOK {
		t.Fatalf("save keeping the secret: %d %s", w.Code, w.Body)
	}
	oidc = readYAMLRoot(t, path)["auth"].(map[string]any)["oidc"].(map[string]any)
	if oidc["client-secret"] != "${"+oidcSecretName+"}" || oidc["hide-button"] != nil || oidc["create-accounts"] != false {
		t.Fatalf("after second save: %v", oidc)
	}

	bad := `{"issuerURL":"` + issuer.URL + `/nope","clientID":"abc","clientSecret":"x","showOnLogin":true}`
	if w := settingsCall(a, a.handleSettingsOIDCSave, "PUT", "/api/settings/oidc", bad, "mom", nil); w.Code != http.StatusBadRequest {
		t.Fatalf("an issuer without a discovery document should be refused: %d", w.Code)
	}

	if w := settingsCall(a, a.handleSettingsOIDCDelete, "DELETE", "/api/settings/oidc", "", "mom", nil); w.Code != http.StatusOK {
		t.Fatalf("remove: %d %s", w.Code, w.Body)
	}
	if auth, ok := readYAMLRoot(t, path)["auth"].(map[string]any); ok && auth["oidc"] != nil {
		t.Fatal("oidc should be gone")
	}
}

func TestOIDCProfileDecision(t *testing.T) {
	a := newHouseholdTestApp(t)
	yes, no := true, false

	a.Config.Auth.OIDC = &oidcConfig{}
	if name, create, refused := a.oidcProfileDecision("stranger"); name != "stranger" || create || refused {
		t.Fatal("without create-accounts anyone gets in, as upstream")
	}
	a.Config.Auth.OIDC.CreateAccounts = &no
	if _, _, refused := a.oidcProfileDecision("stranger"); !refused {
		t.Fatal("create-accounts: false should turn away people without a profile")
	}
	if name, _, refused := a.oidcProfileDecision("MOM"); refused || name != "mom" {
		t.Fatal("people with a profile get in, under its spelling")
	}
	a.Config.Auth.OIDC.CreateAccounts = &yes
	if _, create, refused := a.oidcProfileDecision("stranger"); !create || refused {
		t.Fatal("create-accounts: true should make a profile")
	}
}

func TestOIDCButtonAndCallback(t *testing.T) {
	a := newHouseholdTestApp(t)
	a.OIDCEnabled = true
	a.Config.Auth.OIDC = &oidcConfig{ButtonName: "Tinyauth"}
	if a.OIDCButtonName() != "Sign in with Tinyauth" || !a.OIDCButtonShown() {
		t.Fatal(a.OIDCButtonName())
	}
	a.Config.Auth.OIDC.HideButton = true
	if a.OIDCButtonShown() {
		t.Fatal("hide-button should hide it")
	}
	r := httptest.NewRequest("GET", "/login", nil)
	r.Host = "dash.example.com"
	r.Header.Set("X-Forwarded-Proto", "https")
	r.RemoteAddr = "10.0.0.5:1"
	if got := a.oidcCallbackURL(r); got != "https://dash.example.com/api/oidc/callback" {
		t.Fatalf("callback = %s", got)
	}
}

func TestOIDCDownDoesNotStopStartup(t *testing.T) {
	cfg, err := newConfigFromYAML([]byte(strings.Replace(loginTestConfig, "auth:\n", "auth:\n  oidc:\n    issuer-url: http://127.0.0.1:1\n    client-id: x\n    client-secret: y\n", 1)))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(generatedSecretKeyName, "a long enough secret for tests")
	a, err := newApplication(cfg)
	if err != nil || !a.OIDCEnabled {
		t.Fatalf("an unreachable provider shouldn't stop DashDen: %v", err)
	}
}
