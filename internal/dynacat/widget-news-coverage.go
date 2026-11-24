package dynacat

// DashDen: news-coverage widget. Groups headlines from many feeds (for example FreshRSS
// saved queries shared as RSS) into stories, rates each outlet's political lean and shows
// how a story is covered across the left, center and right, Ground News style.

import (
	"context"
	"fmt"
	"html/template"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"
)

var newsCoverageWidgetTemplate = mustParseTemplate("news-coverage.html", "widget-base.html")

type newsCoverageWidget struct {
	widgetBase     `yaml:",inline"`
	FeedRequests   []rssFeedRequest            `yaml:"feeds"`
	Sources        map[string]newsSourceRating `yaml:"sources"`
	BuiltinRatings *bool                       `yaml:"builtin-ratings"`
	Window         durationField               `yaml:"window"`
	Limit          int                         `yaml:"limit"`
	CollapseAfter  int                         `yaml:"collapse-after"`
	MinSources     int                         `yaml:"min-sources"`
	BlindspotShare float64                     `yaml:"blindspot-share"`
	BlindspotMin   int                         `yaml:"blindspot-min-sources"`
	Similarity     float64                     `yaml:"similarity"`
	HideBalance    bool                        `yaml:"hide-balance"`

	Stories    []*newsStory `yaml:"-"`
	Blindspots []*newsStory `yaml:"-"`
	Balance    newsCoverage `yaml:"-"`
	feeds      *rssWidget   `yaml:"-"`
	outlets    map[string]newsOutlet
	channels   map[string]newsOutlet `yaml:"-"`
}

// newsSourceRating is one entry of the `sources` option. It can be written as just the
// lean ("lean-left") or as a map with bias, name and factuality.
type newsSourceRating struct {
	Bias       string `yaml:"bias"`
	Name       string `yaml:"name"`
	Factuality string `yaml:"factuality"`
}

func (r *newsSourceRating) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		return node.Decode(&r.Bias)
	}
	type plain newsSourceRating
	return node.Decode((*plain)(r))
}

type newsOutlet struct {
	Name       string
	Bias       newsBias
	Factuality string
}

type newsArticle struct {
	Title       string
	Link        string
	Outlet      string
	Bias        newsBias
	Factuality  string
	PublishedAt time.Time

	domain string
	tokens map[string]float64
	vec    map[string]float64
	sum    float64
}

type newsCoverage struct {
	Left, Center, Right, Unrated int
	LeftPct, CenterPct, RightPct int
}

func (c newsCoverage) Rated() int { return c.Left + c.Center + c.Right }

func (c *newsCoverage) add(b newsBias) {
	if b == newsBiasUnrated {
		c.Unrated++
		return
	}
	switch b.side() {
	case -1:
		c.Left++
	case 1:
		c.Right++
	default:
		c.Center++
	}
}

func (c *newsCoverage) finish() {
	rated := c.Rated()
	if rated == 0 {
		return
	}
	c.LeftPct = int(math.Round(float64(c.Left) * 100 / float64(rated)))
	c.RightPct = int(math.Round(float64(c.Right) * 100 / float64(rated)))
	c.CenterPct = 100 - c.LeftPct - c.RightPct
}

type newsStory struct {
	Title    string
	Link     string
	Articles []*newsArticle
	Coverage newsCoverage
	Outlets  int
	Latest   time.Time
	// Blindspot is "left" when the story is covered almost only by the left (so the right
	// barely sees it), "right" for the opposite, or empty.
	Blindspot string
}

