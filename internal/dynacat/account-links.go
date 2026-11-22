package dynacat

// DashDen: linked accounts. A household member can list other sign-in names
// under linked-accounts, so that signing in through Tinyauth (or OIDC, or a
// second local account) as one of them signs in as that member. People link
// their own sign-in from /account by proving a DashDen password, and admins
// link anyone's from Settings > People.

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

var accountPageTemplate = mustParseTemplate("account.html", "document.html", "footer.html")

const maxLinkedAccountLength = 254

// ---- Sign-ins seen without a profile ----

// unlinkedSignIn is someone who signed in through the proxy or OIDC and has
// no household profile, kept so admins can link them without typing names.
type unlinkedSignIn struct {
	Username string    `json:"username"`
	Email    string    `json:"email,omitempty"`
	Method   string    `json:"method"`
	LastSeen time.Time `json:"lastSeen"`
}

type unlinkedSignInStore struct {
	mu    sync.Mutex
	items map[string]*unlinkedSignIn
}

const unlinkedSignInLimit = 50
const unlinkedSignInMaxAge = 14 * 24 * time.Hour

// Shared across config reloads, like the OIDC sessions.
var recentUnlinkedSignIns = &unlinkedSignInStore{}

func (s *unlinkedSignInStore) record(u *authenticatedUser) {
	method := "Sign-in proxy"
	if u.IsOIDC {
		method = "Single sign-on"
	}
	key := strings.ToLower(u.Username)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items == nil {
		s.items = map[string]*unlinkedSignIn{}
	}
	if item, ok := s.items[key]; ok {
		item.LastSeen = time.Now()
		if u.Email != "" {
			item.Email = u.Email
		}
		return
	}
	if len(s.items) >= unlinkedSignInLimit {
		oldest := ""
		for k, item := range s.items {
			if oldest == "" || item.LastSeen.Before(s.items[oldest].LastSeen) {
				oldest = k
			}
		}
		delete(s.items, oldest)
	}
	s.items[key] = &unlinkedSignIn{Username: u.Username, Email: u.Email, Method: method, LastSeen: time.Now()}
}

// list returns the recent sign-ins that still have no profile, newest first.
func (s *unlinkedSignInStore) list(a *application) []unlinkedSignIn {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []unlinkedSignIn{}
	for k, item := range s.items {
		if time.Since(item.LastSeen) > unlinkedSignInMaxAge {
			delete(s.items, k)
			continue
		}
		if _, _, m := a.findMemberForSignIn(item.Username, item.Email); m != nil {
			continue
		}
		out = append(out, *item)
	}
	slices.SortFunc(out, func(x, y unlinkedSignIn) int { return y.LastSeen.Compare(x.LastSeen) })
	return out
}

func (s *unlinkedSignInStore) forget(name string) {
	s.mu.Lock()
	delete(s.items, strings.ToLower(name))
	s.mu.Unlock()
}

// ---- Checks shared by /account and Settings ----

func cleanLinkedAccount(link string) (string, error) {
	link = strings.TrimSpace(link)
	if link == "" {
		return "", nil
	}
	if len(link) > maxLinkedAccountLength || strings.ContainsAny(link, " \t\r\n,") {
		return "", fmt.Errorf("%q isn't a sign-in name or email address", link)
	}
	return link, nil
}

// linkOwner reports who else uses name: the member who has it as their
// username or linked account, other than except.
func (a *application) linkOwner(name, except string) (owner string, isProfile bool) {
	if key, m := a.findHouseholdMember(name); m != nil && key != except {
		return key, true
	}
	if key, _, m := a.findLinkedMember(name); m != nil && key != except {
		return key, false
	}
	return "", false
}

// ---- /account ----

func (a *application) registerAccountRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /account", a.handleAccountPage)
	mux.HandleFunc("POST /account/link", a.handleAccountLink)
	mux.HandleFunc("POST /account/unlink", a.handleAccountUnlink)
}

