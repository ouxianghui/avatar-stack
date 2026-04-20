package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"avatar-stack/session-api/internal/config"
	"avatar-stack/session-api/internal/model"
	"avatar-stack/session-api/internal/store"
	"github.com/google/uuid"
)

var ErrUnauthorized = errors.New("unauthorized")

// SessionService owns business orchestration:
// - session lifecycle
// - MediaMTX callback state transitions
// - MediaMTX HTTP auth (short-lived opaque tokens as passwords)
type SessionService struct {
	cfg   config.Config
	store store.Store
	log   *slog.Logger
}

// NewSessionService builds the use-case service.
func NewSessionService(cfg config.Config, st store.Store, log *slog.Logger) *SessionService {
	return &SessionService{
		cfg:   cfg,
		store: st,
		log:   log,
	}
}

// Health delegates backend reachability check to store.
func (s *SessionService) Health(ctx context.Context) error {
	return s.store.Ping(ctx)
}

// CreateSession creates a new session snapshot for WHIP/WHEP routing.
// Plaintext tokens are returned only in this response; they are stored as bcrypt hashes in Redis.
func (s *SessionService) CreateSession(ctx context.Context, req model.CreateSessionRequest) (*model.SessionPayload, error) {
	sessionID := uuid.NewString()
	now := time.Now().UTC()

	pubPlain, err := generateMediaToken()
	if err != nil {
		return nil, fmt.Errorf("publish token: %w", err)
	}
	playPlain, err := generateMediaToken()
	if err != nil {
		return nil, fmt.Errorf("playback token: %w", err)
	}

	pubHash, err := hashMediaToken(pubPlain)
	if err != nil {
		return nil, fmt.Errorf("hash publish token: %w", err)
	}
	playHash, err := hashMediaToken(playPlain)
	if err != nil {
		return nil, fmt.Errorf("hash playback token: %w", err)
	}

	session := &model.Session{
		SessionID:         sessionID,
		AvatarID:          normalizeAvatarID(req.AvatarID),
		Status:            model.StatusWaitingInput,
		CreatedAt:         now,
		UpdatedAt:         now,
		InputReady:        false,
		OutputReady:       false,
		ViewerCount:       0,
		PublishTokenHash:  pubHash,
		PlaybackTokenHash: playHash,
	}

	if err := s.store.PutSession(ctx, session); err != nil {
		return nil, fmt.Errorf("put session: %w", err)
	}

	s.log.Info("created session", "session_id", sessionID, "avatar_id", session.AvatarID)
	return s.toPayloadIssue(session, pubPlain, playPlain), nil
}

// GetSession returns current session snapshot (passwords are never echoed).
func (s *SessionService) GetSession(ctx context.Context, sessionID string) (*model.SessionPayload, error) {
	session, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return s.toPayloadMasked(session), nil
}

// StopSession revokes credentials by deleting the session record.
func (s *SessionService) StopSession(ctx context.Context, sessionID string) (*model.SessionPayload, error) {
	session, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	session.Status = model.StatusStopped
	session.UpdatedAt = time.Now().UTC()
	out := s.toPayloadMasked(session)

	if err := s.store.DeleteSession(ctx, sessionID); err != nil {
		return nil, fmt.Errorf("delete session: %w", err)
	}

	s.log.Info("stopped session", "session_id", sessionID)
	return out, nil
}

// HandleMediaHook applies state transitions based on MediaMTX callbacks.
// Unknown paths/events are ignored to keep callback handling idempotent and tolerant.
func (s *SessionService) HandleMediaHook(ctx context.Context, eventName, rawPath string) error {
	sessionID, ok := model.ParseSessionPath(rawPath)
	if !ok {
		return nil
	}

	session, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return err
	}

	now := time.Now().UTC()
	switch eventName {
	case "on-ready":
		session.InputReady = true
		session.OutputReady = true
		session.Status = model.StatusOutputLive
	case "on-not-ready":
		session.InputReady = false
		session.OutputReady = false
		if session.Status != model.StatusStopping && session.Status != model.StatusStopped {
			session.Status = model.StatusWaitingInput
		}
	case "on-read":
		session.ViewerCount++
	case "on-unread":
		if session.ViewerCount > 0 {
			session.ViewerCount--
		}
	default:
		return nil
	}

	session.UpdatedAt = now
	if err := s.store.PutSession(ctx, session); err != nil {
		return err
	}

	s.log.Debug("processed mediamtx hook", "event", eventName, "path", model.NormalizePath(rawPath), "session_id", sessionID)
	return nil
}

