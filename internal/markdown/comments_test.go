package markdown

import (
	"strings"
	"testing"

	"github.com/famasya/gdocs-cli/internal/gdocs"
	"gopkg.in/yaml.v3"
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
			wantSub: `id: AAAB9AVHdik`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertComments(tt.comments)
			if tt.wantSub == "" {
				if got != "" {
					t.Errorf("ConvertComments() = %q, want empty", got)
				}
				return
			}
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("ConvertComments() = %q, want to contain %q", got, tt.wantSub)
			}
			if tt.name == "single comment with details" {
				if !strings.Contains(got, "status: draft") {
					t.Errorf("ConvertComments() missing 'status: draft', got: %q", got)
				}
				if !strings.Contains(got, `new-reply: ""`) {
					t.Errorf("ConvertComments() missing 'new-reply: \"\"', got: %q", got)
				}
			}

			// Validate that the output inside <!-- gdoc-comment-content: and --> is indeed valid YAML
			if got != "" {
				lines := strings.Split(got, "\n")
				var yamlLines []string
				recording := false
				for _, line := range lines {
					if strings.HasPrefix(line, "<!-- gdoc-comment-content:") {
						recording = true
						continue
					}
					if strings.HasPrefix(line, "-->") {
						recording = false
						continue
					}
					if recording {
						yamlLines = append(yamlLines, line)
					}
				}

				yamlStr := strings.Join(yamlLines, "\n")
				var parsed []gdocs.Comment
				if err := yaml.Unmarshal([]byte(yamlStr), &parsed); err != nil {
					t.Errorf("Failed to unmarshal emitted YAML inside comment block: %v\nYAML string:\n%s", err, yamlStr)
				} else {
					if len(parsed) != 1 || parsed[0].ID != tt.comments[0].ID {
						t.Errorf("Parsed comment ID mismatch: %v", parsed)
					}
				}
			}
		})
	}
}
