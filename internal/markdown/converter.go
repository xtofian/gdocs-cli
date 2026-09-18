package markdown

import (
	"fmt"
	"strings"

	"github.com/famasya/gdocs-cli/internal/gdocs"
	"google.golang.org/api/docs/v1"
)

// Converter handles the conversion of Google Docs to markdown.
type Converter struct {
	doc              *docs.Document
	body             *docs.Body
	title            string
	tabName          string
	comments         []gdocs.Comment
	commentMap       map[string]gdocs.Comment // Maps comment ID -> Comment
	anchorOffsets    map[int][]string         // absolute char offset → comment IDs
	anchoredIDs      map[string]bool          // comment IDs anchored in the body
	anchorOrder      []string                 // anchored comment IDs in document order
	footnoteMap      map[string]docs.Footnote // footnote ID → content
	footnoteOrder    []string                 // footnote IDs in document order (preserved for compatibility/legacy, though not used in new design)
	footnoteSeen     map[string]bool          // tracks which footnote IDs have been registered
	openCommentsOnly bool                     // whether --comments=open mode is active
}

type section struct {
	content       strings.Builder
	footnoteOrder []string
	footnoteSeen  map[string]bool
	commentOrder  []string
	commentSeen   map[string]bool
}

func newSection() *section {
	return &section{
		footnoteSeen: make(map[string]bool),
		commentSeen:  make(map[string]bool),
	}
}

func isHeading(p *docs.Paragraph) bool {
	if p == nil || p.ParagraphStyle == nil {
		return false
	}
	switch p.ParagraphStyle.NamedStyleType {
	case "TITLE", "SUBTITLE", "HEADING_1", "HEADING_2", "HEADING_3", "HEADING_4", "HEADING_5", "HEADING_6":
		// Check if it actually has text content
		var textBuilder strings.Builder
		for _, el := range p.Elements {
			if el.TextRun != nil {
				textBuilder.WriteString(el.TextRun.Content)
			}
		}
		return strings.TrimSpace(textBuilder.String()) != ""
	}
	return false
}

// NewConverter creates a new Converter for the given document.
// Uses the first tab's content by default.
func NewConverter(doc *docs.Document) *Converter {
	c := &Converter{
		doc:          doc,
		title:        doc.Title,
		footnoteSeen: make(map[string]bool),
		commentMap:   make(map[string]gdocs.Comment),
	}

	// Use tab content if available, otherwise fall back to legacy doc.Body
	if doc.Tabs != nil && len(doc.Tabs) > 0 {
		tab := doc.Tabs[0]
		if tab.DocumentTab != nil {
			c.body = tab.DocumentTab.Body
			c.footnoteMap = tab.DocumentTab.Footnotes
		}
		if tab.TabProperties != nil {
			c.tabName = tab.TabProperties.Title
		}
	} else if doc.Body != nil {
		c.body = doc.Body
		c.footnoteMap = doc.Footnotes
	}

	return c
}

// NewConverterFromTab creates a new Converter for a specific tab.
func NewConverterFromTab(doc *docs.Document, tab *docs.Tab) *Converter {
	c := &Converter{
		doc:          doc,
		title:        doc.Title,
		footnoteSeen: make(map[string]bool),
		commentMap:   make(map[string]gdocs.Comment),
	}

	if tab != nil && tab.DocumentTab != nil {
		c.body = tab.DocumentTab.Body
		c.footnoteMap = tab.DocumentTab.Footnotes
		if tab.TabProperties != nil {
			c.tabName = tab.TabProperties.Title
		}
	}

	return c
}

// SetComments sets the comments and resolves their anchor positions in the document body.
func (c *Converter) SetComments(comments []gdocs.Comment, mobileBasicHTML string) {
	c.comments = comments
	c.commentMap = make(map[string]gdocs.Comment, len(comments))
	for _, cm := range comments {
		c.commentMap[cm.ID] = cm
	}
	res := gdocs.BuildAnchorResult(c.body, comments, mobileBasicHTML)
	c.anchorOffsets = res.Offsets
	c.anchorOrder = res.AnchoredIDs
	c.anchoredIDs = toIDSet(res.AnchoredIDs)
}

// SetOpenCommentsOnly sets whether --comments=open mode is active.
func (c *Converter) SetOpenCommentsOnly(val bool) {
	c.openCommentsOnly = val
}

