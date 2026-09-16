//go:build video_contract

package repository

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestVideoTaskPostgresDurabilityAndAtomicReservation(t *testing.T) {
	dsn := os.Getenv("VIDEO_CONTRACT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL database required")
	}
	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	ctx := context.Background()
	_, err = db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS accounts(id bigint PRIMARY KEY,platform text,type text);
	 CREATE TABLE IF NOT EXISTS usage_logs(user_id bigint,api_key_id bigint,group_id bigint,account_id bigint,request_id text,model text,upstream_model text,requested_model text,video_count integer,video_resolution text,video_duration_seconds integer,created_at timestamptz);
	 INSERT INTO accounts VALUES (91,'openai','apikey') ON CONFLICT DO NOTHING;
	 INSERT INTO usage_logs VALUES(10,20,53,91,'grok-video:historical-task','video3.0','3.0','video3.0',1,'720p',5,NOW());`)
	require.NoError(t, err)
	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "239_video_task_records.sql"))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	repo := NewVideoTaskRepository(db)
	legacy, err := repo.GetTask(ctx, "historical-task", 10, 20)
	require.NoError(t, err)
	require.NotNil(t, legacy)
	require.Equal(t, int64(91), legacy.AccountID)
	require.Equal(t, "grok-video:historical-task", legacy.Pending.BillingRequestID)
	_, err = db.ExecContext(ctx, `DELETE FROM video_task_records WHERE operation_key='concurrent-operation'`)
	require.NoError(t, err)
	task := service.VideoTaskRecord{OperationKey: "concurrent-operation", UserID: 10, APIKeyID: 20, GroupID: 53, AccountID: 91, RequestHash: "body-hash", Pending: service.GrokVideoPendingBilling{Model: "video3.5", VideoDurationSeconds: 30, VideoResolution: "720p"}}
	var wg sync.WaitGroup
	var created atomic.Int32
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := repo.Reserve(ctx, &task)
			if err != nil {
				t.Error(err)
			}
			if ok {
				created.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int32(1), created.Load())
	task.TaskID = "accepted-task"
	task.ResponseStatus = 200
	task.ResponseBody = []byte(`{"id":"accepted-task","status":"queued"}`)
	require.NoError(t, repo.Finish(ctx, &task))
	require.NoError(t, db.Close())
	// A new connection and repository have no process memory or Redis state.
	db, err = sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer db.Close()
	repo = NewVideoTaskRepository(db)
	recovered, err := repo.GetTask(ctx, "accepted-task", 10, 20)
	require.NoError(t, err)
	require.Equal(t, 30, recovered.Pending.VideoDurationSeconds)
	require.Equal(t, task.ResponseBody, recovered.ResponseBody)
	for _, owner := range [][2]int64{{11, 20}, {10, 21}} {
		other, err := repo.GetTask(ctx, "accepted-task", owner[0], owner[1])
		require.NoError(t, err)
		require.Nil(t, other)
		other, err = repo.GetOperation(ctx, task.OperationKey, owner[0], owner[1])
		require.NoError(t, err)
		require.Nil(t, other)
	}
	wrongOwner := task
	wrongOwner.APIKeyID = 21
	require.Error(t, repo.Finish(ctx, &wrongOwner))
}
