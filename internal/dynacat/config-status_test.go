package dynacat

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvChangeIsOursSkipsOnlySettingsWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(path, []byte("A=1\n"), 0o600)

	rememberEnvWrite(path)
	if !envChangeIsOurs(path) {
		t.Fatal("a change Settings wrote should be skipped")
	}

	os.WriteFile(path, []byte("A=2\n"), 0o600)
	if envChangeIsOurs(path) {
		t.Fatal("a hand edit should reload")
	}

	rememberEnvWrite(path)
	os.WriteFile(path, []byte("A=3\n"), 0o600)
	if envChangeIsOurs(path) {
		t.Fatal("an edit after a Settings save should still reload")
	}
}
