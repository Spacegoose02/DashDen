package dynacat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// The Apps tab: the household's own apps, each with a form to connect it
// (address, token, home id), a connection test, and a button that adds the
// right widget to a page. Addresses and tokens go in the secrets file under
// the names the DashDen-config repository already uses, and the widget
// refers to them as ${NAME}.

type appCatalogEntry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	Description string `json:"description"`
	TokenHint   string `json:"tokenHint"`
	NeedsHome   bool   `json:"needsHome"`
	Prefix      string `json:"prefix"`

	testPath string
	// den-app app name, or den-now source app name
	denApp string
	denNow bool
}

var appCatalog = []appCatalogEntry{
	{
		ID: "den-duty", Name: "Den Duty", Icon: "🧹", Prefix: "DENDUTY", NeedsHome: true,
		Description: "Today's chores for each kid, with Done and Approve buttons.",
		TokenHint:   "In Den Duty, open Settings, Home Assistant, and create a token with read and control access.",
		testPath:    "/api/homes/{home}/ha/today", denApp: denAppDenDuty,
	},
	{
		ID: "nighttime-ninja", Name: "Nighttime Ninja", Icon: "🥷", Prefix: "NINJA", NeedsHome: true,
		Description: "The shopping list, with an add box and tick-off.",
		TokenHint:   "An API token from Nighttime Ninja's settings.",
		testPath:    "/api/homes/{home}/shopping", denApp: denAppNighttimeNinja,
	},
	{
		ID: "wavelength", Name: "Wavelength", Icon: "🎵", Prefix: "WAVELENGTH",
		Description: "What each speaker is playing, with play, pause and skip.",
		TokenHint:   "The Home Assistant token from Wavelength's settings.",
		testPath:    "/api/ha/players", denApp: denAppWavelength,
	},
	{
		ID: "lootden", Name: "LootDen", Icon: "🎮", Prefix: "LOOTDEN",
		Description: "Who is playing what, in the Now in the Den list.",
		TokenHint:   "A personal token from your LootDen profile.",
		testPath:    "/api/family/now-playing", denNow: true, denApp: denNowLootDen,
	},
	{
		ID: "bingebandit", Name: "BingeBandit", Icon: "🎬", Prefix: "BINGEBANDIT",
		Description: "Who is watching what, in the Now in the Den list.",
		TokenHint:   "An admin API token from BingeBandit.",
		testPath:    "/api/admin/activity", denNow: true, denApp: denNowBingeBandit,
	},
}

func appByID(id string) (appCatalogEntry, bool) {
	for _, e := range appCatalog {
		if e.ID == id {
			return e, true
		}
	}
	return appCatalogEntry{}, false
}

func (e appCatalogEntry) urlVar() string   { return e.Prefix + "_URL" }
func (e appCatalogEntry) tokenVar() string { return e.Prefix + "_TOKEN" }
func (e appCatalogEntry) homeVar() string  { return e.Prefix + "_HOME_ID" }

func (e appCatalogEntry) vars() []string {
	if e.NeedsHome {
		return []string{e.urlVar(), e.tokenVar(), e.homeVar()}
	}
	return []string{e.urlVar(), e.tokenVar()}
}

func ref(name string) string { return "${" + name + "}" }

func (a *application) registerSettingsAppsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings/apps", a.settingsJSON(a.handleSettingsAppsList))
	mux.HandleFunc("POST /api/settings/apps/{id}/test", a.settingsJSON(a.handleSettingsAppTest))
	mux.HandleFunc("POST /api/settings/apps/{id}", a.settingsJSON(a.handleSettingsAppAdd))
	mux.HandleFunc("GET /api/settings/server", a.settingsJSON(a.handleSettingsServerGet))
	mux.HandleFunc("PUT /api/settings/server", a.settingsJSON(a.handleSettingsServerSave))
}

