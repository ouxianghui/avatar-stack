package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds all runtime settings for session-api.
// All values are sourced from environment variables in Load().
type Config struct {
	// HTTPAddr is the HTTP listen address, for example ":8080".
	HTTPAddr               string
	// RedisURL is the session/state backend URL.
	RedisURL               string
	// MediamtxWebRTCBaseURL is used to build client WHIP/WHEP URLs.
	MediamtxWebRTCBaseURL  string
	// MediamtxRTSPBaseURL is used to build worker internal RTSP URLs.
	MediamtxRTSPBaseURL    string
	// Credentials for publisher (WHIP ingress).
	WhipUsername           string
	WhipPassword           string
	// Credentials for viewer (WHEP egress).
	WhepUsername           string
	WhepPassword           string
	// Credentials for worker internal read/write.
	WorkerRTSPUser         string
	WorkerRTSPPass         string
	// Redis queues used by worker controller.
	StartQueue             string
	StopQueue              string
	// SessionTTL controls how long session records stay in Redis.
	SessionTTL             time.Duration
	// RequestTimeout applies to incoming HTTP requests.
	RequestTimeout         time.Duration
	// ShutdownTimeout bounds graceful stop duration.
	ShutdownTimeout        time.Duration
	// DefaultWorkerMode is used when create request omits/uses invalid mode.
	DefaultWorkerMode      string
	// SessionKeyPrefix namespaces keys in Redis.
	SessionKeyPrefix       string
	// AllowedInternalAuthIPs limits access to internal auth endpoint.
	AllowedInternalAuthIPs map[string]struct{}
}

// Load parses environment variables and validates critical settings.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:              getEnv("HTTP_ADDR", ":8080"),
		RedisURL:              getEnv("REDIS_URL", "redis://redis:6379/0"),
		MediamtxWebRTCBaseURL: normalizeBaseURL(getEnv("MEDIAMTX_WEBRTC_BASE_URL", "http://localhost:8889")),
		MediamtxRTSPBaseURL:   normalizeBaseURL(getEnv("MEDIAMTX_RTSP_BASE_URL", "rtsp://mediamtx:8554")),
		WhipUsername:          getEnv("WHIP_USERNAME", "publisher"),
		WhipPassword:          getEnv("WHIP_PASSWORD", "publisher-pass"),
		WhepUsername:          getEnv("WHEP_USERNAME", "viewer"),
		WhepPassword:          getEnv("WHEP_PASSWORD", "viewer-pass"),
		WorkerRTSPUser:        getEnv("WORKER_RTSP_USER", "worker"),
		WorkerRTSPPass:        getEnv("WORKER_RTSP_PASS", "worker-pass"),
		StartQueue:            getEnv("START_QUEUE", "avatar:sessions:start"),
		StopQueue:             getEnv("STOP_QUEUE", "avatar:sessions:stop"),
		DefaultWorkerMode:     getEnv("DEFAULT_WORKER_MODE", "passthrough"),
		SessionKeyPrefix:      getEnv("SESSION_KEY_PREFIX", "avatar:session:"),
	}

	var err error
	cfg.SessionTTL, err = getDurationEnv("SESSION_TTL", 24*time.Hour)
	if err != nil {
		return Config{}, fmt.Errorf("invalid SESSION_TTL: %w", err)
	}

	cfg.RequestTimeout, err = getDurationEnv("REQUEST_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, fmt.Errorf("invalid REQUEST_TIMEOUT: %w", err)
	}

	cfg.ShutdownTimeout, err = getDurationEnv("SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, fmt.Errorf("invalid SHUTDOWN_TIMEOUT: %w", err)
	}

	allowedIPs := strings.TrimSpace(getEnv("INTERNAL_AUTH_ALLOWED_IPS", "127.0.0.1,::1,mediamtx"))
	cfg.AllowedInternalAuthIPs = make(map[string]struct{})
	if allowedIPs != "" {
		for _, ip := range strings.Split(allowedIPs, ",") {
			cfg.AllowedInternalAuthIPs[strings.TrimSpace(ip)] = struct{}{}
		}
	}

	if cfg.MediamtxWebRTCBaseURL == "" || cfg.MediamtxRTSPBaseURL == "" {
		return Config{}, fmt.Errorf("mediamtx base URLs must not be empty")
	}

	return cfg, nil
}

// getEnv returns env value with fallback and trims spaces.
func getEnv(key, fallback string) string {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	return strings.TrimSpace(v)
}

// getDurationEnv parses duration env value such as "5s", "10m", "24h".
func getDurationEnv(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(getEnv(key, ""))
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	return d, nil
}

// normalizeBaseURL removes trailing slash to keep URL joins deterministic.
func normalizeBaseURL(input string) string {
	return strings.TrimRight(strings.TrimSpace(input), "/")
}
