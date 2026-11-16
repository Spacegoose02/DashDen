package dynacat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

var denAppWidgetTemplate = mustParseTemplate("den-app.html", "widget-base.html")

// denAppWidget shows one of the household's own apps with buttons that act on
// it: ticking off or approving Den Duty chores and parent to-dos, adding to and
// checking off the Nighttime Ninja shopping list (or adding what's running
// low), play/pause/skip on Wavelength devices, answering LootDen's "more game
// time" asks, and posting quick messages to a Dendrop channel.
type denAppWidget struct {
	widgetBase    `yaml:",inline"`
	App           string   `yaml:"app"`
	View          string   `yaml:"view"`
	Channel       string   `yaml:"channel"`
	Messages      []string `yaml:"messages"`
	URL           string   `yaml:"url"`
	Token         string   `yaml:"token"`
	HomeID        string   `yaml:"home-id"`
	Kid           string   `yaml:"kid"`
	ActionGroups  []string `yaml:"action-groups"`
	ApproveGroups []string `yaml:"approve-groups"`
	AllowInsecure bool     `yaml:"allow-insecure"`
	CollapseAfter int      `yaml:"collapse-after"`

	Kids     []denDutyKid         `yaml:"-"`
	Shopping []ninjaShoppingRow   `yaml:"-"`
	Players  []wavelengthPlayer   `yaml:"-"`
	Duties   []denDutyParentCard  `yaml:"-"`
	Low      []ninjaLowItem       `yaml:"-"`
	TimeAsks []lootdenTimeRequest `yaml:"-"`
	Chat     []dendropMessage     `yaml:"-"`
	baseURL  string               `yaml:"-"`
}

const (
	denAppDenDuty        = "den-duty"
	denAppNighttimeNinja = "nighttime-ninja"
	denAppWavelength     = "wavelength"
	denAppLootDen        = "lootden"
	denAppDendrop        = "dendrop"

	denViewParentDuty = "parent-duty"
	denViewLow        = "low"
)

var denAppNames = []string{denAppDenDuty, denAppNighttimeNinja, denAppWavelength, denAppLootDen, denAppDendrop}

// denAppViews lists the views each app has besides its default one.
var denAppViews = map[string][]string{denAppDenDuty: {denViewParentDuty}, denAppNighttimeNinja: {denViewLow}}

type denDutyKid struct {
	Kid    string         `json:"kid"`
	KidID  string         `json:"kid_id"`
	Chores []denDutyChore `json:"chores"`
}

type denDutyChore struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Icon   string `json:"icon"`
	Points int    `json:"points"`
	Status string `json:"status"`
}

type ninjaShoppingRow struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Quantity *float64 `json:"quantity"`
	Unit     string   `json:"unit"`
	Checked  bool     `json:"checked"`
}

func (r ninjaShoppingRow) Label() string {
	parts := make([]string, 0, 3)
	if r.Quantity != nil {
		parts = append(parts, strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.2f", *r.Quantity), "0"), "."))
	}
	if r.Unit != "" {
		parts = append(parts, r.Unit)
	}
	return strings.Join(append(parts, r.Name), " ")
}

type wavelengthPlayer struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	State    string  `json:"state"`
	Title    string  `json:"title"`
	Artist   string  `json:"artist"`
	Album    string  `json:"album"`
	Position float64 `json:"position"`
	Duration float64 `json:"duration"`
}

type wavelengthPlayersResponse struct {
	Players []wavelengthPlayer `json:"players"`
}

func (p wavelengthPlayer) Percent() int {
	if p.Duration <= 0 {
		return 0
	}
	return int(min(100, p.Position*100/p.Duration))
}

