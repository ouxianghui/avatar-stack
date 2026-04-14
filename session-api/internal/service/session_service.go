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
// - static credential authorization policy
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

// CreateSession creates a new session snapshot and enqueues worker start.
// This endpoint is intentionally synchronous and returns only orchestration metadata,
// not media readiness.
func (s *SessionService) CreateSession(ctx context.Context, req model.CreateSessionRequest) (*model.SessionPayload, error) {
	workerMode := normalizeWorkerMode(req.WorkerMode, s.cfg.DefaultWorkerMode)
	sessionID := uuid.NewString()
	now := time.Now().UTC()

	session := &model.Session{
		SessionID:   sessionID,
		AvatarID:    normalizeAvatarID(req.AvatarID),
		WorkerMode:  workerMode,
		Status:      model.StatusWaitingInput,
		CreatedAt:   now,
		UpdatedAt:   now,
		InputReady:  false,
		OutputReady: false,
		ViewerCount: 0,
	}

	if err := s.store.PutSession(ctx, session); err != nil {
		return nil, fmt.Errorf("put session: %w", err)
	}

	startPayload := model.StartQueueMessage{
		Action:           "start",
		SessionID:        sessionID,
		WorkerMode:       string(workerMode),
		WorkerInputRTSP:  s.workerInputRTSPURL(sessionID),
		WorkerOutputRTSP: s.workerOutputRTSPURL(sessionID),
	}
	if err := s.store.PublishStart(ctx, startPayload); err != nil {
		return nil, fmt.Errorf("publish start queue: %w", err)
	}

	s.log.Info("created session", "session_id", sessionID, "avatar_id", session.AvatarID, "worker_mode", workerMode)
	return s.toPayload(session), nil
}

// GetSession returns current session snapshot.
func (s *SessionService) GetSession(ctx context.Context, sessionID string) (*model.SessionPayload, error) {
	session, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return s.toPayload(session), nil
}

// StopSession marks session as stopping and enqueues worker stop command.
func (s *SessionService) StopSession(ctx context.Context, sessionID string) (*model.SessionPayload, error) {
	session, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	if session.Status != model.StatusStopped {
		session.Status = model.StatusStopping
		session.UpdatedAt = time.Now().UTC()
		if err := s.store.PutSession(ctx, session); err != nil {
			return nil, fmt.Errorf("update session status: %w", err)
		}
	}

	stopPayload := model.StopQueueMessage{
		Action:    "stop",
		SessionID: sessionID,
	}
	if err := s.store.PublishStop(ctx, stopPayload); err != nil {
		return nil, fmt.Errorf("publish stop queue: %w", err)
	}

	s.log.Info("stopping session", "session_id", sessionID)
	return s.toPayload(session), nil
}