type accountPageData struct {
	templateData
	Notice        string
	Error         string
	SignInName    string
	ViaLink       bool
	Unlinked      bool
	CanLink       bool
	CanEditConfig bool
	Links         []string
}

func (a *application) handleAccountPage(w http.ResponseWriter, r *http.Request) {
	a.renderAccountPage(w, r, http.StatusOK, r.URL.Query().Get("done"), "")
}

func (a *application) renderAccountPage(w http.ResponseWriter, r *http.Request, status int, notice, failure string) {
	user := a.getAuthenticatedUser(w, r)
	if user == nil {
		http.Redirect(w, r, a.Config.Server.BaseURL+"/login", http.StatusSeeOther)
		return
	}

	data := accountPageData{templateData: templateData{App: a, AuthUser: user}, Error: failure}
	switch notice {
	case "linked":
		data.Notice = "Linked. It takes a second or two to apply, so reload if this page still looks the same."
	case "unlinked":
		data.Notice = "Unlinked. That sign-in no longer signs in as you."
	}
	data.SignInName = user.Username
	if user.SignInName != "" {
		data.SignInName = user.SignInName
	}
	data.ViaLink = user.LinkedBy != ""
	data.CanEditConfig = a.configPath != ""
	if m := user.Member(); m != nil {
		data.Links = m.LinkedAccounts
	} else {
		data.Unlinked = user.IsProxy || user.IsOIDC
		data.CanLink = data.Unlinked && a.PasswordEnabled && data.CanEditConfig
	}

	a.populateTemplateRequestData(&data.Request, r, user)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := accountPageTemplate.Execute(w, data); err != nil {
		slog.Error("Rendering account page failed", "error", err)
	}
}

// sameOriginForm refuses form posts from other sites. Proxy sign-in rides on
// the proxy's own cookie, so a cross-site form would otherwise be signed in.
func sameOriginForm(r *http.Request) bool {
	source := r.Header.Get("Origin")
	if source == "" || source == "null" {
		source = r.Header.Get("Referer")
	}
	if source == "" {
		return false
	}
	u, err := url.Parse(source)
	return err == nil && u.Host == r.Host
}

func (a *application) handleAccountLink(w http.ResponseWriter, r *http.Request) {
	if !sameOriginForm(r) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return
	}
	user := a.getAuthenticatedUser(w, r)
	if user == nil {
		http.Redirect(w, r, a.Config.Server.BaseURL+"/login", http.StatusSeeOther)
		return
	}
	if user.Member() != nil {
		a.renderAccountPage(w, r, http.StatusConflict, "", "This sign-in already has a profile.")
		return
	}
	if !(user.IsProxy || user.IsOIDC) || !a.PasswordEnabled || a.configPath == "" {
		a.renderAccountPage(w, r, http.StatusBadRequest, "", "Linking from here needs DashDen's own sign-in. Ask an admin to link you under Settings, People.")
		return
	}

	ip := a.addressOfRequest(r)
	if limited, _ := a.checkAuthRateLimit(ip); limited {
		a.renderAccountPage(w, r, http.StatusTooManyRequests, "", "Too many tries. Wait a few minutes and try again.")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	r.ParseForm()
	username := strings.TrimSpace(r.PostForm.Get("username"))
	password := r.PostForm.Get("password")
	account, pw := a.findPasswordUser(username)
	if pw == nil || len(password) > 100 || bcrypt.CompareHashAndPassword(pw.PasswordHash, []byte(password)) != nil {
		slog.Warn("Failed attempt to link an account", "sign_in", user.Username, "username", username, "ip", ip)
		time.Sleep(time.Second)
		a.renderAccountPage(w, r, http.StatusUnauthorized, "", "That username and password don't match a DashDen account.")
		return
	}
	a.clearAuthRateLimit(ip)

	link, err := cleanLinkedAccount(user.Username)
	if err != nil || link == "" {
		a.renderAccountPage(w, r, http.StatusBadRequest, "", "Your sign-in name can't be linked. Ask an admin.")
		return
	}
	// A local account with the same name is someone else's, and linking
	// would sign them in as you too.
	if other, _ := a.findPasswordUser(link); other != "" && other != account {
		a.renderAccountPage(w, r, http.StatusConflict, "", "There's also a DashDen account called "+other+", so an admin has to link this one for you.")
		return
	}

	// The account may itself be linked to someone's profile.
	target, _, m := a.findMemberForSignIn(account, "")
	if m == nil {
		target = account
	}
	if err := a.addLinkedAccount(target, link); err != nil {
		slog.Error("Could not link an account", "sign_in", link, "to", target, "error", err)
		a.renderAccountPage(w, r, http.StatusInternalServerError, "", "Couldn't save the link: "+err.Error())
		return
	}
	recentUnlinkedSignIns.forget(link)
	slog.Info("Sign-in linked to a household profile by its owner", "sign_in", link, "to", target)
	http.Redirect(w, r, a.Config.Server.BaseURL+"/account?done=linked", http.StatusSeeOther)
}

