package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

func (r *openAIOAuthReauthRepository) ListSIWCStatuses(ctx context.Context, ids []int64) ([]service.SIWCAccountSnapshot, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT a.id,
		       COALESCE(c.credentials_ciphertext, '') <> '',
		       COALESCE(a.extra->>'openai_siwc', '') = 'true',
		       t.id, t.status, t.stage, COALESCE(t.error_message, '')
		FROM accounts a
		LEFT JOIN openai_siwc_credentials c ON c.account_id = a.id
		LEFT JOIN LATERAL (
			SELECT id, status, stage, error_message
			FROM openai_oauth_reauth_tasks
			WHERE account_id = a.id AND oauth_profile = 'siwc'
			  AND NOT EXISTS (
			    SELECT 1 FROM openai_totp_rotations rotation
			    WHERE rotation.task_id = openai_oauth_reauth_tasks.id
			  )
			ORDER BY created_at DESC, id DESC
			LIMIT 1
		) t ON true
		WHERE a.id = ANY($1)
		  AND a.deleted_at IS NULL
		  AND a.platform = 'openai'
		  AND a.type = 'oauth'
		  AND a.parent_account_id IS NULL
	`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []service.SIWCAccountSnapshot
	for rows.Next() {
		var item service.SIWCAccountSnapshot
		var taskID sql.NullInt64
		var status, stage sql.NullString
		var taskError string
		if err := rows.Scan(&item.AccountID, &item.Bound, &item.Enabled, &taskID, &status, &stage, &taskError); err != nil {
			return nil, err
		}
		if taskID.Valid {
			item.Task = &service.OpenAIOAuthReauthTaskRecord{
				OAuthProfile: "siwc",
				ID:           taskID.Int64,
				AccountID:    item.AccountID,
				Status:       status.String,
				Stage:        stage.String,
				Error:        taskError,
			}
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *openAIOAuthReauthRepository) SIWCRegistration(ctx context.Context, id int64, host string) (string, string, error) {
	var saved, ciphertext string
	err := r.db.QueryRowContext(ctx, `INSERT INTO openai_siwc_credentials(account_id,host_id) VALUES($1,$2)
 ON CONFLICT(account_id) DO UPDATE SET account_id=EXCLUDED.account_id RETURNING host_id,credentials_ciphertext`, id, host).Scan(&saved, &ciphertext)
	return saved, ciphertext, err
}
func (r *openAIOAuthReauthRepository) GetSIWCCredentials(ctx context.Context, id int64) (string, error) {
	var ciphertext string
	err := r.db.QueryRowContext(ctx, `SELECT credentials_ciphertext FROM openai_siwc_credentials WHERE account_id=$1`, id).Scan(&ciphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return ciphertext, err
}
func (r *openAIOAuthReauthRepository) CreateSIWCTask(ctx context.Context, id int64, hash string) (*service.OpenAIOAuthReauthTaskRecord, error) {
	row := r.db.QueryRowContext(ctx, `INSERT INTO openai_oauth_reauth_tasks(account_id,status,stage,expected_credentials_hash,oauth_profile)
 VALUES($1,'queued','queued',$2,'siwc') RETURNING id,account_id,status,stage,COALESCE(worker_id,''),COALESCE(auth_session_id,''),expected_credentials_hash,
 COALESCE(error_message,''),attempt,created_at,updated_at,finished_at,oauth_profile`, id, hash)
	record, err := scanOpenAIOAuthReauthTask(row)
	var pg *pq.Error
	if errors.As(err, &pg) && pg.Code == "23505" {
		return nil, infraerrors.Conflict("OPENAI_REAUTH_TASK_ALREADY_ACTIVE", "an authorization task is already active")
	}
	return record, err
}

// Account identity, rotating grant and task ownership are checked in one transaction.
func (r *openAIOAuthReauthRepository) SaveSIWCCredentials(ctx context.Context, id int64, credentials map[string]any, expected, replacement string, exactCredentials bool, task *service.OpenAIOAuthReauthTaskRecord) (bool, error) {
	raw, err := json.Marshal(credentials)
	if err != nil {
		return false, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	var locked int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE id=$1 AND deleted_at IS NULL AND platform='openai' AND type='oauth' AND parent_account_id IS NULL
 AND (($3 AND credentials=$2::jsonb) OR (NOT $3
 AND credentials->>'chatgpt_account_id' IS NOT DISTINCT FROM ($2::jsonb)->>'chatgpt_account_id'
 AND credentials->>'chatgpt_user_id' IS NOT DISTINCT FROM ($2::jsonb)->>'chatgpt_user_id'
 AND credentials->>'email' IS NOT DISTINCT FROM ($2::jsonb)->>'email'))
 FOR UPDATE`, id, string(raw), exactCredentials).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if task != nil {
		err = tx.QueryRowContext(ctx, `SELECT id FROM openai_oauth_reauth_tasks WHERE id=$1 AND account_id=$2 AND worker_id=$3 AND oauth_profile='siwc' AND status='callback_processing' FOR UPDATE`, task.ID, id, task.WorkerID).Scan(&locked)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE openai_siwc_credentials SET credentials_ciphertext=$3,updated_at=NOW() WHERE account_id=$1 AND credentials_ciphertext=$2`, id, expected, replacement)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return false, err
	}
	if task != nil {
		if _, err = tx.ExecContext(ctx, `UPDATE openai_oauth_reauth_tasks SET status='succeeded',stage='succeeded',error_message=NULL,finished_at=NOW(),updated_at=NOW() WHERE id=$1`, task.ID); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}
