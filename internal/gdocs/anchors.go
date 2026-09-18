package gdocs

import (
	"bytes"
	"log"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	nethtml "golang.org/x/net/html"
	"google.golang.org/api/docs/v1"
)

// AnchorResult records where each comment thread attaches to the document body.
type AnchorResult struct {
	// Offsets maps an absolute document character offset to the IDs of the
	// comments anchored there, in the order the document shows them.
	Offsets map[int][]string
	// AnchoredIDs lists every ID in Offsets, in document order.
	AnchoredIDs []string
	// UnanchoredIDs lists the comments that could not be placed in the body,
	// in the order they were passed in.
	UnanchoredIDs []string
}

// BuildAnchorResult resolves each comment thread's position in the document body.
//
// Neither field the Drive API gives us is enough on its own. The anchor is an
// opaque "kix.*" identifier that no public API resolves to a character range,
// and quotedFileContent is often too short to locate — on a real chapter draft,
// quoted text such as "Go", "both" or "." placed fewer than a fifth of the
// threads unambiguously. (The Docs API grew a commentsViewMode parameter that
// would give us ranges directly, but as of September 2026 it is still restricted
// to the Workspace Developer Preview program.)
//
// The document's mobilebasic rendering, though, marks every open thread inline,
// so we read the positions from there and carry them over to Docs API offsets:
//
//  1. Parse mobilebasic into a transcript of the document text, the position of
//     each inline comment marker within it, and the text of the box each marker
//     links to.
//  2. Pair each thread with its marker by matching box text against thread
//     content, consuming the boxes of the thread's replies as we go.
//  3. Project the body into the same form and carry each marker over by
//     searching for the text that precedes it.
//
// Both sides are reduced to letters and digits, which makes step 3 immune to the
// differences in whitespace, entity escaping and punctuation between the two
// renderings, and lets a single substring search stand in for the sequence
// alignment this used to do.
//
// Comments that mobilebasic does not mark (resolved threads, and threads whose
// anchor text has been deleted) and comments whose marker sits outside the body
// being converted (another tab, or a footnote) end up in UnanchoredIDs.
func BuildAnchorResult(body *docs.Body, comments []Comment, mobileBasicHTML string) AnchorResult {
	res := AnchorResult{Offsets: make(map[int][]string)}

	mobile := parseMobileBasic(mobileBasicHTML)
	bodyText := indexBody(body)
	markers := assignMarkers(mobile, comments)

	type placement struct {
		offset int
		order  int
		id     string
	}
	var placed []placement
	anchored := make(map[string]bool, len(comments))
	for _, c := range comments {
		m, ok := markers[c.ID]
		if !ok {
			continue
		}
		offset, ok := bodyText.locate(&mobile.text, m.pos)
		if !ok {
			continue
		}
		placed = append(placed, placement{offset: offset, order: m.order, id: c.ID})
		anchored[c.ID] = true
	}

	// Several threads can share an offset: replies aside, two people can comment
	// on the same word. Document order breaks the tie.
	sort.Slice(placed, func(i, j int) bool {
		if placed[i].offset != placed[j].offset {
			return placed[i].offset < placed[j].offset
		}
		return placed[i].order < placed[j].order
	})
	for _, p := range placed {
		res.Offsets[p.offset] = append(res.Offsets[p.offset], p.id)
		res.AnchoredIDs = append(res.AnchoredIDs, p.id)
	}
	for _, c := range comments {
		if !anchored[c.ID] {
			res.UnanchoredIDs = append(res.UnanchoredIDs, c.ID)
		}
	}

	log.Printf("Anchored %d of %d comment thread(s) in the document body", len(res.AnchoredIDs), len(comments))
	return res
}

// marker is one inline comment marker in the mobilebasic rendering.
type marker struct {
	key   string // the N in the "cmntN" the marker links to
	order int    // the marker's index in document order
	pos   int    // position in the transcript, in retained characters
}

// mobileView is what we need out of a document's mobilebasic rendering.
type mobileView struct {
	text    alnumText
	markers []marker
	boxes   map[string]string // "cmntN" key → text of the comment or reply box
}

