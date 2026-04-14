package model

import "testing"

// TestParseSessionPath validates path normalization + extraction behavior
// against typical MediaMTX callback path variants.
func TestParseSessionPath(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantID    string
		wantDir   StreamDirection
		wantValid bool
	}{
		{
			name:      "plain path",
			raw:       "avatar/s1/in",
			wantID:    "s1",
			wantDir:   DirectionIn,
			wantValid: true,
		},
		{
			name:      "whip suffix path",
			raw:       "/avatar/s2/out/whep",
			wantID:    "s2",
			wantDir:   DirectionOut,
			wantValid: true,
		},
		{
			name:      "full url",
			raw:       "https://media.example.com/avatar/s3/in/whip?token=x",
			wantID:    "s3",
			wantDir:   DirectionIn,
			wantValid: true,
		},
		{
			name:      "invalid path",
			raw:       "/random/path",
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotDir, ok := ParseSessionPath(tt.raw)
			if ok != tt.wantValid {
				t.Fatalf("valid mismatch: got=%v want=%v", ok, tt.wantValid)
			}
			if !tt.wantValid {
				return
			}
			if gotID != tt.wantID {
				t.Fatalf("session id mismatch: got=%s want=%s", gotID, tt.wantID)
			}
			if gotDir != tt.wantDir {
				t.Fatalf("direction mismatch: got=%s want=%s", gotDir, tt.wantDir)
			}
		})
	}
}
