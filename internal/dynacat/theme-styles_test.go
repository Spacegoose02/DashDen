package dynacat

import (
	"strings"
	"testing"
)

func TestEveryBuiltinThemeCompiles(t *testing.T) {
	seen := map[string]bool{}
	for _, b := range builtinThemes {
		if seen[b.key] {
			t.Fatalf("theme key %s is used twice", b.key)
		}
		seen[b.key] = true
		p := b.properties()
		p.Key = b.key
		if err := p.init(); err != nil {
			t.Fatalf("%s: %v", b.key, err)
		}
		if b.style != "classic" && !strings.Contains(string(p.CSS), "--border-radius") {
			t.Fatalf("%s: style %s didn't reach the theme CSS", b.key, b.style)
		}
		if strings.Contains(string(p.CSS), "&#39;") || strings.Contains(string(p.CSS), "&#34;") {
			t.Fatalf("%s: quotes were escaped in the theme CSS", b.key)
		}
	}
}

func TestThemeStyleAndWallpaperChecks(t *testing.T) {
	p := &themeProperties{Style: "sparkly"}
	if _, err := p.styleCSS(); err == nil || !strings.Contains(err.Error(), "glass") {
		t.Fatalf("an unknown style should list the real ones, got %v", err)
	}

	for _, bad := range []string{`/a.jpg"); color: red`, "javascript:alert(1)", "/a b.jpg", `/a\.jpg`} {
		p := &themeProperties{Wallpaper: bad}
		if _, err := p.styleCSS(); err == nil {
			t.Fatalf("wallpaper %q was accepted", bad)
		}
	}

	p = &themeProperties{Style: "Glass", Wallpaper: "/assets/beach.jpg"}
	css, err := p.styleCSS()
	if err != nil || !strings.Contains(string(css), "url(/assets/beach.jpg)") {
		t.Fatalf("wallpaper not applied: %v %s", err, css)
	}
}

func TestBuiltinThemesDontReplaceTheConfigsOwn(t *testing.T) {
	mine := &themeProperties{PrimaryColor: &hslColorField{1, 2, 3}}
	presets, _ := newOrderedYAMLMap([]string{"glass"}, []*themeProperties{mine})
	merged := withBuiltinThemes(presets)

	got, _ := merged.Get("glass")
	if got != mine {
		t.Fatal("the config's own glass preset was replaced")
	}
	if _, ok := merged.Get("cozy-den"); !ok {
		t.Fatal("built-in themes missing")
	}
	var first string
	for k := range merged.Items() {
		first = k
		break
	}
	if first != "glass" {
		t.Fatalf("the config's presets should come first, got %s", first)
	}
}

func TestThemeNameFromKey(t *testing.T) {
	if got := themeNameFromKey("green-dialy"); got != "Green dialy" {
		t.Fatal(got)
	}
}
