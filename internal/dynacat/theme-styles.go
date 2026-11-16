package dynacat

import (
	"fmt"
	"html/template"
	"regexp"
	"sort"
	"strings"
)

// Theme styles go beyond colors: corner rounding, glass, glow, wallpaper,
// font and size. A style only sets the --dd-* variables that
// static/css/theme-styles.css reads, so it works over any page layout and
// switches with the rest of the theme, without a reload.
//
//	theme:
//	  presets:
//	    my-theme:
//	      style: glass
//	      wallpaper: /assets/beach.jpg

// Font names stay unquoted: the theme template escapes quotes.
const (
	roundedFont = `ui-rounded, SF Pro Rounded, Nunito, Varela Round, system-ui, sans-serif`
	systemFont  = `system-ui, -apple-system, Segoe UI, Roboto, sans-serif`
	monoFont    = `JetBrains Mono, ui-monospace, monospace`

	primaryMix = `color-mix(in srgb, var(--color-primary) %d%%, transparent)`
)

func mix(percent int) string { return fmt.Sprintf(primaryMix, percent) }

var themeStyles = map[string]string{
	// The look DashDen always had.
	"classic": ``,

	"cozy": `
		--border-radius: 14px;
		--widget-gap: 20px;
		--dd-font: ` + roundedFont + `;
		--dd-widget-shadow: 0 6px 18px -8px rgba(0, 0, 0, 0.45);
		--dd-wallpaper: radial-gradient(ellipse at top, ` + mix(10) + `, transparent 60%);`,

	"glass": `
		--border-radius: 18px;
		--dd-font: ` + systemFont + `;
		--color-widget-background: hsla(var(--color-widget-background-hsl-values), 0.45);
		--color-widget-content-border: hsla(0, 0%, 100%, 0.12);
		--dd-backdrop: blur(18px) saturate(150%);
		--dd-widget-shadow: 0 10px 30px -12px rgba(0, 0, 0, 0.5);
		--dd-wallpaper: radial-gradient(circle at 12% 18%, ` + mix(45) + `, transparent 40%),
			radial-gradient(circle at 85% 30%, color-mix(in srgb, var(--color-positive) 35%, transparent), transparent 45%),
			radial-gradient(circle at 50% 95%, color-mix(in srgb, var(--color-negative) 30%, transparent), transparent 45%);`,

	"neon": `
		--border-radius: 10px;
		--color-widget-content-border: ` + mix(55) + `;
		--dd-widget-shadow: 0 0 14px -2px ` + mix(55) + `, inset 0 0 12px -6px ` + mix(60) + `;
		--dd-title-glow: 0 0 8px ` + mix(80) + `;
		--dd-text-glow: 0 0 6px ` + mix(70) + `;
		--dd-wallpaper: radial-gradient(ellipse at bottom, ` + mix(18) + `, transparent 65%);`,

	"laser": `
		--border-radius: 2px;
		--dd-font: ` + systemFont + `;
		--dd-title-spacing: 0.12em;
		--color-widget-content-border: var(--color-primary);
		--dd-widget-shadow: 0 0 0 1px ` + mix(35) + `, 0 0 22px -6px var(--color-primary);
		--dd-title-glow: 0 0 6px var(--color-primary), 0 0 14px var(--color-primary);
		--dd-text-glow: 0 0 5px var(--color-primary);
		--dd-wallpaper: linear-gradient(` + mix(9) + ` 1px, transparent 1px),
			linear-gradient(90deg, ` + mix(9) + ` 1px, transparent 1px);
		--dd-wallpaper-size: 48px 48px;`,

	"velvet": `
		--border-radius: 20px;
		--dd-font: ` + roundedFont + `;
		--color-widget-content-border: ` + mix(25) + `;
		--dd-widget-shadow: 0 8px 26px -10px ` + mix(55) + `;
		--dd-title-glow: 0 0 10px ` + mix(45) + `;
		--dd-wallpaper: radial-gradient(circle at 80% 0%, ` + mix(25) + `, transparent 45%),
			radial-gradient(circle at 0% 100%, ` + mix(18) + `, transparent 50%);`,

	"retro": `
		--border-radius: 4px;
		--dd-title-spacing: 0.08em;
		--dd-widget-border-width: 2px;
		--color-widget-content-border: ` + mix(60) + `;
		--dd-widget-shadow: 4px 4px 0 ` + mix(45) + `;
		--dd-title-glow: 2px 2px 0 color-mix(in srgb, var(--color-negative) 70%, transparent);
		--dd-wallpaper: linear-gradient(180deg, transparent 55%, color-mix(in srgb, var(--color-negative) 22%, transparent) 80%, ` + mix(30) + `);
		--dd-overlay: repeating-linear-gradient(0deg, rgba(0, 0, 0, 0.12) 0 1px, transparent 1px 3px);`,

	"nuclear": `
		--border-radius: 6px;
		--dd-title-spacing: 0.1em;
		--color-widget-content-border: ` + mix(40) + `;
		--dd-widget-decor: repeating-linear-gradient(-45deg, var(--color-primary) 0 10px, hsl(0 0% 6%) 10px 20px);
		--dd-widget-decor-size: 5px;
		--dd-widget-shadow: 0 0 20px -8px var(--color-primary);
		--dd-text-glow: 0 0 6px ` + mix(80) + `;
		--dd-title-glow: 0 0 8px ` + mix(70) + `;
		--dd-wallpaper: radial-gradient(circle at 50% 40%, ` + mix(14) + `, transparent 55%);`,

	"terminal": `
		--border-radius: 0;
		--dd-font: ` + monoFont + `;
		--color-widget-content-border: ` + mix(45) + `;
		--dd-text-glow: 0 0 4px ` + mix(60) + `;
		--dd-overlay: repeating-linear-gradient(0deg, rgba(0, 0, 0, 0.18) 0 1px, transparent 1px 2px);`,

	"soft": `
		--border-radius: 18px;
		--widget-gap: 22px;
		--dd-font: ` + systemFont + `;
		--color-widget-content-border: transparent;
		--dd-widget-shadow: 0 2px 6px rgba(0, 0, 0, 0.08), 0 12px 32px -14px rgba(0, 0, 0, 0.3);`,

	"flat": `
		--border-radius: 2px;
		--dd-font: ` + systemFont + `;
		--color-widget-content-border: transparent;`,

	"bold": `
		--border-radius: 6px;
		--dd-widget-border-width: 2px;
		--color-widget-content-border: var(--color-text-highlight);`,

	// Big, round and friendly: larger text and tap targets for kids.
	"kids": `
		--dd-root-size: 11.5px;
		--border-radius: 24px;
		--widget-gap: 24px;
		--dd-font: ` + roundedFont + `;
		--dd-widget-border-width: 3px;
		--color-widget-content-border: ` + mix(55) + `;
		--dd-widget-shadow: 0 6px 0 ` + mix(35) + `;
		--dd-wallpaper: radial-gradient(circle at 0% 100%, color-mix(in srgb, var(--color-positive) 22%, transparent) 0 140px, transparent 141px),
			radial-gradient(circle at 100% 100%, ` + mix(22) + ` 0 200px, transparent 201px),
			radial-gradient(circle at 100% 0%, color-mix(in srgb, var(--color-negative) 16%, transparent) 0 90px, transparent 91px);`,
}

