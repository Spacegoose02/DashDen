package dynacat

import (
	"database/sql"
	"strings"
)

// openSQLite opens the shared database file. To-dos, notices, people's
// preferences and page use each keep their own connection to it, so a
// connection that finds the file busy waits a moment for the others instead
// of failing with "database is locked".
func openSQLite(path string) (*sql.DB, error) {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return sql.Open("sqlite", path+sep+"_pragma=busy_timeout(5000)")
}
