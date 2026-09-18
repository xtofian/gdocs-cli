package markdown

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"google.golang.org/api/docs/v1"
)

// sameRenderedStyle reports whether two text styles produce the same markdown
// markup. Only the properties this converter actually emits count: two runs
// that differ solely in, say, font size are indistinguishable in the output.
func sameRenderedStyle(a, b *docs.TextStyle) bool {
	if a == nil {
		a = &docs.TextStyle{}
	}
	if b == nil {
		b = &docs.TextStyle{}
	}
	if a.Bold != b.Bold || a.Italic != b.Italic || a.Strikethrough != b.Strikethrough {
		return false
	}
	if linkURL(a) != linkURL(b) {
		return false
	}
	return isMonospaceStyle(a) == isMonospaceStyle(b)
}

func linkURL(s *docs.TextStyle) string {
	if s.Link == nil {
		return ""
	}
	return s.Link.Url
}

func isMonospaceStyle(s *docs.TextStyle) bool {
	return s.WeightedFontFamily != nil && isMonospaceFont(s.WeightedFontFamily.FontFamily)
}

// coalesceRuns merges consecutive text runs that render identically.
//
// Google Docs splits a single styled phrase into several runs for reasons that
// have nothing to do with how it looks — an edit boundary, a spell-check span,
// a font-size tweak. Emitting one delimiter pair per run turns a continuous
// italic phrase into "*continuous* assurance at *scale*", which is both ugly
// and a different document from "*continuous assurance at scale*" as far as any
// consumer is concerned. Runs are contiguous, so the merged element keeps the
// first run's StartIndex and comment-anchor offsets stay valid; an anchor that
// genuinely falls inside the phrase still splits it, in textRunWithAnchors.
func coalesceRuns(elements []*docs.ParagraphElement) []*docs.ParagraphElement {
	merged := make([]*docs.ParagraphElement, 0, len(elements))
	for _, element := range elements {
		if element.TextRun != nil && len(merged) > 0 {
			prev := merged[len(merged)-1]
			if prev.TextRun != nil && sameRenderedStyle(prev.TextRun.TextStyle, element.TextRun.TextStyle) {
				combined := *prev.TextRun
				combined.Content = prev.TextRun.Content + element.TextRun.Content
				joined := *prev
				joined.TextRun = &combined
				merged[len(merged)-1] = &joined
				continue
			}
		}
		merged = append(merged, element)
	}
	return merged
}

