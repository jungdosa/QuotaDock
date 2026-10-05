package grok

import (
	"encoding/binary"
	"errors"
	"math"
	"time"

	"github.com/jungdosa/QuotaDock/internal/model"
)

var (
	errProtoInvalid        = errors.New("Grok protobuf payload is invalid")
	errBillingWindowAbsent = errors.New("Grok billing window is absent")
)

// Field is one decoded wire-format field, addressed by its nested path.
type Field struct {
	Path  []int
	Type  int
	Value uint64
	Bytes []byte
}

func ScanFields(payload []byte, maxDepth int) ([]Field, error) {
	if maxDepth < 0 {
		return nil, errProtoInvalid
	}
	fields, err := scanMessage(payload, nil, 0, maxDepth)
	if err != nil {
		return nil, errProtoInvalid
	}
	return fields, nil
}

func scanMessage(payload []byte, parent []int, depth, maxDepth int) ([]Field, error) {
	complete := true
	return scanMessageChecked(payload, parent, depth, maxDepth, &complete)
}

func scanMessageChecked(payload []byte, parent []int, depth, maxDepth int, complete *bool) ([]Field, error) {
	fields := make([]Field, 0)
	for offset := 0; offset < len(payload); {
		key, next, ok := readVarint(payload, offset)
		if !ok {
			return nil, errProtoInvalid
		}
		offset = next
		fieldNumberValue := key >> 3
		if fieldNumberValue == 0 || fieldNumberValue > (1<<29)-1 {
			return nil, errProtoInvalid
		}
		fieldNumber := int(fieldNumberValue)
		wireType := int(key & 7)
		path := appendPath(parent, fieldNumber)
		field := Field{Path: path, Type: wireType}
		switch wireType {
		case 0:
			value, end, ok := readVarint(payload, offset)
			if !ok {
				return nil, errProtoInvalid
			}
			field.Value = value
			offset = end
			fields = append(fields, field)
		case 1:
			if len(payload)-offset < 8 {
				return nil, errProtoInvalid
			}
			field.Value = binary.LittleEndian.Uint64(payload[offset : offset+8])
			offset += 8
			fields = append(fields, field)
		case 2:
			length, start, ok := readVarint(payload, offset)
			if !ok || length > uint64(len(payload)-start) {
				return nil, errProtoInvalid
			}
			end := start + int(length)
			field.Bytes = append([]byte(nil), payload[start:end]...)
			fields = append(fields, field)
			// Without a schema, opaque bytes and malformed nested messages cannot
			// be distinguished. Preserve tolerant scanning, but veto inferred zero
			// whenever nested contents cannot be inspected completely.
			if depth < maxDepth {
				if nested, nestedErr := scanMessageChecked(field.Bytes, path, depth+1, maxDepth, complete); nestedErr == nil {
					fields = append(fields, nested...)
				} else {
					*complete = false
				}
			} else if len(field.Bytes) != 0 {
				*complete = false
			}
			offset = end
		case 5:
			if len(payload)-offset < 4 {
				return nil, errProtoInvalid
			}
			field.Value = uint64(binary.LittleEndian.Uint32(payload[offset : offset+4]))
			offset += 4
			fields = append(fields, field)
		default:
			return nil, errProtoInvalid
		}
	}
	return fields, nil
}

func readVarint(payload []byte, offset int) (uint64, int, bool) {
	var value uint64
	for shift := uint(0); shift < 64 && offset < len(payload); shift += 7 {
		current := payload[offset]
		offset++
		if shift == 63 && current > 1 {
			return 0, 0, false
		}
		value |= uint64(current&0x7f) << shift
		if current&0x80 == 0 {
			return value, offset, true
		}
	}
	return 0, 0, false
}

func appendPath(parent []int, fieldNumber int) []int {
	path := make([]int, len(parent)+1)
	copy(path, parent)
	path[len(parent)] = fieldNumber
	return path
}

func NormalizeBilling(payload []byte, fetchedAt time.Time) (model.UsageSnapshot, error) {
	// A bare protobuf payload cannot prove how many transport frames arrived.
	return normalizeBilling(payload, fetchedAt, 0)
}

