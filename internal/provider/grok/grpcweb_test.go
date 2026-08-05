package grok

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEmptyRequestFrame(t *testing.T) {
	if got := emptyRequestFrame(); len(got) != 5 || got[0] != 0 || binary.BigEndian.Uint32(got[1:]) != 0 {
		t.Fatalf("request frame = %v", got)
	}
}

func TestDecodeGRPCWebFrames(t *testing.T) {
	frames, err := decodeGRPCWebFrames(fixture(t, "grok-billing-week.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 || frames[0].Trailer || !frames[1].Trailer || len(frames[0].Payload) == 0 {
		t.Fatalf("frames = %+v", frames)
	}
}

func TestDecodeGRPCWebEmptyAndTruncated(t *testing.T) {
	if _, err := decodeGRPCWebFrames(fixture(t, "grok-billing-empty.bin")); !errors.Is(err, errGRPCWebEmptyBody) {
		t.Fatalf("empty error = %v", err)
	}
	if _, err := decodeGRPCWebFrames(fixture(t, "grok-billing-truncated.bin")); !errors.Is(err, errGRPCWebInvalid) {
		t.Fatalf("truncated error = %v", err)
	}
}

func TestDecodeGRPCWebTrailerOnly(t *testing.T) {
	frames, err := decodeGRPCWebFrames(grpcWebFrameBytes(0x80, []byte("grpc-status: 0\r\n")))
	if err != nil || len(frames) != 1 || !frames[0].Trailer {
		t.Fatalf("trailer-only frames = %+v, %v", frames, err)
	}
}

func grpcWebFrameBytes(flags byte, payload []byte) []byte {
	frame := make([]byte, 5+len(payload))
	frame[0] = flags
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
	copy(frame[5:], payload)
	return frame
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
