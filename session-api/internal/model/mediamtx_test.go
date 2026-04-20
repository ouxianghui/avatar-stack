package model

import "testing"

// TestParseSessionPath validates path normalization + extraction behavior
// against typical MediaMTX callback path variants.
func TestParseSessionPath(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantID    string
		wantValid bool
	}{
		{
			name:      "plain path",
			raw:       "avatar/s1/live",
			wantID:    "s1",
			wantValid: true,
		},
		{
			name:      "whep suffix path",
			raw:       "/avatar/s2/live/whep",
			wantID:    "s2",
			wantValid: true,
		},
		{
			name:      "full url",
			raw:       "https://media.example.com/avatar/s3/live/whip?token=x",
			wantID:    "s3",
			wantValid: true,
		},
		{
			name:      "legacy in path rejected",
			raw:       "avatar/s1/in",
			wantValid: false,
		},
		{
			name:      "invalid path",
			raw:       "/random/path",
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, ok := ParseSessionPath(tt.raw)
			if ok != tt.wantValid {
				t.Fatalf("valid mismatch: got=%v want=%v", ok, tt.wantValid)
			}
			if !tt.wantValid {
				return
			}
			if gotID != tt.wantID {
				t.Fatalf("session id mismatch: got=%s want=%s", gotID, tt.wantID)
			}
		})
	}
}
