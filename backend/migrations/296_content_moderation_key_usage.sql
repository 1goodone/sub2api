CREATE TABLE IF NOT EXISTS content_moderation_key_usage (
    engine VARCHAR(32) NOT NULL,
    key_hash VARCHAR(128) NOT NULL,
    bucket VARCHAR(16) NOT NULL,
    usage_date DATE NOT NULL,
    calls BIGINT NOT NULL DEFAULT 0,
    success BIGINT NOT NULL DEFAULT 0,
    errors BIGINT NOT NULL DEFAULT 0,
    latency_ms_sum BIGINT NOT NULL DEFAULT 0,
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (engine, key_hash, bucket, usage_date)
);
