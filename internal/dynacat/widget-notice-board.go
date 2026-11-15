package dynacat

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var noticeBoardWidgetTemplate = mustParseTemplate("notice-board.html", "widget-base.html")

const noticeMaxLength = 280

var noticeBoardIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// noticeBoardWidget is the family fridge door: short notes anyone allowed
// can post, shown with who wrote them and when. Notes expire after keep-for.
type noticeBoardWidget struct {
	widgetBase    `yaml:",inline"`
	BoardID       string        `yaml:"id"`
	PostGroups    []string      `yaml:"post-groups"`
	KeepFor       durationField `yaml:"keep-for"`
	MaxNotes      int           `yaml:"max-notes"`
	CollapseAfter int           `yaml:"collapse-after"`

	Notes []notice `yaml:"-"`
}

func (widget *noticeBoardWidget) initialize() error {
	if !noticeBoardIDPattern.MatchString(widget.BoardID) {
		return errors.New("id is required: letters, numbers, dashes and underscores, e.g. family")
	}
	if widget.KeepFor == 0 {
		widget.KeepFor = durationField(7 * 24 * time.Hour)
	}
	if widget.MaxNotes <= 0 {
		widget.MaxNotes = 20
	}
	if widget.CollapseAfter == 0 {
		widget.CollapseAfter = 5
	}
	widget.withTitle("Notice board").withCacheDuration(time.Minute)
	return nil
}

func (widget *noticeBoardWidget) storage() *noticeStorage {
	if widget.Providers == nil || widget.Providers.app == nil {
		return nil
	}
	return widget.Providers.app.noticeStorage
}

func (widget *noticeBoardWidget) update(ctx context.Context) {
	s := widget.storage()
	if s == nil {
		widget.canContinueUpdateAfterHandlingErr(errors.New("notice storage isn't available"))
		return
	}
	notes, err := s.list(widget.BoardID, time.Duration(widget.KeepFor), widget.MaxNotes)
	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}
	widget.Notes = notes
}

func (widget *noticeBoardWidget) Render() template.HTML {
	return widget.renderTemplate(widget, noticeBoardWidgetTemplate)
}

func (n notice) AvatarIsImage() bool {
	return strings.HasPrefix(n.Avatar, "/") || strings.HasPrefix(n.Avatar, "https://") || strings.HasPrefix(n.Avatar, "http://")
}

func (n notice) Initial() string {
	r, _ := utf8.DecodeRuneInString(strings.TrimSpace(n.Author))
	if r == utf8.RuneError {
		return "?"
	}
	return strings.ToUpper(string(r))
}

func (widget *noticeBoardWidget) handleRequest(w http.ResponseWriter, r *http.Request) {
	s := widget.storage()
	if s == nil {
		http.Error(w, "notice storage isn't available", http.StatusServiceUnavailable)
		return
	}
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		http.Error(w, "expected application/json", http.StatusUnsupportedMediaType)
		return
	}
	var body map[string]string
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	var user *authenticatedUser
	if widget.Providers != nil && widget.Providers.app != nil {
		user = widget.Providers.app.getAuthenticatedUser(w, r)
	}

	switch strings.Trim(r.PathValue("action"), "/") {
	case "post":
		if !userInGroups(user, widget.PostGroups) {
			http.Error(w, "You can't post on this board.", http.StatusForbidden)
			return
		}
		text := strings.TrimSpace(body["text"])
		if text == "" {
			http.Error(w, "Write something first.", http.StatusBadRequest)
			return
		}
		if utf8.RuneCountInString(text) > noticeMaxLength {
			http.Error(w, "Notes can be at most "+strconv.Itoa(noticeMaxLength)+" characters.", http.StatusBadRequest)
			return
		}
		n := notice{Board: widget.BoardID, Text: text, Author: "Someone"}
		if user != nil {
			n.Username = user.Username
			n.Author = user.DisplayName()
			if m := user.Member(); m != nil {
				n.Avatar = m.Avatar
			}
		}
		if err := s.add(n); err != nil {
			http.Error(w, "Couldn't save the note.", http.StatusInternalServerError)
			return
		}

	case "delete":
		id, err := strconv.ParseInt(body["id"], 10, 64)
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		existing, err := s.get(widget.BoardID, id)
		if err != nil {
			http.Error(w, "Couldn't read the note.", http.StatusInternalServerError)
			return
		}
		if existing == nil {
			break
		}
		// With sign-in, only the author or an admin can take a note down.
		if user != nil && existing.Username != user.Username && !user.IsAdmin() {
			http.Error(w, "Only the person who wrote it, or an admin, can remove it.", http.StatusForbidden)
			return
		}
		if user == nil && existing.Username != "" {
			http.Error(w, "Sign in to remove this note.", http.StatusForbidden)
			return
		}
		if err := s.remove(widget.BoardID, id); err != nil {
			http.Error(w, "Couldn't remove the note.", http.StatusInternalServerError)
			return
		}

	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
		return
	}

	widget.update(r.Context())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(widget.Render()))
}