// convertElementsWithFootnotes processes paragraph elements with anchor and
// footnote reference support. registerFootnote may be nil to skip footnotes.
func convertElementsWithFootnotes(elements []*docs.ParagraphElement, anchors map[int][]string, registerFootnote func(id string), registerComment func(id string)) string {
	var builder strings.Builder
	for _, element := range coalesceRuns(elements) {
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

func isMonospaceFont(fontFamily string) bool {
	switch strings.ToLower(fontFamily) {
	case "consolas", "courier new", "inconsolata", "roboto mono", "source code pro", "pt mono":
		return true
	}
	return false
}

// markdownMetacharacters are the inline metacharacters this converter itself
// emits, and therefore the ones a literal occurrence in the document could be
// mistaken for.
const markdownMetacharacters = "\\`*_[]<~"

// escapeMarkdown makes text safe to emit as markdown inline content.
//
// Without it a literal "*" or "`" typed into the document comes back out as
// markup: a Doc reading `*continuous assurance at scale*` is indistinguishable
// from one where that phrase is genuinely italic, so any tool that parses the
// output — or diffs it against the document to sync edits back — silently
// corrupts the document.
//
// Code-span content is exempt (see wrapInBackticks): everything between
// backticks is already literal, so escaping there would emit real backslashes.
func escapeMarkdown(text string) string {
	if !strings.ContainsAny(text, markdownMetacharacters) {
		return text
	}
	runes := []rune(text)
	var b strings.Builder
	b.Grow(len(text) + 8)
	for i, r := range runes {
		switch r {
		case '\\', '`', '*', '[', ']', '<', '~':
			b.WriteByte('\\')
		case '_':
			// CommonMark never reads an underscore flanked by alphanumerics as
			// emphasis, so snake_case identifiers and URLs are left alone.
			intraword := i > 0 && isAlphanumeric(runes[i-1]) &&
				i+1 < len(runes) && isAlphanumeric(runes[i+1])
			if !intraword {
				b.WriteByte('\\')
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isAlphanumeric(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func isMarkdownSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// splitSurroundingSpace splits text into leading whitespace, the non-blank
// core, and trailing whitespace. All four whitespace bytes it recognizes are
// ASCII, so they can never appear inside a multi-byte UTF-8 sequence.
func splitSurroundingSpace(text string) (leading, core, trailing string) {
	start := 0
	for start < len(text) && isMarkdownSpace(text[start]) {
		start++
	}
	end := len(text)
	for end > start && isMarkdownSpace(text[end-1]) {
		end--
	}
	return text[:start], text[start:end], text[end:]
}

// wrapDelimited wraps text in a markdown delimiter, keeping any leading or
// trailing whitespace outside it.
//
// Markdown emphasis delimiters have to sit flush against non-space text.
// A run that carries its styling across a trailing space or across the
// paragraph mark would otherwise produce "** **" or "**AI.\n**", neither of
// which is emphasis — they are literal asterisks, and the second one also
// breaks the paragraph. A run that is nothing but whitespace gets no
// delimiters at all.
func wrapDelimited(text, delim string) string {
	leading, core, trailing := splitSurroundingSpace(text)
	if core == "" {
		return text
	}
	return leading + delim + core + delim + trailing
}

// wrapInBackticks renders text as a code span, widening the fence when the
// content itself contains backticks and padding when it starts or ends with
// one, per the CommonMark code-span rules.
func wrapInBackticks(text string) string {
	leading, core, trailing := splitSurroundingSpace(text)
	if core == "" {
		return text
	}
	fence := "`"
	for strings.Contains(core, fence) {
		fence += "`"
	}
	pad := ""
	if strings.HasPrefix(core, "`") || strings.HasSuffix(core, "`") {
		pad = " "
	}
	return leading + fence + pad + core + pad + fence + trailing
}

// ApplyTextStyle applies markdown formatting to text based on TextStyle.
func ApplyTextStyle(text string, style *docs.TextStyle) string {
	if style == nil {
		return escapeMarkdown(text)
	}

	// Apply monospace/code style first. A code span is literal, so its content
	// is fenced rather than escaped; everything else has to be escaped before
	// any delimiter is added around it.
	if style.WeightedFontFamily != nil && isMonospaceFont(style.WeightedFontFamily.FontFamily) {
		text = wrapInBackticks(text)
	} else {
		text = escapeMarkdown(text)
	}

	// Handle links
	if style.Link != nil && style.Link.Url != "" {
		text = formatLink(text, style.Link.Url)
	}

	// Handle bold and italic
	// Check both combinations to apply correct markdown syntax
	if style.Bold && style.Italic {
		text = wrapDelimited(text, "***")
	} else if style.Bold {
		text = wrapDelimited(text, "**")
	} else if style.Italic {
		text = wrapDelimited(text, "*")
	}

	// Handle strikethrough
	if style.Strikethrough {
		text = wrapDelimited(text, "~~")
	}

	return text
}

// formatLink creates a markdown link from text and URL.
func formatLink(text string, url string) string {
	// Remove trailing newlines from link text for cleaner markdown
	text = strings.TrimRight(text, "\n")
	return "[" + text + "](" + formatLinkDestination(url) + ")"
}

// formatLinkDestination renders a URL as a markdown link destination. A bare
// destination cannot contain whitespace or unescaped angle brackets, and a ")"
// would close the link early, so those URLs use the pointy-bracket form.
func formatLinkDestination(url string) string {
	if !strings.ContainsAny(url, " \t\n<>()") {
		return url
	}
	replacer := strings.NewReplacer("<", `\<`, ">", `\>`)
	return "<" + replacer.Replace(url) + ">"
}

// ConvertParagraphElements converts all paragraph elements to markdown text.
func ConvertParagraphElements(elements []*docs.ParagraphElement) string {
	var builder strings.Builder

	for _, element := range coalesceRuns(elements) {
		if element.TextRun != nil {
			builder.WriteString(ConvertTextRun(element.TextRun))
		}
		// Handle other element types if needed (e.g., InlineObject, PageBreak)
	}

	return builder.String()
}

// anchorOffsetsIn returns the anchored offsets in [start, end), in order.
func anchorOffsetsIn(anchors map[int][]string, start, end int) []int {
	var offsets []int
	for offset := range anchors {
		if offset >= start && offset < end {
			offsets = append(offsets, offset)
		}
	}
	sort.Ints(offsets)
	return offsets
}

// textRunWithAnchors renders a text run, injecting HTML comment anchor markers
// at any character offsets within the run that map to a comment ID.
func textRunWithAnchors(pe *docs.ParagraphElement, anchors map[int][]string, registerComment func(id string)) string {
	tr := pe.TextRun
	if tr == nil || tr.Content == "" {
		return ""
	}

	start := int(pe.StartIndex)
	content := tr.Content
	cLen := utf16Len(content)

	offsets := anchorOffsetsIn(anchors, start, start+cLen)
	if len(offsets) == 0 {
		return ApplyTextStyle(content, tr.TextStyle)
	}

	var result strings.Builder
	prevUTF16 := 0
	for _, offset := range offsets {
		local := offset - start
		if local > prevUTF16 {
			prevByte := utf16ToByteIndex(content, prevUTF16)
			posByte := utf16ToByteIndex(content, local)
			result.WriteString(ApplyTextStyle(content[prevByte:posByte], tr.TextStyle))
		}
		for _, id := range anchors[offset] {
			result.WriteString(fmt.Sprintf("<!-- gdoc-comment: %s -->", id))
			if registerComment != nil {
				registerComment(id)
			}
		}
		prevUTF16 = local
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
