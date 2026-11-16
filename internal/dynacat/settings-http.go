package dynacat

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"time"
)

var settingsPageTemplate = mustParseTemplate("settings.html", "document.html", "footer.html")

// Set by serveApp; reloads the config as if a config file had changed.
var requestConfigReload = func() {}

// CanUseSettings decides who may open Settings. When the household has at
// least one admin, only admins may; otherwise the editor's rules apply.
func (a *application) CanUseSettings(user *authenticatedUser) bool {
	if !a.EditorEnabled || a.configPath == "" {
		return false
	}

	for _, m := range a.Config.Household.Members {
		if m.Role == householdRoleAdmin {
			return user.IsAdmin()
		}
	}

	return a.UserAllowedToEdit(user) && a.Config.Server.AllowEditing
}

func (a *application) registerSettingsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /settings", a.handleSettingsPage)
	if recoveryCode() != "" {
		mux.HandleFunc("GET /settings/recover", a.handleRecoveryPage)
		mux.HandleFunc("POST /settings/recover", a.handleRecoveryPage)
	}
	a.registerSettingsPeopleRoutes(mux)
	a.registerSettingsOIDCRoutes(mux)
	a.registerSettingsAppsRoutes(mux)
	a.registerSettingsThemesRoutes(mux)
	mux.HandleFunc("GET /api/settings/overview", a.settingsJSON(a.handleSettingsOverview))
	mux.HandleFunc("GET /api/settings/secrets", a.settingsJSON(a.handleSettingsSecretsList))
	mux.HandleFunc("PUT /api/settings/secrets/{name}", a.settingsJSON(a.handleSettingsSecretSet))
	mux.HandleFunc("DELETE /api/settings/secrets/{name}", a.settingsJSON(a.handleSettingsSecretDelete))
	mux.HandleFunc("GET /api/settings/history", a.settingsJSON(a.handleSettingsHistoryList))
	mux.HandleFunc("POST /api/settings/history/{id}/restore", a.settingsJSON(a.handleSettingsHistoryRestore))
}

// settingsJSON checks sign-in, permission and, for changes, that the request
// is a same-origin JSON call, which a cross-site form cannot make.
func (a *application) settingsJSON(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.configPath == "" {
			writeJSONError(w, http.StatusNotFound, "Settings need a config file.")
			return
		}
		if !inRecovery(r) {
			if a.handleUnauthorizedResponse(w, r, showUnauthorizedJSON) {
				return
			}
			if !a.CanUseSettings(a.getAuthenticatedUser(w, r)) {
				writeJSONError(w, http.StatusForbidden, "Only household admins can change settings.")
				return
			}
		}
		if r.Method != http.MethodGet {
			// A cross-site form can only POST, so DELETE needs no body or JSON type.
			if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" && r.Method != http.MethodDelete {
				writeJSONError(w, http.StatusUnsupportedMediaType, "expected application/json")
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				if u, err := url.Parse(origin); err != nil || u.Host != r.Host {
					writeJSONError(w, http.StatusForbidden, "cross-origin request refused")
					return
				}
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		next(w, r)
	}
}

