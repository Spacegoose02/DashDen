package dynacat

import (
	"strings"
	"testing"
)

func TestParseBentoTile(t *testing.T) {
	cases := map[string]bentoTile{
		"":       {w: 1},
		"auto":   {w: 1},
		"2":      {w: 2},
		"wide":   {w: 2},
		"big":    {w: 2, h: 2},
		"tall":   {w: 1, h: 2},
		"3x1":    {w: 3, h: 1},
		"2xauto": {w: 2},
		" BIG ":  {w: 2, h: 2},
	}
	for in, want := range cases {
		got, err := parseBentoTile(in)
		if err != nil || got != want {
			t.Errorf("%q: got %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, bad := range []string{"0", "5", "2x9", "huge", "x2", "-1"} {
		if _, err := parseBentoTile(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestTileAttrs(t *testing.T) {
	w := &widgetBase{Tile: "2x3"}
	if got := string(w.TileAttrs()); got != `data-tile-w="2" data-tile-h="3" style="--tile-h: 3"` {
		t.Fatal(got)
	}
	if (&widgetBase{}).TileAttrs() != "" {
		t.Fatal("no tile should add nothing")
	}
}

func TestBentoPageConfig(t *testing.T) {
	_, err := newConfigFromYAML([]byte(`
pages:
  - name: Home
    layout: bento
    width: full
    columns:
      - size: full
        widgets:
          - type: clock
            tile: big
`))
	if err != nil {
		t.Fatal(err)
	}

	_, err = newConfigFromYAML([]byte(`
pages:
  - name: Home
    layout: bento
    columns:
      - size: full
        widgets:
          - type: clock
            tile: enormous
`))
	if err == nil || !strings.Contains(err.Error(), "enormous") {
		t.Fatalf("a bad tile should be reported, got %v", err)
	}

	_, err = newConfigFromYAML([]byte(`
pages:
  - name: Home
    layout: masonry
    columns:
      - size: full
        widgets:
          - type: clock
`))
	if err == nil {
		t.Fatal("unknown layout accepted")
	}
}
