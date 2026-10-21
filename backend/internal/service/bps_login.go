package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	bpsLoginAccountLimit   = 50
	bpsLoginProxyPoolLimit = 10
	bpsLoginProxyAttempts  = 3
)

type BPSLoginAccountResult struct {
	AccountID int64  `json:"account_id"`
	TaskID    int64  `json:"task_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

type BPSLoginResult struct {
	Results []BPSLoginAccountResult `json:"results"`
}

type excelTaskProxyCreator interface {
	CreateExcelTaskWithProxy(context.Context, int64, string, string) (*OpenAIOAuthReauthTaskRecord, error)
}

type excelTaskProxyPoolCreator interface {
	CreateExcelTaskWithProxyPool(context.Context, int64, string, string, string, int) (*OpenAIOAuthReauthTaskRecord, error)
}

type reauthTaskProxyReader interface {
	GetTaskProxyCiphertext(context.Context, int64) (string, error)
}

type reauthTaskProxyRotationReader interface {
	GetTaskProxyRotation(context.Context, int64) (string, int, error)
}

func (s *OpenAIOAuthReauthService) StartBPSLogins(ctx context.Context, accountIDs []int64, proxies []string) (*BPSLoginResult, error) {
	pool, err := NormalizeBPSLoginProxyPool(proxies)
	if err != nil {
		if infraerrors.Code(err) != 0 {
			return nil, err
		}
		return nil, infraerrors.BadRequest("OPENAI_BPS_PROXY_INVALID", "each proxy must look like username:password@hostname:port")
	}
	if err = s.ensureReady(); err != nil {
		return nil, err
	}
	if err = s.ensureDurableEncryption(); err != nil {
		return nil, err
	}
	ids := uniqueBPSAccountIDs(accountIDs)
	if len(ids) == 0 {
		return nil, infraerrors.BadRequest("OPENAI_BPS_ACCOUNTS_REQUIRED", "select at least one account")
	}
	if len(ids) > bpsLoginAccountLimit {
		return nil, infraerrors.BadRequest("OPENAI_BPS_ACCOUNTS_LIMIT", "select at most 50 accounts")
	}
	result := &BPSLoginResult{Results: make([]BPSLoginAccountResult, 0, len(ids))}
	for index, id := range ids {
		current, rest := bpsProxyRotation(pool, index)
		currentCipher, cipherErr := s.encryptor.Encrypt(current)
		if cipherErr != nil {
			return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_BPS_PROXY_ENCRYPT_FAILED", "failed to protect the login proxy")
		}
		poolCipher := ""
		if len(rest) > 0 {
			raw, _ := json.Marshal(rest)
			poolCipher, cipherErr = s.encryptor.Encrypt(string(raw))
			if cipherErr != nil {
				return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_BPS_PROXY_ENCRYPT_FAILED", "failed to protect the login proxy")
			}
		}
		task, taskErr := s.createProfileTaskWithPool(ctx, id, "excel", currentCipher, poolCipher, 1)
		item := BPSLoginAccountResult{AccountID: id}
		if taskErr != nil {
			item.Error = infraerrors.Message(taskErr)
		} else if task != nil {
			item.TaskID = task.ID
		}
		result.Results = append(result.Results, item)
	}
	return result, nil
}

func NormalizeBPSLoginProxyPool(lines []string) ([]string, error) {
	out := make([]string, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		for _, part := range strings.Split(line, "\n") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			normalized, err := NormalizeBPSLoginProxy(part)
			if err != nil {
				return nil, err
			}
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			out = append(out, normalized)
			if len(out) > bpsLoginProxyPoolLimit {
				return nil, errBPSProxyPoolLimit
			}
		}
	}
	if len(out) == 0 {
		return nil, errBPSProxyRequired
	}
	return out, nil
}

var (
	errBPSProxyRequired  = infraerrors.BadRequest("OPENAI_BPS_PROXY_INVALID", "enter at least one proxy")
	errBPSProxyPoolLimit = infraerrors.BadRequest("OPENAI_BPS_PROXY_LIMIT", "enter at most 10 proxies")
)

func bpsProxyRotation(pool []string, start int) (string, []string) {
	count := len(pool)
	if count == 0 {
		return "", nil
	}
	if start < 0 {
		start = 0
	}
	index := start % count
	limit := bpsLoginProxyAttempts - 1
	if limit > count-1 {
		limit = count - 1
	}
	rest := make([]string, 0, limit)
	for step := 1; step <= limit; step++ {
		rest = append(rest, pool[(index+step)%count])
	}
	return pool[index], rest
}

func (s *OpenAIOAuthReauthService) rotateBPSLoginProxy(ctx context.Context, record *OpenAIOAuthReauthTaskRecord, reason string) bool {
	code := excelLoginFailureCode(reason)
	if record == nil || (code != "warmup_failed" && code != "security_check") {
		return false
	}
	reader, ok := s.repo.(reauthTaskProxyRotationReader)
	creator, creatorOK := s.repo.(excelTaskProxyPoolCreator)
	if !ok || !creatorOK || s.encryptor == nil {
		return false
	}
	poolCipher, attempt, err := reader.GetTaskProxyRotation(ctx, record.ID)
	if err != nil || poolCipher == "" || attempt < 1 || attempt >= bpsLoginProxyAttempts {
		if err != nil {
			slog.Warn("excel_bps_login_proxy_rotate", "account_id", record.AccountID, "task_id", record.ID)
		}
		return false
	}
	plain, err := s.decryptReauthSecret(poolCipher, "bps login proxy pool cannot be decrypted")
	if err != nil {
		slog.Warn("excel_bps_login_proxy_rotate", "account_id", record.AccountID, "task_id", record.ID)
		return false
	}
	var rest []string
	if err = json.Unmarshal([]byte(plain), &rest); err != nil || len(rest) == 0 {
		return false
	}
	nextCipher, err := s.encryptor.Encrypt(rest[0])
	if err != nil {
		return false
	}
	nextPool := ""
	if len(rest) > 1 {
		raw, _ := json.Marshal(rest[1:])
		nextPool, err = s.encryptor.Encrypt(string(raw))
		if err != nil {
			return false
		}
	}
	if _, err = creator.CreateExcelTaskWithProxyPool(ctx, record.AccountID, record.ExpectedCredentialsHash, nextCipher, nextPool, attempt+1); err != nil {
		slog.Warn("excel_bps_login_proxy_rotate", "account_id", record.AccountID, "task_id", record.ID)
		return false
	}
	return true
}

func uniqueBPSAccountIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (s *OpenAIOAuthReauthService) oneShotReauthProxy(ctx context.Context, record *OpenAIOAuthReauthTaskRecord) (string, error) {
	if record == nil || record.OAuthProfile != "excel" {
		return "", nil
	}
	reader, ok := s.repo.(reauthTaskProxyReader)
	if !ok {
		return "", nil
	}
	ciphertext, err := reader.GetTaskProxyCiphertext(ctx, record.ID)
	if err != nil || ciphertext == "" {
		return "", err
	}
	return s.decryptReauthSecret(ciphertext, "one-shot login proxy cannot be decrypted")
}
