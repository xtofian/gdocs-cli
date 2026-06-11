package markdown

import (
	"strings"
	"testing"

	"github.com/famasya/gdocs-cli/internal/gdocs"
	"google.golang.org/api/docs/v1"
)

func TestApplyTextStyle(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		style *docs.TextStyle
		want  string
	}{
		{
			name:  "no style",
			text:  "plain text",
			style: nil,
			want:  "plain text",
		},
		{
			name:  "bold text",
			text:  "bold text",
			style: &docs.TextStyle{Bold: true},
			want:  "**bold text**",
		},
		{
			name:  "italic text",
			text:  "italic text",
			style: &docs.TextStyle{Italic: true},
			want:  "*italic text*",
		},
		{
			name:  "bold and italic",
			text:  "bold italic",
			style: &docs.TextStyle{Bold: true, Italic: true},
			want:  "***bold italic***",
		},
		{
			name:  "strikethrough",
			text:  "strikethrough",
			style: &docs.TextStyle{Strikethrough: true},
			want:  "~~strikethrough~~",
		},
		{
			name:  "bold and strikethrough",
			text:  "text",
			style: &docs.TextStyle{Bold: true, Strikethrough: true},
			want:  "~~**text**~~",
		},
		{
			name:  "link",
			text:  "click here",
			style: &docs.TextStyle{Link: &docs.Link{Url: "https://example.com"}},
			want:  "[click here](https://example.com)",
		},
		{
			name:  "bold link",
			text:  "click here",
			style: &docs.TextStyle{Bold: true, Link: &docs.Link{Url: "https://example.com"}},
			want:  "**[click here](https://example.com)**",
		},
		{
			name:  "link with trailing newline",
			text:  "click here\n",
			style: &docs.TextStyle{Link: &docs.Link{Url: "https://example.com"}},
			want:  "[click here](https://example.com)",
		},
		{
			name:  "monospace text (Roboto Mono)",
			text:  "my_code",
			style: &docs.TextStyle{WeightedFontFamily: &docs.WeightedFontFamily{FontFamily: "Roboto Mono"}},
			want:  "`my_code`",
		},
		{
			name:  "monospace bold text (Courier New)",
			text:  "important_code",
			style: &docs.TextStyle{Bold: true, WeightedFontFamily: &docs.WeightedFontFamily{FontFamily: "Courier New"}},
			want:  "**`important_code`**",
		},
		{
			name:  "monospace text with spaces (PT Mono)",
			text:  " trimmed_code ",
			style: &docs.TextStyle{WeightedFontFamily: &docs.WeightedFontFamily{FontFamily: "PT Mono"}},
			want:  " `trimmed_code` ",
		},
		{
			name:  "monospace text with newline (Consolas)",
			text:  "\nnewline_code\n",
			style: &docs.TextStyle{WeightedFontFamily: &docs.WeightedFontFamily{FontFamily: "Consolas"}},
			want:  "\n`newline_code`\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyTextStyle(tt.text, tt.style)
			if got != tt.want {
				t.Errorf("ApplyTextStyle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConvertTextRun(t *testing.T) {
	tests := []struct {
		name    string
		textRun *docs.TextRun
		want    string
	}{
		{
			name:    "nil text run",
			textRun: nil,
			want:    "",
		},
		{
			name:    "empty content",
			textRun: &docs.TextRun{Content: ""},
			want:    "",
		},
		{
			name: "plain text",
			textRun: &docs.TextRun{
				Content:   "Hello World",
				TextStyle: &docs.TextStyle{},
			},
			want: "Hello World",
		},
		{
			name: "bold text",
			textRun: &docs.TextRun{
				Content:   "Bold",
				TextStyle: &docs.TextStyle{Bold: true},
			},
			want: "**Bold**",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertTextRun(tt.textRun)
			if got != tt.want {
				t.Errorf("ConvertTextRun() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConvertElementsWithFootnotes(t *testing.T) {
	var registered []string
	register := func(id string) { registered = append(registered, id) }

	elements := []*docs.ParagraphElement{
		{TextRun: &docs.TextRun{Content: "Hello"}},
		{FootnoteReference: &docs.FootnoteReference{FootnoteId: "fn1"}},
		{TextRun: &docs.TextRun{Content: " world"}},
		{FootnoteReference: &docs.FootnoteReference{FootnoteId: "fn2"}},
	}

	got := convertElementsWithFootnotes(elements, nil, register, nil)
	want := "Hello[^fn1] world[^fn2]"
	if got != want {
		t.Errorf("convertElementsWithFootnotes() = %q, want %q", got, want)
	}
	if len(registered) != 2 || registered[0] != "fn1" || registered[1] != "fn2" {
		t.Errorf("registered = %v, want [fn1 fn2]", registered)
	}
}

func TestConvertFootnoteContent(t *testing.T) {
	content := []*docs.StructuralElement{
		{
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{TextRun: &docs.TextRun{Content: "See example.com for details."}},
				},
			},
		},
	}
	got := convertFootnoteContent(content)
	want := "See example.com for details."
	if got != want {
		t.Errorf("convertFootnoteContent() = %q, want %q", got, want)
	}
}

func TestConverterFootnotes(t *testing.T) {
	doc := &docs.Document{
		Title: "Test",
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					Paragraph: &docs.Paragraph{
						ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "NORMAL_TEXT"},
						Elements: []*docs.ParagraphElement{
							{TextRun: &docs.TextRun{Content: "Text with footnote"}},
							{FootnoteReference: &docs.FootnoteReference{FootnoteId: "fn-a"}},
							{TextRun: &docs.TextRun{Content: " and another"}},
							{FootnoteReference: &docs.FootnoteReference{FootnoteId: "fn-b"}},
							{TextRun: &docs.TextRun{Content: ".\n"}},
						},
					},
				},
			},
		},
		Footnotes: map[string]docs.Footnote{
			"fn-a": {
				FootnoteId: "fn-a",
				Content: []*docs.StructuralElement{
					{Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{TextRun: &docs.TextRun{Content: "First footnote."}},
						},
					}},
				},
			},
			"fn-b": {
				FootnoteId: "fn-b",
				Content: []*docs.StructuralElement{
					{Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{TextRun: &docs.TextRun{Content: "Second footnote."}},
						},
					}},
				},
			},
		},
	}

	c := NewConverter(doc)
	out, err := c.Convert()
	if err != nil {
		t.Fatalf("Convert() error: %v", err)
	}

	if !contains(out, "[^fn-a]") || !contains(out, "[^fn-b]") {
		t.Errorf("output missing footnote references:\n%s", out)
	}
	if !contains(out, "[^fn-a]: First footnote.") {
		t.Errorf("output missing footnote fn-a definition:\n%s", out)
	}
	if !contains(out, "[^fn-b]: Second footnote.") {
		t.Errorf("output missing footnote fn-b definition:\n%s", out)
	}
}

