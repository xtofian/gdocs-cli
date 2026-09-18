package markdown

import (
	"strings"

	"google.golang.org/api/docs/v1"
)

func formatHeadingWithAnchor(text string, level int, headingId string) string {
	suffix := ""
	if headingId != "" {
		suffix = " {#" + headingId + "}"
	}
	prefix := strings.Repeat("#", level)
	return prefix + " " + text + suffix + "\n\n"
}

// convertParagraphWithFootnotes is the full-featured paragraph converter with
// anchor and footnote reference support.
func convertParagraphWithFootnotes(paragraph *docs.Paragraph, style *docs.ParagraphStyle, anchors map[int][]string, registerFootnote func(id string), registerComment func(id string)) string {
	if paragraph == nil {
		return ""
	}

	text := convertElementsWithFootnotes(paragraph.Elements, anchors, registerFootnote, registerComment)
	text = strings.TrimRight(text, "\n")

	if text == "" {
		return "\n"
	}

	if style != nil && style.NamedStyleType != "" {
		headingID := style.HeadingId
		switch style.NamedStyleType {
		case "TITLE":
			return formatHeadingWithAnchor(text, 1, headingID)
		case "SUBTITLE":
			return formatHeadingWithAnchor(text, 2, headingID)
		case "HEADING_1":
			return formatHeadingWithAnchor(text, 1, headingID)
		case "HEADING_2":
			return formatHeadingWithAnchor(text, 2, headingID)
		case "HEADING_3":
			return formatHeadingWithAnchor(text, 3, headingID)
		case "HEADING_4":
			return formatHeadingWithAnchor(text, 4, headingID)
		case "HEADING_5":
			return formatHeadingWithAnchor(text, 5, headingID)
		case "HEADING_6":
			return formatHeadingWithAnchor(text, 6, headingID)
		}
	}

	if paragraph.Bullet != nil {
		return convertListItem(text, paragraph.Bullet)
	}

	return text + "\n\n"
}

// ConvertParagraph converts a Google Docs paragraph to markdown.
func ConvertParagraph(paragraph *docs.Paragraph, style *docs.ParagraphStyle) string {
	if paragraph == nil {
		return ""
	}

	// Get the text content
	text := ConvertParagraphElements(paragraph.Elements)
	// Remove trailing newlines for cleaner output
	text = strings.TrimRight(text, "\n")

	// If paragraph is empty, return blank line
	if text == "" {
		return "\n"
	}

	// Handle headings
	if style != nil && style.NamedStyleType != "" {
		headingID := style.HeadingId
		switch style.NamedStyleType {
		case "TITLE":
			return formatHeadingWithAnchor(text, 1, headingID)
		case "SUBTITLE":
			return formatHeadingWithAnchor(text, 2, headingID)
		case "HEADING_1":
			return formatHeadingWithAnchor(text, 1, headingID)
		case "HEADING_2":
			return formatHeadingWithAnchor(text, 2, headingID)
		case "HEADING_3":
			return formatHeadingWithAnchor(text, 3, headingID)
		case "HEADING_4":
			return formatHeadingWithAnchor(text, 4, headingID)
		case "HEADING_5":
			return formatHeadingWithAnchor(text, 5, headingID)
		case "HEADING_6":
			return formatHeadingWithAnchor(text, 6, headingID)
		}
	}

	// Handle lists
	if paragraph.Bullet != nil {
		return convertListItem(text, paragraph.Bullet)
	}

	// Regular paragraph
	return text + "\n\n"
}

// convertListItem converts a list item to markdown.
func convertListItem(text string, bullet *docs.Bullet) string {
	// Get nesting level (0-8)
	nestingLevel := bullet.NestingLevel

	// Create indentation (2 spaces per level)
	indent := strings.Repeat("  ", int(nestingLevel))

	// Determine bullet type
	bulletChar := "- "
	if bullet.ListId != "" {
		// Check glyph type if available
		// For simplicity, we'll use "-" for bullets and "1." for numbered
		// In a more complete implementation, we'd track list properties
		// from the document.Lists map
		bulletChar = "- "
	}

	return indent + bulletChar + text + "\n"
}

// ConvertTable converts a Google Docs table to markdown.
func ConvertTable(table *docs.Table) string {
	if table == nil || len(table.TableRows) == 0 {
		return ""
	}

	var builder strings.Builder

	// Process each row
	for i, row := range table.TableRows {
		// Process each cell
		builder.WriteString("|")
		for _, cell := range row.TableCells {
			cellText := extractTableCellText(cell)
			builder.WriteString(" ")
			builder.WriteString(cellText)
			builder.WriteString(" |")
		}
		builder.WriteString("\n")

		// Add separator after first row (header)
		if i == 0 {
			builder.WriteString("|")
			for range row.TableCells {
				builder.WriteString("---|")
			}
			builder.WriteString("\n")
		}
	}

	builder.WriteString("\n")
	return builder.String()
}

// extractTableCellText extracts plain text from a table cell.
func extractTableCellText(cell *docs.TableCell) string {
	if cell == nil || len(cell.Content) == 0 {
		return ""
	}

	var builder strings.Builder
	for _, element := range cell.Content {
		if element.Paragraph != nil {
			text := ConvertParagraphElements(element.Paragraph.Elements)
			text = strings.TrimSpace(text)
			// Replace newlines with spaces for single-line cell content
			text = strings.ReplaceAll(text, "\n", " ")
			// A literal "|" would end the cell early. This is the one
			// metacharacter whose meaning depends on being inside a table, so
			// it is escaped here rather than in escapeMarkdown.
			text = strings.ReplaceAll(text, "|", `\|`)
			builder.WriteString(text)
		}
	}

	return builder.String()
}