type settingsAppView struct {
	appCatalogEntry
	Configured map[string]bool `json:"configured"`
	OnPages    []string        `json:"onPages"`
}

type settingsColumnView struct {
	Index int    `json:"index"`
	Size  string `json:"size"`
}

type settingsPageColumns struct {
	Index   int                  `json:"index"`
	Title   string               `json:"title"`
	Columns []settingsColumnView `json:"columns"`
}

// appsInUse maps catalog ids to the pages that show them.
func (a *application) appsInUse() map[string][]string {
	inUse := map[string][]string{}
	add := func(id string, w widget) {
		p := a.widgetToPage[w.GetID()]
		title := ""
		if p != nil {
			title = p.Title
		}
		if !slices.Contains(inUse[id], title) {
			inUse[id] = append(inUse[id], title)
		}
	}
	for _, w := range a.widgetByID {
		switch x := w.(type) {
		case *denAppWidget:
			add(x.App, w)
		case *denNowWidget:
			for _, s := range x.Sources {
				add(s.App, w)
			}
		}
	}
	return inUse
}

func (a *application) settingsPageColumns() []settingsPageColumns {
	pages := []settingsPageColumns{}
	for i := range a.Config.Pages {
		p := &a.Config.Pages[i]
		pc := settingsPageColumns{Index: i, Title: p.Title, Columns: []settingsColumnView{}}
		for ci, c := range p.Columns {
			pc.Columns = append(pc.Columns, settingsColumnView{Index: ci, Size: c.Size})
		}
		pages = append(pages, pc)
	}
	return pages
}

