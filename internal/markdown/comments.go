package markdown

import (
	"fmt"
	"strings"
	"time"

	"github.com/famasya/gdocs-cli/internal/gdocs"
)

// ConvertSingleComment renders a single comment enclosed in an HTML comment compatible with markdown.
func ConvertSingleComment(c gdocs.Comment) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("<!-- gdoc-comment: %s\n", c.ID))

	if c.QuotedText != "" {
		builder.WriteString("> ")
		builder.WriteString(strings.ReplaceAll(c.QuotedText, "\n", "\n> "))
		builder.WriteString("\n\n")
	}

	author := c.Author
	if author == "" {
		author = "Unknown"
	}
	author = escapeMarkdown(author)
	builder.WriteString(fmt.Sprintf("**%s**", author))
	if ts := formatTime(c.CreatedTime); ts != "" {
		builder.WriteString(fmt.Sprintf(" (%s)", ts))
	}
	if c.Resolved {
		builder.WriteString(" ✓ resolved")
	}
	builder.WriteString(": ")
	builder.WriteString(c.Content)
	builder.WriteString("\n")

	for _, r := range c.Replies {
		rAuthor := r.Author
		if rAuthor == "" {
			rAuthor = "Unknown"
		}
		rAuthor = escapeMarkdown(rAuthor)
		builder.WriteString(fmt.Sprintf("  ↳ **%s**", rAuthor))
		if ts := formatTime(r.CreatedTime); ts != "" {
			builder.WriteString(fmt.Sprintf(" (%s)", ts))
		}
		builder.WriteString(": ")
		builder.WriteString(r.Content)
		builder.WriteString("\n")
	}

	builder.WriteString("\n-->")
	return builder.String()
}

// ConvertComments renders comments as a markdown section with three subsections:
// anchored (uniquely located in the document), ambiguous (location unclear), and
// deleted (the commented-on text no longer exists in the document).
// Subsections with no comments are omitted.
func ConvertComments(anchored, ambiguous, deleted []gdocs.Comment) string {
	total := len(anchored) + len(ambiguous) + len(deleted)
	if total == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("## Comments\n\n")

	if len(anchored) > 0 {
		// Only add subsection heading for anchored if there's also ambiguous or deleted.
		if len(ambiguous) > 0 || len(deleted) > 0 {
			builder.WriteString("### Anchored\n\n")
		}
		for _, c := range anchored {
			builder.WriteString(ConvertSingleComment(c))
			builder.WriteString("\n\n")
		}
	}

	if len(ambiguous) > 0 {
		builder.WriteString("### Location ambiguous\n\n")
		for _, c := range ambiguous {
			builder.WriteString(ConvertSingleComment(c))
			builder.WriteString("\n\n")
		}
	}

	if len(deleted) > 0 {
		builder.WriteString("### Deleted content\n\n")
		for _, c := range deleted {
			builder.WriteString(ConvertSingleComment(c))
			builder.WriteString("\n\n")
		}
	}

	return builder.String()
}

func countNonEmpty(flags ...bool) int {
	n := 0
	for _, f := range flags {
		if f {
			n++
		}
	}
	return n
}

func renderComment(c gdocs.Comment, builder *strings.Builder) {
	if c.QuotedText != "" {
		builder.WriteString("> ")
		builder.WriteString(strings.ReplaceAll(c.QuotedText, "\n", "\n> "))
		builder.WriteString("\n\n")
	}

	if c.ID != "" {
		builder.WriteString(fmt.Sprintf("(id: %s) ", c.ID))
	}
	author := c.Author
	if author == "" {
		author = "Unknown"
	}
	author = escapeMarkdown(author)
	builder.WriteString(fmt.Sprintf("**%s**", author))
	if ts := formatTime(c.CreatedTime); ts != "" {
		builder.WriteString(fmt.Sprintf(" (%s)", ts))
	}
	if c.Resolved {
		builder.WriteString(" ✓ resolved")
	}
	builder.WriteString(": ")
	builder.WriteString(c.Content)
	builder.WriteString("\n")

	for _, r := range c.Replies {
		rAuthor := r.Author
		if rAuthor == "" {
			rAuthor = "Unknown"
		}
		rAuthor = escapeMarkdown(rAuthor)
		builder.WriteString(fmt.Sprintf("  ↳ **%s**", rAuthor))
		if ts := formatTime(r.CreatedTime); ts != "" {
			builder.WriteString(fmt.Sprintf(" (%s)", ts))
		}
		builder.WriteString(": ")
		builder.WriteString(r.Content)
		builder.WriteString("\n")
	}

	builder.WriteString("\n")
}

// formatTime converts an RFC 3339 timestamp to a short date string.
func formatTime(rfc3339 string) string {
	if rfc3339 == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return ""
	}
	return t.Format("2006-01-02")
}

func escapeMarkdown(s string) string {
	s = strings.ReplaceAll(s, "*", "\\*")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}
