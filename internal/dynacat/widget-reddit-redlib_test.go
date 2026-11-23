package dynacat

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Shaped like Redlib's /r/<sub>.rss output with REDLIB_FULL_URL unset (relative links).
const redlibTestFeed = `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
<channel>
<title>selfhosted</title>
<link>/r/selfhosted</link>
<description>A place to share self-hosted alternatives</description>
<item>
<title>Article &amp;amp; link post</title>
<link>https://www.example.com/story</link>
<description>&lt;a href='/r/selfhosted/comments/abc123/article_link_post/'&gt;Comments&lt;/a&gt;</description>
<author>someone</author>
<pubDate>Sun, 22 Nov 2026 18:00:00 +0000</pubDate>
<enclosure url="/preview/pre/thumb.jpg" length="0" type="image/jpeg"/>
</item>
<item>
<title>A text post</title>
<link>/r/selfhosted/comments/def456/a_text_post/</link>
<description>&lt;a href='/r/selfhosted/comments/def456/a_text_post/'&gt;Comments&lt;/a&gt;</description>
<pubDate>Sun, 22 Nov 2026 17:00:00 +0000</pubDate>
</item>
<item>
<title>A gallery</title>
<link>/r/homelab/comments/ghi789/a_gallery/</link>
<description>Gallery with 3 images</description>
<pubDate>Sun, 22 Nov 2026 16:00:00 +0000</pubDate>
</item>
</channel>
</rss>`

func TestRedditRedlibFeed(t *testing.T) {
	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(redlibTestFeed))
	}))
	defer server.Close()

	widget := &redditWidget{Subreddit: "selfhosted", SortBy: "top", TopPeriod: "week", RedlibURL: server.URL + "/"}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	posts, fromFeed, err := widget.fetchRedditPosts()
	if err != nil {
		t.Fatal(err)
	}
	if !fromFeed {
		t.Error("posts from redlib should be marked as feed posts")
	}
	if gotPath != "/r/selfhosted.rss" || !strings.Contains(gotQuery, "sort=top") || !strings.Contains(gotQuery, "t=week") {
		t.Errorf("unexpected request %s?%s", gotPath, gotQuery)
	}
	if len(posts) != 3 {
		t.Fatalf("got %d posts, want 3", len(posts))
	}

	link := posts[0]
	if link.Title != "Article & link post" {
		t.Errorf("title = %q", link.Title)
	}
	if link.DiscussionUrl != server.URL+"/r/selfhosted/comments/abc123/article_link_post/" {
		t.Errorf("discussion = %q", link.DiscussionUrl)
	}
	if link.TargetUrl != "https://www.example.com/story" || link.TargetUrlDomain != "example.com" {
		t.Errorf("target = %q (%q)", link.TargetUrl, link.TargetUrlDomain)
	}
	if link.ThumbnailUrl != server.URL+"/preview/pre/thumb.jpg" {
		t.Errorf("thumbnail = %q", link.ThumbnailUrl)
	}
	if !link.StatsUnknown || link.TimePosted.IsZero() {
		t.Error("expected unknown stats and a post time")
	}

	if text := posts[1]; text.TargetUrl != "" || text.DiscussionUrl != server.URL+"/r/selfhosted/comments/def456/a_text_post/" {
		t.Errorf("text post target %q discussion %q", text.TargetUrl, text.DiscussionUrl)
	}

	if gallery := posts[2]; gallery.Subreddit != "homelab" || gallery.TargetUrl != "" {
		t.Errorf("gallery subreddit %q target %q", gallery.Subreddit, gallery.TargetUrl)
	}
}

func TestRedditRedlibCommentsTemplate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(redlibTestFeed))
	}))
	defer server.Close()

	widget := &redditWidget{Subreddit: "selfhosted", RedlibURL: server.URL, CommentsURLTemplate: "https://old.reddit.com/{POST-PATH}"}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	posts, err := widget.fetchSubredditPostsFromRedlib()
	if err != nil {
		t.Fatal(err)
	}
	if posts[0].DiscussionUrl != "https://old.reddit.com/r/selfhosted/comments/abc123/article_link_post/" {
		t.Errorf("discussion = %q", posts[0].DiscussionUrl)
	}
}

func TestRedditRedlibRSSDisabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<html>RSS is disabled on this instance.</html>"))
	}))
	defer server.Close()

	widget := &redditWidget{Subreddit: "selfhosted", RedlibURL: server.URL}
	if err := widget.initialize(); err != nil {
		t.Fatal(err)
	}

	_, err := widget.fetchSubredditPostsFromRedlib()
	if err == nil || !strings.Contains(err.Error(), "REDLIB_ENABLE_RSS") {
		t.Errorf("expected a hint about REDLIB_ENABLE_RSS, got %v", err)
	}
}

func TestRedditRedlibURLValidation(t *testing.T) {
	for _, bad := range []string{"redlib:8080", "ftp://redlib", "http://"} {
		widget := &redditWidget{Subreddit: "selfhosted", RedlibURL: bad}
		if err := widget.initialize(); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}
