package grok

import (
	"math"
	"testing"
	"time"
)

func TestScanFieldsRecursesIntoValidNestedMessages(t *testing.T) {
	payload := protoBytesField(1, protoVarintField(5, 42))
	fields, err := ScanFields(payload, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 || !samePath(fields[1].Path, []int{1, 5}) || fields[1].Value != 42 {
		t.Fatalf("fields = %+v", fields)
	}
}

func TestScanFieldsDoesNotMisreadArbitraryBytesAsNestedMessage(t *testing.T) {
	payload := protoBytesField(1, []byte{0xff})
	fields, err := ScanFields(payload, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 1 || !samePath(fields[0].Path, []int{1}) || len(fields[0].Bytes) != 1 {
		t.Fatalf("fields = %+v", fields)
	}
}

func TestScanFieldsHonorsDepthLimit(t *testing.T) {
	payload := protoVarintField(1, 7)
	for range 6 {
		payload = protoBytesField(1, payload)
	}
	fields, err := ScanFields(payload, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range fields {
		if len(field.Path) > 3 {
			t.Fatalf("depth limit exceeded by path %v", field.Path)
		}
	}
}

func TestScanFieldsReturnsErrorForTruncatedInputWithoutPanic(t *testing.T) {
	if _, err := ScanFields([]byte{0x80}, 4); err == nil {
		t.Fatal("truncated varint was accepted")
	}
	if _, err := ScanFields([]byte{0x0a, 0x05, 0x01}, 4); err == nil {
		t.Fatal("oversized length-delimited field was accepted")
	}
}

func TestNormalizeBillingExtractsWeeklyWindow(t *testing.T) {
	now := time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC)
	frames, err := decodeGRPCWebFrames(fixture(t, "grok-billing-week.bin"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := NormalizeBilling(frames[0].Payload, now)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Provider != "grok" || snapshot.Plan != "UNKNOWN" || len(snapshot.Limits) != 1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	limit := snapshot.Limits[0]
	if limit.ID != "weekly" || limit.Label != "Weekly" || limit.WindowMinutes != 10080 {
		t.Fatalf("limit = %+v", limit)
	}
	if limit.UsedPercent != 0 || limit.RemainingPercent != 0 || !limit.ResetsAt.Equal(time.Date(2030, 1, 8, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("limit values = %+v", limit)
	}
}

func TestNormalizeBillingReadsWeeklyUsagePercent(t *testing.T) {
	now := time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC)
	start, end := now.Add(-24*time.Hour), now.Add(6*24*time.Hour)
	snapshot, err := NormalizeBilling(protoBillingPayloadWithUsage(start, end, 6), now)
	if err != nil {
		t.Fatal(err)
	}
	limit := snapshot.Limits[0]
	if limit.UsageUnknown {
		t.Fatal("usage was marked unknown even though field 1.1 was present")
	}
	if limit.UsedPercent != 6 {
		t.Fatalf("used percent = %v, want 6", limit.UsedPercent)
	}
}

func TestNormalizeBillingMarksUsageUnknownWhenPercentAbsentOrOutOfRange(t *testing.T) {
	now := time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC)
	start, end := now.Add(-24*time.Hour), now.Add(6*24*time.Hour)
	// No field 1.1 at all.
	absent, err := NormalizeBilling(protoBillingPayload(start, end), now)
	if err != nil {
		t.Fatal(err)
	}
	if !absent.Limits[0].UsageUnknown || absent.Limits[0].UsedPercent != 0 {
		t.Fatalf("absent usage should read unknown, got %+v", absent.Limits[0])
	}
	// A value outside 0..100 is treated as absent, not shown as a wrong number.
	for _, bad := range []float32{-1, 250, float32(math.Inf(1))} {
		snapshot, err := NormalizeBilling(protoBillingPayloadWithUsage(start, end, bad), now)
		if err != nil {
			t.Fatal(err)
		}
		if !snapshot.Limits[0].UsageUnknown {
			t.Fatalf("out-of-range %v was not rejected", bad)
		}
	}
}

func TestNormalizeBillingRejectsInvalidWindowRangesWithoutCreatingLane(t *testing.T) {
	now := time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC)
	tests := map[string][]byte{
		"reversed": protoBillingPayload(now.Add(24*time.Hour), now),
		"too long": protoBillingPayload(now.Add(-time.Hour), now.Add(32*24*time.Hour)),
		"past":     protoBillingPayload(now.Add(-8*24*time.Hour), now.Add(-25*time.Hour)),
	}
	for name, payload := range tests {
		t.Run(name, func(t *testing.T) {
			snapshot, err := NormalizeBilling(payload, now)
			if err != nil {
				t.Fatal(err)
			}
			if len(snapshot.Limits) != 0 {
				t.Fatalf("invalid window created a lane: %+v", snapshot.Limits)
			}
		})
	}
}

func protoBillingPayload(start, end time.Time) []byte {
	inner := make([]byte, 0)
	inner = append(inner, protoBytesField(2, nil)...)
	inner = append(inner, protoBytesField(3, nil)...)
	inner = append(inner, protoBytesField(4, protoTimestamp(start))...)
	inner = append(inner, protoBytesField(5, protoTimestamp(end))...)
	duplicate := append(protoVarintField(1, 2), protoBytesField(2, protoTimestamp(start))...)
	duplicate = append(duplicate, protoBytesField(3, protoTimestamp(end))...)
	inner = append(inner, protoBytesField(8, duplicate)...)
	inner = append(inner, protoVarintField(11, 1)...)
	inner = append(inner, protoBytesField(12, protoVarintField(1, 4000))...)
	inner = append(inner, protoVarintField(13, 1)...)
	return protoBytesField(1, inner)
}

// protoBillingPayloadWithUsage mirrors the live response, which carries the
// weekly used-percent as a float32 in field 1.1.
func protoBillingPayloadWithUsage(start, end time.Time, usedPercent float32) []byte {
	inner := protoI32Field(1, math.Float32bits(usedPercent))
	inner = append(inner, protoBytesField(4, protoTimestamp(start))...)
	inner = append(inner, protoBytesField(5, protoTimestamp(end))...)
	return protoBytesField(1, inner)
}

func protoI32Field(number int, bits uint32) []byte {
	encoded := appendVarint(nil, uint64(number<<3|5))
	return append(encoded, byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24))
}

func protoTimestamp(value time.Time) []byte {
	message := protoVarintField(1, uint64(value.Unix()))
	if value.Nanosecond() != 0 {
		message = append(message, protoVarintField(2, uint64(value.Nanosecond()))...)
	}
	return message
}

func protoVarintField(number int, value uint64) []byte {
	encoded := appendVarint(nil, uint64(number<<3))
	return appendVarint(encoded, value)
}

func protoBytesField(number int, value []byte) []byte {
	encoded := appendVarint(nil, uint64(number<<3|2))
	encoded = appendVarint(encoded, uint64(len(value)))
	return append(encoded, value...)
}

func appendVarint(destination []byte, value uint64) []byte {
	for value >= 0x80 {
		destination = append(destination, byte(value)|0x80)
		value >>= 7
	}
	return append(destination, byte(value))
}
