package dynacat

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type notice struct {
	ID       int64
	Board    string
	Username string
	Author   string
	Avatar   string
	Text     string
	Created  time.Time
}

// noticeStorage keeps notice-board notes in the same SQLite file as to-dos.
type noticeStorage struct {
	path string
	once sync.Once
	db   *sql.DB
	err  error
}

func newNoticeStorage(path string) *noticeStorage {
	return &noticeStorage{path: path}
}

func (s *noticeStorage) init() error {
	s.once.Do(func() {
		db, err := sql.Open("sqlite", s.path)
		if err != nil {
			s.err = fmt.Errorf("opening sqlite db: %w", err)
			return
		}
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS notices (
			id       INTEGER PRIMARY KEY AUTOINCREMENT,
			board    TEXT    NOT NULL,
			username TEXT    NOT NULL,
			author   TEXT    NOT NULL,
			avatar   TEXT    NOT NULL DEFAULT '',
			text     TEXT    NOT NULL,
			created  INTEGER NOT NULL
		)`)
		if err == nil {
			_, err = db.Exec(`CREATE INDEX IF NOT EXISTS notices_board ON notices (board, created)`)
		}
		if err != nil {
			db.Close()
			s.err = fmt.Errorf("creating notices table: %w", err)
			return
		}
		s.db = db
	})
	return s.err
}

// list returns a board's notes, newest first, after dropping expired ones.
func (s *noticeStorage) list(board string, keepFor time.Duration, limit int) ([]notice, error) {
	if err := s.init(); err != nil {
		return nil, err
	}
	if keepFor > 0 {
		if _, err := s.db.Exec(`DELETE FROM notices WHERE board = ? AND created < ?`, board, time.Now().Add(-keepFor).Unix()); err != nil {
			return nil, err
		}
	}
	rows, err := s.db.Query(`SELECT id, username, author, avatar, text, created FROM notices WHERE board = ? ORDER BY created DESC, id DESC LIMIT ?`, board, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []notice
	for rows.Next() {
		n := notice{Board: board}
		var created int64
		if err := rows.Scan(&n.ID, &n.Username, &n.Author, &n.Avatar, &n.Text, &created); err != nil {
			return nil, err
		}
		n.Created = time.Unix(created, 0)
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *noticeStorage) add(n notice) error {
	if err := s.init(); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO notices (board, username, author, avatar, text, created) VALUES (?, ?, ?, ?, ?, ?)`,
		n.Board, n.Username, n.Author, n.Avatar, n.Text, time.Now().Unix())
	return err
}

func (s *noticeStorage) get(board string, id int64) (*notice, error) {
	if err := s.init(); err != nil {
		return nil, err
	}
	n := notice{Board: board}
	var created int64
	err := s.db.QueryRow(`SELECT id, username, author, avatar, text, created FROM notices WHERE board = ? AND id = ?`, board, id).
		Scan(&n.ID, &n.Username, &n.Author, &n.Avatar, &n.Text, &created)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	n.Created = time.Unix(created, 0)
	return &n, nil
}

func (s *noticeStorage) remove(board string, id int64) error {
	if err := s.init(); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM notices WHERE board = ? AND id = ?`, board, id)
	return err
}