func (widget *newsCoverageWidget) initialize() error {
	widget.withTitle("News Coverage").withCacheDuration(30 * time.Minute)

	if widget.UpdateInterval == nil {
		interval := updateIntervalField(15 * time.Minute)
		widget.UpdateInterval = &interval
	}
	if len(widget.FeedRequests) == 0 {
		return fmt.Errorf("add at least one feed under feeds")
	}
	if widget.Window <= 0 {
		widget.Window = durationField(48 * time.Hour)
	}
	if widget.Limit <= 0 {
		widget.Limit = 20
	}
	if widget.CollapseAfter == 0 || widget.CollapseAfter < -1 {
		widget.CollapseAfter = 5
	}
	if widget.MinSources <= 0 {
		widget.MinSources = 1
	}
	if widget.BlindspotShare <= 0 || widget.BlindspotShare > 1 {
		widget.BlindspotShare = 0.8
	}
	if widget.BlindspotMin <= 0 {
		widget.BlindspotMin = 3
	}
	if widget.Similarity <= 0 || widget.Similarity >= 1 {
		widget.Similarity = 0.4
	}

	widget.outlets = make(map[string]newsOutlet)
	if widget.BuiltinRatings == nil || *widget.BuiltinRatings {
		for domain, o := range builtinNewsOutlets {
			widget.outlets[domain] = newsOutlet{Name: o.name, Bias: o.bias, Factuality: o.factuality}
		}
	}
	widget.channels = make(map[string]newsOutlet)
	for name, o := range builtinNewsChannels {
		widget.channels[newsNameKey(name)] = newsOutlet{Name: name, Bias: o.bias, Factuality: o.factuality}
	}
	for domain, rating := range widget.Sources {
		if !strings.Contains(domain, ".") {
			// A channel or show name, for video feeds: "Breaking Points: center".
			bias, ok := newsBiasNames[strings.ToLower(strings.TrimSpace(rating.Bias))]
			if !ok {
				return fmt.Errorf("sources: %s has bias %q; use left, lean-left, center, lean-right or right", domain, rating.Bias)
			}
			name := domain
			if rating.Name != "" {
				name = rating.Name
			}
			widget.channels[newsNameKey(domain)] = newsOutlet{Name: name, Bias: bias, Factuality: strings.ToLower(rating.Factuality)}
			continue
		}
		domain = normalizeNewsDomain(domain)
		bias := newsBiasUnrated
		if rating.Bias != "" {
			b, ok := newsBiasNames[strings.ToLower(strings.TrimSpace(rating.Bias))]
			if !ok {
				return fmt.Errorf("sources: %s has bias %q; use left, lean-left, center, lean-right or right", domain, rating.Bias)
			}
			bias = b
		}
		existing := widget.outlets[domain]
		if rating.Name != "" {
			existing.Name = rating.Name
		}
		if rating.Factuality != "" {
			existing.Factuality = strings.ToLower(rating.Factuality)
		}
		existing.Bias = bias
		widget.outlets[domain] = existing
	}

	requests := make([]rssFeedRequest, len(widget.FeedRequests))
	copy(requests, widget.FeedRequests)
	for i := range requests {
		requests[i].IsDetailed = true
		requests[i].HideCategories = true
	}
	widget.feeds = &rssWidget{FeedRequests: requests, cachedFeeds: make(map[string]*cachedRSSFeed)}

	return nil
}

func (widget *newsCoverageWidget) update(ctx context.Context) {
	items, err := widget.feeds.fetchItemsFromFeeds()
	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	cutoff := time.Now().Add(-time.Duration(widget.Window))
	articles := make([]*newsArticle, 0, len(items))
	for i := range items {
		if items[i].PublishedAt.Before(cutoff) {
			continue
		}
		articles = append(articles, widget.articleFromItem(&items[i]))
	}

	stories := clusterNewsArticles(articles, widget.Similarity)

	var balance newsCoverage
	for _, a := range articles {
		balance.add(a.Bias)
	}
	balance.finish()

	shown := make([]*newsStory, 0, len(stories))
	blindspots := make([]*newsStory, 0)
	for _, s := range stories {
		widget.finishStory(s)
		if s.Outlets < widget.MinSources {
			continue
		}
		shown = append(shown, s)
		if s.Blindspot != "" {
			blindspots = append(blindspots, s)
		}
	}
	if len(shown) > widget.Limit {
		shown = shown[:widget.Limit]
	}
	if len(blindspots) > widget.Limit {
		blindspots = blindspots[:widget.Limit]
	}

	widget.Stories = shown
	widget.Blindspots = blindspots
	widget.Balance = balance
}

