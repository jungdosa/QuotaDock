// Package datetime formats UTC timestamps at the rendering boundary.
package datetime

import (
	"fmt"
	"strings"
	"time"

	"github.com/jungdosa/QuotaDock/internal/i18n"
)

type Format string

const (
	Hour12Date    Format = "12h-date"
	Hour12DateDay Format = "12h-date-day"
	Hour24Date    Format = "24h-date"
	Hour24DateDay Format = "24h-date-day"
)

type clockData struct {
	am          string
	pm          string
	periodFirst bool
	days        [7]string
	dateStyle   string
}

var clocks = map[i18n.Language]clockData{
	i18n.Korean: {
		am: "오전", pm: "오후", periodFirst: true,
		days:      [7]string{"일", "월", "화", "수", "목", "금", "토"},
		dateStyle: "ko",
	},
	i18n.English: {
		am: "AM", pm: "PM",
		days:      [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"},
		dateStyle: "en",
	},
	i18n.German: {
		am: "AM", pm: "PM",
		days:      [7]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"},
		dateStyle: "de",
	},
	i18n.French: {
		am: "AM", pm: "PM",
		days:      [7]string{"dim", "lun", "mar", "mer", "jeu", "ven", "sam"},
		dateStyle: "slash",
	},
	i18n.Italian: {
		am: "AM", pm: "PM",
		days:      [7]string{"dom", "lun", "mar", "mer", "gio", "ven", "sab"},
		dateStyle: "slash",
	},
	i18n.Indonesian: {
		am: "AM", pm: "PM",
		days:      [7]string{"Min", "Sen", "Sel", "Rab", "Kam", "Jum", "Sab"},
		dateStyle: "slash",
	},
	i18n.PortugueseBrazil: {
		am: "AM", pm: "PM",
		days:      [7]string{"dom", "seg", "ter", "qua", "qui", "sex", "sáb"},
		dateStyle: "slash",
	},
	i18n.SpanishSpain: {
		am: "AM", pm: "PM",
		days:      [7]string{"dom", "lun", "mar", "mié", "jue", "vie", "sáb"},
		dateStyle: "slash",
	},
	i18n.SpanishLatinAmerica: {
		am: "AM", pm: "PM",
		days:      [7]string{"dom", "lun", "mar", "mié", "jue", "vie", "sáb"},
		dateStyle: "slash",
	},
	i18n.Japanese: {
		am: "午前", pm: "午後", periodFirst: true,
		days:      [7]string{"日", "月", "火", "水", "木", "金", "土"},
		dateStyle: "ja",
	},
	i18n.ChineseSimplified: {
		am: "上午", pm: "下午", periodFirst: true,
		days:      [7]string{"日", "一", "二", "三", "四", "五", "六"},
		dateStyle: "zh",
	},
	i18n.ChineseTraditional: {
		am: "上午", pm: "下午", periodFirst: true,
		days:      [7]string{"日", "一", "二", "三", "四", "五", "六"},
		dateStyle: "zh",
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

func FormatUnix(timestamp int64, location *time.Location, language i18n.Language, format Format) (string, error) {
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
	case i18n.Korean, i18n.Japanese, i18n.ChineseSimplified, i18n.ChineseTraditional:
		return formatEastAsian(local, format, clock), nil
	case i18n.English:
		return formatEnglish(local, format, clock), nil
	default:
		return formatLatin(local, format, clock), nil
	}
}

func formatEastAsian(value time.Time, format Format, clock clockData) string {
	date := fmt.Sprintf("%d.%d", value.Month(), value.Day())
	if clock.dateStyle == "ja" || clock.dateStyle == "zh" {
		date = fmt.Sprintf("%d/%d", value.Month(), value.Day())
	}
	if format == Hour12DateDay || format == Hour24DateDay {
		if clock.dateStyle == "ja" || clock.dateStyle == "zh" {
			date += fmt.Sprintf("（%s）", clock.days[value.Weekday()])
		} else {
			date += fmt.Sprintf(" (%s)", clock.days[value.Weekday()])
		}
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
