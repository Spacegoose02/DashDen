package dynacat

import (
	"fmt"
	"html/template"
	"strconv"
	"strings"
)

// Bento pages lay every widget out on one grid of tiles instead of in
// columns. The grid gets more columns on wider screens, and each widget can
// ask for a tile size:
//
//	pages:
//	  - name: Home
//	    layout: bento
//	    columns:
//	      - size: full
//	        widgets:
//	          - type: clock
//	            tile: wide      # or 2, 2x2, big, tall, 3x1...
//
// The first number is how many grid columns the tile spans, the second how
// many tile heights it is tall. Without a height ("2", "wide") a tile is as
// tall as its content.
// static/js/bento.js packs the tiles.

const (
	pageLayoutColumns = "columns"
	pageLayoutBento   = "bento"
	bentoMaxTileSpan  = 6
	bentoMaxColumns   = 12
)

// A size without a height ("2") is as tall as its content.
var bentoTileAliases = map[string]string{
	"small":  "1",
	"wide":   "2",
	"tall":   "1x2",
	"big":    "2x2",
	"large":  "2x2",
	"banner": "4",
}

// Widget files written for the family apps size themselves with a CSS class
// (bento-s, bento-w, bento-t, bento-l) so they also work on dashboards that
// read classes. A widget's own tile wins over its class.
var bentoClassTiles = map[string]string{
	"bento-s": "1x1",
	"bento-w": "2x1",
	"bento-t": "1x2",
	"bento-l": "2x2",
}

func bentoTileFromClass(cssClass string) string {
	for _, class := range strings.Fields(cssClass) {
		if tile, ok := bentoClassTiles[strings.ToLower(class)]; ok {
			return tile
		}
	}
	return ""
}

type bentoTile struct{ w, h int } // h == 0: as tall as the content

func parseBentoTile(value string) (bentoTile, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" || v == "auto" {
		return bentoTile{w: 1}, nil
	}
	if alias, ok := bentoTileAliases[v]; ok {
		v = alias
	}

	wText, hText, hasHeight := strings.Cut(v, "x")
	w, err := strconv.Atoi(wText)
	if err != nil || w < 1 || w > bentoMaxTileSpan {
		return bentoTile{}, fmt.Errorf("tile %q: use a size like 2 or 2x1 (up to %d) or small, wide, tall, big or banner", value, bentoMaxTileSpan)
	}
	tile := bentoTile{w: w}
	if hasHeight && hText != "auto" {
		h, err := strconv.Atoi(hText)
		if err != nil || h < 1 || h > bentoMaxTileSpan {
			return bentoTile{}, fmt.Errorf("tile %q: the height after the x goes from 1 to %d, or auto", value, bentoMaxTileSpan)
		}
		tile.h = h
	}
	return tile, nil
}

// TileAttrs gives a widget's tile size to the bento grid.
func (w *widgetBase) TileAttrs() template.HTMLAttr {
	value := w.Tile
	if value == "" {
		value = bentoTileFromClass(w.CSSClass)
	}
	if value == "" {
		return ""
	}
	tile, err := parseBentoTile(value)
	if err != nil {
		return ""
	}
	attrs := fmt.Sprintf(`data-tile-w="%d"`, tile.w)
	if tile.h > 0 {
		attrs += fmt.Sprintf(` data-tile-h="%d" style="--tile-h: %d"`, tile.h, tile.h)
	}
	return template.HTMLAttr(attrs)
}

func (p *page) IsBento() bool {
	return p.Layout == pageLayoutBento
}

// BentoAttrs fixes the grid's column count when the page sets bento-columns,
// so tiles line up in the same rows on every wide screen. Narrower screens
// still drop to two columns and phones to one (bento.css).
func (p *page) BentoAttrs() template.HTMLAttr {
	if p.BentoColumns <= 0 {
		return ""
	}
	return template.HTMLAttr(fmt.Sprintf(` data-bento-columns="%d" style="--bento-cols: %d"`, p.BentoColumns, p.BentoColumns))
}

func validatePageLayout(p *page, pageNumber int) error {
	switch p.Layout {
	case "", pageLayoutColumns, pageLayoutBento:
	default:
		return fmt.Errorf("page %d: layout can be columns or bento", pageNumber)
	}
	if p.BentoColumns < 0 || p.BentoColumns > bentoMaxColumns {
		return fmt.Errorf("page %d: bento-columns goes from 1 to %d", pageNumber, bentoMaxColumns)
	}

	for c := range p.Columns {
		for _, w := range p.Columns[c].Widgets {
			tiled, ok := w.(interface{ tileValue() string })
			if !ok || tiled.tileValue() == "" {
				continue
			}
			tile := tiled.tileValue()
			if _, err := parseBentoTile(tile); err != nil {
				return fmt.Errorf("page %d, %s widget: %v", pageNumber, w.GetType(), err)
			}
		}
	}
	return nil
}

func (w *widgetBase) tileValue() string { return w.Tile }
