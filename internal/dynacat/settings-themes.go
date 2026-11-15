package dynacat

import (
	"log/slog"
	"net/http"
	"slices"

	"gopkg.in/yaml.v3"
)

// The Den theme pack: presets anyone can add from Settings, Themes. They're
// copied into theme.presets, so they can be edited like any other preset.

type packTheme struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Background  string `json:"background"`
	Primary     string `json:"primary"`
	Positive    string `json:"positive"`
	Negative    string `json:"negative"`
	Light       bool   `json:"light"`
	Contrast    string `json:"contrast"`
	TextSat     string `json:"textSaturation"`
	Season      string `json:"season"`
	SeasonFrom  string `json:"seasonFrom"`
	SeasonTo    string `json:"seasonTo"`
}

var denThemePack = []packTheme{
	{Key: "den-night", Name: "Den Night", Description: "Deep blue with a violet glow.", Background: "225 20 10", Primary: "265 80 70", Positive: "150 50 55", Negative: "0 70 65", Contrast: "1.1"},
	{Key: "campfire", Name: "Campfire", Description: "Warm embers for evenings.", Background: "20 18 9", Primary: "28 90 60", Positive: "90 45 55", Negative: "0 75 65", Contrast: "1.1", TextSat: "0.9"},
	{Key: "forest", Name: "Forest", Description: "Mossy greens.", Background: "150 15 9", Primary: "140 45 55", Positive: "90 50 55", Negative: "10 70 62"},
	{Key: "ocean", Name: "Ocean", Description: "Cool teal and navy.", Background: "210 30 11", Primary: "195 80 55", Positive: "165 55 50", Negative: "355 70 65"},
	{Key: "candy", Name: "Candy", Description: "Bright and playful, for the kids.", Background: "300 25 12", Primary: "320 85 70", Positive: "180 70 55", Negative: "10 85 65", TextSat: "1.2"},
	{Key: "paper", Name: "Paper", Description: "Light and calm, easy on a kitchen tablet in daylight.", Background: "40 30 95", Primary: "210 70 40", Positive: "150 55 35", Negative: "0 65 50", Light: true, Contrast: "1.2", TextSat: "0.6"},
	{Key: "sunny", Name: "Sunny", Description: "A cheerful light theme.", Background: "50 80 94", Primary: "25 90 45", Positive: "140 60 35", Negative: "0 70 45", Light: true, Contrast: "1.3"},
	{Key: "high-contrast", Name: "High Contrast", Description: "Black with bold colors, for readability.", Background: "0 0 3", Primary: "55 100 60", Positive: "120 80 55", Negative: "0 90 65", Contrast: "1.6"},
	{Key: "spooky", Name: "Spooky", Description: "Pumpkin orange on midnight purple.", Background: "270 20 8", Primary: "28 95 55", Positive: "100 60 50", Negative: "0 80 60", Season: "Halloween", SeasonFrom: "10-15", SeasonTo: "10-31"},
	{Key: "winter-holidays", Name: "Winter Holidays", Description: "Berry red and pine green on a frosty night.", Background: "210 25 10", Primary: "0 70 58", Positive: "140 55 45", Negative: "0 80 65", Season: "December", SeasonFrom: "12-01", SeasonTo: "12-31"},
	{Key: "spring", Name: "Spring", Description: "Blossom pink on fresh green, light.", Background: "100 40 95", Primary: "330 60 50", Positive: "140 50 40", Negative: "0 60 50", Light: true, Contrast: "1.2", Season: "Spring", SeasonFrom: "03-20", SeasonTo: "04-20"},
}

func (t packTheme) node() *yaml.Node {
	n := newMappingNode()
	addPair(n, "background-color", scalarNode(t.Background))
	addPair(n, "primary-color", scalarNode(t.Primary))
	addPair(n, "positive-color", scalarNode(t.Positive))
	addPair(n, "negative-color", scalarNode(t.Negative))
	if t.Light {
		addPair(n, "light", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
	}
	if t.Contrast != "" {
		addPair(n, "contrast-multiplier", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: t.Contrast})
	}
	if t.TextSat != "" {
		addPair(n, "text-saturation-multiplier", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: t.TextSat})
	}
	return n
}

