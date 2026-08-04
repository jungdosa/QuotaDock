package main

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

func TestWatcherPollIntervalIsOneSecond(t *testing.T) {
	if pollInterval != time.Second {
		t.Fatalf("poll interval=%v", pollInterval)
	}
}

func TestUptimeSecondsUsesProcessStart(t *testing.T) {
	started := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	if got := uptimeSeconds(started, started.Add(91*time.Second)); got != 91 {
		t.Fatalf("uptime=%d", got)
	}
}

func TestEventXMLKeepsOnlyCrashAndDisplayEvidence(t *testing.T) {
	xmlData := []byte(
		"<?xml version=\"1.0\" encoding=\"utf-8\"?>" +
			"<Events>" +
			"<Event><System><Provider Name=\"Application Error\"/><EventID>1000</EventID><TimeCreated SystemTime=\"2026-08-04T08:00:00.125Z\"/></System><EventData><Data>quotadock.exe</Data></EventData></Event>" +
			"<Event><System><Provider Name=\"Windows Error Reporting\"/><EventID>1001</EventID><TimeCreated SystemTime=\"2026-08-04T08:00:01.250Z\"/></System><EventData><Data>LiveKernelEvent</Data></EventData></Event>" +
			"<Event><System><Provider Name=\"nvlddmkm\"/><EventID>14</EventID><TimeCreated SystemTime=\"2026-08-04T08:00:02.375Z\"/></System><EventData><Data>reset</Data></EventData></Event>" +
			"<Event><System><Provider Name=\"Unrelated Provider\"/><EventID>7</EventID><TimeCreated SystemTime=\"2026-08-04T08:00:03.500Z\"/></System></Event>" +
			"</Events>",
	)
	events, err := parseSystemEventsXML("System", xmlData)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("events=%#v", events)
	}
	if events[0].Kind != "application_error" || events[1].Kind != "live_kernel_event" || events[2].Kind != "display_driver" {
		t.Fatalf("event kinds=%#v", events)
	}
	if events[1].EventID != 1001 || events[1].Log != "System" {
		t.Fatalf("event fields=%#v", events[1])
	}
}

func TestEventXMLAcceptsUTF16Output(t *testing.T) {
	source := "<?xml version=\"1.0\" encoding=\"utf-16\"?><Events><Event><System><Provider Name=\"Display\"/><EventID>4101</EventID><TimeCreated SystemTime=\"2026-08-04T08:00:00.000Z\"/></System></Event></Events>"
	encoded := utf16.Encode([]rune(source))
	data := make([]byte, 2+len(encoded)*2)
	data[0], data[1] = 0xff, 0xfe
	for index, value := range encoded {
		binary.LittleEndian.PutUint16(data[2+index*2:], value)
	}
	events, err := parseSystemEventsXML("System", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Kind != "display_driver" {
		t.Fatalf("UTF-16 events=%#v", events)
	}
}

func TestClassifiedSystemEventsDoNotExposeEventData(t *testing.T) {
	event, ok := classifySystemEvent("Windows Error Reporting", []string{"user@example.invalid", "C:\\private\\file"})
	if !ok || event != "windows_error_reporting" {
		t.Fatalf("classification=%q ok=%v", event, ok)
	}
	// The classifier returns only a fixed category. Raw EventData is never
	// represented on systemEvent and therefore cannot reach watch.log.
	if strings.Contains(event, "@") || strings.Contains(event, "\\") {
		t.Fatalf("classification leaked raw event data: %q", event)
	}
}
