package dynacat

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func newsTestFeed(items [][2]string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><rss version="2.0"><channel><title>FreshRSS query</title><link>https://rss.example</link>`)
	now := time.Now().Add(-time.Hour)
	for i, it := range items {
		fmt.Fprintf(&b, `<item><title>%s</title><link>%s</link><pubDate>%s</pubDate></item>`,
			it[0], it[1], now.Add(time.Duration(i)*time.Minute).Format(time.RFC1123Z))
	}
	b.WriteString(`</channel></rss>`)
	return b.String()
}

func newTestNewsCoverageWidget(t *testing.T, feedXML string, extraYAML string) *newsCoverageWidget {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(feedXML))
	}))
	t.Cleanup(srv.Close)

	w, err := newWidget("news-coverage")
	if err != nil {
		t.Fatal(err)
	}
	config := "type: news-coverage\nfeeds:\n  - url: " + srv.URL + "\n" + extraYAML
	if err := yaml.Unmarshal([]byte(config), w); err != nil {
		t.Fatal(err)
	}
	if err := w.initialize(); err != nil {
		t.Fatal(err)
	}
	nc := w.(*newsCoverageWidget)
	nc.update(context.Background())
	if nc.Error != nil {
		t.Fatalf("update: %v", nc.Error)
	}
	return nc
}

func TestNewsCoverageGroupsStoriesAcrossOutlets(t *testing.T) {
	feed := newsTestFeed([][2]string{
		{"Senate passes $1.2 trillion infrastructure bill after marathon vote", "https://www.reuters.com/world/us/senate-bill"},
		{"Senate passes infrastructure bill in 69-30 vote", "https://www.foxnews.com/politics/senate-bill"},
		{"Infrastructure bill passes the Senate after weeks of talks", "https://www.nytimes.com/2026/11/20/us/politics/senate-bill.html"},
		{"Apple unveils new iPhone with satellite messaging", "https://www.theverge.com/apple-iphone"},
		{"Wildfire forces thousands to evacuate in southern Spain", "https://www.bbc.co.uk/news/world-europe-1"},
	})
	w := newTestNewsCoverageWidget(t, feed, "")

	if len(w.Stories) != 3 {
		for _, s := range w.Stories {
			t.Logf("story %q with %d articles", s.Title, len(s.Articles))
		}
		t.Fatalf("got %d stories, want 3", len(w.Stories))
	}
	top := w.Stories[0]
	if top.Outlets != 3 {
		t.Fatalf("top story has %d outlets, want 3", top.Outlets)
	}
	if top.Coverage.Left != 1 || top.Coverage.Center != 1 || top.Coverage.Right != 1 {
		t.Fatalf("coverage = %+v, want one each", top.Coverage)
	}
	if !strings.Contains(top.Link, "reuters.com") {
		t.Errorf("headline should come from the center outlet, got %s", top.Link)
	}
	if top.Coverage.LeftPct+top.Coverage.CenterPct+top.Coverage.RightPct != 100 {
		t.Errorf("percentages don't add up: %+v", top.Coverage)
	}
	if w.Balance.Rated() != 5 {
		t.Errorf("balance counted %d rated articles, want 5", w.Balance.Rated())
	}

	html := string(w.Render())
	for _, want := range []string{"3 sources", "Reuters", "Fox News", "Lean left", "Your feeds lean", "Blindspots"} {
		if !strings.Contains(html, want) {
			t.Errorf("render is missing %q", want)
		}
	}
}

func TestNewsCoverageBlindspot(t *testing.T) {
	feed := newsTestFeed([][2]string{
		{"Border agents report record migrant crossings in Arizona sector", "https://www.foxnews.com/us/border"},
		{"Record migrant crossings reported at Arizona border sector", "https://www.dailywire.com/news/border"},
		{"Arizona border sector sees record migrant crossings", "https://nypost.com/2026/border"},
		{"Unrelated story about a cat show winner", "https://www.npr.org/cat"},
	})
	w := newTestNewsCoverageWidget(t, feed, "")

	if len(w.Blindspots) != 1 {
		t.Fatalf("got %d blindspots, want 1", len(w.Blindspots))
	}
	if w.Blindspots[0].Blindspot != "right" {
		t.Errorf("blindspot side = %q, want right", w.Blindspots[0].Blindspot)
	}
	if !strings.Contains(string(w.Render()), "Mostly right") {
		t.Error("render is missing the Mostly right badge")
	}
}

