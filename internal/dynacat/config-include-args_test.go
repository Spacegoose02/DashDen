package dynacat

import (
	"strings"
	"testing"
)

func TestIncludeWithArgs(t *testing.T) {
	main := writeConfigFiles(t, map[string]string{
		"dynacat.yml": `pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - $include: widgets/subreddit.yml
            with:
              title: /r/history
              feed: https://example.com/history

          - $include: widgets/subreddit.yml
            with:
              title: /r/science
              feed: https://example.com/science
              limit: 5
          - type: clock
            titel: typo
`,
		"widgets/subreddit.yml": `- type: rss
  title: ${arg:title}
  limit: ${arg:limit|10}
  feeds:
    - url: ${arg:feed}
`,
	})

	contents, includes, err := parseYAMLIncludes(main)
	if err != nil {
		t.Fatal(err)
	}
	if len(includes) != 1 {
		t.Errorf("want 1 include tracked, got %d", len(includes))
	}

	cfg, err := newConfigFromYAML(contents)
	if err != nil {
		t.Fatalf("config: %v\n%s", err, contents)
	}

	ws := cfg.Pages[0].Columns[0].Widgets
	if len(ws) != 3 {
		t.Fatalf("want 3 widgets, got %d", len(ws))
	}
	first, second := ws[0].(*rssWidget), ws[1].(*rssWidget)
	if first.Title != "/r/history" || first.Limit != 10 || first.FeedRequests[0].URL != "https://example.com/history" {
		t.Errorf("first widget: %+v", first.widgetBase.Title)
	}
	if second.Title != "/r/science" || second.Limit != 5 {
		t.Errorf("second widget: %q %d", second.Title, second.Limit)
	}

	// Line numbers still point at the right place after with: blocks.
	warnings := checkConfigKeys(contents, newConfigLocator(main))
	if len(warnings) != 1 || !strings.HasPrefix(warnings[0], "dynacat.yml line 17:") {
		t.Errorf("got %q", warnings)
	}
}

func TestIncludeMissingArg(t *testing.T) {
	main := writeConfigFiles(t, map[string]string{
		"dynacat.yml": `pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - $include: widgets/subreddit.yml
            with:
              title: /r/history
`,
		"widgets/subreddit.yml": "- type: rss\n  title: ${arg:title}\n  feeds:\n    - url: ${arg:feed}\n",
	})
	_, _, err := parseYAMLIncludes(main)
	if err == nil || !strings.Contains(err.Error(), "subreddit.yml needs feed") {
		t.Fatalf("got %v", err)
	}
}

func TestIncludeWithoutArgsUnchanged(t *testing.T) {
	main := writeConfigFiles(t, map[string]string{
		"dynacat.yml": "pages:\n  - $include: home.yml\n",
		"home.yml":    "name: Home\ncolumns:\n  - size: full\n    widgets: []\n",
	})
	contents, _, err := parseYAMLIncludes(main)
	if err != nil {
		t.Fatal(err)
	}
	want := "pages:\n  name: Home\n  columns:\n    - size: full\n      widgets: []\n  \n"
	if string(contents) != want {
		t.Errorf("got %q", contents)
	}
}