func (widget *denAppWidget) initialize() error {
	if !slices.Contains(denAppNames, widget.App) {
		return fmt.Errorf("app must be one of %s", strings.Join(denAppNames, ", "))
	}
	if widget.URL == "" || widget.Token == "" {
		return errors.New("url and token are required")
	}
	if (widget.App == denAppDenDuty || widget.App == denAppNighttimeNinja) && widget.HomeID == "" {
		return errors.New("home-id is required for " + widget.App)
	}
	if widget.View != "" && !slices.Contains(denAppViews[widget.App], widget.View) {
		if len(denAppViews[widget.App]) == 0 {
			return errors.New(widget.App + " has no views")
		}
		return fmt.Errorf("view must be one of %s for %s", strings.Join(denAppViews[widget.App], ", "), widget.App)
	}
	if widget.App == denAppDendrop {
		if widget.Channel == "" {
			return errors.New("channel is required for dendrop")
		}
		widget.Messages = slices.DeleteFunc(widget.Messages, func(m string) bool { return strings.TrimSpace(m) == "" })
		if len(widget.Messages) == 0 {
			widget.Messages = []string{"Dinner's ready! 🍽️"}
		}
	}

	widget.baseURL = strings.TrimRight(widget.URL, "/")
	if widget.CollapseAfter == 0 {
		widget.CollapseAfter = 8
	}

	titles := map[string]string{
		denAppDenDuty: "Chores", denAppNighttimeNinja: "Shopping list", denAppWavelength: "Wavelength",
		denAppLootDen: "More game time?", denAppDendrop: "Family chat",
		denAppDenDuty + "/" + denViewParentDuty: "Parent to-dos", denAppNighttimeNinja + "/" + denViewLow: "Running low",
	}
	key := widget.App
	if widget.View != "" {
		key += "/" + widget.View
	}
	widget.withTitle(titles[key]).withTitleURL(widget.baseURL).withCacheDuration(time.Minute)
	if widget.App == denAppWavelength {
		widget.withCacheDuration(15 * time.Second)
	}

	return nil
}

func (widget *denAppWidget) client() requestDoer {
	return ternary[requestDoer](widget.AllowInsecure, defaultInsecureHTTPClient, defaultHTTPClient)
}

func (widget *denAppWidget) newRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, widget.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	// Dendrop's bot API takes "Bot <token>"; the other apps take a bearer token.
	req.Header.Set("Authorization", ternary(widget.App == denAppDendrop, "Bot ", "Bearer ")+widget.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (widget *denAppWidget) homePath(rest string) string {
	return "/api/homes/" + url.PathEscape(widget.HomeID) + rest
}

func (widget *denAppWidget) update(ctx context.Context) {
	var err error

	switch {
	case widget.View != "" || widget.App == denAppLootDen || widget.App == denAppDendrop:
		err = widget.updateMore(ctx)
	case widget.App == denAppDenDuty:
		var kids []denDutyKid
		kids, err = fetchDenApp[[]denDutyKid](ctx, widget, widget.homePath("/ha/today"))
		if err == nil && widget.Kid != "" {
			kids = slices.DeleteFunc(kids, func(k denDutyKid) bool { return !strings.EqualFold(k.Kid, widget.Kid) })
		}
		if err == nil {
			widget.Kids = kids
		}
	case widget.App == denAppNighttimeNinja:
		var rows []ninjaShoppingRow
		rows, err = fetchDenApp[[]ninjaShoppingRow](ctx, widget, widget.homePath("/shopping"))
		if err == nil {
			widget.Shopping = slices.DeleteFunc(rows, func(r ninjaShoppingRow) bool { return r.Checked })
		}
	case widget.App == denAppWavelength:
		var res wavelengthPlayersResponse
		res, err = fetchDenApp[wavelengthPlayersResponse](ctx, widget, "/api/ha/players")
		if err == nil {
			widget.Players = res.Players
		}
	}

	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}
}

func fetchDenApp[T any](ctx context.Context, widget *denAppWidget, path string) (T, error) {
	var zero T
	// The apps change underneath us (and right after a button press), so skip
	// the shared response cache; identical in-flight requests are still merged.
	req, err := widget.newRequest(withSharedFetchMaxAge(ctx, time.Nanosecond), http.MethodGet, path, nil)
	if err != nil {
		return zero, err
	}
	return decodeJsonFromRequest[T](widget.client(), req)
}

