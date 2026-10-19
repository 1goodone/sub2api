-- Employee API admission only. Internal account tests and quality judges select
-- accounts directly and do not pass through the public gateway middleware.
-- Local numbering starts at 292 because 287-291 are already used by this fork.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS codex_cli_only BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN groups.codex_cli_only IS
    'Only allow Codex clients at employee API ingress; internal account tests and quality judges are unaffected';
