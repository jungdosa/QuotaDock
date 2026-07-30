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
	Korean  Language = "ko"
	English Language = "en"
)

var koreanWeekdays = [...]string{"일", "월", "화", "수", "목", "금", "토"}

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
	local := time.Unix(timestamp, 0).UTC().In(location)
	switch language {
	case Korean:
		return formatKorean(local, format), nil
	case English:
		return formatEnglish(local, format), nil
	default:
		return "", fmt.Errorf("unsupported language %q", language)
	}
}
func formatKorean(value time.Time, format Format) string {
	date := fmt.Sprintf("%d.%d", value.Month(), value.Day())
	if format == Hour12DateDay || format == Hour24DateDay {
		date += fmt.Sprintf(" (%s)", koreanWeekdays[value.Weekday()])
	}
	if format == Hour24Date || format == Hour24DateDay {
		return fmt.Sprintf("%s %02d:%02d", date, value.Hour(), value.Minute())
	}
	period := "오전"
	hour := value.Hour()
	if hour >= 12 {
		period = "오후"
	}
	hour %= 12
	if hour == 0 {
		hour = 12
	}
	return fmt.Sprintf("%s %s %d:%02d", date, period, hour, value.Minute())
}
func formatEnglish(value time.Time, format Format) string {
	date := fmt.Sprintf("%s %d", value.Month().String()[:3], value.Day())
	if format == Hour12DateDay || format == Hour24DateDay {
		date = value.Weekday().String()[:3] + " " + date
	}
	if format == Hour24Date || format == Hour24DateDay {
		return fmt.Sprintf("%s %02d:%02d", date, value.Hour(), value.Minute())
	}
	return date + " " + strings.TrimLeft(value.Format("03:04 PM"), "0")
}
