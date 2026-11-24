package dynacat

import (
	"path/filepath"
	"testing"
)

func TestCheckIncludesStayInConfigDir(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "dynacat.yml")
	page := filepath.Join(dir, "pages", "home.yml")

	cases := []struct {
		name    string
		file    string
		content string
		ok      bool
	}{
		{"root relative", root, "- $include: ./widgets/calendar.yml\n", true},
		{"subfolder reaches sibling folder", page, "      - $include: ../widgets/calendar.yml\n", true},
		{"subfolder same folder", page, "- $include: other.yml\n", true},
		{"root escapes", root, "- $include: ../secret.yml\n", false},
		{"subfolder escapes", page, "- $include: ../../secret.yml\n", false},
		{"absolute outside", root, "- $include: /etc/passwd\n", false},
		{"absolute inside", root, "- $include: " + filepath.Join(dir, "widgets", "a.yml") + "\n", true},
	}

	for _, c := range cases {
		err := checkIncludesStayInConfigDir([]byte(c.content), c.file, dir)
		if (err == nil) != c.ok {
			t.Errorf("%s: got err=%v, want ok=%v", c.name, err, c.ok)
		}
	}
}
