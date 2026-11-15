package dynacat

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newProxyTestApp(t *testing.T, cfg *proxyAuthConfig, trusted string) *application {
	t.Helper()
	cfg.applyDefaults()
	_, ipnet, err := net.ParseCIDR(trusted)
	if err != nil {
		t.Fatal(err)
	}
	a := &application{Config: config{}, ProxyAuthEnabled: true, RequiresAuth: true}
	a.Config.Auth.Proxy = cfg
	a.Config.Server.trustedProxyNets = []*net.IPNet{ipnet}
	return a
}

func proxyRequest(remote string, headers map[string]string) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = remote
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func TestProxyAuthTrustsHeadersFromTrustedProxy(t *testing.T) {
	a := newProxyTestApp(t, &proxyAuthConfig{}, "10.0.0.0/24")
	r := proxyRequest("10.0.0.5:1234", map[string]string{
		"Remote-User":   "sam",
		"Remote-Name":   "Sam Den",
		"Remote-Email":  "sam@example.com",
		"Remote-Groups": "adults, admins ,,",
	})

	u := a.getAuthenticatedUser(httptest.NewRecorder(), r)
	if u == nil {
		t.Fatal("expected a signed-in user")
	}
	if u.Username != "sam" || u.DisplayName() != "Sam Den" || u.Email != "sam@example.com" || !u.IsProxy {
		t.Fatalf("unexpected user: %+v", u)
	}
	if len(u.Groups) != 2 || u.Groups[0] != "adults" || u.Groups[1] != "admins" {
		t.Fatalf("unexpected groups: %#v", u.Groups)
	}
}

func TestProxyAuthIgnoresHeadersFromUntrustedPeer(t *testing.T) {
	a := newProxyTestApp(t, &proxyAuthConfig{}, "10.0.0.0/24")
	r := proxyRequest("192.168.1.9:1234", map[string]string{"Remote-User": "sam"})
	r.Header.Set("X-Forwarded-For", "10.0.0.5")

	if u := a.getAuthenticatedUser(httptest.NewRecorder(), r); u != nil {
		t.Fatalf("headers from an untrusted peer must be ignored, got %+v", u)
	}
}

func TestProxyAuthRequiresUserHeader(t *testing.T) {
	a := newProxyTestApp(t, &proxyAuthConfig{}, "10.0.0.0/24")
	r := proxyRequest("10.0.0.5:1234", map[string]string{"Remote-Name": "Sam"})

	if u := a.getAuthenticatedUser(httptest.NewRecorder(), r); u != nil {
		t.Fatalf("no Remote-User means no user, got %+v", u)
	}
}

func TestProxyAuthCustomHeadersAndAllowList(t *testing.T) {
	a := newProxyTestApp(t, &proxyAuthConfig{UserHeader: "X-User", AllowedGroups: []string{"family"}}, "10.0.0.1/32")

	if u := a.getAuthenticatedUser(httptest.NewRecorder(), proxyRequest("10.0.0.1:1", map[string]string{"X-User": "guest"})); u != nil {
		t.Fatalf("user outside allowed-groups must be refused, got %+v", u)
	}

	u := a.getAuthenticatedUser(httptest.NewRecorder(), proxyRequest("10.0.0.1:1", map[string]string{"X-User": "kid", "Remote-Groups": "family"}))
	if u == nil || u.Username != "kid" {
		t.Fatalf("expected kid to be allowed, got %+v", u)
	}
}

func TestProxyAuthConfigNeedsTrustedProxies(t *testing.T) {
	cfg := &config{}
	cfg.Auth.Proxy = &proxyAuthConfig{}
	if err := validateProxyAuthConfig(cfg); err == nil {
		t.Fatal("expected an error without server.trusted-proxies")
	}

	cfg.Server.TrustedProxies = []string{"10.0.0.2"}
	cfg.Auth.Proxy.LogoutURL = "auth.example.com/logout"
	if err := validateProxyAuthConfig(cfg); err == nil {
		t.Fatal("expected an error for a relative logout-url")
	}

	cfg.Auth.Proxy.LogoutURL = "https://auth.example.com/logout"
	if err := validateProxyAuthConfig(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProxyOnlyLoginRedirectsToProxy(t *testing.T) {
	a := newProxyTestApp(t, &proxyAuthConfig{LoginURL: "https://auth.example.com/login"}, "10.0.0.0/24")

	w := httptest.NewRecorder()
	a.handleLoginPageRequest(w, proxyRequest("192.168.1.9:1", nil))
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "https://auth.example.com/login" {
		t.Fatalf("expected redirect to the proxy login, got %d %q", w.Code, w.Header().Get("Location"))
	}
}

func TestProxyLogoutGoesToProxy(t *testing.T) {
	a := newProxyTestApp(t, &proxyAuthConfig{LogoutURL: "https://auth.example.com/logout"}, "10.0.0.0/24")

	w := httptest.NewRecorder()
	r := proxyRequest("10.0.0.5:1", map[string]string{"Remote-User": "sam"})
	r.Method = "POST"
	a.handleLogoutRequest(w, r)
	if w.Header().Get("Location") != "https://auth.example.com/logout" {
		t.Fatalf("expected redirect to the proxy logout, got %q", w.Header().Get("Location"))
	}
}
