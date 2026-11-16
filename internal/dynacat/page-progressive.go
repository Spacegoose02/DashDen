package dynacat

import (
	"context"
	"fmt"
	"html/template"
	"sync"
	"time"
)

// A page used to wait for its slowest widget before showing anything, so
// one service that was down or slow left the whole page blank. Now the page
// waits a few seconds at most. Widgets that are still loading show a
// placeholder and fill in on their own when their data arrives.
//
// The page lock stays held until every update has finished, exactly as
// before, so nothing reads a widget while it's being updated. Only the
// response goes out early.

var pageWidgetWait = 3 * time.Second

// widgetsStillLoading holds the widgets that went out as placeholders.
var widgetsStillLoading sync.Map // uint64 -> struct{}

// updateWidgetsWithin starts the page's outdated widgets updating and
// returns once they're done or the wait is over. done is closed when all of
// them have finished; the caller must keep the page locked until then.
func (p *page) updateWidgetsWithin(wait time.Duration) (done chan struct{}, late map[uint64]bool) {
	now := time.Now()
	ctx := context.Background()

	var mu sync.Mutex
	pending := map[uint64]bool{}
	var wg sync.WaitGroup

	start := func(w widget) {
		if !w.requiresUpdate(&now) {
			return
		}
		id := w.GetID()
		mu.Lock()
		pending[id] = true
		mu.Unlock()
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.update(withSharedFetchMaxAge(ctx, w.getCacheDuration()))
			mu.Lock()
			delete(pending, id)
			mu.Unlock()
		}()
	}

	for _, w := range p.HeadWidgets {
		start(w)
	}
	for c := range p.Columns {
		for _, w := range p.Columns[c].Widgets {
			start(w)
		}
	}

	done = make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return done, nil
	case <-time.After(wait):
	}

	mu.Lock()
	defer mu.Unlock()
	late = make(map[uint64]bool, len(pending))
	for id := range pending {
		late[id] = true
		widgetsStillLoading.Store(id, struct{}{})
	}
	return done, late
}

// RenderWidget is what page-content.html calls for each widget: the widget
// itself, or a placeholder while its first update is still running.
func (t templateData) RenderWidget(w widget) template.HTML {
	if t.lateWidgets[w.GetID()] {
		return widgetLoadingPlaceholder(w)
	}
	return w.Render()
}

func widgetLoadingPlaceholder(w widget) template.HTML {
	title := template.HTMLEscapeString(w.GetTitle())
	header := ""
	if title != "" {
		header = `<div class="widget-header"><h2 class="uppercase widget-title-wrapper"><span>` + title + `</span></h2></div>`
	}
	return template.HTML(fmt.Sprintf(
		`<div class="widget widget-type-%s widget-still-loading" data-widget-id="%d" data-lazy-load="true">%s<div class="widget-content"><div class="widget-skeleton" aria-label="Loading"><span></span><span></span><span></span></div></div></div>`,
		template.HTMLEscapeString(w.GetType()), w.GetID(), header,
	))
}

// takeStillLoading reports whether the widget went out as a placeholder,
// and forgets it: its data is fresh, so serving it needs no new update.
func takeStillLoading(id uint64) bool {
	_, ok := widgetsStillLoading.LoadAndDelete(id)
	return ok
}
