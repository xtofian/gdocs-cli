package gdocs

import (
	"os"
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

func TestParseCommentsFromFile(t *testing.T) {
	// Setup embedded comments MD temp file
	tmpFile, err := os.CreateTemp("", "comments-*.md")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := `---
title: Test Doc
gdoc_id: 123456
---
Some body text.
<!-- gdoc-comment-content:
- id: c1
  comment: "This is a comment"
  new-reply: "This is my reply"
-->
More body text.
<!-- gdoc-comment-content:
- id: c2
  comment: "Another comment"
  new-reply: "Another reply"
-->
`
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile.Close()

	// Case 1: valid doc ID
	comments, err := ParseCommentsFromFile(tmpFile.Name(), "123456")
	if err != nil {
		t.Fatalf("ParseCommentsFromFile failed: %v", err)
	}
	if len(comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(comments))
	}
	if comments[0].ID != "c1" || comments[0].NewReply != "This is my reply" {
		t.Errorf("first comment mismatched: %+v", comments[0])
	}

	// Case 2: doc ID mismatch
	_, err = ParseCommentsFromFile(tmpFile.Name(), "mismatched-doc-id")
	if err == nil {
		t.Error("Expected error for document ID mismatch, but got nil")
	} else if !strings.Contains(err.Error(), "document ID mismatch") {
		t.Errorf("expected error message to contain 'document ID mismatch', got: %v", err)
	}

	// Case 3: raw YAML comment file (no html comments, no frontmatter)
	tmpFile2, err := os.CreateTemp("", "raw-comments-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile2.Name())

	rawContent := `- id: c3
  comment: "Raw comment"
  new-reply: "Raw reply"
`
	if _, err := tmpFile2.WriteString(rawContent); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tmpFile2.Close()

	comments, err = ParseCommentsFromFile(tmpFile2.Name(), "")
	if err != nil {
		t.Fatalf("ParseCommentsFromFile failed on raw YAML: %v", err)
	}
	if len(comments) != 1 || comments[0].ID != "c3" || comments[0].NewReply != "Raw reply" {
		t.Errorf("expected 1 comment from raw YAML, got %+v", comments)
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
