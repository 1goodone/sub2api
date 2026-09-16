package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type videoTaskRepository struct{ db *sql.DB }

func NewVideoTaskRepository(db *sql.DB) service.VideoTaskRepository { return &videoTaskRepository{db} }

func (r *videoTaskRepository) Reserve(ctx context.Context, task *service.VideoTaskRecord) (bool, error) {
	pending, err := json.Marshal(task.Pending)
	if err != nil {
		return false, err
	}
	res, err := r.db.ExecContext(ctx, `INSERT INTO video_task_records
		(operation_key,user_id,api_key_id,group_id,account_id,request_hash,pending_billing)
		VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`,
		task.OperationKey, task.UserID, task.APIKeyID, task.GroupID, task.AccountID, task.RequestHash, pending)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (r *videoTaskRepository) Finish(ctx context.Context, task *service.VideoTaskRecord) error {
	res, err := r.db.ExecContext(ctx, `UPDATE video_task_records SET task_id=$1,response_status=$2,response_body=$3,updated_at=NOW()
		WHERE operation_key=$4 AND user_id=$5 AND api_key_id=$6 AND account_id=$7 AND request_hash=$8`,
		task.TaskID, task.ResponseStatus, task.ResponseBody, task.OperationKey, task.UserID, task.APIKeyID, task.AccountID, task.RequestHash)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n != 1 {
		return fmt.Errorf("video task receipt was not saved")
	}
	return err
}

const videoRecordColumns = `operation_key,user_id,api_key_id,group_id,account_id,request_hash,task_id,response_status,response_body,pending_billing`

func (r *videoTaskRepository) get(ctx context.Context, predicate string, value string, userID, apiKeyID int64) (*service.VideoTaskRecord, error) {
	var task service.VideoTaskRecord
	var pending []byte
	err := r.db.QueryRowContext(ctx, `SELECT `+videoRecordColumns+` FROM video_task_records WHERE `+predicate+`=$1 AND user_id=$2 AND api_key_id=$3`, value, userID, apiKeyID).Scan(
		&task.OperationKey, &task.UserID, &task.APIKeyID, &task.GroupID, &task.AccountID, &task.RequestHash, &task.TaskID, &task.ResponseStatus, &task.ResponseBody, &pending)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(pending, &task.Pending); err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *videoTaskRepository) GetOperation(ctx context.Context, key string, userID, apiKeyID int64) (*service.VideoTaskRecord, error) {
	return r.get(ctx, "operation_key", key, userID, apiKeyID)
}

func (r *videoTaskRepository) GetTask(ctx context.Context, id string, userID, apiKeyID int64) (*service.VideoTaskRecord, error) {
	return r.get(ctx, "task_id", id, userID, apiKeyID)
}
