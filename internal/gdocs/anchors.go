package gdocs

import (
	"html"
	"log"
	"regexp"
	"sort"
	"strings"

	nethtml "golang.org/x/net/html"
	"google.golang.org/api/docs/v1"
)

var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

func stripHTML(s string) string {
	s = htmlTagRe.ReplaceAllString(s, "")
	return html.UnescapeString(s)
}

// textSeg pairs a document character start offset with a text run's content.
type textSeg struct {
	start int
	text  string
}

// AnchorResult categorises comments by how well their anchor could be located.
type AnchorResult struct {
	// Offsets maps absolute character offset → comment ID for uniquely-located comments.
	Offsets map[int]string
	// AnchoredIDs lists comment IDs with a unique text match, sorted by document position.
	AnchoredIDs []string
	// AmbiguousIDs lists comment IDs whose quoted text appears more than once, or is absent.
	AmbiguousIDs []string
	// DeletedIDs lists comment IDs whose quoted text is no longer present in the document.
	DeletedIDs []string
}

// BuildAnchorResult resolves each comment's position in the document body by
// searching for its quoted text.
//
// Note on the Drive API anchor field: for Google Docs comments the anchor is an
// opaque internal "kix.*" identifier that cannot be decoded into a character
// range via any public API. We locate comments by searching their quotedFileContent
// text instead. An anchor is only emitted when the text is unambiguous (exactly
// one match); otherwise the comment is placed in AmbiguousIDs or DeletedIDs.
func BuildAnchorResult(body *docs.Body, comments []Comment) AnchorResult {
	res := AnchorResult{Offsets: make(map[int]string)}
	if body == nil {
		for _, c := range comments {
			res.AmbiguousIDs = append(res.AmbiguousIDs, c.ID)
		}
		return res
	}

	// Build paragraph index once.
	type para struct {
		segs     []textSeg
		paraText string
	}
	var paras []para
	for _, elem := range body.Content {
		if elem.Paragraph == nil {
			continue
		}
		var segs []textSeg
		var sb strings.Builder
		for _, pe := range elem.Paragraph.Elements {
			if pe.TextRun != nil && pe.TextRun.Content != "" {
				segs = append(segs, textSeg{int(pe.StartIndex), pe.TextRun.Content})
				sb.WriteString(pe.TextRun.Content)
			}
		}
		paras = append(paras, para{segs, sb.String()})
	}

	// offsets collects (offset, commentID) pairs for anchored comments so we can
	// sort them into document order afterwards.
	type anchoredEntry struct {
		offset int
		id     string
	}
	var anchored []anchoredEntry

	for _, c := range comments {
		if c.ID == "" {
			continue
		}
		if c.QuotedText == "" {
			// No quoted text — can't determine location.
			res.AmbiguousIDs = append(res.AmbiguousIDs, c.ID)
			continue
		}
		quoted := stripHTML(c.QuotedText)
		if quoted == "" {
			res.AmbiguousIDs = append(res.AmbiguousIDs, c.ID)
			continue
		}

		var matches []int
		for _, p := range paras {
			haystack := p.paraText
			searchFrom := 0
			for {
				idx := strings.Index(haystack[searchFrom:], quoted)
				if idx < 0 {
					break
				}
				abs := absOffset(p.segs, searchFrom+idx)
				if abs >= 0 {
					matches = append(matches, abs)
				}
				searchFrom += idx + 1
			}
		}

		switch len(matches) {
		case 0:
			res.DeletedIDs = append(res.DeletedIDs, c.ID)
		case 1:
			res.Offsets[matches[0]] = c.ID
			anchored = append(anchored, anchoredEntry{matches[0], c.ID})
		default:
			log.Printf("Comment %s: quoted text appears %d times; anchor skipped (ambiguous)", c.ID, len(matches))
			res.AmbiguousIDs = append(res.AmbiguousIDs, c.ID)
		}
	}

	// Return anchored IDs in document order (ascending offset).
	sort.Slice(anchored, func(i, j int) bool { return anchored[i].offset < anchored[j].offset })
	for _, e := range anchored {
		res.AnchoredIDs = append(res.AnchoredIDs, e.id)
	}

	return res
}

