package dynacat

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
)

// householdMember is a person's profile, keyed by the username their sign-in
// method reports (the Tinyauth username, OIDC preferred_username or a local user).
type householdMember struct {
	Name   string   `yaml:"name"`
	Avatar string   `yaml:"avatar"`
	Role   string   `yaml:"role"`
	Home   string   `yaml:"home"`
	Theme  string   `yaml:"theme"`
	Groups []string `yaml:"groups"`
	// LinkedAccounts are other sign-in names (or email addresses) that belong
	// to this person, such as their Tinyauth username or a second local
	// account. Signing in as any of them signs in as this person.
	LinkedAccounts []string `yaml:"linked-accounts"`
}

const (
	householdRoleAdmin = "admin"
	householdRoleAdult = "adult"
	householdRoleKid   = "kid"
	householdRoleGuest = "guest"
)

var householdRoles = []string{householdRoleAdmin, householdRoleAdult, householdRoleKid, householdRoleGuest}

// AvatarIsImage tells templates whether the avatar is a picture or a short
// piece of text such as an emoji or initials.
func (m *householdMember) AvatarIsImage() bool {
	a := m.Avatar
	return strings.HasPrefix(a, "/") || strings.HasPrefix(a, "http://") || strings.HasPrefix(a, "https://")
}

func validateHousehold(cfg *config) error {
	linkedTo := map[string]string{}
	for username, m := range cfg.Household.Members {
		if m == nil {
			continue
		}
		for _, link := range m.LinkedAccounts {
			key := strings.ToLower(strings.TrimSpace(link))
			if key == "" {
				return fmt.Errorf("household member %q has an empty linked account", username)
			}
			if other, ok := linkedTo[key]; ok && other != username {
				return fmt.Errorf("linked account %q belongs to both %q and %q", link, other, username)
			}
			linkedTo[key] = username
		}
	}
	for username := range cfg.Household.Members {
		if other, ok := linkedTo[strings.ToLower(username)]; ok {
			if other == username {
				return fmt.Errorf("household member %q is linked to their own username", username)
			}
			return fmt.Errorf("%q has a household profile of their own and is also linked to %q; remove one of the two", username, other)
		}
	}
	for username, m := range cfg.Household.Members {
		if m == nil {
			return fmt.Errorf("household member %q has no settings", username)
		}
		if strings.TrimSpace(username) == "" {
			return fmt.Errorf("household member has no username")
		}
		if m.Role != "" && !slices.Contains(householdRoles, m.Role) {
			return fmt.Errorf("household member %q: role must be one of %s, got %q", username, strings.Join(householdRoles, ", "), m.Role)
		}
		if m.Theme != "" && m.Theme != "default" {
			_, ok := cfg.Theme.Presets.Get(m.Theme)
			// DashDen: built-in themes are added after the config loads.
			ok = ok || (isBuiltinThemeKey(m.Theme) && (cfg.Theme.BuiltinThemes == nil || *cfg.Theme.BuiltinThemes))
			if !ok {
				return fmt.Errorf("household member %q: theme %q is not a preset or built-in theme", username, m.Theme)
			}
		}
	}
	return nil
}

// checkHouseholdHomePages runs once page slugs are final.
func (a *application) checkHouseholdHomePages() error {
	for username, m := range a.Config.Household.Members {
		if m.Home == "" {
			continue
		}
		if _, ok := a.slugToPage[m.Home]; !ok {
			return fmt.Errorf("household member %q: home %q is not a page slug", username, m.Home)
		}
	}
	return nil
}

func (a *application) prepareHousehold() {
	for _, m := range a.Config.Household.Members {
		if m.AvatarIsImage() {
			m.Avatar = a.resolveUserDefinedAssetPath(m.Avatar)
		}
	}
}

// Member returns the household profile of a signed-in person, or nil.
func (u *authenticatedUser) Member() *householdMember {
	return u.member
}

// IsAdmin reports whether the person has the admin role.
func (u *authenticatedUser) IsAdmin() bool {
	return u != nil && u.member != nil && u.member.Role == householdRoleAdmin
}

// withHousehold attaches the person's profile, fills in their name when the
// sign-in method didn't send one, and adds their role and extra groups so that
// allowed-groups works even when the sign-in method has no groups of its own.
func (a *application) withHousehold(u *authenticatedUser) *authenticatedUser {
	if u == nil {
		return nil
	}

	username, link, m := a.findMemberForSignIn(u.Username, u.Email)
	if m == nil {
		if u.IsProxy || u.IsOIDC {
			recentUnlinkedSignIns.record(u)
		}
		return u
	}

	enriched := *u
	// Sign-in names ignore capital letters; use the spelling the household has.
	enriched.Username = username
	enriched.SignInName = u.Username
	enriched.LinkedBy = link
	enriched.member = m
	if m.Name != "" {
		enriched.Name = m.Name
	}

	groups := slices.Clone(u.Groups)
	extra := m.Groups
	if m.Role != "" {
		extra = append([]string{m.Role}, extra...)
		// Admins count as adults everywhere an adult is allowed.
		if m.Role == householdRoleAdmin {
			extra = append(extra, householdRoleAdult)
		}
	}
	for _, g := range extra {
		if !slices.Contains(groups, g) {
			groups = append(groups, g)
		}
	}
	enriched.Groups = groups

	return &enriched
}

// redirectToHomePage sends a signed-in person from the root address to their
// own home page. Returns true when it redirected.
func (a *application) redirectToHomePage(w http.ResponseWriter, r *http.Request, u *authenticatedUser) bool {
	if u == nil || u.member == nil || u.member.Home == "" || r.PathValue("page") != "" {
		return false
	}

	home, ok := a.slugToPage[u.member.Home]
	if !ok || home == a.slugToPage[""] || !a.canUserAccessPage(u, home) {
		return false
	}

	target := a.Config.Server.BaseURL + "/" + home.Slug
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
	return true
}

// findHouseholdMember looks a username up without caring about capital letters
// and returns the name as the household spells it.
func (a *application) findHouseholdMember(name string) (string, *householdMember) {
	if m, ok := a.Config.Household.Members[name]; ok {
		return name, m
	}
	for configured, m := range a.Config.Household.Members {
		if strings.EqualFold(configured, name) {
			return configured, m
		}
	}
	return "", nil
}

// findMemberForSignIn finds the profile for a sign-in: the member with that
// username, otherwise the member who has the username, then the email
// address, among their linked accounts. link is the linked account that
// matched, empty for a direct match.
func (a *application) findMemberForSignIn(username, email string) (key, link string, m *householdMember) {
	if key, m := a.findHouseholdMember(username); m != nil {
		return key, "", m
	}
	for _, name := range []string{username, email} {
		if name = strings.TrimSpace(name); name == "" {
			continue
		}
		if key, link, m := a.findLinkedMember(name); m != nil {
			return key, link, m
		}
	}
	return "", "", nil
}

// findLinkedMember returns the member who lists name among their linked
// accounts, ignoring capital letters.
func (a *application) findLinkedMember(name string) (key, link string, m *householdMember) {
	for configured, member := range a.Config.Household.Members {
		if member == nil {
			continue
		}
		for _, l := range member.LinkedAccounts {
			if strings.EqualFold(strings.TrimSpace(l), name) {
				return configured, l, member
			}
		}
	}
	return "", "", nil
}
