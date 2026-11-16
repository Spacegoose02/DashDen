package dynacat

import (
	"sort"
)

// The Config health card on the Settings overview: settings DashDen
// ignores, and widgets whose last update failed.

type settingsWidgetProblem struct {
	Title string `json:"title"`
	Type  string `json:"type"`
	Error string `json:"error"`
}

type settingsHealth struct {
	Warnings []string                `json:"warnings"`
	Widgets  []settingsWidgetProblem `json:"widgets"`
}

func (a *application) settingsHealth() settingsHealth {
	h := settingsHealth{
		Warnings: currentConfigHealth.snapshot().Warnings,
		Widgets:  []settingsWidgetProblem{},
	}
	if h.Warnings == nil {
		h.Warnings = []string{}
	}

	for _, w := range a.widgetByID {
		err := w.GetError()
		if err == nil {
			continue
		}
		h.Widgets = append(h.Widgets, settingsWidgetProblem{
			Title: w.GetTitle(),
			Type:  w.GetType(),
			Error: err.Error(),
		})
	}
	sort.Slice(h.Widgets, func(i, j int) bool {
		if h.Widgets[i].Title != h.Widgets[j].Title {
			return h.Widgets[i].Title < h.Widgets[j].Title
		}
		return h.Widgets[i].Error < h.Widgets[j].Error
	})
	return h
}