// parseMobileBasic reads a mobilebasic rendering into a mobileView.
//
// Docs renders every open thread twice: as an <a href="#cmntN"> superscript at
// the point it is anchored, and, after the whole document, as a bordered box
// holding one comment or reply behind an <a id="cmntN"> backlink. Every marker
// therefore precedes every box, which is what lets a single pass tell document
// text from box text without having to recognise the box markup itself.
func parseMobileBasic(htmlStr string) *mobileView {
	v := &mobileView{boxes: make(map[string]string)}
	if strings.TrimSpace(htmlStr) == "" {
		return v
	}

	doc, err := nethtml.Parse(strings.NewReader(htmlStr))
	if err != nil {
		log.Printf("Warning: mobilebasic HTML did not parse (%v); comments will not be anchored", err)
		return v
	}

	// Once the first box backlink is seen, all remaining text belongs to boxes.
	var boxKey string
	var box strings.Builder
	closeBox := func() {
		if boxKey != "" {
			v.boxes[boxKey] = strings.TrimSpace(box.String())
		}
		box.Reset()
	}

	var walk func(*nethtml.Node)
	walk = func(n *nethtml.Node) {
		switch {
		case n.Type == nethtml.TextNode:
			if boxKey != "" {
				box.WriteString(n.Data)
			} else {
				for _, r := range n.Data {
					v.text.add(r, 0)
				}
			}
		case n.Type == nethtml.ElementNode && (n.Data == "script" || n.Data == "style"):
			return
		case n.Type == nethtml.ElementNode && n.Data == "a":
			href := attrValue(n, "href")
			switch {
			case strings.HasPrefix(href, "#cmnt_ref"):
				// Backlink opening a comment box: "[a]" label, then the body.
				closeBox()
				boxKey = attrValue(n, "id")
				return
			case strings.HasPrefix(href, "#cmnt"):
				v.markers = append(v.markers, marker{
					key:   strings.TrimPrefix(href, "#"),
					order: len(v.markers),
					pos:   v.text.len(),
				})
				return
			case strings.HasPrefix(href, "#ftnt"):
				// Footnote reference or backlink: the "[1]" label is not text.
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	closeBox()

	return v
}

func attrValue(n *nethtml.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

// assignMarkers pairs comment threads with the markers that point at them.
//
// The markers come in document order, and the boxes are in the matching order:
// a thread's box is followed by one box per reply, plus the occasional box for a
// reaction or a "marked as resolved" notice, all sharing the thread's position.
// Consuming a thread's replies along with the thread keeps a reply from being
// mistaken for a thread of its own, which matters when the reply says no more
// than "+1" — a substring of half the threads in the document, and the source of
// the mis-attributed anchors this replaced.
func assignMarkers(v *mobileView, comments []Comment) map[string]marker {
	byContent := make(map[string][]int, len(comments))
	for i, c := range comments {
		if c.ID == "" {
			continue
		}
		key := normalizeBoxText(c.Content)
		if key == "" {
			continue
		}
		byContent[key] = append(byContent[key], i)
	}

	claimed := make(map[int]bool, len(comments))
	// unclaimedThread reports the comment a box starts a thread for, or -1.
	unclaimedThread := func(boxText string) int {
		for _, i := range byContent[boxText] {
			if !claimed[i] {
				return i
			}
		}
		return -1
	}

	out := make(map[string]marker, len(comments))
	for i := 0; i < len(v.markers); {
		m := v.markers[i]
		i++

		ci := unclaimedThread(normalizeBoxText(v.boxes[m.key]))
		if ci < 0 {
			continue
		}
		claimed[ci] = true
		out[comments[ci].ID] = m

		pending := comments[ci].Replies
		for i < len(v.markers) && len(pending) > 0 {
			text := normalizeBoxText(v.boxes[v.markers[i].key])
			expected := text == normalizeBoxText(pending[0].Content)
			if !expected && unclaimedThread(text) >= 0 {
				break // the next thread starts here
			}
			// Reading a box as the reply we expect, rather than as a thread of
			// its own, is what keeps a "+1" reply from claiming a "+1" thread's
			// anchor. Anything else here is a reaction or a status notice.
			if expected {
				pending = pending[1:]
			}
			i++
		}
	}
	return out
}

// normalizeBoxText reduces comment text to lowercase words, so that a thread's
// content compares equal to the box mobilebasic renders for it despite
// differences in whitespace, entity escaping and stray markup.
func normalizeBoxText(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// alnumText holds the letters and digits of one rendering of a document, with a
// back-map from each retained character to where it came from. Dropping
// everything else lets two renderings be compared without having to agree on how
// each spells a non-breaking space, a smart quote or a line break.
type alnumText struct {
	buf    []byte
	starts []int // byte index in buf at which each retained character begins
	after  []int // source position just past each retained character
	runs   []span
}

// span is a half-open offset range the converter is able to put a marker in.
type span struct{ start, end int }

func (a *alnumText) add(r rune, after int) {
	if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
		return
	}
	a.starts = append(a.starts, len(a.buf))
	a.after = append(a.after, after)
	a.buf = utf8.AppendRune(a.buf, r)
}

func (a *alnumText) len() int { return len(a.starts) }

// slice returns the retained characters in [from, to), clamped to the text.
func (a *alnumText) slice(from, to int) []byte {
	if from < 0 {
		from = 0
	}
	if to > a.len() {
		to = a.len()
	}
	if from >= to {
		return nil
	}
	end := len(a.buf)
	if to < a.len() {
		end = a.starts[to]
	}
	return a.buf[a.starts[from]:end]
}

// contextWindows are the lengths, in retained characters, of the run of text
// before a marker that locate tries to find in the body. Longest first, so that
// a placement rests on as much agreeing text as it can get.
var contextWindows = []int{512, 384, 256, 192, 128, 96, 64, 48, 32, 24, 16, 12, 8}

// locate carries a position in the mobilebasic transcript over to a document
// offset by searching the body for the text that precedes it.
//
// The context has to occur exactly once on both sides. A repeat in the body
// would leave the destination ambiguous; a repeat in the transcript means the
// same text appears elsewhere in the rendering — most often on another tab,
// whose comments must not be placed in the tab being converted.
func (a *alnumText) locate(mobile *alnumText, pos int) (int, bool) {
	if a.len() == 0 {
		return 0, false
	}
	for _, w := range contextWindows {
		ctx := mobile.slice(pos-w, pos)
		if len(ctx) == 0 {
			continue
		}
		if bytes.Count(mobile.buf, ctx) != 1 || bytes.Count(a.buf, ctx) != 1 {
			continue
		}
		// Anchor just past the context's last character, which is where the
		// commented text ends.
		end := bytes.Index(a.buf, ctx) + len(ctx)
		return a.snap(a.after[sort.SearchInts(a.starts, end)-1]), true
	}
	return 0, false
}

// snap moves an offset into the nearest text run at or after it. An offset can
// land between runs — a comment ending on the word right before a footnote
// reference does exactly that — and the converter only emits markers from
// inside a run, so an unsnapped offset would drop the comment silently.
func (a *alnumText) snap(offset int) int {
	if len(a.runs) == 0 {
		return offset
	}
	i := sort.Search(len(a.runs), func(i int) bool { return a.runs[i].end > offset })
	switch {
	case i == len(a.runs):
		return a.runs[len(a.runs)-1].end - 1
	case offset < a.runs[i].start:
		return a.runs[i].start
	default:
		return offset
	}
}

// indexBody projects the body's text runs into an alnumText keyed by UTF-16
// document offset. Only top-level paragraphs are indexed, because those are the
// only places the converter can emit an anchor marker.
func indexBody(body *docs.Body) *alnumText {
	a := &alnumText{}
	if body == nil {
		return a
	}
	for _, el := range body.Content {
		if el.Paragraph == nil {
			continue
		}
		for _, pe := range el.Paragraph.Elements {
			if pe.TextRun == nil {
				continue
			}
			offset := int(pe.StartIndex)
			// A comment's range ends after the punctuation that closes the
			// text it covers, so let a retained character's end position run
			// on over any punctuation directly behind it. Whitespace ends the
			// run: a marker belongs to the sentence it comments on, not to the
			// word that happens to start the next one.
			adjacent := false
			a.runs = append(a.runs, span{start: offset, end: offset + utf16Len(pe.TextRun.Content)})
			for _, r := range pe.TextRun.Content {
				next := offset + utf16RuneLen(r)
				switch {
				case unicode.IsLetter(r) || unicode.IsDigit(r):
					a.add(r, next)
					adjacent = true
				case unicode.IsSpace(r):
					adjacent = false
				case adjacent:
					a.after[a.len()-1] = next
				}
				offset = next
			}
		}
	}
	return a
}

func utf16RuneLen(r rune) int {
	if r >= 0x10000 {
		return 2
	}
	return 1
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16RuneLen(r)
	}
	return n
}
