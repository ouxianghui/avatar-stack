package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"avatar-stack/session-api/internal/model"
	"github.com/redis/go-redis/v9"
)

// RedisStore is the default Store implementation.
// It stores session records as JSON with a TTL.
type RedisStore struct {
	client     *redis.Client
	keyPrefix  string
	sessionTTL time.Duration
}

// NewRedisStore initializes redis client but does not perform network I/O yet.
func NewRedisStore(redisURL, keyPrefix string, sessionTTL time.Duration) (*RedisStore, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	client := redis.NewClient(opts)
	return &RedisStore{
		client:     client,
		keyPrefix:  keyPrefix,
		sessionTTL: sessionTTL,
	}, nil
}

// Close closes underlying Redis connections.
func (s *RedisStore) Close() error {
	return s.client.Close()
}

// Ping is used by health/readiness endpoints.
func (s *RedisStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

// PutSession writes one session snapshot with TTL.
// The service layer always writes full snapshot, not partial updates.
func (s *RedisStore) PutSession(ctx context.Context, session *model.Session) error {
	raw, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	key := s.sessionKey(session.SessionID)
	if err := s.client.Set(ctx, key, raw, s.sessionTTL).Err(); err != nil {
		return fmt.Errorf("redis set session: %w", err)
	}
	return nil
}

// GetSession reads and decodes one session snapshot.
func (s *RedisStore) GetSession(ctx context.Context, sessionID string) (*model.Session, error) {
	raw, err := s.client.Get(ctx, s.sessionKey(sessionID)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("redis get session: %w", err)
	}

	var session model.Session
	if err := json.Unmarshal(raw, &session); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}

	return &session, nil
}

// DeleteSession removes one session key if present.
func (s *RedisStore) DeleteSession(ctx context.Context, sessionID string) error {
	if err := s.client.Del(ctx, s.sessionKey(sessionID)).Err(); err != nil {
		return fmt.Errorf("redis del session: %w", err)
	}
	return nil
}

// sessionKey builds one Redis key for session snapshot.
func (s *RedisStore) sessionKey(sessionID string) string {
	return s.keyPrefix + sessionID
}
