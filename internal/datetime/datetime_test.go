package datetime

import (
	"strings"
	"testing"
	"time"

	"github.com/jungdosa/QuotaDock/internal/i18n"
)

func TestFourDateTimeFormatsKoreanAndEnglish(t *testing.T) {
	location := time.FixedZone("KST", 9*60*60)
	local := time.Date(2026, 7, 24, 13, 0, 0, 0, location)
	cases := []struct {
		language i18n.Language
		format   Format
		want     string
	}{{i18n.Korean, Hour12Date, "7.24 오후 1:00"}, {i18n.Korean, Hour12DateDay, "7.24 (금) 오후 1:00"}, {i18n.Korean, Hour24Date, "7.24 13:00"}, {i18n.Korean, Hour24DateDay, "7.24 (금) 13:00"}, {i18n.English, Hour12Date, "Jul 24 1:00 PM"}, {i18n.English, Hour12DateDay, "Fri Jul 24 1:00 PM"}, {i18n.English, Hour24Date, "Jul 24 13:00"}, {i18n.English, Hour24DateDay, "Fri Jul 24 13:00"}}
	for _, tc := range cases {
		got, err := FormatUnix(local.Unix(), location, tc.language, tc.format)
		if err != nil || got != tc.want {
			t.Errorf("FormatUnix(%s, %s) = %q, %v; want %q", tc.language, tc.format, got, err, tc.want)
		}
	}
}
func Test24HourFormatsHaveNoMeridiem(t *testing.T) {
	for _, language := range []i18n.Language{i18n.Korean, i18n.English, i18n.German, i18n.French, i18n.Italian, i18n.Indonesian, i18n.PortugueseBrazil, i18n.SpanishSpain, i18n.SpanishLatinAmerica} {
		got, err := FormatUnix(time.Date(2026, 7, 24, 23, 5, 0, 0, time.UTC).Unix(), time.UTC, language, Hour24DateDay)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range []string{"오전", "오후", "AM", "PM"} {
			if strings.Contains(got, marker) {
				t.Errorf("%q contains %q", got, marker)
			}
		}
	}
}
func TestMidnightTimezoneAndDSTTransitions(t *testing.T) {
	seoul := time.FixedZone("KST", 9*60*60)
	before, _ := FormatUnix(time.Date(2026, 7, 24, 14, 59, 0, 0, time.UTC).Unix(), seoul, i18n.Korean, Hour24DateDay)
	after, _ := FormatUnix(time.Date(2026, 7, 24, 15, 1, 0, 0, time.UTC).Unix(), seoul, i18n.Korean, Hour24DateDay)
	if before != "7.24 (금) 23:59" || after != "7.25 (토) 00:01" {
		t.Fatalf("midnight transition = %q -> %q", before, after)
	}
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	beforeDST, _ := FormatUnix(time.Date(2026, 3, 8, 6, 59, 0, 0, time.UTC).Unix(), newYork, i18n.English, Hour24Date)
	afterDST, _ := FormatUnix(time.Date(2026, 3, 8, 7, 1, 0, 0, time.UTC).Unix(), newYork, i18n.English, Hour24Date)
	if beforeDST != "Mar 8 01:59" || afterDST != "Mar 8 03:01" {
		t.Fatalf("DST transition = %q -> %q", beforeDST, afterDST)
	}
}

func TestNewLocalesUseClockAssetWeekdaysAndMeridiem(t *testing.T) {
	value := time.Date(2026, 7, 26, 13, 5, 0, 0, time.UTC)
	cases := []struct {
		language i18n.Language
		want     string
	}{
		{i18n.German, "So 26.07. 1:05 PM"},
		{i18n.French, "dim 26/07 1:05 PM"},
		{i18n.Italian, "dom 26/07 1:05 PM"},
		{i18n.Indonesian, "Min 26/07 1:05 PM"},
		{i18n.PortugueseBrazil, "dom 26/07 1:05 PM"},
		{i18n.SpanishSpain, "dom 26/07 1:05 PM"},
		{i18n.SpanishLatinAmerica, "dom 26/07 1:05 PM"},
	}
	for _, tc := range cases {
		got, err := FormatUnix(value.Unix(), time.UTC, tc.language, Hour12DateDay)
		if err != nil || got != tc.want {
			t.Errorf("FormatUnix(%s) = %q, %v; want %q", tc.language, got, err, tc.want)
		}
	}
}

