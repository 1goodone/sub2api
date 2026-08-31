CREATE TABLE IF NOT EXISTS api_key_packages (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    cycle VARCHAR(10) NOT NULL CHECK (cycle IN ('week', 'month')),
    amount_usd DECIMAL(20,8) NOT NULL CHECK (amount_usd > 0),
    limit_5h_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    limit_8h_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    limit_7d_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_api_key_packages_user_id
    ON api_key_packages (user_id);

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS package_id BIGINT,
    ADD COLUMN IF NOT EXISTS rate_limit_8h DECIMAL(20,8) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS usage_8h DECIMAL(20,8) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS window_8h_start TIMESTAMPTZ;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'api_keys_package_id_fkey'
          AND conrelid = 'api_keys'::regclass
    ) THEN
        ALTER TABLE api_keys
            ADD CONSTRAINT api_keys_package_id_fkey
            FOREIGN KEY (package_id) REFERENCES api_key_packages(id) ON DELETE RESTRICT;
    END IF;
END;
$$;

CREATE INDEX IF NOT EXISTS idx_api_keys_package_id
    ON api_keys (package_id);
