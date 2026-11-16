package dynacat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// More den-app screens: Den Duty's Parent Duty cards, Nighttime Ninja's
// running-low list, LootDen's "more game time" asks and Dendrop quick
// messages. Each one reads from the app's API and turns a button press into
// one call on it, like the screens in widget-den-app.go.

type denDutyParentCard struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	DueDate string `json:"dueDate"`
}

// Due is "Overdue", "Today" or a short date, from the home's own date.
func (c denDutyParentCard) Due() string {
	if c.DueDate == "" {
		return ""
	}
	today := time.Now().Format(time.DateOnly)
	switch {
	case c.DueDate < today:
		return "Overdue"
	case c.DueDate == today:
		return "Today"
	}
	if t, err := time.Parse(time.DateOnly, c.DueDate); err == nil {
		return t.Format("Jan 2")
	}
	return c.DueDate
}

func (c denDutyParentCard) Late() bool {
	return c.DueDate != "" && c.DueDate <= time.Now().Format(time.DateOnly)
}

type ninjaLowItem struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Icon           string   `json:"icon"`
	Quantity       *float64 `json:"quantity"`
	Unit           string   `json:"unit"`
	OnShoppingList bool     `json:"onShoppingList"`
}

func (i ninjaLowItem) Left() string {
	if i.Quantity == nil {
		return "low"
	}
	q := strings.TrimSuffix(strings.TrimRight(strconv.FormatFloat(*i.Quantity, 'f', 2, 64), "0"), ".")
	return strings.TrimSpace(q+" "+i.Unit) + " left"
}

type lootdenTimeRequest struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Minutes int    `json:"minutes"`
	Note    string `json:"note"`
	State   string `json:"state"`
}

type dendropMessage struct {
	Body       string `json:"body"`
	CreatedAt  string `json:"createdAt"`
	Deleted    bool   `json:"deleted"`
	KeyVersion *int   `json:"keyVersion"`
	Author     *struct {
		DisplayName string `json:"displayName"`
	} `json:"author"`
	Webhook *struct {
		Name string `json:"name"`
	} `json:"webhook"`
}

func (m dendropMessage) Who() string {
	switch {
	case m.Author != nil && m.Author.DisplayName != "":
		return m.Author.DisplayName
	case m.Webhook != nil && m.Webhook.Name != "":
		return m.Webhook.Name
	}
	return "Someone"
}

// Text is what to show for the message: encrypted ones can't be read here.
func (m dendropMessage) Text() string {
	if m.KeyVersion != nil {
		return "🔒 Encrypted message"
	}
	if m.Body == "" {
		return "📎 Sent a file"
	}
	return m.Body
}

func (m dendropMessage) Time() time.Time {
	t, _ := time.Parse(time.RFC3339, m.CreatedAt)
	return t
}

func (widget *denAppWidget) v1HomePath(rest string) string {
	return "/api/v1/homes/" + url.PathEscape(widget.HomeID) + rest
}

func (widget *denAppWidget) dendropMessagesPath() string {
	return "/api/bot/v1/channels/" + url.PathEscape(widget.Channel) + "/messages"
}

func (widget *denAppWidget) updateMore(ctx context.Context) error {
	switch {
	case widget.App == denAppDenDuty && widget.View == denViewParentDuty:
		cards, err := fetchDenApp[[]denDutyParentCard](ctx, widget, widget.v1HomePath("/parent-duty"))
		if err != nil {
			return err
		}
		cards = slices.DeleteFunc(cards, func(c denDutyParentCard) bool { return c.Status == "done" })
		// Soonest due first; cards without a date go last, in the app's order.
		slices.SortStableFunc(cards, func(a, b denDutyParentCard) int {
			da, db := cmpOr(a.DueDate, "9999"), cmpOr(b.DueDate, "9999")
			return strings.Compare(da, db)
		})
		widget.Duties = cards

	case widget.App == denAppNighttimeNinja && widget.View == denViewLow:
		items, err := fetchDenApp[[]ninjaLowItem](ctx, widget, widget.homePath("/items?low=true"))
		if err != nil {
			return err
		}
		widget.Low = items

	case widget.App == denAppLootDen:
		asks, err := fetchDenApp[[]lootdenTimeRequest](ctx, widget, "/api/time-requests")
		if err != nil {
			return err
		}
		widget.TimeAsks = slices.DeleteFunc(asks, func(a lootdenTimeRequest) bool { return a.State != "pending" })

	case widget.App == denAppDendrop:
		// The quick buttons work even when the bot isn't allowed to read the
		// channel, so a 403 here only leaves the message list empty.
		req, err := widget.newRequest(withSharedFetchMaxAge(ctx, time.Nanosecond), http.MethodGet, widget.dendropMessagesPath()+"?limit=5", nil)
		if err != nil {
			return err
		}
		status, body, err := fetchRequestBody(widget.client(), req)
		if err != nil {
			return err
		}
		widget.Chat = nil
		switch {
		case status == http.StatusForbidden:
		case status >= 300:
			return fmt.Errorf("dendrop said %d", status)
		default:
			var msgs []dendropMessage
			if err := json.Unmarshal(body, &msgs); err != nil {
				return err
			}
			widget.Chat = slices.DeleteFunc(msgs, func(m dendropMessage) bool { return m.Deleted })
		}
	}
	return nil
}

func (widget *denAppWidget) planMoreAction(action string, body map[string]string, user *authenticatedUser) (string, string, any, error) {
	switch {
	case widget.App == denAppDenDuty && widget.View == denViewParentDuty && action == "done":
		if body["id"] == "" {
			return "", "", nil, errors.New("id is required")
		}
		return http.MethodPatch, widget.v1HomePath("/parent-duty/" + url.PathEscape(body["id"])), map[string]string{"status": "done"}, nil

	case widget.App == denAppNighttimeNinja && widget.View == denViewLow && action == "add":
		// Add the item by its name as the app knows it, not text from the page.
		i := slices.IndexFunc(widget.Low, func(it ninjaLowItem) bool { return it.ID == body["id"] })
		if body["id"] == "" || i < 0 {
			return "", "", nil, errors.New("that item isn't running low any more")
		}
		return http.MethodPost, widget.homePath("/shopping"), map[string]string{"text": widget.Low[i].Name}, nil

	case widget.App == denAppLootDen && (action == "approve" || action == "decline"):
		if !userInGroups(user, widget.ApproveGroups) {
			return "", "", nil, errors.New("only grown-ups can answer these")
		}
		id, err := strconv.Atoi(body["id"])
		if err != nil || id <= 0 {
			return "", "", nil, errors.New("id is required")
		}
		return http.MethodPost, fmt.Sprintf("/api/time-requests/%d/%s", id, action), map[string]string{}, nil

	case widget.App == denAppDendrop && action == "post":
		// Only the messages written in the config can be sent, by their number.
		i, err := strconv.Atoi(body["id"])
		if err != nil || i < 0 || i >= len(widget.Messages) {
			return "", "", nil, errors.New("unknown message")
		}
		return http.MethodPost, widget.dendropMessagesPath(), map[string]string{"content": widget.Messages[i]}, nil
	}
	return "", "", nil, fmt.Errorf("unknown action %q", action)
}

func cmpOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
