ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS disabled_models JSONB NOT NULL DEFAULT '[]'::jsonb;

COMMENT ON COLUMN groups.disabled_models IS '分组禁用模型列表；优先于模型映射和账号调度';
