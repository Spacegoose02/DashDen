package dynacat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
)

var (
	pageTemplate        = mustParseTemplate("page.html", "document.html", "footer.html", "config-problems.html")
	pageContentTemplate = mustParseTemplate("page-content.html")
	manifestTemplate    = mustParseTemplate("manifest.json")
)

const STATIC_ASSETS_CACHE_DURATION = 24 * time.Hour
const REMOTE_IMAGE_CACHE_DURATION = 7 * 24 * time.Hour

var reservedPageSlugs = []string{"login", "logout", "callback"}

type imageProxyInfo struct {
	URL           string
	AllowInsecure bool
}

type application struct {
	Version    string
	CreatedAt  time.Time
	Config     config
	configPath string

	parsedManifest []byte

	slugToPage    map[string]*page
	widgetByID    map[uint64]widget
	widgetByAPIID map[string]widget
	// rules from allowed-users/allowed-groups, inherited by a container's widgets
	widgetAccessByID map[uint64]*widgetAccess
	widgetToPage     map[uint64]*page
	searchTargets    searchTargetRegistry

	globalBackground string

	RequiresAuth           bool
	OIDCEnabled            bool
	PasswordEnabled        bool
	ProxyAuthEnabled       bool
	authSecretKey          []byte
	usernameHashToUsername map[string]string
	authAttemptsMu         sync.Mutex
	failedAuthAttempts     map[string]*failedAuthAttempt

	oidcProvider *gooidc.Provider
	oidcVerifier *gooidc.IDTokenVerifier
	oauth2Config *oauth2.Config
	oidcMu       sync.Mutex
	oidcTriedAt  time.Time
	oidcSessions *sessionStore

	todoStorage      *todoStorage
	userPrefs        *userPrefsStorage
	todoListIDToPage map[string]*page
	todoPerPerson    map[string]bool
	noticeStorage    *noticeStorage

	sseMu                sync.RWMutex
	sseClients           map[*sseClient]struct{}
	DynamicUpdateEnabled bool
	EditorEnabled        bool
	EditorRequiresLogin  bool
	editorMu             sync.Mutex

	apiRateMu       sync.Mutex
	apiRateRequests map[string]*apiRateWindow

	imageProxyMu   sync.RWMutex
	imageProxyURLs map[string]imageProxyInfo

	searchAutocompleteURLs map[uint64]searchAutocompleteSource

	imageCache *imageCache
}

