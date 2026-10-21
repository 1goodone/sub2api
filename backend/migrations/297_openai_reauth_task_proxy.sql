-- One-shot BPS login proxies stay on the task and are not copied onto the account.
CREATE TABLE IF NOT EXISTS openai_oauth_reauth_task_proxies (
    task_id BIGINT PRIMARY KEY REFERENCES openai_oauth_reauth_tasks(id) ON DELETE CASCADE,
    proxy_ciphertext TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
