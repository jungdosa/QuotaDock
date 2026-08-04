package main

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

type systemEvent struct {
	Log      string
	Provider string
	EventID  int
	Time     time.Time
	Kind     string
}

type eventEnvelope struct {
	Events []eventXML `xml:"Event"`
}

type eventXML struct {
	System struct {
		Provider struct {
			Name string `xml:"Name,attr"`
		} `xml:"Provider"`
		EventID string `xml:"EventID"`
		Created struct {
			SystemTime string `xml:"SystemTime,attr"`
		} `xml:"TimeCreated"`
	} `xml:"System"`
	EventData struct {
		Data []string `xml:"Data"`
	} `xml:"EventData"`
}

func parseSystemEventsXML(logName string, data []byte) ([]systemEvent, error) {
	data = normalizeEventXML(data)
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil
	}
	var envelope eventEnvelope
	if err := xml.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	events := make([]systemEvent, 0, len(envelope.Events))
	for _, raw := range envelope.Events {
		kind, relevant := classifySystemEvent(raw.System.Provider.Name, raw.EventData.Data)
		if !relevant {
			continue
		}
		eventTime, err := time.Parse(time.RFC3339Nano, raw.System.Created.SystemTime)
		if err != nil {
			continue
		}
		eventID, _ := strconv.Atoi(strings.TrimSpace(raw.System.EventID))
		events = append(events, systemEvent{
			Log:      logName,
			Provider: raw.System.Provider.Name,
			EventID:  eventID,
			Time:     eventTime,
			Kind:     kind,
		})
	}
	return events, nil
}

func classifySystemEvent(provider string, data []string) (string, bool) {
	providerLower := strings.ToLower(strings.TrimSpace(provider))
	details := strings.ToLower(strings.Join(data, " "))
	switch {
	case strings.Contains(details, "livekernelevent"):
		return "live_kernel_event", true
	case providerLower == "application error":
		return "application_error", true
	case providerLower == "windows error reporting":
		return "windows_error_reporting", true
	case providerLower == "display" || strings.Contains(providerLower, "display") || strings.Contains(providerLower, "nvlddmkm"):
		return "display_driver", true
	default:
		return "", false
	}
}

func normalizeEventXML(data []byte) []byte {
	if len(data) < 2 {
		return data
	}
	var order binary.ByteOrder
	switch {
	case data[0] == 0xff && data[1] == 0xfe:
		order = binary.LittleEndian
	case data[0] == 0xfe && data[1] == 0xff:
		order = binary.BigEndian
	default:
		return data
	}
	values := make([]uint16, 0, (len(data)-2)/2)
	for offset := 2; offset+1 < len(data); offset += 2 {
		values = append(values, order.Uint16(data[offset:offset+2]))
	}
	decoded := []byte(string(utf16.Decode(values)))
	decoded = bytes.Replace(decoded, []byte("encoding=\"utf-16\""), []byte("encoding=\"utf-8\""), 1)
	decoded = bytes.Replace(decoded, []byte("encoding=\"UTF-16\""), []byte("encoding=\"utf-8\""), 1)
	return decoded
}
