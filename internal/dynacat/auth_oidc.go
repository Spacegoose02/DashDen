package dynacat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const OIDC_STATE_COOKIE_NAME = "oidc_state"
const OIDC_PKCE_COOKIE_NAME = "oidc_pkce"
const OIDC_STATE_VALID_PERIOD = 5 * time.Minute

func initOIDCProvider(cfg *oidcConfig) (*gooidc.Provider, *gooidc.IDTokenVerifier, *oauth2.Config, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	provider, err := gooidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("initializing OIDC provider from %s: %w", cfg.IssuerURL, err)
	}

	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{gooidc.ScopeOpenID, "profile", "email"}
	}

	oauth2Cfg := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       scopes,
	}

	verifier := provider.Verifier(&gooidc.Config{ClientID: cfg.ClientID})

	return provider, verifier, oauth2Cfg, nil
}

// ensureOIDC looks the provider up if that hasn't worked yet, at most every
// 15 seconds.
func (a *application) ensureOIDC() error {
	a.oidcMu.Lock()
	defer a.oidcMu.Unlock()
	if a.oidcProvider != nil {
		return nil
	}
	if !a.oidcTriedAt.IsZero() && time.Since(a.oidcTriedAt) < 15*time.Second {
		return fmt.Errorf("couldn't reach %s a moment ago", a.Config.Auth.OIDC.IssuerURL)
	}
	a.oidcTriedAt = time.Now()
	provider, verifier, oauth2Cfg, err := initOIDCProvider(a.Config.Auth.OIDC)
	if err != nil {
		return err
	}
	a.oidcProvider, a.oidcVerifier, a.oauth2Config = provider, verifier, oauth2Cfg
	return nil
}

// oidcCallbackURL is redirect-url when set, otherwise this server's own
// callback address as the browser sees it.
func (a *application) oidcCallbackURL(r *http.Request) string {
	if a.Config.Auth.OIDC != nil && a.Config.Auth.OIDC.RedirectURL != "" {
		return a.Config.Auth.OIDC.RedirectURL
	}
	// The provider only accepts the callback address it was given, so a
	// spoofed header here can't do harm; it just has to match what the
	// browser used, which behind a reverse proxy is usually https.
	scheme := "http"
	if a.isRequestHTTPS(r) || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host + a.Config.Server.BaseURL + "/api/oidc/callback"
}

// oidcClient is the OAuth2 setup for this request, with its callback address.
func (a *application) oidcClient(r *http.Request) (*oauth2.Config, error) {
	if err := a.ensureOIDC(); err != nil {
		return nil, err
	}
	cfg := *a.oauth2Config
	cfg.RedirectURL = a.oidcCallbackURL(r)
	return &cfg, nil
}

// OIDCButtonName is the text on the sign-in button.
func (a *application) OIDCButtonName() string {
	if a.Config.Auth.OIDC != nil && a.Config.Auth.OIDC.ButtonName != "" {
		return "Sign in with " + a.Config.Auth.OIDC.ButtonName
	}
	return "Login with SSO"
}

// OIDCButtonShown is false when the provider is set to stay off the sign-in page.
func (a *application) OIDCButtonShown() bool {
	return a.OIDCEnabled && !a.Config.Auth.OIDC.HideButton
}

