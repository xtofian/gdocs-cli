# Design: upstream sync in gdocs-cli

Status: proposal
Author: drafted with Claude, 2026-09-17

## 1. Why

`gdocs-cli` pulls a Google Doc down to markdown. Pushing local edits back is
currently done by `sync-google-doc`, a Python skill that shells out to `gog`
(via the `gog-agent` sandbox wrapper) one edit at a time. That arrangement has
now failed destructively on a real chapter-length document, and the failure was
not a coding slip — it is inherent to doing the job from outside the tool.

### The failure

`sync_doc.py` builds its "before" picture from `gog docs paragraphs list`,
which returns the document's **plain text**. It builds its "after" picture from
the local **markdown** file. It then word-diffs one against the other and
writes the differences into the document at character indices derived from the
plain-text side.

Every `*`, `**` and `` ` `` in the local file therefore looks like missing
content. The sync dutifully inserted it. On a 120k-character chapter this
turned 8 literal asterisks into 310, destroyed the real character styling on 97
spans, and — because `gdocs-cli` did not escape metacharacters at the time —
re-downloading produced markdown that still *looked* like emphasis, so the
sync's own verification step passed. Nobody noticed for a day.

The escaping half of that has been fixed (see "Markup Fidelity" in README.md).
The diffing half cannot be fixed from outside: any external tool has to
reconstruct the correspondence between markdown offsets and Docs character
indices by guesswork, because only the renderer knows how one became the other.

### Secondary problems with the current arrangement

- **One API call per edit.** A single sync issued ~550 sequential writes, hit
  the per-minute write quota repeatedly, and died partway through leaving the
  document in a half-edited state that then had to be hand-repaired.
- **No atomicity and no conflict detection.** Nothing pins the revision, so a
  concurrent editor's changes are silently overwritten or interleaved.
- **Addressing is by text match.** `gog docs format` can only target text via
  `--match`, which finds the first occurrence in the document. Short spans are
  ambiguous, and the skill has no way to disambiguate them.
- **Brittle plumbing.** Flag drift between `gog` subcommands (`insert` takes
  `--file` but not `--text`; `update --text=` rejects the empty string) has
  caused several mid-run aborts, each of which required another repair pass.

## 2. Is moving it into gdocs-cli the right route?

Yes, and it needs **less** logic than the skill has today, not more.

### What we would have to build

The write path is one API method: `documents.batchUpdate`. The request types
needed for the supported scope are `insertText`, `deleteContentRange`,
`updateTextStyle`, `updateParagraphStyle`, `createParagraphBullets`,
`deleteParagraphBullets`, `createFootnote`, and `updateTextStyle` on footnote
segments. All are already generated types in `google.golang.org/api/docs/v1`,
which `gdocs-cli` imports. Authentication, the API client, and the document
fetch already exist in `internal/auth` and `internal/gdocs`.

The genuinely new pieces are the document model, the diff, and the request
generator — described in §4. They are new work, but they are *the* work; no
part of them exists in `gog` either.

### What we would not have to replicate

The largest and fiddliest part of `gog`'s docs surface is **locating things**:
`--at` anchor resolution, `--occurrence`, `--match`/`--match-all`, paragraph
addressing, the sedmat DSL, table cell references, image references. We need
none of it. Inside `gdocs-cli` the document is already in memory as a
`docs.Document` tree with exact `startIndex`/`endIndex` on every run, and the
renderer can record the markdown↔index correspondence as it emits. Addressing
becomes constructive instead of reverse-engineered — which is precisely the
class of bug that caused the incident.

Also not replicated: account/OAuth management (we have our own), the
`gog-agent` policy wrapper (see §8), and every editing command outside the
supported scope.

### What stays with gog

Tables, images, drawings, equations, page/section breaks, chips, structural
surgery, and anything else in §7. These stay manual, driven through `gog` from
a skill. `gdocs-cli push` refuses rather than guesses when a local edit touches
one of them, so the division of labour is enforced, not just documented.

### Honest counter-arguments

- **A markdown parser becomes a dependency.** Reading the local file means
  parsing CommonMark plus footnotes. `goldmark` is the obvious choice (pure Go,
  spec-compliant, has a footnote extension). This is real new surface area, but
  the alternative — inferring intent from raw text diffs — is what we are
  trying to get away from.
- **Two tools can now write to the document.** The sandbox story needs
  rethinking (§8) rather than inheriting `gog-agent`'s grant model for free.
- **Renderer and parser must agree.** If the renderer emits markup the parser
  reads back differently, sync corrupts the document. §5 makes that a tested
  invariant rather than an assumption.

## 3. Goals and non-goals

### Goals

1. `gdocs-cli push` applies local markdown edits to the upstream document with
   no loss of formatting, comments, or footnotes.
2. Edits go up as **one** `documents.batchUpdate` where possible, revision-locked
   and atomic.
3. A push that cannot be expressed safely **fails before writing anything**,
   with a message naming the construct and the line.
4. Round-trip is an identity: pull → (no edits) → push is a no-op, and
   pull → edit → push → pull returns the edited file byte-for-byte.
5. Concurrent upstream edits are detected and reported, never clobbered.

### Non-goals

1. Replacing `gog` as a general Workspace CLI.
2. Editing tables, images, or any construct in §7.
3. Three-way merge. A conflict is reported; resolving it is the user's job
   (re-pull and re-apply).
4. Posting comments. `--upload-comments` already exists and stays separate.
5. Preserving character properties the markdown cannot express (font family,
   size, colour, highlight). These are left untouched on spans we do not edit,
   and inherit from the insertion point on spans we do — see §5.4.

## 4. Architecture

### 4.1 Document model

A single intermediate representation sits between the Docs API and markdown:

```
Document
  └── Block[]                  paragraph | heading(level) | listItem(level, ordered) | opaque
        ├── Span[]             text + Style{bold, italic, strike, code, link}
        ├── FootnoteRef[]      interleaved with spans
        └── CommentAnchor[]    interleaved with spans
