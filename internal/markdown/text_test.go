package markdown

import (
	"strings"
	"testing"

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

	got := convertElementsWithFootnotes(elements, nil, register)
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
