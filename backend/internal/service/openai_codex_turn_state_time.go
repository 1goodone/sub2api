package service

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
	"time"
)

// The timestamp is readable without the upstream key; this does not verify a signature.
func codexStateTimes(state string, obtained, now time.Time) (time.Time, time.Time, error) {
	raw, err := base64.URLEncoding.DecodeString(state)
	if err != nil {
		raw, err = base64.RawURLEncoding.DecodeString(strings.TrimRight(state, "="))
	}
	if err != nil || len(raw) < 73 || raw[0] != 0x80 || (len(raw)-57)%16 != 0 {
		return time.Time{}, time.Time{}, errors.New("invalid_state_format")
	}
	seconds := binary.BigEndian.Uint64(raw[1:9])
	if seconds > uint64(now.Add(time.Minute).Unix()) {
		return time.Time{}, time.Time{}, errors.New("future_state_timestamp")
	}
	issued := time.Unix(int64(seconds), 0).UTC()
	expires := issued.Add(codexTurnStateTTL)
	if limit := obtained.Add(codexTurnStateTTL); limit.Before(expires) {
		expires = limit
	}
	if !expires.After(now) {
		return issued, expires, errors.New("expired_state")
	}
	return issued, expires, nil
}

func codexStateEffectiveExpiry(entry codexTurnStateCacheEntry, now time.Time) time.Time {
	_, expires, err := codexStateTimes(entry.State, entry.ObtainedAt, now)
	if err != nil {
		return time.Time{}
	}
	if entry.ExpiresAt.Before(expires) {
		expires = entry.ExpiresAt
	}
	return expires
}