// absOffset maps a byte position within the concatenated paragraph text back to
// an absolute document character offset using the segment table.
func absOffset(segs []textSeg, bytePos int) int {
	pos := 0
	for _, s := range segs {
		end := pos + len(s.text)
		if end > bytePos {
			return s.start + byteToUTF16Index(s.text, bytePos-pos)
		}
		pos = end
	}
	return -1
}

func byteToUTF16Index(s string, byteIdx int) int {
	uLen := 0
	for i, r := range s {
		if i >= byteIdx {
			break
		}
		if r >= 0x10000 {
			uLen += 2
		} else {
			uLen += 1
		}
	}
	return uLen
}

type paraInfo struct {
	segs     []textSeg
	paraText string
}

// ParseMobileBasicHTML parses the mobilebasic HTML and returns a map of mobile indexes
// (e.g. "1", "2") to their footnote comments text, and the cleaned document body text
// containing placeholders like "__CMNT_ANCHOR_1__" separated by newlines.
func ParseMobileBasicHTML(htmlStr string) (map[string]string, string) {
	footnotes := make(map[string]string)
	var bodyBuilder strings.Builder

	doc, err := nethtml.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return footnotes, ""
	}

	isBlock := map[string]bool{
		"p": true, "div": true, "h1": true, "h2": true, "h3": true, "h4": true,
		"h5": true, "h6": true, "li": true, "tr": true, "br": true,
	}

	var traverse func(*nethtml.Node)
	traverse = func(n *nethtml.Node) {
		if n.Type == nethtml.ElementNode && n.Data == "div" {
			// Check if this div is a comment footnote block
			isFootnote := false
			for _, attr := range n.Attr {
				if attr.Key == "style" && strings.Contains(attr.Val, "border:1px solid black") {
					isFootnote = true
					break
				}
			}
			if isFootnote {
				cmntID, text := extractFootnoteInfo(n)
				if cmntID != "" {
					footnotes[cmntID] = text
				}
				return // Skip children to avoid duplicate text
			}
		}

		if n.Type == nethtml.ElementNode && n.Data == "a" {
			// Check if this is an inline comment anchor
			var href string
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					href = attr.Val
					break
				}
			}
			if strings.HasPrefix(href, "#cmnt") && !strings.HasPrefix(href, "#cmnt_ref") {
				cmntID := strings.TrimPrefix(href, "#cmnt")
				bodyBuilder.WriteString("__CMNT_ANCHOR_" + cmntID + "__")
				return // Skip "[a]" link text
			}
		}

		if n.Type == nethtml.ElementNode && isBlock[n.Data] {
			bodyBuilder.WriteByte('\n')
		}

		if n.Type == nethtml.TextNode {
			bodyBuilder.WriteString(n.Data)
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}

		if n.Type == nethtml.ElementNode && isBlock[n.Data] {
			bodyBuilder.WriteByte('\n')
		}
	}

	traverse(doc)

	// Clean and split lines
	lines := strings.Split(bodyBuilder.String(), "\n")
	var cleanedLines []string
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			cleanedLines = append(cleanedLines, trimmed)
		}
	}

	return footnotes, strings.Join(cleanedLines, "\n")
}