// HandleMediaHook applies state transitions based on MediaMTX callbacks.
// Unknown paths/events are ignored to keep callback handling idempotent and tolerant.
func (s *SessionService) HandleMediaHook(ctx context.Context, eventName, rawPath string) error {
	sessionID, direction, ok := model.ParseSessionPath(rawPath)
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
		if direction == model.DirectionIn {
			session.InputReady = true
			session.Status = model.StatusInputLive
		}
		if direction == model.DirectionOut {
			session.OutputReady = true
			session.Status = model.StatusOutputLive
		}
	case "on-not-ready":
		if direction == model.DirectionIn {
			session.InputReady = false
			if session.Status != model.StatusStopping && session.Status != model.StatusStopped {
				session.Status = model.StatusWaitingInput
			}
		}
		if direction == model.DirectionOut {
			session.OutputReady = false
			if session.Status != model.StatusStopping && session.Status != model.StatusStopped && session.InputReady {
				session.Status = model.StatusProcessing
			}
		}
	case "on-read":
		if direction == model.DirectionOut {
			session.ViewerCount++
		}
	case "on-unread":
		if direction == model.DirectionOut && session.ViewerCount > 0 {
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

// Authorize validates static credentials + action/path combination.
// This is the minimal policy for local/first production phase.
func (s *SessionService) Authorize(req model.MediaMTXAuthRequest) error {
	path := model.NormalizePath(req.Path)
	sessionID, direction, ok := model.ParseSessionPath(path)
	if !ok {
		return ErrUnauthorized
	}

	_ = sessionID
	action := strings.ToLower(strings.TrimSpace(req.Action))
	user := strings.TrimSpace(req.User)
	pass := strings.TrimSpace(req.Password)

	if action == "publish" && direction == model.DirectionIn &&
		user == s.cfg.WhipUsername && pass == s.cfg.WhipPassword {
		return nil
	}

	if action == "read" && direction == model.DirectionOut &&
		user == s.cfg.WhepUsername && pass == s.cfg.WhepPassword {
		return nil
	}

	if ((action == "read" && direction == model.DirectionIn) ||
		(action == "publish" && direction == model.DirectionOut)) &&
		user == s.cfg.WorkerRTSPUser && pass == s.cfg.WorkerRTSPPass {
		return nil
	}

	return ErrUnauthorized
}

// toPayload converts internal model into API response model.
func (s *SessionService) toPayload(session *model.Session) *model.SessionPayload {
	return &model.SessionPayload{
		SessionID:   session.SessionID,
		AvatarID:    session.AvatarID,
		WorkerMode:  session.WorkerMode,
		Status:      session.Status,
		CreatedAt:   session.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   session.UpdatedAt.Format(time.RFC3339),
		InputReady:  session.InputReady,
		OutputReady: session.OutputReady,
		ViewerCount: session.ViewerCount,
		Publish: model.SessionPublish{
			WhipURL:  s.whipURL(session.SessionID),
			Username: s.cfg.WhipUsername,
			Password: s.cfg.WhipPassword,
		},
		Playback: model.SessionPlayback{
			WhepURL:  s.whepURL(session.SessionID),
			Username: s.cfg.WhepUsername,
			Password: s.cfg.WhepPassword,
		},
		Internal: model.SessionInternal{
			WorkerInputRTSP:  s.workerInputRTSPURL(session.SessionID),
			WorkerOutputRTSP: s.workerOutputRTSPURL(session.SessionID),
		},
	}
}

// URL builders keep URL composition logic centralized.
func (s *SessionService) whipURL(sessionID string) string {
	return fmt.Sprintf("%s/%s/whip", s.cfg.MediamtxWebRTCBaseURL, inputPath(sessionID))
}

func (s *SessionService) whepURL(sessionID string) string {
	return fmt.Sprintf("%s/%s/whep", s.cfg.MediamtxWebRTCBaseURL, outputPath(sessionID))
}

func (s *SessionService) workerInputRTSPURL(sessionID string) string {
	return fmt.Sprintf("%s/%s", s.cfg.MediamtxRTSPBaseURL, inputPath(sessionID))
}

func (s *SessionService) workerOutputRTSPURL(sessionID string) string {
	return fmt.Sprintf("%s/%s", s.cfg.MediamtxRTSPBaseURL, outputPath(sessionID))
}

// normalizeAvatarID applies server-side defaults.
func normalizeAvatarID(avatarID string) string {
	value := strings.TrimSpace(avatarID)
	if value == "" {
		return "default-avatar"
	}
	return value
}

// normalizeWorkerMode validates requested mode and falls back to config default.
func normalizeWorkerMode(workerMode, fallback string) model.WorkerMode {
	raw := strings.ToLower(strings.TrimSpace(workerMode))
	switch model.WorkerMode(raw) {
	case model.WorkerModeSoulX:
		return model.WorkerModeSoulX
	case model.WorkerModePassthrough:
		return model.WorkerModePassthrough
	default:
		if strings.ToLower(strings.TrimSpace(fallback)) == string(model.WorkerModeSoulX) {
			return model.WorkerModeSoulX
		}
		return model.WorkerModePassthrough
	}
}

// Path helpers define canonical stream paths.
func inputPath(sessionID string) string {
	return fmt.Sprintf("avatar/%s/in", sessionID)
}

func outputPath(sessionID string) string {
	return fmt.Sprintf("avatar/%s/out", sessionID)
}
