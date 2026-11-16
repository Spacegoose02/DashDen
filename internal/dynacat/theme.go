package dynacat

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"time"
)

var (
	themeStyleTemplate         = mustParseTemplate("theme-style.gotmpl")
	themePresetPreviewTemplate = mustParseTemplate("theme-preset-preview.html")
)

func (a *application) handleThemeChangeRequest(w http.ResponseWriter, r *http.Request) {
	if a.handleUnauthorizedResponse(w, r, showUnauthorizedJSON) {
		return
	}

	themeKey := r.PathValue("key")

	properties, exists := a.themeByKey(themeKey)
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if user := a.getAuthenticatedUser(w, r); user != nil && a.userPrefs != nil {
		if err := a.userPrefs.set(user.Username, userPrefTheme, themeKey); err != nil {
			slog.Error("Saving theme choice failed", "username", user.Username, "error", err)
		}
	}

	a.setCookie(w, r, &http.Cookie{
		Name:     "theme",
		Value:    themeKey,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(2 * 365 * 24 * time.Hour),
	})

	w.Header().Set("Content-Type", "text/css")
	w.Header().Set("X-Scheme", ternary(properties.Light, "light", "dark"))
	w.Write([]byte(properties.CSS))
}

type themeProperties struct {
	BackgroundColor          *hslColorField `yaml:"background-color"`
	PrimaryColor             *hslColorField `yaml:"primary-color"`
	PositiveColor            *hslColorField `yaml:"positive-color"`
	NegativeColor            *hslColorField `yaml:"negative-color"`
	Light                    bool           `yaml:"light"`
	ContrastMultiplier       float32        `yaml:"contrast-multiplier"`
	TextSaturationMultiplier float32        `yaml:"text-saturation-multiplier"`
	// DashDen: see theme-styles.go
	Name      string `yaml:"name"`
	Style     string `yaml:"style"`
	Wallpaper string `yaml:"wallpaper"`

	Key                  string        `yaml:"-"`
	CSS                  template.CSS  `yaml:"-"`
	PreviewHTML          template.HTML `yaml:"-"`
	StyleCSS             template.CSS  `yaml:"-"`
	BackgroundColorAsHex string        `yaml:"-"`
}

func (t *themeProperties) init() error {
	if t.Name == "" && t.Key != "" {
		t.Name = themeNameFromKey(t.Key)
	}
	styleCSS, err := t.styleCSS()
	if err != nil {
		return err
	}
	t.StyleCSS = styleCSS

	css, err := executeTemplateToString(themeStyleTemplate, t)
	if err != nil {
		return fmt.Errorf("compiling theme style: %v", err)
	}
	t.CSS = template.CSS(whitespaceAtBeginningOfLinePattern.ReplaceAllString(css, ""))

	previewHTML, err := executeTemplateToString(themePresetPreviewTemplate, t)
	if err != nil {
		return fmt.Errorf("compiling theme preview: %v", err)
	}
	t.PreviewHTML = template.HTML(previewHTML)

	if t.BackgroundColor != nil {
		t.BackgroundColorAsHex = t.BackgroundColor.ToHex()
	} else {
		t.BackgroundColorAsHex = "#151519"
	}

	return nil
}

func (t1 *themeProperties) SameAs(t2 *themeProperties) bool {
	if t1 == nil && t2 == nil {
		return true
	}
	if t1 == nil || t2 == nil {
		return false
	}
	if t1.Light != t2.Light {
		return false
	}
	if t1.ContrastMultiplier != t2.ContrastMultiplier {
		return false
	}
	if t1.TextSaturationMultiplier != t2.TextSaturationMultiplier {
		return false
	}
	if t1.Style != t2.Style || t1.Wallpaper != t2.Wallpaper {
		return false
	}
	if !t1.BackgroundColor.SameAs(t2.BackgroundColor) {
		return false
	}
	if !t1.PrimaryColor.SameAs(t2.PrimaryColor) {
		return false
	}
	if !t1.PositiveColor.SameAs(t2.PositiveColor) {
		return false
	}
	if !t1.NegativeColor.SameAs(t2.NegativeColor) {
		return false
	}
	return true
}

func (a *application) themeByKey(key string) (*themeProperties, bool) {
	if key == "default" {
		return &a.Config.Theme.themeProperties, true
	}
	return a.Config.Theme.Presets.Get(key)
}

// profileTheme is the signed-in person's saved theme choice, or else their
// household default. It wins over the browser's cookie so the theme follows
// the person rather than the device.
func (a *application) profileTheme(user *authenticatedUser) (*themeProperties, bool) {
	if user == nil {
		return nil, false
	}

	if a.userPrefs != nil {
		key, ok, err := a.userPrefs.get(user.Username, userPrefTheme)
		if err != nil {
			slog.Error("Loading theme choice failed", "username", user.Username, "error", err)
		} else if ok {
			if theme, exists := a.themeByKey(key); exists {
				return theme, true
			}
		}
	}

	if m := user.Member(); m != nil && m.Theme != "" {
		return a.themeByKey(m.Theme)
	}

	return nil, false
}
