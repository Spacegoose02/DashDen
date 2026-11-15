package dynacat

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
)

// proxyAuthConfig signs people in from the headers a forward-auth proxy such as
// Tinyauth, Authelia or Authentik adds after it has checked the session.
//
// The headers are only believed when the request's direct peer is one of
// server.trusted-proxies, so a client that reaches DashDen without going
// through the proxy cannot claim to be someone else.
type proxyAuthConfig struct {
	UserHeader    string   `yaml:"user-header"`
	NameHeader    string   `yaml:"name-header"`
	EmailHeader   string   `yaml:"email-header"`
	GroupsHeader  string   `yaml:"groups-header"`
	LoginURL      string   `yaml:"login-url"`
	LogoutURL     string   `yaml:"logout-url"`
	AllowedUsers  []string `yaml:"allowed-users"`
	AllowedGroups []string `yaml:"allowed-groups"`
}

func (c *proxyAuthConfig) applyDefaults() {
	if c.UserHeader == "" {
		c.UserHeader = "Remote-User"
	}
	if c.NameHeader == "" {
		c.NameHeader = "Remote-Name"
	}
	if c.EmailHeader == "" {
		c.EmailHeader = "Remote-Email"
	}
	if c.GroupsHeader == "" {
		c.GroupsHeader = "Remote-Groups"
	}
}

func validateProxyAuthConfig(cfg *config) error {
	if cfg.Auth.Proxy == nil {
		return nil
	}

	if len(cfg.Server.TrustedProxies) == 0 {
		return fmt.Errorf("auth.proxy requires server.trusted-proxies, otherwise anyone could send the sign-in headers")
	}

	for _, u := range []string{cfg.Auth.Proxy.LoginURL, cfg.Auth.Proxy.LogoutURL} {
		if u != "" && !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
			return fmt.Errorf("auth.proxy: login-url and logout-url must be absolute http(s) URLs, got %q", u)
		}
	}

	return nil
}

// proxyOnly reports whether the forward-auth proxy is the only way to sign in.
func (a *application) proxyOnly() bool {
	return a.ProxyAuthEnabled && !a.PasswordEnabled && !a.OIDCEnabled
}

func (a *application) getProxyAuthenticatedUser(r *http.Request) *authenticatedUser {
	if !a.ProxyAuthEnabled {
		return nil
	}

	cfg := a.Config.Auth.Proxy

	// Only the direct peer counts here: X-Forwarded-For can be set by anyone.
	if !a.ipIsTrustedProxy(remoteAddrWithoutPort(r)) {
		return nil
	}

	username := strings.TrimSpace(r.Header.Get(cfg.UserHeader))
	if username == "" {
		return nil
	}

	groups := splitProxyGroups(r.Header.Get(cfg.GroupsHeader))

	if !proxyUserAllowed(cfg, username, groups) {
		return nil
	}

	return &authenticatedUser{
		Username: username,
		Name:     strings.TrimSpace(r.Header.Get(cfg.NameHeader)),
		Email:    strings.TrimSpace(r.Header.Get(cfg.EmailHeader)),
		Groups:   groups,
		IsProxy:  true,
	}
}

func splitProxyGroups(raw string) []string {
	if raw == "" {
		return nil
	}

	groups := make([]string, 0, 4)
	for _, g := range strings.Split(raw, ",") {
		if g = strings.TrimSpace(g); g != "" {
			groups = append(groups, g)
		}
	}

	return groups
}

func proxyUserAllowed(cfg *proxyAuthConfig, username string, groups []string) bool {
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

// handleProxyOnlyLogin covers /login when the proxy is the only sign-in method.
// Normally the proxy never lets an anonymous request through, so getting here
// means the request bypassed it or the person isn't allowed.
func (a *application) handleProxyOnlyLogin(w http.ResponseWriter, r *http.Request) {
	if a.Config.Auth.Proxy.LoginURL != "" && !r.URL.Query().Has("logged-out") {
		http.Redirect(w, r, a.Config.Auth.Proxy.LoginURL, http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte("Sign in through your sign-in proxy to use this dashboard.\n"))
}