Footnote{id, Span[]}
```

`opaque` blocks carry the raw markdown for constructs we render but never edit
(tables, images, code blocks, horizontal rules). They participate in the diff
only as "must be identical"; see §7.

Three conversions, all in `internal/markdown`:

- `FromDocument(*docs.Document) (Document, Provenance)` — existing render logic,
  refactored to build the model instead of a string.
- `Render(Document) string` — model → markdown. Existing escaping and
  coalescing rules apply here.
- `Parse(string) (Document, error)` — markdown → model, via `goldmark`.

### 4.2 Provenance

`FromDocument` additionally returns, for every `Span` and `Block` in the model,
the Docs segment ID and `[startIndex, endIndex)` it came from. This is the
whole trick: it is recorded at the moment the correspondence is known, not
inferred afterwards. Indices are UTF-16 code units, matching the Docs API;
the model stores them as such and never converts through Go byte offsets.

Provenance is **not** persisted in the markdown file. `push` recomputes it by
fetching the document afresh and re-running `FromDocument`.

### 4.3 Pull

```
fetch(docID, revision?) → FromDocument → Render → write file + frontmatter
```

Unchanged from today except that it goes through the model. Frontmatter keeps
`gdoc_id`, `revision_id`, `tab`.

### 4.4 Push

```
1. read local file, Parse                                → local: Document
2. fetch document (current revision)                     → doc
3. FromDocument(doc)                                     → base: Document, prov: Provenance
4. if doc.revisionId != frontmatter.revision_id          → conflict (§6)
5. diff(base, local)                                     → []Change
6. reject(changes touching out-of-scope constructs)      → §7
7. plan(changes, prov)                                   → []docs.Request
8. batchUpdate(docID, requests, requiredRevisionId)      → newRevisionId
9. re-fetch, Render, compare against local               → verify
10. rewrite local frontmatter with newRevisionId
```

Step 3 is what makes this sound: **the base is produced by the same renderer
that produced the local file's ancestor**, so base and local are the same kind
of object and the diff is meaningful. The current skill compares a rendered
markdown file against unrendered plain text; that is the bug.

Step 9 is cheap insurance and should stay even once we trust the planner. On
mismatch it reports a diff and exits non-zero; the write has already happened,
so this is a detector, not a rollback.

### 4.5 Diff

Two levels, both operating on the model:

- **Block level:** `difflib`-style LCS over blocks keyed by their rendered
  text, yielding matched pairs, inserted blocks and deleted blocks.
- **Span level:** within each matched pair, a word-granularity diff over the
  concatenated text, plus a separate comparison of the style runs.

Splitting text changes from style changes matters. Today's skill conflates
them, which is why adding italics to an existing phrase gets planned as a text
rewrite. Here, "the words are the same but the style boundary moved" produces a
single `updateTextStyle` and no text mutation at all — leaving comment anchors,
footnote references, and unexpressible character properties untouched.

### 4.6 Request generation

Per change kind:

| Change | Requests |
|---|---|
| text edit inside a block | `deleteContentRange` + `insertText` |
| pure insertion | `insertText` |
| pure deletion | `deleteContentRange` |
| style added/removed/moved | `updateTextStyle` with an explicit `fields` mask |
| link added/changed/removed | `updateTextStyle` with `fields: "link"` |
| heading level changed | `updateParagraphStyle` |
| block became a list item | `createParagraphBullets` |
| list item became a paragraph | `deleteParagraphBullets` |
| new block | `insertText` (with trailing `\n`) + style requests |
| deleted block | `deleteContentRange` covering its paragraph mark |
| new footnote | `createFootnote` + `insertText` into the returned segment |
| footnote text edit | same as text edit, with `segmentId` set |

**Ordering.** Within one `batchUpdate`, requests apply sequentially and indices
shift as they go. Requests are therefore emitted in strictly descending index
order so that every index is still valid when its request runs. Requests
against footnote segments are independent of body indices and are grouped
separately.

**Field masks.** `updateTextStyle` must always set `fields` to exactly the
properties being changed. Setting a whole `TextStyle` would wipe font, size and
colour on the target range — the failure mode that left inline code in the
wrong font during the incident.

## 5. Correctness invariants

These are properties the implementation must hold, and §9 tests each one.

1. **Round-trip identity.** `Render(FromDocument(d))` parsed and re-rendered is
   unchanged: `Render(Parse(Render(FromDocument(d)))) == Render(FromDocument(d))`.
   This is what guarantees renderer and parser agree.
2. **Empty diff ⇒ empty batch.** If the local file equals `Render(base)`, `plan`
   produces zero requests. A no-op push must not touch the document.
3. **Provenance covers all document text.** Every character of `Render(base)`
   is either mapped to a document index or explicitly marked synthetic
   (delimiters, `#`, `- `, `[^id]`, comment markers, footnote definitions,
   frontmatter). No character is unaccounted for.