// Authorize validates MediaMTX HTTP auth: path, action, user, and bcrypt token (password field).
func (s *SessionService) Authorize(ctx context.Context, req model.MediaMTXAuthRequest) error {
	path := model.NormalizePath(req.Path)
	sessionID, ok := model.ParseSessionPath(path)
	if !ok {
		return ErrUnauthorized
	}

	action := strings.ToLower(strings.TrimSpace(req.Action))
	user := strings.TrimSpace(req.User)
	pass := strings.TrimSpace(req.Password)

	session, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrUnauthorized
		}
		return err
	}

	switch action {
	case "publish":
		if user != s.cfg.WhipUsername {
			return ErrUnauthorized
		}
		if session.PublishTokenHash == "" {
			return ErrUnauthorized
		}
		if err := checkMediaToken(session.PublishTokenHash, pass); err != nil {
			return ErrUnauthorized
		}
		return nil
	case "read", "playback":
		if user != s.cfg.WhepUsername {
			return ErrUnauthorized
		}
		if session.PlaybackTokenHash == "" {
			return ErrUnauthorized
		}
		if err := checkMediaToken(session.PlaybackTokenHash, pass); err != nil {
			return ErrUnauthorized
		}
		return nil
	default:
		return ErrUnauthorized
	}
}

func (s *SessionService) toPayloadIssue(session *model.Session, publishPlain, playbackPlain string) *model.SessionPayload {
	return &model.SessionPayload{
		SessionID:   session.SessionID,
		AvatarID:    session.AvatarID,
		Status:      session.Status,
		CreatedAt:   session.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   session.UpdatedAt.Format(time.RFC3339),
		InputReady:  session.InputReady,
		OutputReady: session.OutputReady,
		ViewerCount: session.ViewerCount,
		Publish: model.SessionPublish{
			WhipURL:  s.whipURL(session.SessionID),
			Username: s.cfg.WhipUsername,
			Password: publishPlain,
		},
		Playback: model.SessionPlayback{
			WhepURL:  s.whepURL(session.SessionID),
			Username: s.cfg.WhepUsername,
			Password: playbackPlain,
		},
	}
}

func (s *SessionService) toPayloadMasked(session *model.Session) *model.SessionPayload {
	return &model.SessionPayload{
		SessionID:   session.SessionID,
		AvatarID:    session.AvatarID,
		Status:      session.Status,
		CreatedAt:   session.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   session.UpdatedAt.Format(time.RFC3339),
		InputReady:  session.InputReady,
		OutputReady: session.OutputReady,
		ViewerCount: session.ViewerCount,
		Publish: model.SessionPublish{
			WhipURL:  s.whipURL(session.SessionID),
			Username: s.cfg.WhipUsername,
			Password: "",
		},
		Playback: model.SessionPlayback{
			WhepURL:  s.whepURL(session.SessionID),
			Username: s.cfg.WhepUsername,
			Password: "",
		},
	}
}

// URL builders keep URL composition logic centralized.
func (s *SessionService) whipURL(sessionID string) string {
	return fmt.Sprintf("%s/%s/whip", s.cfg.MediamtxWebRTCBaseURL, streamPath(sessionID))
}

func (s *SessionService) whepURL(sessionID string) string {
	return fmt.Sprintf("%s/%s/whep", s.cfg.MediamtxWebRTCBaseURL, streamPath(sessionID))
}

// normalizeAvatarID applies server-side defaults.
func normalizeAvatarID(avatarID string) string {
	value := strings.TrimSpace(avatarID)
	if value == "" {
		return "default-avatar"
	}
	return value
}

// Path helper defines canonical stream path (single path for publish + playback).
func streamPath(sessionID string) string {
	return fmt.Sprintf("avatar/%s/live", sessionID)
}
