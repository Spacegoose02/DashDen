package dynacat

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// DashDen keeps secrets in the .env file next to the config (config/.env in
// Docker), the same file people already keep their keys in. It is loaded at
// start-up, after --env-file, and its values win over the container's
// environment, so a value changed in Settings sticks after a restart even when
// the container was started with an older copy of the file. Settings edits the
// file in place: other lines, comments and order are kept. Values are never sent
// back to the browser.
const managedSecretsFileName = ".env"

// Secrets saved by earlier DashDen builds. Still loaded, and folded into .env
// the next time a secret is saved.
const legacySecretsFileName = "dashden-secrets.env"

var secretNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

// Matches ${NAME} and ${env:NAME}, the forms that read the process environment.
var configEnvReferencePattern = regexp.MustCompile(`\$\{(?:env:)?([A-Za-z_][A-Za-z0-9_]*)\}`)

var managedSecretsMu sync.Mutex

func managedSecretsPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), managedSecretsFileName)
}

func legacySecretsPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), legacySecretsFileName)
}

func loadManagedSecrets(configPath string) error {
	for _, path := range []string{managedSecretsPath(configPath), legacySecretsPath(configPath)} {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		if err := loadEnvFile(path); err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
	}
	return nil
}

func readManagedSecrets(path string) (map[string]string, error) {
	values := map[string]string{}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return values, nil
	}
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		if key, value, ok := parseEnvLine(scanner.Text()); ok {
			values[key] = value
		}
	}

	return values, scanner.Err()
}

func parseEnvLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	key, value, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
	if !ok {
		return "", "", false
	}
	return strings.TrimSpace(key), parseEnvValue(strings.TrimSpace(value)), true
}

func formatEnvLine(name, value string) string {
	v := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
	return fmt.Sprintf("%s=\"%s\"", name, v)
}

// updateEnvFile sets or removes names in the env file, keeping every other
// line as it was. A nil value removes the name.
func updateEnvFile(path string, changes map[string]*string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}

	var lines []string
	if len(data) > 0 {
		lines = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	}

	done := map[string]bool{}
	out := lines[:0:0]
	for _, line := range lines {
		key, _, ok := parseEnvLine(line)
		change, changing := changes[key]
		if !ok || !changing {
			out = append(out, line)
			continue
		}
		if change == nil || done[key] {
			continue
		}
		out = append(out, formatEnvLine(key, *change))
		done[key] = true
	}

	names := make([]string, 0, len(changes))
	for name, v := range changes {
		if v != nil && !done[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		out = append(out, formatEnvLine(name, *changes[name]))
	}

	content := strings.Join(out, "\n")
	if content != "" {
		content += "\n"
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// migrateLegacySecrets folds dashden-secrets.env into .env and removes it.
func migrateLegacySecrets(configPath string) error {
	legacy := legacySecretsPath(configPath)
	values, err := readManagedSecrets(legacy)
	if err != nil || len(values) == 0 {
		if err == nil {
			os.Remove(legacy)
		}
		return err
	}
	changes := make(map[string]*string, len(values))
	for name, v := range values {
		changes[name] = &v
	}
	if err := updateEnvFile(managedSecretsPath(configPath), changes); err != nil {
		return err
	}
	return os.Remove(legacy)
}

func validateSecret(name, value string) error {
	if !secretNamePattern.MatchString(name) {
		return fmt.Errorf("a secret name uses letters, digits and underscores and doesn't start with a digit")
	}
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("a secret value must be a single line")
	}
	return nil
}

// setManagedSecret saves the value, or removes the secret when value is nil,
// and applies it to the running process.
func setManagedSecret(configPath, name string, value *string) error {
	if value != nil {
		if err := validateSecret(name, *value); err != nil {
			return err
		}
	}

	managedSecretsMu.Lock()
	defer managedSecretsMu.Unlock()

	if err := migrateLegacySecrets(configPath); err != nil {
		return fmt.Errorf("moving saved secrets into .env: %w", err)
	}
	if err := updateEnvFile(managedSecretsPath(configPath), map[string]*string{name: value}); err != nil {
		return err
	}
	if value == nil {
		return os.Unsetenv(name)
	}
	return os.Setenv(name, *value)
}

type secretStatus struct {
	Name string `json:"name"`
	// "settings" when it is in config/.env (where Settings saves), "environment"
	// when it only comes from the container or --env-file, "missing" when the config uses it but it isn't set.
	Source     string `json:"source"`
	UsedByConf bool   `json:"usedByConfig"`
}

func listSecretStatuses(configPath string, rawConfig []byte) ([]secretStatus, error) {
	managedSecretsMu.Lock()
	saved, err := readManagedSecrets(managedSecretsPath(configPath))
	if legacy, lerr := readManagedSecrets(legacySecretsPath(configPath)); err == nil {
		err = lerr
		for k, v := range legacy {
			saved[k] = v
		}
	}
	managedSecretsMu.Unlock()
	if err != nil {
		return nil, err
	}

	used := map[string]bool{}
	for _, m := range configEnvReferencePattern.FindAllSubmatch(rawConfig, -1) {
		used[string(m[1])] = true
	}

	names := map[string]bool{}
	for n := range saved {
		names[n] = true
	}
	for n := range used {
		names[n] = true
	}

	out := make([]secretStatus, 0, len(names))
	for n := range names {
		s := secretStatus{Name: n, UsedByConf: used[n]}
		switch {
		case saved[n] != "" || hasKey(saved, n):
			s.Source = "settings"
		case os.Getenv(n) != "":
			s.Source = "environment"
		default:
			s.Source = "missing"
		}
		out = append(out, s)
	}

	sort.Slice(out, func(i, j int) bool {
		// Missing first, since those are the ones to fix.
		if (out[i].Source == "missing") != (out[j].Source == "missing") {
			return out[i].Source == "missing"
		}
		return out[i].Name < out[j].Name
	})

	return out, nil
}

func hasKey(m map[string]string, k string) bool {
	_, ok := m[k]
	return ok
}