4. **Indices are UTF-16.** A document containing astral-plane characters
   (emoji) maps correctly. Go string offsets are never used as Docs indices.
5. **Style-only changes issue no text mutations**, and vice versa.
6. **Refusal is total.** If any change is out of scope, zero requests are sent.

## 6. Conflicts, atomicity, quota

- **Revision lock.** Every `batchUpdate` sets
  `writeControl.requiredRevisionId` to the revision the base was fetched at.
  Google rejects the write if the document moved, so a concurrent edit produces
  a clean failure with nothing applied.
- **Staleness check.** If the fetched revision already differs from the
  frontmatter's `revision_id`, stop before diffing and tell the user to re-pull.
  Offer `--force` to diff against the fetched revision anyway (useful when the
  only upstream change was someone resolving a comment).
- **Batch size.** `documents.batchUpdate` accepts at most 500 requests. Under
  the limit, one atomic call. Over it, chunk in descending index order and warn
  that the operation is no longer atomic; record the last successful chunk so a
  failure can be resumed rather than replanned. Recomputing indices between
  chunks is not needed because chunks are disjoint and ordered back-to-front.
- **Quota.** One batch replaces hundreds of calls, which removes the problem
  rather than managing it. Retry with exponential backoff on 429 and 5xx
  regardless; the observed per-minute write quota took several minutes to clear
  after sustained use, so backoff must go to minutes, not seconds.

## 7. Scope and the escape hatch

**Supported for push:** paragraph text; bold, italic, strikethrough, inline
code; links; headings H1–H6; bullet and numbered lists including nesting;
footnote text; new footnotes; new and deleted paragraphs.

**Not supported for push:** tables, images, drawings, equations, page and
section breaks, chips, comments, suggestions, multi-tab restructuring, and any
character property markdown cannot express (font, size, colour, highlight).

These render on pull as today and are held in `opaque` blocks. If a local edit
changes one, `push` aborts with the construct name and line number, and
suggests the `gog` command to do it by hand. This is deliberate: a partial
implementation that silently mangles a table is worse than a refusal, and the
manual path through `gog` already works.

## 8. Credentials and the sandbox

Today `gdocs-cli` requests `documents.readonly` + `drive.readonly`, and the
sandboxed wrapper `gdocs-cli-ro` runs outside the sandbox with exactly those.
Writes go through `gog-agent`, which enforces a per-document grant
(`gog-agent-access allow docs <id>`).

Adding push means `gdocs-cli` needs `documents` (read-write). Requirements:

1. **Tiered scopes.** Pull must keep working with read-only credentials. Only
   `push` requests the write scope, and it must fail with a clear message —
   not a raw 403 — when handed a read-only token.
2. **Keep the ro/rw split at the wrapper level.** `gdocs-cli-ro` stays
   read-only. A separate `gdocs-cli-rw` carries write credentials, so the
   default agent-facing binary cannot mutate anything.
