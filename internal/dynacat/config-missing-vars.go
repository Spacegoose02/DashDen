package dynacat

import (
	"errors"
	"fmt"
	"html/template"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// DashDen: a widget that uses an environment variable nobody has set is
// turned off on its own, showing which variable it needs, instead of the
// whole config failing to load. A variable missing anywhere else, such as
// in server or page settings, still stops the config as before.
//
// Expansion leaves a marker where the value would go. The marker is plain
// letters, dots and dashes, so it is valid in every kind of YAML scalar.

const missingVariablePrefix = "dashden-missing-env."

var missingVariableMarker = regexp.MustCompile(regexp.QuoteMeta(missingVariablePrefix) + `([A-Z0-9_]+)\.`)

func missingVariableValue(name string) string {
	return missingVariablePrefix + name + "."
}

type errMissingVariables struct{ names []string }

func (e *errMissingVariables) Error() string {
	return fmt.Sprintf("these variables aren't set: %s. Add them in Settings > Secrets or in config/.env", strings.Join(e.names, ", "))
}

func (e *errMissingVariables) friendly() string {
	what := "a setting that isn't"
	if len(e.names) == 1 {
		what = e.names[0] + ", which isn't"
	} else {
		what = strings.Join(e.names, " and ") + ", which aren't"
	}
	return "This widget needs " + what + " set yet. Add it in Settings > Secrets."
}

func friendlyMissingVariables(err error) (string, bool) {
	var missing *errMissingVariables
	if errors.As(err, &missing) {
		return missing.friendly(), true
	}
	return "", false
}

// missingVariablesIn lists the variables marked missing in a widget's own
// settings. Widgets nested in a group are checked on their own, so one
// child missing a variable doesn't turn the whole group off.
func missingVariablesIn(node *yaml.Node) []string {
	var names []string
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		switch n.Kind {
		case yaml.ScalarNode:
			for _, m := range missingVariableMarker.FindAllStringSubmatch(n.Value, -1) {
				if !slices.Contains(names, m[1]) {
					names = append(names, m[1])
				}
			}
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				if n.Content[i].Value == "widgets" && n.Content[i+1].Kind == yaml.SequenceNode {
					continue
				}
				walk(n.Content[i+1])
			}
		default:
			for _, c := range n.Content {
				walk(c)
			}
		}
	}
	walk(node)
	return names
}

// missingVariablesOutsideWidgets lists variables marked missing anywhere
// that isn't inside a widget. Those still stop the config from loading.
func missingVariablesOutsideWidgets(contents []byte) ([]string, error) {
	if !strings.Contains(string(contents), missingVariablePrefix) {
		return nil, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(contents, &doc); err != nil {
		return nil, err
	}
	var names []string
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		switch n.Kind {
		case yaml.ScalarNode:
			for _, m := range missingVariableMarker.FindAllStringSubmatch(n.Value, -1) {
				if !slices.Contains(names, m[1]) {
					names = append(names, m[1])
				}
			}
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				key, value := n.Content[i], n.Content[i+1]
				if (key.Value == "widgets" || key.Value == "head-widgets") && value.Kind == yaml.SequenceNode {
					// Widgets handle their own; only look for nested widget lists.
					for _, w := range value.Content {
						if w.Kind == yaml.MappingNode {
							for j := 0; j+1 < len(w.Content); j += 2 {
								if w.Content[j].Value == "widgets" {
									walk(&yaml.Node{Kind: yaml.MappingNode, Content: w.Content[j : j+2]})
								}
							}
						} else {
							walk(w)
						}
					}
					continue
				}
				walk(key)
				walk(value)
			}
		default:
			for _, c := range n.Content {
				walk(c)
			}
		}
	}
	walk(&doc)
	return names, nil
}

// missingVariablesWidget stands in for a widget whose variables aren't set.
// It keeps the widget's title, size and access rules so the layout and who
// can see it stay the same.
type missingVariablesWidget struct {
	widgetBase `yaml:",inline"`
	missing    []string
}

var missingVariablesWidgetTemplate = mustParseTemplate("missing-variables.html", "widget-base.html")

func newMissingVariablesWidget(node *yaml.Node, names []string) *missingVariablesWidget {
	w := &missingVariablesWidget{missing: names}
	// Only the plain settings are read; anything that fails to decode
	// (a marker where a number belongs) just keeps its default.
	var base struct {
		Type       string `yaml:"type"`
		Title      string `yaml:"title"`
		HideHeader bool   `yaml:"hide-header"`
		CSSClass   string `yaml:"css-class"`
		Tile       string `yaml:"tile"`
	}
	_ = node.Decode(&base)
	_ = node.Decode(&w.widgetAccess)
	w.Type = cmpOr(base.Type, "widget")
	w.Title = base.Title
	w.CSSClass = base.CSSClass
	w.Tile = base.Tile
	if missingVariableMarker.MatchString(w.Title) {
		w.Title = ""
	}
	w.Error = &errMissingVariables{names: names}
	return w
}

func (w *missingVariablesWidget) initialize() error {
	w.withTitle("Not set up yet")
	w.ContentAvailable = false
	w.cacheType = cacheTypeInfinite
	return nil
}

func (w *missingVariablesWidget) Render() template.HTML {
	return w.renderTemplate(w, missingVariablesWidgetTemplate)
}
