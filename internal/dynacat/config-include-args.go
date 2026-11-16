package dynacat

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Reusable widget files. An include can pass values to the file it pulls
// in, so one file can serve many widgets that differ only in a title or a
// feed:
//
//	- $include: widgets/subreddit.yml
//	  with:
//	    title: /r/history
//	    feed: ${HISTORY_FEED}
//
// and widgets/subreddit.yml uses them as ${arg:title} and ${arg:feed}. A
// default can follow a bar: ${arg:limit|10}.

var includeArgPattern = regexp.MustCompile(`\$\{arg:([a-zA-Z0-9_-]+)(?:\|([^}]*))?\}`)

// includeWithBlock looks for a `with:` block right under the include on
// lines[i] and returns its values and how many lines it took up.
func includeWithBlock(lines []string, i int) (map[string]string, int, error) {
	includeLine := lines[i]
	keyColumn := strings.Index(includeLine, "include:") - 1 // the $ or !
	if keyColumn < 0 {
		return nil, 0, nil
	}

	j := i + 1
	for j < len(lines) && strings.TrimSpace(lines[j]) == "" {
		j++
	}
	if j >= len(lines) {
		return nil, 0, nil
	}

	withIndent := leadingSpaces(lines[j])
	if withIndent != keyColumn || strings.TrimSpace(lines[j]) != "with:" {
		return nil, 0, nil
	}

	end := j + 1
	for end < len(lines) {
		if strings.TrimSpace(lines[end]) != "" && leadingSpaces(lines[end]) <= withIndent {
			break
		}
		end++
	}
	// Trailing blank lines belong to whatever comes next.
	for end > j+1 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}

	block := strings.Join(lines[j+1:end], "\n")
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(dedent(block)), &raw); err != nil {
		return nil, 0, fmt.Errorf("the with: block under line %d isn't valid: %w", i+1, err)
	}

	args := make(map[string]string, len(raw))
	for k, v := range raw {
		switch v := v.(type) {
		case nil:
			args[k] = ""
		case string:
			args[k] = v
		case bool, int, int64, float64:
			args[k] = fmt.Sprint(v)
		default:
			// Lists and maps go in as JSON, which YAML reads too.
			encoded, err := json.Marshal(v)
			if err != nil {
				return nil, 0, fmt.Errorf("with: %s: %w", k, err)
			}
			args[k] = string(encoded)
		}
	}

	return args, end - i - 1, nil
}

// applyIncludeArgs fills ${arg:name} in an included file. Values are
// quoted as YAML would need them only when they stand alone as a value.
func applyIncludeArgs(contents []byte, args map[string]string, includedFrom string) ([]byte, error) {
	var missing []string
	out := includeArgPattern.ReplaceAllFunc(contents, func(m []byte) []byte {
		groups := includeArgPattern.FindSubmatch(m)
		name := string(groups[1])
		if v, ok := args[name]; ok {
			return []byte(v)
		}
		if groups[2] != nil {
			return groups[2]
		}
		missing = append(missing, name)
		return m
	})

	if len(missing) > 0 {
		return nil, fmt.Errorf("%s needs %s; add %s under with: at the include", includedFrom, strings.Join(missing, ", "), pluralise(len(missing), "it", "them"))
	}
	return out, nil
}

func pluralise(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func leadingSpaces(s string) int {
	return len(s) - len(strings.TrimLeft(s, " \t"))
}

func dedent(block string) string {
	lines := strings.Split(block, "\n")
	minIndent := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if n := leadingSpaces(l); minIndent < 0 || n < minIndent {
			minIndent = n
		}
	}
	if minIndent <= 0 {
		return block
	}
	for i, l := range lines {
		if len(l) >= minIndent {
			lines[i] = l[minIndent:]
		} else {
			lines[i] = strings.TrimLeft(l, " \t")
		}
	}
	return strings.Join(lines, "\n")
}

// sharedIncludeSlots stands in for the widgets an include with a with:
// block brings in: one slot per widget in the shared file, all pointing at
// the include entry itself.
func sharedIncludeSlots(raw widgetSlot, include, ownerPath string, docs *editorDocs) []widgetSlot {
	count := 1
	if _, doc, err := docs.load(ownerPath, include); err == nil {
		if root := documentRoot(doc); root.Kind == yaml.SequenceNode {
			count = len(root.Content)
		} else if root.Kind == 0 || root.Tag == "!!null" {
			count = 0
		}
	}

	raw.shared, raw.sharedCount = true, count
	slots := make([]widgetSlot, count)
	for i := range slots {
		slots[i] = raw
	}
	return slots
}

func checkSharedSlot(slot widgetSlot, editing bool) error {
	if !slot.shared {
		return nil
	}
	include := includeTarget(slot.node)
	if editing {
		return fmt.Errorf("this widget comes from the shared file %s; change its values under with: in the config, or edit %s itself", include, include)
	}
	if slot.sharedCount > 1 {
		return fmt.Errorf("this widget is one of several in the shared file %s; move or remove the include in the config instead", include)
	}
	return nil
}
