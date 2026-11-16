package dynacat

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// When the config is broken at startup, DashDen used to exit, and Docker
// restarted it into the same error over and over. Now it serves a page on
// the dashboard's own address that says what's wrong, keeps watching the
// config, and switches to the dashboard as soon as the config loads.

var configErrorPageTemplate = template.Must(template.New("config-error").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="refresh" content="5">
<title>DashDen needs a config fix</title>
<style>
  :root { color-scheme: dark; }
  body { margin: 0; min-height: 100vh; display: grid; place-items: center; background: #15141a; color: #d6d4dc; font: 15px/1.6 ui-monospace, "JetBrains Mono", monospace; }
  main { max-width: 760px; padding: 32px 20px; }
  h1 { font-size: 20px; color: #fff; margin: 0 0 8px; }
  .error { margin: 20px 0; padding: 16px 18px; border-left: 4px solid #e5636b; background: #1e1c24; border-radius: 6px; white-space: pre-wrap; word-break: break-word; }
  code { color: #fff; }
  .muted { color: #8d8a96; }
</style>
</head>
<body>
<main>
  <h1>DashDen can't load its config</h1>
  <p class="muted">Fix the problem below and save. This page checks again every few seconds and turns into your dashboard once the config loads.</p>
  <div class="error">{{ .Error }}</div>
  <p class="muted">To see every problem at once, run <code>docker exec dashden dashden check</code>.</p>
</main>
</body>
</html>`))

var configErrorValuePattern = regexp.MustCompile("`[^`]*`")

var configErrorPage struct {
	mu     sync.Mutex
	server *http.Server
	text   string
}

func showConfigErrorPage(contents []byte, err error) {
	configErrorPage.mu.Lock()
	defer configErrorPage.mu.Unlock()

	// Anyone who can reach the port sees this page, so values quoted in the
	// error (which could be a secret in the wrong place) are left out.
	configErrorPage.text = configErrorValuePattern.ReplaceAllString(err.Error(), "`…`")
	if configErrorPage.server != nil {
		return
	}

	addr := configErrorPageAddr(contents)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		configErrorPage.mu.Lock()
		text := configErrorPage.text
		configErrorPage.mu.Unlock()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusServiceUnavailable)
		configErrorPageTemplate.Execute(w, struct{ Error string }{text})
	})

	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	configErrorPage.server = server
	slog.Warn("Serving a page that explains the config problem until it's fixed", "address", addr)

	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Couldn't serve the config problem page", "error", err)
		}
	}()
}

func stopConfigErrorPage() {
	configErrorPage.mu.Lock()
	server := configErrorPage.server
	configErrorPage.server = nil
	configErrorPage.mu.Unlock()

	if server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}

// configErrorPageAddr reads server.host and server.port from a config that
// may not load as a whole, so the page appears where the dashboard would.
func configErrorPageAddr(contents []byte) string {
	var partial struct {
		Server struct {
			Host string `yaml:"host"`
			Port string `yaml:"port"`
		} `yaml:"server"`
	}
	if yaml.Unmarshal(contents, &partial) != nil {
		// The YAML itself is broken, so read the server block line by line.
		inServer := false
		for _, line := range strings.Split(string(contents), "\n") {
			if line != "" && line[0] != ' ' && line[0] != '\t' && line[0] != '#' {
				inServer = strings.HasPrefix(line, "server:")
				continue
			}
			if !inServer {
				continue
			}
			key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
			value = strings.Trim(strings.TrimSpace(value), `"'`)
			if ok && key == "port" {
				partial.Server.Port = value
			} else if ok && key == "host" {
				partial.Server.Host = value
			}
		}
	}

	port := 8080
	if p, err := strconv.Atoi(partial.Server.Port); err == nil && p > 0 && p < 65536 {
		port = p
	}
	return net.JoinHostPort(partial.Server.Host, strconv.Itoa(port))
}

// waitForReadableConfig reads the config and its includes, and while that
// fails (a missing include, a file that can't be read) shows the problem
// page and tries again.
func waitForReadableConfig(configPath string) ([]byte, map[string]struct{}, error) {
	for {
		contents, includes, err := parseYAMLIncludes(configPath)
		if err == nil {
			return contents, includes, nil
		}

		err = fmt.Errorf("couldn't read the config files: %w", err)
		slog.Error("Config can't be read", "error", err)
		raw, _ := os.ReadFile(configPath)
		showConfigErrorPage(raw, err)
		time.Sleep(5 * time.Second)
	}
}
