package gdocs

import (
	"strings"
	"testing"

	"google.golang.org/api/docs/v1"
)

// mobileBasic assembles a mobilebasic-shaped document: the body paragraphs
// first, then one bordered box per comment or reply, in the same order.
func mobileBasic(body string, boxes ...string) string {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html><body>")
	b.WriteString(body)
	for i, text := range boxes {
		n := i + 1
		b.WriteString(`<div style="border:1px solid black;margin:5px"><p>`)
		b.WriteString(`<a href="#cmnt_ref` + itoa(n) + `" id="cmnt` + itoa(n) + `">[x]</a>`)
		b.WriteString(`<span>` + text + `</span></p></div>`)
	}
	b.WriteString("</body></html>")
	return b.String()
}

// ref renders the inline superscript marker for the nth comment box.
func ref(n int) string {
	return `<sup><a href="#cmnt` + itoa(n) + `" id="cmnt_ref` + itoa(n) + `">[x]</a></sup>`
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

// bodyOf builds a one-run-per-paragraph body starting at offset 1, the way the
// Docs API numbers a document.
func bodyOf(paragraphs ...string) *docs.Body {
	body := &docs.Body{}
	offset := int64(1)
	for _, text := range paragraphs {
		content := text + "\n"
		body.Content = append(body.Content, &docs.StructuralElement{
			StartIndex: offset,
			Paragraph: &docs.Paragraph{
				Elements: []*docs.ParagraphElement{{
					StartIndex: offset,
					TextRun:    &docs.TextRun{Content: content},
				}},
			},
		})
		offset += int64(len([]rune(content)))
	}
	return body
}

func TestBuildAnchorResult(t *testing.T) {
	// The quoted text is "a", which occurs all over the paragraph: only the
	// mobilebasic marker can say which occurrence the comment belongs to.
	body := bodyOf("This is a paragraph with a comment on a word.")
	comments := []Comment{{
		ID:         "comment-abc",
		Content:    "This is the comment text.",
		QuotedText: "a",
	}}
	html := mobileBasic(
		"<p>This is a paragraph with a comment "+ref(1)+" on a word.</p>",
		"This is the comment text.",
	)

	res := BuildAnchorResult(body, comments, html)

	// "This is a paragraph with a comment" is 34 characters from offset 1, so
	// the marker lands just past it, at 35.
	const want = 35
	if got := res.Offsets[want]; len(got) != 1 || got[0] != "comment-abc" {
		t.Errorf("Offsets[%d] = %v, want [comment-abc]; full result %v", want, got, res.Offsets)
	}
	if len(res.AnchoredIDs) != 1 || res.AnchoredIDs[0] != "comment-abc" {
		t.Errorf("AnchoredIDs = %v, want [comment-abc]", res.AnchoredIDs)
	}
	if len(res.UnanchoredIDs) != 0 {
		t.Errorf("UnanchoredIDs = %v, want none", res.UnanchoredIDs)
	}
}

func TestBuildAnchorResultRepliesDoNotStealAnchors(t *testing.T) {
	// The first thread's reply says "+1", and so does a second thread anchored
	// further down. Matching boxes to threads one at a time, without accounting
	// for replies, would anchor the second thread on the first one's reply box.
	body := bodyOf(
		"Memory safety is the first topic we cover here.",
		"Supply chain integrity is the second topic we cover.",
	)
	comments := []Comment{
		{
			ID:      "thread-1",
			Content: "Worth mentioning hardened libc++ too.",
			Replies: []Reply{{Author: "Reviewer", Content: "+1"}},
		},
		{ID: "thread-2", Content: "+1"},
	}
	html := mobileBasic(
		"<p>Memory safety is the first topic we cover here."+ref(1)+ref(2)+"</p>"+
			"<p>Supply chain integrity is the second topic we cover."+ref(3)+"</p>",
		"Worth mentioning hardened libc++ too.", "+1", "+1",
	)

	res := BuildAnchorResult(body, comments, html)

	if len(res.AnchoredIDs) != 2 {
		t.Fatalf("AnchoredIDs = %v, want both threads anchored", res.AnchoredIDs)
	}
	first, second := res.AnchoredIDs[0], res.AnchoredIDs[1]
	if first != "thread-1" || second != "thread-2" {
		t.Errorf("AnchoredIDs = %v, want [thread-1 thread-2]", res.AnchoredIDs)
	}
	// thread-2 belongs to the second paragraph, past the end of the first.
	var offset1, offset2 int
	for off, ids := range res.Offsets {
		for _, id := range ids {
			if id == "thread-1" {
				offset1 = off
			} else {
				offset2 = off
			}
		}
	}
	if offset2 <= offset1+len("Memory safety is the first topic we cover here.") {
		t.Errorf("thread-2 anchored at %d, expected it in the second paragraph (thread-1 at %d)", offset2, offset1)
	}
}

func TestBuildAnchorResultSharedOffset(t *testing.T) {
	// Two threads on the same word: both anchors are kept, in document order.
	body := bodyOf("Language choice is the most consequential decision.")
	comments := []Comment{
		{ID: "second", Content: "Also worth saying."},
		{ID: "first", Content: "Worth saying."},
	}
	html := mobileBasic(
		"<p>Language choice"+ref(1)+ref(2)+" is the most consequential decision.</p>",
		"Worth saying.", "Also worth saying.",
	)

	res := BuildAnchorResult(body, comments, html)

	if len(res.Offsets) != 1 {
		t.Fatalf("Offsets = %v, want a single shared offset", res.Offsets)
	}
	for _, ids := range res.Offsets {
		if len(ids) != 2 || ids[0] != "first" || ids[1] != "second" {
			t.Errorf("shared offset holds %v, want [first second]", ids)
		}
	}
}

func TestBuildAnchorResultOtherTab(t *testing.T) {
	// mobilebasic renders every tab; the body being converted is only one of
	// them. A comment on another tab's copy of a paragraph must not be placed
	// on this tab's copy.
	body := bodyOf("Paragraph in current tab.")
	comments := []Comment{{
		ID:         "comment-tab2",
		Content:    "This comment is on the second tab.",
		QuotedText: "current tab",
	}}
	html := mobileBasic(
		"<p>Paragraph in current tab.</p>"+
			"<p>Some other text on tab 2.</p>"+
			"<p>Paragraph in current tab"+ref(1)+".</p>",
		"This comment is on the second tab.",
	)

	res := BuildAnchorResult(body, comments, html)

	if len(res.Offsets) != 0 {
		t.Errorf("Offsets = %v, want the comment left unanchored", res.Offsets)
	}
	if len(res.UnanchoredIDs) != 1 || res.UnanchoredIDs[0] != "comment-tab2" {
		t.Errorf("UnanchoredIDs = %v, want [comment-tab2]", res.UnanchoredIDs)
	}
}

func TestBuildAnchorResultDeletedAnchorText(t *testing.T) {
	// A thread whose anchor text is gone gets no marker in mobilebasic, so it
	// must not borrow the marker of the thread that is still anchored.
	body := bodyOf("Any non-trivial C++ codebase contains memory safety violations.")
	comments := []Comment{
		{ID: "live", Content: "Surprised not to see libc++ mentioned."},
		{ID: "orphan", Content: "I am not sure the argument is sound."},
	}
	html := mobileBasic(
		"<p>Any non-trivial C++ codebase contains memory safety violations."+ref(1)+"</p>",
		"Surprised not to see libc++ mentioned.",
	)

	res := BuildAnchorResult(body, comments, html)

	if len(res.AnchoredIDs) != 1 || res.AnchoredIDs[0] != "live" {
		t.Errorf("AnchoredIDs = %v, want [live]", res.AnchoredIDs)
	}
	if len(res.UnanchoredIDs) != 1 || res.UnanchoredIDs[0] != "orphan" {
		t.Errorf("UnanchoredIDs = %v, want [orphan]", res.UnanchoredIDs)
	}
}

func TestBuildAnchorResultNoMobileBasic(t *testing.T) {
	body := bodyOf("Some text with a comment on it.")
	comments := []Comment{{ID: "c1", Content: "A comment.", QuotedText: "a comment"}}

	res := BuildAnchorResult(body, comments, "")

	if len(res.Offsets) != 0 {
		t.Errorf("Offsets = %v, want none without mobilebasic HTML", res.Offsets)
	}
	if len(res.UnanchoredIDs) != 1 {
		t.Errorf("UnanchoredIDs = %v, want [c1]", res.UnanchoredIDs)
	}
}

func TestParseMobileBasic(t *testing.T) {
	html := mobileBasic(
		"<p>First paragraph"+ref(1)+" continues.</p><p>Second"+ref(2)+" paragraph.</p>",
		"First comment body.", "Second comment body.",
	)

	v := parseMobileBasic(html)

	if got := string(v.text.buf); got != "FirstparagraphcontinuesSecondparagraph" {
		t.Errorf("transcript = %q, want only the body's letters and digits", got)
	}
	if len(v.markers) != 2 {
		t.Fatalf("markers = %v, want 2", v.markers)
	}
	if v.markers[0].pos != len("Firstparagraph") || v.markers[1].pos != len("FirstparagraphcontinuesSecond") {
		t.Errorf("marker positions = %d, %d; want %d, %d",
			v.markers[0].pos, v.markers[1].pos, len("Firstparagraph"), len("FirstparagraphcontinuesSecond"))
	}
	if v.boxes["cmnt1"] != "First comment body." || v.boxes["cmnt2"] != "Second comment body." {
		t.Errorf("boxes = %v, want the two comment bodies", v.boxes)
	}
}

func TestNormalizeBoxText(t *testing.T) {
	tests := []struct{ in, want string }{
		{"I'm  not sure.", "i m not sure"},
		{"+1", "1"},
		{"“Smart quotes” — and dashes", "smart quotes and dashes"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := normalizeBoxText(tt.in); got != tt.want {
			t.Errorf("normalizeBoxText(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