func newApplication(c *config) (*application, error) {
	app := &application{
		Version:                buildVersion,
		CreatedAt:              time.Now(),
		Config:                 *c,
		slugToPage:             make(map[string]*page),
		widgetByID:             make(map[uint64]widget),
		widgetAccessByID:       make(map[uint64]*widgetAccess),
		widgetByAPIID:          make(map[string]widget),
		apiRateRequests:        make(map[string]*apiRateWindow),
		widgetToPage:           make(map[uint64]*page),
		sseClients:             make(map[*sseClient]struct{}),
		imageProxyURLs:         make(map[string]imageProxyInfo),
		todoListIDToPage:       make(map[string]*page),
		todoPerPerson:          make(map[string]bool),
		searchAutocompleteURLs: make(map[uint64]searchAutocompleteSource),
	}
	config := &app.Config

	hasAnyAuth := len(config.Auth.Users) > 0 || config.Auth.OIDC != nil
	if hasAnyAuth {
		secretBytes, err := resolveAuthSecretKey(config.Auth.SecretKey)
		if err != nil {
			return nil, err
		}

		app.authSecretKey = secretBytes
		app.failedAuthAttempts = make(map[string]*failedAuthAttempt)

		requireAuth := true
		if config.Auth.RequireAuth != nil {
			requireAuth = *config.Auth.RequireAuth
		}
		app.RequiresAuth = requireAuth
	}

	if len(config.Auth.Users) > 0 && !config.Auth.DisablePassword {
		app.PasswordEnabled = true
		app.usernameHashToUsername = make(map[string]string)

		for username := range config.Auth.Users {
			user := config.Auth.Users[username]

			credential := user.PasswordHashString
			if credential == "" {
				credential = user.Password
			}

			usernameHash, err := computeUsernameHash(username, credential, app.authSecretKey)
			if err != nil {
				return nil, fmt.Errorf("computing username hash for user %s: %v", username, err)
			}
			user.usernameHash = usernameHash
			user.credential = credential // DashDen: settings-rename.go
			app.usernameHashToUsername[string(usernameHash)] = username

			if user.PasswordHashString != "" {
				user.PasswordHash = []byte(user.PasswordHashString)
				user.PasswordHashString = ""
			} else {
				hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
				if err != nil {
					return nil, fmt.Errorf("hashing password for user %s: %v", username, err)
				}

				user.Password = ""
				user.PasswordHash = hashedPassword
			}
		}
	}

	if config.Auth.OIDC != nil {
		// The provider is looked up now, but a provider that's down (or a typo
		// made in Settings) doesn't stop DashDen from starting; the sign-in
		// button tries again and explains what went wrong.
		if err := app.ensureOIDC(); err != nil {
			slog.Warn("Sign-in provider isn't reachable yet, will retry when someone signs in", "issuer", config.Auth.OIDC.IssuerURL, "error", err)
		}
		app.oidcSessions = sharedOIDCSessions
		app.OIDCEnabled = true
	}

	if config.Auth.Proxy != nil {
		config.Auth.Proxy.applyDefaults()
		app.ProxyAuthEnabled = true
		app.RequiresAuth = config.Auth.RequireAuth == nil || *config.Auth.RequireAuth
	}

	if !config.Theme.DisablePicker {
		themeKeys := make([]string, 0, 2)
		themeProps := make([]*themeProperties, 0, 2)

		defaultDarkTheme, ok := config.Theme.Presets.Get("default-dark")
		if ok && !config.Theme.SameAs(defaultDarkTheme) || !config.Theme.SameAs(&themeProperties{}) {
			themeKeys = append(themeKeys, "default-dark")
			themeProps = append(themeProps, &themeProperties{})
		}

		themeKeys = append(themeKeys, "default-light")
		themeProps = append(themeProps, &themeProperties{
			Light:                    true,
			BackgroundColor:          &hslColorField{240, 13, 95},
			PrimaryColor:             &hslColorField{230, 100, 30},
			NegativeColor:            &hslColorField{0, 70, 50},
			ContrastMultiplier:       1.3,
			TextSaturationMultiplier: 0.5,
		})

		themePresets, err := newOrderedYAMLMap(themeKeys, themeProps)
		if err != nil {
			return nil, fmt.Errorf("creating theme presets: %v", err)
		}
		config.Theme.Presets = *themePresets.Merge(&config.Theme.Presets)
		if config.Theme.BuiltinThemes == nil || *config.Theme.BuiltinThemes {
			config.Theme.Presets = *withBuiltinThemes(&config.Theme.Presets) // DashDen
		}

		for key, properties := range config.Theme.Presets.Items() {
			properties.Key = key
			if err := properties.init(); err != nil {
				return nil, fmt.Errorf("initializing preset theme %s: %v", key, err)
			}
		}
	}

	config.Theme.Key = "default"
	if err := config.Theme.init(); err != nil {
		return nil, fmt.Errorf("initializing default theme: %v", err)
	}

	if err := app.prepareSeasonalThemes(); err != nil {
		return nil, err
	}

	config.Server.BaseURL = strings.TrimRight(config.Server.BaseURL, "/")
	if config.Server.CacheDir == "" {
		config.Server.CacheDir = ".cache"
	}
	cacheDir := config.Server.CacheDir
	if !filepath.IsAbs(cacheDir) {
		absCacheDir, err := filepath.Abs(cacheDir)
		if err != nil {
			return nil, fmt.Errorf("resolving cache-dir: %v", err)
		}
		cacheDir = absCacheDir
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		if hint := ownershipHint(err); hint != "" {
			return nil, fmt.Errorf("creating cache-dir: %v, %s", err, hint)
		}
		return nil, fmt.Errorf("creating cache-dir: %v", err)
	}
	config.Server.CacheDir = cacheDir

	for _, cidr := range config.Server.TrustedProxies {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		if !strings.Contains(cidr, "/") {
			if ip := net.ParseIP(cidr); ip != nil {
				if ip.To4() != nil {
					cidr = cidr + "/32"
				} else {
					cidr = cidr + "/128"
				}
			}
		}
		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted-proxies entry %q: %v", cidr, err)
		}
		config.Server.trustedProxyNets = append(config.Server.trustedProxyNets, ipnet)
	}

	app.slugToPage[""] = &config.Pages[0]

	dynamicUpdateEnabled := true
	if v := os.Getenv("ENABLE_DYNAMIC_UPDATE"); v == "false" || v == "0" || v == "f" {
		dynamicUpdateEnabled = false
	}

	app.DynamicUpdateEnabled = dynamicUpdateEnabled

	app.EditorEnabled = editorEnabledFromEnv()
	if !app.EditorEnabled {
		warnAboutIgnoredEditorConfig(config)
	}

	app.imageCache = newImageCache(config.Server.BaseURL, config.Server.CacheDir)

	providers := &widgetProviders{
		assetResolver:        app.StaticAssetPath,
		imageCache:           app.imageCache,
		baseURL:              config.Server.BaseURL,
		DynamicUpdateEnabled: dynamicUpdateEnabled,
		app:                  app,
	}

	usedSlugs := make(map[string]struct{}, len(config.Pages))

	for p := range config.Pages {
		page := &config.Pages[p]
		page.PrimaryColumnIndex = -1

		if page.Slug == "" {
			page.Slug = titleToSlug(page.Title)
		}

		page.NameIcon.prepare(providers)

		if slices.Contains(reservedPageSlugs, page.Slug) {
			return nil, fmt.Errorf("page slug \"%s\" is reserved", page.Slug)
		}

		baseSlug := page.Slug
		for i := 2; ; i++ {
			if _, taken := usedSlugs[page.Slug]; !taken {
				break
			}
			page.Slug = fmt.Sprintf("%s-%d", baseSlug, i)
		}
		usedSlugs[page.Slug] = struct{}{}

		app.slugToPage[page.Slug] = page

		page.PageBackground = app.resolveUserDefinedAssetPath(page.PageBackground)
		if app.globalBackground == "" && page.PageBackground != "" && page.GlobalBackgroundEnabled() {
			app.globalBackground = page.PageBackground
		}

		if page.Width == "default" {
			page.Width = ""
		}

		if page.DesktopNavigationWidth == "" && page.DesktopNavigationWidth != "default" {
			page.DesktopNavigationWidth = page.Width
		}

		var collectTodoListIDs func(ws widgets)
		collectTodoListIDs = func(ws widgets) {
			for _, w := range ws {
				switch v := w.(type) {
				case *todoWidget:
					if v.Storage == "server" && v.TodoID != "" {
						app.todoListIDToPage[v.TodoID] = page
						if v.PerPerson {
							app.todoPerPerson[v.TodoID] = true
						}
					}
				case *groupWidget:
					collectTodoListIDs(v.Widgets)
				case *splitColumnWidget:
					collectTodoListIDs(v.Widgets)
				}
			}
		}

		var registerWidget func(widget widget, rules *widgetAccess)
		registerWidget = func(widget widget, rules *widgetAccess) {
			if own := widget.access(); own != nil {
				rules = own
			}
			if rules != nil {
				app.widgetAccessByID[widget.GetID()] = rules
			}
			app.widgetByID[widget.GetID()] = widget
			app.widgetToPage[widget.GetID()] = page
			if apiID := widget.GetAPIID(); apiID != "" {
				app.widgetByAPIID[apiID] = widget
			}
			widget.setProviders(providers)

			switch v := widget.(type) {
			case *groupWidget:
				for i := range v.Widgets {
					registerWidget(v.Widgets[i], rules)
				}
			case *splitColumnWidget:
				for i := range v.Widgets {
					registerWidget(v.Widgets[i], rules)
				}
			}
		}

		for i := range page.HeadWidgets {
			registerWidget(page.HeadWidgets[i], nil)
		}
		collectTodoListIDs(page.HeadWidgets)

		for c := range page.Columns {
			column := &page.Columns[c]

			if page.PrimaryColumnIndex == -1 && column.Size == "full" {
				page.PrimaryColumnIndex = int8(c)
			}

			for w := range column.Widgets {
				registerWidget(column.Widgets[w], nil)
			}
			collectTodoListIDs(column.Widgets)
		}
	}

	for id, w := range app.widgetByID {
		sw, ok := w.(*searchWidget)
		if !ok || !sw.IncludeBookmarks {
			continue
		}

		var pageFilter *page
		if !sw.CrossPageBookmarks {
			pageFilter = app.widgetToPage[id]
		}

		sw.collectBookmarks(app, pageFilter)
	}

	if config.API.Enabled {
		// Slugs are only final at this point, so allowed-pages cannot be validated during config parsing.
		for _, slug := range config.API.AllowedPages {
			if _, exists := app.slugToPage[slug]; !exists {
				return nil, fmt.Errorf("api: allowed-pages references unknown page slug %q", slug)
			}
		}
	}

	if err := app.checkHouseholdHomePages(); err != nil {
		return nil, err
	}
	app.prepareHousehold()

	app.warnAboutAPIExposure()
	app.setupEditorLoginRequirement()
	app.warnAboutEditorExposure()
	warnAboutRecoveryCode()

	config.Theme.CustomCSSFile = app.resolveUserDefinedAssetPath(config.Theme.CustomCSSFile)
	config.Branding.LogoURL = app.resolveUserDefinedAssetPath(config.Branding.LogoURL)

	config.Branding.FaviconURL = ternary(
		config.Branding.FaviconURL == "",
		app.StaticAssetPath("favicon.png"),
		app.resolveUserDefinedAssetPath(config.Branding.FaviconURL),
	)

	config.Branding.FaviconType = ternary(
		strings.HasSuffix(config.Branding.FaviconURL, ".svg"),
		"image/svg+xml",
		"image/png",
	)

	if config.Branding.AppName == "" {
		config.Branding.AppName = "DashDen"
	}

	if config.Branding.AppIconURL == "" {
		config.Branding.AppIconURL = app.StaticAssetPath("app-icon.png")
	}

	if config.Branding.AppBackgroundColor == "" {
		config.Branding.AppBackgroundColor = config.Theme.BackgroundColorAsHex
	}

	manifest, err := executeTemplateToString(manifestTemplate, templateData{App: app})
	if err != nil {
		return nil, fmt.Errorf("parsing manifest.json: %v", err)
	}
	app.parsedManifest = []byte(manifest)

	dbPath := config.Server.DBPath
	if dbPath == "" {
		dbPath = filepath.Join(config.assetsPath(), "dynacat.db")
	}

	pageUsage.setPath(dbPath) // DashDen: page-usage.go

	if app.AnyAuthEnabled() && !config.Theme.DisablePicker {
		app.userPrefs = newUserPrefsStorage(dbPath)
		if err := app.userPrefs.init(); err != nil {
			slog.Error("Could not initialize user preferences, theme choices won't follow people between devices", "path", dbPath, "error", err)
			app.userPrefs = nil
		}
	}

	for _, w := range app.widgetByID {
		if _, ok := w.(*noticeBoardWidget); ok {
			app.noticeStorage = newNoticeStorage(dbPath)
			if err := app.noticeStorage.init(); err != nil {
				slog.Error("Could not initialize the notice board database", "path", dbPath, "error", err)
			}
			break
		}
	}

	if len(app.todoPerPerson) > 0 && !app.AnyAuthEnabled() {
		return nil, fmt.Errorf("a to-do list with per-person needs sign-in to tell people apart")
	}

	if len(app.todoListIDToPage) > 0 {
		app.todoStorage = newTodoStorage(dbPath)

		if err := app.todoStorage.init(); err != nil {
			slog.Error("Could not initialize todo database", "path", dbPath, "error", err)
		}
	}

	return app, nil
}

