package requestcapture

import (
	"bytes"
	"encoding/json"
	"reflect"
)

// WSRequest begins one actual upstream frame/attempt. RawMessage avoids an extra
// encoding allocation. Other values must reserve a conservative encoding bound.
func (s *Session) WSRequest(account int64, value any) int {
	if s == nil {
		return 0
	}
	n := s.BeginAttempt(account)
	var payload []byte
	switch v := value.(type) {
	case json.RawMessage:
		payload = v
	case []byte:
		payload = v
	default:
		charge := jsonBound(reflect.ValueOf(value), 0)
		if charge > BufferLimit || !s.m.reserve(charge) {
			s.MarkPartial("serialization_buffer_limit")
			return n
		}
		defer s.m.buffer.Add(-charge)
		var err error
		payload, err = json.Marshal(value)
		if err != nil {
			s.MarkPartial("serialization_failed")
			return n
		}
	}
	s.Frame("upstream_request", n, payload)
	return n
}
func (s *Session) LastAttempt() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.attempts)
}

// WebSocket response frames are stored as SSE data events in one stage/turn
// segment, preserving frame order without a separate file per delta.
func (s *Session) Frame(stage string, attempt int, payload []byte) {
	if s == nil {
		return
	}
	if stage != "upstream_response" && stage != "client_response" {
		stream := s.NewStream(stage, attempt, 0, "application/json", nil)
		_, _ = stream.Write(payload)
		_ = stream.Close()
		return
	}
	if stage == "client_response" {
		s.ObserveResult(payload)
	}
	s.frameMu.Lock()
	s.mu.Lock()
	turn := s.turn
	closed := s.closed || s.failed
	s.mu.Unlock()
	if closed {
		s.frameMu.Unlock()
		return
	}
	if s.frameStreams == nil {
		s.frameStreams = map[string]*Stream{}
	}
	stream := s.frameStreams[stage]
	if stream != nil && (stream.part.Attempt != attempt || stream.part.Turn != turn) {
		_ = stream.Close()
		stream = nil
	}
	if stream == nil {
		stream = s.NewStream(stage, attempt, turn, "text/event-stream; profile=websocket-frames", nil)
		s.frameStreams[stage] = stream
	}
	for len(payload) > 0 {
		line, rest, found := bytes.Cut(payload, []byte{'\n'})
		_, _ = stream.Write([]byte("data: "))
		_, _ = stream.Write(line)
		_, _ = stream.Write([]byte("\n"))
		payload = rest
		if !found {
			break
		}
	}
	_, _ = stream.Write([]byte("\n"))
	s.frameMu.Unlock()
}
func (s *Session) closeFrameStreams() {
	s.frameMu.Lock()
	defer s.frameMu.Unlock()
	for key, stream := range s.frameStreams {
		_ = stream.Close()
		delete(s.frameStreams, key)
	}
}
func jsonBound(v reflect.Value, depth int) int64 {
	if depth > 128 {
		return BufferLimit + 1
	}
	if !v.IsValid() {
		return 16
	}
	switch v.Kind() {
	case reflect.Interface, reflect.Pointer:
		if v.IsNil() {
			return 16
		}
		return jsonBound(v.Elem(), depth+1)
	case reflect.String:
		return int64(v.Len())*6 + 16
	case reflect.Map:
		n := int64(64)
		it := v.MapRange()
		for it.Next() {
			n += jsonBound(it.Key(), depth+1) + jsonBound(it.Value(), depth+1)
			if n > BufferLimit {
				return n
			}
		}
		return n * 2
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return int64(v.Len())*8 + 64
		}
		n := int64(64)
		for i := 0; i < v.Len(); i++ {
			n += jsonBound(v.Index(i), depth+1)
			if n > BufferLimit {
				return n
			}
		}
		return n * 2
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return 64
	default:
		return BufferLimit + 1
	}
}