3. **Per-document grants.** Match `gog-agent`'s model: a write is allowed only
   against a document the user has explicitly granted. Otherwise moving sync
   into `gdocs-cli` quietly widens what an agent in the sandbox can do.
4. Note the existing inconsistency: `--upload-comments` already performs Drive
   writes (`Replies.Create`) while the OAuth config only asks for
   `drive.readonly`. That needs resolving as part of the scope work.

## 9. Integration test plan

Unit tests cover the model, escaping, diff and planner against synthetic
`docs.Document` values. They cannot catch the failures that actually hurt:
those come from the real API's behaviour around indices, footnote segments,
comment anchors and revision locking. So there is a live-document suite.

### 9.1 Harness

- Build tag `//go:build integration`, so `go test ./...` stays hermetic and
  offline.
- Gated on `GDOCS_CLI_IT=1` plus a credential path; skips with a clear message
  otherwise.
- Run with `just test-integration` (add to the existing `justfile`).
- Every test gets its **own** fixture document. No shared state, so cases can
  run in parallel up to a small limit (write quota is per user, not per doc —
  keep parallelism at 2–4).

### 9.2 Fixtures are built with gog, not with the code under test

Fixture setup goes through `gog-agent`. This is the point: if `gdocs-cli`
created its own fixtures, a bug in the writer would produce a document that
matches the same bug in the reader, and the test would pass.

```sh
# create; gog-agent auto-grants comments+edit-content on docs it creates,
# for roughly an hour, so no manual grant step is needed
DOC=$(gog-agent docs create "gdocs-cli-it-$(date +%s)-$RANDOM" --json | jq -r .file.id)

# seed the body from a markdown fixture
gog-agent docs write "$DOC" --replace --markdown --file testdata/fixture-basic.md

# add what --markdown cannot express
gog-agent docs format "$DOC" --match "continuous assurance at scale" --match-case --italic
gog-agent docs insert-footnote "$DOC" --at "human-written code." --file testdata/fn-1.txt
gog-agent docs comments add "$DOC" "anchor me"
```

Sandbox constraints the harness must respect, all observed in practice:

- `gog-agent` refuses to run when the working directory is outside the
  workspace ("CWD outside workspace"). Run it with `cmd.Dir` set to the repo
  root.
- `gog-agent` refuses `--file` paths outside the working directory. Fixture
  files must live under the repo, not in `/tmp`.
- `gog-agent batch` is not in the baked safety profile. Do not depend on it.
- `gog docs sed` matches only within a single text run. Do not use it to build
  fixtures that span styling boundaries; it will silently replace nothing.
- Editing a document the agent did not create requires
  `gog-agent-access allow docs <id>` on the host. Fixtures sidestep this by
  always creating their own document.

### 9.3 Lifecycle

```
create → seed → snapshot → [test body] → assert → trash
```

- `t.Cleanup` trashes the document. Teardown must also run on panic.
- A `TestMain` prune step trashes any doc named `gdocs-cli-it-*` older than
  24h, to mop up after crashed runs.
- Snapshot = `gog-agent docs raw "$DOC" --pretty` before and after, so a failing
  test can print a structural diff rather than just "strings differ".

### 9.4 Edit matrix

Each case: seed a fixture, `pull`, apply the described edit to the local file,
`push`, then assert. Cases marked ⚠ reproduce something that actually broke.

