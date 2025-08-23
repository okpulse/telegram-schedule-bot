package timeutil

import "time"

const (
	BitMon = 1 << 0
	BitTue = 1 << 1
	BitWed = 1 << 2
	BitThu = 1 << 3
	BitFri = 1 << 4
	BitSat = 1 << 5
	BitSun = 1 << 6
)

func MaskWorkdays() int { return BitMon|BitTue|BitWed|BitThu|BitFri }
func MaskDaily() int { return BitMon|BitTue|BitWed|BitThu|BitFri|BitSat|BitSun }

func WeekdayBit(w time.Weekday) int {
	switch w {
	case time.Monday:
		return BitMon
	case time.Tuesday:
		return BitTue
	case time.Wednesday:
		return BitWed
	case time.Thursday:
		return BitThu
	case time.Friday:
		return BitFri
	case time.Saturday:
		return BitSat
	case time.Sunday:
		return BitSun
	}
	return 0
}

func ParseHHMM(s string) (int, int, bool) {
	if len(s) < 4 || len(s) > 5 { return 0,0,false }
	layout := "15:04"
	t, err := time.Parse(layout, s)
	if err != nil { return 0,0,false }
	return t.Hour(), t.Minute(), true
}

func NextLocalMidnightPlus(tz string, plusMinutes int) (time.Time, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil { return time.Time{}, err }
	now := time.Now().In(loc)
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, plusMinutes, 0, 0, loc)
	return next.UTC(), nil
}

func LocalDateTime(tz string, h, m, dayOffset int) (time.Time, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil { return time.Time{}, err }
	now := time.Now().In(loc)
	dt := time.Date(now.Year(), now.Month(), now.Day()+dayOffset, h, m, 0, 0, loc)
	return dt, nil
}

// RangeUTC returns [fromUTC, toUTC) for a given period in the user's TZ.
func RangeUTC(period, tz string) (time.Time, time.Time, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil { return time.Time{}, time.Time{}, err }
	now := time.Now().In(loc)

	var start time.Time
	switch period {
	case "day":
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	case "week":
		weekday := int(now.Weekday())
		if weekday == 0 { weekday = 7 }
		monday := now.AddDate(0,0, -(weekday-1))
		start = time.Date(monday.Year(), monday.Month(), monday.Day(), 0,0,0,0, loc)
	case "month":
		start = time.Date(now.Year(), now.Month(), 1, 0,0,0,0, loc)
	case "all":
		return time.Unix(0,0).UTC(), time.Now().UTC(), nil
	default:
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	}
	return start.UTC(), time.Now().UTC(), nil
}
