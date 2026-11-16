package dynacat

import (
	"os"
	"sync"
	"time"
)

var (
	lastConfigErrorMu sync.Mutex
	lastConfigError   error
)

// setLastConfigError remembers why the latest config change wasn't applied,
// so the sign-in page and Settings can say so instead of failing quietly.
func setLastConfigError(err error) {
	lastConfigErrorMu.Lock()
	lastConfigError = err
	lastConfigErrorMu.Unlock()
}

func getLastConfigError() error {
	lastConfigErrorMu.Lock()
	defer lastConfigErrorMu.Unlock()
	return lastConfigError
}

// ConfigChangeFailed is true when the newest config couldn't be loaded and an
// older one is still running.
func (a *application) ConfigChangeFailed() bool {
	return getLastConfigError() != nil
}

// watchEnvFile calls onChange when config/.env is created, edited or removed.
// It polls, since the file is often replaced rather than written in place.
func watchEnvFile(configPath string, onChange func()) func() {
	path := managedSecretsPath(configPath)
	stat := func() (time.Time, int64) {
		info, err := os.Stat(path)
		if err != nil {
			return time.Time{}, -1
		}
		return info.ModTime(), info.Size()
	}

	stop := make(chan struct{})
	lastMod, lastSize := stat()
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				mod, size := stat()
				if mod.Equal(lastMod) && size == lastSize {
					continue
				}
				lastMod, lastSize = mod, size
				onChange()
			}
		}
	}()
	return func() { close(stop) }
}
