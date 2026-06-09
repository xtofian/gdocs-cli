package gdocs

import (
	"strings"
	"testing"
)

func TestIsDraft(t *testing.T) {
	tests := []struct {
		name   string
		update CommentUpdate
		want   bool
	}{
		{
			name: "active comment",
			update: CommentUpdate{
				ID:         "123",
				NewComment: "Hello",
				Status:     "ready",
			},
			want: false,
		},
		{
			name: "active comment without status",
			update: CommentUpdate{
				ID:         "123",
				NewComment: "Hello",
			},
			want: false,
		},
		{
			name: "draft status",
			update: CommentUpdate{
				ID:         "123",
				NewComment: "Hello",
				Status:     "draft",
			},
			want: true,
		},
		{
			name: "empty comment",
			update: CommentUpdate{
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
				{"id": "c1", "new-comment": "Good comment", "status": "ready"},
				{"id": "c2", "new-comment": "Draft comment", "status": "draft"},
				{"id": "", "new-comment": "Another draft", "status": "draft"}
			]`,
			wantErr: false,
		},
		{
			name: "missing id for active comment",
			json: `[
				{"id": "", "new-comment": "Active but missing id", "status": "ready"}
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
