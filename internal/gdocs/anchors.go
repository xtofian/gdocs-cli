package gdocs

import (
	"html"
	"regexp"
	"strings"

	"google.golang.org/api/docs/v1"
)

var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

func stripHTML(s string) string {
	s = htmlTagRe.ReplaceAllString(s, "")
	return html.UnescapeString(s)
}

// BuildAnchorMap returns a map from absolute character offset → comment ID
// for each comment whose quoted text can be located in the document body.
// Comments on deleted content are skipped (their text is no longer in the doc).
func BuildAnchorMap(body *docs.Body, comments []Comment) map[int]string {
	result := make(map[int]string)
	if body == nil {
		return result
	}

	for _, c := range comments {
		if c.ID == "" || c.QuotedText == "" {
			continue
		}
		quoted := stripHTML(c.QuotedText)
		if quoted == "" {
			continue
		}

		offset, ok := findTextOffset(body, quoted)
		if ok {
			result[offset] = c.ID
		}
	}

	return result
}

// findTextOffset searches for the first occurrence of needle in the document
// body and returns the absolute character offset of the match start.
func findTextOffset(body *docs.Body, needle string) (int, bool) {
	for _, elem := range body.Content {
		if elem.Paragraph == nil {
			continue
		}

		// Build the paragraph text and a parallel slice of absolute offsets.
		type seg struct {
			start int
			text  string
		}
		var segs []seg
		var sb strings.Builder
		for _, pe := range elem.Paragraph.Elements {
			if pe.TextRun != nil && pe.TextRun.Content != "" {
				segs = append(segs, seg{int(pe.StartIndex), pe.TextRun.Content})
				sb.WriteString(pe.TextRun.Content)
			}
		}

		paraText := sb.String()
		idx := strings.Index(paraText, needle)
		if idx < 0 {
			continue
		}

		// Map the byte position within paraText back to an absolute offset.
		pos := 0
		for _, s := range segs {
			end := pos + len(s.text)
			if end > idx {
				return s.start + (idx - pos), true
			}
			pos = end
		}
	}
	return 0, false
}