func (a *application) sseRegisterClient(c *sseClient) {
	a.sseMu.Lock()
	a.sseClients[c] = struct{}{}
	a.sseMu.Unlock()
}

func (a *application) sseUnregisterClient(c *sseClient) {
	a.sseMu.Lock()
	delete(a.sseClients, c)
	a.sseMu.Unlock()
}

func (p *page) updateOutdatedWidgets() {
	now := time.Now()

	var wg sync.WaitGroup
	ctx := context.Background()

	for w := range p.HeadWidgets {
		widget := p.HeadWidgets[w]

		if !widget.requiresUpdate(&now) {
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			widget.update(withSharedFetchMaxAge(ctx, widget.getCacheDuration()))
		}()
	}

	for c := range p.Columns {
		for w := range p.Columns[c].Widgets {
			widget := p.Columns[c].Widgets[w]

			if !widget.requiresUpdate(&now) {
				continue
			}

			wg.Add(1)
			go func() {
				defer wg.Done()
				widget.update(withSharedFetchMaxAge(ctx, widget.getCacheDuration()))
			}()
		}
	}

	wg.Wait()
}

func (p *page) GetMinUpdateInterval() int64 {
	if !p.DynamicUpdatesEnabled() {
		return 0
	}

	min, found := getMinUpdateIntervalForWidgets(p.HeadWidgets)

	for c := range p.Columns {
		m, f := getMinUpdateIntervalForWidgets(p.Columns[c].Widgets)
		if f {
			if !found || m < min {
				min = m
				found = true
			}
		}
	}

	if !found {
		return 0
	}

	return min.Milliseconds()
}

