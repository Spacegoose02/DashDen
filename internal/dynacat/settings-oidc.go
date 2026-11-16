package dynacat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// The secret Settings saves for the sign-in provider, in config/.env.
const oidcSecretName = "DASHDEN_OIDC_SECRET"

var oidcBaseScopes = []string{"openid", "profile", "email"}

var envOnlyReferencePattern = regexp.MustCompile(`^\$\{(?:env:)?([A-Za-z_][A-Za-z0-9_]*)\}$`)

type settingsOIDC struct {
	Enabled        bool     `json:"enabled"`
	ButtonName     string   `json:"buttonName"`
	IssuerURL      string   `json:"issuerURL"`
	ClientID       string   `json:"clientID"`
	ClientSecret   string   `json:"clientSecret"`
	SecretSaved    bool     `json:"secretSaved"`
	ExtraScopes    []string `json:"extraScopes"`
	CreateAccounts bool     `json:"createAccounts"`
	NewAccountRole string   `json:"newAccountRole"`
	ShowOnLogin    bool     `json:"showOnLogin"`
	CallbackURL    string   `json:"callbackURL"`
	Reachable      *bool    `json:"reachable,omitempty"`
}

func (a *application) registerSettingsOIDCRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings/oidc", a.settingsJSON(a.handleSettingsOIDCGet))
	mux.HandleFunc("PUT /api/settings/oidc", a.settingsJSON(a.handleSettingsOIDCSave))
	mux.HandleFunc("DELETE /api/settings/oidc", a.settingsJSON(a.handleSettingsOIDCDelete))
	mux.HandleFunc("POST /api/settings/oidc/test", a.settingsJSON(a.handleSettingsOIDCTest))
}

func (a *application) handleSettingsOIDCGet(w http.ResponseWriter, r *http.Request) {
	out := settingsOIDC{ShowOnLogin: true, NewAccountRole: householdRoleAdult, CallbackURL: a.oidcCallbackURL(r), ExtraScopes: []string{}}
	if c := a.Config.Auth.OIDC; c != nil {
		out.Enabled = true
		out.ButtonName = c.ButtonName
		out.IssuerURL = c.IssuerURL
		out.ClientID = c.ClientID
		out.SecretSaved = c.ClientSecret != ""
		for _, s := range c.Scopes {
			if !slices.Contains(oidcBaseScopes, s) {
				out.ExtraScopes = append(out.ExtraScopes, s)
			}
		}
		out.CreateAccounts = c.CreateAccounts != nil && *c.CreateAccounts
		if c.NewAccountRole != "" {
			out.NewAccountRole = c.NewAccountRole
		}
		out.ShowOnLogin = !c.HideButton
		reachable := a.ensureOIDC() == nil
		out.Reachable = &reachable
	}
	writeJSON(w, http.StatusOK, map[string]any{"oidc": out, "roles": householdRoles})
}