func extractFootnoteInfo(n *nethtml.Node) (string, string) {
	var cmntID string
	var sb strings.Builder

	var collect func(*nethtml.Node)
	collect = func(curr *nethtml.Node) {
		if curr.Type == nethtml.ElementNode && curr.Data == "a" {
			for _, attr := range curr.Attr {
				if attr.Key == "id" && strings.HasPrefix(attr.Val, "cmnt") {
					cmntID = strings.TrimPrefix(attr.Val, "cmnt")
				}
			}
		}
		if curr.Type == nethtml.TextNode {
			sb.WriteString(curr.Data)
		}
		for c := curr.FirstChild; c != nil; c = c.NextSibling {
			collect(c)
		}
	}
	collect(n)

	text := strings.TrimSpace(sb.String())
	// Strip leading "[a]" or similar brackets
	if strings.HasPrefix(text, "[") {
		closeIdx := strings.Index(text, "]")
		if closeIdx != -1 {
			text = strings.TrimSpace(text[closeIdx+1:])
		}
	}
	return cmntID, text
}

// BuildAnchorResultWithMobileBasic resolves comment offsets using mobilebasic HTML
// as well as the Docs API structure.
func BuildAnchorResultWithMobileBasic(body *docs.Body, comments []Comment, mobileBasicHTML string) AnchorResult {
	res := AnchorResult{Offsets: make(map[int]string)}
	if body == nil {
		for _, c := range comments {
			res.AmbiguousIDs = append(res.AmbiguousIDs, c.ID)
		}
		return res
	}

	// 1. Parse mobilebasic HTML
	footnotes, cleanText := ParseMobileBasicHTML(mobileBasicHTML)

	// 2. Map Comment ID <-> Mobile Index
	commentToMobileIndex := make(map[string]string)
	mobileIndexToCommentID := make(map[string]string)
	for _, c := range comments {
		if c.ID == "" {
			continue
		}
		mIdx := matchCommentToFootnote(c, footnotes)
		if mIdx != "" {
			commentToMobileIndex[c.ID] = mIdx
			mobileIndexToCommentID[mIdx] = c.ID
		}
	}

	// 3. Build Docs API paragraphs
	var apiParas []paraInfo
	for _, elem := range body.Content {
		if elem.Paragraph == nil {
			continue
		}
		var segs []textSeg
		var sb strings.Builder
		for _, pe := range elem.Paragraph.Elements {
			if pe.TextRun != nil && pe.TextRun.Content != "" {
				segs = append(segs, textSeg{int(pe.StartIndex), pe.TextRun.Content})
				sb.WriteString(pe.TextRun.Content)
			}
		}
		apiParas = append(apiParas, paraInfo{segs, sb.String()})
	}

	// 4. Extract placeholder positions and map to API paragraphs
	mobileParas := strings.Split(cleanText, "\n")
	anchoredMap := make(map[string]int) // comment.ID -> absolute offset

	// Perform sequence alignment of API paragraphs and mobile paragraphs
	mobileToAPIMap := alignParagraphs(apiParas, mobileParas)

	for j, mp := range mobileParas {
		pids := findPlaceholders(mp)
		if len(pids) == 0 {
			continue
		}

		apiIdx, matched := mobileToAPIMap[j]
		if !matched {
			continue
		}
		ap := apiParas[apiIdx]

		s1, placeholderIndices := parseParagraphPlaceholders(mp)
		s2 := ap.paraText

		runes1 := []rune(s1)
		runes2 := []rune(s2)
		alignment := alignRunes(runes1, runes2)

		for pid, runeIdxInS1 := range placeholderIndices {
			commentID, ok := mobileIndexToCommentID[pid]
			if !ok {
				continue
			}

			runeIdxInS2 := mapRuneIndex(runeIdxInS1, alignment, len(runes1), len(runes2))
			byteIdxInS2 := runeToByteIndex(s2, runeIdxInS2)
			abs := absOffset(ap.segs, byteIdxInS2)
			if abs >= 0 {
				anchoredMap[commentID] = abs
			}
		}
	}

	// 5. Finalize AnchorResult
	type anchoredEntry struct {
		offset int
		id     string
	}
	var anchored []anchoredEntry

	isAnchored := make(map[string]bool)
	for id, offset := range anchoredMap {
		mIdx := commentToMobileIndex[id]
		fullID := id
		if mIdx != "" {
			fullID = id + ",#cmnt" + mIdx
		}
		res.Offsets[offset] = fullID
		anchored = append(anchored, anchoredEntry{offset, id})
		isAnchored[id] = true
	}

	// All comments that were not anchored via mobilebasic are classified as AmbiguousIDs
	for _, c := range comments {
		if !isAnchored[c.ID] {
			res.AmbiguousIDs = append(res.AmbiguousIDs, c.ID)
		}
	}

	// Sort in document order
	sort.Slice(anchored, func(i, j int) bool { return anchored[i].offset < anchored[j].offset })
	for _, e := range anchored {
		res.AnchoredIDs = append(res.AnchoredIDs, e.id)
	}

	return res
}