func (a *application) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	client, err := a.oidcClient(r)
	if err != nil {
		slog.Error("OIDC provider unavailable", "error", err)
		http.Redirect(w, r, a.Config.Server.BaseURL+"/login?error=provider_down", http.StatusSeeOther)
		return
	}

	state, err := makeAuthSecretKey(16)
	if err != nil {
		slog.Error("OIDC could not generate state", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	pkceVerifier := oauth2.GenerateVerifier()

	a.setCookie(w, r, &http.Cookie{
		Name:     OIDC_STATE_COOKIE_NAME,
		Value:    state,
		Expires:  time.Now().Add(OIDC_STATE_VALID_PERIOD),
		SameSite: http.SameSiteLaxMode,
	})

	a.setCookie(w, r, &http.Cookie{
		Name:     OIDC_PKCE_COOKIE_NAME,
		Value:    pkceVerifier,
		Expires:  time.Now().Add(OIDC_STATE_VALID_PERIOD),
		SameSite: http.SameSiteLaxMode,
	})

	url := client.AuthCodeURL(state, oauth2.S256ChallengeOption(pkceVerifier))
	http.Redirect(w, r, url, http.StatusFound)
}

func (a *application) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	baseURL := a.Config.Server.BaseURL

	stateCookie, err := r.Cookie(OIDC_STATE_COOKIE_NAME)
	if err != nil || stateCookie.Value == "" {
		http.Redirect(w, r, baseURL+"/login?error=invalid_state", http.StatusSeeOther)
		return
	}

	if r.URL.Query().Get("state") != stateCookie.Value {
		http.Redirect(w, r, baseURL+"/login?error=invalid_state", http.StatusSeeOther)
		return
	}

	pkceCookie, err := r.Cookie(OIDC_PKCE_COOKIE_NAME)
	if err != nil || pkceCookie.Value == "" {
		http.Redirect(w, r, baseURL+"/login?error=invalid_state", http.StatusSeeOther)
		return
	}

	a.clearCookie(w, r, OIDC_STATE_COOKIE_NAME)
	a.clearCookie(w, r, OIDC_PKCE_COOKIE_NAME)

	code := r.URL.Query().Get("code")
	if code == "" {
		errParam := r.URL.Query().Get("error")
		if errParam == "" {
			errParam = "missing_code"
		}
		http.Redirect(w, r, baseURL+"/login?error="+url.QueryEscape(errParam), http.StatusSeeOther)
		return
	}

	client, err := a.oidcClient(r)
	if err != nil {
		slog.Error("OIDC provider unavailable", "error", err)
		http.Redirect(w, r, baseURL+"/login?error=provider_down", http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	oauth2Token, err := client.Exchange(ctx, code, oauth2.VerifierOption(pkceCookie.Value))
	if err != nil {
		slog.Error("OIDC token exchange failed", "error", err)
		http.Redirect(w, r, baseURL+"/login?error=token_exchange", http.StatusSeeOther)
		return
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		http.Redirect(w, r, baseURL+"/login?error=no_id_token", http.StatusSeeOther)
		return
	}

	idToken, err := a.oidcVerifier.Verify(ctx, rawIDToken)
	if err != nil {
		slog.Error("OIDC ID token verification failed", "error", err)
		http.Redirect(w, r, baseURL+"/login?error=token_verify", http.StatusSeeOther)
		return
	}

	var claims map[string]interface{}
	if err := idToken.Claims(&claims); err != nil {
		slog.Error("OIDC could not extract claims", "error", err)
		http.Redirect(w, r, baseURL+"/login?error=claims", http.StatusSeeOther)
		return
	}

	usernameClaim := a.Config.Auth.OIDC.UsernameClaim
	if usernameClaim == "" {
		usernameClaim = "preferred_username"
	}

	username, _ := claims[usernameClaim].(string)
	if username == "" {
		username, _ = claims["sub"].(string)
	}
	if username == "" {
		http.Redirect(w, r, baseURL+"/login?error=no_username", http.StatusSeeOther)
		return
	}

	groupsClaim := a.Config.Auth.OIDC.GroupsClaim
	if groupsClaim == "" {
		groupsClaim = "groups"
	}
	groups := extractGroupsClaim(claims, groupsClaim)

	if !oidcUserAllowed(a.Config.Auth.OIDC, username, groups) {
		slog.Warn("OIDC user not in allowed users/groups", "username", username)
		http.Redirect(w, r, baseURL+"/login?error=not_allowed", http.StatusSeeOther)
		return
	}

	known, create, refused := a.oidcProfileDecision(username)
	if refused {
		slog.Warn("OIDC sign-in by someone without a household profile", "username", username)
		http.Redirect(w, r, baseURL+"/login?error=no_account", http.StatusSeeOther)
		return
	}
	username = known
	if create {
		name, _ := claims["name"].(string)
		if err := a.addHouseholdMemberFromSignIn(username, name); err != nil {
			slog.Error("Could not create a profile for a new sign-in", "username", username, "error", err)
		} else {
			slog.Info("Created a household profile on first sign-in", "username", username)
		}
	}

	sessionID, err := makeAuthSecretKey(32)
	if err != nil {
		slog.Error("OIDC could not generate session ID", "error", err)
		http.Redirect(w, r, baseURL+"/login?error=internal", http.StatusSeeOther)
		return
	}

	a.oidcSessions.set(sessionID, &oidcSession{
		Username:  username,
		Groups:    groups,
		Token:     oauth2Token,
		CreatedAt: time.Now(),
	})

	// Strict would not survive the redirect back from the provider.
	a.setCookie(w, r, &http.Cookie{
		Name:     OIDC_SESSION_COOKIE_NAME,
		Value:    sessionID,
		Expires:  time.Now().Add(OIDC_SESSION_VALID_PERIOD),
		SameSite: http.SameSiteLaxMode,
	})

	slog.Info("OIDC user logged in", "username", username)
	http.Redirect(w, r, a.takeLoginRedirect(w, r), http.StatusSeeOther)
}

// oidcProfileDecision applies create-accounts: it returns the username to use
// (the household's spelling when there is a profile), whether a profile
// should be made, and whether the person is refused for not having one.
func (a *application) oidcProfileDecision(username string) (string, bool, bool) {
	create := a.Config.Auth.OIDC.CreateAccounts
	known, m := a.findHouseholdMember(username)
	if m != nil {
		return known, false, false
	}
	if create == nil {
		return username, false, false
	}
	return username, *create, !*create
}

func oidcUserAllowed(cfg *oidcConfig, username string, groups []string) bool {
	if len(cfg.AllowedUsers) == 0 && len(cfg.AllowedGroups) == 0 {
		return true
	}

	if slices.Contains(cfg.AllowedUsers, username) {
		return true
	}

	for _, g := range cfg.AllowedGroups {
		if slices.Contains(groups, g) {
			return true
		}
	}

	return false
}

func extractGroupsClaim(claims map[string]interface{}, claimName string) []string {
	raw, ok := claims[claimName]
	if !ok {
		return nil
	}

	switch v := raw.(type) {
	case []interface{}:
		groups := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				groups = append(groups, s)
			}
		}
		return groups
	case string:
		if v == "" {
			return nil
		}
		if strings.Contains(v, ",") {
			parts := strings.Split(v, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			return parts
		}
		return strings.Fields(v)
	case json.RawMessage:
		var items []string
		if err := json.Unmarshal(v, &items); err == nil {
			return items
		}
	}

	return nil
}
