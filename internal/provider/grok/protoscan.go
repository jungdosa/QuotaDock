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
			if depth < maxDepth {
				if nested, nestedErr := scanMessage(field.Bytes, path, depth+1, maxDepth); nestedErr == nil {
					fields = append(fields, nested...)
				}
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
	snapshot := model.UsageSnapshot{
		Provider:  model.ProviderGrok,
		Plan:      model.PlanUnknown,
		FetchedAt: fetchedAt.UTC(),
	}
	fields, err := ScanFields(payload, 4)
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
	snapshot.Limits = []model.UsageLimit{{
		ID:            "weekly",
		Label:         model.UsageWindowLabel(windowMinutes),
		WindowMinutes: windowMinutes,
		ResetsAt:      end.UTC(),
	}}
	return snapshot, nil
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
