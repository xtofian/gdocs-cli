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
	anchorOffsets map[int]string           // absolute char offset → comment ID
	anchoredIDs   map[string]bool          // comment IDs with a unique body anchor
	ambiguousIDs  map[string]bool          // comment IDs with repeated/missing quoted text
	deletedIDs    map[string]bool          // comment IDs on text no longer in the document
	anchorOrder   []string                 // anchored comment IDs in document order
	footnoteMap   map[string]docs.Footnote // footnote ID → content
	footnoteOrder []string                 // footnote IDs in document order
	footnoteSeen  map[string]bool          // tracks which footnote IDs have been registered
}

// NewConverter creates a new Converter for the given document.
// Uses the first tab's content by default.
func NewConverter(doc *docs.Document) *Converter {
	c := &Converter{doc: doc, title: doc.Title, footnoteSeen: make(map[string]bool)}

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
	c := &Converter{doc: doc, title: doc.Title, footnoteSeen: make(map[string]bool)}

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

	// Append footnotes if any were encountered during body conversion.
	builder.WriteString(c.buildFootnotes())

	// Append comments if present, split into three groups.
	if len(c.comments) > 0 {
		anchored, ambiguous, deleted := c.splitComments()
		builder.WriteString("\n")
		builder.WriteString(ConvertComments(anchored, ambiguous, deleted))
	}

	return builder.String(), nil
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

// registerFootnote records a footnote ID the first time it is encountered,
// preserving document order for the definitions section.
func (c *Converter) registerFootnote(id string) {
	if !c.footnoteSeen[id] {
		c.footnoteSeen[id] = true
		c.footnoteOrder = append(c.footnoteOrder, id)
	}
}

// buildFootnotes emits the markdown footnote definitions for any footnotes
// collected during body conversion.
func (c *Converter) buildFootnotes() string {
	if len(c.footnoteOrder) == 0 || c.footnoteMap == nil {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("\n")
	for _, id := range c.footnoteOrder {
		footnote, ok := c.footnoteMap[id]
		if !ok {
			continue
		}
		content := convertFootnoteContent(footnote.Content)
		builder.WriteString(fmt.Sprintf("[^%s]: %s\n", id, content))
	}
	return builder.String()
}

// convertBody converts the document body to markdown.
func (c *Converter) convertBody() string {
	var builder strings.Builder

	for _, element := range c.body.Content {
		// Convert based on element type
		if element.Paragraph != nil {
			markdown := convertParagraphWithFootnotes(element.Paragraph, element.Paragraph.ParagraphStyle, c.anchorOffsets, c.registerFootnote)
			builder.WriteString(markdown)
		} else if element.Table != nil {
			markdown := ConvertTable(element.Table)
			builder.WriteString(markdown)
		}
		// Other structural elements can be added here (e.g., SectionBreak, TableOfContents)
	}

	return builder.String()
}
