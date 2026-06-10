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

// ConvertComments renders comments as a markdown section for unattached comments.
func ConvertComments(anchored, ambiguous, deleted []gdocs.Comment) string {
	total := len(anchored) + len(ambiguous) + len(deleted)
	if total == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("## Comments (unattached)\n\n")

	// Merge all of them into a single list
	var all []gdocs.Comment
	all = append(all, anchored...)
	all = append(all, ambiguous...)
	all = append(all, deleted...)

	for _, c := range all {
		builder.WriteString(ConvertSingleComment(c))
		builder.WriteString("\n\n")
	}

	return builder.String()
}