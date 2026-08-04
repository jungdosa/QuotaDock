package ui

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/jungdosa/QuotaDock/internal/model"
)

func TestProviderStateLogIncludesFromAndTo(t *testing.T) {
	var output bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	defer slog.SetDefault(previousLogger)

	logProviderStateTransitions(
		ViewState{Lanes: []LaneState{{Provider: model.ProviderCodex, Status: model.StatusConnected}}},
		ViewState{Lanes: []LaneState{{Provider: model.ProviderCodex, Status: model.StatusError, Error: model.ErrInitialization}}},
	)

	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record); err != nil {
		t.Fatal(err)
	}
	if record["msg"] != "provider.state" || record["provider"] != "codex" {
		t.Fatalf("state event=%#v", record)
	}
	if record["from"] != "connected" || record["to"] != "error" {
		t.Fatalf("state transition=%#v", record)
	}
	if record["err"] != "initialization_failed" {
		t.Fatalf("state error=%#v", record)
	}
}
