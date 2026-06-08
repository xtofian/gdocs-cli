package gdocs

import (
	"html"
	"log"
	"regexp"
	"sort"
	"strings"

	"google.golang.org/api/docs/v1"
)

var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

func stripHTML(s string) string {
	s = htmlTagRe.ReplaceAllString(s, "")
	return html.UnescapeString(s)
}

// textSeg pairs a document character start offset with a text run's content.
type textSeg struct {
	start int
	text  string
}

// AnchorResult categorises comments by how well their anchor could be located.
type AnchorResult struct {
	// Offsets maps absolute character offset → comment ID for uniquely-located comments.
	Offsets map[int]string
	// AnchoredIDs lists comment IDs with a unique text match, sorted by document position.
	AnchoredIDs []string
	// AmbiguousIDs lists comment IDs whose quoted text appears more than once, or is absent.
	AmbiguousIDs []string
	// DeletedIDs lists comment IDs whose quoted text is no longer present in the document.
	DeletedIDs []string
}

// BuildAnchorResult resolves each comment's position in the document body by
// searching for its quoted text.
//
// Note on the Drive API anchor field: for Google Docs comments the anchor is an
// opaque internal "kix.*" identifier that cannot be decoded into a character
// range via any public API. We locate comments by searching their quotedFileContent
// text instead. An anchor is only emitted when the text is unambiguous (exactly
// one match); otherwise the comment is placed in AmbiguousIDs or DeletedIDs.
func BuildAnchorResult(body *docs.Body, comments []Comment) AnchorResult {
	res := AnchorResult{Offsets: make(map[int]string)}
	if body == nil {
		for _, c := range comments {
			res.AmbiguousIDs = append(res.AmbiguousIDs, c.ID)
		}
		return res
	}

	// Build paragraph index once.
	type para struct {
		segs     []textSeg
		paraText string
	}
	var paras []para
	for _, elem := range body.Content {
		if elem.Paragraph == nil {
			continue
		}
		var segs []textSeg
		var sb strings.Builder
		for _, pe := range elem.Paragraph.Elements {
			if pe.TextRun != nil && pe.TextRun.Content != "" {
				segs = append(segs, textSeg{int(pe.StartIndex), pe.TextRun.Content})
				sb.WriteString(pe.TextRun.Content)
			}
		}
		paras = append(paras, para{segs, sb.String()})
	}

	// offsets collects (offset, commentID) pairs for anchored comments so we can
	// sort them into document order afterwards.
	type anchoredEntry struct {
		offset int
		id     string
	}
	var anchored []anchoredEntry

	for _, c := range comments {
		if c.ID == "" {
			continue
		}
		if c.QuotedText == "" {
			// No quoted text — can't determine location.
			res.AmbiguousIDs = append(res.AmbiguousIDs, c.ID)
			continue
		}
		quoted := stripHTML(c.QuotedText)
		if quoted == "" {
			res.AmbiguousIDs = append(res.AmbiguousIDs, c.ID)
			continue
		}

		var matches []int
		for _, p := range paras {
			haystack := p.paraText
			searchFrom := 0
			for {
				idx := strings.Index(haystack[searchFrom:], quoted)
				if idx < 0 {
					break
				}
				abs := absOffset(p.segs, searchFrom+idx)
				if abs >= 0 {
					matches = append(matches, abs)
				}
				searchFrom += idx + 1
			}
		}

		switch len(matches) {
		case 0:
			res.DeletedIDs = append(res.DeletedIDs, c.ID)
		case 1:
			res.Offsets[matches[0]] = c.ID
			anchored = append(anchored, anchoredEntry{matches[0], c.ID})
		default:
			log.Printf("Comment %s: quoted text appears %d times; anchor skipped (ambiguous)", c.ID, len(matches))
			res.AmbiguousIDs = append(res.AmbiguousIDs, c.ID)
		}
	}

	// Return anchored IDs in document order (ascending offset).
	sort.Slice(anchored, func(i, j int) bool { return anchored[i].offset < anchored[j].offset })
	for _, e := range anchored {
		res.AnchoredIDs = append(res.AnchoredIDs, e.id)
	}

	return res
}

// absOffset maps a byte position within the concatenated paragraph text back to
// an absolute document character offset using the segment table.
func absOffset(segs []textSeg, bytePos int) int {
	pos := 0
	for _, s := range segs {
		end := pos + len(s.text)
		if end > bytePos {
			return s.start + (bytePos - pos)
		}
		pos = end
	}
	return -1
}
