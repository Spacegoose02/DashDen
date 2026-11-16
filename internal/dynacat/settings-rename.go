package dynacat

// DashDen: change someone's username from Settings > People. The new name
// takes over their profile, password, page and widget access, theme, notes and
// to-dos, and the person doing it stays signed in if it's their own name.

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func (a *application) registerSettingsRenameRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/settings/people/{username}/rename", a.settingsJSON(a.handleSettingsPersonRename))
}

// Lists of people in the config: allowed-users, editing-users and the den
// app and notice board groups, which also take usernames.
func listsUsernames(key string) bool {
	switch key {
	case "action-groups", "approve-groups", "post-groups":
		return true
	}
	return strings.HasSuffix(key, "-users")
}

func (a *application) handleSettingsPersonRename(w http.ResponseWriter, r *http.Request) {
	oldName := r.PathValue("username")
	var in struct {
		NewUsername string `json:"newUsername"`
	}
	if err := decodeSettingsBody(r, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	newName := strings.TrimSpace(in.NewUsername)
	if !settingsUsernamePattern.MatchString(newName) {
		writeJSONError(w, http.StatusBadRequest, "Usernames are letters, numbers, dots, dashes, underscores and @, and match what the person signs in with.")
		return
	}

	// Sign-in ignores capital letters, so find the spelling in use.
	memberName, member := a.findHouseholdMember(oldName)
	userName, authUser := a.findPasswordUser(oldName)
	switch {
	case member != nil:
		oldName = memberName
	case authUser != nil:
		oldName = userName
	default:
		writeJSONError(w, http.StatusNotFound, "There's no one called "+oldName+".")
		return
	}
	if newName == oldName {
		writeJSON(w, http.StatusOK, map[string]any{"renamed": newName})
		return
	}
	if !strings.EqualFold(newName, oldName) {
		if taken, m := a.findHouseholdMember(newName); m != nil {
			writeSettingsWriteError(w, &settingsRefusal{"Someone already has the username " + taken + "."})
			return
		}
		if taken, u := a.findPasswordUser(newName); u != nil {
			writeSettingsWriteError(w, &settingsRefusal{"Someone already signs in as " + taken + "."})
			return
		}
	}

	if err := a.renameInConfigFiles(oldName, newName); err != nil {
		writeSettingsWriteError(w, err)
		return
	}

	carried := true
	if err := renameUserData(appDBPath(&a.Config), oldName, newName); err != nil {
		carried = false
		slog.Error("Username changed, but saved themes, notes or to-dos could not be moved", "from", oldName, "to", newName, "error", err)
	}
	sharedOIDCSessions.renameUser(oldName, newName)

	// Password sessions are tied to the name, so give the person doing this
	// a fresh one when they renamed themselves.
	you := a.settingsUser(w, r)
	if you != nil && strings.EqualFold(you.Username, oldName) && !you.IsOIDC && !you.IsProxy && authUser != nil {
		a.reissueSessionAfterRename(w, r, newName, authUser)
	}

	slog.Info("Username changed from Settings", "from", oldName, "to", newName)
	writeJSON(w, http.StatusOK, map[string]any{"renamed": newName, "dataCarried": carried})
}

func (a *application) reissueSessionAfterRename(w http.ResponseWriter, r *http.Request, newName string, u *user) {
	remember := false
	if c, err := r.Cookie(AUTH_SESSION_COOKIE_NAME); err == nil {
		if _, _, rem, err := verifySessionTokenFor(c.Value, a.authSecretKey, time.Now()); err == nil {
			remember = rem
		}
	}
	hash, err := computeUsernameHash(newName, u.credential, a.authSecretKey)
	if err != nil {
		slog.Error("Could not keep you signed in after the rename", "error", err)
		return
	}
	token, err := generateSessionTokenFor(hash, a.authSecretKey, time.Now(), remember)
	if err != nil {
		slog.Error("Could not keep you signed in after the rename", "error", err)
		return
	}
	a.setSessionCookie(w, r, token, remember)
}

// renameInConfigFiles changes the name in the main config and every file it
// includes, in one write that is checked and can be undone like any other.
func (a *application) renameInConfigFiles(oldName, newName string) error {
	a.editorMu.Lock()
	defer a.editorMu.Unlock()

	mainPath := filepath.Clean(a.configPath)
	mainDoc, err := loadYAMLDocument(mainPath)
	if err != nil {
		return err
	}
	root := documentRoot(mainDoc)
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("the config file isn't a YAML mapping")
	}

	renamed := false
	for _, path := range [][]string{{"household", "members"}, {"auth", "users"}} {
		m := getMappingValue(getMappingValue(root, path[0]), path[1])
		if renameMappingKey(m, oldName, newName) {
			renamed = true
		}
	}
	if !renamed {
		return &settingsRefusal{oldName + " isn't set up in the main config file, so DashDen can't rename them here."}
	}

	candidates := map[string][]byte{}
	renameInLists(mainDoc, oldName, newName)
	b, err := marshalDocument(mainDoc)
	if err != nil {
		return err
	}
	candidates[mainPath] = b

	if _, includes, err := parseYAMLIncludes(mainPath); err == nil {
		for path := range includes {
			path = filepath.Clean(path)
			if abs, err := filepath.Abs(path); err == nil && abs == mustAbs(mainPath) {
				continue
			}
			doc, err := loadYAMLDocument(path)
			if err != nil {
				continue
			}
			if !renameInLists(doc, oldName, newName) {
				continue
			}
			if b, err := marshalDocument(doc); err == nil {
				candidates[path] = b
			}
		}
	}
	return a.writeConfigCandidates(candidates)
}

