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
			if _, ok := cfg.Theme.Presets.Get(m.Theme); !ok {
				return fmt.Errorf("household member %q: theme %q is not a preset", username, m.Theme)
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

	m, ok := a.Config.Household.Members[u.Username]
	if !ok {
		return u
	}

	enriched := *u
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
