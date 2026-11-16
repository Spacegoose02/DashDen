package dynacat

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"

	_ "modernc.org/sqlite"
)

// userPrefsStorage keeps small per-person settings, such as the chosen theme,
// in the same SQLite file as the todo widget, so they follow a person from
// device to device.
type userPrefsStorage struct {
	path string
	once sync.Once
	db   *sql.DB
	err  error
}

const userPrefTheme = "theme"

func newUserPrefsStorage(path string) *userPrefsStorage {
	return &userPrefsStorage{path: path}
}

func (s *userPrefsStorage) init() error {
	s.once.Do(func() {
		db, err := openSQLite(s.path)
		if err != nil {
			s.err = fmt.Errorf("opening sqlite db: %w", err)
			return
		}

		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS user_prefs (
			username TEXT NOT NULL,
			key      TEXT NOT NULL,
			value    TEXT NOT NULL,
			PRIMARY KEY (username, key)
		)`)
		if err != nil {
			db.Close()
			s.err = fmt.Errorf("creating user_prefs table: %w", err)
			return
		}

		s.db = db
	})
	return s.err
}

func (s *userPrefsStorage) get(username, key string) (string, bool, error) {
	if err := s.init(); err != nil {
		return "", false, err
	}

	var value string
	err := s.db.QueryRow(`SELECT value FROM user_prefs WHERE username = ? AND key = ?`, username, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (s *userPrefsStorage) set(username, key, value string) error {
	if err := s.init(); err != nil {
		return err
	}

	_, err := s.db.Exec(
		`INSERT INTO user_prefs (username, key, value) VALUES (?, ?, ?)
		 ON CONFLICT (username, key) DO UPDATE SET value = excluded.value`,
		username, key, value,
	)
	return err
}
