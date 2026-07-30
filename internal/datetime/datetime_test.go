package datetime

import (
	"strings"
	"testing"
	"time"
)

func TestFourDateTimeFormatsKoreanAndEnglish(t *testing.T) {
	location := time.FixedZone("KST", 9*60*60)
	local := time.Date(2026, 7, 24, 13, 0, 0, 0, location)
	cases := []struct {
		language Language
		format   Format
		want     string
	}{{Korean, Hour12Date, "7.24 오후 1:00"}, {Korean, Hour12DateDay, "7.24 (금) 오후 1:00"}, {Korean, Hour24Date, "7.24 13:00"}, {Korean, Hour24DateDay, "7.24 (금) 13:00"}, {English, Hour12Date, "Jul 24 1:00 PM"}, {English, Hour12DateDay, "Fri Jul 24 1:00 PM"}, {English, Hour24Date, "Jul 24 13:00"}, {English, Hour24DateDay, "Fri Jul 24 13:00"}}
	for _, tc := range cases {
		got, err := FormatUnix(local.Unix(), location, tc.language, tc.format)
		if err != nil || got != tc.want {
			t.Errorf("FormatUnix(%s, %s) = %q, %v; want %q", tc.language, tc.format, got, err, tc.want)
		}
	}
}
func Test24HourFormatsHaveNoMeridiem(t *testing.T) {
	for _, language := range []Language{Korean, English} {
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
	before, _ := FormatUnix(time.Date(2026, 7, 24, 14, 59, 0, 0, time.UTC).Unix(), seoul, Korean, Hour24DateDay)
	after, _ := FormatUnix(time.Date(2026, 7, 24, 15, 1, 0, 0, time.UTC).Unix(), seoul, Korean, Hour24DateDay)
	if before != "7.24 (금) 23:59" || after != "7.25 (토) 00:01" {
		t.Fatalf("midnight transition = %q -> %q", before, after)
	}
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	beforeDST, _ := FormatUnix(time.Date(2026, 3, 8, 6, 59, 0, 0, time.UTC).Unix(), newYork, English, Hour24Date)
	afterDST, _ := FormatUnix(time.Date(2026, 3, 8, 7, 1, 0, 0, time.UTC).Unix(), newYork, English, Hour24Date)
	if beforeDST != "Mar 8 01:59" || afterDST != "Mar 8 03:01" {
		t.Fatalf("DST transition = %q -> %q", beforeDST, afterDST)
	}
}
