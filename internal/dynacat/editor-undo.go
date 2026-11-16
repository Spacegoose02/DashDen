package dynacat

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Undo in the editor. Every save already copies the files it changes into
// the config history (see settings-history.go). After a save the editor is
// told which copy that was, and offers Undo for a little while. Undo only
// works while nothing else has been saved since, so it never takes someone
// else's change with it, and only for the person who made the change.

const editorUndoWindow = 10 * time.Minute

type editorUndoOffer struct {
	id       string
	username string
	offered  time.Time
}

var editorUndo struct {
	mu sync.Mutex
	// The app is rebuilt after every save, so this lives outside it.
	offers map[string]editorUndoOffer
}

func undoUsername(user *authenticatedUser) string {
	if user == nil {
		return ""
	}
	return user.Username
}

// offerEditorUndo records the newest history entry, which the save that just
// finished made, for user and returns its id.
func (a *application) offerEditorUndo(user *authenticatedUser, savedAfter time.Time) string {
	snaps, err := listConfigSnapshots(a.configPath)
	if err != nil || len(snaps) == 0 || snaps[0].Created.Before(savedAfter) {
		return ""
	}
	id := snaps[0].ID

	editorUndo.mu.Lock()
	defer editorUndo.mu.Unlock()
	if editorUndo.offers == nil {
		editorUndo.offers = map[string]editorUndoOffer{}
	}
	for k, o := range editorUndo.offers {
		if time.Since(o.offered) > editorUndoWindow {
			delete(editorUndo.offers, k)
		}
	}
	editorUndo.offers[id] = editorUndoOffer{id: id, username: undoUsername(user), offered: time.Now()}
	return id
}

func (a *application) handleEditorUndo(w http.ResponseWriter, r *http.Request) {
	if a.handleUnauthorizedResponse(w, r, showUnauthorizedJSON) {
		return
	}
	user := a.getAuthenticatedUser(w, r)
	if !a.userCanEditAnything(user) {
		writeEditorForbidden(w)
		return
	}

	var body struct {
		ID string `json:"id"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	data, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(data, &body); err != nil || body.ID == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid body")
		return
	}

	if status, err := a.undoEditorChange(user, body.ID); err != nil {
		writeJSONError(w, status, err.Error())
		return
	}

	slog.Info("Editor change undone", "snapshot", body.ID, "user", undoUsername(user))
	writeJSON(w, http.StatusOK, map[string]any{"undone": body.ID})
}

// undoEditorChange restores the files from before the save with history
// entry id, if user made it and nothing has been saved since.
func (a *application) undoEditorChange(user *authenticatedUser, id string) (int, error) {
	editorUndo.mu.Lock()
	defer editorUndo.mu.Unlock()

	offer, ok := editorUndo.offers[id]
	if !ok || offer.username != undoUsername(user) || time.Since(offer.offered) > editorUndoWindow {
		return http.StatusConflict, errors.New("That change can't be undone from here any more. Settings, History has every earlier version.")
	}

	snaps, err := listConfigSnapshots(a.configPath)
	if err != nil || len(snaps) == 0 || snaps[0].ID != id {
		return http.StatusConflict, errors.New("Something else was saved after this change, so it can't be undone here. Settings, History has every earlier version.")
	}

	if err := a.restoreConfigSnapshot(id); err != nil {
		status := http.StatusInternalServerError
		if _, ok := err.(*editorValidationError); ok {
			status = http.StatusUnprocessableEntity
		}
		return status, fmt.Errorf("Undo failed: %w", err)
	}
	delete(editorUndo.offers, id)
	return http.StatusOK, nil
}