func getMinUpdateIntervalForWidgets(ws widgets) (time.Duration, bool) {
	min := 1 * time.Second
	found := false

	for _, w := range ws {
		var interval time.Duration
		widgetFound := false

		if cw, ok := w.(*customAPIWidget); ok {
			if cw.UpdateInterval == nil {
				widgetFound = true
				interval = 1 * time.Second
			}
		} else if group, ok := w.(*groupWidget); ok {
			interval, widgetFound = getMinUpdateIntervalForWidgets(group.Widgets)
		} else if sc, ok := w.(*splitColumnWidget); ok {
			interval, widgetFound = getMinUpdateIntervalForWidgets(sc.Widgets)
		}

		if widgetFound {
			if !found || interval < min {
				min = interval
			}
			found = true
		}
	}

	return min, found
}

func (a *application) resolveUserDefinedAssetPath(path string) string {
	if strings.HasPrefix(path, "/assets/") {
		return a.Config.Server.BaseURL + path
	}

	return path
}

func (a *application) BackgroundFor(p *page) string {
	if p != nil && p.PageBackground != "" {
		return p.PageBackground
	}

	return a.globalBackground
}

type templateRequestData struct {
	Theme *themeProperties
	// ThemeFromProfile is set when the theme came from the signed-in person's
	// saved choice or household default, so the browser must not override it.
	ThemeFromProfile bool
}

