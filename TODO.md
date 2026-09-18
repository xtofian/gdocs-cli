# TODO

## Comment handling
- [x] Align upload comment JSON with structure from API
  - [x] adjust field names, e.g. `new-reply` instead of `new-comment`
  - [/] Include author email (does not work -- not returned by API?)
- [x] change formatting of embedded comment threads in .md to pretty-printed JSON
- [ ] allow uploading of comments from drafts / `new-reply` fields in in-line comment blocks directly from the .md
- [x] Consider using yaml for comment blocks instead of JSON, for easier manual editing. 
- [x] Fix comment placement: substring matching of comment text against
      mobilebasic footnotes paired threads with the wrong marker (any comment
      containing "1" matched a "+1" reply), and `map[int]string` offsets dropped
      one of two comments landing in the same spot, so which comment appeared
      where varied between runs.
- [ ] Anchor comments that sit inside a footnote. They are marked in mobilebasic
      like any other, but `indexBody` only covers body paragraphs and the
      converter has no way to emit a marker inside a `[^id]: ...` definition.
- [ ] Replace the mobilebasic path with the Docs API's `commentsViewMode`
      (`documents.get?commentsViewMode=COMMENTS_VIEW_MODE_INCLUDED&includeTabsContent=true`)
      once it leaves the Workspace Developer Preview. It returns comment ranges
      as document indexes, which would make `anchors.go` almost entirely
      unnecessary. Checked 2026-09-17: still 400s with "Field
      'comments_view_mode' could not be found in request message".


## Markup fidelity
- [x] Escape literal markdown metacharacters in run text (a literal `*` in the
      doc used to come out as emphasis syntax, indistinguishable from real
      italics, which silently corrupted documents on sync)
- [x] Keep emphasis delimiters flush against non-space text (`**AI.**\n`, not
      `**AI.\n**`)
- [x] Coalesce adjacent runs that render identically
- [ ] Escape block markers (`#`, `-`, `>`, `1.`) when a paragraph's text starts
      with one

## Upstream sync
- [ ] Move sync from the `sync-google-doc` skill into `gdocs-cli push`.
      Design and integration test plan: [docs/sync-design.md](docs/sync-design.md)