// newsStoryList is what the template's story list needs.
type newsStoryList struct {
	Stories       []*newsStory
	CollapseAfter int
	Empty         string
}

func (widget *newsCoverageWidget) TopList() newsStoryList {
	return newsStoryList{widget.Stories, widget.CollapseAfter, "No stories in your feeds yet."}
}

func (widget *newsCoverageWidget) BlindspotList() newsStoryList {
	return newsStoryList{widget.Blindspots, widget.CollapseAfter, "No blindspots right now."}
}

func (widget *newsCoverageWidget) Render() template.HTML {
	return widget.renderTemplate(widget, newsCoverageWidgetTemplate)
}

// articleFromItem works out which outlet published an item from its link, since feeds
// that come through FreshRSS or another aggregator all share the aggregator's name.
func (widget *newsCoverageWidget) articleFromItem(item *rssFeedItem) *newsArticle {
	a := &newsArticle{
		Title:       strings.TrimSpace(item.Title),
		Link:        item.Link,
		PublishedAt: item.PublishedAt,
	}

	domain := ""
	if u, err := url.Parse(item.Link); err == nil {
		domain = normalizeNewsDomain(u.Hostname())
	}

	// Google News and similar put the outlet at the end of the title: "Headline - Reuters".
	if domain == "news.google.com" || domain == "" {
		if i := strings.LastIndex(a.Title, " - "); i > 0 {
			outletName := strings.TrimSpace(a.Title[i+3:])
			a.Title = strings.TrimSpace(a.Title[:i])
			if d, ok := widget.outletByName(outletName); ok {
				domain = d
			} else {
				a.Outlet = outletName
			}
		}
	}

	// Video sites: every link is youtube.com, so the channel is the outlet. Through
	// FreshRSS the channel is the item's author; in a channel's own feed it's the title.
	if newsVideoHosts[domain] {
		channel := ""
		for _, name := range []string{item.Author, item.ChannelName} {
			if name = strings.TrimSpace(name); name == "" {
				continue
			}
			if o, ok := widget.channels[newsNameKey(name)]; ok {
				a.Outlet, a.Bias, a.Factuality = o.Name, o.Bias, o.Factuality
				a.domain = "channel:" + newsNameKey(o.Name)
				a.tokens = newsTitleTokens(a.Title)
				return a
			}
			if d, ok := widget.outletByName(name); ok {
				domain, channel = d, ""
				break
			}
			if channel == "" {
				channel = name
			}
		}
		if channel != "" {
			a.Outlet = channel
			a.domain = "channel:" + newsNameKey(channel)
			a.tokens = newsTitleTokens(a.Title)
			return a
		}
	}

	a.domain = domain
	if o, d, ok := widget.lookupOutlet(domain); ok {
		a.domain = d
		a.Bias = o.Bias
		a.Factuality = o.Factuality
		if a.Outlet == "" {
			a.Outlet = o.Name
		}
	}
	if a.Outlet == "" {
		if domain != "" {
			a.Outlet = domain
		} else {
			a.Outlet = item.ChannelName
		}
	}

	a.tokens = newsTitleTokens(a.Title)
	return a
}

// lookupOutlet tries the domain, then each parent domain: edition.cnn.com, cnn.com.
func (widget *newsCoverageWidget) lookupOutlet(domain string) (newsOutlet, string, bool) {
	for d := domain; d != ""; {
		if o, ok := widget.outlets[d]; ok {
			return o, d, true
		}
		i := strings.IndexByte(d, '.')
		if i < 0 || strings.IndexByte(d[i+1:], '.') < 0 {
			break
		}
		d = d[i+1:]
	}
	return newsOutlet{}, domain, false
}