// checkIssuer fetches the provider's discovery document.
func checkIssuer(ctx context.Context, issuer string) error {
	u, err := url.Parse(issuer)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return fmt.Errorf("the issuer URL should look like https://auth.example.com")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(issuer, "/")+"/.well-known/openid-configuration", nil)
	if err != nil {
		return err
	}
	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("couldn't reach %s", u.Host)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %d at /.well-known/openid-configuration; is OIDC turned on there?", u.Host, resp.StatusCode)
	}
	var doc struct {
		Issuer string `json:"issuer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil || doc.Issuer == "" {
		return fmt.Errorf("%s didn't send an OIDC discovery document", u.Host)
	}
	if strings.TrimRight(doc.Issuer, "/") != strings.TrimRight(issuer, "/") {
		return fmt.Errorf("the provider calls itself %s; use exactly that as the issuer URL", doc.Issuer)
	}
	return nil
}

func (a *application) handleSettingsOIDCTest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IssuerURL string `json:"issuerURL"`
	}
	if err := decodeSettingsBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := checkIssuer(r.Context(), strings.TrimSpace(in.IssuerURL)); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Found the provider."})
}

func (a *application) handleSettingsOIDCSave(w http.ResponseWriter, r *http.Request) {
	var in settingsOIDC
	if err := decodeSettingsBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	in.ButtonName = strings.TrimSpace(in.ButtonName)
	in.IssuerURL = strings.TrimSpace(in.IssuerURL)
	in.ClientID = strings.TrimSpace(in.ClientID)
	in.ClientSecret = strings.TrimSpace(in.ClientSecret)
	extra := []string{}
	for _, s := range cleanList(in.ExtraScopes) {
		for _, f := range strings.Fields(s) {
			if !slices.Contains(oidcBaseScopes, f) && !slices.Contains(extra, f) {
				extra = append(extra, f)
			}
		}
	}
	if in.IssuerURL == "" || in.ClientID == "" {
		writeJSONError(w, http.StatusBadRequest, "The issuer URL and client ID are needed.")
		return
	}
	if in.ClientSecret == "" && (a.Config.Auth.OIDC == nil || a.Config.Auth.OIDC.ClientSecret == "") {
		writeJSONError(w, http.StatusBadRequest, "Paste the client secret from your provider.")
		return
	}
	if strings.ContainsAny(in.ClientSecret, "\r\n") {
		writeJSONError(w, http.StatusBadRequest, "The client secret must be one line.")
		return
	}
	if !slices.Contains(householdRoles, in.NewAccountRole) {
		in.NewAccountRole = householdRoleAdult
	}
	// Saving a provider that can't be reached would leave a button that only
	// shows an error, so check it first.
	if err := checkIssuer(r.Context(), in.IssuerURL); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	err := a.changeConfig(func(root *yaml.Node) error {
		oidc, err := editableSection(root, "auth", "oidc")
		if err != nil {
			return err
		}
		secretRef := scalarValue(getMappingValue(oidc, "client-secret"))
		secretName := oidcSecretName
		if m := envOnlyReferencePattern.FindStringSubmatch(secretRef); m != nil {
			secretName = m[1]
		}
		if in.ClientSecret != "" {
			if err := setManagedSecret(a.configPath, secretName, &in.ClientSecret); err != nil {
				return fmt.Errorf("could not save the client secret: %w", err)
			}
			setMappingKey(oidc, "client-secret", scalarNode("${"+secretName+"}"))
		}

		setOrRemove(oidc, "button-name", in.ButtonName)
		setMappingKey(oidc, "issuer-url", scalarNode(in.IssuerURL))
		setMappingKey(oidc, "client-id", scalarNode(in.ClientID))
		if len(extra) > 0 {
			setListOrRemove(oidc, "scopes", append(slices.Clone(oidcBaseScopes), extra...))
		} else {
			removeMappingKey(oidc, "scopes")
		}
		setMappingKey(oidc, "create-accounts", boolScalar(in.CreateAccounts))
		if in.CreateAccounts && in.NewAccountRole != householdRoleAdult {
			setMappingKey(oidc, "new-account-role", scalarNode(in.NewAccountRole))
		} else {
			removeMappingKey(oidc, "new-account-role")
		}
		if in.ShowOnLogin {
			removeMappingKey(oidc, "hide-button")
		} else {
			setMappingKey(oidc, "hide-button", boolScalar(true))
		}
		return nil
	})
	if err != nil {
		writeSettingsWriteError(w, err)
		return
	}
	slog.Info("Sign-in provider saved from Settings", "issuer", in.IssuerURL)
	writeJSON(w, http.StatusOK, map[string]any{"saved": true})
}

func (a *application) handleSettingsOIDCDelete(w http.ResponseWriter, r *http.Request) {
	if u := a.settingsUser(w, r); u != nil && !u.IsProxy && !a.PasswordEnabled && !a.ProxyAuthEnabled {
		writeSettingsWriteError(w, &settingsRefusal{"This provider is the only way to sign in, so removing it would lock everyone out. Set a password for yourself under People first."})
		return
	}
	err := a.changeConfig(func(root *yaml.Node) error {
		auth, err := editableSection(root, "auth")
		if err != nil {
			return err
		}
		removeMappingKey(auth, "oidc")
		removeMappingKey(auth, "disable-password")
		removeMappingKey(auth, "auto-redirect-oidc")
		pruneEmptySection(root, "auth")
		return nil
	})
	if err != nil {
		writeSettingsWriteError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": true})
}

func boolScalar(v bool) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: fmt.Sprint(v)}
}
