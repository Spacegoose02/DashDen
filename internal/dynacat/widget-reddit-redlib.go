package dynacat

import (
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

// DashDen: Reddit ended its RSS feeds on Nov 13, 2026 and blocks signed-out API calls,
// so the reddit widget can read posts from a self-hosted Redlib instance instead.
// Redlib serves a subreddit as RSS at /r/<sub>.rss when REDLIB_ENABLE_RSS is set.

var redlibHTTPClient = &http.Client{Timeout: 15 * time.Second}

func (widget *redditWidget) initializeRedlib() error {
	if widget.RedlibURL == "" {
		return nil
	}

	parsed, err := url.Parse(widget.RedlibURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return errors.New("redlib-url must be a full http:// or https:// address, such as http://192.168.1.10:8080")
	}

	widget.RedlibURL = strings.TrimRight(widget.RedlibURL, "/")
	return nil
}

// fetchRedditPosts picks the source: Redlib alone when there is no app-auth, otherwise
// the Reddit API with Redlib (if set) or Reddit's old RSS feed as the fallback.
// The bool reports whether the posts came from a feed, which has no scores or comment counts.
func (widget *redditWidget) fetchRedditPosts() (forumPostList, bool, error) {
	if widget.RedlibURL != "" && !widget.AppAuth.enabled {
		posts, err := widget.fetchSubredditPostsFromRedlib()
		return posts, true, err
	}

	posts, err := widget.fetchSubredditPosts()
	if err == nil {
		return posts, false, nil
	}

	if widget.RedlibURL != "" {
		redlibPosts, redlibErr := widget.fetchSubredditPostsFromRedlib()
		if redlibErr != nil {
			return nil, false, fmt.Errorf("%w, redlib fallback also failed: %v", err, redlibErr)
		}
		return redlibPosts, true, nil
	}

	if widget.AppAuth.enabled {
		return nil, false, err
	}

	rssPosts, rssErr := widget.fetchSubredditPostsFromRSS()
	if rssErr != nil {
		return nil, false, fmt.Errorf("%w, rss fallback also failed: %v (Reddit ended RSS on Nov 13, 2026, set `redlib-url` to read through Redlib)", err, rssErr)
	}

	return rssPosts, true, nil
}

func (widget *redditWidget) redlibFeedURL() string {
	query := url.Values{}
	query.Set("sort", widget.SortBy)
	query.Set("limit", strconv.Itoa(widget.Limit))

	if widget.SortBy == "top" {
		query.Set("t", widget.TopPeriod)
	}

	return widget.RedlibURL + "/r/" + url.PathEscape(widget.Subreddit) + ".rss?" + query.Encode()
}

func (widget *redditWidget) fetchSubredditPostsFromRedlib() (forumPostList, error) {
	requestURL := widget.redlibFeedURL()

	request, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "DashDen")

	response, err := redlibHTTPClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("could not reach redlib: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("redlib returned status code %d for %s", response.StatusCode, requestURL)
	}

	if contentType := response.Header.Get("Content-Type"); strings.HasPrefix(contentType, "text/html") {
		return nil, errors.New("redlib sent a web page instead of a feed, set REDLIB_ENABLE_RSS=on on the Redlib container")
	}

	feed, err := feedParser.Parse(response.Body)
	if err != nil {
		return nil, fmt.Errorf("reading redlib feed: %w", err)
	}

	posts := widget.redlibFeedToPosts(feed)
	if len(posts) == 0 {
		return nil, errors.New("no posts found")
	}

	return posts, nil
}

var (
	redlibCommentsLinkPattern = regexp.MustCompile(`<a href=['"]([^'"]+)['"]>Comments</a>`)
	redlibPostPathPattern     = regexp.MustCompile(`^/r/([^/]+)/comments/([^/]+)`)
)

func (widget *redditWidget) redlibFeedToPosts(feed *gofeed.Feed) forumPostList {
	base, _ := url.Parse(widget.RedlibURL + "/")
	resolve := func(raw string) *url.URL {
		raw = html.UnescapeString(strings.TrimSpace(raw))
		if raw == "" {
			return nil
		}
		parsed, err := url.Parse(raw)
		if err != nil {
			return nil
		}
		if base != nil {
			parsed = base.ResolveReference(parsed)
		}
		return parsed
	}

	posts := make(forumPostList, 0, len(feed.Items))

	for _, item := range feed.Items {
		link := resolve(item.Link)

		var discussion *url.URL
		if matches := redlibCommentsLinkPattern.FindStringSubmatch(item.Description); matches != nil {
			discussion = resolve(matches[1])
		}

		// Galleries replace the Comments link, but their item link is the post itself.
		if discussion == nil && link != nil && redlibPostPathPattern.MatchString(link.Path) {
			discussion = link
		}

		if discussion == nil {
			continue
		}

		post := forumPost{
			Title:         html.UnescapeString(item.Title),
			Subreddit:     widget.Subreddit,
			DiscussionUrl: discussion.String(),
			StatsUnknown:  true,
		}

		if matches := redlibPostPathPattern.FindStringSubmatch(discussion.Path); matches != nil {
			post.Subreddit = matches[1]

			if widget.CommentsURLTemplate != "" {
				post.DiscussionUrl = widget.parseCustomCommentsURL(matches[1], matches[2], discussion.Path)
			}
		}

		if item.PublishedParsed != nil {
			post.TimePosted = *item.PublishedParsed
		}

		if link != nil && link.String() != discussion.String() {
			if base != nil && link.Host == base.Host {
				// A link back into Redlib is a crosspost or a Reddit-hosted page.
				if matches := redlibPostPathPattern.FindStringSubmatch(link.Path); matches != nil {
					post.IsCrosspost = matches[1] != post.Subreddit
					post.TargetUrl = link.String()
					post.TargetUrlDomain = "r/" + matches[1]
				}
			} else if link.Scheme == "http" || link.Scheme == "https" {
				post.TargetUrl = link.String()
				post.TargetUrlDomain = strings.TrimPrefix(link.Hostname(), "www.")
			}
		}

		for _, enclosure := range item.Enclosures {
			if enclosure == nil || !strings.HasPrefix(enclosure.Type, "image/") {
				continue
			}
			if thumbnail := resolve(enclosure.URL); thumbnail != nil {
				post.ThumbnailUrl = thumbnail.String()
				break
			}
		}

		posts = append(posts, post)
	}

	return posts
}
