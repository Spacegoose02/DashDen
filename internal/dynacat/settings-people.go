package dynacat

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

// People, Sign-in and Access tabs of Settings. Every change is written back
// into the YAML config through writeConfigCandidates, so it is validated,
// saved to History, and can be undone like any other edit.

func (a *application) registerSettingsPeopleRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings/people", a.settingsJSON(a.handleSettingsPeopleList))
	mux.HandleFunc("PUT /api/settings/people/{username}", a.settingsJSON(a.handleSettingsPersonSave))
	mux.HandleFunc("DELETE /api/settings/people/{username}", a.settingsJSON(a.handleSettingsPersonDelete))
	mux.HandleFunc("GET /api/settings/signin", a.settingsJSON(a.handleSettingsSignInGet))
	mux.HandleFunc("PUT /api/settings/signin", a.settingsJSON(a.handleSettingsSignInSave))
	mux.HandleFunc("GET /api/settings/access", a.settingsJSON(a.handleSettingsAccessList))
	mux.HandleFunc("PUT /api/settings/access/{index}", a.settingsJSON(a.handleSettingsAccessSave))
}

var settingsUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@-]{0,63}$`)

// settingsUser is who is making the change, or nil in recovery mode.
func (a *application) settingsUser(w http.ResponseWriter, r *http.Request) *authenticatedUser {
	if inRecovery(r) {
		return nil
	}
	return a.getAuthenticatedUser(w, r)
}

func decodeSettingsBody(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errors.New("invalid JSON: " + err.Error())
	}
	return nil
}

// editableSection walks to root.a.b..., creating mappings on the way, and
// refuses sections that live in an included file.
func editableSection(root *yaml.Node, path ...string) (*yaml.Node, error) {
	node := root
	for i, key := range path {
		next := getMappingValue(node, key)
		where := strings.Join(path[:i+1], ".")
		if next == nil || (next.Kind == yaml.ScalarNode && next.Value == "" && next.Tag != "" && !isIncludeTag(next.Tag)) || next.Tag == "!!null" {
			next = newMappingNode()
			setMappingKey(node, key, next)
		}
		if isIncludeTag(next.Tag) || includeTarget(next) != "" {
			return nil, fmt.Errorf("%s is kept in an included file; change it there or with the config editor", where)
		}
		if next.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%s in the config isn't a section DashDen can edit", where)
		}
		node = next
	}
	return node, nil
}

func pruneEmptySection(root *yaml.Node, path ...string) {
	for len(path) > 0 {
		parent := root
		for _, key := range path[:len(path)-1] {
			parent = getMappingValue(parent, key)
			if parent == nil {
				return
			}
		}
		last := path[len(path)-1]
		n := getMappingValue(parent, last)
		if n == nil || n.Kind != yaml.MappingNode || len(n.Content) > 0 {
			return
		}
		removeMappingKey(parent, last)
		path = path[:len(path)-1]
	}
}

func stringListNode(values []string) *yaml.Node {
	seq := sequenceNode()
	seq.Style = yaml.FlowStyle
	for _, v := range values {
		seq.Content = append(seq.Content, scalarNode(v))
	}
	return seq
}

func setOrRemove(m *yaml.Node, key, value string) {
	if value == "" {
		removeMappingKey(m, key)
		return
	}
	setMappingKey(m, key, scalarNode(value))
}

func setListOrRemove(m *yaml.Node, key string, values []string) {
	if len(values) == 0 {
		removeMappingKey(m, key)
		return
	}
	setMappingKey(m, key, stringListNode(values))
}

func cleanList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// changeConfig loads the main config file, applies change and writes it back.
func (a *application) changeConfig(change func(root *yaml.Node) error) error {
	a.editorMu.Lock()
	defer a.editorMu.Unlock()

	doc, err := loadYAMLDocument(a.configPath)
	if err != nil {
		return err
	}
	root := documentRoot(doc)
	if root.Kind != yaml.MappingNode {
		return errors.New("the config file isn't a YAML mapping")
	}
	if err := change(root); err != nil {
		return err
	}
	return a.writeDocument(a.configPath, doc)
}

func writeSettingsWriteError(w http.ResponseWriter, err error) {
	var verr *editorValidationError
	var perr *settingsRefusal
	switch {
	case errors.As(err, &perr):
		writeJSONError(w, http.StatusConflict, perr.Error())
	case errors.As(err, &verr):
		writeJSONError(w, http.StatusUnprocessableEntity, "The config wouldn't load with that change, so nothing was saved: "+verr.Error())
	default:
		writeJSONError(w, http.StatusBadRequest, err.Error())
	}
}

// settingsRefusal is a change Settings won't make because it would lock the
// person making it out.
type settingsRefusal struct{ msg string }

func (e *settingsRefusal) Error() string { return e.msg }

// ---- People ----

type settingsPerson struct {
	Username    string   `json:"username"`
	Name        string   `json:"name"`
	Avatar      string   `json:"avatar"`
	Role        string   `json:"role"`
	Home        string   `json:"home"`
	Theme       string   `json:"theme"`
	Groups      []string `json:"groups"`
	HasPassword bool     `json:"hasPassword"`
	// LinkedAccounts are other sign-in names that sign in as this person.
	LinkedAccounts []string `json:"linkedAccounts"`
}

type settingsPersonInput struct {
	Name     string   `json:"name"`
	Avatar   string   `json:"avatar"`
	Role     string   `json:"role"`
	Home     string   `json:"home"`
	Theme    string   `json:"theme"`
	Groups   []string `json:"groups"`
	Password string   `json:"password"`
	// LinkedAccounts replaces the person's linked accounts; left out, they
	// stay as they are.
	LinkedAccounts *[]string `json:"linkedAccounts"`
	// ReplaceProfiles allows linking names that have a profile of their own,
	// which removes that profile.
	ReplaceProfiles bool `json:"replaceProfiles"`
}

type settingsOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

func (a *application) handleSettingsPeopleList(w http.ResponseWriter, r *http.Request) {
	people := []settingsPerson{}
	for username, m := range a.Config.Household.Members {
		_, hasPassword := a.Config.Auth.Users[username]
		people = append(people, settingsPerson{
			Username: username, Name: m.Name, Avatar: m.Avatar, Role: m.Role,
			Home: m.Home, Theme: m.Theme, Groups: m.Groups, HasPassword: hasPassword,
			LinkedAccounts: nonNil(m.LinkedAccounts),
		})
	}
	// People who can sign in with a password but have no profile yet, so they
	// can be found here and given one.
	for username := range a.Config.Auth.Users {
		if _, _, m := a.findMemberForSignIn(username, ""); m == nil {
			people = append(people, settingsPerson{Username: username, HasPassword: true, LinkedAccounts: []string{}})
		}
	}
	slices.SortFunc(people, func(x, y settingsPerson) int { return strings.Compare(x.Username, y.Username) })

	pages := []settingsOption{}
	for i := range a.Config.Pages {
		pages = append(pages, settingsOption{Value: a.Config.Pages[i].Slug, Label: a.Config.Pages[i].Title})
	}
	themes := []settingsOption{}
	for key := range a.Config.Theme.Presets.Items() {
		themes = append(themes, settingsOption{Value: key, Label: key})
	}

	you := ""
	if u := a.settingsUser(w, r); u != nil {
		you = u.Username
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"people":         people,
		"roles":          householdRoles,
		"pages":          pages,
		"themes":         themes,
		"you":            you,
		"passwordSignIn": a.PasswordEnabled || len(a.Config.Auth.Users) > 0,
		"unlinked":       recentUnlinkedSignIns.list(a),
		"recovery":       inRecovery(r),
	})
}

func (a *application) handleSettingsPersonSave(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	if !settingsUsernamePattern.MatchString(username) {
		writeJSONError(w, http.StatusBadRequest, "Usernames are letters, numbers, dots, dashes, underscores and @, and match what the person signs in with.")
		return
	}
	// Sign-in ignores capital letters, so reuse the spelling already in use.
	if existing, m := a.findHouseholdMember(username); m != nil {
		username = existing
	} else if existing, u := a.findPasswordUser(username); u != nil {
		username = existing
	}
	var in settingsPersonInput
	if err := decodeSettingsBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Avatar = strings.TrimSpace(in.Avatar)
	in.Groups = cleanList(in.Groups)
	if in.Role != "" && !slices.Contains(householdRoles, in.Role) {
		writeJSONError(w, http.StatusBadRequest, "Unknown role.")
		return
	}
	if in.Home != "" && !a.pageSlugExists(in.Home) {
		writeJSONError(w, http.StatusBadRequest, "That home page doesn't exist.")
		return
	}
	if in.Password != "" && len(in.Password) < 8 {
		writeJSONError(w, http.StatusBadRequest, "Passwords need at least 8 characters.")
		return
	}
	if len(in.Password) > 72 {
		writeJSONError(w, http.StatusBadRequest, "Passwords can be at most 72 characters.")
		return
	}

	if u := a.settingsUser(w, r); u != nil && u.Username == username && u.IsAdmin() && in.Role != householdRoleAdmin {
		writeSettingsWriteError(w, &settingsRefusal{"You can't take admin away from yourself. Ask another admin, or use the recovery code."})
		return
	}

	var links, replaced []string
	if in.LinkedAccounts != nil {
		var refused bool
		links, replaced, refused = a.checkLinkedAccounts(w, r, username, *in.LinkedAccounts, in.ReplaceProfiles)
		if refused {
			return
		}
	}

	var hash []byte
	if in.Password != "" {
		var err error
		if hash, err = bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "could not hash the password")
			return
		}
	}

	err := a.changeConfig(func(root *yaml.Node) error {
		members, err := editableSection(root, "household", "members")
		if err != nil {
			return err
		}
		member := getMappingValue(members, username)
		if member == nil || member.Kind != yaml.MappingNode {
			member = newMappingNode()
			setMappingKey(members, username, member)
		}
		setOrRemove(member, "name", in.Name)
		setOrRemove(member, "avatar", in.Avatar)
		setOrRemove(member, "role", in.Role)
		setOrRemove(member, "home", in.Home)
		setOrRemove(member, "theme", in.Theme)
		setListOrRemove(member, "groups", in.Groups)
		if in.LinkedAccounts != nil {
			// Profiles being merged in bring their own linked accounts along.
			for _, name := range replaced {
				if old := getMappingValue(members, name); old != nil {
					if seq := getMappingValue(old, "linked-accounts"); seq != nil {
						for _, item := range seq.Content {
							if !slices.ContainsFunc(links, func(l string) bool { return strings.EqualFold(l, item.Value) }) {
								links = append(links, item.Value)
							}
						}
					}
				}
				removeMappingKey(members, name)
			}
			setListOrRemove(member, "linked-accounts", links)
		}

		if hash != nil {
			if err := ensureAuthSecretKey(a.configPath, root); err != nil {
				return err
			}
			users, err := editableSection(root, "auth", "users")
			if err != nil {
				return err
			}
			entry := getMappingValue(users, username)
			if entry == nil || entry.Kind != yaml.MappingNode {
				entry = newMappingNode()
				setMappingKey(users, username, entry)
			}
			removeMappingKey(entry, "password")
			setMappingKey(entry, "password-hash", scalarNode(string(hash)))
		}
		return nil
	})
	if err != nil {
		writeSettingsWriteError(w, err)
		return
	}
	for _, l := range links {
		recentUnlinkedSignIns.forget(l)
	}
	slog.Info("Household member saved from Settings", "username", username, "password_changed", hash != nil, "profiles_merged", len(replaced))
	writeJSON(w, http.StatusOK, map[string]any{"saved": username})
}

// checkLinkedAccounts cleans the linked accounts for username and refuses
// ones that belong to someone else. Names with a profile of their own are
// returned in replaced when replace is set, and otherwise answered with a
// 409 that lists them, so Settings can ask before merging. It writes the
// response itself when it refuses.
func (a *application) checkLinkedAccounts(w http.ResponseWriter, r *http.Request, username string, input []string, replace bool) (links, replaced []string, refused bool) {
	links = []string{}
	for _, raw := range input {
		link, err := cleanLinkedAccount(raw)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return nil, nil, true
		}
		if link == "" || slices.ContainsFunc(links, func(l string) bool { return strings.EqualFold(l, link) }) {
			continue
		}
		if strings.EqualFold(link, username) {
			writeJSONError(w, http.StatusBadRequest, link+" is this person's own username, so it doesn't need linking.")
			return nil, nil, true
		}
		owner, isProfile := a.linkOwner(link, username)
		switch {
		case owner == "":
		case !isProfile:
			writeSettingsWriteError(w, &settingsRefusal{link + " is already linked to " + owner + ". Unlink it there first."})
			return nil, nil, true
		default:
			if !slices.Contains(replaced, owner) {
				replaced = append(replaced, owner)
			}
		}
		links = append(links, link)
	}

	you := a.settingsUser(w, r)
	if you != nil && slices.Contains(replaced, you.Username) {
		writeSettingsWriteError(w, &settingsRefusal{"That would remove your own profile. Link the other way round: edit yourself and link " + username + " to you."})
		return nil, nil, true
	}
	// Don't let an admin unlink the sign-in they're using right now.
	if you != nil && you.Username == username && you.LinkedBy != "" &&
		!slices.ContainsFunc(links, func(l string) bool { return strings.EqualFold(l, you.LinkedBy) }) {
		writeSettingsWriteError(w, &settingsRefusal{"You're signed in through " + you.LinkedBy + ", so unlinking it would sign you out of your profile."})
		return nil, nil, true
	}
	if len(replaced) > 0 && !replace {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":           strings.Join(replaced, ", ") + " already has a profile. Linking replaces it.",
			"replaceProfiles": replaced,
		})
		return nil, nil, true
	}
	return links, replaced, false
}

func (a *application) pageSlugExists(slug string) bool {
	for i := range a.Config.Pages {
		if a.Config.Pages[i].Slug == slug {
			return true
		}
	}
	return false
}

// ensureAuthSecretKey adds the signing key that password sign-in needs. The
// key itself goes in the managed secrets file, and the config refers to it.
func ensureAuthSecretKey(configPath string, root *yaml.Node) error {
	auth, err := editableSection(root, "auth")
	if err != nil {
		return err
	}
	if scalarValue(getMappingValue(auth, "secret-key")) != "" {
		return nil
	}
	if os.Getenv(generatedSecretKeyName) == "" {
		key, err := makeAuthSecretKey(AUTH_SECRET_KEY_LENGTH)
		if err != nil {
			return err
		}
		if err := setManagedSecret(configPath, generatedSecretKeyName, &key); err != nil {
			return fmt.Errorf("could not save a sign-in key: %w", err)
		}
	}
	setMappingKey(auth, "secret-key", scalarNode("${"+generatedSecretKeyName+"}"))
	return nil
}

func (a *application) handleSettingsPersonDelete(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	if u := a.settingsUser(w, r); u != nil && u.Username == username {
		writeSettingsWriteError(w, &settingsRefusal{"You can't remove yourself."})
		return
	}
	removePassword := r.URL.Query().Get("password") == "remove"

	err := a.changeConfig(func(root *yaml.Node) error {
		members, err := editableSection(root, "household", "members")
		if err != nil {
			return err
		}
		removeMappingKey(members, username)
		pruneEmptySection(root, "household", "members")
		if removePassword {
			if users := getMappingValue(getMappingValue(root, "auth"), "users"); users != nil && users.Kind == yaml.MappingNode {
				removeMappingKey(users, username)
				pruneEmptySection(root, "auth", "users")
			}
		}
		return nil
	})
	if err != nil {
		writeSettingsWriteError(w, err)
		return
	}
	slog.Info("Household member removed from Settings", "username", username)
	writeJSON(w, http.StatusOK, map[string]any{"removed": username})
}

// ---- Sign-in ----

type settingsProxy struct {
	Enabled       bool     `json:"enabled"`
	LoginURL      string   `json:"loginURL"`
	LogoutURL     string   `json:"logoutURL"`
	AllowedUsers  []string `json:"allowedUsers"`
	AllowedGroups []string `json:"allowedGroups"`
	UserHeader    string   `json:"userHeader"`
	GroupsHeader  string   `json:"groupsHeader"`
}

type settingsSignIn struct {
	Proxy          settingsProxy `json:"proxy"`
	TrustedProxies []string      `json:"trustedProxies"`
}

func (a *application) handleSettingsSignInGet(w http.ResponseWriter, r *http.Request) {
	out := settingsSignIn{TrustedProxies: a.Config.Server.TrustedProxies}
	if out.TrustedProxies == nil {
		out.TrustedProxies = []string{}
	}
	if p := a.Config.Auth.Proxy; p != nil {
		out.Proxy = settingsProxy{
			Enabled: true, LoginURL: p.LoginURL, LogoutURL: p.LogoutURL,
			AllowedUsers: p.AllowedUsers, AllowedGroups: p.AllowedGroups,
		}
		if p.UserHeader != "Remote-User" {
			out.Proxy.UserHeader = p.UserHeader
		}
		if p.GroupsHeader != "Remote-Groups" {
			out.Proxy.GroupsHeader = p.GroupsHeader
		}
	}

	user := a.settingsUser(w, r)
	peer := remoteAddrWithoutPort(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"settings":        out,
		"peer":            peer,
		"peerIsTrusted":   len(a.Config.Server.trustedProxyNets) > 0 && a.ipIsTrustedProxy(peer),
		"signedInByProxy": user != nil && user.IsProxy,
		"oidc":            a.OIDCEnabled,
		"password":        a.PasswordEnabled,
		"recovery":        inRecovery(r),
	})
}

func (a *application) handleSettingsSignInSave(w http.ResponseWriter, r *http.Request) {
	var in settingsSignIn
	if err := decodeSettingsBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	in.TrustedProxies = cleanList(in.TrustedProxies)
	in.Proxy.AllowedUsers = cleanList(in.Proxy.AllowedUsers)
	in.Proxy.AllowedGroups = cleanList(in.Proxy.AllowedGroups)
	for _, entry := range in.TrustedProxies {
		if net.ParseIP(entry) == nil {
			if _, _, err := net.ParseCIDR(entry); err != nil {
				writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("%q isn't an IP address or range like 172.18.0.0/16.", entry))
				return
			}
		}
	}
	if in.Proxy.Enabled && len(in.TrustedProxies) == 0 {
		writeJSONError(w, http.StatusBadRequest, "Add the address of your sign-in proxy under trusted proxies first, otherwise anyone could pretend to be signed in.")
		return
	}

	// Don't let the person saving lock themselves out of the proxy.
	if user := a.settingsUser(w, r); user != nil && user.IsProxy {
		if !in.Proxy.Enabled && !a.PasswordEnabled && !a.OIDCEnabled {
			writeSettingsWriteError(w, &settingsRefusal{"You're signed in through the proxy and there's no other way to sign in, so turning it off from here would leave the dashboard open to anyone. Edit the config directly if that's what you want."})
			return
		}
		if in.Proxy.Enabled {
			cfg := &proxyAuthConfig{AllowedUsers: in.Proxy.AllowedUsers, AllowedGroups: in.Proxy.AllowedGroups}
			if !proxyUserAllowed(cfg, user.Username, user.Groups) {
				writeSettingsWriteError(w, &settingsRefusal{"That would stop you from signing in. Add yourself or one of your groups to the allowed list."})
				return
			}
			if !trustedListCovers(in.TrustedProxies, remoteAddrWithoutPort(r)) {
				writeSettingsWriteError(w, &settingsRefusal{"Your own request came from " + remoteAddrWithoutPort(r) + ", which isn't in the new trusted proxies, so you would be signed out."})
				return
			}
		}
	}

	err := a.changeConfig(func(root *yaml.Node) error {
		server, err := editableSection(root, "server")
		if err != nil {
			return err
		}
		setListOrRemove(server, "trusted-proxies", in.TrustedProxies)

		auth, err := editableSection(root, "auth")
		if err != nil {
			return err
		}
		if !in.Proxy.Enabled {
			removeMappingKey(auth, "proxy")
			pruneEmptySection(root, "auth")
			return nil
		}
		proxy, err := editableSection(root, "auth", "proxy")
		if err != nil {
			return err
		}
		setOrRemove(proxy, "login-url", strings.TrimSpace(in.Proxy.LoginURL))
		setOrRemove(proxy, "logout-url", strings.TrimSpace(in.Proxy.LogoutURL))
		setOrRemove(proxy, "user-header", strings.TrimSpace(in.Proxy.UserHeader))
		setOrRemove(proxy, "groups-header", strings.TrimSpace(in.Proxy.GroupsHeader))
		setListOrRemove(proxy, "allowed-users", in.Proxy.AllowedUsers)
		setListOrRemove(proxy, "allowed-groups", in.Proxy.AllowedGroups)
		return nil
	})
	if err != nil {
		writeSettingsWriteError(w, err)
		return
	}
	slog.Info("Sign-in settings saved from Settings", "proxy", in.Proxy.Enabled)
	writeJSON(w, http.StatusOK, map[string]any{"saved": true})
}

func trustedListCovers(entries []string, ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, e := range entries {
		if p := net.ParseIP(e); p != nil && p.Equal(parsed) {
			return true
		}
		if _, n, err := net.ParseCIDR(e); err == nil && n.Contains(parsed) {
			return true
		}
	}
	return false
}

// ---- Access ----

type settingsPageAccess struct {
	Index         int      `json:"index"`
	Title         string   `json:"title"`
	Slug          string   `json:"slug"`
	AllowedUsers  []string `json:"allowedUsers"`
	AllowedGroups []string `json:"allowedGroups"`
	Editable      bool     `json:"editable"`
}

func (a *application) handleSettingsAccessList(w http.ResponseWriter, r *http.Request) {
	doc, err := loadYAMLDocument(a.configPath)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not read the config")
		return
	}
	slots := expandPageSlots(doc, a.configPath, newEditorDocs())

	pages := []settingsPageAccess{}
	for i, slot := range slots {
		p := a.configPageForSlot(slot, i)
		if p == nil {
			continue
		}
		pages = append(pages, settingsPageAccess{
			Index: i, Title: p.Title, Slug: p.Slug,
			AllowedUsers: nonNil(p.AllowedUsers), AllowedGroups: nonNil(p.AllowedGroups),
			Editable: slot.doc != nil,
		})
	}

	groups := []string{householdRoleAdmin, householdRoleAdult, householdRoleKid, householdRoleGuest}
	users := []string{}
	for username, m := range a.Config.Household.Members {
		users = append(users, username)
		for _, g := range m.Groups {
			if !slices.Contains(groups, g) {
				groups = append(groups, g)
			}
		}
	}
	slices.Sort(users)

	writeJSON(w, http.StatusOK, map[string]any{
		"pages":       pages,
		"users":       users,
		"groups":      groups,
		"authEnabled": a.AnyAuthEnabled(),
	})
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func (a *application) handleSettingsAccessSave(w http.ResponseWriter, r *http.Request) {
	index, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid page")
		return
	}
	var in struct {
		AllowedUsers  []string `json:"allowedUsers"`
		AllowedGroups []string `json:"allowedGroups"`
	}
	if err := decodeSettingsBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	in.AllowedUsers = cleanList(in.AllowedUsers)
	in.AllowedGroups = cleanList(in.AllowedGroups)

	if (len(in.AllowedUsers) > 0 || len(in.AllowedGroups) > 0) && !a.AnyAuthEnabled() {
		writeJSONError(w, http.StatusBadRequest, "Set up sign-in first. Without it DashDen can't tell who's looking.")
		return
	}

	a.editorMu.Lock()
	defer a.editorMu.Unlock()

	mainDoc, err := loadYAMLDocument(a.configPath)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not read the config")
		return
	}
	slots := expandPageSlots(mainDoc, a.configPath, newEditorDocs())
	if index < 0 || index >= len(slots) || slots[index].doc == nil {
		writeJSONError(w, http.StatusNotFound, "page not found")
		return
	}
	slot := slots[index]
	p := a.configPageForSlot(slot, index)
	if p == nil {
		writeJSONError(w, http.StatusNotFound, "page not found")
		return
	}

	// Don't let the person saving lose sight of the page they're restricting.
	if user := a.settingsUser(w, r); user != nil && (len(in.AllowedUsers) > 0 || len(in.AllowedGroups) > 0) {
		check := &page{AllowedUsers: in.AllowedUsers, AllowedGroups: in.AllowedGroups}
		if !a.isUserAllowedOnPage(user, check) {
			writeSettingsWriteError(w, &settingsRefusal{"That would hide " + p.Title + " from you too. Include yourself or one of your groups."})
			return
		}
	}

	setListOrRemove(slot.node, "allowed-users", in.AllowedUsers)
	setListOrRemove(slot.node, "allowed-groups", in.AllowedGroups)
	if err := a.writeDocument(slot.path, slot.doc); err != nil {
		writeSettingsWriteError(w, err)
		return
	}
	slog.Info("Page access saved from Settings", "page", p.Title)
	writeJSON(w, http.StatusOK, map[string]any{"saved": p.Title})
}

// addHouseholdMemberFromSignIn gives someone a profile the first time they
// sign in through a provider that is set to create accounts.
func (a *application) addHouseholdMemberFromSignIn(username, name string) error {
	if a.configPath == "" {
		return errors.New("no config file to write to")
	}
	role := a.Config.Auth.OIDC.NewAccountRole
	if role == "" {
		role = householdRoleAdult
	}
	return a.changeConfig(func(root *yaml.Node) error {
		members, err := editableSection(root, "household", "members")
		if err != nil {
			return err
		}
		if getMappingValue(members, username) != nil {
			return nil
		}
		member := newMappingNode()
		setOrRemove(member, "name", strings.TrimSpace(name))
		setMappingKey(member, "role", scalarNode(role))
		setMappingKey(members, username, member)
		return nil
	})
}
