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

// DashDen keeps secrets set from the Settings screen in its own env file next
// to the config, loaded at start-up after --env-file, so they win over older
// values. The file is written with 0600 permissions and values are never sent
// back to the browser.
const managedSecretsFileName = "dashden-secrets.env"

var secretNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

// Matches ${NAME} and ${env:NAME}, the forms that read the process environment.
var configEnvReferencePattern = regexp.MustCompile(`\$\{(?:env:)?([A-Za-z_][A-Za-z0-9_]*)\}`)

var managedSecretsMu sync.Mutex

func managedSecretsPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), managedSecretsFileName)
}

func loadManagedSecrets(configPath string) error {
	path := managedSecretsPath(configPath)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return loadEnvFile(path)
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
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = parseEnvValue(strings.TrimSpace(value))
	}

	return values, scanner.Err()
}

func writeManagedSecrets(path string, values map[string]string) error {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("# Written by DashDen Settings. Values here override the same names from --env-file.\n")
	for _, name := range names {
		v := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(values[name])
		fmt.Fprintf(&b, "%s=\"%s\"\n", name, v)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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
	managedSecretsMu.Lock()
	defer managedSecretsMu.Unlock()

	path := managedSecretsPath(configPath)
	values, err := readManagedSecrets(path)
	if err != nil {
		return err
	}

	if value == nil {
		if _, ok := values[name]; !ok {
			return nil
		}
		delete(values, name)
		if err := writeManagedSecrets(path, values); err != nil {
			return err
		}
		return os.Unsetenv(name)
	}

	if err := validateSecret(name, *value); err != nil {
		return err
	}
	values[name] = *value
	if err := writeManagedSecrets(path, values); err != nil {
		return err
	}
	return os.Setenv(name, *value)
}

type secretStatus struct {
	Name string `json:"name"`
	// "settings" when saved from Settings, "environment" when it only comes from
	// the container or --env-file, "missing" when the config uses it but it isn't set.
	Source     string `json:"source"`
	UsedByConf bool   `json:"usedByConfig"`
}

func listSecretStatuses(configPath string, rawConfig []byte) ([]secretStatus, error) {
	managedSecretsMu.Lock()
	saved, err := readManagedSecrets(managedSecretsPath(configPath))
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
