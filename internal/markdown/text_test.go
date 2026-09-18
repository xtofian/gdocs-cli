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
		{
			name:  "literal asterisks are escaped, not emitted as emphasis",
			text:  "*continuous assurance at scale*",
			style: nil,
			want:  `\*continuous assurance at scale\*`,
		},
		{
			name:  "literal asterisks inside an italic run stay literal",
			text:  "3 * 4",
			style: &docs.TextStyle{Italic: true},
			want:  `*3 \* 4*`,
		},
		{
			name:  "intraword underscores are left alone, delimiting ones are escaped",
			text:  "see 2023_stubborn_weaknesses.html and _this_ word",
			style: &docs.TextStyle{},
			want:  `see 2023_stubborn_weaknesses.html and \_this\_ word`,
		},
		{
			name:  "literal backtick, brackets and backslash are escaped",
			text:  `a ` + "`" + `b [c] d \ e`,
			style: &docs.TextStyle{},
			want:  `a \` + "`" + `b \[c\] d \\ e`,
		},
		{
			name:  "monospace content is fenced, not escaped",
			text:  "args ...any",
			style: &docs.TextStyle{WeightedFontFamily: &docs.WeightedFontFamily{FontFamily: "Roboto Mono"}},
			want:  "`args ...any`",
		},
		{
			name:  "monospace content containing a backtick widens the fence",
			text:  "echo `date`",
			style: &docs.TextStyle{WeightedFontFamily: &docs.WeightedFontFamily{FontFamily: "Roboto Mono"}},
			// CommonMark strips one leading and one trailing space only as a
			// pair, so the padding has to be symmetric.
			want: "`` echo `date` ``",
		},
		{
			name:  "bold run that is only a space gets no delimiters",
			text:  " ",
			style: &docs.TextStyle{Bold: true},
			want:  " ",
		},
		{
			name:  "bold run carrying the paragraph mark keeps it outside",
			text:  "AI.\n",
			style: &docs.TextStyle{Bold: true},
			want:  "**AI.**\n",
		},
		{
			name:  "bold run with a trailing space keeps it outside",
			text:  "Exception processes ",
			style: &docs.TextStyle{Bold: true},
			want:  "**Exception processes** ",
		},
		{
			name:  "italic run with surrounding space keeps it outside",
			text:  " emergent property ",
			style: &docs.TextStyle{Italic: true},
			want:  " *emergent property* ",
		},
		{
			name:  "strikethrough run with a trailing newline keeps it outside",
			text:  "gone\n",
			style: &docs.TextStyle{Strikethrough: true},
			want:  "~~gone~~\n",
		},
		{
			name:  "link destination with parentheses uses pointy brackets",
			text:  "Ada",
			style: &docs.TextStyle{Link: &docs.Link{Url: "https://en.wikipedia.org/wiki/Ada_(language)"}},
			want:  "[Ada](<https://en.wikipedia.org/wiki/Ada_(language)>)",
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
							{StartIndex: 48, TextRun: &docs.TextRun{Content: " and a comment\n"}},
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

	mobileBasic := `<html><body>` +
		`<h1>First Heading</h1>` +
		`<p>This is some text with footnote<sup><a href="#ftnt1" id="ftnt_ref1">[1]</a></sup>` +
		` and a comment<sup><a href="#cmnt1" id="cmnt_ref1">[a]</a></sup></p>` +
		`<h1>Second Heading</h1>` +
		`<p>This is some more text with footnote<sup><a href="#ftnt2" id="ftnt_ref2">[2]</a></sup>.</p>` +
		`<div style="border:1px solid black"><p><a href="#cmnt_ref1" id="cmnt1">[a]</a>` +
		`<span>A comment in first section</span></p></div>` +
		`</body></html>`

	c := NewConverter(doc)
	c.SetComments(comments, mobileBasic)
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

func TestCoalesceAdjacentRuns(t *testing.T) {
	tests := []struct {
		name     string
		elements []*docs.ParagraphElement
		want     string
	}{
		{
			name: "one italic phrase split across runs emits one span",
			elements: []*docs.ParagraphElement{
				{StartIndex: 1, TextRun: &docs.TextRun{Content: "you can achieve ", TextStyle: &docs.TextStyle{}}},
				{StartIndex: 17, TextRun: &docs.TextRun{Content: "continuous", TextStyle: &docs.TextStyle{Italic: true}}},
				{StartIndex: 27, TextRun: &docs.TextRun{Content: " assurance at ", TextStyle: &docs.TextStyle{Italic: true}}},
				{StartIndex: 41, TextRun: &docs.TextRun{Content: "scale", TextStyle: &docs.TextStyle{Italic: true}}},
				{StartIndex: 46, TextRun: &docs.TextRun{Content: ".", TextStyle: &docs.TextStyle{}}},
			},
			want: "you can achieve *continuous assurance at scale*.",
		},
		{
			name: "runs differing only in font size still merge",
			elements: []*docs.ParagraphElement{
				{StartIndex: 1, TextRun: &docs.TextRun{Content: "emergent ", TextStyle: &docs.TextStyle{Italic: true, FontSize: &docs.Dimension{Magnitude: 11}}}},
				{StartIndex: 10, TextRun: &docs.TextRun{Content: "property", TextStyle: &docs.TextStyle{Italic: true}}},
			},
			want: "*emergent property*",
		},
		{
			name: "differently styled runs are left alone",
			elements: []*docs.ParagraphElement{
				{StartIndex: 1, TextRun: &docs.TextRun{Content: "bold", TextStyle: &docs.TextStyle{Bold: true}}},
				{StartIndex: 5, TextRun: &docs.TextRun{Content: " and ", TextStyle: &docs.TextStyle{}}},
				{StartIndex: 10, TextRun: &docs.TextRun{Content: "italic", TextStyle: &docs.TextStyle{Italic: true}}},
			},
			want: "**bold** and *italic*",
		},
		{
			name: "adjacent code runs in different mono fonts merge into one span",
			elements: []*docs.ParagraphElement{
				{StartIndex: 1, TextRun: &docs.TextRun{Content: "notexist'", TextStyle: &docs.TextStyle{WeightedFontFamily: &docs.WeightedFontFamily{FontFamily: "Courier New"}}}},
				{StartIndex: 10, TextRun: &docs.TextRun{Content: " OR ", TextStyle: &docs.TextStyle{WeightedFontFamily: &docs.WeightedFontFamily{FontFamily: "Roboto Mono"}}}},
				{StartIndex: 14, TextRun: &docs.TextRun{Content: "album_id='xyz456", TextStyle: &docs.TextStyle{WeightedFontFamily: &docs.WeightedFontFamily{FontFamily: "Courier New"}}}},
			},
			want: "`notexist' OR album_id='xyz456`",
		},
		{
			name: "runs with different link targets do not merge",
			elements: []*docs.ParagraphElement{
				{StartIndex: 1, TextRun: &docs.TextRun{Content: "a", TextStyle: &docs.TextStyle{Link: &docs.Link{Url: "https://a.example"}}}},
				{StartIndex: 2, TextRun: &docs.TextRun{Content: "b", TextStyle: &docs.TextStyle{Link: &docs.Link{Url: "https://b.example"}}}},
			},
			want: "[a](https://a.example)[b](https://b.example)",
		},
		{
			name: "a footnote reference between runs prevents merging",
			elements: []*docs.ParagraphElement{
				{StartIndex: 1, TextRun: &docs.TextRun{Content: "before", TextStyle: &docs.TextStyle{Italic: true}}},
				{StartIndex: 7, FootnoteReference: &docs.FootnoteReference{FootnoteId: "fn1"}},
				{StartIndex: 8, TextRun: &docs.TextRun{Content: "after", TextStyle: &docs.TextStyle{Italic: true}}},
			},
			want: "*before*[^fn1]*after*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertElementsWithFootnotes(tt.elements, nil, func(string) {}, nil)
			if got != tt.want {
				t.Errorf("convertElementsWithFootnotes() = %q, want %q", got, tt.want)
			}
			if plain := ConvertParagraphElements(tt.elements); tt.elements[1].FootnoteReference == nil && plain != tt.want {
				t.Errorf("ConvertParagraphElements() = %q, want %q", plain, tt.want)
			}
		})
	}
}

// A comment anchor inside a styled phrase necessarily interrupts it: the HTML
// marker cannot sit inside markdown emphasis delimiters.
func TestCoalesceRunsSplitAtCommentAnchor(t *testing.T) {
	elements := []*docs.ParagraphElement{
		{StartIndex: 1, TextRun: &docs.TextRun{Content: "Language choice ", TextStyle: &docs.TextStyle{Bold: true}}},
		{StartIndex: 17, TextRun: &docs.TextRun{Content: "is among the best", TextStyle: &docs.TextStyle{Bold: true}}},
	}
	got := convertElementsWithFootnotes(elements, map[int][]string{17: {"c1"}}, nil, func(string) {})
	want := "**Language choice** <!-- gdoc-comment: c1 -->**is among the best**"
	if got != want {
		t.Errorf("convertElementsWithFootnotes() = %q, want %q", got, want)
	}
}
