package pricing

import "time"

// Calendars holds public holidays per calendar name, as local dates ("2026-10-01").
type Calendars map[string]map[string]bool

// Holiday dates are local to the calendar's country. China has no daylight saving time.
var calendarZones = map[string]*time.Location{"CN": time.FixedZone("CST", 8*3600)}

// IsHoliday reports whether t is a holiday in the calendar's own time zone.
func (c Calendars) IsHoliday(name string, t time.Time) bool {
	zone := calendarZones[name]
	if zone == nil {
		zone = time.UTC
	}
	return c[name][t.In(zone).Format(time.DateOnly)]
}
