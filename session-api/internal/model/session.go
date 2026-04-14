package model

import "time"

// WorkerMode controls how worker handles a session stream.
type WorkerMode string

const (
	WorkerModePassthrough WorkerMode = "passthrough"
	WorkerModeSoulX       WorkerMode = "soulx"
)

// SessionStatus models a lightweight runtime state machine.
type SessionStatus string

const (
	// StatusWaitingInput means WHIP ingest is not ready yet.
	StatusWaitingInput SessionStatus = "waiting_input"
	// StatusInputLive means input stream is live.
	StatusInputLive    SessionStatus = "input_live"
	// StatusProcessing means worker is expected to process but output is not ready.
	StatusProcessing   SessionStatus = "processing"
	// StatusOutputLive means output stream is live for viewers.
	StatusOutputLive   SessionStatus = "output_live"
	StatusStopping     SessionStatus = "stopping"
	StatusStopped      SessionStatus = "stopped"
	StatusFailed       SessionStatus = "failed"
)

// Session is the internal persistence model stored in Redis.
type Session struct {
	SessionID   string        `json:"session_id"`
	AvatarID    string        `json:"avatar_id"`
	WorkerMode  WorkerMode    `json:"worker_mode"`
	Status      SessionStatus `json:"status"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	InputReady  bool          `json:"input_ready"`
	OutputReady bool          `json:"output_ready"`
	ViewerCount int           `json:"viewer_count"`
	LastError   string        `json:"last_error,omitempty"`
}

// CreateSessionRequest is the external API request payload.
type CreateSessionRequest struct {
	AvatarID   string `json:"avatar_id"`
	WorkerMode string `json:"worker_mode"`
}

// SessionPayload is the external API response payload.
// It contains both client URLs and internal worker URLs.
type SessionPayload struct {
	SessionID   string          `json:"session_id"`
	AvatarID    string          `json:"avatar_id"`
	WorkerMode  WorkerMode      `json:"worker_mode"`
	Status      SessionStatus   `json:"status"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
	InputReady  bool            `json:"input_ready"`
	OutputReady bool            `json:"output_ready"`
	ViewerCount int             `json:"viewer_count"`
	Publish     SessionPublish  `json:"publish"`
	Playback    SessionPlayback `json:"playback"`
	Internal    SessionInternal `json:"internal"`
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

// SessionInternal contains worker-only addresses.
type SessionInternal struct {
	WorkerInputRTSP  string `json:"worker_input_rtsp"`
	WorkerOutputRTSP string `json:"worker_output_rtsp"`
}

// StartQueueMessage is consumed by worker supervisor to start processing.
type StartQueueMessage struct {
	Action           string `json:"action"`
	SessionID        string `json:"session_id"`
	WorkerMode       string `json:"worker_mode"`
	WorkerInputRTSP  string `json:"worker_input_rtsp"`
	WorkerOutputRTSP string `json:"worker_output_rtsp"`
}

// StopQueueMessage is consumed by worker supervisor to stop processing.
type StopQueueMessage struct {
	Action    string `json:"action"`
	SessionID string `json:"session_id"`
}
