package dynacat

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const undoTestConfig = `pages:
  - name: Home
    columns:
      - size: full
        widgets:
          - type: clock
`

func undoTestApp(t *testing.T) (*application, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dashden.yml")
	if err := os.WriteFile(path, []byte(undoTestConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	return &application{configPath: path}, path
}

// editorSave stands in for an editor save: history first, then the write.
func editorSave(t *testing.T, a *application, path, contents string) {
	t.Helper()
	if err := a.writeConfigCandidates(map[string][]byte{path: []byte(contents)}); err != nil {
		t.Fatal(err)
	}
}

func TestUndoRestoresTheChangeForTheSamePerson(t *testing.T) {
	a, path := undoTestApp(t)
	alice := &authenticatedUser{Username: "alice"}
	bob := &authenticatedUser{Username: "bob"}

	started := time.Now()
	editorSave(t, a, path, undoTestConfig+"          - type: calendar\n")
	id := a.offerEditorUndo(alice, started)
	if id == "" {
		t.Fatal("no undo offered")
	}

	if status, err := a.undoEditorChange(bob, id); err == nil || status != http.StatusConflict {
		t.Fatalf("bob undid alice's change: %d %v", status, err)
	}
	if status, err := a.undoEditorChange(alice, id); err != nil {
		t.Fatalf("alice couldn't undo: %d %v", status, err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != undoTestConfig {
		t.Fatalf("config not restored:\n%s", got)
	}
	if _, err := a.undoEditorChange(alice, id); err == nil {
		t.Fatal("the same change was undone twice")
	}
}

func TestUndoRefusesOnceSomethingElseWasSaved(t *testing.T) {
	a, path := undoTestApp(t)
	alice := &authenticatedUser{Username: "alice"}

	started := time.Now()
	editorSave(t, a, path, undoTestConfig+"          - type: calendar\n")
	id := a.offerEditorUndo(alice, started)

	time.Sleep(2 * time.Millisecond) // history ids are timestamps
	editorSave(t, a, path, undoTestConfig+"          - type: calendar\n          - type: weather\n            location: London\n")

	if status, err := a.undoEditorChange(alice, id); err == nil || status != http.StatusConflict {
		t.Fatalf("undo went ahead over a later save: %d %v", status, err)
	}
}
