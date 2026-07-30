// Package datetime formats UTC timestamps at the rendering boundary.
package datetime

import (
	"fmt"
	"strings"
	"time"
)

type Format string

const (
	Hour12Date    Format = "12h-date"
	Hour12DateDay Format = "12h-date-day"
	Hour24Date    Format = "24h-date"
	Hour24DateDay Format = "24h-date-day"
)

type Language string

const (
	Korean              Language = "ko"
	English             Language = "en"
	German              Language = "de"
	French              Language = "fr"
	Italian             Language = "it"
	Indonesian          Language = "id"
	PortugueseBrazil    Language = "pt-BR"
	SpanishSpain        Language = "es-ES"
	SpanishLatinAmerica Language = "es-419"
)

type clockData struct {
	am          string
	pm          string
	periodFirst bool
	days        [7]string
	dateStyle   string
}

var clocks = map[Language]clockData{
	Korean: {
		am: "오전", pm: "오후", periodFirst: true,
		days:      [7]string{"일", "월", "화", "수", "목", "금", "토"},
		dateStyle: "ko",
	},
	English: {
		am: "AM", pm: "PM",
		days:      [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"},
		dateStyle: "en",
	},
	German: {
		am: "AM", pm: "PM",
		days:      [7]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"},
		dateStyle: "de",
	},
	French: {
		am: "AM", pm: "PM",
		days:      [7]string{"dim", "lun", "mar", "mer", "jeu", "ven", "sam"},
		dateStyle: "slash",
	},
	Italian: {
		am: "AM", pm: "PM",
		days:      [7]string{"dom", "lun", "mar", "mer", "gio", "ven", "sab"},
		dateStyle: "slash",
	},
	Indonesian: {
		am: "AM", pm: "PM",
		days:      [7]string{"Min", "Sen", "Sel", "Rab", "Kam", "Jum", "Sab"},
		dateStyle: "slash",
	},
	PortugueseBrazil: {
		am: "AM", pm: "PM",
		days:      [7]string{"dom", "seg", "ter", "qua", "qui", "sex", "sáb"},
		dateStyle: "slash",
	},
	SpanishSpain: {
		am: "AM", pm: "PM",
		days:      [7]string{"dom", "lun", "mar", "mié", "jue", "vie", "sáb"},
		dateStyle: "slash",
	},
	SpanishLatinAmerica: {
		am: "AM", pm: "PM",
		days:      [7]string{"dom", "lun", "mar", "mié", "jue", "vie", "sáb"},
		dateStyle: "slash",
	},
}

func IsValid(format Format) bool {
	switch format {
	case Hour12Date, Hour12DateDay, Hour24Date, Hour24DateDay:
		return true
	default:
		return false
	}
}

func FormatUnix(timestamp int64, location *time.Location, language Language, format Format) (string, error) {
	if location == nil {
		return "", fmt.Errorf("location is required")
	}
	if !IsValid(format) {
		return "", fmt.Errorf("unknown date/time format %q", format)
	}
	clock, supported := clocks[language]
	if !supported {
		return "", fmt.Errorf("unsupported language %q", language)
	}
	local := time.Unix(timestamp, 0).UTC().In(location)
	switch language {
	case Korean:
		return formatKorean(local, format, clock), nil
	case English:
		return formatEnglish(local, format, clock), nil
	default:
		return formatLatin(local, format, clock), nil
	}
}

func formatKorean(value time.Time, format Format, clock clockData) string {
	date := fmt.Sprintf("%d.%d", value.Month(), value.Day())
	if format == Hour12DateDay || format == Hour24DateDay {
		date += fmt.Sprintf(" (%s)", clock.days[value.Weekday()])
	}
	if format == Hour24Date || format == Hour24DateDay {
		return fmt.Sprintf("%s %02d:%02d", date, value.Hour(), value.Minute())
	}
	period := clock.am
	hour := value.Hour()
	if hour >= 12 {
		period = clock.pm
	}
	hour %= 12
	if hour == 0 {
		hour = 12
	}
	return fmt.Sprintf("%s %s %d:%02d", date, period, hour, value.Minute())
}

func formatEnglish(value time.Time, format Format, clock clockData) string {
	date := fmt.Sprintf("%s %d", value.Month().String()[:3], value.Day())
	if format == Hour12DateDay || format == Hour24DateDay {
		date = clock.days[value.Weekday()] + " " + date
	}
	if format == Hour24Date || format == Hour24DateDay {
		return fmt.Sprintf("%s %02d:%02d", date, value.Hour(), value.Minute())
	}
	return date + " " + strings.TrimLeft(value.Format("03:04 PM"), "0")
}

func formatLatin(value time.Time, format Format, clock clockData) string {
	date := fmt.Sprintf("%02d/%02d", value.Day(), value.Month())
	if clock.dateStyle == "de" {
		date = fmt.Sprintf("%02d.%02d.", value.Day(), value.Month())
	}
	if format == Hour12DateDay || format == Hour24DateDay {
		date = clock.days[value.Weekday()] + " " + date
	}
	if format == Hour24Date || format == Hour24DateDay {
		return fmt.Sprintf("%s %02d:%02d", date, value.Hour(), value.Minute())
	}
	period := clock.am
	hour := value.Hour()
	if hour >= 12 {
		period = clock.pm
	}
	hour %= 12
	if hour == 0 {
		hour = 12
	}
	timeText := fmt.Sprintf("%d:%02d", hour, value.Minute())
	if clock.periodFirst {
		return fmt.Sprintf("%s %s %s", date, period, timeText)
	}
	return fmt.Sprintf("%s %s %s", date, timeText, period)
}