func TestExistingLocaleOutputRegression(t *testing.T) {
	value := time.Date(2026, 7, 26, 13, 5, 0, 0, time.UTC)
	formats := []Format{Hour12Date, Hour12DateDay, Hour24Date, Hour24DateDay}
	want := map[i18n.Language][4]string{
		i18n.Korean:              {"7.26 오후 1:05", "7.26 (일) 오후 1:05", "7.26 13:05", "7.26 (일) 13:05"},
		i18n.English:             {"Jul 26 1:05 PM", "Sun Jul 26 1:05 PM", "Jul 26 13:05", "Sun Jul 26 13:05"},
		i18n.German:              {"26.07. 1:05 PM", "So 26.07. 1:05 PM", "26.07. 13:05", "So 26.07. 13:05"},
		i18n.French:              {"26/07 1:05 PM", "dim 26/07 1:05 PM", "26/07 13:05", "dim 26/07 13:05"},
		i18n.Italian:             {"26/07 1:05 PM", "dom 26/07 1:05 PM", "26/07 13:05", "dom 26/07 13:05"},
		i18n.Indonesian:          {"26/07 1:05 PM", "Min 26/07 1:05 PM", "26/07 13:05", "Min 26/07 13:05"},
		i18n.PortugueseBrazil:    {"26/07 1:05 PM", "dom 26/07 1:05 PM", "26/07 13:05", "dom 26/07 13:05"},
		i18n.SpanishSpain:        {"26/07 1:05 PM", "dom 26/07 1:05 PM", "26/07 13:05", "dom 26/07 13:05"},
		i18n.SpanishLatinAmerica: {"26/07 1:05 PM", "dom 26/07 1:05 PM", "26/07 13:05", "dom 26/07 13:05"},
	}
	for language, outputs := range want {
		for index, format := range formats {
			got, err := FormatUnix(value.Unix(), time.UTC, language, format)
			if err != nil || got != outputs[index] {
				t.Errorf("FormatUnix(%s, %s) = %q, %v; want unchanged %q", language, format, got, err, outputs[index])
			}
		}
	}
}

func TestCJKClockAssetsPlaceMeridiemBeforeTimeAndUseExpectedWeekdays(t *testing.T) {
	languages := []struct {
		language i18n.Language
		am       string
		pm       string
		days     [7]string
	}{
		{i18n.Japanese, "午前", "午後", [7]string{"日", "月", "火", "水", "木", "金", "土"}},
		{i18n.ChineseSimplified, "上午", "下午", [7]string{"日", "一", "二", "三", "四", "五", "六"}},
		{i18n.ChineseTraditional, "上午", "下午", [7]string{"日", "一", "二", "三", "四", "五", "六"}},
	}
	for _, locale := range languages {
		morning := time.Date(2026, 7, 26, 9, 5, 0, 0, time.UTC)
		got, err := FormatUnix(morning.Unix(), time.UTC, locale.language, Hour12Date)
		if err != nil || got != "7/26 "+locale.am+" 9:05" {
			t.Errorf("%s morning = %q, %v", locale.language, got, err)
		}
		afternoon := time.Date(2026, 7, 26, 13, 5, 0, 0, time.UTC)
		got, err = FormatUnix(afternoon.Unix(), time.UTC, locale.language, Hour12Date)
		if err != nil || got != "7/26 "+locale.pm+" 1:05" {
			t.Errorf("%s afternoon = %q, %v", locale.language, got, err)
		}
		for day, weekday := range locale.days {
			value := time.Date(2026, 7, 26+day, 13, 5, 0, 0, time.UTC)
			got, err = FormatUnix(value.Unix(), time.UTC, locale.language, Hour24DateDay)
			if err != nil || !strings.Contains(got, "（"+weekday+"）") {
				t.Errorf("%s weekday %d = %q, %v; want %q", locale.language, day, got, err, weekday)
			}
		}
	}
}
