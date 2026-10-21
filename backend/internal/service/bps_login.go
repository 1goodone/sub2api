package service

import (
	"context"
	"net/http"
	"sort"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const bpsLoginAccountLimit = 50

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

type reauthTaskProxyReader interface {
	GetTaskProxyCiphertext(context.Context, int64) (string, error)
}

func (s *OpenAIOAuthReauthService) StartBPSLogins(ctx context.Context, accountIDs []int64, proxy string) (*BPSLoginResult, error) {
	normalized, err := NormalizeBPSLoginProxy(proxy)
	if err != nil {
		return nil, infraerrors.BadRequest("OPENAI_BPS_PROXY_INVALID", "proxy must look like username:password@hostname:port")
	}
	if err = s.ensureReady(); err != nil {
		return nil, err
	}
	if err = s.ensureDurableEncryption(); err != nil {
		return nil, err
	}
	ciphertext, err := s.encryptor.Encrypt(normalized)
	if err != nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_BPS_PROXY_ENCRYPT_FAILED", "failed to protect the login proxy")
	}
	ids := uniqueBPSAccountIDs(accountIDs)
	if len(ids) == 0 {
		return nil, infraerrors.BadRequest("OPENAI_BPS_ACCOUNTS_REQUIRED", "select at least one account")
	}
	if len(ids) > bpsLoginAccountLimit {
		return nil, infraerrors.BadRequest("OPENAI_BPS_ACCOUNTS_LIMIT", "select at most 50 accounts")
	}
	result := &BPSLoginResult{Results: make([]BPSLoginAccountResult, 0, len(ids))}
	for _, id := range ids {
		task, taskErr := s.createProfileTask(ctx, id, "excel", ciphertext)
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