func (widget *newsCoverageWidget) outletByName(name string) (string, bool) {
	key := newsNameKey(name)
	if key == "" {
		return "", false
	}
	if d, ok := newsChannelDomains[key]; ok {
		if _, rated := widget.outlets[d]; rated {
			return d, true
		}
	}
	for d, o := range widget.outlets {
		if newsNameKey(o.Name) == key {
			return d, true
		}
	}
	return "", false
}

var newsNameKeyStrip = regexp.MustCompile(`[^a-z0-9]+`)

// newsNameKey makes outlet and channel names comparable: "The Daily Wire" and
// "daily wire" both become "dailywire".
func newsNameKey(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.TrimPrefix(name, "the ")
	return newsNameKeyStrip.ReplaceAllString(name, "")
}

var newsVideoHosts = map[string]bool{
	"youtube.com": true, "youtu.be": true, "music.youtube.com": true, "vimeo.com": true,
	"rumble.com": true, "odysee.com": true, "dailymotion.com": true, "tiktok.com": true,
}

func normalizeNewsDomain(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	if i := strings.IndexByte(host, '/'); i >= 0 {
		host = host[:i]
	}
	for _, p := range []string{"www.", "m.", "amp.", "mobile."} {
		host = strings.TrimPrefix(host, p)
	}
	return host
}

func (widget *newsCoverageWidget) finishStory(s *newsStory) {
	sort.SliceStable(s.Articles, func(i, j int) bool {
		return s.Articles[i].PublishedAt.After(s.Articles[j].PublishedAt)
	})

	seen := make(map[string]bool)
	for _, a := range s.Articles {
		key := a.domain
		if key == "" {
			key = a.Outlet
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		s.Coverage.add(a.Bias)
	}
	s.Outlets = len(seen)
	s.Coverage.finish()

	if rated := s.Coverage.Rated(); rated >= widget.BlindspotMin {
		left := float64(s.Coverage.Left) / float64(rated)
		right := float64(s.Coverage.Right) / float64(rated)
		if left >= widget.BlindspotShare {
			s.Blindspot = "left"
		} else if right >= widget.BlindspotShare {
			s.Blindspot = "right"
		}
	}

	// Headline: prefer a center outlet's wording, then the newest.
	lead := s.Articles[0]
	for _, a := range s.Articles {
		if a.Bias == newsBiasCenter {
			lead = a
			break
		}
	}
	s.Title = lead.Title
	s.Link = lead.Link
}

// clusterNewsArticles groups articles about the same story. Title words are weighted by
// how rare they are across all the articles (so "Trump" counts for less than "Gaza" when
// every other headline mentions Trump), and two headlines match by the weighted share of
// words they have in common (Dice). An article joins the story holding its closest match,
// if that reaches threshold and they share at least two words. Stories come back
// most-covered first.
func clusterNewsArticles(articles []*newsArticle, threshold float64) []*newsStory {
	df := make(map[string]int)
	for _, a := range articles {
		for t := range a.tokens {
			df[t]++
		}
	}
	n := float64(len(articles))
	for _, a := range articles {
		a.vec = make(map[string]float64, len(a.tokens))
		a.sum = 0
		for t, w := range a.tokens {
			v := w * math.Log(1+n/float64(df[t]))
			a.vec[t] = v
			a.sum += v
		}
	}

	// Oldest first, so a story's first article sets its topic.
	sorted := make([]*newsArticle, len(articles))
	copy(sorted, articles)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].PublishedAt.Before(sorted[j].PublishedAt) })

	var stories []*newsStory
	for _, a := range sorted {
		var best *newsStory
		bestScore := 0.0
		for _, s := range stories {
			for _, b := range s.Articles {
				if score := newsHeadlineMatch(a, b); score > bestScore {
					best, bestScore = s, score
				}
			}
		}

		if best == nil || bestScore < threshold {
			best = &newsStory{}
			stories = append(stories, best)
		}
		best.Articles = append(best.Articles, a)
		if a.PublishedAt.After(best.Latest) {
			best.Latest = a.PublishedAt
		}
	}

	stories = mergeNewsStories(stories)

	sort.SliceStable(stories, func(i, j int) bool {
		ci, cj := newsDistinctOutlets(stories[i]), newsDistinctOutlets(stories[j])
		if ci != cj {
			return ci > cj
		}
		return stories[i].Latest.After(stories[j].Latest)
	})
	return stories
}

