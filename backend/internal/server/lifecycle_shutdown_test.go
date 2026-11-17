package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLifecycleShutdownWithdrawsReadinessAndFinishesActiveRequest(t *testing.T) {
	lifecycle := NewLifecycle()
	stopped := make(chan struct{})
	lifecycle.onShutdown = append(lifecycle.onShutdown, func() { close(stopped) })
	started, release := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(lifecycle.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "completed once")
	})))
	defer srv.Close()
	var releaseOnce sync.Once
	releaseRequest := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseRequest()
	response := make(chan string, 1)
	go func() {
		res, err := srv.Client().Get(srv.URL + "/work")
		if err != nil {
			response <- err.Error()
			return
		}
		defer func() { _ = res.Body.Close() }()
		body, err := io.ReadAll(res.Body)
		if err != nil {
			response <- err.Error()
			return
		}
		response <- string(body)
	}()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- lifecycle.Shutdown(ctx, srv.Config, 500*time.Millisecond) }()
	require.Eventually(t, lifecycle.isDraining, time.Second, time.Millisecond)
	for _, path := range []string{"/readyz", "/new-work"} {
		res, err := srv.Client().Get(srv.URL + path)
		require.NoError(t, err)
		require.Equal(t, http.StatusServiceUnavailable, res.StatusCode)
		_, err = io.Copy(io.Discard, res.Body)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())
	}
	select {
	case <-stopped:
		t.Fatal("dependencies stopped before the active request finished")
	default:
	}
	releaseRequest()
	require.Equal(t, "completed once", <-response)
	require.NoError(t, <-done)
	select {
	case <-stopped:
	default:
		t.Fatal("dependencies were not stopped after draining")
	}
}

func TestLifecycleShutdownWaitsForHijackedHandler(t *testing.T) {
	lifecycle := NewLifecycle()
	started, release := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(lifecycle.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error(err)
			close(started)
			return
		}
		defer func() { _ = conn.Close() }()
		close(started)
		<-release
	})))
	defer srv.Close()
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		res, _ := srv.Client().Get(srv.URL)
		if res != nil {
			_ = res.Body.Close()
		}
	}()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := lifecycle.Shutdown(ctx, srv.Config, 0)
	close(release)
	<-requestDone
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
