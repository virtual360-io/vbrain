---
name: vbrain-add-knowledge
description: Ingests a file or URL into vbrain — copies it to raw/, chunks it via a sub-agent, generates grounded wiki pages, reindexes SQLite FTS5. Use when the user asks "save this to vbrain", "add to the base", or provides a notes/markdown file, a URL (article, blog), or a tweet/X article to archive.
allowed-tools: Bash, Read, Write, Agent, AskUserQuestion, WebFetch
---

# vbrain-add-knowledge

Deterministic pipeline (Go) + 2 LLM sub-agents (chunker + wiki-writer) to turn a
raw file into wiki pages indexed in vbrain.

## Inputs

- **path** (required): absolute path to the file OR a URL (http/https).
- **--type** (optional): force the `source_type` (`text` | `url` | `tweet`)
  when the heuristic detection is wrong. Only include it if the user asks.

## Supported sources

| `source_type` | Detection | How it extracts |
|---|---|---|
| `tweet` | URL `twitter.com|x.com/<user>/status/<id>` | Public syndication endpoint (`cdn.syndication.twimg.com`) + headless Chrome to pull the full body of linked X Articles |
| `url` | Other http(s) URLs | Jina Reader (`r.jina.ai`) — returns clean markdown |
| `text` | `.md`, `.txt`, extensionless + UTF-8 | passthrough |

## Steps

### 0. Ensure a git repo in `~/vbrain` (first ingest only)

Before anything else, check whether `~/vbrain/.git/` exists:

```bash
test -d ~/vbrain/.git && echo present || echo absent
```

**If absent** (first ingestion into the base), use `AskUserQuestion`:

> "Your base isn't a git repository yet. I'd like to version wiki/raw so you can
> revert/sync across machines. What do you prefer?"
>
> 1. **Private repo on GitHub** (Recommended)
> 2. **Public repo on GitHub**
> 3. **Local git only, no GitHub**
> 4. **Skip versioning for now**

Depending on the answer, run:

- Private: `vbrain setup --github private`
- Public: `vbrain setup --github public`
- Local: `vbrain setup`
- Skip: don't run anything, and note mentally that step 6 should be skipped.

Parse the JSON:

- `{"initialized":true,...,"remote_url":"..."}` → repo ready, proceed.
- `{"initialized":false,"reason":"already a repo"}` → idempotent, proceed.
- `{"needs_token":true}` → no GitHub PAT available. Use `AskUserQuestion` to ask
  whether the user wants to provide a PAT (scope `repo`). If yes, re-run
  `vbrain setup --github <vis> --token <PAT>`. If no, fall back to "Local git
  only" (`vbrain setup` without `--github`).

**If present**: go straight to step 1.

### 1. Ingest the raw

```bash
vbrain ingest <path>
```

Parse the output JSON. Possible cases:

- `{"source_type":"text"|"url"|"tweet","raw_id":N,"raw_path":...,"extracted_path":...}` → proceed to step 2.
- `{"duplicate":true,"raw_id":N,...}` → this file's sha256 already exists. Ask
  the user whether to reprocess (`--force`) or abort.
