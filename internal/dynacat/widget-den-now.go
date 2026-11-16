package dynacat

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

var denNowWidgetTemplate = mustParseTemplate("den-now.html", "widget-base.html")

// denNowWidget answers "what's on in the house right now?" by putting
// BingeBandit streams, LootDen games and Wavelength music in one list.
type denNowWidget struct {
	widgetBase `yaml:",inline"`
	Sources    []denNowSource `yaml:"sources"`

	Items  []denNowItem `yaml:"-"`
	Failed []string     `yaml:"-"`
}

type denNowSource struct {
	App           string `yaml:"app"`
	URL           string `yaml:"url"`
	Token         string `yaml:"token"`
	Name          string `yaml:"name"`
	AllowInsecure bool   `yaml:"allow-insecure"`
}

const (
	denNowBingeBandit = "bingebandit"
	denNowLootDen     = "lootden"
	denNowWavelength  = "wavelength"
)

var denNowApps = []string{denNowBingeBandit, denNowLootDen, denNowWavelength}

type denNowItem struct {
	Kind     string // watching, playing, listening
	Who      string
	Title    string
	Subtitle string
	Where    string
	ImageURL string
	Link     string
	Paused   bool
	Percent  int
	Source   string

	allowInsecure bool
}

func (widget *denNowWidget) initialize() error {
	if len(widget.Sources) == 0 {
		return errors.New("add at least one source")
	}
	for i := range widget.Sources {
		s := &widget.Sources[i]
		if !slices.Contains(denNowApps, s.App) {
			return fmt.Errorf("sources[%d]: app must be one of %s", i, strings.Join(denNowApps, ", "))
		}
		if s.URL == "" || s.Token == "" {
			return fmt.Errorf("sources[%d]: url and token are required", i)
		}
		s.URL = strings.TrimRight(s.URL, "/")
		if s.Name == "" {
			s.Name = map[string]string{denNowBingeBandit: "BingeBandit", denNowLootDen: "LootDen", denNowWavelength: "Wavelength"}[s.App]
		}
	}
	widget.withTitle("Now in the Den").withCacheDuration(30 * time.Second)
	return nil
}

func (widget *denNowWidget) update(ctx context.Context) {
	results := make([][]denNowItem, len(widget.Sources))
	errs := make([]error, len(widget.Sources))

	var wg sync.WaitGroup
	for i, source := range widget.Sources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = source.fetch(ctx)
		}()
	}
	wg.Wait()

	items := []denNowItem{}
	failed := []string{}
	var firstErr error
	for i, r := range results {
		if errs[i] != nil {
			failed = append(failed, widget.Sources[i].Name)
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", widget.Sources[i].Name, errs[i])
			}
			continue
		}
		items = append(items, r...)
	}

	// Playing things first, paused after.
	slices.SortStableFunc(items, func(a, b denNowItem) int {
		if a.Paused == b.Paused {
			return 0
		}
		if a.Paused {
			return 1
		}
		return -1
	})

	var err error
	if len(failed) == len(widget.Sources) {
		err = firstErr
	} else if firstErr != nil {
		err = fmt.Errorf("%w: %w", errPartialContent, firstErr)
	}
	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	if widget.Providers != nil {
		for i := range items {
			if items[i].ImageURL != "" {
				items[i].ImageURL = widget.Providers.SecureImageURL(ctx, items[i].ImageURL, items[i].allowInsecure)
			}
		}
	}

	widget.Items = items
	widget.Failed = failed
}

