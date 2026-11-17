-- Credential inspections already emit blocked for upstream challenges.
ALTER TABLE account_token_guard_v2_accounts
    DROP CONSTRAINT IF EXISTS account_token_guard_v2_probe_state_check;

ALTER TABLE account_token_guard_v2_accounts
    ADD CONSTRAINT account_token_guard_v2_probe_state_check
    CHECK (probe_state IN ('pending', 'ok', 'auth', 'transient', 'blocked'));
