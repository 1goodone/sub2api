ALTER TABLE account_groups
    ADD COLUMN IF NOT EXISTS is_focus BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE account_groups ag
SET is_focus = TRUE
FROM groups g
WHERE g.id = ag.group_id
  AND g.focus_account_id = ag.account_id
  AND NOT ag.is_focus;

UPDATE groups g
SET focus_account_id = NULL
WHERE g.focus_account_id IS NOT NULL
  AND NOT EXISTS (
      SELECT 1
      FROM account_groups ag
      WHERE ag.group_id = g.id
        AND ag.account_id = g.focus_account_id
  );

CREATE INDEX IF NOT EXISTS idx_account_groups_focus_pool
    ON account_groups (group_id, priority, account_id)
    WHERE is_focus = TRUE;