type templateData struct {
	App             *application
	Page            *page
	Request         templateRequestData
	AuthUser        *authenticatedUser
	AccessiblePages []*page
	OIDCError       string
	lateWidgets     map[uint64]bool
}

func (a *application) populateTemplateRequestData(data *templateRequestData, r *http.Request, user *authenticatedUser) {
	data.Theme = &a.Config.Theme.themeProperties
	if seasonal := a.seasonalThemeAt(timeNow()); seasonal != nil {
		data.Theme = seasonal
	}

	if a.Config.Theme.DisablePicker {
		return
	}

	if theme, ok := a.profileTheme(user); ok {
		data.Theme = theme
		data.ThemeFromProfile = true
		return
	}

	// Picking "default" in the picker is a choice too, and overrides the season.
	if selectedTheme, err := r.Cookie("theme"); err == nil {
		if preset, exists := a.themeByKey(selectedTheme.Value); exists {
			data.Theme = preset
		}
	}
}

func (a *application) getAccessiblePages(user *authenticatedUser) []*page {
	pages := make([]*page, 0, len(a.Config.Pages))
	for i := range a.Config.Pages {
		p := &a.Config.Pages[i]
		if user == nil {
			if len(p.AllowedUsers) == 0 && len(p.AllowedGroups) == 0 {
				pages = append(pages, p)
			}
		} else {
			if a.isUserAllowedOnPage(user, p) {
				pages = append(pages, p)
			}
		}
	}
	return pages
}

func (a *application) handlePageRequest(w http.ResponseWriter, r *http.Request) {
	page, exists := a.slugToPage[r.PathValue("page")]
	if !exists {
		a.handleNotFound(w, r)
		return
	}

	if a.handleAccessControl(w, r, page, redirectToLogin) {
		return
	}

	user := a.getAuthenticatedUser(w, r)
	if a.redirectToHomePage(w, r, user) {
		return
	}
	pageUsage.record(page.Slug) // DashDen: page-usage.go

	data := templateData{
		Page:            page,
		App:             a,
		AuthUser:        user,
		AccessiblePages: a.getAccessiblePages(user),
	}
	a.populateTemplateRequestData(&data.Request, r, user)

	var responseBytes bytes.Buffer
	err := pageTemplate.Execute(&responseBytes, data)
	if err != nil {
		slog.Error("Rendering page template failed", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
		return
	}

	w.Write(responseBytes.Bytes())
}

func (a *application) handlePageContentRequest(w http.ResponseWriter, r *http.Request) {
	page, exists := a.slugToPage[r.PathValue("page")]
	if !exists {
		a.handleNotFound(w, r)
		return
	}

	if a.handleAccessControl(w, r, page, showUnauthorizedJSON) {
		return
	}

	pageData := templateData{
		Page:     page,
		App:      a,
		AuthUser: a.getAuthenticatedUser(w, r),
	}

	var err error
	var responseBytes bytes.Buffer
	isCacheBuilding := false

	func() {
		page.mu.Lock()

		// DashDen: answer within a few seconds, see page-progressive.go.
		done, late := page.updateWidgetsWithin(pageWidgetWait)
		defer func() {
			go func() {
				<-done
				page.mu.Unlock()
			}()
		}()

		pageData.lateWidgets = late
		if a.imageCache != nil {
			isCacheBuilding = a.imageCache.IsBuildingCache()
		}
		err = pageContentTemplate.Execute(&responseBytes, pageData)
	}()

	w.Header().Set("X-Dynacat-Cache-Building", strconv.FormatBool(isCacheBuilding))
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	if err != nil {
		slog.Error("Rendering page content template failed", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
		return
	}

	w.Write(responseBytes.Bytes())
}

func remoteAddrWithoutPort(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func (a *application) ipIsTrustedProxy(ipStr string) bool {
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return false
	}
	for _, n := range a.Config.Server.trustedProxyNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// Without trusted-proxies there is nothing to check the peer against.
func (a *application) requestCameThroughTrustedProxy(r *http.Request) bool {
	return len(a.Config.Server.trustedProxyNets) == 0 || a.ipIsTrustedProxy(remoteAddrWithoutPort(r))
}

func (a *application) addressOfRequest(r *http.Request) string {
	remote := remoteAddrWithoutPort(r)

	if !a.Config.Server.Proxied || len(a.Config.Server.trustedProxyNets) == 0 {
		return remote
	}

	if !a.ipIsTrustedProxy(remote) {
		return remote
	}

	forwardedFor := r.Header.Get("X-Forwarded-For")
	if forwardedFor == "" {
		return remote
	}

	ips := strings.Split(forwardedFor, ",")
	for i := len(ips) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(ips[i])
		if candidate == "" {
			continue
		}
		if a.ipIsTrustedProxy(candidate) {
			continue
		}
		return candidate
	}

	return remote
}

func (a *application) handleNotFound(w http.ResponseWriter, _ *http.Request) {
	// TODO: add proper not found page
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte("Page not found"))
}

func (a *application) handleWidgetContentRequest(w http.ResponseWriter, r *http.Request) {
	if a.handleUnauthorizedResponse(w, r, showUnauthorizedJSON) {
		return
	}

	widgetValue := r.PathValue("widget")
	widgetID, err := strconv.ParseUint(widgetValue, 10, 64)
	if err != nil {
		a.handleNotFound(w, r)
		return
	}

	widget, exists := a.widgetByID[widgetID]
	if !exists {
		a.handleNotFound(w, r)
		return
	}

	page, exists := a.widgetToPage[widgetID]
	if !exists {
		a.handleNotFound(w, r)
		return
	}

	if a.handleAccessControl(w, r, page, showUnauthorizedJSON) {
		return
	}

	if !a.canUserSeeWidget(a.getAuthenticatedUser(w, r), widgetID) {
		a.handleNotFound(w, r)
		return
	}

	page.mu.Lock()
	defer page.mu.Unlock()

	// DashDen: a placeholder's update finished while we waited for the lock.
	if !takeStillLoading(widgetID) {
		widget.update(context.Background())
	}

	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(widget.Render()))
}