func (a *application) registerSettingsThemesRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings/themes", a.settingsJSON(a.handleSettingsThemesGet))
	mux.HandleFunc("POST /api/settings/themes/pack", a.settingsJSON(a.handleSettingsThemePackAdd))
	mux.HandleFunc("PUT /api/settings/themes/seasonal", a.settingsJSON(a.handleSettingsSeasonalSave))
}

type settingsPackTheme struct {
	packTheme
	Installed bool `json:"installed"`
}

type settingsSeason struct {
	Preset string `json:"preset"`
	From   string `json:"from"`
	To     string `json:"to"`
}

func (a *application) handleSettingsThemesGet(w http.ResponseWriter, r *http.Request) {
	pack := make([]settingsPackTheme, 0, len(denThemePack))
	for _, t := range denThemePack {
		_, installed := a.Config.Theme.Presets.Get(t.Key)
		pack = append(pack, settingsPackTheme{packTheme: t, Installed: installed})
	}
	presets := []string{}
	for key := range a.Config.Theme.Presets.Items() {
		presets = append(presets, key)
	}
	seasons := []settingsSeason{}
	for _, s := range a.Config.Theme.Seasonal {
		seasons = append(seasons, settingsSeason{Preset: s.Preset, From: s.From, To: s.To})
	}
	current := ""
	if s := a.seasonalThemeAt(timeNow()); s != nil {
		current = s.Key
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pack": pack, "presets": presets, "seasonal": seasons, "currentSeason": current,
		"pickerDisabled": a.Config.Theme.DisablePicker,
	})
}

func (a *application) handleSettingsThemePackAdd(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Keys []string `json:"keys"`
	}
	if err := decodeSettingsBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	added := []string{}
	err := a.changeConfig(func(root *yaml.Node) error {
		presets, err := editableSection(root, "theme", "presets")
		if err != nil {
			return err
		}
		for _, t := range denThemePack {
			if !slices.Contains(in.Keys, t.Key) || getMappingValue(presets, t.Key) != nil {
				continue
			}
			addPair(presets, t.Key, t.node())
			added = append(added, t.Key)
		}
		return nil
	})
	if err != nil {
		writeSettingsWriteError(w, err)
		return
	}
	slog.Info("Theme pack presets added from Settings", "presets", added)
	writeJSON(w, http.StatusOK, map[string]any{"added": added})
}

func (a *application) handleSettingsSeasonalSave(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Seasonal []settingsSeason `json:"seasonal"`
	}
	if err := decodeSettingsBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	for _, s := range in.Seasonal {
		if _, ok := a.Config.Theme.Presets.Get(s.Preset); !ok {
			writeJSONError(w, http.StatusBadRequest, "\""+s.Preset+"\" isn't one of your themes. Add it first.")
			return
		}
		for _, d := range []string{s.From, s.To} {
			if _, err := parseMonthDay(d); err != nil {
				writeJSONError(w, http.StatusBadRequest, "Dates are month-day, like 12-24: "+err.Error())
				return
			}
		}
	}

	err := a.changeConfig(func(root *yaml.Node) error {
		theme, err := editableSection(root, "theme")
		if err != nil {
			return err
		}
		if len(in.Seasonal) == 0 {
			removeMappingKey(theme, "seasonal")
			pruneEmptySection(root, "theme")
			return nil
		}
		seq := sequenceNode()
		for _, s := range in.Seasonal {
			item := newMappingNode()
			addPair(item, "preset", scalarNode(s.Preset))
			addPair(item, "from", &yaml.Node{Kind: yaml.ScalarNode, Value: s.From, Style: yaml.DoubleQuotedStyle})
			addPair(item, "to", &yaml.Node{Kind: yaml.ScalarNode, Value: s.To, Style: yaml.DoubleQuotedStyle})
			seq.Content = append(seq.Content, item)
		}
		setMappingKey(theme, "seasonal", seq)
		return nil
	})
	if err != nil {
		writeSettingsWriteError(w, err)
		return
	}
	slog.Info("Seasonal themes saved from Settings", "count", len(in.Seasonal))
	writeJSON(w, http.StatusOK, map[string]any{"saved": len(in.Seasonal)})
}
