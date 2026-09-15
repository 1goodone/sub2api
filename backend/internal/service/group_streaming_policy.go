package service

import (
	"context"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrNonStreamingDisabled = infraerrors.Forbidden(
	"NON_STREAMING_DISABLED",
	"This group only allows streaming text generation. Set stream to true.",
)

type nonStreamingTextRequestKey struct{}

// Preserve the client mode before protocol bridges force upstream streaming.
func WithNonStreamingTextRequest(ctx context.Context) context.Context {
	return context.WithValue(ctx, nonStreamingTextRequestKey{}, true)
}

func CheckGroupStreamingPolicy(ctx context.Context, group *Group) error {
	if group != nil && group.DisableNonStreaming {
		if nonStreaming, _ := ctx.Value(nonStreamingTextRequestKey{}).(bool); nonStreaming {
			return ErrNonStreamingDisabled
		}
	}
	return nil
}
