//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type videoRecordMemory struct {
	mu    sync.Mutex
	tasks map[string]VideoTaskRecord
	fail  bool
}

func (m *videoRecordMemory) Reserve(_ context.Context, r *VideoTaskRecord) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return false, errors.New("database unavailable")
	}
	if m.tasks == nil {
		m.tasks = map[string]VideoTaskRecord{}
	}
	if _, ok := m.tasks[r.OperationKey]; ok {
		return false, nil
	}
	m.tasks[r.OperationKey] = *r
	return true, nil
}
func (m *videoRecordMemory) Finish(ctx context.Context, r *VideoTaskRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks[r.OperationKey] = *r
	return nil
}
func (m *videoRecordMemory) GetOperation(_ context.Context, key string, userID, apiKeyID int64) (*VideoTaskRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.tasks[key]
	if !ok || r.UserID != userID || r.APIKeyID != apiKeyID {
		return nil, nil
	}
	return &r, nil
}
func (m *videoRecordMemory) GetTask(_ context.Context, id string, userID, apiKeyID int64) (*VideoTaskRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.tasks {
		if r.TaskID == id && r.UserID == userID && r.APIKeyID == apiKeyID {
			return &r, nil
		}
	}
	return nil, nil
}

type videoRecordUpstream struct {
	HTTPUpstream
	calls     atomic.Int32
	afterSend func()
	fail      bool
	request   *http.Request
}

func (u *videoRecordUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.calls.Add(1)
	u.request = req
	if u.afterSend != nil {
		u.afterSend()
	}
	if u.fail {
		return nil, errors.New("response lost")
	}
	return openAIVideoResponse(200, "application/json", `{"id":"durable-task","status":"queued"}`), nil
}

func TestVideoRecordDurableReceiptSurvivesClosedClientAndMissingRedis(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	repo := &videoRecordMemory{}
	u := &videoRecordUpstream{afterSend: cancel}
	s := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: u, videoTaskRepo: repo}
	group := int64(53)
	body := []byte(`{"model":"video3.0","prompt":"  keep\nspaces ","duration":10,"resolution":"720p"}`)
	ctx = WithVideoTaskOwner(ctx, 10, 20, &group, "video3.0", "", body)
	c, w := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
	c.Request.Header.Set("Idempotency-Key", "operation-one")
	result, err := s.ForwardOpenAIVideo(ctx, c, openAIVideoTestAccount(), GrokMediaEndpointVideosGenerations, "", body, "application/json", "3.0")
	require.NoError(t, err)
	require.Equal(t, 200, w.Code)
	require.Equal(t, "durable-task", result.ResponseID)
	// Completion was durably recorded even after the downstream context closed.
	r, err := s.GetVideoOperation(context.Background(), "operation-one", 10, 20)
	require.NoError(t, err)
	require.NotNil(t, r)
	require.Equal(t, "durable-task", r.TaskID)
	require.Equal(t, videoOperationKey("operation-one", 10, 20), u.request.Header.Get("Idempotency-Key"))
	owner, err := s.ResolveGrokMediaVideoRequestAccount(context.Background(), &group, r.TaskID, 10, 20)
	require.NoError(t, err)
	require.Equal(t, int64(91), owner)
	pending, err := s.LoadGrokVideoPendingBilling(context.Background(), r.TaskID, 10, 20)
	require.NoError(t, err)
	require.Equal(t, 10, pending.VideoDurationSeconds)
	require.Equal(t, "video3.0", pending.Model)
	foreign, err := s.GetVideoOperation(context.Background(), "operation-one", 10, 21)
	require.NoError(t, err)
	require.Nil(t, foreign)
	wrongGroup := int64(54)
	owner, err = s.ResolveGrokMediaVideoRequestAccount(context.Background(), &wrongGroup, r.TaskID, 10, 20)
	require.NoError(t, err)
	require.Zero(t, owner)
}

func TestVideoRecordConcurrentReplayAndConflictNeverCreateAgain(t *testing.T) {
	repo := &videoRecordMemory{}
	u := &videoRecordUpstream{}
	s := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: u, videoTaskRepo: repo}
	group := int64(53)
	body := []byte(`{"model":"video3.0","prompt":"test","duration":5}`)
	ctx := WithVideoTaskOwner(context.Background(), 10, 20, &group, "video3.0", "", body)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, w := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
			c.Request.Header.Set("Idempotency-Key", "same-operation")
			_, _ = s.ForwardOpenAIVideo(ctx, c, openAIVideoTestAccount(), GrokMediaEndpointVideosGenerations, "", body, "application/json", "3.0")
			if w.Code != 200 && w.Code != 409 {
				t.Errorf("unexpected response %d", w.Code)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int32(1), u.calls.Load())
	c, w := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
	c.Request.Header.Set("Idempotency-Key", "same-operation")
	_, err := s.ForwardOpenAIVideo(ctx, c, openAIVideoTestAccount(), GrokMediaEndpointVideosGenerations, "", append(body, ' '), "application/json", "3.0")
	require.Error(t, err)
	require.Equal(t, 409, w.Code)
	require.Equal(t, int32(1), u.calls.Load())
}