func (a *application) handleSettingsAppsList(w http.ResponseWriter, r *http.Request) {
	inUse := a.appsInUse()
	apps := make([]settingsAppView, 0, len(appCatalog))
	for _, e := range appCatalog {
		v := settingsAppView{appCatalogEntry: e, Configured: map[string]bool{}, OnPages: inUse[e.denApp]}
		if v.OnPages == nil {
			v.OnPages = []string{}
		}
		for _, name := range e.vars() {
			v.Configured[name] = os.Getenv(name) != ""
		}
		apps = append(apps, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": apps, "pages": a.settingsPageColumns()})
}

type settingsAppInput struct {
	URL    string `json:"url"`
	Token  string `json:"token"`
	HomeID string `json:"homeId"`
	Page   int    `json:"page"`
	Column int    `json:"column"`
}

// resolve fills blank fields from what is already set, and checks the result.
func (in *settingsAppInput) resolve(e appCatalogEntry) error {
	in.URL = strings.TrimRight(strings.TrimSpace(in.URL), "/")
	in.Token = strings.TrimSpace(in.Token)
	in.HomeID = strings.TrimSpace(in.HomeID)
	if in.URL == "" {
		in.URL = strings.TrimRight(os.Getenv(e.urlVar()), "/")
	}
	if in.Token == "" {
		in.Token = os.Getenv(e.tokenVar())
	}
	if e.NeedsHome && in.HomeID == "" {
		in.HomeID = os.Getenv(e.homeVar())
	}

	u, err := url.Parse(in.URL)
	if in.URL == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("Enter the app's address, like https://chores.example.com.")
	}
	if in.Token == "" {
		return errors.New("Enter a token.")
	}
	if e.NeedsHome && in.HomeID == "" {
		return errors.New("Enter the home id.")
	}
	return nil
}

func (a *application) handleSettingsAppTest(w http.ResponseWriter, r *http.Request) {
	e, ok := appByID(r.PathValue("id"))
	if !ok {
		writeJSONError(w, http.StatusNotFound, "unknown app")
		return
	}
	var in settingsAppInput
	if err := decodeSettingsBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := in.resolve(e); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	path := strings.ReplaceAll(e.testPath, "{home}", url.PathEscape(in.HomeID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, in.URL+path, nil)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "That address doesn't work.")
		return
	}
	req.Header.Set("Authorization", "Bearer "+in.Token)
	req.Header.Set("Accept", "application/json")
	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "message": "Couldn't reach " + e.Name + " at that address."})
		return
	}
	resp.Body.Close()

	result := map[string]any{"ok": resp.StatusCode < 300, "status": resp.StatusCode}
	switch {
	case resp.StatusCode < 300:
		result["message"] = "Connected to " + e.Name + "."
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		result["message"] = e.Name + " didn't accept the token."
	case resp.StatusCode == http.StatusNotFound && e.NeedsHome:
		result["message"] = e.Name + " answered, but couldn't find that home id."
	default:
		result["message"] = fmt.Sprintf("%s answered with an error (%d).", e.Name, resp.StatusCode)
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *application) handleSettingsAppAdd(w http.ResponseWriter, r *http.Request) {
	e, ok := appByID(r.PathValue("id"))
	if !ok {
		writeJSONError(w, http.StatusNotFound, "unknown app")
		return
	}
	var in settingsAppInput
	if err := decodeSettingsBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	given := in
	if err := in.resolve(e); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Save what was typed; values already set elsewhere are left alone.
	for name, value := range map[string]string{e.urlVar(): strings.TrimSpace(given.URL), e.tokenVar(): strings.TrimSpace(given.Token), e.homeVar(): strings.TrimSpace(given.HomeID)} {
		if value == "" {
			continue
		}
		if name == e.urlVar() {
			value = in.URL
		}
		if err := validateSecret(name, value); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := setManagedSecret(a.configPath, name, &value); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "could not save "+name+": "+err.Error())
			return
		}
	}

	a.editorMu.Lock()
	defer a.editorMu.Unlock()

	mainDoc, err := loadYAMLDocument(a.configPath)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not read the config")
		return
	}
	slots := expandPageSlots(mainDoc, a.configPath, newEditorDocs())
	if in.Page < 0 || in.Page >= len(slots) || slots[in.Page].doc == nil {
		writeJSONError(w, http.StatusBadRequest, "Pick a page.")
		return
	}
	slot := slots[in.Page]
	columns := getMappingValue(slot.node, "columns")
	if columns == nil || columns.Kind != yaml.SequenceNode || in.Column < 0 || in.Column >= len(columns.Content) {
		writeJSONError(w, http.StatusBadRequest, "Pick a column on that page. Columns kept in an included file can't be changed from here.")
		return
	}
	column := columns.Content[in.Column]
	widgetsNode := getMappingValue(column, "widgets")
	if widgetsNode == nil {
		widgetsNode = sequenceNode()
		setMappingKey(column, "widgets", widgetsNode)
	}
	if widgetsNode.Kind != yaml.SequenceNode {
		writeJSONError(w, http.StatusBadRequest, "That column's widgets are kept in an included file; add it there or pick another column.")
		return
	}

	added := "widget"
	if e.denNow {
		source := newMappingNode()
		addPair(source, "app", scalarNode(e.denApp))
		addPair(source, "url", scalarNode(ref(e.urlVar())))
		addPair(source, "token", scalarNode(ref(e.tokenVar())))
		if existing := findDenNowNode(widgetsNode); existing != nil {
			sources := getMappingValue(existing, "sources")
			sources.Content = append(sources.Content, source)
			added = "source"
		} else {
			node := newMappingNode()
			addPair(node, "type", scalarNode("den-now"))
			sources := sequenceNode()
			sources.Content = append(sources.Content, source)
			addPair(node, "sources", sources)
			widgetsNode.Content = append(widgetsNode.Content, node)
		}
	} else {
		node := newMappingNode()
		addPair(node, "type", scalarNode("den-app"))
		addPair(node, "app", scalarNode(e.denApp))
		addPair(node, "url", scalarNode(ref(e.urlVar())))
		addPair(node, "token", scalarNode(ref(e.tokenVar())))
		if e.NeedsHome {
			addPair(node, "home-id", scalarNode(ref(e.homeVar())))
		}
		if e.ID == "den-duty" && len(a.Config.Household.Members) > 0 {
			addPair(node, "approve-groups", stringListNode([]string{householdRoleAdult}))
		}
		widgetsNode.Content = append(widgetsNode.Content, node)
	}
	setBlockStyleDeep(widgetsNode)

	if err := a.writeDocument(slot.path, slot.doc); err != nil {
		writeSettingsWriteError(w, err)
		return
	}
	slog.Info("App added from Settings", "app", e.ID, "page", in.Page, "column", in.Column)
	writeJSON(w, http.StatusOK, map[string]any{"added": added})
}