func (a *application) handleWidgetActionRequest(w http.ResponseWriter, r *http.Request) {
	if a.handleUnauthorizedResponse(w, r, showUnauthorizedJSON) {
		return
	}

	widgetID, err := strconv.ParseUint(r.PathValue("widget"), 10, 64)
	if err != nil {
		http.Error(w, "invalid widget", http.StatusBadRequest)
		return
	}

	widget, exists := a.widgetByID[widgetID]
	if !exists {
		a.handleNotFound(w, r)
		return
	}

	page, exists := a.widgetToPage[widgetID]
	if !exists {
		a.handleNotFound(w, r)
		return
	}

	if a.handleAccessControl(w, r, page, showUnauthorizedJSON) {
		return
	}

	if !a.canUserSeeWidget(a.getAuthenticatedUser(w, r), widgetID) {
		a.handleNotFound(w, r)
		return
	}

	page.mu.Lock()
	defer page.mu.Unlock()

	// DashDen: show the change on every other open screen too.
	rec := &statusRecorder{ResponseWriter: w}
	widget.handleRequest(rec, r)
	if rec.succeeded() {
		a.pushAfterWidgetAction(widget)
	}
}

func (a *application) StaticAssetPath(asset string) string {
	return a.Config.Server.BaseURL + "/static/" + getStaticFSHash() + "/" + asset
}

func (a *application) VersionedAssetPath(asset string) string {
	return a.Config.Server.BaseURL + asset +
		"?v=" + strconv.FormatInt(a.CreatedAt.Unix(), 10)
}

const todoMaxBodyBytes = 1 << 20

