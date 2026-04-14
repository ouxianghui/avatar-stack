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

type fakeStore struct{}

func (f *fakeStore) Ping(context.Context) error { return nil }
func (f *fakeStore) PutSession(context.Context, *model.Session) error {
	return nil
}
func (f *fakeStore) GetSession(context.Context, string) (*model.Session, error) {
	return nil, store.ErrNotFound
}
func (f *fakeStore) PublishStart(context.Context, model.StartQueueMessage) error {
	return nil
}
func (f *fakeStore) PublishStop(context.Context, model.StopQueueMessage) error {
	return nil
}
func (f *fakeStore) Close() error { return nil }

// TestAuthorize is table-driven to document current auth policy matrix.
func TestAuthorize(t *testing.T) {
	cfg := config.Config{
		WhipUsername:      "publisher",
		WhipPassword:      "publisher-pass",
		WhepUsername:      "viewer",
		WhepPassword:      "viewer-pass",
		WorkerRTSPUser:    "worker",
		WorkerRTSPPass:    "worker-pass",
		MediamtxRTSPBaseURL:   "rtsp://mediamtx:8554",
		MediamtxWebRTCBaseURL: "http://localhost:8889",
		SessionTTL:        time.Hour,
	}

	svc := NewSessionService(cfg, &fakeStore{}, slog.Default())

	tests := []struct {
		name    string
		req     model.MediaMTXAuthRequest
		wantErr bool
	}{
		{
			name: "allow publisher input publish",
			req: model.MediaMTXAuthRequest{
				User: "publisher", Password: "publisher-pass", Action: "publish", Path: "avatar/a1/in",
			},
			wantErr: false,
		},
		{
			name: "deny publisher output publish",
			req: model.MediaMTXAuthRequest{
				User: "publisher", Password: "publisher-pass", Action: "publish", Path: "avatar/a1/out",
			},
			wantErr: true,
		},
		{
			name: "allow viewer output read",
			req: model.MediaMTXAuthRequest{
				User: "viewer", Password: "viewer-pass", Action: "read", Path: "avatar/a1/out",
			},
			wantErr: false,
		},
		{
			name: "allow worker input read",
			req: model.MediaMTXAuthRequest{
				User: "worker", Password: "worker-pass", Action: "read", Path: "avatar/a1/in",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.Authorize(tt.req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Authorize() error=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}
}
