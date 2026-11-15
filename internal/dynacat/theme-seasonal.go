package dynacat

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// seasonalTheme switches the dashboard's default theme for part of the year,
// for example a spooky preset in late October. A theme someone picked for
// themselves, or their household theme, still wins.
type seasonalTheme struct {
	Preset string `yaml:"preset"`
	From   string `yaml:"from"`
	To     string `yaml:"to"`

	from, to monthDay `yaml:"-"`
}

type monthDay struct{ month, day int }

func (m monthDay) before(o monthDay) bool {
	return m.month < o.month || (m.month == o.month && m.day < o.day)
}

func parseMonthDay(s string) (monthDay, error) {
	month, day, ok := strings.Cut(strings.TrimSpace(s), "-")
	if !ok {
		return monthDay{}, fmt.Errorf("%q should look like 12-24 (month-day)", s)
	}
	m, err1 := strconv.Atoi(month)
	d, err2 := strconv.Atoi(day)
	if err1 != nil || err2 != nil || m < 1 || m > 12 || d < 1 || d > 31 {
		return monthDay{}, fmt.Errorf("%q should look like 12-24 (month-day)", s)
	}
	// Feb 29 is allowed, so ranges ending on it still work in leap years.
	if time.Date(2024, time.Month(m), d, 0, 0, 0, 0, time.UTC).Day() != d {
		return monthDay{}, fmt.Errorf("%q isn't a date", s)
	}
	return monthDay{m, d}, nil
}

// covers is true when day falls in from..to, both included. A range can wrap
// past the new year, like 12-20 to 01-06.
func (s *seasonalTheme) covers(day monthDay) bool {
	if !s.to.before(s.from) {
		return !day.before(s.from) && !s.to.before(day)
	}
	return !day.before(s.from) || !s.to.before(day)
}

func (a *application) prepareSeasonalThemes() error {
	for i := range a.Config.Theme.Seasonal {
		s := &a.Config.Theme.Seasonal[i]
		var err error
		if s.from, err = parseMonthDay(s.From); err != nil {
			return fmt.Errorf("theme.seasonal[%d].from: %w", i, err)
		}
		if s.to, err = parseMonthDay(s.To); err != nil {
			return fmt.Errorf("theme.seasonal[%d].to: %w", i, err)
		}
		preset, ok := a.Config.Theme.Presets.Get(s.Preset)
		if !ok {
			return fmt.Errorf("theme.seasonal[%d]: %q is not a theme preset", i, s.Preset)
		}
		if preset.CSS == "" {
			// Presets aren't prepared when the picker is off.
			preset.Key = s.Preset
			if err := preset.init(); err != nil {
				return fmt.Errorf("initializing seasonal theme %s: %v", s.Preset, err)
			}
		}
	}
	return nil
}

// seasonalThemeAt is the first seasonal theme covering now, if any.
func (a *application) seasonalThemeAt(now time.Time) *themeProperties {
	day := monthDay{int(now.Month()), now.Day()}
	for i := range a.Config.Theme.Seasonal {
		s := &a.Config.Theme.Seasonal[i]
		if s.covers(day) {
			if preset, ok := a.Config.Theme.Presets.Get(s.Preset); ok {
				return preset
			}
		}
	}
	return nil
}

// timeNow is replaced in tests.
var timeNow = time.Now
