package repository

import (
	"context"
	"encoding/json"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"time"
)

func (r *accountRepository) AppendCodexStateEvent(ctx context.Context, event service.CodexStateEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = r.sql.ExecContext(ctx, `INSERT INTO codex_state_events(account_id,model,created_at,payload) VALUES($1,$2,$3,$4)`, event.AccountID, event.Model, event.CreatedAt, string(payload))
	return err
}
func (r *accountRepository) ListCodexStateEvents(ctx context.Context, id int64, model string, before int64, limit int) ([]service.CodexStateEvent, error) {
	rows, err := r.sql.QueryContext(ctx, `SELECT id,payload FROM codex_state_events WHERE account_id=$1 AND model=$2 AND ($3::bigint=0 OR id<$3) AND created_at>NOW()-INTERVAL '7 days' ORDER BY id DESC LIMIT $4`, id, model, before, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	events := make([]service.CodexStateEvent, 0)
	for rows.Next() {
		var event service.CodexStateEvent
		var key int64
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &event); err != nil {
			return nil, err
		}
		event.ID = key
		events = append(events, event)
	}
	return events, rows.Err()
}
func (r *accountRepository) CleanupCodexStateEvents(ctx context.Context, before time.Time) error {
	_, err := r.sql.ExecContext(ctx, `DELETE FROM codex_state_events WHERE created_at<$1`, before)
	return err
}