func TestNewsCoverageSourceOverridesAndSubdomains(t *testing.T) {
	feed := newsTestFeed([][2]string{
		{"City council approves new bike lanes downtown", "https://edition.cnn.com/local/bikes"},
		{"Council approves downtown bike lanes plan", "https://www.localpaper.example/bikes"},
		{"Downtown bike lanes approved by city council - Reuters", "https://news.google.com/rss/articles/abc"},
	})
	w := newTestNewsCoverageWidget(t, feed, `sources:
  localpaper.example: { bias: lean-right, name: Local Paper, factuality: high }
  cnn.com: center
`)
	if len(w.Stories) != 1 {
		t.Fatalf("got %d stories, want 1", len(w.Stories))
	}
	byOutlet := map[string]newsBias{}
	for _, a := range w.Stories[0].Articles {
		byOutlet[a.Outlet] = a.Bias
	}
	if byOutlet["CNN"] != newsBiasCenter {
		t.Errorf("cnn override not applied through subdomain: %v", byOutlet)
	}
	if byOutlet["Local Paper"] != newsBiasLeanRight {
		t.Errorf("custom outlet not rated: %v", byOutlet)
	}
	if byOutlet["Reuters"] != newsBiasCenter {
		t.Errorf("Google News title suffix not matched to Reuters: %v", byOutlet)
	}
}

func TestNewsCoverageRejectsUnknownBias(t *testing.T) {
	w := &newsCoverageWidget{
		FeedRequests: []rssFeedRequest{{URL: "https://example.com/feed"}},
		Sources:      map[string]newsSourceRating{"example.com": {Bias: "far-left"}},
	}
	if err := w.initialize(); err == nil {
		t.Fatal("want an error for an unknown bias")
	}
}

func TestNewsTitleTokensFoldsPluralsAndStopWords(t *testing.T) {
	got := newsTitleTokens("The tariffs: Trump's new tariffs hit 3 countries")
	for _, want := range []string{"tariff", "trump", "hit", "country", "3"} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing token %q in %v", want, got)
		}
	}
	for _, unwanted := range []string{"the", "new", "tariffs"} {
		if _, ok := got[unwanted]; ok {
			t.Errorf("unexpected token %q", unwanted)
		}
	}
}

// A day's worth of headlines: same-story pairs worded differently must group, and
// different stories about the same people (Trump tariffs on China vs on Canada) must not.
func TestNewsCoverageClusteringSample(t *testing.T) {
	titles := []string{
		"Trump announces new tariffs on Chinese electronics",
		"Trump slaps 25% tariffs on Chinese electronics imports",
		"China vows retaliation after Trump tariff move on electronics",
		"Trump threatens tariffs on Canadian lumber",
		"Canada responds to Trump's lumber tariff threat",
		"Trump says he will meet Putin next month",
		"Putin and Trump to meet in Budapest, Kremlin says",
		"Zelensky warns against deal made without Ukraine",
		"Fed holds interest rates steady, signals cuts in 2027",
		"Federal Reserve keeps rates unchanged as inflation cools",
		"Stocks rally after Fed decision",
		"Hurricane Melissa strengthens to Category 4 near Jamaica",
		"Jamaica braces as Hurricane Melissa nears",
		"Taylor Swift announces new album",
		"House Republicans unveil spending bill to avert shutdown",
		"Government shutdown looms as House GOP releases spending plan",
		"Supreme Court to hear birthright citizenship case",
		"Supreme Court agrees to take up Trump birthright citizenship order",
	}
	var as []*newsArticle
	for i, ti := range titles {
		as = append(as, &newsArticle{Title: ti, Outlet: fmt.Sprint(i), PublishedAt: time.Now().Add(time.Duration(i) * time.Minute), tokens: newsTitleTokens(ti)})
	}
	stories := clusterNewsArticles(as, 0.4)
	storyOf := map[string]*newsStory{}
	for _, s := range stories {
		for _, a := range s.Articles {
			storyOf[a.Title] = s
		}
	}
	same := func(a, b int) bool { return storyOf[titles[a]] == storyOf[titles[b]] }
	for _, pair := range [][2]int{{0, 1}, {0, 2}, {3, 4}, {5, 6}, {8, 9}, {11, 12}, {14, 15}, {16, 17}} {
		if !same(pair[0], pair[1]) {
			t.Errorf("want same story: %q and %q", titles[pair[0]], titles[pair[1]])
		}
	}
	for _, pair := range [][2]int{{0, 3}, {5, 7}, {9, 10}, {13, 0}} {
		if same(pair[0], pair[1]) {
			t.Errorf("want different stories: %q and %q", titles[pair[0]], titles[pair[1]])
		}
	}
	if len(stories) != 10 {
		t.Errorf("got %d stories, want 10", len(stories))
	}
}
