package dynacat

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// Live updates: a change made on one screen shows up on every other open
// screen right away, instead of on that screen's next poll.
//
//   - Pressing a button in a notice board or den app widget re-renders
//     that widget, and every other widget showing the same thing, for
//     everyone who can see it.
//   - Saving a server-stored to-do list tells the other screens showing the
//     list to load it again; a personal list only reaches its owner.
//   - When the config changes the server restarts; open screens notice the
//     new start time when they reconnect and reload themselves.
//
// Everything rides the existing /api/sse/updates stream as widget-update
// events, told apart by their fields.

// liveKeyed widgets share what they show with other widgets that return the
// same key, such as two notice boards with the same id on different pages.
type liveKeyed interface {
	liveKey() string
}

func (widget *noticeBoardWidget) liveKey() string {
	return "notice-board:" + widget.BoardID
}

func (widget *denAppWidget) liveKey() string {
	return "den-app:" + widget.App + "|" + strings.TrimRight(widget.URL, "/") + "|" + widget.HomeID
}

type widgetUpdatePayload struct {
	WidgetID uint64 `json:"widgetId"`
	HTML     string `json:"html"`
}

// statusRecorder remembers the status a widget's action handler answered with.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

func (s *statusRecorder) succeeded() bool {
	return s.status == 0 || s.status < 300
}

// pushAfterWidgetAction runs once an action handler has finished with acted,
// with acted's page still locked by the caller. The push itself waits for
// that lock in the background.
func (a *application) pushAfterWidgetAction(acted widget) {
	if !a.DynamicUpdateEnabled {
		return
	}

	// Only widgets whose actions change shared state opt in; a calendar
	// fetching another month, for example, changes nothing for anyone else.
	keyed, ok := acted.(liveKeyed)
	if !ok {
		return
	}

	go func() {
		a.pushWidget(acted, false)

		key := keyed.liveKey()
		for id, other := range a.widgetByID {
			if id == acted.GetID() {
				continue
			}
			if k, ok := other.(liveKeyed); ok && k.liveKey() == key {
				a.pushWidget(other, true)
			}
		}
	}()
}

func (a *application) pushWidget(w widget, refresh bool) {
	pg, ok := a.widgetToPage[w.GetID()]
	if !ok {
		return
	}

	pg.mu.Lock()
	if refresh {
		w.update(withSharedFetchMaxAge(context.Background(), w.getCacheDuration()))
	}
	html := string(w.Render())
	pg.mu.Unlock()

	msg, err := json.Marshal(widgetUpdatePayload{WidgetID: w.GetID(), HTML: html})
	if err != nil {
		return
	}
	a.sseBroadcastWidgetUpdate(pg, w.GetID(), string(msg))
}

type todoChangedPayload struct {
	TodoID string `json:"todoId"`
	// From is the tab that saved, so it doesn't reload its own change.
	From string `json:"from,omitempty"`
}

// pushTodoChange tells other screens that a server-stored list changed.
// storageKey is the list id as authorizeTodoRequest returned it, which for
// a personal list carries its owner.
func (a *application) pushTodoChange(r *http.Request, storageKey string) {
	if !a.DynamicUpdateEnabled {
		return
	}

	listID, owner, personal := strings.Cut(storageKey, "\x1f")
	pg, ok := a.todoListIDToPage[listID]
	if !ok {
		return
	}

	from := r.Header.Get("X-DashDen-Tab")
	if len(from) > 64 {
		from = ""
	}
	msg, err := json.Marshal(todoChangedPayload{TodoID: listID, From: from})
	if err != nil {
		return
	}

	a.sseMu.RLock()
	defer a.sseMu.RUnlock()
	for c := range a.sseClients {
		if !a.canUserAccessPage(c.user, pg) {
			continue
		}
		if personal && (c.user == nil || c.user.Username != owner) {
			continue
		}
		select {
		case c.ch <- string(msg):
		default:
		}
	}
}

type appStartedPayload struct {
	AppStarted int64 `json:"appStarted"`
}

// sseHello greets a new connection with when this server started, so a
// screen opened before a config change knows to reload.
func (a *application) sseHello(c *sseClient) {
	msg, err := json.Marshal(appStartedPayload{AppStarted: a.CreatedAt.Unix()})
	if err != nil {
		return
	}
	select {
	case c.ch <- string(msg):
	default:
	}
}