func (a *application) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	recovery := inRecovery(r)
	var user *authenticatedUser
	if !recovery {
		if a.handleUnauthorizedResponse(w, r, redirectToLogin) {
			return
		}
		user = a.getAuthenticatedUser(w, r)
		if !a.CanUseSettings(user) {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("Only household admins can open Settings."))
			return
		}
	} else if a.configPath == "" {
		a.handleNotFound(w, r)
		return
	}

	data := settingsPageData{
		templateData: templateData{App: a, AuthUser: user, AccessiblePages: a.getAccessiblePages(user)},
		Recovery:     recovery,
	}
	a.populateTemplateRequestData(&data.Request, r, user)

	var buf bytes.Buffer
	if err := settingsPageTemplate.Execute(&buf, data); err != nil {
		slog.Error("Rendering settings page failed", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Write(buf.Bytes())
}

type settingsPageData struct {
	templateData
	Recovery bool
}

type settingsOverview struct {
	Version       string        `json:"version"`
	SignIn        []string      `json:"signIn"`
	Members       int           `json:"members"`
	Admins        []string      `json:"admins"`
	Pages         int           `json:"pages"`
	ConfigPath    string        `json:"configPath"`
	SecretsFile   string        `json:"secretsFile"`
	MissingSecret int           `json:"missingSecrets"`
	Snapshots     int           `json:"snapshots"`
	Setup         settingsSetup `json:"setup"`
	ConfigError   string        `json:"configError,omitempty"`
}

// settingsSetup drives the getting-started steps on the Overview tab.
type settingsSetup struct {
	You          string `json:"you"`
	YouAreMember bool   `json:"youAreMember"`
	HasAdmin     bool   `json:"hasAdmin"`
	SignIn       bool   `json:"signIn"`
	AppsAdded    int    `json:"appsAdded"`
}

func (a *application) handleSettingsOverview(w http.ResponseWriter, r *http.Request) {
	o := settingsOverview{
		Version:     buildVersion,
		Members:     len(a.Config.Household.Members),
		Pages:       len(a.Config.Pages),
		ConfigPath:  a.configPath,
		SecretsFile: managedSecretsPath(a.configPath),
		SignIn:      []string{},
		Admins:      []string{},
	}
	if a.ProxyAuthEnabled {
		o.SignIn = append(o.SignIn, "Forward-auth proxy")
	}
	if a.OIDCEnabled {
		o.SignIn = append(o.SignIn, "OIDC")
	}
	if a.PasswordEnabled {
		o.SignIn = append(o.SignIn, "Password")
	}
	for name, m := range a.Config.Household.Members {
		if m.Role == householdRoleAdmin {
			o.Admins = append(o.Admins, name)
		}
	}
	if raw, _, err := parseYAMLIncludes(a.configPath); err == nil {
		if secrets, err := listSecretStatuses(a.configPath, raw); err == nil {
			for _, s := range secrets {
				if s.Source == "missing" {
					o.MissingSecret++
				}
			}
		}
	}
	if snaps, err := listConfigSnapshots(a.configPath); err == nil {
		o.Snapshots = len(snaps)
	}
	if err := getLastConfigError(); err != nil {
		o.ConfigError = err.Error()
	}
	o.Setup = settingsSetup{HasAdmin: a.hasHouseholdAdmin(), SignIn: a.AnyAuthEnabled(), AppsAdded: len(a.appsInUse())}
	if u := a.settingsUser(w, r); u != nil {
		o.Setup.You = u.Username
		o.Setup.YouAreMember = u.Member() != nil
	}
	writeJSON(w, http.StatusOK, o)
}

func (a *application) handleSettingsSecretsList(w http.ResponseWriter, r *http.Request) {
	raw, _, err := parseYAMLIncludes(a.configPath)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not read the config")
		return
	}
	secrets, err := listSecretStatuses(a.configPath, raw)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not read the secrets file")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"secrets": secrets, "file": managedSecretsPath(a.configPath)})
}

func (a *application) handleSettingsSecretSet(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	name := r.PathValue("name")
	if err := validateSecret(name, body.Value); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := setManagedSecret(a.configPath, name, &body.Value); err != nil {
		slog.Error("Saving secret failed", "name", name, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "could not save the secret: "+err.Error())
		return
	}
	slog.Info("Secret saved from Settings", "name", name)
	writeJSON(w, http.StatusOK, map[string]any{"saved": name, "reloading": true})
	reloadSoon()
}

func (a *application) handleSettingsSecretDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !secretNamePattern.MatchString(name) {
		writeJSONError(w, http.StatusBadRequest, "invalid name")
		return
	}
	if err := setManagedSecret(a.configPath, name, nil); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not remove the secret: "+err.Error())
		return
	}
	slog.Info("Secret removed from Settings", "name", name)
	writeJSON(w, http.StatusOK, map[string]any{"removed": name, "reloading": true})
	reloadSoon()
}

func (a *application) handleSettingsHistoryList(w http.ResponseWriter, r *http.Request) {
	snaps, err := listConfigSnapshots(a.configPath)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not read the history")
		return
	}
	if snaps == nil {
		snaps = []configSnapshot{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"snapshots": snaps})
}

func (a *application) handleSettingsHistoryRestore(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := a.restoreConfigSnapshot(id); err != nil {
		status := http.StatusInternalServerError
		if _, ok := err.(*editorValidationError); ok {
			status = http.StatusUnprocessableEntity
		}
		writeJSONError(w, status, "restore failed: "+err.Error())
		return
	}
	slog.Info("Config restored from Settings history", "snapshot", id)
	writeJSON(w, http.StatusOK, map[string]any{"restored": id})
}

// The response has to reach the browser before the server restarts.
func reloadSoon() {
	go func() {
		time.Sleep(300 * time.Millisecond)
		requestConfigReload()
	}()
}
