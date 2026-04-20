package service

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"avatar-stack/session-api/internal/config"
	"avatar-stack/session-api/internal/model"
	"avatar-stack/session-api/internal/store"
)

type mapStore struct {
	byID map[string]*model.Session
}

func (m *mapStore) Ping(context.Context) error { return nil }

func (m *mapStore) PutSession(_ context.Context, s *model.Session) error {
	if m.byID == nil {
		m.byID = make(map[string]*model.Session)
	}
	m.byID[s.SessionID] = s
	return nil
}

func (m *mapStore) GetSession(_ context.Context, id string) (*model.Session, error) {
	s, ok := m.byID[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return s, nil
}

func (m *mapStore) DeleteSession(_ context.Context, id string) error {
	delete(m.byID, id)
	return nil
}

func (m *mapStore) Close() error { return nil }

func TestAuthorizePublishAndPlayback(t *testing.T) {
	pubPlain := "test-publish-token-xxxxxxxx"
	playPlain := "test-playback-token-yyyyyyyy"
	pubHash, err := hashMediaToken(pubPlain)
	if err != nil {
		t.Fatal(err)
	}
	playHash, err := hashMediaToken(playPlain)
	if err != nil {
		t.Fatal(err)
	}

	st := &mapStore{
		byID: map[string]*model.Session{
			"s1": {
				SessionID:         "s1",
				PublishTokenHash:  pubHash,
				PlaybackTokenHash: playHash,
			},
		},
	}

	cfg := config.Config{
		WhipUsername:          "publisher",
		WhepUsername:          "viewer",
		MediamtxWebRTCBaseURL: "http://localhost:8889",
		RequestTimeout:        2 * time.Second,
	}

	svc := NewSessionService(cfg, st, slog.Default())
	ctx := context.Background()

	t.Run("publish ok", func(t *testing.T) {
		err := svc.Authorize(ctx, model.MediaMTXAuthRequest{
			User: "publisher", Password: pubPlain, Action: "publish", Path: "avatar/s1/live",
		})
		if err != nil {
			t.Fatalf("Authorize: %v", err)
		}
	})

	t.Run("playback ok", func(t *testing.T) {
		err := svc.Authorize(ctx, model.MediaMTXAuthRequest{
			User: "viewer", Password: playPlain, Action: "read", Path: "avatar/s1/live",
		})
		if err != nil {
			t.Fatalf("Authorize: %v", err)
		}
	})

	t.Run("playback action alias", func(t *testing.T) {
		err := svc.Authorize(ctx, model.MediaMTXAuthRequest{
			User: "viewer", Password: playPlain, Action: "playback", Path: "avatar/s1/live",
		})
		if err != nil {
			t.Fatalf("Authorize: %v", err)
		}
	})

	t.Run("wrong publish password", func(t *testing.T) {
		err := svc.Authorize(ctx, model.MediaMTXAuthRequest{
			User: "publisher", Password: "nope", Action: "publish", Path: "avatar/s1/live",
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("wrong user", func(t *testing.T) {
		err := svc.Authorize(ctx, model.MediaMTXAuthRequest{
			User: "other", Password: pubPlain, Action: "publish", Path: "avatar/s1/live",
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
