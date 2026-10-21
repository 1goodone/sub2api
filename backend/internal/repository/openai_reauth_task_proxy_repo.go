package repository

import (
	"context"
	"database/sql"
	"errors"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

func (r *openAIOAuthReauthRepository) CreateExcelTaskWithProxy(ctx context.Context, accountID int64, hash, proxyCiphertext string) (*service.OpenAIOAuthReauthTaskRecord, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	row := tx.QueryRowContext(ctx, `
 INSERT INTO openai_oauth_reauth_tasks(account_id,status,stage,expected_credentials_hash,oauth_profile)
 VALUES($1,'queued','queued',$2,'excel')
 RETURNING id,account_id,status,stage,COALESCE(worker_id,''),COALESCE(auth_session_id,''),
 expected_credentials_hash,COALESCE(error_message,''),attempt,created_at,updated_at,finished_at,oauth_profile`, accountID, hash)
	record, err := scanOpenAIOAuthReauthTask(row)
	var pg *pq.Error
	if errors.As(err, &pg) && pg.Code == "23505" {
		return nil, infraerrors.Conflict("OPENAI_REAUTH_TASK_ALREADY_ACTIVE", "an authorization task is already active for this account")
	}
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO openai_oauth_reauth_task_proxies(task_id, proxy_ciphertext) VALUES($1,$2)`, record.ID, proxyCiphertext); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return record, nil
}

func (r *openAIOAuthReauthRepository) CreateExcelTaskWithProxyPool(ctx context.Context, accountID int64, hash, proxyCiphertext, poolCiphertext string, attempt int) (*service.OpenAIOAuthReauthTaskRecord, error) {
	if attempt < 1 {
		attempt = 1
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	row := tx.QueryRowContext(ctx, `
 INSERT INTO openai_oauth_reauth_tasks(account_id,status,stage,expected_credentials_hash,oauth_profile)
 VALUES($1,'queued','queued',$2,'excel')
 RETURNING id,account_id,status,stage,COALESCE(worker_id,''),COALESCE(auth_session_id,''),
 expected_credentials_hash,COALESCE(error_message,''),attempt,created_at,updated_at,finished_at,oauth_profile`, accountID, hash)
	record, err := scanOpenAIOAuthReauthTask(row)
	var pg *pq.Error
	if errors.As(err, &pg) && pg.Code == "23505" {
		return nil, infraerrors.Conflict("OPENAI_REAUTH_TASK_ALREADY_ACTIVE", "an authorization task is already active for this account")
	}
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO openai_oauth_reauth_task_proxies(task_id, proxy_ciphertext, pool_ciphertext, attempt) VALUES($1,$2,$3,$4)`, record.ID, proxyCiphertext, poolCiphertext, attempt); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return record, nil
}

func (r *openAIOAuthReauthRepository) GetTaskProxyRotation(ctx context.Context, taskID int64) (string, int, error) {
	var pool string
	var attempt int
	err := r.db.QueryRowContext(ctx, `SELECT COALESCE(pool_ciphertext,''), attempt FROM openai_oauth_reauth_task_proxies WHERE task_id=$1`, taskID).Scan(&pool, &attempt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, nil
	}
	return pool, attempt, err
}

func (r *openAIOAuthReauthRepository) GetTaskProxyCiphertext(ctx context.Context, taskID int64) (string, error) {
	var ciphertext string
	err := r.db.QueryRowContext(ctx, `SELECT proxy_ciphertext FROM openai_oauth_reauth_task_proxies WHERE task_id=$1`, taskID).Scan(&ciphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return ciphertext, err
}