- `{"source_type":"unknown",...}` **OR** the deterministic extraction returned
  trivial content (< 100 useful words, login wall, "Sign in", "Continue with
  Apple", etc.):
  - **DEFAULT: via LLM.** Don't ask the user whether it's worth creating a
    permanent source. Use `WebFetch` (for URLs) or a generic extractor sub-agent
    (for files) to produce plain text in `raw/.tmp/extracted-<raw_id>.txt`.
    Update the `raw_sources` row to `source_type='oneshot'`. Then proceed to
    step 2 using `prompts/chunker/text.md` as the chunker.
  - **Special case — X/Twitter links** (tweets, X Articles, gated posts): do NOT
    attempt additional deterministic builds. Do NOT suggest a dedicated source.
    X blocks scraping consistently; the path that always works is `WebFetch`
    reading the URL the way a human would, and using the result as the chunker
    input. The same applies to any site with auth/paywall.
  - Only suggest "create a permanent source" if the user explicitly asks ("I
    want to ingest many files of this format, is it worth a source?").

### 2. Chunk (sub-agent)

Read `extracted_path`. Launch a `general-purpose` sub-agent with the content of
`prompts/chunker/<source_type>.md` as the system prompt and the extracted text
as the user message. Require strict JSON output.

Expected schema:

```json
{"chunks":[
  {"suggested_title":"...",
   "kind":"concept|decision|gotcha|note|rule",
   "tags":["..."],
   "raw_excerpt":"literal excerpt",
   "summary_hint":"1 neutral sentence"}
]}
```

If `chunks` comes back **empty**, do NOT abort yet — follow step 2b.

### 2b. Extraction fallback (when the chunker returns 0 chunks)

It means the deterministic extraction didn't yield durable content. Try in
order, **stopping at the first one that produces chunks > 0**:

1. **Follow embedded links** (essential for tweets that are just an article
   link): Open `extracted_path` and look for a `## Cited links` or `## References`
   section with URLs, or a dominant URL in the body. For each URL found (limit to
   the 3 most relevant — prioritize articles/blogs over already-expanded
   `t.co`/shorteners):
   - Run `WebFetch` on the URL with the prompt:
     > "Extract the article's main content as clean markdown (title, author if
     > any, full body preserving structure). If it's a login wall or a redirect
     > to Sign in, report it explicitly."
   - Concatenate the responses with `## <URL>` headings into
     `raw/.tmp/extracted-<raw_id>-followed.txt`.
   - Re-run the chunker sub-agent with `prompts/chunker/text.md` (no longer
     `tweet.md` — it's article content now) and the new file. If it produces
     chunks, proceed.

2. **Wayback Machine** (if WebFetch gave an HTTP 4xx/5xx error or a login wall):
   For each URL that failed, try `https://web.archive.org/web/<URL>` via
   `WebFetch`. Save into the same `extracted-<raw_id>-followed.txt`. Re-run the
   chunker.

3. **Ask the user** (last resort): Use `AskUserQuestion` to ask whether they
   want to:
   (a) Paste the article content in the next message — you save it into
       `extracted-<raw_id>-followed.txt` and re-run the chunker.
   (b) Discard the ingestion (step 6 only commits the raw as an audit log).
   (c) Try a different URL.

4. **Honest abort**: if the user discards, skip steps 3-5 but follow step 6
   (commit the raw as audit; explicitly report "no page created").

### 3. Write wiki (sub-agent, **strictly one chunk at a time**)

Process the chunks **one at a time, in sequence — NEVER in parallel**. Each
chunk goes through the full **writer → persist → reindex** cycle before the next
one starts. That's what lets the next writer see (and merge with) the pages the
previous ones created: it navigates the **already-updated** index.

> **Hard rule:** never launch two writers in the same message. One writer, one
> `write-pages`, one `reindex`, and only then the next chunk. Parallelizing
> breaks graph navigation and can make two chunks clobber the same page.

For **each chunk**, in order, do the cycle:

1. **Writer** — launch ONE `general-purpose` sub-agent (with Bash + Read) using
   `prompts/wiki-writer.md` as the system prompt. In the user message pass: the
   chunk (chunker JSON), the wiki's absolute path (`<data_home>/wiki`), the
   `source_raw`, and the navigation command
   `vbrain query "<terms>" --format json --limit 8`.
   The writer returns ONE JSON: `op` (`create`|`update`), `slug` (when
   `update`), `title`, `tags`, `kind`, `body_markdown` (the **final, complete**
   body).
2. **Persist** — write that single page-object to `raw/.tmp/page-<raw_id>.json`
   (`{"pages":[<the object>]}`) and run step 4.
3. **Reindex** — run step 4b (linkify) and step 5 (reindex) **now**, so the next
   writer sees this page in the index.

Only after reindexing, move to the next chunk. Don't accumulate pages for a
batch `write-pages` — the cycle is per chunk.

### 4. Persist the page (staging + atomic publish)

```bash
vbrain write-pages --raw-id <N> --pages-json raw/.tmp/page-<N>.json
```

The command stages the page (whole body) in `raw/.tmp/wiki-stage-<N>/` and only
then moves it into `wiki/` via rename — the wiki is never left half-written.

- `op: "create"` → new file (slug derived from the title; `-2` suffix only on a
  real collision with a different subject).
- `op: "update"` → overwrites the whole body of the existing page (`slug`) and
  merges the frontmatter (union of `tags`; `source_raw` accumulates the new
  raw). If the `slug` doesn't exist, it falls back to `create` (anti-
  hallucination defense).

The JSON output has `{"written":[...],"updated":[...],"count":N}`. Knowledge
pages live at the **root** of `wiki/` (flat space, no per-type folders).

### 4b. Linkify (deterministic) + resolve unresolved (LLM)

```bash
vbrain linkify
```

Converts the `[[wikilinks]]` resolvable by exact slug into markdown links
`[Title](slug.md)` — **navigable on GitHub and Obsidian** (GitHub doesn't render
`[[ ]]` in repo files). Deterministic, idempotent, preserves the frontmatter
verbatim. Output: `{"changed":N,"scanned":N}`.

After `vbrain reindex` (step 5), the links that remained **unresolved** (the
title's slug doesn't match any page) stay in the `links` table with
`to_page_id NULL`. To strengthen the graph, a **judgment layer (LLM)** decides
which existing page each one refers to:

1. List the unresolved ones and the page index:
   `SELECT DISTINCT target_title FROM links WHERE to_page_id IS NULL` +
   `SELECT path, title FROM pages`.
2. Launch a sub-agent that, for each `target_title`, picks the `slug` of the
   existing page it references (or `null` if none fits — **don't invent**).
   Output: JSON `{"Title": "slug" | null}`.
3. Apply deterministically and reindex:
   ```bash
   vbrain resolve-links --map raw/.tmp/linkmap.json
   vbrain reindex
   ```
   `vbrain resolve-links` discards slugs that don't exist (anti-hallucination
   defense).

### 5. Reindex

```bash
vbrain reindex
```

The output has `{"inserted":N,"updated":N,"deleted":N,"links":N}`. The index is
purely SQLite (`pages` + virtual `pages_fts` + the `links` table); AI/AD/AU
triggers sync FTS5 automatically. Besides reindexing the FTS, `vbrain reindex`
**parses the `[[wikilinks]]` from the body and rebuilds the graph** in the
`links` table (a nonexistent target → an edge with `to_page_id` NULL, resolved
in a future reindex). There is no `wiki/index.md` — we mirror ai-memory: the
structure is the link graph + the derived SQLite, not a folder tree.

### 6. Commit + push

If step 0 wasn't skipped:

```bash
vbrain commit --message "add: <N> pages from <basename>"
```

Where `<N>` is the `count` returned in step 4 and `<basename>` is the short name
of the original file/URL. The command is idempotent:

- If there are no changes in `~/vbrain/` (degenerate case: the chunker returned 0
  pages), it returns `{"committed":false,"reason":"no changes"}`.
- If there are changes, it commits and tries to push automatically when there's
  an `origin` configured (`{"pushed":true}`).
- Without a remote, it only commits locally (`{"pushed":false,"reason":"no remote"}`).

Don't interrupt the pipeline if the push fails — the local commit was made;
report the error and proceed.

### 7. Report to the user

Show:
- The detected type (`source_type`)
- Pages **created** (`written`) and **updated** (`updated`) in `wiki/`
- Database stats: `vbrain stats` (returns JSON with total pages, distribution by
  kind, and the 5 most recent)
- Commit/push status (sha + remote URL when applicable)

## Hard rules

- **Never** write into `wiki/` directly; always via `vbrain write-pages`.
- **Never** touch the soul layer (`wiki/_soul/`). That folder is the user's
  identity — how and *why* they act — and is written **only** by the daily `soul`
  routine after it consolidates their actions. This skill ingests knowledge, not
  beliefs: never set `kind: soul`, never target a `_soul/` slug, never call
  `vbrain soul-write`. Reading a source is not adopting it as a belief.
- **Never** modify `raw/` after ingestion — it's immutable.
- If `go test` fails before the final run (case 1 of the unknown type), do
  **not** proceed until the user fixes it.
- **FAITHFULNESS**: the sub-agents have a hard rule not to invent; if they
  return clearly fabricated data, flag it to the user.
- **Tweet with a link to an article → follow the link**: when the ingested tweet
  has little/no text of its own but references an external URL (article, blog,
  thread on another site), the user wants **the article's content**, not just
  the tweet metadata. Step 2b is mandatory in that case — don't finish with "no
  pages" if there's a link to follow.
- **Fallback attempts are ordered and exhaustive**: direct WebFetch → Wayback
  Machine → ask the user manually. Never jump to "abort" without exercising the
  chain.
