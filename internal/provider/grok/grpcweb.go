package grok

import (
	"encoding/binary"
	"errors"
)

var (
	errGRPCWebEmptyBody = errors.New("Grok billing response is empty")
	errGRPCWebInvalid   = errors.New("Grok billing response framing is invalid")
)

type grpcWebFrame struct {
	Trailer bool
	Payload []byte
}

func emptyRequestFrame() []byte {
	return []byte{0, 0, 0, 0, 0}
}

func decodeGRPCWebFrames(body []byte) ([]grpcWebFrame, error) {
	if len(body) == 0 {
		return nil, errGRPCWebEmptyBody
	}
	frames := make([]grpcWebFrame, 0, 2)
	for offset := 0; offset < len(body); {
		if len(body)-offset < 5 {
			return nil, errGRPCWebInvalid
		}
		flags := body[offset]
		if flags != 0 && flags != 0x80 {
			return nil, errGRPCWebInvalid
		}
		length := uint64(binary.BigEndian.Uint32(body[offset+1 : offset+5]))
		offset += 5
		if length > uint64(len(body)-offset) {
			return nil, errGRPCWebInvalid
		}
		end := offset + int(length)
		payload := append([]byte(nil), body[offset:end]...)
		frames = append(frames, grpcWebFrame{Trailer: flags&0x80 != 0, Payload: payload})
		offset = end
	}
	return frames, nil
}
