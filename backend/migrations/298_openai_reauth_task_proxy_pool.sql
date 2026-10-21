-- Remaining one-shot BPS login proxies stay encrypted on the task.
-- They are not copied onto the account.
ALTER TABLE openai_oauth_reauth_task_proxies
    ADD COLUMN IF NOT EXISTS pool_ciphertext TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS attempt INTEGER NOT NULL DEFAULT 1;
