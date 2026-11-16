package dynacat

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestSeparateConnectionsWaitForEachOther(t *testing.T) {
	path := filepath.Join(t.TempDir(), "den.db")
	first, err := openSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := openSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	if _, err := first.Exec(`CREATE TABLE t (n INTEGER)`); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func(n int) { defer wg.Done(); _, err := first.Exec(`INSERT INTO t VALUES (?)`, n); errs <- err }(i)
		go func(n int) { defer wg.Done(); _, err := second.Exec(`INSERT INTO t VALUES (?)`, n); errs <- err }(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("a write failed: %v", err)
		}
	}
}
