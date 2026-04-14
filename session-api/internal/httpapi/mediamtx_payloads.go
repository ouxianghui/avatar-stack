package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"avatar-stack/session-api/internal/model"
)

// genericBody represents a superset of fields we may receive from MediaMTX.
// MediaMTX callbacks can be configured as JSON body or URL-encoded form.
type genericBody struct {
	User     string `json:"user"`
	Password string `json:"password"`
	Action   string `json:"action"`
	Path     string `json:"path"`
	IP       string `json:"ip"`
	Protocol string `json:"protocol"`
	ID       string `json:"id"`
}

// readMediaMTXAuthRequest parses callback payload in a tolerant way:
// - prefer JSON when content-type is JSON
// - otherwise fallback to form fields
func readMediaMTXAuthRequest(r *http.Request) model.MediaMTXAuthRequest {
	if isJSONRequest(r.Header.Get("Content-Type")) {
		body := readBodyOrEmpty(r)
		var payload genericBody
		if err := json.Unmarshal(body, &payload); err == nil {
			return model.MediaMTXAuthRequest{
				User:     payload.User,
				Password: payload.Password,
				Action:   payload.Action,
				Path:     payload.Path,
				IP:       payload.IP,
				Protocol: payload.Protocol,
				ID:       payload.ID,
			}
		}
	}

	_ = r.ParseForm()
	// Accept both user/pass and username/password naming styles.
	return model.MediaMTXAuthRequest{
		User:     firstNonEmpty(r.FormValue("user"), r.FormValue("username")),
		Password: firstNonEmpty(r.FormValue("pass"), r.FormValue("password")),
		Action:   r.FormValue("action"),
		Path:     r.FormValue("path"),
		IP:       firstNonEmpty(r.FormValue("ip"), r.FormValue("remote_ip")),
		Protocol: r.FormValue("protocol"),
		ID:       r.FormValue("id"),
	}
}

// readPathFromBody extracts `path` for hook callback handlers.
func readPathFromBody(r *http.Request) string {
	if isJSONRequest(r.Header.Get("Content-Type")) {
		body := readBodyOrEmpty(r)
		var payload genericBody
		if err := json.Unmarshal(body, &payload); err == nil {
			return payload.Path
		}
	}

	_ = r.ParseForm()
	return firstNonEmpty(r.FormValue("path"), r.URL.Query().Get("path"))
}

// isJSONRequest checks whether content-type indicates JSON payload.
func isJSONRequest(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "application/json")
}

// readBodyOrEmpty reads request body once and returns empty bytes on read failure.
func readBodyOrEmpty(r *http.Request) []byte {
	if r.Body == nil {
		return nil
	}
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil
	}
	return body
}

// firstNonEmpty returns the first non-empty candidate after trimming spaces.
func firstNonEmpty(candidates ...string) string {
	for _, candidate := range candidates {
		trimmed := strings.TrimSpace(candidate)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
