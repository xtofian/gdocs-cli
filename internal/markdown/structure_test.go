package markdown

import (
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestConvertParagraph(t *testing.T) {
	tests := []struct {
		name  string
		para  *docs.Paragraph
		style *docs.ParagraphStyle
		want  string
	}{
		{
			name:  "nil paragraph",
			para:  nil,
			style: nil,
			want:  "",
		},
		{
			name: "empty paragraph",
			para: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{
						TextRun: &docs.TextRun{Content: "\n"},
					},
				},
			},
			style: &docs.ParagraphStyle{},
			want:  "\n",
		},
		{
			name: "normal paragraph",
			para: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{
						TextRun: &docs.TextRun{Content: "This is a paragraph.\n"},
					},
				},
			},
			style: &docs.ParagraphStyle{NamedStyleType: "NORMAL_TEXT"},
			want:  "This is a paragraph.\n\n",
		},
		{
			name: "heading 1",
			para: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{
						TextRun: &docs.TextRun{Content: "Heading 1\n"},
					},
				},
			},
			style: &docs.ParagraphStyle{NamedStyleType: "HEADING_1"},
			want:  "# Heading 1\n\n",
		},
		{
			name: "heading 2",
			para: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{
						TextRun: &docs.TextRun{Content: "Heading 2\n"},
					},
				},
			},
			style: &docs.ParagraphStyle{NamedStyleType: "HEADING_2"},
			want:  "## Heading 2\n\n",
		},
		{
			name: "heading 3",
			para: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{
						TextRun: &docs.TextRun{Content: "Heading 3\n"},
					},
				},
			},
			style: &docs.ParagraphStyle{NamedStyleType: "HEADING_3"},
			want:  "### Heading 3\n\n",
		},
		{
			name: "title",
			para: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{
						TextRun: &docs.TextRun{Content: "Document Title\n"},
					},
				},
			},
			style: &docs.ParagraphStyle{NamedStyleType: "TITLE"},
			want:  "# Document Title\n\n",
		},
		{
			name: "subtitle",
			para: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{
						TextRun: &docs.TextRun{Content: "Subtitle\n"},
					},
				},
			},
			style: &docs.ParagraphStyle{NamedStyleType: "SUBTITLE"},
			want:  "## Subtitle\n\n",
		},
		{
			name: "bullet list item - level 0",
			para: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{
						TextRun: &docs.TextRun{Content: "First item\n"},
					},
				},
				Bullet: &docs.Bullet{
					ListId:       "list1",
					NestingLevel: 0,
				},
			},
			style: &docs.ParagraphStyle{},
			want:  "- First item\n",
		},
		{
			name: "bullet list item - level 1",
			para: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{
						TextRun: &docs.TextRun{Content: "Nested item\n"},
					},
				},
				Bullet: &docs.Bullet{
					ListId:       "list1",
					NestingLevel: 1,
				},
			},
			style: &docs.ParagraphStyle{},
			want:  "  - Nested item\n",
		},
		{
			name: "bullet list item - level 2",
			para: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{
						TextRun: &docs.TextRun{Content: "Double nested\n"},
					},
				},
				Bullet: &docs.Bullet{
					ListId:       "list1",
					NestingLevel: 2,
				},
			},
			style: &docs.ParagraphStyle{},
			want:  "    - Double nested\n",
		},
		{
			name: "heading 1 with ID",
			para: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{
						TextRun: &docs.TextRun{Content: "My Heading\n"},
					},
				},
			},
			style: &docs.ParagraphStyle{
				NamedStyleType: "HEADING_1",
				HeadingId:      "h.test-id-123",
			},
			want:  "# My Heading {#h.test-id-123}\n\n",
		},
		{
			name: "title with ID",
			para: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{
					{
						TextRun: &docs.TextRun{Content: "My Title\n"},
					},
				},
			},
			style: &docs.ParagraphStyle{
				NamedStyleType: "TITLE",
				HeadingId:      "h.title-id-456",
			},
			want:  "# My Title {#h.title-id-456}\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertParagraph(tt.para, tt.style)
			if got != tt.want {
				t.Errorf("ConvertParagraph() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConvertTable(t *testing.T) {
	tests := []struct {
		name  string
		table *docs.Table
		want  string
	}{
		{
			name:  "nil table",
			table: nil,
			want:  "",
		},
		{
			name: "empty table",
			table: &docs.Table{
				TableRows: []*docs.TableRow{},
			},
			want: "",
		},
		{
			name: "simple 2x2 table",
			table: &docs.Table{
				TableRows: []*docs.TableRow{
					{
						TableCells: []*docs.TableCell{
							{
								Content: []*docs.StructuralElement{
									{
										Paragraph: &docs.Paragraph{
											Elements: []*docs.ParagraphElement{
												{TextRun: &docs.TextRun{Content: "Header 1"}},
											},
										},
									},
								},
							},
							{
								Content: []*docs.StructuralElement{
									{
										Paragraph: &docs.Paragraph{
											Elements: []*docs.ParagraphElement{
												{TextRun: &docs.TextRun{Content: "Header 2"}},
											},
										},
									},
								},
							},
						},
					},
					{
						TableCells: []*docs.TableCell{
							{
								Content: []*docs.StructuralElement{
									{
										Paragraph: &docs.Paragraph{
											Elements: []*docs.ParagraphElement{
												{TextRun: &docs.TextRun{Content: "Cell 1"}},
											},
										},
									},
								},
							},
							{
								Content: []*docs.StructuralElement{
									{
										Paragraph: &docs.Paragraph{
											Elements: []*docs.ParagraphElement{
												{TextRun: &docs.TextRun{Content: "Cell 2"}},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			want: "| Header 1 | Header 2 |\n|---|---|\n| Cell 1 | Cell 2 |\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertTable(tt.table)
			if got != tt.want {
				t.Errorf("ConvertTable() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCodeBlocksAndTables(t *testing.T) {
	doc := &docs.Document{
		Title: "Test Document",
		Body: &docs.Body{
			Content: []*docs.StructuralElement{
				// Native code block paragraph 1
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								TextRun: &docs.TextRun{
									Content: "\ue907",
								},
							},
							{
								TextRun: &docs.TextRun{
									Content: "func main() {\n",
									TextStyle: &docs.TextStyle{
										WeightedFontFamily: &docs.WeightedFontFamily{FontFamily: "Roboto Mono"},
									},
								},
							},
						},
					},
				},
				// Native code block paragraph 2 (empty line)
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								TextRun: &docs.TextRun{
									Content: "\ue907",
								},
							},
							{
								TextRun: &docs.TextRun{
									Content: "\n",
								},
							},
						},
					},
				},
				// Native code block paragraph 3
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								TextRun: &docs.TextRun{
									Content: "\ue907",
								},
							},
							{
								TextRun: &docs.TextRun{
									Content: "  println(\"hello\")\n",
									TextStyle: &docs.TextStyle{
										WeightedFontFamily: &docs.WeightedFontFamily{FontFamily: "Roboto Mono"},
									},
								},
							},
						},
					},
				},
				// Normal paragraph to flush code block
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								TextRun: &docs.TextRun{
									Content: "This is some normal text.\n",
								},
							},
						},
						ParagraphStyle: &docs.ParagraphStyle{
							NamedStyleType: "NORMAL_TEXT",
						},
					},
				},
				// Manual monospace block paragraph 1
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								TextRun: &docs.TextRun{
									Content: "echo \"hello\"\n",
									TextStyle: &docs.TextStyle{
										WeightedFontFamily: &docs.WeightedFontFamily{FontFamily: "Courier New"},
									},
								},
							},
						},
					},
				},
				// Manual monospace block paragraph 2 (empty line)
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								TextRun: &docs.TextRun{
									Content: "\n",
								},
							},
						},
					},
				},
				// Manual monospace block paragraph 3
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								TextRun: &docs.TextRun{
									Content: "echo \"world\"\n",
									TextStyle: &docs.TextStyle{
										WeightedFontFamily: &docs.WeightedFontFamily{FontFamily: "Courier New"},
									},
								},
							},
						},
					},
				},
				// Normal paragraph to flush manual block
				{
					Paragraph: &docs.Paragraph{
						Elements: []*docs.ParagraphElement{
							{
								TextRun: &docs.TextRun{
									Content: "End of script.\n",
								},
							},
						},
						ParagraphStyle: &docs.ParagraphStyle{
							NamedStyleType: "NORMAL_TEXT",
						},
					},
				},
				// 1x1 table
				{
					Table: &docs.Table{
						TableRows: []*docs.TableRow{
							{
								TableCells: []*docs.TableCell{
									{
										Content: []*docs.StructuralElement{
											{
												Paragraph: &docs.Paragraph{
													Elements: []*docs.ParagraphElement{
														{
															TextRun: &docs.TextRun{
																Content: "select * from users;\n",
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	converter := NewConverter(doc)
	got := converter.convertBody()
	
	want := "```\nfunc main() {\n\n  println(\"hello\")\n```\n\nThis is some normal text.\n\n```\necho \"hello\"\n\necho \"world\"\n```\n\nEnd of script.\n\n```\nselect * from users;\n```\n\n"
	
	if got != want {
		t.Errorf("convertBody() =\n%q\nwant =\n%q", got, want)
	}
}
