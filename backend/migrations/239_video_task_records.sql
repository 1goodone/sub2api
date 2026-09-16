-- Video ownership and accepted receipts must outlive Redis sticky sessions.
CREATE TABLE IF NOT EXISTS video_task_records (
    operation_key VARCHAR(64) PRIMARY KEY,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    group_id BIGINT NOT NULL DEFAULT 0,
    account_id BIGINT NOT NULL,
    request_hash VARCHAR(64) NOT NULL,
    task_id VARCHAR(255) NOT NULL DEFAULT '',
    response_status INTEGER NOT NULL DEFAULT 0,
    response_body BYTEA,
    pending_billing JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS video_task_records_owner_task_idx
    ON video_task_records (user_id, api_key_id, task_id) WHERE task_id <> '';

-- Recover only ownership proven by an existing, task-scoped billed usage record.
-- This does not create usage or change balances, and does not guess failed jobs.
INSERT INTO video_task_records
    (operation_key,user_id,api_key_id,group_id,account_id,request_hash,task_id,response_status,response_body,pending_billing,created_at)
SELECT encode(sha256(convert_to('legacy-video:' || u.api_key_id::text || ':' || u.request_id,'UTF8')),'hex'),
    u.user_id,u.api_key_id,COALESCE(u.group_id,0),u.account_id,'',substring(u.request_id from 12),200,
    convert_to(json_build_object('id',substring(u.request_id from 12),'status','queued')::text,'UTF8'),
    jsonb_build_object('billing_request_id',u.request_id,'model',u.model,'billing_model',u.model,'upstream_model',COALESCE(u.upstream_model,''),
        'original_model',COALESCE(u.requested_model,u.model),'video_resolution',COALESCE(u.video_resolution,''),
        'video_duration_seconds',COALESCE(u.video_duration_seconds,0),'created_at',u.created_at),u.created_at
FROM usage_logs u JOIN accounts a ON a.id=u.account_id
WHERE a.platform='openai' AND a.type='apikey' AND u.video_count>0
    AND u.request_id LIKE 'grok-video:%' AND length(u.request_id)>11
ON CONFLICT DO NOTHING;
