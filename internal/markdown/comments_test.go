package markdown

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/famasya/gdocs-cli/internal/gdocs"
)

func TestConvertComments(t *testing.T) {
	tests := []struct {
		name     string
		comments []gdocs.Comment
		wantSub  string // Verifies a substring of the output for cleanliness & correctness
	}{
		{
			name:     "nil comments",
			comments: nil,
			wantSub:  "",
		},
		{
			name:     "empty comments",
			comments: []gdocs.Comment{},
			wantSub:  "",
		},
		{
			name: "single comment with details",
			comments: []gdocs.Comment{
				{
					ID:          "AAAB9AVHdik",
					Author:      "Alice",
					AuthorEmail: "alice@example.com",
					Content:     "This is a comment",
					QuotedText:  "some text",
					Replies: []gdocs.Reply{
						{
							Author:      "Bob",
							AuthorEmail: "bob@example.com",
							Content:     "I agree",
							CreatedTime: "2026-06-08",
						},
					},
				},
			},
			wantSub: `"id": "AAAB9AVHdik"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertComments(tt.comments, nil, nil)
			if tt.wantSub == "" {
				if got != "" {
					t.Errorf("ConvertComments() = %q, want empty", got)
				}
				return
			}
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("ConvertComments() = %q, want to contain %q", got, tt.wantSub)
			}

			// Validate that the output inside <!-- gdoc-comment: and --> is indeed valid pretty-printed JSON
			if got != "" {
				lines := strings.Split(got, "\n")
				var jsonLines []string
				recording := false
				for _, line := range lines {
					if strings.HasPrefix(line, "<!-- gdoc-comment:") {
						recording = true
						continue
					}
					if strings.HasPrefix(line, "-->") {
						recording = false
						continue
					}
					if recording {
						jsonLines = append(jsonLines, line)
					}
				}

				jsonStr := strings.Join(jsonLines, "\n")
				var parsed []gdocs.Comment
				if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
					t.Errorf("Failed to unmarshal emitted JSON inside comment block: %v\nJSON string:\n%s", err, jsonStr)
				} else {
					if len(parsed) != 1 || parsed[0].ID != tt.comments[0].ID {
						t.Errorf("Parsed comment ID mismatch: %v", parsed)
					}
				}
			}
		})
	}
}