// dataFrames is supplied only after the entire gRPC-web body decodes successfully.
func normalizeBilling(payload []byte, fetchedAt time.Time, dataFrames int) (model.UsageSnapshot, error) {
	snapshot := model.UsageSnapshot{
		Provider:  model.ProviderGrok,
		Plan:      model.PlanUnknown,
		FetchedAt: fetchedAt.UTC(),
	}
	complete := true
	fields, err := scanMessageChecked(payload, nil, 0, 4, &complete)
	if err != nil {
		return model.UsageSnapshot{}, err
	}
	start, end, err := extractBillingWindow(fields)
	if err != nil {
		return model.UsageSnapshot{}, err
	}
	if !validBillingWindow(start, end, fetchedAt) {
		return snapshot, nil
	}
	windowMinutes := int(end.Sub(start) / time.Minute)
	limit := model.UsageLimit{
		ID:            "weekly",
		Label:         model.UsageWindowLabel(windowMinutes),
		WindowMinutes: windowMinutes,
		ResetsAt:      end.UTC(),
	}
	// Field 1.1 is the weekly used-percent as a float32 (confirmed live: it
	// tracks the web dashboard's "주간 한도 N%", and equals the sum of the
	// per-feature breakdown in the repeated field 1.7). Out-of-range or absent
	// leaves the row marked unknown unless all implicit-zero guards hold.
	if percent, ok := usagePercentAt(fields, []int{1, 1}); ok {
		limit.UsedPercent = percent
	} else {
		limit.UsageUnknown = !(dataFrames == 1 && complete && end.After(fetchedAt) && implicitZeroPeriod(fields, fetchedAt))
	}
	snapshot.Limits = []model.UsageLimit{limit}
	return snapshot, nil
}

func implicitZeroPeriod(fields []Field, now time.Time) bool {
	periodTypes := 0
	for _, field := range fields {
		// Any fixed32, even outside the known usage path, may be a moved metric.
		if field.Type == 5 || samePath(field.Path, []int{1, 1}) {
			return false
		}
		if samePath(field.Path, []int{1, 8, 1}) {
			if field.Type != 0 || (field.Value != 1 && field.Value != 2) {
				return false
			}
			periodTypes++
		}
	}
	if periodTypes != 1 {
		return false
	}
	start, startErr := timestampAt(fields, []int{1, 8, 2})
	end, endErr := timestampAt(fields, []int{1, 8, 3})
	return startErr == nil && endErr == nil && start.Before(end) && !now.Before(start) && now.Before(end)
}

// usagePercentAt reads a float32 (wire type i32) at the given path and accepts
// it only as a percentage in [0, 100]. Anything else is treated as absent so a
// changed response degrades to "unknown" instead of a wrong number.
func usagePercentAt(fields []Field, path []int) (float64, bool) {
	for _, field := range fields {
		if !samePath(field.Path, path) || field.Type != 5 {
			continue
		}
		value := float64(math.Float32frombits(uint32(field.Value)))
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 100 {
			return 0, false
		}
		return value, true
	}
	return 0, false
}

func extractBillingWindow(fields []Field) (time.Time, time.Time, error) {
	start, err := timestampAt(fields, []int{1, 4})
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end, err := timestampAt(fields, []int{1, 5})
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return start, end, nil
}

func timestampAt(fields []Field, parent []int) (time.Time, error) {
	parentFound := false
	var seconds uint64
	secondsFound := false
	var nanos uint64
	nanosFound := false
	for _, field := range fields {
		if samePath(field.Path, parent) && field.Type == 2 {
			parentFound = true
		}
		if samePath(field.Path, appendPath(parent, 1)) && field.Type == 0 {
			if secondsFound {
				return time.Time{}, errProtoInvalid
			}
			seconds, secondsFound = field.Value, true
		}
		if samePath(field.Path, appendPath(parent, 2)) && field.Type == 0 {
			if nanosFound {
				return time.Time{}, errProtoInvalid
			}
			nanos, nanosFound = field.Value, true
		}
	}
	if !parentFound || !secondsFound {
		return time.Time{}, errBillingWindowAbsent
	}
	if seconds > math.MaxInt64 || nanos >= uint64(time.Second) {
		return time.Time{}, errProtoInvalid
	}
	return time.Unix(int64(seconds), int64(nanos)).UTC(), nil
}

func samePath(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func validBillingWindow(start, end, now time.Time) bool {
	if !start.Before(end) {
		return false
	}
	duration := end.Sub(start)
	if duration < time.Hour || duration > 31*24*time.Hour {
		return false
	}
	end = end.UTC()
	now = now.UTC()
	return !end.Before(now.Add(-24*time.Hour)) && !end.After(now.Add(31*24*time.Hour))
}
