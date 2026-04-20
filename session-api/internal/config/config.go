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
	HTTPAddr              string
	// RedisURL is the session/state backend URL.
	RedisURL              string
	// MediamtxWebRTCBaseURL is used to build client WHIP/WHEP URLs.
	MediamtxWebRTCBaseURL string
	// WhipUsername is the Basic Auth user WHIP clients must send (password is the short-lived token).
	WhipUsername          string
	// WhepUsername is the Basic Auth user WHEP clients must send (password is the short-lived token).
	WhepUsername          string
	// SessionTTL is Redis TTL for session + token validity (short-lived credentials).
	SessionTTL            time.Duration
	// RequestTimeout applies to incoming HTTP requests.
	RequestTimeout        time.Duration
	// ShutdownTimeout bounds graceful stop duration.
	ShutdownTimeout       time.Duration
	// SessionKeyPrefix namespaces keys in Redis.
	SessionKeyPrefix      string
	// AllowedInternalAuthIPs limits access to internal auth/hook endpoints when non-empty (exact IP strings).
	// When empty, loopback and RFC1918/ULA addresses are allowed (typical Docker Compose).
	AllowedInternalAuthIPs map[string]struct{}
}

// Load parses environment variables and validates critical settings.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:              getEnv("HTTP_ADDR", ":8080"),
		RedisURL:              getEnv("REDIS_URL", "redis://redis:6379/0"),
		MediamtxWebRTCBaseURL: normalizeBaseURL(getEnv("MEDIAMTX_WEBRTC_BASE_URL", "http://localhost:8889")),
		WhipUsername:          getEnv("WHIP_USERNAME", "publisher"),
		WhepUsername:          getEnv("WHEP_USERNAME", "viewer"),
		SessionKeyPrefix:      getEnv("SESSION_KEY_PREFIX", "avatar:session:"),
	}

	var err error
	cfg.SessionTTL, err = resolveSessionTTL()
	if err != nil {
		return Config{}, err
	}

	cfg.RequestTimeout, err = getDurationEnv("REQUEST_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, fmt.Errorf("invalid REQUEST_TIMEOUT: %w", err)
	}

	cfg.ShutdownTimeout, err = getDurationEnv("SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, fmt.Errorf("invalid SHUTDOWN_TIMEOUT: %w", err)
	}

	allowedIPs := strings.TrimSpace(getEnv("INTERNAL_AUTH_ALLOWED_IPS", ""))
	cfg.AllowedInternalAuthIPs = make(map[string]struct{})
	if allowedIPs != "" {
		for _, ip := range strings.Split(allowedIPs, ",") {
			cfg.AllowedInternalAuthIPs[strings.TrimSpace(ip)] = struct{}{}
		}
	}

	if cfg.MediamtxWebRTCBaseURL == "" {
		return Config{}, fmt.Errorf("mediamtx WebRTC base URL must not be empty")
	}
	if cfg.WhipUsername == "" || cfg.WhepUsername == "" {
		return Config{}, fmt.Errorf("WHIP_USERNAME and WHEP_USERNAME must not be empty")
	}

	return cfg, nil
}

// resolveSessionTTL prefers SESSION_TTL when set; otherwise MEDIAMTX_TOKEN_TTL (default 1h).
func resolveSessionTTL() (time.Duration, error) {
	rawSession := strings.TrimSpace(os.Getenv("SESSION_TTL"))
	if rawSession != "" {
		d, err := time.ParseDuration(rawSession)
		if err != nil {
			return 0, fmt.Errorf("invalid SESSION_TTL: %w", err)
		}
		return d, nil
	}
	return getDurationEnv("MEDIAMTX_TOKEN_TTL", time.Hour)
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
