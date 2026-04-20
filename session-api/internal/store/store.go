package store

import (
	"context"
	"errors"

	"avatar-stack/session-api/internal/model"
)

var ErrNotFound = errors.New("not found")

// Store abstracts persistence.
// Current implementation is Redis, but service layer only depends on this interface.
type Store interface {
	// Ping validates backend availability for health checks.
	Ping(ctx context.Context) error
	// PutSession upserts one session record.
	PutSession(ctx context.Context, session *model.Session) error
	// GetSession fetches one session by ID.
	GetSession(ctx context.Context, sessionID string) (*model.Session, error)
	// DeleteSession removes one session record (revokes tokens).
	DeleteSession(ctx context.Context, sessionID string) error
	// Close releases backend resources.
	Close() error
}
