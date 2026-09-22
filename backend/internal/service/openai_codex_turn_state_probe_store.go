package service

import (
	"context"
	"errors"
)

// CodexTurnStateProbeRepository fences background and observation writes against
// the credentials, configuration and runtime snapshot that produced them.
// An old response or a slow probe must never replace a newer candidate.
type CodexTurnStateProbeRepository interface {
	CompareAndSwapCodexTurnStateProbe(context.Context, *Account, map[string]any) (bool, error)
}

func codexTurnStateProbeRuntimeUpdates(cache map[string]codexTurnStateCacheEntry, failures map[string]codexTurnStateProbeFailure) map[string]any {
	updates := map[string]any{CodexTurnStateProbeCacheExtraKey: nil, CodexTurnStateProbeFailureExtraKey: nil}
	if encoded := codexTurnStateCacheToExtra(cache); encoded != nil {
		updates[CodexTurnStateProbeCacheExtraKey] = encoded
	}
	if encoded := codexTurnStateProbeFailuresToExtra(failures); encoded != nil {
		updates[CodexTurnStateProbeFailureExtraKey] = encoded
	}
	return updates
}

func codexTurnStateProbeRuntimeUpdatesWithIdentity(cache map[string]codexTurnStateCacheEntry, failures map[string]codexTurnStateProbeFailure, identity map[string]any) map[string]any {
	updates := codexTurnStateProbeRuntimeUpdates(cache, failures)
	updates[CodexTurnStateOutboundIdentityExtraKey] = identity
	return updates
}

func saveCodexTurnStateProbeRuntime(ctx context.Context, repo AccountRepository, expected *Account, updates map[string]any) (bool, error) {
	store, ok := repo.(CodexTurnStateProbeRepository)
	if !ok {
		return false, errors.New("codex turn-state repository does not support conditional updates")
	}
	return store.CompareAndSwapCodexTurnStateProbe(ctx, expected, updates)
}

func retryCodexTurnStateProbe(ctx context.Context, repo AccountRepository, id int64) error {
	for range 3 {
		account, err := repo.GetByID(ctx, id)
		if err != nil {
			return err
		}
		config, err := CodexTurnStateProbeConfigFromExtra(account.Extra)
		if err != nil || !config.Enabled || !IsCodexTurnStateProbeAccount(account) {
			return errors.New("codex turn-state probing must be enabled before retrying")
		}
		updates := codexTurnStateProbeRuntimeUpdates(codexTurnStateCacheFromExtra(account.Extra), nil)
		saved, err := saveCodexTurnStateProbeRuntime(ctx, repo, account, updates)
		if err != nil || saved {
			return err
		}
	}
	return errors.New("codex turn-state changed concurrently; retry the operation")
}

// PreserveCodexTurnStateProbeRuntimeForEdit takes runtime fields from the locked
// database row, never from an edit form or a stale service read. Switching
// engines keeps the dormant pool intact; only credential-owner changes discard it.
func PreserveCodexTurnStateProbeRuntimeForEdit(updated, current *Account) {
	if updated == nil {
		return
	}
	if updated.Extra == nil {
		updated.Extra = make(map[string]any)
	}
	for _, key := range []string{CodexTurnStateProbeCacheExtraKey, CodexTurnStateProbeFailureExtraKey, CodexTurnStateOutboundIdentityExtraKey, CodexTurnStateRoutingCookieExtraKey} {
		delete(updated.Extra, key)
	}
	if !IsCodexTurnStateProbeAccount(updated) || !IsCodexTurnStateProbeAccount(current) {
		return
	}
	if updated.Type != current.Type || codexTurnStateOwnerIdentity(updated) != codexTurnStateOwnerIdentity(current) {
		return
	}
	for _, key := range []string{CodexTurnStateProbeCacheExtraKey, CodexTurnStateProbeFailureExtraKey, CodexTurnStateOutboundIdentityExtraKey, CodexTurnStateRoutingCookieExtraKey} {
		if value, ok := current.Extra[key]; ok {
			updated.Extra[key] = value
		}
	}
}

// Only credential-owner fields affect candidate identity. Resubmitting model
// mappings or unrelated credential options must not reset an existing state.
type codexTurnStateCredentialOwner struct {
	token     string
	accountID string
	fedRAMP   bool
}

func codexTurnStateOwnerIdentity(account *Account) codexTurnStateCredentialOwner {
	if account == nil {
		return codexTurnStateCredentialOwner{}
	}
	return codexTurnStateCredentialOwner{token: account.GetOpenAIAccessToken(), accountID: account.GetChatGPTAccountID(), fedRAMP: account.IsChatGPTAccountFedRAMP()}
}
