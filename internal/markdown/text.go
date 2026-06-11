package markdown

import (
	"fmt"
	"sort"
	"strings"

	"google.golang.org/api/docs/v1"
)

// convertElementsWithFootnotes processes paragraph elements with anchor and
// footnote reference support. registerFootnote may be nil to skip footnotes.
func convertElementsWithFootnotes(elements []*docs.ParagraphElement, anchors map[int]string, registerFootnote func(id string), registerComment func(id string)) string {
	var builder strings.Builder
	for _, element := range elements {
		if element.TextRun != nil {
			if len(anchors) > 0 {
				builder.WriteString(textRunWithAnchors(element, anchors, registerComment))
			} else {
				builder.WriteString(ConvertTextRun(element.TextRun))
			}
		} else if element.FootnoteReference != nil && registerFootnote != nil {
			id := element.FootnoteReference.FootnoteId
			registerFootnote(id)
			builder.WriteString(fmt.Sprintf("[^%s]", id))
		}
	}
	return builder.String()
}

// convertFootnoteContent extracts plain text from a footnote's structural elements.
func convertFootnoteContent(content []*docs.StructuralElement) string {
	var parts []string
	for _, elem := range content {
		if elem.Paragraph != nil {
			text := ConvertParagraphElements(elem.Paragraph.Elements)
			text = strings.TrimSpace(text)
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, " ")
}

// ConvertTextRun converts a Google Docs TextRun to markdown with formatting.
func ConvertTextRun(textRun *docs.TextRun) string {
	if textRun == nil || textRun.Content == "" {
		return ""
	}

	text := textRun.Content
	style := textRun.TextStyle

	return ApplyTextStyle(text, style)
}

// ApplyTextStyle applies markdown formatting to text based on TextStyle.
func ApplyTextStyle(text string, style *docs.TextStyle) string {
	if style == nil {
		return text
	}

	// Handle links
	if style.Link != nil && style.Link.Url != "" {
		text = formatLink(text, style.Link.Url)
	}

	// Handle bold and italic
	// Check both combinations to apply correct markdown syntax
	if style.Bold && style.Italic {
		text = "***" + text + "***"
	} else if style.Bold {
		text = "**" + text + "**"
	} else if style.Italic {
		text = "*" + text + "*"
	}

	// Handle strikethrough
	if style.Strikethrough {
		text = "~~" + text + "~~"
	}

	return text
}

// formatLink creates a markdown link from text and URL.
func formatLink(text string, url string) string {
	// Remove trailing newlines from link text for cleaner markdown
	text = strings.TrimRight(text, "\n")
	return "[" + text + "](" + url + ")"
}

// ConvertParagraphElements converts all paragraph elements to markdown text.
func ConvertParagraphElements(elements []*docs.ParagraphElement) string {
	var builder strings.Builder

	for _, element := range elements {
		if element.TextRun != nil {
			builder.WriteString(ConvertTextRun(element.TextRun))
		}
		// Handle other element types if needed (e.g., InlineObject, PageBreak)
	}

	return builder.String()
}

// convertParagraphElementsInternal is the anchor-aware version of ConvertParagraphElements.
func convertParagraphElementsInternal(elements []*docs.ParagraphElement, anchors map[int]string) string {
	if len(anchors) == 0 {
		return ConvertParagraphElements(elements)
	}
	var builder strings.Builder
	for _, element := range elements {
		if element.TextRun != nil {
			builder.WriteString(textRunWithAnchors(element, anchors, nil))
		}
	}
	return builder.String()
}

// textRunWithAnchors renders a text run, injecting HTML comment anchor markers
// at any character offsets within the run that map to a comment ID.
func textRunWithAnchors(pe *docs.ParagraphElement, anchors map[int]string, registerComment func(id string)) string {
	tr := pe.TextRun
	if tr == nil || tr.Content == "" {
		return ""
	}

	start := int(pe.StartIndex)
	content := tr.Content
	cLen := utf16Len(content)

	type anchorPos struct {
		local int
		id    string
	}
	var positions []anchorPos
	for offset, id := range anchors {
		if offset >= start && offset < start+cLen {
			positions = append(positions, anchorPos{offset - start, id})
		}
	}

	if len(positions) == 0 {
		return ApplyTextStyle(content, tr.TextStyle)
	}

	sort.Slice(positions, func(i, j int) bool { return positions[i].local < positions[j].local })

	var result strings.Builder
	prevUTF16 := 0
	for _, pos := range positions {
		if pos.local > prevUTF16 {
			prevByte := utf16ToByteIndex(content, prevUTF16)
			posByte := utf16ToByteIndex(content, pos.local)
			result.WriteString(ApplyTextStyle(content[prevByte:posByte], tr.TextStyle))
		}
		result.WriteString(fmt.Sprintf("<!-- gdoc-comment: %s -->", pos.id))
		if registerComment != nil {
			registerComment(pos.id)
		}
		prevUTF16 = pos.local
	}
	if prevUTF16 < cLen {
		prevByte := utf16ToByteIndex(content, prevUTF16)
		result.WriteString(ApplyTextStyle(content[prevByte:], tr.TextStyle))
	}
	return result.String()
}

func utf16Len(s string) int {
	uLen := 0
	for _, r := range s {
		if r >= 0x10000 {
			uLen += 2
		} else {
			uLen += 1
		}
	}
	return uLen
}

func utf16ToByteIndex(s string, utf16Idx int) int {
	if utf16Idx <= 0 {
		return 0
	}
	currentUTF16 := 0
	for byteIdx, r := range s {
		if currentUTF16 >= utf16Idx {
			return byteIdx
		}
		if r >= 0x10000 {
			currentUTF16 += 2
		} else {
			currentUTF16 += 1
		}
	}
	return len(s)
}