func matchCommentToFootnote(comment Comment, footnotes map[string]string) string {
	normComment := normalizeForMatching(comment.Content)
	if normComment == "" {
		return ""
	}

	for idx, fnText := range footnotes {
		normFn := normalizeForMatching(fnText)
		if strings.Contains(normFn, normComment) || strings.Contains(normComment, normFn) {
			return idx
		}
	}
	return ""
}

func findPlaceholders(s string) []string {
	var ids []string
	i := 0
	for i < len(s) {
		if strings.HasPrefix(s[i:], "__CMNT_ANCHOR_") {
			end := strings.Index(s[i+14:], "__")
			if end != -1 {
				ids = append(ids, s[i+14:i+14+end])
				i += 14 + end + 2
				continue
			}
		}
		i++
	}
	return ids
}

func parseParagraphPlaceholders(s string) (string, map[string]int) {
	var sb strings.Builder
	placeholderIndices := make(map[string]int)

	runes := []rune(s)
	i := 0
	for i < len(runes) {
		if i <= len(runes)-14 && string(runes[i:i+14]) == "__CMNT_ANCHOR_" {
			sub := runes[i+14:]
			end := -1
			for j := 0; j < len(sub)-1; j++ {
				if sub[j] == '_' && sub[j+1] == '_' {
					end = j
					break
				}
			}
			if end != -1 {
				id := string(sub[:end])
				placeholderIndices[id] = len([]rune(sb.String()))
				i += 14 + end + 2
				continue
			}
		}
		sb.WriteRune(runes[i])
		i++
	}
	return sb.String(), placeholderIndices
}

func findBestMatchingParagraph(mobilePara string, apiParas []paraInfo) int {
	cleanMobile := removePlaceholders(mobilePara)
	normMobile := normalizeForMatching(cleanMobile)
	if normMobile == "" {
		return -1
	}

	bestIdx := -1
	maxOverlap := 0

	for idx, ap := range apiParas {
		normAPI := normalizeForMatching(ap.paraText)
		if normAPI == "" {
			continue
		}

		overlap := wordOverlap(normMobile, normAPI)
		if overlap > maxOverlap {
			maxOverlap = overlap
			bestIdx = idx
		}
	}

	// Require a minimal threshold of matching words
	if maxOverlap < 1 {
		return -1
	}

	return bestIdx
}

func removePlaceholders(s string) string {
	var sb strings.Builder
	i := 0
	for i < len(s) {
		if strings.HasPrefix(s[i:], "__CMNT_ANCHOR_") {
			end := strings.Index(s[i+14:], "__")
			if end != -1 {
				i += 14 + end + 2
				continue
			}
		}
		sb.WriteByte(s[i])
		i++
	}
	return sb.String()
}

func normalizeForMatching(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' {
			sb.WriteRune(r)
		}
	}
	fields := strings.Fields(sb.String())
	return strings.Join(fields, " ")
}

func wordOverlap(s1, s2 string) int {
	w1 := strings.Fields(s1)
	w2 := strings.Fields(s2)

	m := make(map[string]bool)
	for _, w := range w1 {
		m[w] = true
	}

	overlap := 0
	for _, w := range w2 {
		if m[w] {
			overlap++
		}
	}
	return overlap
}