func (widget *denAppWidget) Render() template.HTML {
	return widget.renderTemplate(widget, denAppWidgetTemplate)
}

// userInGroups is true when no groups are listed, or the user has one of them.
func userInGroups(user *authenticatedUser, groups []string) bool {
	if len(groups) == 0 {
		return true
	}
	if user == nil {
		return false
	}
	for _, g := range groups {
		if slices.Contains(user.Groups, g) || g == user.Username {
			return true
		}
	}
	return false
}

func (widget *denAppWidget) currentUser(w http.ResponseWriter, r *http.Request) *authenticatedUser {
	if widget.Providers == nil || widget.Providers.app == nil {
		return nil
	}
	return widget.Providers.app.getAuthenticatedUser(w, r)
}

func (widget *denAppWidget) handleRequest(w http.ResponseWriter, r *http.Request) {
	action := strings.Trim(r.PathValue("action"), "/")
	user := widget.currentUser(w, r)

	if !userInGroups(user, widget.ActionGroups) {
		http.Error(w, "you can't change this from here", http.StatusForbidden)
		return
	}

	// A cross-site form can't send JSON without a preflight, so this also
	// stops other sites from pressing these buttons for a signed-in visitor.
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		http.Error(w, "expected application/json", http.StatusUnsupportedMediaType)
		return
	}

	var body map[string]string
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	method, path, payload, err := widget.planAction(action, body, user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	req, err := widget.newRequest(ctx, method, path, payload)
	if err != nil {
		http.Error(w, "could not build the request", http.StatusInternalServerError)
		return
	}
	status, respBody, err := fetchRequestBody(widget.client(), req)
	if err != nil {
		http.Error(w, "the app didn't answer", http.StatusBadGateway)
		return
	}
	if status >= 300 {
		msg, _ := limitStringLength(string(respBody), 200)
		http.Error(w, fmt.Sprintf("%s said %d: %s", widget.App, status, msg), http.StatusBadGateway)
		return
	}

	widget.update(ctx)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(widget.Render()))
}

// planAction turns a button press into the app's own API call.
func (widget *denAppWidget) planAction(action string, body map[string]string, user *authenticatedUser) (string, string, any, error) {
	if widget.View != "" || widget.App == denAppLootDen || widget.App == denAppDendrop {
		return widget.planMoreAction(action, body, user)
	}
	switch widget.App {
	case denAppDenDuty:
		if action != "complete" && action != "approve" {
			break
		}
		if action == "approve" && !userInGroups(user, widget.ApproveGroups) {
			return "", "", nil, errors.New("only grown-ups can approve chores")
		}
		if body["kid"] == "" || body["chore"] == "" {
			return "", "", nil, errors.New("kid and chore are required")
		}
		return http.MethodPost, widget.homePath("/ha/" + action), map[string]string{"kid": body["kid"], "chore": body["chore"]}, nil

	case denAppNighttimeNinja:
		switch action {
		case "add":
			text := strings.TrimSpace(body["text"])
			if text == "" || len(text) > 500 {
				return "", "", nil, errors.New("type what to add")
			}
			return http.MethodPost, widget.homePath("/shopping"), map[string]string{"text": text}, nil
		case "check":
			if body["id"] == "" {
				return "", "", nil, errors.New("id is required")
			}
			return http.MethodPatch, widget.homePath("/shopping/" + url.PathEscape(body["id"])), map[string]bool{"checked": true}, nil
		}

	case denAppWavelength:
		if action != "control" {
			break
		}
		op := body["op"]
		if !slices.Contains([]string{"toggle", "next", "prev"}, op) || body["id"] == "" {
			return "", "", nil, errors.New("unknown control")
		}
		return http.MethodPost, "/api/ha/players/" + url.PathEscape(body["id"]), map[string]string{"action": op}, nil
	}

	return "", "", nil, fmt.Errorf("unknown action %q", action)
}
