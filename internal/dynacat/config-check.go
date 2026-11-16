package dynacat

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// The config is decoded with plain yaml.Unmarshal, which ignores keys it
// doesn't know. A misspelled key, or a block at the wrong indent, then does
// nothing and says nothing. This file finds those keys and reports them with
// the file and line they came from, so they can be shown in the log, by
// `dashden check` and on the dashboard.

// configSourceLine is where one line of the expanded config came from.
type configSourceLine struct {
	file string
	line int
}

// configLineMap mirrors recursiveParseYAMLIncludes line by line, so a line
// number in the expanded config can be traced back to its file.
func configLineMap(mainFilePath string) ([]configSourceLine, error) {
	return recursiveConfigLineMap(mainFilePath, 0)
}

func recursiveConfigLineMap(path string, depth int) ([]configSourceLine, error) {
	if depth > CONFIG_INCLUDE_RECURSION_DEPTH_LIMIT {
		return nil, fmt.Errorf("recursion depth limit of %d reached", CONFIG_INCLUDE_RECURSION_DEPTH_LIMIT)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(absPath)

	// Arguments are filled in before includes are expanded, and a value with
	// a line break would shift the lines below it. Values are almost always
	// single-line, so the map ignores that case.
	lines := strings.Split(string(contents), "\n")
	var out []configSourceLine
	for i := 0; i < len(lines); i++ {
		matches := configIncludePattern.FindStringSubmatch(lines[i])
		if matches == nil {
			out = append(out, configSourceLine{file: absPath, line: i + 1})
			continue
		}

		includePath := strings.TrimSpace(matches[2])
		if !filepath.IsAbs(includePath) {
			includePath = filepath.Join(dir, includePath)
		}

		_, skip, err := includeWithBlock(lines, i)
		if err != nil {
			return nil, err
		}
		i += skip

		included, err := recursiveConfigLineMap(includePath, depth+1)
		if err != nil {
			return nil, err
		}
		out = append(out, included...)
	}

	return out, nil
}

// configLocator turns a line of the expanded config into "file.yml line 12".
type configLocator struct {
	lines   []configSourceLine
	baseDir string
}

func newConfigLocator(mainFilePath string) *configLocator {
	lines, err := configLineMap(mainFilePath)
	if err != nil {
		return nil
	}
	absMain, _ := filepath.Abs(mainFilePath)
	return &configLocator{lines: lines, baseDir: filepath.Dir(absMain)}
}

func (l *configLocator) at(line int) string {
	if l == nil || line < 1 || line > len(l.lines) {
		return fmt.Sprintf("line %d", line)
	}
	src := l.lines[line-1]
	name := src.file
	if rel, err := filepath.Rel(l.baseDir, src.file); err == nil && !strings.HasPrefix(rel, "..") {
		name = filepath.ToSlash(rel)
	}
	return fmt.Sprintf("%s line %d", name, src.line)
}

var yamlErrorLinePattern = regexp.MustCompile(`\bline (\d+)\b`)

// locateError rewrites "line N" in a YAML or widget error so it names the
// file the line is really in.
func (l *configLocator) locateError(err error) error {
	if l == nil || err == nil {
		return err
	}
	msg := yamlErrorLinePattern.ReplaceAllStringFunc(err.Error(), func(m string) string {
		n, convErr := strconv.Atoi(yamlErrorLinePattern.FindStringSubmatch(m)[1])
		if convErr != nil {
			return m
		}
		return l.at(n)
	})
	return fmt.Errorf("%s", msg)
}

// checkConfigKeys reports every key in the expanded config that DashDen
// doesn't read. It never fails the load: unknown keys are warnings. The same
// mistake repeated in many places is reported once, with every place listed.
func checkConfigKeys(contents []byte, locate *configLocator) []string {
	var root yaml.Node
	if err := yaml.Unmarshal(contents, &root); err != nil || len(root.Content) == 0 {
		return nil
	}

	c := &configKeyChecker{locate: locate}
	c.walk(root.Content[0], reflect.TypeOf(config{}), "")
	sort.SliceStable(c.issues, func(i, j int) bool { return c.issues[i].line < c.issues[j].line })

	var order []string
	places := map[string][]string{}
	for _, issue := range c.issues {
		if _, seen := places[issue.text]; !seen {
			order = append(order, issue.text)
		}
		places[issue.text] = append(places[issue.text], locate.at(issue.line))
	}

	out := make([]string, 0, len(order))
	for _, text := range order {
		out = append(out, formatConfigIssuePlaces(places[text])+": "+text)
	}
	return out
}

// formatConfigIssuePlaces lists where a problem is, folding repeats in one
// file: "dynacat.yml lines 339, 367 and 526".
func formatConfigIssuePlaces(places []string) string {
	if len(places) == 1 {
		return places[0]
	}

	var files []string
	lines := map[string][]string{}
	for _, p := range places {
		file, line, ok := strings.Cut(p, " line ")
		if !ok {
			file, line = p, ""
		}
		if _, seen := lines[file]; !seen {
			files = append(files, file)
		}
		if !slices.Contains(lines[file], line) {
			lines[file] = append(lines[file], line)
		}
	}

	const maxLines = 6
	parts := make([]string, 0, len(files))
	for _, file := range files {
		ls := lines[file]
		if len(ls) == 1 {
			parts = append(parts, file+" line "+ls[0])
			continue
		}
		shown := ls
		more := ""
		if len(ls) > maxLines {
			shown = ls[:maxLines]
			more = fmt.Sprintf(" and %d more", len(ls)-maxLines)
		}
		parts = append(parts, file+" lines "+strings.Join(shown, ", ")+more)
	}
	return strings.Join(parts, "; ")
}

type configKeyIssue struct {
	line int
	text string
}

type configKeyChecker struct {
	locate *configLocator
	issues []configKeyIssue
}

var (
	yamlUnmarshalerType = reflect.TypeOf((*yaml.Unmarshaler)(nil)).Elem()
	widgetsType         = reflect.TypeOf(widgets{})
	topLevelConfigKeys  = yamlFieldsOf(reflect.TypeOf(config{}))
)

// Names other dashboards use for a setting DashDen calls something else.
var configKeyAliases = map[string]string{
	"skip-tls-verify":      "allow-insecure",
	"insecure-skip-verify": "allow-insecure",
	"insecure":             "allow-insecure",
}

func (c *configKeyChecker) walk(node *yaml.Node, t reflect.Type, path string) {
	if node == nil {
		return
	}
	if node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		node = node.Content[0]
	}

	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if t == widgetsType {
		c.walkWidgets(node)
		return
	}

	// Types that decode themselves accept whatever shape they like.
	if t.Implements(yamlUnmarshalerType) || reflect.PointerTo(t).Implements(yamlUnmarshalerType) {
		return
	}

	switch t.Kind() {
	case reflect.Struct:
		if node.Kind != yaml.MappingNode {
			return
		}
		fields := yamlFieldsOf(t)
		if fields == nil {
			return // an inline map takes any key
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if key.Value == "<<" {
				c.walk(value, t, path)
				continue
			}
			field, ok := fields[key.Value]
			if !ok {
				c.unknownKey(key, path, fields)
				continue
			}
			c.walk(value, field, joinConfigPath(path, key.Value))
		}
	case reflect.Slice, reflect.Array:
		if node.Kind != yaml.SequenceNode {
			return
		}
		for _, item := range node.Content {
			c.walk(item, t.Elem(), path)
		}
	case reflect.Map:
		if node.Kind != yaml.MappingNode {
			return
		}
		// Map keys are names the person chose, so they stay out of the path.
		for i := 0; i+1 < len(node.Content); i += 2 {
			c.walk(node.Content[i+1], t.Elem(), path)
		}
	}
}