func alignRunes(r1, r2 []rune) map[int]int {
	n1 := len(r1)
	n2 := len(r2)

	dp := make([][]int, n1+1)
	for i := range dp {
		dp[i] = make([]int, n2+1)
	}

	for i := 1; i <= n1; i++ {
		for j := 1; j <= n2; j++ {
			if r1[i-1] == r2[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				dp[i][j] = maxInt(dp[i-1][j], dp[i][j-1])
			}
		}
	}

	alignment := make(map[int]int)
	i, j := n1, n2
	for i > 0 && j > 0 {
		if r1[i-1] == r2[j-1] {
			alignment[i-1] = j-1
			i--
			j--
		} else if dp[i-1][j] >= dp[i][j-1] {
			i--
		} else {
			j--
		}
	}
	return alignment
}

func mapRuneIndex(posInS1 int, alignment map[int]int, lenS1, lenS2 int) int {
	for r := posInS1; r < lenS1; r++ {
		if idxS2, ok := alignment[r]; ok {
			return idxS2
		}
	}
	for l := posInS1 - 1; l >= 0; l-- {
		if idxS2, ok := alignment[l]; ok {
			return idxS2 + 1
		}
	}
	return lenS2
}

func runeToByteIndex(s string, runeIdx int) int {
	byteIdx := 0
	for i, r := range s {
		if runeIdx <= 0 {
			break
		}
		byteIdx = i + len(string(r))
		runeIdx--
	}
	return byteIdx
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func jaccardSimilarity(s1, s2 string) float64 {
	norm1 := normalizeForMatching(s1)
	norm2 := normalizeForMatching(s2)
	if norm1 == "" || norm2 == "" {
		return 0.0
	}

	w1 := strings.Fields(norm1)
	w2 := strings.Fields(norm2)

	set1 := make(map[string]bool)
	for _, w := range w1 {
		set1[w] = true
	}
	set2 := make(map[string]bool)
	for _, w := range w2 {
		set2[w] = true
	}

	intersectSize := 0
	for w := range set1 {
		if set2[w] {
			intersectSize++
		}
	}

	unionSize := len(set1) + len(set2) - intersectSize
	if unionSize == 0 {
		return 0.0
	}

	return float64(intersectSize) / float64(unionSize)
}

func alignParagraphs(apiParas []paraInfo, mobileParas []string) map[int]int {
	n := len(apiParas)
	m := len(mobileParas)

	// dp[i][j] stores the max score for apiParas[0..i-1] and mobileParas[0..j-1]
	dp := make([][]float64, n+1)
	for i := range dp {
		dp[i] = make([]float64, m+1)
	}

	// backtrack table
	// Choices:
	// 0: skip API (i-1)
	// 1: skip Mobile (j-1)
	// 2: match API (i-1) and Mobile (j-1)
	choices := make([][]int, n+1)
	for i := range choices {
		choices[i] = make([]int, m+1)
	}

	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			// Option 1: skip API paragraph
			score := dp[i-1][j]
			choice := 0

			// Option 2: skip Mobile paragraph
			if dp[i][j-1] > score {
				score = dp[i][j-1]
				choice = 1
			}

			// Option 3: match API and Mobile paragraph
			cleanMobile := removePlaceholders(mobileParas[j-1])
			sim := jaccardSimilarity(apiParas[i-1].paraText, cleanMobile)
			if sim >= 0.5 {
				matchScore := dp[i-1][j-1] + sim
				if matchScore > score {
					score = matchScore
					choice = 2
				}
			}

			dp[i][j] = score
			choices[i][j] = choice
		}
	}

	// Backtrack to find the matching
	alignment := make(map[int]int) // mobileParaIndex -> apiParaIndex
	i, j := n, m
	for i > 0 && j > 0 {
		choice := choices[i][j]
		if choice == 2 {
			alignment[j-1] = i-1
			i--
			j--
		} else if choice == 0 {
			i--
		} else {
			j--
		}
	}

	return alignment
}