func findDenNowNode(widgetsNode *yaml.Node) *yaml.Node {
	for _, item := range widgetsNode.Content {
		if item.Kind != yaml.MappingNode || scalarValue(getMappingValue(item, "type")) != "den-now" {
			continue
		}
		if s := getMappingValue(item, "sources"); s != nil && s.Kind == yaml.SequenceNode {
			return item
		}
	}
	return nil
}

// ---- Server ----

type settingsServer struct {
	Port          int  `json:"port"`
	Proxied       bool `json:"proxied"`
	AllowEditing  bool `json:"allowEditing"`
	DisablePicker bool `json:"disablePicker"`
}

func (a *application) hasHouseholdAdmin() bool {
	for _, m := range a.Config.Household.Members {
		if m.Role == householdRoleAdmin {
			return true
		}
	}
	return false
}

func (a *application) handleSettingsServerGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"settings": settingsServer{
			Port:          int(a.Config.Server.Port),
			Proxied:       a.Config.Server.Proxied,
			AllowEditing:  a.Config.Server.AllowEditing,
			DisablePicker: a.Config.Theme.DisablePicker,
		},
		"hasAdmins": a.hasHouseholdAdmin(),
	})
}

func (a *application) handleSettingsServerSave(w http.ResponseWriter, r *http.Request) {
	var in settingsServer
	if err := decodeSettingsBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if in.Port < 1 || in.Port > 65535 {
		writeJSONError(w, http.StatusBadRequest, "The port must be between 1 and 65535.")
		return
	}
	if !in.AllowEditing && !a.hasHouseholdAdmin() && !inRecovery(r) {
		writeSettingsWriteError(w, &settingsRefusal{"Without a household admin, turning off editing would also close Settings. Make someone an admin on the People tab first."})
		return
	}

	err := a.changeConfig(func(root *yaml.Node) error {
		server, err := editableSection(root, "server")
		if err != nil {
			return err
		}
		if in.Port == 8080 && getMappingValue(server, "port") == nil {
			// Leave the default implicit.
		} else {
			setMappingKey(server, "port", scalarNode(strconv.Itoa(in.Port)))
		}
		setBoolOrRemove(server, "proxied", in.Proxied)
		// allow-editing defaults to true, so only false is written.
		if in.AllowEditing {
			removeMappingKey(server, "allow-editing")
		} else {
			setMappingKey(server, "allow-editing", scalarNode("false"))
		}
		theme, err := editableSection(root, "theme")
		if err != nil {
			return err
		}
		setBoolOrRemove(theme, "disable-picker", in.DisablePicker)
		pruneEmptySection(root, "theme")
		pruneEmptySection(root, "server")
		return nil
	})
	if err != nil {
		writeSettingsWriteError(w, err)
		return
	}
	slog.Info("Server settings saved from Settings", "port", in.Port, "proxied", in.Proxied)
	writeJSON(w, http.StatusOK, map[string]any{"saved": true, "portChanged": in.Port != int(a.Config.Server.Port)})
}

func setBoolOrRemove(m *yaml.Node, key string, v bool) {
	if v {
		setMappingKey(m, key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
		return
	}
	removeMappingKey(m, key)
}
