package dynacat

import (
	"fmt"
	"slices"
)

// widgetAccess limits who sees a widget. It's set with allowed-users and
// allowed-groups on a widget placed directly in a column or in head-widgets,
// and covers everything inside it, such as the widgets of a group.
//
// The check runs on the server wherever widget HTML or data leaves it: the
// page content, a single widget's content, widget actions, live updates and
// the read API. A hidden widget is simply not there for that person.
type widgetAccess struct {
	AllowedUsers  []string `yaml:"allowed-users"`
	AllowedGroups []string `yaml:"allowed-groups"`
}

func (w *widgetAccess) restricted() bool {
	return w != nil && (len(w.AllowedUsers) > 0 || len(w.AllowedGroups) > 0)
}

func (w *widgetAccess) allows(user *authenticatedUser) bool {
	if !w.restricted() {
		return true
	}
	if user == nil {
		return false
	}
	if slices.Contains(w.AllowedUsers, user.Username) {
		return true
	}
	for _, g := range w.AllowedGroups {
		if slices.Contains(user.Groups, g) {
			return true
		}
	}
	return false
}

func (w *widgetBase) access() *widgetAccess {
	if !w.widgetAccess.restricted() {
		return nil
	}
	return &w.widgetAccess
}

// canUserSeeWidget looks up the rules that apply to a widget by id, including
// ones inherited from the group it sits in.
func (a *application) canUserSeeWidget(user *authenticatedUser, id uint64) bool {
	rules, ok := a.widgetAccessByID[id]
	return !ok || rules.allows(user)
}

// CanSeeWidget is used by the page content template.
func (d templateData) CanSeeWidget(w widget) bool {
	if d.App == nil {
		return true
	}
	return d.App.canUserSeeWidget(d.AuthUser, w.GetID())
}

func checkNestedWidgetAccess(children widgets, container string) error {
	for _, child := range children {
		if child.access() != nil {
			return fmt.Errorf("allowed-users and allowed-groups only work on widgets placed directly in a column; put them on the %s instead", container)
		}
	}
	return nil
}

func checkWidgetAccessNeedsAuth(p *page, hasAuth bool, pageNumber int) error {
	if hasAuth {
		return nil
	}
	all := append(widgets{}, p.HeadWidgets...)
	for _, c := range p.Columns {
		all = append(all, c.Widgets...)
	}
	for _, w := range all {
		if w.access() != nil {
			return fmt.Errorf("page %d has a widget with allowed-users/allowed-groups but no auth method is configured", pageNumber)
		}
	}
	return nil
}