func TestVideoRecordAmbiguousResponseAndStorageFailureDoNotResubmit(t *testing.T) {
	for _, storageFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "response-lost", true: "database-down"}[storageFailure], func(t *testing.T) {
			repo := &videoRecordMemory{fail: storageFailure}
			u := &videoRecordUpstream{fail: true}
			s := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: u, videoTaskRepo: repo}
			body := []byte(`{"model":"video3.0","prompt":"test"}`)
			ctx := WithVideoTaskOwner(context.Background(), 10, 20, nil, "video3.0", "", body)
			for range 2 {
				c, _ := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
				c.Request.Header.Set("Idempotency-Key", "ambiguous")
				_, err := s.ForwardOpenAIVideo(ctx, c, openAIVideoTestAccount(), GrokMediaEndpointVideosGenerations, "", body, "application/json", "3.0")
				require.Error(t, err)
			}
			want := int32(1)
			if storageFailure {
				want = 0
			}
			require.Equal(t, want, u.calls.Load())
		})
	}
}

func TestOpenAIVideoOriginalDownloadHeadersAndHead(t *testing.T) {
	for _, path := range []string{"/v1/videos/task/content?download=1", "/v1/videos/task/download"} {
		c, w := grokMediaContentTestContext(http.MethodHead, "https://api.example"+path, nil)
		c.Request.Header.Set("Range", "bytes=0-11")
		c.Request.Header.Set("If-Range", `"etag"`)
		response := openAIVideoResponse(206, "video/mp4", "")
		response.Header.Set("X-Video-Quality", "original")
		response.Header.Set("ETag", `"etag"`)
		u := &grokMediaContentUpstreamStub{response: response}
		s := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: u}
		_, err := s.ForwardOpenAIVideo(context.Background(), c, openAIVideoTestAccount(), GrokMediaEndpointVideoContent, "task", nil, "", "")
		require.NoError(t, err)
		require.Equal(t, http.MethodHead, u.request.Method)
		require.Equal(t, "1", u.request.URL.Query().Get("download"))
		require.Equal(t, "bytes=0-11", u.request.Header.Get("Range"))
		require.Equal(t, `"etag"`, u.request.Header.Get("If-Range"))
		require.Equal(t, "original", w.Header().Get("X-Video-Quality"))
		require.Equal(t, `"etag"`, w.Header().Get("ETag"))
	}
	c, _ := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
	c.Request.Header.Set("Idempotency-Key", "pass-through")
	s := &OpenAIGatewayService{}
	req, err := s.buildOpenAIVideoRequest(context.Background(), c, openAIVideoTestAccount(), GrokMediaEndpointVideosGenerations, "https://upstream.example/v1/videos", []byte("  exact bytes\n"), "application/json", "upstream-key")
	require.NoError(t, err)
	forwarded, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.Equal(t, "  exact bytes\n", string(forwarded))
	require.Equal(t, "pass-through", req.Header.Get("Idempotency-Key"))
	require.False(t, strings.Contains(req.URL.String(), "upstream-key"))
}

func TestOpenAIVideoTransportNeverReplaysLostPost(t *testing.T) {
	var posts atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte("warm connection"))
			return
		}
		posts.Add(1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer upstream.Close()
	client := upstream.Client()
	resp, err := client.Get(upstream.URL)
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	c, _ := grokMediaContentTestContext(http.MethodPost, upstream.URL+"/v1/videos", nil)
	c.Request.Header.Set("Idempotency-Key", "lost-post")
	s := &OpenAIGatewayService{}
	req, err := s.buildOpenAIVideoRequest(context.Background(), c, openAIVideoTestAccount(), GrokMediaEndpointVideosGenerations, upstream.URL+"/v1/videos", []byte(`{"model":"video3.0","prompt":"test"}`), "application/json", "test-only")
	require.NoError(t, err)
	_, err = client.Do(req)
	require.Error(t, err)
	require.Equal(t, int32(1), posts.Load(), "transport replayed a potentially accepted video submission")
}

func TestOpenAIVideoTaskIDDoesNotUseRequestTraceID(t *testing.T) {
	body := []byte(`{"id":"task-one","task_id":"task-one","request_id":"trace-one","status":"completed","url":"/video"}`)
	require.Equal(t, "task-one", extractOpenAIVideoTaskID(body))
	meta := openAIVideoUsageFromResponse(GrokMediaEndpointVideoStatus, GrokMediaRequestInfo{}, body, "task-one")
	require.Equal(t, "task-one", meta.ResponseID)
}