func (c *configKeyChecker) walkWidgets(node *yaml.Node) {
	if node.Kind != yaml.SequenceNode {
		return
	}
	for _, item := range node.Content {
		if item.Kind == yaml.AliasNode {
			item = item.Alias
		}
		if item.Kind != yaml.MappingNode {
			continue
		}
		var meta struct {
			Type string `yaml:"type"`
		}
		if item.Decode(&meta) != nil {
			continue
		}
		w, err := newWidget(meta.Type)
		if err != nil {
			continue // the real decode reports unknown widget types
		}
		c.walk(item, reflect.TypeOf(w), "the "+meta.Type+" widget")
	}
}

func (c *configKeyChecker) unknownKey(key *yaml.Node, path string, known map[string]reflect.Type) {
	where := "at the top level"
	if path != "" {
		where = "in " + path
	}

	text := fmt.Sprintf("%q is not a setting %s, so it is ignored", key.Value, where)

	switch {
	case path != "" && topLevelConfigKeys[key.Value] != nil && !strings.HasPrefix(path, "the "):
		text += fmt.Sprintf(". %q is a top-level section; move it all the way to the left", key.Value)
	case key.Value == "type" && path != "":
		text += ". This looks like a widget that is indented too far or is missing the \"- \" in front of it"
	case configKeyAliases[key.Value] != "" && known[configKeyAliases[key.Value]] != nil:
		text += fmt.Sprintf(". DashDen calls this %q", configKeyAliases[key.Value])
	default:
		if suggestion := closestConfigKey(key.Value, known); suggestion != "" {
			text += fmt.Sprintf(". Did you mean %q?", suggestion)
		}
	}

	c.issues = append(c.issues, configKeyIssue{line: key.Line, text: text})
}

func joinConfigPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + " > " + key
}

var yamlFieldsCache sync.Map // reflect.Type -> map[string]reflect.Type

// yamlFieldsOf lists the keys yaml.v3 decodes into a struct, following
// inline fields the same way yaml.v3 does. It returns nil when an inline map
// accepts any key.
func yamlFieldsOf(t reflect.Type) map[string]reflect.Type {
	if cached, ok := yamlFieldsCache.Load(t); ok {
		return cached.(map[string]reflect.Type)
	}

	fields := map[string]reflect.Type{}
	anyKey := false

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" && !f.Anonymous {
			continue
		}

		tag := f.Tag.Get("yaml")
		if tag == "" && strings.Index(string(f.Tag), ":") < 0 {
			tag = string(f.Tag)
		}
		if tag == "-" {
			continue
		}

		parts := strings.Split(tag, ",")
		name := parts[0]
		inline := false
		for _, flag := range parts[1:] {
			if flag == "inline" {
				inline = true
			}
		}

		if inline {
			ft := f.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			switch ft.Kind() {
			case reflect.Map:
				anyKey = true
			case reflect.Struct:
				inner := yamlFieldsOf(ft)
				if inner == nil {
					anyKey = true
				}
				for k, v := range inner {
					fields[k] = v
				}
			}
			continue
		}

		if f.PkgPath != "" {
			continue
		}
		if name == "" {
			name = strings.ToLower(f.Name)
		}
		fields[name] = f.Type
	}

	if anyKey {
		fields = nil
	}
	yamlFieldsCache.Store(t, fields)
	return fields
}

func closestConfigKey(key string, known map[string]reflect.Type) string {
	best, bestDistance := "", 3
	for candidate := range known {
		d := levenshtein(strings.ToLower(key), candidate)
		if d < bestDistance || (d == bestDistance && candidate < best) {
			best, bestDistance = candidate, d
		}
	}
	if bestDistance > 2 || bestDistance >= len(key) {
		return ""
	}
	return best
}

