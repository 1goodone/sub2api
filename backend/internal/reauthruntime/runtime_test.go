package reauthruntime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func archive(t *testing.T, entries []*tar.Header, content []string) []byte {
	t.Helper()
	var data bytes.Buffer
	gz := gzip.NewWriter(&data)
	tw := tar.NewWriter(gz)
	for i, h := range entries {
		require.NoError(t, tw.WriteHeader(h))
		if h.Typeflag == tar.TypeReg {
			_, err := tw.Write([]byte(content[i]))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return data.Bytes()
}

func TestExtractRejectsEscapesAndLinks(t *testing.T) {
	for _, h := range []*tar.Header{
		{Name: "../escape", Typeflag: tar.TypeReg, Size: 1},
		{Name: "/escape", Typeflag: tar.TypeReg, Size: 1},
		{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../escape"},
		{Name: "device", Typeflag: tar.TypeChar},
	} {
		t.Run(h.Name, func(t *testing.T) {
			require.Error(t, extract(archive(t, []*tar.Header{h}, []string{"x"}), t.TempDir()))
		})
	}
}

func TestExtractPreservesExecutableAndRejectsDuplicate(t *testing.T) {
	root := t.TempDir()
	h := &tar.Header{Name: "bin/run", Typeflag: tar.TypeReg, Size: 4, Mode: 0755}
	require.NoError(t, extract(archive(t, []*tar.Header{h}, []string{"test"}), root))
	info, err := os.Stat(filepath.Join(root, "bin/run"))
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0700), info.Mode().Perm())
	}
	require.Error(t, extract(archive(t, []*tar.Header{h}, []string{"test"}), root))
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestReleaseRepositoryConfiguration(t *testing.T) {
	t.Setenv("OPENAI_REAUTH_RUNTIME_REPOSITORY", "")
	require.Equal(t, defaultReleaseRepository, New(t.TempDir(), "1.2.3", "http://127.0.0.1:4040", "token").repository)

	t.Setenv("OPENAI_REAUTH_RUNTIME_REPOSITORY", "owner/runtime-repo")
	require.Equal(t, "owner/runtime-repo", New(t.TempDir(), "1.2.3", "http://127.0.0.1:4040", "token").repository)
}

func TestInvalidReleaseRepositoryIsUnavailable(t *testing.T) {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		t.Skip("managed runtime platform")
	}
	t.Setenv("OPENAI_REAUTH_RUNTIME_REPOSITORY", "invalid")
	m := New(t.TempDir(), "1.2.3", "http://127.0.0.1:4040", "token")
	m.Ensure()
	require.Equal(t, "release_repository_invalid", m.Status().Reason)
}

func TestPrepareVerifiesDigestBeforeExecutingAndReusesCache(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux runtime")
	}
	for _, valid := range []bool{false, true} {
		t.Run(map[bool]string{true: "valid", false: "tampered"}[valid], func(t *testing.T) {
			t.Setenv("OPENAI_REAUTH_RUNTIME_REPOSITORY", "owner/runtime-repo")
			root := t.TempDir()
			script := "#!/bin/sh\nexit 0\n"
			data := archive(t, []*tar.Header{{Name: "python", Typeflag: tar.TypeReg, Size: int64(len(script)), Mode: 0755}}, []string{script})
			sum := sha256.Sum256(data)
			digest := hex.EncodeToString(sum[:])
			if !valid {
				digest = strings.Repeat("0", 64)
			}
			requests := 0
			m := New(root, "1.2.3", "http://127.0.0.1:4040", strings.Repeat("x", 64))
			m.client = &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				require.NotContains(t, req.URL.String(), m.token)
				require.Contains(t, req.URL.String(), "owner/runtime-repo/releases/")
				body := data
				if strings.Contains(req.URL.Path, "/releases/tags/") {
					body, _ = json.Marshal(map[string]any{"assets": []map[string]string{{"name": "sub2api-reauth_1.2.3_linux_" + runtime.GOARCH + ".tar.gz", "digest": "sha256:" + digest}}})
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}}, nil
			})}
			dir, err := m.prepare(context.Background())
			if !valid {
				require.ErrorContains(t, err, "checksum")
				return
			}
			require.NoError(t, err)
			require.FileExists(t, filepath.Join(dir, "ready"))
			_, err = m.prepare(context.Background())
			require.NoError(t, err)
			require.Equal(t, 2, requests)
		})
	}
}

func TestStopCancelsPreparationAndCannotRestart(t *testing.T) {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		t.Skip("managed runtime platform")
	}
	m := New(t.TempDir(), "1.2.3", "http://127.0.0.1:4040", strings.Repeat("x", 64))
	entered := make(chan struct{})
	m.client = &http.Client{Transport: transportFunc(func(req *http.Request) (*http.Response, error) {
		close(entered)
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
	m.Ensure()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("preparation did not start")
	}
	m.Stop()
	require.Equal(t, "unavailable", m.Status().State)
	m.Ensure()
	require.False(t, m.started)
}
