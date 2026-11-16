package dynacat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfigFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "dynacat.yml")
}

func checkFiles(t *testing.T, files map[string]string) []string {
	t.Helper()
	main := writeConfigFiles(t, files)
	contents, _, err := parseYAMLIncludes(main)
	if err != nil {
		t.Fatal(err)
	}
	return checkConfigKeys(contents, newConfigLocator(main))
}

func TestCheckConfigKeysFindsTyposAndMisplacedSections(t *testing.T) {
	warnings := checkFiles(t, map[string]string{
		"dynacat.yml": `server:
  port: 8080
  auth:
    secret-key: abc
pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - $include: widgets/news.yml
`,
		"widgets/news.yml": `- type: rss
  titel: News
  feeds:
    - url: https://example.com/feed
      show-thumbnails: true
`,
	})

	joined := strings.Join(warnings, "\n")
	for _, want := range []string{
		`dynacat.yml line 3: "auth" is not a setting in server`,
		`move it all the way to the left`,
		`widgets/news.yml line 2: "titel" is not a setting in the rss widget`,
		`Did you mean "title"?`,
		`widgets/news.yml line 5: "show-thumbnails" is not a setting in the rss widget > feeds`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in:\n%s", want, joined)
		}
	}
	if len(warnings) != 3 {
		t.Errorf("want 3 warnings, got %d:\n%s", len(warnings), joined)
	}
}

func TestCheckConfigKeysAcceptsAValidConfig(t *testing.T) {
	warnings := checkFiles(t, map[string]string{
		"dynacat.yml": `theme:
  background-color: 240 13 14
  presets:
    dark:
      primary-color: 51 33 68
pages:
  - name: Home
    head-widgets:
      - type: search
        search-engine: duckduckgo
    columns:
      - size: full
        widgets:
          - type: group
            widgets:
              - type: bookmarks
                allowed-users: [kid]
                groups:
                  - title: Links
                    links:
                      - title: Example
                        url: https://example.com
                        same-tab: true
          - type: custom-api
            url: https://example.com
            options:
              anything: goes
            template: "{{ .JSON.String \"x\" }}"
`,
	})
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got:\n%s", strings.Join(warnings, "\n"))
	}
}

func TestCheckConfigKeysGroupsRepeats(t *testing.T) {
	warnings := checkFiles(t, map[string]string{
		"dynacat.yml": `pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: rss
            show-thumbnails: true
          - type: rss
            show-thumbnails: true
`,
	})
	if len(warnings) != 1 || !strings.HasPrefix(warnings[0], "dynacat.yml lines 7, 9:") {
		t.Fatalf("got %q", warnings)
	}
}

func TestDuplicateSectionsAreAnError(t *testing.T) {
	main := writeConfigFiles(t, map[string]string{
		"dynacat.yml": "auth:\n  secret-key: a\npages: []\nauth:\n  secret-key: b\n",
	})
	contents, _, err := parseYAMLIncludes(main)
	if err != nil {
		t.Fatal(err)
	}
	_, err = newConfigFromYAML(contents)
	if err == nil {
		t.Fatal("expected duplicate auth sections to fail")
	}
	explained := explainConfigError(err, contents, newConfigLocator(main)).Error()
	if !strings.Contains(explained, "dynacat.yml line") {
		t.Errorf("error doesn't name the file: %s", explained)
	}
}

func TestMissingConfigVariablesListsAll(t *testing.T) {
	main := writeConfigFiles(t, map[string]string{
		"dynacat.yml": "pages:\n  - name: ${DASHDEN_TEST_MISSING_A}\n    slug: ${DASHDEN_TEST_MISSING_B}\n# ${DASHDEN_TEST_MISSING_C}\n",
	})
	contents, _, _ := parseYAMLIncludes(main)
	missing := missingConfigVariables(contents, newConfigLocator(main))
	if len(missing) != 2 || !strings.Contains(missing[0], "DASHDEN_TEST_MISSING_A (used in dynacat.yml line 2)") {
		t.Fatalf("got %q", missing)
	}
}