func toIDSet(ids []string) map[string]bool {
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

// Convert processes the entire document and returns markdown.
func (c *Converter) Convert() (string, error) {
	var builder strings.Builder

	// Generate frontmatter
	frontmatter, err := c.generateFrontmatter()
	if err != nil {
		return "", fmt.Errorf("failed to generate frontmatter: %w", err)
	}
	builder.WriteString(frontmatter)
	builder.WriteString("\n")

	// Convert body content
	if c.body != nil && c.body.Content != nil {
		body := c.convertBody()
		builder.WriteString(body)
	}

	// Append the comments that could not be anchored in the body.
	if len(c.comments) > 0 {
		_, unattached := c.splitComments()
		if len(unattached) > 0 {
			bodyStr := builder.String()
			bodyStr = strings.TrimRight(bodyStr, " \t\r\n")
			builder.Reset()
			builder.WriteString(bodyStr)
			builder.WriteString("\n\n\n") // Two blank lines before "## Comments"
			builder.WriteString(ConvertComments(unattached))
		}
	}

	// Trim trailing newlines/spaces from the final output and add exactly one trailing newline.
	res := builder.String()
	res = strings.TrimRight(res, " \t\r\n") + "\n"

	return res, nil
}

// generateFrontmatter creates frontmatter including tab info if present.
func (c *Converter) generateFrontmatter() (string, error) {
	// Use the existing GenerateFrontmatter for the base, but we'll
	// add tab info if we have it
	frontmatter, err := GenerateFrontmatter(c.doc)
	if err != nil {
		return "", err
	}

	// If we have a tab name that differs from the doc title, include it
	if c.tabName != "" && c.tabName != c.title {
		// Insert tab info before the closing ---
		frontmatter = strings.TrimSuffix(frontmatter, "---\n")
		frontmatter += fmt.Sprintf("tab: %s\n---\n", c.tabName)
	}

	return frontmatter, nil
}

// splitComments partitions c.comments into the ones anchored in the body, in
// document order, and the rest, in the order they were fetched.
func (c *Converter) splitComments() (anchored, unattached []gdocs.Comment) {
	byID := make(map[string]gdocs.Comment, len(c.comments))
	for _, cm := range c.comments {
		byID[cm.ID] = cm
	}

	for _, id := range c.anchorOrder {
		if cm, ok := byID[id]; ok {
			anchored = append(anchored, cm)
		}
	}
	for _, cm := range c.comments {
		if !c.anchoredIDs[cm.ID] {
			unattached = append(unattached, cm)
		}
	}
	return
}

// convertBody converts the document body to markdown, partitioning it into sections by headings.
func (c *Converter) convertBody() string {
	var builder strings.Builder

	var sections []*section
	currSection := newSection()
	sections = append(sections, currSection)

	var activeCodeLines []string
	var pendingEmptyParas []*docs.Paragraph

	flushActiveCodeBlock := func(sec *section) {
		if len(activeCodeLines) > 0 {
			var sb strings.Builder
			for _, line := range activeCodeLines {
				sb.WriteString(line)
			}
			content := sb.String()
			content = strings.TrimRight(content, " \t\r\n")
			if content != "" {
				sec.content.WriteString("```\n" + content + "\n```\n\n")
			}
			activeCodeLines = nil
		}
		for range pendingEmptyParas {
			sec.content.WriteString("\n")
		}
		pendingEmptyParas = nil
	}

	for _, element := range c.body.Content {
		if element.Paragraph != nil {
			p := element.Paragraph

			sec := currSection
			registerFootnote := func(id string) {
				if !sec.footnoteSeen[id] {
					sec.footnoteSeen[id] = true
					sec.footnoteOrder = append(sec.footnoteOrder, id)
				}
			}

			registerComment := func(id string) {
				// Only register if it's an anchored comment
				if c.anchoredIDs[id] && !sec.commentSeen[id] {
					sec.commentSeen[id] = true
					sec.commentOrder = append(sec.commentOrder, id)
				}
			}

			if isHeading(p) {
				flushActiveCodeBlock(currSection)
				if currSection.content.Len() > 0 {
					currSection = newSection()
					sections = append(sections, currSection)
				}
				markdown := convertParagraphWithFootnotes(p, p.ParagraphStyle, c.anchorOffsets, registerFootnote, registerComment)
				currSection.content.WriteString(markdown)
				continue
			}

			if p.Bullet != nil {
				flushActiveCodeBlock(currSection)
				markdown := convertParagraphWithFootnotes(p, p.ParagraphStyle, c.anchorOffsets, registerFootnote, registerComment)
				currSection.content.WriteString(markdown)
				continue
			}

			if startsWithE907(p) || isParagraphEntirelyMonospace(p) {
				registerParagraphCommentsAndFootnotes(p, registerFootnote, registerComment, c.anchorOffsets)
				for _, ep := range pendingEmptyParas {
					activeCodeLines = append(activeCodeLines, extractParagraphRawText(ep))
				}
				pendingEmptyParas = nil
				activeCodeLines = append(activeCodeLines, extractParagraphRawText(p))
				continue
			}

			if isEmptyParagraph(p) {
				if len(activeCodeLines) > 0 {
					pendingEmptyParas = append(pendingEmptyParas, p)
				} else {
					flushActiveCodeBlock(currSection)
					markdown := convertParagraphWithFootnotes(p, p.ParagraphStyle, c.anchorOffsets, registerFootnote, registerComment)
					currSection.content.WriteString(markdown)
				}
				continue
			}

			flushActiveCodeBlock(currSection)
			markdown := convertParagraphWithFootnotes(p, p.ParagraphStyle, c.anchorOffsets, registerFootnote, registerComment)
			currSection.content.WriteString(markdown)
		} else if element.Table != nil {
			flushActiveCodeBlock(currSection)
			table := element.Table
			if len(table.TableRows) == 1 && len(table.TableRows[0].TableCells) == 1 {
				cell := table.TableRows[0].TableCells[0]
				var cellRawSb strings.Builder
				for _, se := range cell.Content {
					if se.Paragraph != nil {
						cellRawSb.WriteString(extractParagraphRawText(se.Paragraph))
					}
				}
				rawContent := cellRawSb.String()
				rawContent = strings.TrimRight(rawContent, " \t\r\n")
				if rawContent != "" {
					currSection.content.WriteString("```\n" + rawContent + "\n```\n\n")
				}
			} else {
				markdown := ConvertTable(table)
				currSection.content.WriteString(markdown)
			}
		}
	}

	flushActiveCodeBlock(currSection)

	// Now process and merge all sections.
	for _, sec := range sections {
		content := sec.content.String()
		if content == "" {
			continue
		}

		// Check if there's any footnote or comment in this section.
		hasFootnotes := len(sec.footnoteOrder) > 0
		hasComments := len(sec.commentOrder) > 0

		if hasFootnotes || hasComments {
			// Trim trailing spaces/newlines from body content to have a clean slate.
			content = strings.TrimRight(content, " \t\r\n")

			if hasFootnotes {
				// Separator between content and footnotes is two blank lines (three newlines).
				content += "\n\n\n"
				for fIdx, id := range sec.footnoteOrder {
					if fIdx > 0 {
						content += "\n"
					}
					footnote, ok := c.footnoteMap[id]
					if ok {
						fc := convertFootnoteContent(footnote.Content)
						content += fmt.Sprintf("[^%s]: %s", id, fc)
					}
				}
			}

			if hasComments {
				// Separator between footnotes/content and comments.
				// One blank line if there were footnotes.
				// Two blank lines if there were no footnotes.
				if hasFootnotes {
					content += "\n\n"
				} else {
					content += "\n\n\n"
				}

				for cIdx, id := range sec.commentOrder {
					if cIdx > 0 {
						content += "\n\n"
					}
					commentObj, ok := c.commentMap[id]
					if ok {
						content += ConvertSingleComment(commentObj)
					}
				}
			}

			// Add spacing after the section's footnotes/comments.
			// Two blank lines (three newlines) so the transition to the next heading is clean.
			content += "\n\n\n"
		}

		builder.WriteString(content)
	}

	return builder.String()
}

func startsWithE907(p *docs.Paragraph) bool {
	if p == nil || len(p.Elements) == 0 {
		return false
	}
	first := p.Elements[0]
	return first.TextRun != nil && strings.HasPrefix(first.TextRun.Content, "\ue907")
}

func isParagraphEntirelyMonospace(p *docs.Paragraph) bool {
	if p == nil || len(p.Elements) == 0 {
		return false
	}
	hasText := false
	for _, element := range p.Elements {
		if element.TextRun != nil {
			content := element.TextRun.Content
			trimmed := strings.TrimSpace(content)
			if trimmed == "" {
				continue
			}
			hasText = true
			style := element.TextRun.TextStyle
			if style == nil || style.WeightedFontFamily == nil || !isMonospaceFont(style.WeightedFontFamily.FontFamily) {
				return false
			}
		}
	}
	return hasText
}

func isEmptyParagraph(p *docs.Paragraph) bool {
	if p == nil {
		return true
	}
	var sb strings.Builder
	for _, el := range p.Elements {
		if el.TextRun != nil {
			sb.WriteString(el.TextRun.Content)
		}
	}
	return strings.TrimSpace(sb.String()) == ""
}

func extractParagraphRawText(paragraph *docs.Paragraph) string {
	if paragraph == nil {
		return ""
	}
	var sb strings.Builder
	for i, element := range paragraph.Elements {
		if element.TextRun != nil {
			content := element.TextRun.Content
			if i == 0 && strings.HasPrefix(content, "\ue907") {
				content = content[len("\ue907"):]
			}
			content = strings.ReplaceAll(content, "\u000b", "\n")
			sb.WriteString(content)
		}
	}
	return sb.String()
}

func registerParagraphCommentsAndFootnotes(paragraph *docs.Paragraph, registerFootnote func(id string), registerComment func(id string), anchors map[int][]string) {
	if paragraph == nil {
		return
	}
	for _, element := range paragraph.Elements {
		if element.FootnoteReference != nil && registerFootnote != nil {
			registerFootnote(element.FootnoteReference.FootnoteId)
		}
		if element.TextRun != nil && registerComment != nil && len(anchors) > 0 {
			start := int(element.StartIndex)
			cLen := utf16Len(element.TextRun.Content)
			for _, offset := range anchorOffsetsIn(anchors, start, start+cLen) {
				for _, id := range anchors[offset] {
					registerComment(id)
				}
			}
		}
	}
}
