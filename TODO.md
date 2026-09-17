# TODO

## Comment handling
- [x] Align upload comment JSON with structure from API
  - [x] adjust field names, e.g. `new-reply` instead of `new-comment`
  - [/] Include author email (does not work -- not returned by API?)
- [x] change formatting of embedded comment threads in .md to pretty-printed JSON
- [ ] allow uploading of comments from drafts / `new-reply` fields in in-line comment blocks directly from the .md
- [x] Consider using yaml for comment blocks instead of JSON, for easier manual editing. 


## Markup fidelity
- [x] Escape literal markdown metacharacters in run text (a literal `*` in the
      doc used to come out as emphasis syntax, indistinguishable from real
      italics, which silently corrupted documents on sync)
- [x] Keep emphasis delimiters flush against non-space text (`**AI.**\n`, not
      `**AI.\n**`)
- [x] Coalesce adjacent runs that render identically
- [ ] Escape block markers (`#`, `-`, `>`, `1.`) when a paragraph's text starts
      with one