func TestConverterSections(t *testing.T) {
	doc := &docs.Document{
		Title: "Test Sections",
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					Paragraph: &docs.Paragraph{
						ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_1"},
						Elements: []*docs.ParagraphElement{
							{StartIndex: 1, TextRun: &docs.TextRun{Content: "First Heading\n"}},
						},
					},
				},
				{
					Paragraph: &docs.Paragraph{
						ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "NORMAL_TEXT"},
						Elements: []*docs.ParagraphElement{
							{StartIndex: 15, TextRun: &docs.TextRun{Content: "This is some text with footnote"}},
							{FootnoteReference: &docs.FootnoteReference{FootnoteId: "fn-1"}},
							{StartIndex: 48, TextRun: &docs.TextRun{Content: " and a comment"}},
						},
					},
				},
				{
					Paragraph: &docs.Paragraph{
						ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "HEADING_1"},
						Elements: []*docs.ParagraphElement{
							{StartIndex: 62, TextRun: &docs.TextRun{Content: "Second Heading\n"}},
						},
					},
				},
				{
					Paragraph: &docs.Paragraph{
						ParagraphStyle: &docs.ParagraphStyle{NamedStyleType: "NORMAL_TEXT"},
						Elements: []*docs.ParagraphElement{
							{StartIndex: 77, TextRun: &docs.TextRun{Content: "This is some more text with footnote"}},
							{FootnoteReference: &docs.FootnoteReference{FootnoteId: "fn-2"}},
							{StartIndex: 114, TextRun: &docs.TextRun{Content: ".\n"}},
						},
					},
				},
			},
		},
		Footnotes: map[string]docs.Footnote{
			"fn-1": {
				FootnoteId: "fn-1",
				Content: []*docs.StructuralElement{
					{Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{TextRun: &docs.TextRun{Content: "First section footnote."}},
						},
					}},
				},
			},
			"fn-2": {
				FootnoteId: "fn-2",
				Content: []*docs.StructuralElement{
					{Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{TextRun: &docs.TextRun{Content: "Second section footnote."}},
						},
					}},
				},
			},
		},
	}

	comments := []gdocs.Comment{
		{
			ID:          "comment-1",
			Author:      "Tester",
			Content:     "A comment in first section",
			QuotedText:  "a comment",
			CreatedTime: "2026-06-08T12:00:00Z",
		},
	}

	c := NewConverter(doc)
	c.SetComments(comments, "")
	out, err := c.Convert()
	if err != nil {
		t.Fatalf("Convert() error: %v", err)
	}

	// Verify that the first footnote and comment are placed before the second heading
	firstSectionEnd := strings.Index(out, "Second Heading")
	if firstSectionEnd == -1 {
		t.Fatalf("Second Heading not found in output")
	}

	firstSectionContent := out[:firstSectionEnd]
	secondSectionContent := out[firstSectionEnd:]

	if !strings.Contains(firstSectionContent, "[^fn-1]: First section footnote.") {
		t.Errorf("First section should contain footnote fn-1 definition")
	}
	if !strings.Contains(firstSectionContent, "<!-- gdoc-comment: comment-1") {
		t.Errorf("First section should contain comment-1 definition")
	}
	if strings.Contains(secondSectionContent, "[^fn-1]: First section footnote.") {
		t.Errorf("Second section should not contain footnote fn-1 definition")
	}

	// Verify that the second footnote is in the second section
	if !strings.Contains(secondSectionContent, "[^fn-2]: Second section footnote.") {
		t.Errorf("Second section should contain footnote fn-2 definition")
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestConvertParagraphElements(t *testing.T) {
	tests := []struct {
		name     string
		elements []*docs.ParagraphElement
		want     string
	}{
		{
			name:     "empty elements",
			elements: []*docs.ParagraphElement{},
			want:     "",
		},
		{
			name: "single text run",
			elements: []*docs.ParagraphElement{
				{
					TextRun: &docs.TextRun{
						Content:   "Hello",
						TextStyle: &docs.TextStyle{},
					},
				},
			},
			want: "Hello",
		},
		{
			name: "multiple text runs",
			elements: []*docs.ParagraphElement{
				{
					TextRun: &docs.TextRun{
						Content:   "Hello ",
						TextStyle: &docs.TextStyle{},
					},
				},
				{
					TextRun: &docs.TextRun{
						Content:   "World",
						TextStyle: &docs.TextStyle{Bold: true},
					},
				},
			},
			want: "Hello **World**",
		},
		{
			name: "mixed formatting",
			elements: []*docs.ParagraphElement{
				{
					TextRun: &docs.TextRun{
						Content:   "This is ",
						TextStyle: &docs.TextStyle{},
					},
				},
				{
					TextRun: &docs.TextRun{
						Content:   "bold",
						TextStyle: &docs.TextStyle{Bold: true},
					},
				},
				{
					TextRun: &docs.TextRun{
						Content:   " and ",
						TextStyle: &docs.TextStyle{},
					},
				},
				{
					TextRun: &docs.TextRun{
						Content:   "italic",
						TextStyle: &docs.TextStyle{Italic: true},
					},
				},
			},
			want: "This is **bold** and *italic*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertParagraphElements(tt.elements)
			if got != tt.want {
				t.Errorf("ConvertParagraphElements() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOpenCommentsMobileBasicOmission(t *testing.T) {
	doc := &docs.Document{
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				{
					StartIndex: 1,
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								StartIndex: 1,
								TextRun: &docs.TextRun{
									Content: "Unrelated text here.",
								},
							},
						},
					},
				},
			},
		},
	}

	comments := []gdocs.Comment{
		{
			ID:          "comment-unanchored",
			Content:     "This comment can't be matched.",
			QuotedText:  "nonexistent",
			CreatedTime: "2026-06-08T12:00:00Z",
		},
	}

	// Case 1: openCommentsOnly = true, mobileBasicSucceeded = true
	// In the new behavior, the unplaced open comment SHOULD populate ## Comments (unattached) section
	c1 := NewConverter(doc)
	c1.SetOpenCommentsOnly(true)
	c1.SetComments(comments, "<html><body>Unrelated</body></html>")
	out1, err := c1.Convert()
	if err != nil {
		t.Fatalf("Convert() error: %v", err)
	}
	if !strings.Contains(out1, "## Comments (unattached)") {
		t.Errorf("expected ## Comments (unattached) section to be included under openCommentsOnly and mobileBasicSucceeded, but was missing: %q", out1)
	}

	// Case 2: openCommentsOnly = false, mobileBasicSucceeded = true
	// The unplaced comment SHOULD populate ## Comments (unattached) section
	c2 := NewConverter(doc)
	c2.SetOpenCommentsOnly(false)
	c2.SetComments(comments, "<html><body>Unrelated</body></html>")
	out2, err := c2.Convert()
	if err != nil {
		t.Fatalf("Convert() error: %v", err)
	}
	if !strings.Contains(out2, "## Comments (unattached)") {
		t.Errorf("expected ## Comments (unattached) section to be populated when openCommentsOnly = false, but was missing")
	}
}

