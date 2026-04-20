package model

import "time"

// SessionStatus models a lightweight runtime state machine.
type SessionStatus string

const (
	// StatusWaitingInput means WHIP ingest is not ready yet.
	StatusWaitingInput SessionStatus = "waiting_input"
	// StatusInputLive means the publisher path is ready (same path used for playback).
	StatusInputLive SessionStatus = "input_live"
	// StatusProcessing is retained for compatibility with older Redis snapshots.
	StatusProcessing SessionStatus = "processing"
	// StatusOutputLive means the stream is available for WHEP viewers.
	StatusOutputLive SessionStatus = "output_live"
	StatusStopping     SessionStatus = "stopping"
	StatusStopped      SessionStatus = "stopped"
	StatusFailed       SessionStatus = "failed"
)

// Session is the internal persistence model stored in Redis.
// Token hashes are never returned on public HTTP APIs.
type Session struct {
	SessionID   string        `json:"session_id"`
	AvatarID    string        `json:"avatar_id"`
	Status      SessionStatus `json:"status"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	InputReady  bool          `json:"input_ready"`
	OutputReady bool          `json:"output_ready"`
	ViewerCount int           `json:"viewer_count"`
	LastError   string        `json:"last_error,omitempty"`
	// PublishTokenHash is a bcrypt hash of the one-time WHIP password.
	PublishTokenHash string `json:"publish_token_hash,omitempty"`
	// PlaybackTokenHash is a bcrypt hash of the one-time WHEP password.
	PlaybackTokenHash string `json:"playback_token_hash,omitempty"`
}

// CreateSessionRequest is the external API request payload.
type CreateSessionRequest struct {
	AvatarID string `json:"avatar_id"`
}

// SessionPayload is the external API response payload.
type SessionPayload struct {
	SessionID   string          `json:"session_id"`
	AvatarID    string          `json:"avatar_id"`
	Status      SessionStatus   `json:"status"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
	InputReady  bool            `json:"input_ready"`
	OutputReady bool            `json:"output_ready"`
	ViewerCount int             `json:"viewer_count"`
	Publish     SessionPublish  `json:"publish"`
	Playback    SessionPlayback `json:"playback"`
}

// SessionPublish is WHIP client publish config.
type SessionPublish struct {
	WhipURL  string `json:"whip_url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// SessionPlayback is WHEP client playback config.
type SessionPlayback struct {
	WhepURL  string `json:"whep_url"`
	Username string `json:"username"`
	Password string `json:"password"`
}
