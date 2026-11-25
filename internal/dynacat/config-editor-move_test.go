package dynacat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Moving an included widget up within its column used to prune the wrong
// include entry, deleting the widget it landed on.
func TestMoveWidgetKeepsNeighboursWithIncludes(t *testing.T) {
	cases := []struct {
		name     string
		from, to []int
		want     []string
	}{
		{"up", []int{0, 1}, []int{0, 0}, []string{"b", "a", "c"}},
		{"up two", []int{0, 2}, []int{0, 0}, []string{"c", "a", "b"}},
		{"down", []int{0, 0}, []int{0, 2}, []string{"b", "a", "c"}},
		{"to end", []int{0, 0}, []int{0, 3}, []string{"b", "c", "a"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, n := range []string{"a", "b", "c"} {
				if err := os.WriteFile(filepath.Join(dir, n+".yml"), []byte("- type: "+n+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			page := filepath.Join(dir, "page.yml")
			src := "- size: full\n  widgets:\n    - $include: a.yml\n    - $include: b.yml\n    - $include: c.yml\n"
			if err := os.WriteFile(page, []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}

			var doc yaml.Node
			if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
				t.Fatal(err)
			}
			docs := newEditorDocs()
			docs.track(page, &doc)
			docs.pagePath = filepath.Clean(page)

			m := editorMutation{Op: "moveWidget", Path: tc.from, ToPath: tc.to}
			if err := mutateColumns(doc.Content[0], m, docs); err != nil {
				t.Fatal(err)
			}

			slots := expandWidgetSlots(getMappingValue(doc.Content[0].Content[0], "widgets"), page, docs, 0)
			var got []string
			for _, s := range slots {
				got = append(got, getMappingValue(s.node, "type").Value)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMoveWidgetInlineOrder(t *testing.T) {
	src := "- size: full\n  widgets:\n    - type: a\n    - type: b\n    - type: c\n    - type: d\n"
	cases := []struct {
		from, to []int
		want     string
	}{
		{[]int{0, 1}, []int{0, 0}, "b,a,c,d"},
		{[]int{0, 1}, []int{0, 3}, "a,c,b,d"},
		{[]int{0, 0}, []int{0, 4}, "b,c,d,a"},
		{[]int{0, 3}, []int{0, 1}, "a,d,b,c"},
	}
	for _, tc := range cases {
		var doc yaml.Node
		if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
			t.Fatal(err)
		}
		m := editorMutation{Op: "moveWidget", Path: tc.from, ToPath: tc.to}
		if err := mutateColumns(doc.Content[0], m, newEditorDocs()); err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, w := range getMappingValue(doc.Content[0].Content[0], "widgets").Content {
			got = append(got, getMappingValue(w, "type").Value)
		}
		if strings.Join(got, ",") != tc.want {
			t.Fatalf("%v -> %v: got %v, want %s", tc.from, tc.to, got, tc.want)
		}
	}
}