| # | Edit | Asserts |
|---|---|---|
| 1 | Change a word inside an italic phrase ⚠ | Phrase stays one italic run; no literal `*` anywhere in the doc |
| 2 | Italicise a previously plain phrase | One `updateTextStyle`, zero text mutations; word text byte-identical |
| 3 | Remove emphasis from a phrase | Style cleared; text untouched |
| 4 | Extend an emphasis span over adjacent plain words ⚠ | Resulting run is contiguous, not three runs |
| 5 | Type a literal `\*` in the local file | Doc contains one literal `*`, unstyled |
| 6 | No edit at all ⚠ | Zero requests; document revision unchanged |
| 7 | Add inline code around an identifier | `updateTextStyle` sets the mono font; no backticks in the doc text |
| 8 | Edit prose adjacent to existing inline code ⚠ | Code span keeps its original font family and size |
| 9 | Add, change, and remove a link | `fields: "link"` only; link text unchanged where only the URL moved |
| 10 | Change a heading's level | `updateParagraphStyle`; heading text and its `{#id}` anchor preserved |
| 11 | Turn a paragraph into a heading and back ⚠ | Round-trips; no stray heading left in the outline |
| 12 | Add and remove a list item; change nesting | `createParagraphBullets`/`deleteParagraphBullets`; siblings unaffected |
| 13 | Insert a paragraph mid-document | Lands between the right neighbours; following text not shifted into it |
| 14 | Delete a paragraph that contains a footnote ref ⚠ | Refusal, or footnote deleted deliberately — never an orphaned footnote |
| 15 | Edit footnote definition text | `segmentId` targeted; body untouched |
| 16 | Add a new footnote mid-sentence ⚠ | Reference lands at the exact anchor, not in its own paragraph |
| 17 | Add a footnote containing a link | URL is a real hyperlink in the footnote segment |
| 18 | Edit a paragraph containing a comment anchor ⚠ | Comment still anchored afterwards; `quotedFileContent` intact |
| 19 | Edit text with em-dashes, smart quotes and an emoji | Text exact; proves UTF-16 index arithmetic (emoji is a surrogate pair) |
| 20 | Edit a table cell | Refusal; document byte-identical afterwards |
| 21 | Edit text inside a code block | Refusal; document byte-identical afterwards |
| 22 | 300+ small edits across the document ⚠ | One `batchUpdate`; one revision bump; no 429 |
| 23 | 600+ edits | Chunked, warns about non-atomicity, all applied |
| 24 | Mutate the doc via `gog` between pull and push ⚠ | Push fails on revision lock; document unchanged |
| 25 | Push the same file twice | Second push is a no-op (case 6 after a real edit) |
| 26 | Push a file with a corrupted/absent `revision_id` | Clean error, no write |

### 9.5 Invariant assertions

Applied after every case that is expected to succeed:

1. **Round-trip.** Re-pull and compare to the local file byte-for-byte, except
   `revision_id`. This is the single strongest assertion and the one the
   current skill has no equivalent of.
2. **No literal markup leak.** The document's plain text contains no `*`, `` ` ``
   or `\` that was not in the fixture. This exact check would have caught the
   incident on its first run.
3. **Comment survival.** `gog-agent docs comments list` returns the same IDs
   and `quotedFileContent` as the pre-edit snapshot.
4. **Footnote conservation.** Footnote count and text match expectation; no
   orphaned references, no orphaned definitions.
5. **Style conservation outside the edit.** Diff the before/after `docs raw`
   snapshots and assert that every changed `textStyle` belongs to a range the
   plan intended to touch. Catches field-mask bugs (case 8) generically.
6. **Revision count.** Exactly one revision bump per successful push.

### 9.6 Property test

A non-live, fast test worth having alongside: generate random `docs.Document`
values (random styles, nesting, footnotes, unicode), assert invariant 1 from
§5, and assert that a random set of model edits plans to a request list whose
simulated application reproduces the target model. This catches planner bugs
without spending quota, and can run in normal CI.

### 9.7 CI

The live suite needs credentials, so it does not run on pull requests from
forks. Run it nightly and on demand, with a prune step first. Keep the property
test and unit tests on every PR.

### 9.8 Deliberately not covered

Multi-tab documents beyond selecting a tab; suggestion mode; documents with
revision history longer than the API returns; concurrent pushes from two
processes (the revision lock is tested, the race is not).

## 10. Rollout

1. Land the model refactor with `FromDocument`/`Render` behind the existing
   pull path. No behaviour change; §5 invariant 1 becomes testable.
2. Add `Parse` and the property test. Still no writes.
3. Add `push --dry-run`, which prints the planned requests as JSON. Run it
   against the real chapter and compare the plan to what a human would do.
4. Add `push` behind `--experimental`, with §9 green.
5. Switch the `sync-google-doc` skill to call `gdocs-cli push`, reducing it to
   a thin wrapper plus the manual `gog` escape hatch for §7 constructs.
6. Delete `sync_doc.py`'s diff/apply logic.

## 11. Open questions

1. `goldmark` or a hand-rolled parser for the restricted subset we emit? A
   hand-rolled parser guarantees renderer/parser symmetry but is more code to
   get wrong on edge cases.
2. Should comment anchors in the local file be editable, or strictly read-only
   markers that `push` refuses to relocate? Read-only is safer and is assumed
   above.
3. How should `push` treat local edits inside `opaque` blocks that are
   whitespace-only? Refuse uniformly, or normalise?
4. Is a `--force` that diffs against the fetched revision (§6) worth the risk,
   or should a moved revision always mean re-pull?
5. Do we want a `pull --check` that reports whether the local file has
   uncommitted divergence from upstream, for use in pre-commit hooks?
