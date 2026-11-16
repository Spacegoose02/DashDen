package dynacat

import (
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Page use, so pages nobody opens can be spotted and retired. Counted per
// page, never per person, in the same SQLite file as the to-do lists.

type pageUsageStorage struct {
	mu   sync.Mutex
	path string
	db   *sql.DB
}

var pageUsage = &pageUsageStorage{}

// setPath is called on every config load; the file only opens once.
func (s *pageUsageStorage) setPath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == path && s.db != nil {
		return
	}
	if s.db != nil {
		s.db.Close()
		s.db = nil
	}
	s.path = path

	db, err := openSQLite(path)
	if err != nil {
		slog.Warn("Page use won't be counted", "error", err)
		return
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS page_usage (
		slug        TEXT PRIMARY KEY,
		views       INTEGER NOT NULL DEFAULT 0,
		first_view  INTEGER NOT NULL,
		last_view   INTEGER NOT NULL
	)`); err != nil {
		db.Close()
		slog.Warn("Page use won't be counted", "error", err)
		return
	}
	s.db = db
}

func (s *pageUsageStorage) record(slug string) {
	s.mu.Lock()
	db := s.db
	s.mu.Unlock()
	if db == nil || slug == "" {
		return
	}
	now := time.Now().Unix()
	go func() {
		if _, err := db.Exec(`INSERT INTO page_usage (slug, views, first_view, last_view) VALUES (?, 1, ?, ?)
			ON CONFLICT (slug) DO UPDATE SET views = views + 1, last_view = excluded.last_view`, slug, now, now); err != nil {
			slog.Debug("Counting a page view failed", "error", err)
		}
	}()
}

type pageUse struct {
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Views    int64  `json:"views"`
	LastView int64  `json:"lastView"` // unix seconds, 0 for never
}

type pageUseReport struct {
	CountingSince int64     `json:"countingSince"` // unix seconds, 0 if nothing counted yet
	Pages         []pageUse `json:"pages"`
}

func (a *application) pageUseReport() (pageUseReport, error) {
	report := pageUseReport{Pages: []pageUse{}}

	pageUsage.mu.Lock()
	db := pageUsage.db
	pageUsage.mu.Unlock()
	if db == nil {
		return report, fmt.Errorf("page use isn't being counted")
	}

	counts := map[string]pageUse{}
	rows, err := db.Query(`SELECT slug, views, first_view, last_view FROM page_usage`)
	if err != nil {
		return report, err
	}
	defer rows.Close()
	for rows.Next() {
		var u pageUse
		var first int64
		if err := rows.Scan(&u.Slug, &u.Views, &first, &u.LastView); err != nil {
			return report, err
		}
		if report.CountingSince == 0 || first < report.CountingSince {
			report.CountingSince = first
		}
		counts[u.Slug] = u
	}

	for i := range a.Config.Pages {
		p := &a.Config.Pages[i]
		u := counts[p.Slug]
		u.Name, u.Slug = p.Title, p.Slug
		report.Pages = append(report.Pages, u)
	}
	return report, rows.Err()
}
