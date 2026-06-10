package gdocs

import (
	"strings"
	"testing"
)

func TestIsDraft(t *testing.T) {
	tests := []struct {
		name   string
		update Comment
		want   bool
	}{
		{
			name: "active comment",
			update: Comment{
				ID:       "123",
				NewReply: "Hello",
				Status:   "ready",
			},
			want: false,
		},
		{
			name: "active comment without status",
			update: Comment{
				ID:       "123",
				NewReply: "Hello",
			},
			want: false,
		},
		{
			name: "draft status",
			update: Comment{
				ID:       "123",
				NewReply: "Hello",
				Status:   "draft",
			},
			want: true,
		},
		{
			name: "empty comment",
			update: Comment{
				ID: "123",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.update.IsDraft(); got != tt.want {
				t.Errorf("IsDraft() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseCommentUpdates(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid JSON with active and draft comments",
			json: `[
				{"id": "c1", "new-reply": "Good comment", "status": "ready"},
				{"id": "c2", "new-reply": "Draft comment", "status": "draft"},
				{"id": "", "new-reply": "Another draft", "status": "draft"}
			]`,
			wantErr: false,
		},
		{
			name: "missing id for active comment",
			json: `[
				{"id": "", "new-reply": "Active but missing id", "status": "ready"}
			]`,
			wantErr: true,
			errMsg:  "comment thread ID ('id') is required for non-draft comments",
		},
		{
			name:    "invalid JSON syntax",
			json:    `invalid json`,
			wantErr: true,
			errMsg:  "failed to decode JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updates, err := ParseCommentUpdates(strings.NewReader(tt.json))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCommentUpdates() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ParseCommentUpdates() error = %q, must contain %q", err.Error(), tt.errMsg)
			}
			if !tt.wantErr && len(updates) == 0 {
				t.Errorf("ParseCommentUpdates() parsed 0 updates, want more")
			}
		})
	}
}

func TestFormatDate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "valid rfc3339",
			input: "2026-06-08T15:30:00Z",
			want:  "2026-06-08",
		},
		{
			name:  "invalid rfc3339",
			input: "invalid",
			want:  "",
		},
		{
			name:  "empty rfc3339",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatDate(tt.input); got != tt.want {
				t.Errorf("formatDate() = %q, want %q", got, tt.want)
			}
		})
	}
}
