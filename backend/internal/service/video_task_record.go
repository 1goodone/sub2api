package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// VideoTaskRecord stores ownership and receipts, never API keys or reference images.
type VideoTaskRecord struct {
	OperationKey                         string
	UserID, APIKeyID, GroupID, AccountID int64
	RequestHash, TaskID                  string
	ResponseStatus                       int
	ResponseBody                         []byte
	Pending                              GrokVideoPendingBilling
}

type VideoTaskRepository interface {
	Reserve(context.Context, *VideoTaskRecord) (bool, error)
	Finish(context.Context, *VideoTaskRecord) error
	GetOperation(context.Context, string, int64, int64) (*VideoTaskRecord, error)
	GetTask(context.Context, string, int64, int64) (*VideoTaskRecord, error)
}

func (s *OpenAIGatewayService) SetVideoTaskRepository(repo VideoTaskRepository) {
	s.videoTaskRepo = repo
}

type videoTaskOwner struct {
	UserID, APIKeyID, GroupID    int64
	Model, CreatedAt, ClientHash string
}
type videoTaskOwnerKey struct{}

func WithVideoTaskOwner(ctx context.Context, userID, apiKeyID int64, groupID *int64, model, createdAt string, body []byte) context.Context {
	return context.WithValue(ctx, videoTaskOwnerKey{}, videoTaskOwner{userID, apiKeyID, derefGroupID(groupID), model, createdAt, fmt.Sprintf("%x", sha256.Sum256(body))})
}

func videoOperationKey(clientKey string, userID, apiKeyID int64) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%s", userID, apiKeyID, clientKey))))
}

func (s *OpenAIGatewayService) GetVideoOperation(ctx context.Context, clientKey string, userID, apiKeyID int64) (*VideoTaskRecord, error) {
	if s.videoTaskRepo == nil {
		return nil, nil
	}
	return s.videoTaskRepo.GetOperation(ctx, videoOperationKey(clientKey, userID, apiKeyID), userID, apiKeyID)
}

func (s *OpenAIGatewayService) reserveVideoOperation(ctx context.Context, clientKey string, account *Account, targetURL, contentType, token string, body []byte, pending GrokVideoPendingBilling) (*VideoTaskRecord, bool, error) {
	owner, ok := ctx.Value(videoTaskOwnerKey{}).(videoTaskOwner)
	if s.videoTaskRepo == nil || !ok {
		return nil, true, nil
	}
	if clientKey == "" {
		clientKey = uuid.NewString()
	}
	if len(clientKey) > 255 || strings.IndexFunc(clientKey, func(r rune) bool { return r < 33 || r > 126 }) >= 0 {
		return nil, false, fmt.Errorf("Idempotency-Key must contain at most 255 ASCII characters without whitespace")
	}
	pending.Model, pending.OriginalModel, pending.CreatedAt = owner.Model, owner.Model, owner.CreatedAt
	if pending.CreatedAt == "" {
		pending.CreatedAt = GrokVideoPendingCreatedAtNow()
	}
	hash := sha256.New()
	fmt.Fprintf(hash, "%d\x00%s\x00%s\x00%s\x00%s\x00", account.ID, targetURL, contentType, token, owner.ClientHash)
	_, _ = hash.Write(body)
	record := &VideoTaskRecord{
		OperationKey: videoOperationKey(clientKey, owner.UserID, owner.APIKeyID),
		UserID:       owner.UserID, APIKeyID: owner.APIKeyID, GroupID: owner.GroupID, AccountID: account.ID,
		RequestHash: fmt.Sprintf("%x", hash.Sum(nil)), Pending: pending,
	}
	created, err := s.videoTaskRepo.Reserve(ctx, record)
	if err != nil || created {
		return record, created, err
	}
	existing, err := s.videoTaskRepo.GetOperation(ctx, record.OperationKey, owner.UserID, owner.APIKeyID)
	if err != nil {
		return nil, false, err
	}
	if existing == nil || existing.GroupID != owner.GroupID || existing.AccountID != record.AccountID || existing.RequestHash != record.RequestHash {
		return nil, false, fmt.Errorf("idempotency_conflict: the operation is already bound to a different request, account or API key")
	}
	return existing, false, nil
}
