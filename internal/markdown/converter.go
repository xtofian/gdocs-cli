package markdown

import (
	"fmt"
	"strings"

	"github.com/famasya/gdocs-cli/internal/gdocs"
	"google.golang.org/api/docs/v1"
)

// Converter handles the conversion of Google Docs to markdown.
type Converter struct {
	doc           *docs.Document
	body          *docs.Body
	title         string
	tabName       string
	comments      []gdocs.Comment
	commentMap    map[string]gdocs.Comment // Maps comment ID -> Comment
	anchorOffsets map[int]string           // absolute char offset → comment ID
	anchoredIDs   map[string]bool          // comment IDs with a unique body anchor
	ambiguousIDs  map[string]bool          // comment IDs with repeated/missing quoted text
	deletedIDs    map[string]bool          // comment IDs on text no longer in the document
	anchorOrder   []string                 // anchored comment IDs in document order
	footnoteMap   map[string]docs.Footnote // footnote ID → content
	footnoteOrder []string                 // footnote IDs in document order (preserved for compatibility/legacy, though not used in new design)
	footnoteSeen  map[string]bool          // tracks which footnote IDs have been registered
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
func (c *Converter) SetComments(comments []gdocs.Comment) {
	c.comments = comments
	c.commentMap = make(map[string]gdocs.Comment, len(comments))
	for _, cm := range comments {
		c.commentMap[cm.ID] = cm
	}
	res := gdocs.BuildAnchorResult(c.body, comments)
	c.anchorOffsets = res.Offsets
	c.anchorOrder = res.AnchoredIDs
	c.anchoredIDs = toIDSet(res.AnchoredIDs)
	c.ambiguousIDs = toIDSet(res.AmbiguousIDs)
	c.deletedIDs = toIDSet(res.DeletedIDs)
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

	// Append remaining comments (ambiguous and deleted) if present.
	if len(c.comments) > 0 {
		_, ambiguous, deleted := c.splitComments()
		if len(ambiguous) > 0 || len(deleted) > 0 {
			bodyStr := builder.String()
			bodyStr = strings.TrimRight(bodyStr, " \t\r\n")
			builder.Reset()
			builder.WriteString(bodyStr)
			builder.WriteString("\n\n\n") // Two blank lines before "## Comments"
			builder.WriteString(ConvertComments(nil, ambiguous, deleted))
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

// splitComments partitions c.comments into three ordered slices:
// anchored (in document order), ambiguous, and deleted.
func (c *Converter) splitComments() (anchored, ambiguous, deleted []gdocs.Comment) {
	byID := make(map[string]gdocs.Comment, len(c.comments))
	for _, cm := range c.comments {
		byID[cm.ID] = cm
	}

	// Anchored: use the document-order ID list from anchor resolution.
	for _, id := range c.anchorOrder {
		if cm, ok := byID[id]; ok {
			anchored = append(anchored, cm)
		}
	}

	// Ambiguous and deleted: preserve the order they appear in c.comments.
	for _, cm := range c.comments {
		if c.ambiguousIDs[cm.ID] {
			ambiguous = append(ambiguous, cm)
		} else if c.deletedIDs[cm.ID] {
			deleted = append(deleted, cm)
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

	for _, element := range c.body.Content {
		if element.Paragraph != nil {
			// Check if this paragraph is a heading.
			// If it's a heading and the current section has content, start a new section.
			if isHeading(element.Paragraph) {
				if currSection.content.Len() > 0 {
					currSection = newSection()
					sections = append(sections, currSection)
				}
			}

			// Capture the current section in callbacks
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

			markdown := convertParagraphWithFootnotes(element.Paragraph, element.Paragraph.ParagraphStyle, c.anchorOffsets, registerFootnote, registerComment)
			currSection.content.WriteString(markdown)
		} else if element.Table != nil {
			markdown := ConvertTable(element.Table)
			currSection.content.WriteString(markdown)
		}
	}

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
