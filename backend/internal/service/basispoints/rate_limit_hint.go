package basispoints

import (
	"context"
	"encoding/json"
	"strconv"
)

// RateLimitHint contains only structured timing metadata, never upstream text.
// It travels out-of-band: Details and normalized client events omit it.
type RateLimitHint struct {
	ResetAtUnix       int64
	ResetAfterSeconds int64
}

func positiveLimitInteger(value any) int64 {
	var raw string
	switch v := value.(type) {
	case json.Number:
		raw = string(v)
	case string:
		raw = v
	default:
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// ParseRateLimitHint accepts HTTP error bodies and Responses failure envelopes.
// Callers must establish that the response is a 429 before using this hint.
func ParseRateLimitHint(raw []byte) *RateLimitHint {
	var payload object
	if decode(raw, &payload) != nil {
		return nil
	}
	return rateLimitHint(payload)
}

func rateLimitHint(payload object) *RateLimitHint {
	response, _ := payload["response"].(object)
	detail, _ := response["error"].(object)
	if detail == nil {
		detail, _ = payload["error"].(object)
	}
	if detail == nil {
		detail = payload
	}
	hint := &RateLimitHint{ResetAtUnix: positiveLimitInteger(detail["resets_at"]), ResetAfterSeconds: positiveLimitInteger(detail["resets_in_seconds"])}
	if hint.ResetAtUnix == 0 && hint.ResetAfterSeconds == 0 {
		return nil
	}
	return hint
}

type upstreamFailureObserverKey struct{}

// WithUpstreamFailureObserver observes safe failure metadata before the bridge
// normalizes the event. The callback runs on the stream goroutine; consumers
// must synchronize access and must not write to the client from this callback.
func WithUpstreamFailureObserver(ctx context.Context, observer func(*UpstreamFailure)) context.Context {
	return context.WithValue(ctx, upstreamFailureObserverKey{}, observer)
}

func observeUpstreamFailure(ctx context.Context, kind string, payload object) {
	observer, _ := ctx.Value(upstreamFailureObserverKey{}).(func(*UpstreamFailure))
	if observer != nil {
		if failure := classifyUpstreamFailure(kind, payload); failure != nil {
			observer(failure)
		}
	}
}
