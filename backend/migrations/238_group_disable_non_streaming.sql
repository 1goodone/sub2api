ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS disable_non_streaming BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN groups.disable_non_streaming IS
    'Reject non-streaming text generation while preserving media and streaming APIs';