func themeStyleNames() []string {
	names := make([]string, 0, len(themeStyles))
	for name := range themeStyles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// A wallpaper ends up inside an unquoted url(), so it can't carry quotes,
// brackets, spaces or backslashes. Quotes would be escaped by the template.
var themeWallpaperPattern = regexp.MustCompile(`^(https?://|/)[^"'()\\\s]+$`)

// styleCSS turns a theme's style and wallpaper into declarations for its
// :root block.
func (t *themeProperties) styleCSS() (template.CSS, error) {
	style := strings.ToLower(strings.TrimSpace(t.Style))
	if style == "" {
		style = "classic"
	}
	decls, ok := themeStyles[style]
	if !ok {
		return "", fmt.Errorf("unknown theme style %q, use one of %s", t.Style, strings.Join(themeStyleNames(), ", "))
	}

	if t.Wallpaper != "" {
		if !themeWallpaperPattern.MatchString(t.Wallpaper) {
			return "", fmt.Errorf("wallpaper must be a plain http(s) URL or a path starting with /")
		}
		decls += "\n--dd-wallpaper: url(" + t.Wallpaper + ");\n--dd-wallpaper-size: cover;"
	}

	return template.CSS(decls), nil
}

// Built-in themes everyone can pick, on top of the config's own presets.
// theme.builtin-themes: false turns them off.
type builtinTheme struct {
	key, name     string
	bg, primary   [3]float64
	positive, neg [3]float64
	light         bool
	contrast, sat float32
	style         string
}

var builtinThemes = []builtinTheme{
	// Everyday
	{key: "cozy-den", name: "Cozy Den", bg: hsl(25, 22, 11), primary: hsl(33, 85, 60), positive: hsl(95, 40, 55), neg: hsl(5, 70, 62), contrast: 1.1, sat: 0.9, style: "cozy"},
	{key: "latte", name: "Latte", bg: hsl(35, 35, 92), primary: hsl(20, 60, 38), positive: hsl(140, 45, 35), neg: hsl(0, 60, 48), light: true, contrast: 1.25, sat: 0.6, style: "cozy"},
	{key: "glass", name: "Glass", bg: hsl(230, 25, 12), primary: hsl(200, 90, 65), positive: hsl(160, 60, 55), neg: hsl(340, 75, 65), style: "glass"},
	{key: "aurora", name: "Aurora", bg: hsl(205, 35, 8), primary: hsl(160, 80, 55), positive: hsl(120, 60, 55), neg: hsl(280, 70, 65), style: "glass"},
	{key: "daylight", name: "Daylight", bg: hsl(220, 20, 96), primary: hsl(220, 80, 45), positive: hsl(150, 55, 35), neg: hsl(0, 65, 50), light: true, contrast: 1.3, sat: 0.5, style: "soft"},
	{key: "midnight", name: "Midnight", bg: hsl(225, 35, 7), primary: hsl(220, 90, 68), positive: hsl(160, 55, 55), neg: hsl(0, 70, 65), style: "soft"},
	{key: "minimal", name: "Minimal", bg: hsl(0, 0, 8), primary: hsl(0, 0, 85), positive: hsl(140, 30, 60), neg: hsl(0, 50, 65), sat: 0.1, style: "flat"},
	{key: "high-contrast", name: "High Contrast", bg: hsl(0, 0, 2), primary: hsl(55, 100, 60), positive: hsl(120, 80, 55), neg: hsl(0, 90, 65), contrast: 1.6, style: "bold"},

	// Loud
	{key: "neon-night", name: "Neon Night", bg: hsl(255, 30, 7), primary: hsl(310, 100, 65), positive: hsl(165, 100, 50), neg: hsl(350, 100, 65), sat: 1.2, style: "neon"},
	{key: "neon-city", name: "Neon City", bg: hsl(200, 40, 6), primary: hsl(185, 100, 55), positive: hsl(95, 100, 55), neg: hsl(330, 100, 65), sat: 1.2, style: "neon"},
	{key: "laser-red", name: "Laser Red", bg: hsl(0, 30, 5), primary: hsl(355, 100, 60), positive: hsl(30, 100, 60), neg: hsl(290, 90, 65), contrast: 1.1, style: "laser"},
	{key: "laser-grid", name: "Laser Grid", bg: hsl(240, 40, 5), primary: hsl(190, 100, 55), positive: hsl(140, 100, 55), neg: hsl(320, 100, 65), contrast: 1.1, style: "laser"},
	{key: "pink-velvet", name: "Pink Velvet", bg: hsl(320, 30, 8), primary: hsl(325, 90, 70), positive: hsl(285, 70, 72), neg: hsl(355, 85, 68), sat: 1.1, style: "velvet"},
	{key: "rose-noir", name: "Rose Noir", bg: hsl(340, 20, 6), primary: hsl(340, 75, 65), positive: hsl(30, 70, 70), neg: hsl(0, 80, 62), style: "velvet"},
	{key: "bubblegum", name: "Bubblegum", bg: hsl(300, 25, 11), primary: hsl(315, 100, 72), positive: hsl(180, 80, 60), neg: hsl(5, 90, 68), sat: 1.2, style: "velvet"},
	{key: "retro-wave", name: "Retro Wave", bg: hsl(265, 45, 9), primary: hsl(320, 100, 65), positive: hsl(185, 100, 55), neg: hsl(30, 100, 60), sat: 1.2, style: "retro"},
	{key: "retro-70s", name: "Retro 70s", bg: hsl(25, 35, 12), primary: hsl(35, 90, 55), positive: hsl(75, 50, 50), neg: hsl(10, 80, 55), style: "retro"},
	{key: "arcade", name: "Arcade", bg: hsl(230, 30, 6), primary: hsl(50, 100, 55), positive: hsl(130, 90, 55), neg: hsl(0, 95, 62), contrast: 1.15, style: "retro"},
	{key: "nuclear", name: "Nuclear", bg: hsl(90, 20, 5), primary: hsl(75, 100, 50), positive: hsl(120, 90, 50), neg: hsl(25, 100, 55), contrast: 1.1, style: "nuclear"},
	{key: "hazard", name: "Hazard", bg: hsl(45, 15, 6), primary: hsl(50, 100, 52), positive: hsl(90, 80, 50), neg: hsl(15, 100, 58), contrast: 1.1, style: "nuclear"},
	{key: "terminal", name: "Terminal", bg: hsl(120, 20, 4), primary: hsl(120, 90, 50), positive: hsl(90, 80, 55), neg: hsl(0, 80, 60), sat: 1.2, style: "terminal"},

	// Kids: bigger text, rounder, brighter
	{key: "playroom", name: "Playroom", bg: hsl(255, 35, 15), primary: hsl(45, 100, 60), positive: hsl(170, 80, 50), neg: hsl(0, 90, 66), sat: 1.2, style: "kids"},
	{key: "rainbow", name: "Rainbow", bg: hsl(200, 60, 95), primary: hsl(265, 75, 55), positive: hsl(145, 65, 38), neg: hsl(5, 85, 55), light: true, contrast: 1.3, sat: 1.1, style: "kids"},
	{key: "dino", name: "Dino", bg: hsl(130, 30, 12), primary: hsl(95, 75, 55), positive: hsl(50, 95, 60), neg: hsl(15, 90, 62), sat: 1.1, style: "kids"},
	{key: "under-the-sea", name: "Under the Sea", bg: hsl(195, 70, 93), primary: hsl(200, 85, 42), positive: hsl(170, 70, 35), neg: hsl(10, 85, 55), light: true, contrast: 1.3, style: "kids"},
	{key: "space-explorer", name: "Space Explorer", bg: hsl(235, 45, 10), primary: hsl(195, 100, 65), positive: hsl(55, 100, 62), neg: hsl(330, 95, 70), sat: 1.15, style: "kids"},
	{key: "unicorn", name: "Unicorn", bg: hsl(290, 45, 94), primary: hsl(310, 75, 52), positive: hsl(185, 70, 38), neg: hsl(350, 85, 55), light: true, contrast: 1.25, sat: 1.1, style: "kids"},
}

func hsl(h, s, l float64) [3]float64 { return [3]float64{h, s, l} }

func (b builtinTheme) properties() *themeProperties {
	color := func(c [3]float64) *hslColorField {
		if c == ([3]float64{}) {
			return nil
		}
		return &hslColorField{H: c[0], S: c[1], L: c[2]}
	}
	return &themeProperties{
		Name:                     b.name,
		Style:                    b.style,
		BackgroundColor:          color(b.bg),
		PrimaryColor:             color(b.primary),
		PositiveColor:            color(b.positive),
		NegativeColor:            color(b.neg),
		Light:                    b.light,
		ContrastMultiplier:       b.contrast,
		TextSaturationMultiplier: b.sat,
	}
}

// withBuiltinThemes adds the built-in themes after presets, leaving out any
// whose key the config already uses.
func withBuiltinThemes(presets *orderedYAMLMap[string, *themeProperties]) *orderedYAMLMap[string, *themeProperties] {
	var keys []string
	var props []*themeProperties
	for _, b := range builtinThemes {
		if _, taken := presets.Get(b.key); taken {
			continue
		}
		keys = append(keys, b.key)
		props = append(props, b.properties())
	}
	builtins, _ := newOrderedYAMLMap(keys, props)
	return presets.Merge(builtins)
}

// themeNameFromKey labels a preset without a name: "green-daily" becomes
// "Green daily".
func themeNameFromKey(key string) string {
	name := strings.NewReplacer("-", " ", "_", " ").Replace(key)
	if name == "" {
		return ""
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func isBuiltinThemeKey(key string) bool {
	for _, b := range builtinThemes {
		if b.key == key {
			return true
		}
	}
	return false
}

// kidsDefaultTheme is where kids start when their household entry names no
// theme: big, round and bright. They can still pick another.
const kidsDefaultTheme = "playroom"
