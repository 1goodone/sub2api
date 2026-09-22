package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// CodexTicketHarvestScope controls background ticket collection only.
type CodexTicketHarvestScope struct {
	Mode     string  `json:"mode"`
	GroupIDs []int64 `json:"group_ids"`
}

func NormalizeCodexTicketHarvestScope(scope CodexTicketHarvestScope) (CodexTicketHarvestScope, error) {
	if scope.Mode == "" {
		scope.Mode = "all"
	}
	if scope.Mode != "all" && scope.Mode != "selected" {
		return scope, fmt.Errorf("harvest scope mode must be all or selected")
	}
	if len(scope.GroupIDs) > 1000 {
		return scope, fmt.Errorf("at most 1000 harvest groups are allowed")
	}
	ids := append([]int64{}, scope.GroupIDs...)
	for _, id := range ids {
		if id <= 0 {
			return scope, fmt.Errorf("harvest group IDs must be positive")
		}
	}
	slices.Sort(ids)
	scope.GroupIDs = slices.Compact(ids)
	return scope, nil
}

func parseCodexTicketHarvestScope(raw string) (CodexTicketHarvestScope, error) {
	if strings.TrimSpace(raw) == "" {
		return NormalizeCodexTicketHarvestScope(CodexTicketHarvestScope{})
	}
	var scope CodexTicketHarvestScope
	if err := json.Unmarshal([]byte(raw), &scope); err != nil {
		return scope, fmt.Errorf("invalid harvest scope JSON")
	}
	if scope.Mode == "" {
		return scope, fmt.Errorf("harvest scope mode is required")
	}
	return NormalizeCodexTicketHarvestScope(scope)
}

// GetCodexTicketHarvestScope reads one complete scope per harvest round. A
// storage or parse error is returned so callers can fail closed for that round.
func (s *SettingService) GetCodexTicketHarvestScope(ctx context.Context) (CodexTicketHarvestScope, error) {
	if s == nil || s.settingRepo == nil {
		return parseCodexTicketHarvestScope("")
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpenAICodexTicketHarvestScope)
	if errors.Is(err, ErrSettingNotFound) {
		return parseCodexTicketHarvestScope("")
	}
	if err != nil {
		return CodexTicketHarvestScope{}, err
	}
	return parseCodexTicketHarvestScope(raw)
}

func (scope CodexTicketHarvestScope) includes(account *Account) bool {
	if account == nil {
		return false
	}
	if scope.Mode == "all" {
		return true
	}
	for _, id := range account.GroupIDs {
		if slices.Contains(scope.GroupIDs, id) {
			return true
		}
	}
	return false
}

// priority returns the best selected membership priority, with the account
// priority as a fallback for legacy snapshots without membership metadata.
func (scope CodexTicketHarvestScope) priority(account *Account) int {
	if account == nil {
		return 0
	}
	best, found := account.Priority, false
	for _, membership := range account.AccountGroups {
		if scope.Mode == "selected" && !slices.Contains(scope.GroupIDs, membership.GroupID) {
			continue
		}
		if !found || membership.Priority < best {
			best, found = membership.Priority, true
		}
	}
	return best
}

type codexHarvestTier struct {
	Schedulable     bool
	Priority        int
	AccountPriority int
}

func (scope CodexTicketHarvestScope) tier(account *Account) codexHarvestTier {
	if account == nil {
		return codexHarvestTier{}
	}
	return codexHarvestTier{Schedulable: account.Schedulable, Priority: scope.priority(account), AccountPriority: account.Priority}
}