// mergeNewsStories joins stories that are about the same event but were worded too
// differently to match headline by headline, such as "Nvidia stock surges as Jensen Huang
// rehired" and "Trump says he convinced Nvidia to reinstate Jensen Huang". Each story is
// reduced to its core words, the ones at least half its headlines use, and two stories
// merge when they share at least three core words and those carry most of the weight.
func mergeNewsStories(stories []*newsStory) []*newsStory {
	type core struct {
		vec map[string]float64
		sum float64
	}
	coreOf := func(s *newsStory) core {
		count := make(map[string]int)
		weight := make(map[string]float64)
		for _, a := range s.Articles {
			for t, v := range a.vec {
				count[t]++
				weight[t] = v
			}
		}
		c := core{vec: make(map[string]float64)}
		for t, n := range count {
			if n*2 >= len(s.Articles) {
				c.vec[t] = weight[t]
				c.sum += weight[t]
			}
		}
		return c
	}

	cores := make([]core, len(stories))
	for i, s := range stories {
		cores[i] = coreOf(s)
	}

	for merged := true; merged; {
		merged = false
		for i := 0; i < len(stories) && !merged; i++ {
			for j := i + 1; j < len(stories); j++ {
				a, b := cores[i], cores[j]
				if a.sum == 0 || b.sum == 0 {
					continue
				}
				shared, weight := 0, 0.0
				for t, v := range a.vec {
					if bv, ok := b.vec[t]; ok {
						shared++
						weight += v + bv
					}
				}
				if shared < 3 || weight/(a.sum+b.sum) < newsStoryMergeThreshold {
					continue
				}
				stories[i].Articles = append(stories[i].Articles, stories[j].Articles...)
				if stories[j].Latest.After(stories[i].Latest) {
					stories[i].Latest = stories[j].Latest
				}
				stories = append(stories[:j], stories[j+1:]...)
				cores = append(cores[:j], cores[j+1:]...)
				cores[i] = coreOf(stories[i])
				merged = true
				break
			}
		}
	}
	return stories
}

const newsStoryMergeThreshold = 0.5

// newsHeadlineMatch is the weighted Dice similarity of two headlines, from 0 to 1. It is
// 0 when they share fewer than two words.
func newsHeadlineMatch(a, b *newsArticle) float64 {
	if a.sum == 0 || b.sum == 0 {
		return 0
	}
	shared, sharedWeight := 0, 0.0
	for t, v := range a.vec {
		if bv, ok := b.vec[t]; ok {
			shared++
			sharedWeight += v + bv
		}
	}
	if shared < 2 {
		return 0
	}
	return sharedWeight / (a.sum + b.sum)
}

func newsDistinctOutlets(s *newsStory) int {
	seen := make(map[string]bool)
	for _, a := range s.Articles {
		seen[a.domain+"|"+a.Outlet] = true
	}
	return len(seen)
}