func (s denNowSource) get(ctx context.Context, path string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(withSharedFetchMaxAge(ctx, 10*time.Second), http.MethodGet, s.URL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.Token)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func (s denNowSource) client() requestDoer {
	return ternary[requestDoer](s.AllowInsecure, defaultInsecureHTTPClient, defaultHTTPClient)
}

func percentOf(position, duration float64) int {
	if duration <= 0 {
		return 0
	}
	return int(min(100, max(0, position*100/duration)))
}

func (s denNowSource) fetch(ctx context.Context) ([]denNowItem, error) {
	switch s.App {
	case denNowBingeBandit:
		req, err := s.get(ctx, "/api/admin/activity")
		if err != nil {
			return nil, err
		}
		res, err := decodeJsonFromRequest[struct {
			NowPlaying []struct {
				Title    string  `json:"title"`
				Subtitle string  `json:"subtitle"`
				Name     string  `json:"name"`
				Client   string  `json:"client"`
				Position float64 `json:"position"`
				Duration float64 `json:"duration"`
				Paused   bool    `json:"paused"`
				Poster   string  `json:"poster"`
				ItemID   int64   `json:"itemId"`
			} `json:"nowPlaying"`
		}](s.client(), req)
		if err != nil {
			return nil, err
		}
		items := make([]denNowItem, 0, len(res.NowPlaying))
		for _, p := range res.NowPlaying {
			items = append(items, denNowItem{
				Kind: "watching", Who: p.Name, Title: p.Title, Subtitle: p.Subtitle,
				Where: p.Client, Paused: p.Paused, Percent: percentOf(p.Position, p.Duration),
				ImageURL: s.bingeBanditPoster(p.Poster), Link: s.bingeBanditItem(p.ItemID), Source: s.Name,
			})
		}
		return items, nil

	case denNowLootDen:
		req, err := s.get(ctx, "/api/family/now-playing")
		if err != nil {
			return nil, err
		}
		res, err := decodeJsonFromRequest[[]struct {
			Name             string `json:"name"`
			Device           string `json:"device"`
			Title            string `json:"title"`
			CoverURL         string `json:"coverUrl"`
			Href             string `json:"href"`
			RemainingSeconds *int   `json:"remainingSeconds"`
		}](s.client(), req)
		if err != nil {
			return nil, err
		}
		items := make([]denNowItem, 0, len(res))
		for _, p := range res {
			item := denNowItem{
				Kind: "playing", Who: p.Name, Title: p.Title, Where: p.Device,
				ImageURL: s.absolute(p.CoverURL), Link: s.absolute(p.Href), Source: s.Name,
				allowInsecure: s.AllowInsecure,
			}
			if p.RemainingSeconds != nil {
				item.Subtitle = fmt.Sprintf("%d min of screen time left", max(0, *p.RemainingSeconds)/60)
			}
			items = append(items, item)
		}
		return items, nil

	case denNowWavelength:
		req, err := s.get(ctx, "/api/ha/players")
		if err != nil {
			return nil, err
		}
		res, err := decodeJsonFromRequest[wavelengthPlayersResponse](s.client(), req)
		if err != nil {
			return nil, err
		}
		items := []denNowItem{}
		for _, p := range res.Players {
			if p.Title == "" || (p.State != "playing" && p.State != "paused") {
				continue
			}
			items = append(items, denNowItem{
				Kind: "listening", Who: p.Name, Title: p.Title, Subtitle: p.Artist,
				Paused: p.State == "paused", Percent: p.Percent(), Link: s.URL, Source: s.Name,
			})
		}
		return items, nil
	}
	return nil, errors.New("unknown app")
}

// bingeBanditPoster turns a now-playing poster path into a URL DashDen can
// fetch. BingeBandit's images need sign-in and an <img> can't send a header,
// so the key goes in ?token=. The URL only ever goes to DashDen's image cache,
// which serves its own copy, so the key doesn't reach the browser.
func (s denNowSource) bingeBanditPoster(poster string) string {
	u := s.absolute(poster)
	if u == "" {
		return ""
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return ""
	}
	q := parsed.Query()
	q.Set("w", "w185")
	q.Set("token", s.Token)
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

func (s denNowSource) bingeBanditItem(id int64) string {
	if id <= 0 {
		return s.URL
	}
	return fmt.Sprintf("%s/item/%d", s.URL, id)
}

func (s denNowSource) absolute(u string) string {
	if u == "" || strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	if strings.HasPrefix(u, "/") {
		return s.URL + u
	}
	return ""
}

func (widget *denNowWidget) Render() template.HTML {
	return widget.renderTemplate(widget, denNowWidgetTemplate)
}
