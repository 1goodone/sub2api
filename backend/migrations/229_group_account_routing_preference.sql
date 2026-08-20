ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS focus_account_id BIGINT NULL REFERENCES accounts(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS account_type_preference VARCHAR NOT NULL DEFAULT 'default';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'groups'::regclass
          AND conname = 'groups_account_type_preference_check'
    ) THEN
        ALTER TABLE groups
            ADD CONSTRAINT groups_account_type_preference_check
            CHECK (account_type_preference IN ('default', 'auth_first', 'apikey_first'));
    END IF;
END $$;
