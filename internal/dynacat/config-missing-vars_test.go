package dynacat

import (
	"strings"
	"testing"
)

func TestMissingVariableTurnsOffOnlyThatWidget(t *testing.T) {
	t.Setenv("DASHDEN_TEST_SET", "https://example.com")
	cfg, err := newConfigFromYAML([]byte(`
pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: custom-api
            title: Chores
            css-class: bento-t
            url: ${DASHDEN_TEST_UNSET_URL}/api
            cache: ${DASHDEN_TEST_UNSET_CACHE}
            headers:
              Authorization: Bearer ${DASHDEN_TEST_UNSET_TOKEN}
            template: |
              <p>{{ .JSON.String "x" }} ${DASHDEN_TEST_UNSET_URL}</p>
          - type: group
            widgets:
              - type: html
                source: ok
              - type: iframe
                source: ${DASHDEN_TEST_UNSET_IFRAME}
          - type: iframe
            source: ${DASHDEN_TEST_SET}
`))
	if err != nil {
		t.Fatalf("a widget's missing variable should not stop the config: %v", err)
	}
	ws := cfg.Pages[0].Columns[0].Widgets
	if len(ws) != 3 {
		t.Fatalf("got %d widgets", len(ws))
	}

	off, ok := ws[0].(*missingVariablesWidget)
	if !ok {
		t.Fatalf("first widget should be turned off, got %T", ws[0])
	}
	if off.Title != "Chores" || off.CSSClass != "bento-t" || off.GetType() != "custom-api" {
		t.Fatalf("title and size should be kept: %+v", off.widgetBase)
	}
	if got := strings.Join(off.missing, ","); got != "DASHDEN_TEST_UNSET_URL,DASHDEN_TEST_UNSET_CACHE,DASHDEN_TEST_UNSET_TOKEN" {
		t.Fatalf("missing: %s", got)
	}
	html := string(off.Render())
	if !strings.Contains(html, "DASHDEN_TEST_UNSET_URL") || !strings.Contains(html, "Settings &gt; Secrets") {
		t.Fatalf("render should name the variable:\n%s", html)
	}

	group := ws[1].(*groupWidget)
	if _, ok := group.Widgets[0].(*htmlWidget); !ok {
		t.Fatal("the group's other widget should still work")
	}
	if _, ok := group.Widgets[1].(*missingVariablesWidget); !ok {
		t.Fatalf("only the group's child missing a variable should be off, got %T", group.Widgets[1])
	}

	if _, ok := ws[2].(*iframeWidget); !ok {
		t.Fatalf("widgets with their variables set are unchanged, got %T", ws[2])
	}
}

func TestMissingVariableOutsideWidgetsStillFails(t *testing.T) {
	for _, conf := range []string{
		"server:\n  base-url: ${DASHDEN_TEST_UNSET_BASE}\npages:\n  - name: Home\n    columns:\n      - size: full\n        widgets: []\n",
		"pages:\n  - name: ${DASHDEN_TEST_UNSET_NAME}\n    columns:\n      - size: full\n        widgets: []\n",
	} {
		_, err := newConfigFromYAML([]byte(conf))
		if err == nil || !strings.Contains(err.Error(), "environment variable DASHDEN_TEST_UNSET_") {
			t.Fatalf("expected the config to fail, got %v", err)
		}
	}
}

func TestMissingVariableMarkerOnlyForEnv(t *testing.T) {
	out, err := parseConfigVariablesMarkingMissing([]byte("a: ${DASHDEN_TEST_UNSET_X}\nb: \\${KEEP}\n"))
	if err != nil || !strings.Contains(string(out), "a: dashden-missing-env.DASHDEN_TEST_UNSET_X.") || !strings.Contains(string(out), "b: ${KEEP}") {
		t.Fatalf("%q %v", out, err)
	}
	if _, err := parseConfigVariables([]byte("a: ${DASHDEN_TEST_UNSET_X}\n")); err == nil {
		t.Fatal("the strict parser should still refuse")
	}
}
