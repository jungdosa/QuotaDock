package grok

import (
	"context"
	"errors"
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/jungdosa/QuotaDock/internal/model"
)

// Fixtures are synthesized from the field layout, without captured account data.
func zeroBillingPayload(billingEnd time.Time, period []byte, extra []byte) []byte {
	inner := protoBytesField(4, protoTimestamp(billingEnd.Add(-7*24*time.Hour)))
	inner = append(inner, protoBytesField(5, protoTimestamp(billingEnd))...)
	inner = append(inner, protoBytesField(8, period)...)
	return protoBytesField(1, append(inner, extra...))
}

func zeroPeriod(kind uint64, start, end time.Time) []byte {
	period := protoVarintField(1, kind)
	period = append(period, protoBytesField(2, protoTimestamp(start))...)
	return append(period, protoBytesField(3, protoTimestamp(end))...)
}

func TestImplicitZeroGuards(t *testing.T) {
	now := time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC)
	end := now.Add(6 * 24 * time.Hour)
	period := zeroPeriod(2, now.Add(-24*time.Hour), end)
	deep := protoI32Field(9, math.Float32bits(12))
	for range 6 {
		deep = protoBytesField(9, deep)
	}
	tests := []struct {
		name       string
		period     []byte
		extra      []byte
		billingEnd time.Time
		frames     int
		known      bool
		percent    float64
	}{
		{name: "all guards", known: true},
		{name: "period type one", period: zeroPeriod(1, now.Add(-time.Hour), end), known: true},
		{name: "start inclusive", period: zeroPeriod(2, now, end), known: true},
		{name: "end just future", period: zeroPeriod(2, now.Add(-time.Hour), now.Add(time.Nanosecond)), known: true},
		{name: "usage wrong wire", extra: protoVarintField(1, 0)},
		{name: "fixed32 elsewhere", extra: protoI32Field(9, 0)},
		{name: "nested fixed32", extra: protoBytesField(9, protoI32Field(1, math.Float32bits(3)))},
		{name: "depth limit", extra: deep},
		{name: "malformed nested suffix", extra: protoBytesField(9, append(protoVarintField(1, 1), 0x80))},
		{name: "opaque bytes", extra: protoBytesField(9, []byte{0xff})},
		{name: "multiple frames", frames: 2},
		{name: "no frame evidence", frames: -1},
		{name: "reset now", billingEnd: now},
		{name: "reset past", billingEnd: now.Add(-time.Second)},
		{name: "period type zero", period: zeroPeriod(0, now.Add(-time.Hour), end)},
		{name: "period type three", period: zeroPeriod(3, now.Add(-time.Hour), end)},
		{name: "period missing", period: []byte{}},
		{name: "period type missing", period: append(protoBytesField(2, protoTimestamp(now)), protoBytesField(3, protoTimestamp(end))...)},
		{name: "period type wrong wire", period: append(protoBytesField(1, nil), period...)},
		{name: "period type duplicate", period: append(protoVarintField(1, 2), period...)},
		{name: "period start future", period: zeroPeriod(2, now.Add(time.Second), end)},
		{name: "period end exclusive", period: zeroPeriod(2, now.Add(-time.Hour), now)},
		{name: "period end past", period: zeroPeriod(2, now.Add(-time.Hour), now.Add(-time.Second))},
		{name: "period reversed", period: zeroPeriod(2, end, now)},
		{name: "period empty", period: zeroPeriod(2, now, now)},
		{name: "period start missing", period: append(protoVarintField(1, 2), protoBytesField(3, protoTimestamp(end))...)},
		{name: "period end missing", period: append(protoVarintField(1, 2), protoBytesField(2, protoTimestamp(now))...)},
		{name: "explicit usage wins", extra: protoI32Field(1, math.Float32bits(27)), known: true, percent: 27},
		{name: "explicit zero wins", extra: protoI32Field(1, 0), known: true},
		{name: "explicit usage with multiple frames", frames: 2, extra: protoI32Field(1, math.Float32bits(27)), known: true, percent: 27},
		{name: "invalid usage", extra: protoI32Field(1, math.Float32bits(101))},
		{name: "nan usage", extra: protoI32Field(1, math.Float32bits(float32(math.NaN())))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := test.period
			if p == nil {
				p = period
			}
			reset := test.billingEnd
			if reset.IsZero() {
				reset = end
			}
			frames := test.frames
			if frames == 0 {
				frames = 1
			} else if frames == -1 {
				frames = 0
			}
			snapshot, err := normalizeBilling(zeroBillingPayload(reset, p, test.extra), now, frames)
			if err != nil || len(snapshot.Limits) != 1 {
				t.Fatalf("normalization: limits=%d err=%v", len(snapshot.Limits), err)
			}
			limit := snapshot.Limits[0]
			if limit.UsageUnknown == test.known || limit.UsedPercent != test.percent {
				t.Fatalf("unknown=%v percent=%v, want known=%v percent=%v", limit.UsageUnknown, limit.UsedPercent, test.known, test.percent)
			}
		})
	}
	payload := zeroBillingPayload(end, period, nil)
	snapshot, err := NormalizeBilling(payload, now)
	if err != nil || len(snapshot.Limits) != 1 || !snapshot.Limits[0].UsageUnknown {
		t.Fatal("bare payload must not infer transport completeness")
	}
}

func TestProviderImplicitZeroTransportGuards(t *testing.T) {
	now := time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC)
	end := now.Add(6 * 24 * time.Hour)
	payload := zeroBillingPayload(end, zeroPeriod(2, now.Add(-time.Hour), end), nil)
	data := grpcWebFrameBytes(0, payload)
	trailer := grpcWebFrameBytes(0x80, []byte("grpc-status: 0\r\n"))
	tests := []struct {
		name    string
		body    []byte
		unknown bool
		invalid bool
	}{
		{name: "one data frame", body: data},
		{name: "one data plus trailer", body: append(append([]byte{}, data...), trailer...)},
		{name: "two data frames", body: append(append([]byte{}, data...), data...), unknown: true},
		{name: "empty second data frame", body: append(append([]byte{}, data...), grpcWebFrameBytes(0, nil)...), unknown: true},
		{name: "truncated trailing header", body: append(append([]byte{}, data...), 0), invalid: true},
		{name: "truncated trailing payload", body: append(append([]byte{}, data...), trailer[:len(trailer)-1]...), invalid: true},
		{name: "truncated protobuf", body: grpcWebFrameBytes(0, append(append([]byte{}, payload...), 0x80)), invalid: true},
		{name: "trailer only", body: trailer, invalid: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := New(httpClientFunc(func(*http.Request) (*http.Response, error) {
				return response(http.StatusOK, test.body), nil
			}), "")
			provider.now = func() time.Time { return now }
			snapshot, err := provider.fetch(context.Background(), Credential{})
			if test.invalid {
				var safe model.SafeError
				if !errors.As(err, &safe) || safe.Code != model.ErrInvalidResponse {
					t.Fatalf("expected invalid response, got %v", err)
				}
				return
			}
			if err != nil || len(snapshot.Limits) != 1 {
				t.Fatalf("fetch: limits=%d err=%v", len(snapshot.Limits), err)
			}
			if limit := snapshot.Limits[0]; limit.UsageUnknown != test.unknown || limit.UsedPercent != 0 {
				t.Fatalf("unknown=%v percent=%v", limit.UsageUnknown, limit.UsedPercent)
			}
		})
	}
}