func mustAbs(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// renameMappingKey renames the key matching oldName, ignoring case.
func renameMappingKey(m *yaml.Node, oldName, newName string) bool {
	if m == nil || m.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if strings.EqualFold(m.Content[i].Value, oldName) {
			m.Content[i].Value = newName
			return true
		}
	}
	return false
}

// renameInLists swaps oldName for newName in every list of people.
func renameInLists(n *yaml.Node, oldName, newName string) bool {
	if n == nil {
		return false
	}
	changed := false
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, value := n.Content[i], n.Content[i+1]
			if listsUsernames(key.Value) && value.Kind == yaml.SequenceNode {
				if renameInSequence(value, oldName, newName) {
					changed = true
				}
				continue
			}
			if renameInLists(value, oldName, newName) {
				changed = true
			}
		}
		return changed
	}
	for _, c := range n.Content {
		if renameInLists(c, oldName, newName) {
			changed = true
		}
	}
	return changed
}

func renameInSequence(seq *yaml.Node, oldName, newName string) bool {
	changed := false
	kept := seq.Content[:0]
	seen := map[string]bool{}
	for _, item := range seq.Content {
		if item.Kind == yaml.ScalarNode {
			if strings.EqualFold(item.Value, oldName) {
				item.Value = newName
				changed = true
			}
			key := strings.ToLower(item.Value)
			if strings.EqualFold(item.Value, newName) && seen[key] {
				changed = true
				continue
			}
			seen[key] = true
		}
		kept = append(kept, item)
	}
	seq.Content = kept
	return changed
}

func appDBPath(c *config) string {
	if c.Server.DBPath != "" {
		return c.Server.DBPath
	}
	return filepath.Join(c.assetsPath(), "dynacat.db")
}

// renameUserData moves saved themes, notes and per-person to-do lists over to
// the new name. Tables that don't exist yet are skipped.
func renameUserData(dbPath, oldName, newName string) error {
	if _, err := os.Stat(dbPath); err != nil {
		return nil
	}
	db, err := openSQLite(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	exists := func(table string) bool {
		var n int
		tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&n)
		return n > 0
	}
	caseOnly := strings.EqualFold(oldName, newName)

	if exists("user_prefs") {
		if !caseOnly {
			if _, err := tx.Exec(`DELETE FROM user_prefs WHERE username = ? COLLATE NOCASE`, newName); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`UPDATE user_prefs SET username = ? WHERE username = ? COLLATE NOCASE`, newName, oldName); err != nil {
			return err
		}
	}
	if exists("notices") {
		if _, err := tx.Exec(`UPDATE notices SET username = ? WHERE username = ? COLLATE NOCASE`, newName, oldName); err != nil {
			return err
		}
	}
	if exists("todo_tasks") {
		if err := renameTodoLists(tx, oldName, newName); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func renameTodoLists(tx *sql.Tx, oldName, newName string) error {
	rows, err := tx.Query(`SELECT DISTINCT list_id FROM todo_tasks WHERE instr(list_id, char(31)) > 0`)
	if err != nil {
		return err
	}
	var lists []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		lists = append(lists, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range lists {
		listID, owner, _ := strings.Cut(id, "\x1f")
		if !strings.EqualFold(owner, oldName) {
			continue
		}
		target := listID + "\x1f" + newName
		if target == id {
			continue
		}
		// A leftover list under the new name would collide, and the person's own list wins.
		if _, err := tx.Exec(`DELETE FROM todo_tasks WHERE list_id = ?`, target); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE todo_tasks SET list_id = ? WHERE list_id = ?`, target, id); err != nil {
			return err
		}
	}
	return nil
}

// renameUser moves signed-in provider sessions to the new name.
func (s *sessionStore) renameUser(oldName, newName string) {
	s.sessions.Range(func(k, v any) bool {
		if sess, ok := v.(*oidcSession); ok && strings.EqualFold(sess.Username, oldName) {
			moved := *sess
			moved.Username = newName
			s.sessions.Store(k, &moved)
		}
		return true
	})
}
