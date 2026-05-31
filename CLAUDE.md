# CLAUDE.md — vbrain

These rules apply to any task in this repo, unless explicitly overridden.
Bias: caution > speed for anything non-trivial. Use judgment on trivial tasks.

Short context: this repo is a personal knowledge base in the ai-memory style.
**Markdown wiki is the source of truth; SQLite is a derived index — disposable
(you can delete it and rebuild with `vbrain reindex`), but versioned alongside
the base for convenience; the LLM only steps in for what needs judgment
(chunking, synthesizing pages)**. See `README.md` for the full architecture.

Stack: **Go**. The deterministic core is a single `vbrain` binary — code in
`cmd/vbrain/` (CLI subcommands) + `internal/<pkg>` (logic), `go test` 1:1 per
package. SQLite via `modernc.org/sqlite` (pure-Go, FTS5 embedded); git via
go-git, falling back to the system git when present. Skills in `.claude/skills/`
call `vbrain <subcommand>` (binary on the PATH).

## Rule 1 — Think Before Coding

State assumptions explicitly. If unsure, ask instead of guessing.
Present multiple interpretations when there's ambiguity.
Push back when there's a simpler path.
Stop when confused. Name what's unclear.

In vbrain: before touching `internal/db` (`SchemaSQL`) or the schema, state what
you think the index is doing today and why — the schema is the most expensive
place to get wrong.

## Rule 2 — Simplicity First

Minimal code that solves the problem. Nothing speculative.
No features beyond what's asked. No abstractions for single-use code.
Test: would a senior call this overengineering? If so, simplify.

In vbrain: this repo follows ai-memory and is deliberately shallow. Skills + Go
+ SQLite, period. Don't introduce layers (cache, queue, ORM, DSL) without an
explicit request.

## Rule 3 — Surgical Changes

Touch only what you need. Clean up only your own mess.
Don't "improve" adjacent code, comments, formatting.
Don't refactor what isn't broken. Match the existing style.

In vbrain: a bug in `sources.Twitter` doesn't justify reformatting `sources.URL`.

## Rule 4 — Goal-Driven Execution

Define success criteria. Iterate until verified.
Don't follow steps — define success and iterate.
Strong success criteria let you iterate on your own.

In vbrain: "tweet-with-link ingest works" means `go test ./...` green +
`/vbrain-add-knowledge <url>` producing pages in `wiki/` whose `path` shows up in
`vbrain query`. Don't stop before those three.

## Rule 5 — Use the model only for judgment

Use the LLM for: classification, drafting, summarization, extraction.
Do NOT use the LLM for: routing, retries, deterministic transforms.
If code can answer, code answers.

In vbrain this is an architectural rule, not a tip: chunker and wiki-writer are
sub-agents because they need judgment; `internal/ingest`, `internal/writepages`,
`internal/index`, `internal/search` (exposed by the `vbrain ingest`,
`write-pages`, `reindex`, `query` subcommands) are deterministic Go with `go
test` 1:1. Detecting source_type, normalizing the FTS5 query, writing
frontmatter, building SQL — all Go. Never delegate to a sub-agent what code can
do.

## Rule 6 — Token budgets are not a suggestion

Per task: 4,000 tokens. Per session: 30,000 tokens.
If you're nearing the limit, summarize and restart.
Surface the overflow. Don't blow past it silently.

In vbrain: chunker and wiki-writer prompts live in
`.claude/skills/vbrain-add-knowledge/prompts/` — if they bloat, refactor the
prompt before raising the budget.

## Rule 7 — Conflicts: pick, don't mix

If two patterns contradict, pick one (most recent / best tested).
Explain why. Flag the other for cleanup.
Don't mix conflicting patterns.

In vbrain: if one source in `internal/sources` does X and another does Y for the
same problem, don't invent Z combining the two — take the pattern covered by
more tests.

## Rule 8 — Read before writing

Before adding code, read exports, immediate callers, shared utilities.
"Looks orthogonal" is dangerous. If you don't understand why the code is
structured a certain way, ask.

In vbrain, before editing a source in `internal/sources`: read the
`Source`/`Ingestable` interface, the `Registry` (dispatcher in `sources.go`),
and the corresponding `*_test.go`. Before touching `internal/page` or
`internal/db`: check the callers (`internal/writepages`, `internal/index`).

## Rule 9 — Tests verify intent, not just behavior

Tests must encode WHY the behavior matters, not just what.
A test that can't fail when the business rule changes is wrong.

In vbrain: every deterministic package under `internal/` has a corresponding
`*_test.go`. This is a **hard rule** — no code lands without a test. Isolate
test data with `t.Setenv("VBRAIN_HOME", t.TempDir())` (or pass explicit dirs),
never touching the real base.

## Rule 10 — Checkpoint after each significant step

Summarize what was done, what's verified, what's left.
Don't continue from a state you can't describe back.
If you lose the thread, stop and restate.

In vbrain: the ingest pipeline has 7 steps — after each one, say what came out
(path, raw_id, count) before moving to the next. Don't reach `vbrain commit`
without having stated how many pages `vbrain write-pages` produced.

## Rule 11 — Match the codebase conventions, even when you disagree

Conformance > taste, within the codebase.
If you genuinely think a convention is harmful, flag it. Don't fork silently.

In vbrain there are conventions that look opinionated but are intentional:

- The `vbrain` subcommands return **JSON** on stdout (read by the skills) and
  human-readable text on stderr. Don't invert this.
- The wiki is written **only** by `vbrain write-pages` (`internal/writepages`).
  Skills never write markdown directly into `wiki/`.
- `raw/` is **immutable** once written. If the content needs to change,
  re-ingest.
- There is no `wiki/index.md`. The index is SQLite. Don't try to recreate one.
- SQLite (`db/vbrain.sqlite3`) **is versioned** (not in `.gitignore`): a derived,
  disposable index, but committed for convenience. Deleting `db/` +
  `vbrain reindex` rebuilds everything. Don't re-add `/db/` to the ignore list.

## Rule 12 — Fail loud

"Done" is wrong if something was silently skipped.
"Tests pass" is wrong if any was skipped.
Default: surface uncertainty, don't hide it.

In vbrain: if the chunker returned 0 pages, **report** "no page created, raw
committed as an audit log" — don't run `vbrain stats` and show only the totals
as if everything worked. If a sub-agent fabricated content without grounding,
flag it to the user before persisting.
