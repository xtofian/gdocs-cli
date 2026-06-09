package gdocs

import (
	"testing"

	"google.golang.org/api/docs/v1"
)

func TestParseMobileBasicHTML(t *testing.T) {
	htmlStr := `<!DOCTYPE html>
<html>
<body>
<p>This is a paragraph with a comment <a href="#cmnt1" id="cmnt_ref1">[a]</a> on a word.</p>
<p>Another paragraph <a href="#cmnt2" id="cmnt_ref2">[b]</a> with another comment.</p>
<div style="border:1px solid black;margin:5px">
<a href="#cmnt_ref1" id="cmnt1">[a]</a><span>First comment body.</span>
</div>
<div style="border:1px solid black;margin:5px">
<a href="#cmnt_ref2" id="cmnt2">[b]</a><span>Second comment body.</span>
</div>
</body>
</html>`

	footnotes, cleanText := ParseMobileBasicHTML(htmlStr)

	// Verify footnotes
	if len(footnotes) != 2 {
		t.Fatalf("expected 2 footnotes, got %d", len(footnotes))
	}
	if footnotes["1"] != "First comment body." {
		t.Errorf("expected footnote '1' to be 'First comment body.', got %q", footnotes["1"])
	}
	if footnotes["2"] != "Second comment body." {
		t.Errorf("expected footnote '2' to be 'Second comment body.', got %q", footnotes["2"])
	}

	// Verify clean text containing placeholders
	expectedText := "This is a paragraph with a comment __CMNT_ANCHOR_1__ on a word.\nAnother paragraph __CMNT_ANCHOR_2__ with another comment."
	if cleanText != expectedText {
		t.Errorf("expected cleanText to be:\n%q\ngot:\n%q", expectedText, cleanText)
	}
}

func TestAlignRunes(t *testing.T) {
	tests := []struct {
		name     string
		s1       string
		s2       string
		index1   int
		expected int
	}{
		{
			name:     "exact match",
			s1:       "hello world",
			s2:       "hello world",
			index1:   5,
			expected: 5,
		},
		{
			name:     "added character in s2",
			s1:       "hello world",
			s2:       "hello  world",
			index1:   6,
			expected: 7, // 'w' shifts to right
		},
		{
			name:     "removed character in s2",
			s1:       "hello world",
			s2:       "helloworld",
			index1:   6,
			expected: 5, // space removed, 'w' shifts to left
		},
		{
			name:     "utf8 smart quotes and spaces",
			s1:       "hello \"world\"",
			s2:       "hello “world”",
			index1:   7,
			expected: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r1 := []rune(tt.s1)
			r2 := []rune(tt.s2)
			alignment := alignRunes(r1, r2)
			got := mapRuneIndex(tt.index1, alignment, len(r1), len(r2))
			if got != tt.expected {
				t.Errorf("alignRunes() mapped index %d to %d, expected %d", tt.index1, got, tt.expected)
			}
		})
	}
}

func TestBuildAnchorResultWithMobileBasic(t *testing.T) {
	// Setup mock Docs API document body
	body := &docs.Body{
		Content: []*docs.StructuralElement{
			{
				StartIndex: 1,
				Paragraph: &docs.Paragraph{
					Elements: []*docs.ParagraphElement{
						{
							StartIndex: 1,
							TextRun: &docs.TextRun{
								Content: "This is a paragraph with a comment on a word.",
							},
						},
					},
				},
			},
		},
	}

	// Setup matching comment with ambiguous text ("a")
	comments := []Comment{
		{
			ID:          "comment-abc",
			Content:     "This is the comment text.",
			QuotedText:  "a", // "a" appears multiple times, so standard search would be ambiguous
			CreatedTime: "2026-06-08T12:00:00Z",
		},
	}

	// Setup mobilebasic HTML
	mobileHTML := `<!DOCTYPE html>
<html>
<body>
<p>This is a paragraph with a comment <a href="#cmnt1" id="cmnt_ref1">[a]</a> on a word.</p>
<div style="border:1px solid black;margin:5px">
<a href="#cmnt_ref1" id="cmnt1">[a]</a><span>This is the comment text.</span>
</div>
</body>
</html>`

	res := BuildAnchorResultWithMobileBasic(body, comments, mobileHTML)

	// The comment is anchored right after "comment" and before " on a word" in:
	// "This is a paragraph with a comment on a word."
	// Index of "comment" is 27. Length of "comment" is 7. StartIndex is 1.
	// So offset should be 1 + 27 + 7 = 35.
	expectedOffset := 35
	expectedID := "comment-abc,#cmnt1"

	if res.Offsets[expectedOffset] != expectedID {
		t.Errorf("expected comment comment-abc to be anchored at offset %d with ID %q, got %q at offset %d", expectedOffset, expectedID, res.Offsets[expectedOffset], expectedOffset)
	}

	if len(res.AnchoredIDs) != 1 || res.AnchoredIDs[0] != "comment-abc" {
		t.Errorf("expected anchored ID list to contain comment-abc, got %v", res.AnchoredIDs)
	}
}