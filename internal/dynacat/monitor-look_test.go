package dynacat

import "testing"

func TestMonogram(t *testing.T) {
	cases := map[string]string{
		"Plex":            "P",
		"subs (manager)":  "S",
		"  (Ñandu) radio": "Ñ",
		"9 Lives":         "9",
		"":                "•",
		"🎬 Cinemask":      "C",
	}
	for in, want := range cases {
		if got := monogram(in); got != want {
			t.Errorf("monogram(%q) = %q, want %q", in, got, want)
		}
	}
}
