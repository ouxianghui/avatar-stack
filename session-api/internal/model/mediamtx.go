package model

import (
	"net/url"
	"regexp"
	"strings"
)

// sessionPathPattern recognizes canonical media paths:
// avatar/{session_id}/live
var sessionPathPattern = regexp.MustCompile(`^avatar/([^/]+)/live$`)

// MediaMTXAuthRequest is a normalized view of MediaMTX auth callback payload.
type MediaMTXAuthRequest struct {
	User     string `json:"user"`
	Password string `json:"password"`
	Action   string `json:"action"`
	Path     string `json:"path"`
	IP       string `json:"ip"`
	Protocol string `json:"protocol"`
	ID       string `json:"id"`
}

// NormalizePath accepts URL/path variants from callbacks and converts them
// into canonical path format: avatar/{session_id}/live
func NormalizePath(raw string) string {
	path := strings.TrimSpace(raw)
	if path == "" {
		return ""
	}

	if strings.Contains(path, "://") {
		if parsed, err := url.Parse(path); err == nil {
			path = parsed.Path
		}
	}

	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")
	path = strings.TrimSuffix(path, "/whip")
	path = strings.TrimSuffix(path, "/whep")

	if idx := strings.Index(path, "?"); idx >= 0 {
		path = path[:idx]
	}

	return path
}

// ParseSessionPath extracts session_id from callback path.
// It returns ok=false for unrelated paths.
func ParseSessionPath(raw string) (sessionID string, ok bool) {
	normalized := NormalizePath(raw)
	matches := sessionPathPattern.FindStringSubmatch(normalized)
	if len(matches) != 2 {
		return "", false
	}

	return matches[1], true
}
