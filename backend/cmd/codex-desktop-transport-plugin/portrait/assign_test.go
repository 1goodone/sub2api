package portrait

import "testing"

func TestAssignPortraitStickyModuloAndFallback(t *testing.T) {
	filled := []string{"P2"}
	if got := AssignPortrait(2, "", filled, "P2"); got != "P2" {
		t.Fatalf("id=2 slot P4 empty should fallback, got %s", got)
	}
	if got := AssignPortrait(0, "", filled, "P2"); got != "P2" {
		t.Fatalf("id=0 want P2, got %s", got)
	}
	if a, b := AssignPortrait(7, "", filled, "P2"), AssignPortrait(7, "", filled, "P2"); a != b {
		t.Fatal("assignment must be sticky")
	}
	if got := AssignPortrait(1, "P2", filled, "P2"); got != "P2" {
		t.Fatalf("pin P2: %s", got)
	}
	if got := AssignPortrait(1, "P5", filled, "P2"); got != "P2" {
		t.Fatalf("pin missing slot must fallback, got %s", got)
	}
}

func TestAssignPortraitUsesFilledSlot(t *testing.T) {
	filled := []string{"P2", "P3"}
	if got := AssignPortrait(1, "", filled, "P2"); got != "P3" {
		t.Fatalf("id=1 is P3, got %s", got)
	}
}
