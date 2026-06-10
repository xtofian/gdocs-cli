package markdown

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/famasya/gdocs-cli/internal/gdocs"
)

// ConvertSingleComment renders a single comment enclosed in an HTML comment compatible with markdown.
func ConvertSingleComment(c gdocs.Comment) string {
	data, err := json.MarshalIndent([]gdocs.Comment{c}, "", "  ")
	if err != nil {
		return fmt.Sprintf("<!-- gdoc-comment: error encoding comment: %v -->", err)
	}
	return fmt.Sprintf("<!-- gdoc-comment:\n%s\n-->", string(data))
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