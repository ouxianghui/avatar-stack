package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"avatar-stack/session-api/internal/config"
	"avatar-stack/session-api/internal/model"
	"avatar-stack/session-api/internal/service"
	"avatar-stack/session-api/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Router hosts HTTP handlers and delegates business logic to SessionService.
type Router struct {
	cfg config.Config
	svc *service.SessionService
}

// NewRouter registers all public and internal endpoints.
// Internal endpoints are used by MediaMTX auth/hook callbacks.
func NewRouter(cfg config.Config, svc *service.SessionService) http.Handler {
	r := chi.NewRouter()
	api := &Router{cfg: cfg, svc: svc}

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(cfg.RequestTimeout))

	r.Get("/healthz", api.healthz)
	r.Get("/readyz", api.readyz)
	r.Post("/sessions", api.createSession)
	r.Get("/sessions/{sessionID}", api.getSession)
	r.Delete("/sessions/{sessionID}", api.stopSession)
	r.Post("/internal/mediamtx/auth", api.mediamtxAuth)
	r.Post("/internal/mediamtx/hooks/{event}", api.mediamtxHook)

	return r
}

// healthz verifies dependency availability (currently Redis).
func (h *Router) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.RequestTimeout)
	defer cancel()

	if err := h.svc.Health(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "redis unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// readyz currently mirrors healthz.
func (h *Router) readyz(w http.ResponseWriter, r *http.Request) {
	h.healthz(w, r)
}

// createSession starts one session orchestration record.
func (h *Router) createSession(w http.ResponseWriter, r *http.Request) {
	var req model.CreateSessionRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	payload, err := h.svc.CreateSession(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// getSession returns one session snapshot.
func (h *Router) getSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	payload, err := h.svc.GetSession(r.Context(), sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to fetch session")
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// stopSession requests worker stop and marks status to stopping.
func (h *Router) stopSession(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "sessionID")
	payload, err := h.svc.StopSession(r.Context(), sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to stop session")
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// mediamtxAuth enforces minimal source-IP filtering before auth check.
func (h *Router) mediamtxAuth(w http.ResponseWriter, r *http.Request) {
	if !h.isIPAllowed(r.RemoteAddr) {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	req := readMediaMTXAuthRequest(r)
	if err := h.svc.Authorize(req); err != nil {
		if errors.Is(err, service.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// mediamtxHook applies MediaMTX callback events to session state.
func (h *Router) mediamtxHook(w http.ResponseWriter, r *http.Request) {
	event := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "event")))
	if event == "" {
		writeError(w, http.StatusBadRequest, "missing event")
		return
	}

	path := readPathFromBody(r)
	if path == "" {
		writeError(w, http.StatusBadRequest, "missing path")
		return
	}

	if err := h.svc.HandleMediaHook(r.Context(), event, path); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to process hook")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "event": event, "path": model.NormalizePath(path)})
}

// isIPAllowed checks remote caller against allowlist.
// Supports either "host:port" or plain host formats.
func (h *Router) isIPAllowed(remoteAddr string) bool {
	if len(h.cfg.AllowedInternalAuthIPs) == 0 {
		return true
	}

	host := strings.TrimSpace(remoteAddr)
	if parsedHost, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = parsedHost
	}
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")

	_, allowedHost := h.cfg.AllowedInternalAuthIPs[host]
	_, allowedRemote := h.cfg.AllowedInternalAuthIPs[remoteAddr]
	return allowedHost || allowedRemote
}

// decodeJSONBody decodes strict JSON by rejecting unknown fields.
func decodeJSONBody(r *http.Request, out any) error {
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

// writeJSON serializes one JSON response object.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeError returns a consistent error shape for API callers.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"ok":    false,
		"error": msg,
		"ts":    time.Now().UTC().Format(time.RFC3339),
	})
}
