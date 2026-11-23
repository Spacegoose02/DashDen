# DashDen and upstream Dynacat

DashDen is a family fork of [Dynacat](https://github.com/Panonim/dynacat). This file lists what DashDen changes, where, and what to watch for when bringing in upstream changes. Keep it up to date when a change touches an upstream file.

- **Forked from:** `Panonim/dynacat` at `86eccbe` ("Fixed windows deployment").
- **Go module path:** still `github.com/Panonim/dynacat`, on purpose, so upstream patches apply without rewriting imports.
- **Keeping up:** `.github/workflows/upstream-watch.yml` runs every Monday and keeps one issue (label `upstream-watch`) listing upstream commits DashDen doesn't have yet. Nothing merges automatically. Cherry-pick what you want, and put the SHA of anything you skip in `.github/upstream-ignore.txt`.

## The rule that keeps merges easy

New behaviour goes in **new files**. An upstream file only gets a short hook that calls into DashDen code: a field, a `case` in a switch, one `if`. When a change needs more than that in an upstream file, list it below.

## What DashDen adds

| Feature | Main files (all new) | Docs |
| ------- | -------------------- | ---- |
| Sign-in through Tinyauth / any forward-auth proxy | `auth_proxy.go` | `authentication.md` |
| Household: names, avatars, roles, a home page per person | `household.go` | `household.md` |
| Theme follows the person across devices | `user-prefs.go` | `themes.md` |
| Settings screen: secrets, history and undo, People (with username changes), Sign-in, Access, Apps, Server, Themes, recovery code | `settings-*.go`, `templates/settings*.html`, `static/js/settings.js`, `static/css/settings.css` | `settings.md` |
| Den app widgets (Den Duty, Nighttime Ninja, Wavelength, LootDen, Dendrop) and Now in the Den | `widget-den-app.go`, `widget-den-app-more.go`, `widget-den-now.go`, `templates/den-*.html`, `static/js/den-app.js`, `static/css/widget-den-app.css` | `configuration.md` |
| Per-widget `allowed-users` / `allowed-groups` | `widget-access.go` | `shared-widget-options.md` |
| Den theme pack and seasonal themes | `theme-seasonal.go`, `settings-themes.go` | `themes.md` |
| News coverage: stories grouped across outlets, left/center/right coverage bars, blindspots | `widget-news-coverage.go`, `news-outlet-ratings.go`, `templates/news-coverage.html`, `static/css/widget-news-coverage.css` | `configuration.md` |
| Notice board | `widget-notice-board.go`, `notice-storage.go`, `templates/notice-board.html` | `configuration.md` |
| Per-person to-do lists | hook in `widget-todo.go`, `dynacat.go` | `configuration.md` |
| Kiosk mode for wall tablets | `static/js/kiosk.js` | `household.md` |
| Config check: ignored settings with file and line, `dashden check`, banner for admins | `config-check.go`, `templates/config-problems.html`, `static/css/config-problems.css`, `docker/dashden` | `configuration.md` |
| Include values (`with:` and `${arg:name}`) for reusable widget files | `config-include-args.go` | `configuration.md` |
| DashDen look and branding: logo, favicon, app icon | `static/css/dashden.css`, `static/logo.png`, `static/favicon.png`, `static/app-icon.png` | |
| Fast startup: icons on disk used at once, missing ones fetched in the background | `image-cache-background.go` | |
| Error page instead of exiting when the config is broken at startup | `config-error-page.go` | `configuration.md` |
| Pages answer within 3 seconds; slow widgets show a placeholder | `page-progressive.go` | |
| Short, readable widget errors | `widget-friendly-error.go` | |
| Config health and Page use cards in Settings | `settings-health.go`, `page-usage.go` | `settings.md` |
| More menu for pages that don't fit in the header | `static/js/nav-overflow.js` | |
| Installable app that opens the last-seen pages offline | `pwa.go`, `static/pwa/sw.js`, `static/js/pwa.js` | |
| Live updates: notice boards, den apps, shared to-do lists and config changes reach every open screen | `live-updates.go` | |
| Shared database waits instead of failing with "database is locked" | `sqlite-open.go` | |
| Undo in the editor right after a change | `editor-undo.go` | |
| Built-in themes and theme styles (glass, neon, kids and more) | `theme-styles.go`, `static/css/theme-styles.css` | `themes.md` |
| Bento pages, `tile` sizes (or a `bento-s`/`-w`/`-t`/`-l` css-class), `width: full` and the Wide view button | `page-bento.go`, `static/js/bento.js`, `static/css/bento.css` | `configuration.md` |
| Tidier monitor rows: icon tiles with a letter when the icon is missing or broken, a small dot when up, tinted rows when down; Show more arrow drawn in CSS | `monitor-look.go`, `static/css/dashden-monitor.css`, `static/css/dashden.css` | |
| Reddit widget reads through a self-hosted Redlib (`redlib-url`) since Reddit ended RSS | `widget-reddit-redlib.go` | `configuration.md` |
| Docker image, Unraid template and compose, upstream watch | `.github/workflows/upstream-watch.yml`, `docker-compose.dashden.yml`, `unraid/` | `README.md` |

Every Go file above has a `_test.go` next to it.

## Upstream files DashDen changes

These are where upstream merges can conflict. Paths are under `internal/dynacat/` unless noted.

| File | What DashDen changed | If upstream changes it |
| ---- | -------------------- | ---------------------- |
| `config.go` | `user.credential` (kept for username changes); `auth.proxy`, `theme.seasonal`, `theme.builtin-themes` and `household` fields; proxy auth counts as a sign-in method; calls `validateProxyAuthConfig` and `validateHousehold`; `recursiveParseYAMLIncludes` calls `parseYAMLIncludesWithArgs`, a line-by-line version that reads `with:` blocks; page `layout` and `bento-columns` fields, `width: full` and the `validatePageLayout` call | Keep the three fields and the two validate calls; port upstream include changes into `parseYAMLIncludesWithArgs` and mirror them in `recursiveConfigLineMap` |
| `config-editor.go` | `widgetSlot` gains `shared`/`sharedCount`; includes with `with:` become shared slots; edit, remove and move call `checkSharedSlot` | Re-apply the four hooks |
| `cli.go` | `check` and `config:check` run the config check | Small |
| `auth.go` | `authenticatedUser` gains `Name`, `Email`, `IsProxy`, `member` and `DisplayName()`; `getAuthenticatedUser` wraps the result in `withHousehold` and tries the proxy headers | The upstream function body becomes `getSignedInUser`; re-apply the wrapper |
| `dynacat.go` | Settings, people, apps and themes routes; `user.credential` set next to `usernameHash`; `widgetAccessByID` filled by `registerWidget`; `userPrefs`, `noticeStorage`, `todoPerPerson`; per-person keys in `authorizeTodoRequest`; seasonal and profile theme in `populateTemplateRequestData` | The biggest risk. Re-apply route blocks and fields by hand; run `go test ./...` |
| `main.go` | Shows `showConfigErrorPage` instead of exiting on a bad startup config, `waitForReadableConfig`; no v0.7 notice or glance.yml fallback; loads secrets saved from Settings before the config; `requestConfigReload` and a reload mutex; `config:validate` runs `checkConfigFile`; reloads record errors and warnings in `currentConfigHealth` | Keep the secrets load ahead of config parsing and the health calls in `onChange` |
| `theme.go` | Theme lookups go through `themeByKey` (presets, seasonal, "default"); saves the choice to the person's profile; `name`, `style` and `wallpaper` fields and `StyleCSS`; kids start on `kidsDefaultTheme` | Keep `themeByKey` as the single lookup |
| `api.go` | `apiPageWidgetViews` takes the user and drops widgets they can't see | Keep the `user` parameter |
| `sse.go` | Broadcasts carry the widget ID so limited widgets aren't sent to others; `sseHello` greets each connection with the server start time | Keep the widget ID argument and the hello |
| `config-editor.go` | `unescapeEmoji` when writing YAML so emoji stay readable | Keep the call in `marshalDocument` |
| `editor-http.go` | Dynawidgets `owner` query parameter; a save answers with `X-DashDen-Undo` | Small |
| `widget-dynawidgets.go` | `owner` option and `DYNAWIDGETS_OWNER`, repo keys as `owner~branch` | Re-apply if upstream reworks repo keys |
| `widget.go` | `case` lines for `den-app`, `den-now`, `notice-board`, `news-coverage`; `widgetAccess` inline in `widgetBase`; `Tile` field | Small |
| `widget-container.go` | `checkNestedWidgetAccess` for groups and split columns | Small |
| `widget-search.go` | Widgets limited to some people stay out of search | Small |
| `widget-todo.go` | `per-person` option | Small |
| `templates/monitor.html` | `monitor-sites` class on the list, `data-status` on each row, icon wrapped in `monitor-site-icon-box` with a `monogram` letter and an `onerror` that shows the letter | Re-apply the three hooks |
| `templates.go` | `monogram` in `globalTemplateFunctions` | One line |
| `widget-reddit.go` | `redlib-url` field, `initializeRedlib` call, `update` gets posts from `fetchRedditPosts` (which now holds the old RSS fallback), 403 hint names `redlib-url` | If upstream changes the fallback in `update`, move it into `fetchRedditPosts` |
| `widget-schema.go` | Editor hints for the new widgets (including `news-coverage`) and common `allowed-*` fields | Merge entries; watch for duplicate keys |
| `templates/page.html` | Kiosk script, den-app script, nav-overflow, pwa and bento scripts, Wide view button, settings link, avatar and display name in the nav, DashDen logo, config problems banner | Small, but spread through the header |
| `templates/footer.html`, `templates/widget-base.html`, `templates/first-run-setup.html` | DashDen name and links | Keep ours |
| `dynacat.go` (defaults) | Default app name, favicon and app icon are DashDen's | Keep ours |
| `dynacat.go` (pages) | `handlePageContentRequest` uses `updateWidgetsWithin` and unlocks the page when updates finish; `handleWidgetContentRequest` skips the update for a widget that just finished as a placeholder; `templateData.lateWidgets`; `pageUsage.setPath` and `pageUsage.record` | Re-apply if upstream changes page rendering |
| `dynacat.go` (live updates) | `handleWidgetActionRequest` records the status and calls `pushAfterWidgetAction`; `handleTodoSave` calls `pushTodoChange`; the `/sw.js` route via `registerPWARoutes`; the `POST /api/editor/undo` route | Four lines |
| `notice-storage.go`, `todo-storage.go`, `user-prefs.go` | Open the database with `openSQLite` | One call each |
| `static/js/todo.js` | Sends `X-DashDen-Tab` when saving and reloads the list on `dashden:todo-changed` | Re-apply if upstream rewrites server storage |
| `config-fields.go` | `customIconField.cacheURL` calls `cachedIconURL` instead of downloading | One call |
| `image_cache.go` | `IsBuildingCache` leaves background icon downloads out | One line |
| `templates/page-content.html` | Widgets render through `$.RenderWidget`; `page-columns-bento` class and `BentoAttrs` | Three lines |
| `templates/widget-base.html` | Friendly error block instead of the ERROR banner; `TileAttrs` | Re-apply if upstream changes the error block |
| `templates.go` | `friendlyWidgetError` template function | One line |
| `templates/page-content.html` | `$.CanSeeWidget` check around each widget | Keep the check, or limited widgets leak |
| `templates/document.html` | One line so a theme chosen on the profile wins over the browser's saved theme; `appStarted` in `pageData` | Small |
| `static/js/page.js` | Exposes `dynacatSetupDynamicRelativeTime` on `window`; the SSE handler passes to-do changes on and reloads after a config change | Small |
| `static/js/editor.js` | Live theme preview in the styling modal, dynawidgets owner, `offerUndo` after a save; bento grid canvas, tile size button, Layout and `full` width page options | Re-apply if upstream rewrites the modal |
| `static/css/site.css` | Kiosk styles at the end | Append-only |
| `static/css/main.css` | Imports `config-problems.css`, `dashden.css`, `theme-styles.css` and `bento.css` | Four lines |
| `templates/theme-style.gotmpl`, `templates/theme-preset-preview.html` | The theme's `StyleCSS`; the name on picker buttons | One line each |
| `dynacat.go` (themes) | `withBuiltinThemes` after the presets merge | Three lines |
| `static/css/widgets.css` | Imports `widget-den-app.css` and `widget-news-coverage.css` | One line |
| `.github/workflows/deploy.yml` | Builds the DashDen image | Small |
| `README.md` | DashDen branding | Keep ours |

## Upstreaming

These are general enough to offer back to Dynacat as pull requests: forward-auth proxy sign-in, per-widget `allowed-users`/`allowed-groups`, seasonal themes, per-person to-do lists, kiosk mode and the notice board. The Den app widgets and the household are DashDen-only.
