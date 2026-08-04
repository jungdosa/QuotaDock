//go:build windows

package main

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

const maximumEventXMLBytes = 8 << 20

func querySystemEvents(ctx context.Context, started, ended time.Time) ([]systemEvent, error) {
	query := fmt.Sprintf(
		"*[System[TimeCreated[@SystemTime>='%s' and @SystemTime<='%s']]]",
		started.UTC().Format(time.RFC3339Nano),
		ended.UTC().Format(time.RFC3339Nano),
	)
	var events []systemEvent
	for _, logName := range []string{"Application", "System"} {
		queryContext, cancel := context.WithTimeout(ctx, 30*time.Second)
		output, err := exec.CommandContext(
			queryContext,
			"wevtutil.exe",
			"qe",
			logName,
			"/q:"+query,
			"/f:xml",
			"/rd:true",
		).Output()
		cancel()
		if err != nil {
			return nil, fmt.Errorf("query %s event log: %w", logName, err)
		}
		if len(output) > maximumEventXMLBytes {
			return nil, fmt.Errorf("%s event log result exceeds limit", logName)
		}
		parsed, err := parseSystemEventsXML(logName, output)
		if err != nil {
			return nil, fmt.Errorf("parse %s event log: %w", logName, err)
		}
		events = append(events, parsed...)
	}
	return events, nil
}
