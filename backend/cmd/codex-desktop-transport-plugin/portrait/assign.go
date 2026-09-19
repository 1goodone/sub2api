package portrait

import (
	"strings"
)

// AssignPortrait maps accountID onto P2–P6. Empty filled slots fall back.
// The slot index is accountID % 5 and is never shuffled.
func AssignPortrait(accountID int64, pin string, filled []string, fallback string) string {
	if fallback == "" {
		fallback = FallbackPortrait
	}
	pin = strings.TrimSpace(pin)
	if pin != "" {
		if containsSlot(filled, pin) {
			return pin
		}
		return fallback
	}
	if len(PortraitSlots) == 0 {
		return fallback
	}
	idx := int(uint64(accountID) % uint64(len(PortraitSlots)))
	slot := PortraitSlots[idx]
	if containsSlot(filled, slot) {
		return slot
	}
	return fallback
}

func containsSlot(filled []string, slot string) bool {
	for _, item := range filled {
		if item == slot {
			return true
		}
	}
	return false
}