var newsTokenSplit = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// newsTitleTokens turns a headline into weighted words: lowercased, stop words dropped,
// simple plurals and possessives folded ("tariffs" and "tariff's" become "tariff").
func newsTitleTokens(text string) map[string]float64 {
	text = sanitizeFeedDescription(text)
	text = strings.ReplaceAll(text, "’", "'")
	text = strings.ReplaceAll(text, "'s ", " ")
	tokens := make(map[string]float64)
	for _, raw := range newsTokenSplit.Split(strings.ToLower(text), -1) {
		if raw == "" || newsStopWords[raw] {
			continue
		}
		isNumber := unicode.IsDigit([]rune(raw)[0])
		if !isNumber && len([]rune(raw)) < 3 {
			continue
		}
		t := raw
		if !isNumber && len(t) > 4 {
			switch {
			case strings.HasSuffix(t, "ies"):
				t = t[:len(t)-3] + "y"
			case strings.HasSuffix(t, "ses"), strings.HasSuffix(t, "xes"):
				t = t[:len(t)-2]
			case strings.HasSuffix(t, "s") && !strings.HasSuffix(t, "ss") && !strings.HasSuffix(t, "us"):
				t = t[:len(t)-1]
			}
		}
		if alias, ok := newsTokenAliases[t]; ok {
			t = alias
		} else if !isNumber {
			// "returning" and "returned" both become "return".
			switch {
			case len(t) >= 7 && strings.HasSuffix(t, "ing"):
				t = t[:len(t)-3]
			case len(t) >= 6 && strings.HasSuffix(t, "ed") && !strings.HasSuffix(t, "eed"):
				t = t[:len(t)-2]
			}
			if alias, ok := newsTokenAliases[t]; ok {
				t = alias
			}
		}
		tokens[t] = 1
	}
	return tokens
}

// newsTokenAliases folds words that name the same thing, so "Chinese tariffs" and "tariffs
// on China" match. Keys are after plural folding.
var newsTokenAliases = map[string]string{
	"chinese": "china", "beijing": "china", "canadian": "canada", "ottawa": "canada",
	"russian": "russia", "moscow": "russia", "kremlin": "russia", "ukrainian": "ukraine", "kyiv": "ukraine",
	"israeli": "israel", "palestinian": "palestine", "iranian": "iran", "tehran": "iran",
	"mexican": "mexico", "british": "britain", "uk": "britain", "european": "europe", "eu": "europe",
	"indian": "india", "japanese": "japan", "korean": "korea", "german": "germany", "french": "france",
	"venezuelan": "venezuela", "syrian": "syria", "taiwanese": "taiwan", "nato": "nato",
	"federal": "fed", "reserve": "fed", "gop": "republican", "republican": "republican",
	"democratic": "democrat", "dem": "democrat", "congressional": "congress", "lawmaker": "congress",
	"presidential": "president", "electoral": "election", "voter": "election", "voting": "vote",
	"tariff": "tariff", "trade": "tariff", "shutdown": "shutdown", "spending": "budget",
	"killed": "kill", "killing": "kill", "dead": "kill", "death": "kill", "die": "kill", "died": "kill",
	"strike": "attack", "attacked": "attack", "airstrike": "attack",
	"rehire": "rehire", "rehired": "rehire", "reinstate": "rehire", "reinstated": "rehire", "reinstating": "rehire",
	"fired": "fire", "oust": "fire", "ousted": "fire", "ouster": "fire", "chief": "ceo",
	"approve": "pass", "approved": "pass", "passed": "pass", "passe": "pass", "clear": "pass", "clears": "pass",
	"rate": "rate", "unchanged": "steady", "hold": "steady", "holds": "steady", "keep": "steady",
}

var newsStopWords = func() map[string]bool {
	words := `a about above after again against all also am an and any are as at be because been before
being below between both but by can could did do does doing down during each few for from further
had has have having he her here hers him his how i if in into is it its itself just me more most my
new news no nor not now of off on once only or other our ours out over own same she should so some
such than that the their them then there these they this those through to too under until up very
was we were what when where which while who whom why will with would you your says said say report
reports reported after amid over first last latest live update updates watch video photos new year
years day days week weeks back get gets got make makes made take takes may might could one two
just like via here's what's know need want see sees saying thank thanks`
	m := make(map[string]bool)
	for _, w := range strings.Fields(words) {
		m[w] = true
	}
	return m
}()
