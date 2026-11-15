package dynacat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Every save from the editor or Settings first copies the files it is about to
// change into .dashden-history/<id>/, so any save can be undone from Settings.
const (
	configHistoryDirName = ".dashden-history"
	configHistoryKeep    = 50
)

var configHistoryIDPattern = regexp.MustCompile(`^\d{8}-\d{6}\.\d{6}$`)

type configSnapshotFile struct {
	Path    string `json:"path"` // relative to the config directory
	Existed bool   `json:"existed"`
}

type configSnapshot struct {
	ID      string               `json:"id"`
	Created time.Time            `json:"created"`
	Files   []configSnapshotFile `json:"files"`
}

func configHistoryDir(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), configHistoryDirName)
}

// snapshotConfigFiles stores the current contents of the files about to be
// written. It returns the snapshot's directory so a failed save can drop it.
func snapshotConfigFiles(configPath string, originals map[string][]byte, created map[string]bool) (string, error) {
	if configPath == "" || len(originals) == 0 {
		return "", nil
	}

	configDir, err := filepath.Abs(filepath.Dir(configPath))
	if err != nil {
		return "", err
	}

	now := time.Now()
	snap := configSnapshot{ID: now.UTC().Format("20060102-150405.000000"), Created: now}
	dir := filepath.Join(configHistoryDir(configPath), snap.ID)

	for path, data := range originals {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		rel, err := filepath.Rel(configDir, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}

		snap.Files = append(snap.Files, configSnapshotFile{Path: filepath.ToSlash(rel), Existed: !created[path]})
		if created[path] {
			continue
		}

		target := filepath.Join(dir, "files", rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return "", err
		}
	}

	if len(snap.Files) == 0 {
		return "", nil
	}

	sort.Slice(snap.Files, func(i, j int) bool { return snap.Files[i].Path < snap.Files[j].Path })
	manifest, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "snapshot.json"), manifest, 0o644); err != nil {
		return "", err
	}

	pruneConfigHistory(configPath, configHistoryKeep)
	return dir, nil
}

func listConfigSnapshots(configPath string) ([]configSnapshot, error) {
	entries, err := os.ReadDir(configHistoryDir(configPath))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	snaps := make([]configSnapshot, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() || !configHistoryIDPattern.MatchString(e.Name()) {
			continue
		}
		snap, err := readConfigSnapshot(configPath, e.Name())
		if err != nil {
			continue
		}
		snaps = append(snaps, snap)
	}

	sort.Slice(snaps, func(i, j int) bool { return snaps[i].ID > snaps[j].ID })
	return snaps, nil
}

func readConfigSnapshot(configPath, id string) (configSnapshot, error) {
	var snap configSnapshot
	if !configHistoryIDPattern.MatchString(id) {
		return snap, fmt.Errorf("invalid snapshot id")
	}
	data, err := os.ReadFile(filepath.Join(configHistoryDir(configPath), id, "snapshot.json"))
	if err != nil {
		return snap, err
	}
	err = json.Unmarshal(data, &snap)
	return snap, err
}

func pruneConfigHistory(configPath string, keep int) {
	snaps, err := listConfigSnapshots(configPath)
	if err != nil || len(snaps) <= keep {
		return
	}
	for _, s := range snaps[keep:] {
		os.RemoveAll(filepath.Join(configHistoryDir(configPath), s.ID))
	}
}

// restoreConfigSnapshot puts back the files a snapshot holds. It goes through
// writeConfigCandidates, so the restore is validated and can itself be undone.
func (a *application) restoreConfigSnapshot(id string) error {
	snap, err := readConfigSnapshot(a.configPath, id)
	if err != nil {
		return err
	}

	configDir := filepath.Dir(a.configPath)
	candidates := map[string][]byte{}
	for _, f := range snap.Files {
		if !f.Existed {
			// Files the save created are left alone; once the files that
			// included them are restored, nothing refers to them any more.
			continue
		}
		rel := filepath.FromSlash(f.Path)
		if strings.HasPrefix(filepath.Clean(rel), "..") {
			return fmt.Errorf("snapshot refers to a file outside the config folder")
		}
		data, err := os.ReadFile(filepath.Join(configHistoryDir(a.configPath), id, "files", rel))
		if err != nil {
			return err
		}
		candidates[filepath.Join(configDir, rel)] = data
	}

	return a.writeConfigCandidates(candidates)
}