func (a *application) authorizeTodoRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	listID := r.PathValue("listID")
	pg, exists := a.todoListIDToPage[listID]
	if !exists {
		a.handleNotFound(w, r)
		return "", false
	}
	if a.handleAccessControl(w, r, pg, showUnauthorizedJSON) {
		return "", false
	}
	if a.todoPerPerson[listID] {
		user := a.getAuthenticatedUser(w, r)
		if user == nil {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "Unauthorized"}`))
			return "", false
		}
		// The unit separator keeps personal lists apart from any list id a
		// config could contain.
		return listID + "\x1f" + user.Username, true
	}
	return listID, true
}

func (a *application) handleTodoLoad(w http.ResponseWriter, r *http.Request) {
	listID, ok := a.authorizeTodoRequest(w, r)
	if !ok {
		return
	}

	tasks, err := a.todoStorage.loadTasks(listID)
	if err != nil {
		slog.Error("Todo load failed", "list_id", listID, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

func (a *application) handleTodoSave(w http.ResponseWriter, r *http.Request) {
	listID, ok := a.authorizeTodoRequest(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, todoMaxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	var tasks []todoTask
	if err := json.Unmarshal(body, &tasks); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if err := a.todoStorage.saveTasks(listID, tasks); err != nil {
		slog.Error("Todo save failed", "list_id", listID, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	a.pushTodoChange(r, listID) // DashDen: live updates
	w.WriteHeader(http.StatusNoContent)
}

func (a *application) securityHeadersMiddleware(next http.Handler) http.Handler {
	frameAncestors := "'self'"
	if len(a.Config.Server.AllowedEmbedHosts) > 0 {
		frameAncestors = "'self' " + strings.Join(a.Config.Server.AllowedEmbedHosts, " ")
	}
	csp := "default-src 'self'; " +
		"img-src 'self' data: blob: https: http:; " +
		"media-src 'self' data: blob: https: http:; " +
		"style-src 'self' 'unsafe-inline'; " +
		"script-src 'self' 'unsafe-inline'; " +
		"font-src 'self' data:; " +
		"connect-src 'self' https: http:; " +
		"frame-src *; " +
		"frame-ancestors " + frameAncestors + "; " +
		"base-uri 'self'; " +
		"form-action 'self'"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		// Only the default case is expressible in this header, embed hosts are covered by frame-ancestors.
		if len(a.Config.Server.AllowedEmbedHosts) == 0 {
			h.Set("X-Frame-Options", "SAMEORIGIN")
		}
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", csp)
		if a.Config.Server.HTTPS || a.isRequestHTTPS(r) {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// Blocks cross-site writes such as a form POST to the editor from another website. Browsers
// always send Origin on unsafe methods; API clients that send none are left alone.
func sameOriginMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if origin == "" {
				// No Origin and no cookies means an API client, not a browser.
				if len(r.Cookies()) > 0 {
					http.Error(w, "cross-origin request rejected", http.StatusForbidden)
					return
				}
			} else if originHost(origin) != r.Host {
				http.Error(w, "cross-origin request rejected", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func originHost(origin string) string {
	parsed, err := url.Parse(origin)
	if err != nil {
		return ""
	}

	return parsed.Host
}

func (a *application) isRequestHTTPS(r *http.Request) bool {
	return a.Config.Server.HTTPS || r.TLS != nil ||
		(a.Config.Server.Proxied && a.requestCameThroughTrustedProxy(r) &&
			strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"))
}

func (a *application) server() (func() error, func() error) {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", a.handlePageRequest)
	mux.HandleFunc("GET /{page}", a.handlePageRequest)

	mux.HandleFunc("GET /api/pages/{page}/content/{$}", a.handlePageContentRequest)

	if !a.Config.Theme.DisablePicker {
		mux.HandleFunc("POST /api/set-theme/{key}", a.handleThemeChangeRequest)
	}

	mux.HandleFunc("GET /api/widgets/{widget}/content/{$}", a.handleWidgetContentRequest)
	mux.HandleFunc("POST /api/widgets/{widget}/action/{action...}", a.handleWidgetActionRequest)
	mux.HandleFunc("GET /api/sse/updates", a.handleSSEUpdates)
	mux.HandleFunc("GET /api/image-proxy/{hash}", a.handleImageProxyRequest)
	mux.HandleFunc("GET /api/search/autocomplete", a.handleSearchAutocompleteRequest)
	mux.HandleFunc("GET /api/search/{widget}/targets", a.handleSearchTargetsRequest)
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	if a.AnyAuthEnabled() {
		mux.HandleFunc("GET /login", a.handleLoginPageRequest)
		mux.HandleFunc("POST /logout", a.handleLogoutRequest)
		a.registerAccountRoutes(mux)
	}

	if a.PasswordEnabled {
		mux.HandleFunc("POST /api/authenticate", a.handleAuthenticationAttempt)
	}

	if a.OIDCEnabled {
		mux.HandleFunc("GET /api/oidc/login", a.handleOIDCLogin)
		mux.HandleFunc("GET /api/oidc/callback", a.handleOIDCCallback)
	}

	if a.todoStorage != nil {
		mux.HandleFunc("GET /api/todo/{listID}", a.handleTodoLoad)
		mux.HandleFunc("PUT /api/todo/{listID}", a.handleTodoSave)
	}

	if a.Config.API.Enabled {
		mux.HandleFunc("GET /api/v1/pages", a.handleAPIPages)
		mux.HandleFunc("GET /api/v1/pages/{page}", a.handleAPIPage)
		mux.HandleFunc("GET /api/v1/widgets/{apiID}", a.handleAPIWidget)
		mux.HandleFunc("OPTIONS /api/v1/{path...}", a.handleAPIPreflight)
	}

	if a.EditorEnabled || recoveryCode() != "" {
		a.registerSettingsRoutes(mux)
	}

	if a.EditorEnabled {
		mux.HandleFunc("GET /api/editor/schema", a.handleEditorSchema)
		mux.HandleFunc("GET /api/editor/status", a.handleEditorStatus)
		mux.HandleFunc("GET /api/editor/config", a.handleEditorConfigLoad)
		mux.HandleFunc("POST /api/editor/config", a.handleEditorConfigSave)
		mux.HandleFunc("POST /api/editor/undo", a.handleEditorUndo) // DashDen
		mux.HandleFunc("POST /api/editor/convert", a.handleEditorConvert)
		mux.HandleFunc("POST /api/editor/custom-api/preview", a.handleEditorCustomAPIPreview)
		mux.HandleFunc("GET /api/editor/dynawidgets/variables", a.handleEditorDynawidgetVariables)
	}

	mux.Handle(
		fmt.Sprintf("GET /static/%s/{path...}", getStaticFSHash()),
		gzipTextAssets(http.StripPrefix(
			"/static/"+getStaticFSHash(),
			fileServerWithCache(http.FS(staticFS), STATIC_ASSETS_CACHE_DURATION),
		)),
	)

	if a.Config.Server.CacheDir != "" {
		// Cached files are fetched from remote widget content and an SVG among them would
		// otherwise run scripts on this origin when opened directly.
		cacheHandler := sandboxedHandler(http.StripPrefix(
			"/.cache",
			fileServerWithCache(http.Dir(a.Config.Server.CacheDir), REMOTE_IMAGE_CACHE_DURATION),
		))

		if a.RequiresAuth {
			mux.Handle("GET /.cache/{path...}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if a.handleUnauthorizedResponse(w, r, showUnauthorizedJSON) {
					return
				}

				cacheHandler.ServeHTTP(w, r)
			}))
		} else {
			mux.Handle("GET /.cache/{path...}", cacheHandler)
		}
	}

	assetCacheControlValue := fmt.Sprintf(
		"public, max-age=%d",
		int(STATIC_ASSETS_CACHE_DURATION.Seconds()),
	)

	serveCSSBundle := func(contents []byte) http.Handler {
		return gzipTextAssets(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Cache-Control", assetCacheControlValue)
			w.Header().Add("Content-Type", "text/css; charset=utf-8")
			w.Write(contents)
		}))
	}

	mux.Handle(fmt.Sprintf("GET /static/%s/css/bundle.css", getStaticFSHash()), serveCSSBundle(bundledCSSContents))
	mux.Handle(fmt.Sprintf("GET /static/%s/css/editor-bundle.css", getStaticFSHash()), serveCSSBundle(bundledEditorCSSContents))

	a.registerPWARoutes(mux) // DashDen: pwa.go

	mux.HandleFunc("GET /manifest.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", assetCacheControlValue)
		w.Header().Add("Content-Type", "application/json")
		w.Write(a.parsedManifest)
	})

	assetsPath := a.Config.assetsPath()

	absAssetsPath, _ := filepath.Abs(assetsPath)
	assetsFS := fileServerWithCache(http.Dir(assetsPath), 2*time.Hour)
	assetsHandler := http.StripPrefix("/assets/", assetsFS)
	if a.RequiresAuth {
		mux.Handle("/assets/{path...}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if a.handleUnauthorizedResponse(w, r, showUnauthorizedJSON) {
				return
			}

			assetsHandler.ServeHTTP(w, r)
		}))
	} else {
		mux.Handle("/assets/{path...}", assetsHandler)
	}

	server := http.Server{
		Addr:    fmt.Sprintf("%s:%d", a.Config.Server.Host, a.Config.Server.Port),
		Handler: a.securityHeadersMiddleware(sameOriginMiddleware(mux)),
	}

	start := func() error {
		slog.Info("Starting server", "host", a.Config.Server.Host, "port", a.Config.Server.Port, "base_url", a.Config.Server.BaseURL, "assets_path", absAssetsPath)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return err
		}

		return nil
	}

	ctx, cancelCtx := context.WithCancel(context.Background())
	go a.sseUpdateLoop(ctx)
	go a.prewarmWidgets()
	if a.oidcSessions != nil {
		go a.oidcSessions.runSweeper(ctx, 15*time.Minute, OIDC_SESSION_VALID_PERIOD)
	}

	stop := func() error {
		cancelCtx()
		err := server.Close()
		if a.todoStorage != nil {
			a.todoStorage.close()
		}
		return err
	}

	return start, stop
}

func (a *application) prewarmWidgets() {
	var wg sync.WaitGroup
	for p := range a.Config.Pages {
		page := &a.Config.Pages[p]
		wg.Add(1)
		go func() {
			defer wg.Done()
			page.mu.Lock()
			defer page.mu.Unlock()
			page.updateOutdatedWidgets()
		}()
	}
	wg.Wait()
}