func (a *application) handleAccountUnlink(w http.ResponseWriter, r *http.Request) {
	if !sameOriginForm(r) {
		http.Error(w, "cross-origin request refused", http.StatusForbidden)
		return
	}
	user := a.getAuthenticatedUser(w, r)
	if user == nil {
		http.Redirect(w, r, a.Config.Server.BaseURL+"/login", http.StatusSeeOther)
		return
	}
	m := user.Member()
	if m == nil || a.configPath == "" {
		a.renderAccountPage(w, r, http.StatusBadRequest, "", "There's nothing to unlink.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	r.ParseForm()
	name := strings.TrimSpace(r.PostForm.Get("account"))
	if !slices.ContainsFunc(m.LinkedAccounts, func(l string) bool { return strings.EqualFold(l, name) }) {
		a.renderAccountPage(w, r, http.StatusNotFound, "", name+" isn't linked to you.")
		return
	}
	if strings.EqualFold(user.LinkedBy, name) {
		a.renderAccountPage(w, r, http.StatusConflict, "", "You're signed in through "+name+" right now, so unlinking it would sign you out of your profile. Sign in another way first, or ask an admin.")
		return
	}

	err := a.changeConfig(func(root *yaml.Node) error {
		member, err := editableSection(root, "household", "members", user.Username)
		if err != nil {
			return err
		}
		links := []string{}
		if seq := getMappingValue(member, "linked-accounts"); seq != nil {
			for _, item := range seq.Content {
				if !strings.EqualFold(item.Value, name) {
					links = append(links, item.Value)
				}
			}
		}
		setListOrRemove(member, "linked-accounts", links)
		return nil
	})
	if err != nil {
		a.renderAccountPage(w, r, http.StatusInternalServerError, "", "Couldn't save: "+err.Error())
		return
	}
	slog.Info("Linked account removed by its owner", "member", user.Username, "account", name)
	http.Redirect(w, r, a.Config.Server.BaseURL+"/account?done=unlinked", http.StatusSeeOther)
}

// addLinkedAccount adds link to the member's linked-accounts, giving them an
// empty profile first if they have none.
func (a *application) addLinkedAccount(member, link string) error {
	return a.changeConfig(func(root *yaml.Node) error {
		node, err := editableSection(root, "household", "members", member)
		if err != nil {
			return err
		}
		links := []string{}
		if seq := getMappingValue(node, "linked-accounts"); seq != nil {
			for _, item := range seq.Content {
				links = append(links, item.Value)
			}
		}
		if !slices.ContainsFunc(links, func(l string) bool { return strings.EqualFold(l, link) }) {
			links = append(links, link)
		}
		setListOrRemove(node, "linked-accounts", links)
		return nil
	})
}