// levenshtein is the edit distance with two swapped letters counting as one
// typo, so "titel" is closer to "title" than to "tile".
func levenshtein(a, b string) int {
	ar, br := []rune(a), []rune(b)
	d := make([][]int, len(ar)+1)
	for i := range d {
		d[i] = make([]int, len(br)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(ar); i++ {
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && ar[i-1] == br[j-2] && ar[i-2] == br[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(ar)][len(br)]
}

// missingConfigVariables lists the ${VARIABLES} the config uses that aren't
// set, with where each is used, so they can all be fixed in one go.
func missingConfigVariables(contents []byte, locate *configLocator) []string {
	var order []string
	places := map[string][]string{}

	for i, line := range strings.Split(string(contents), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		for _, groups := range configVariablePattern.FindAllStringSubmatch(line, -1) {
			prefix, kind, name := groups[1], groups[2], groups[3]
			if prefix == `\` || (kind != "" && kind != configVarTypeEnv) || !envVariableNamePattern.MatchString(name) {
				continue
			}
			if _, ok := os.LookupEnv(name); ok {
				continue
			}
			if _, seen := places[name]; !seen {
				order = append(order, name)
			}
			places[name] = append(places[name], locate.at(i+1))
		}
	}

	out := make([]string, 0, len(order))
	for _, name := range order {
		out = append(out, fmt.Sprintf("%s (used in %s)", name, formatConfigIssuePlaces(places[name])))
	}
	return out
}

// explainConfigError adds file names to line numbers and, when variables
// are missing, names all of them instead of only the first.
func explainConfigError(err error, contents []byte, locate *configLocator) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "environment variable") {
		if missing := missingConfigVariables(contents, locate); len(missing) > 0 {
			return fmt.Errorf("these variables aren't set. Add them in Settings > Secrets, in config/.env or to the container: %s", strings.Join(missing, "; "))
		}
	}
	return locate.locateError(err)
}

// configHealth is what the dashboard shows people who can open Settings
// when the config has problems.
type configHealth struct {
	mu          sync.Mutex
	warnings    []string
	reloadError string
	reloadAt    time.Time
}

var currentConfigHealth = &configHealth{}

func (h *configHealth) loaded(warnings []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.warnings = warnings
	h.reloadError = ""
}

func (h *configHealth) failed(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.reloadError = err.Error()
	h.reloadAt = time.Now()
}

// ConfigProblems is what templates read.
type ConfigProblems struct {
	ReloadError string
	ReloadAt    time.Time
	Warnings    []string
}

func (p ConfigProblems) Count() int {
	n := len(p.Warnings)
	if p.ReloadError != "" {
		n++
	}
	return n
}

func (h *configHealth) snapshot() ConfigProblems {
	h.mu.Lock()
	defer h.mu.Unlock()
	return ConfigProblems{
		ReloadError: h.reloadError,
		ReloadAt:    h.reloadAt,
		Warnings:    append([]string(nil), h.warnings...),
	}
}

// ConfigProblemsFor returns the config problems the person should see: all
// of them for people who can open Settings, none for everyone else.
func (a *application) ConfigProblemsFor(user *authenticatedUser) ConfigProblems {
	if !a.CanUseSettings(user) {
		return ConfigProblems{}
	}
	return currentConfigHealth.snapshot()
}

// checkConfigFile is `dashden check`: it loads the config the way the
// server does and prints every problem it finds.
func checkConfigFile(configPath string) int {
	locate := newConfigLocator(configPath)

	contents, _, err := parseYAMLIncludes(configPath)
	if err != nil {
		fmt.Printf("Could not read the config: %v\n", err)
		return 1
	}

	_, err = newConfigFromYAML(contents)
	warnings := checkConfigKeys(contents, locate)

	if err != nil {
		fmt.Printf("✗ The config has an error and DashDen won't load it:\n  %v\n", explainConfigError(err, contents, locate))
	}

	if len(warnings) > 0 {
		fmt.Printf("\n%d setting(s) are ignored:\n", len(warnings))
		for _, w := range warnings {
			fmt.Printf("  • %s\n", w)
		}
	}

	if err != nil || len(warnings) > 0 {
		return 1
	}

	fmt.Println("✓ The config looks good.")
	return 0
}
