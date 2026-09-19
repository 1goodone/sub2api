package service

import "strings"

// Lengths are the operator's measured acceptance rules, not a protocol guarantee.
// Unknown plans must never silently inherit a personal-plan classification.
func CodexTurnStateHealthyLength(account *Account) int {
	if !IsCodexTurnStateProbeAccount(account) {
		return 0
	}
	plan := strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(account.GetCredential("plan_type"))))
	switch plan {
	case "plus", "pro", "chatgptpro", "prolite", "pro5x", "pro20x":
		return 292
	case "team":
		return 332
	default:
		return 0
	}
}

func codexTurnStateMetadataEvent(kind string) bool {
	return kind == "response.metadata" || kind == "codex.response.metadata"
}
