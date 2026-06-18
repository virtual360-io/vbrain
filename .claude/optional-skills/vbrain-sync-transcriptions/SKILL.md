---
name: vbrain-sync-transcriptions
description: Scans Gmail for Google Meet transcripts shared as Google Docs ("transcript-<uuid>"), ingests each one into vbrain as kind google-transcript (immutable raw + topic chunks + wiki pages), and — only after a successful ingest — archives and marks the email read. Use when the user asks "sync my transcriptions", "ingest my meetings", "vbrain-sync-transcriptions", or from the daily routine.
allowed-tools: Bash, Read, Write, Agent, AskUserQuestion, Skill, mcp__claude_ai_Gmail__search_threads, mcp__claude_ai_Gmail__get_thread, mcp__claude_ai_Gmail__unlabel_thread, mcp__claude_ai_Google_Drive__read_file_content, mcp__claude_ai_Google_Drive__download_file_content
---

# vbrain-sync-transcriptions

**Optional skill** (installed via `vbrain skill install vbrain-sync-transcriptions`).
A thin orchestrator: it discovers Google Meet transcript emails, hands each
transcript to the deterministic `vbrain` ingest pipeline (source type
`google-transcript`), and only archives the email once the ingest succeeded.

The transcript itself is NOT in the email body (that's just a share notice / a
summary snippet). The full transcript is a **Google Doc** named
`transcript-<uuid>`, linked in the body. We ingest the Doc, never the snippet.

Determinism (Rule 5): discovery, FILE_ID extraction, ingest and archiving are
deterministic; only the chunking of the transcript into pages is a judgment
sub-agent (it lives inside `/vbrain-add-knowledge`).

## Inputs

- none — the default behavior is "sync everything pending in the inbox".

## Step 0 — Preconditions

1. Base is a git repo: `test -d ~/vbrain/.git && echo present || echo absent`.
   If absent, run `vbrain __bootstrap` first (or tell the user to run
   `/vbrain-add-knowledge` once to set it up).
2. Gmail + Drive MCP are authenticated. Probe with a light call
   (`mcp__claude_ai_Gmail__search_threads` with the query below). If the response
   is something like "ask the user to run /mcp and select 'claude.ai Gmail'",
   STOP and instruct:
   > "To sync transcriptions, open `/mcp` in Claude Code, authorize **claude.ai
   > Gmail** and **claude.ai Google Drive**, then call `/vbrain-sync-transcriptions`
   > again."
   Don't try to bypass it.

## Step 1 — Find pending transcripts

```
mcp__claude_ai_Gmail__search_threads
  query: from:drive-shares-dm-noreply@google.com subject:transcript in:inbox
```

- `from:drive-shares-dm-noreply@google.com` — the Drive share notice for a Meet
  transcript Doc.
- `subject:transcript` — Meet transcript Docs are named `transcript-<uuid>`, so
  this discriminates them from any other shared Doc. It's language-agnostic (the
  filename, not localized chrome).
- `in:inbox` — "pending": once we ingest a transcript we archive the email (drop
  the `INBOX` label), so anything still in the inbox is unprocessed.

Each returned thread is a candidate. If there are none, report "no pending
transcripts" and stop.

## Step 2 — Process each thread, ONE sub-agent at a time, sequentially

> **Hard rule:** never launch two ingestion sub-agents in the same message.
> Process each transcript fully (download → ingest → pages → commit → archive)
> before the next starts. Parallelizing breaks the wiki link graph and can make
> two writers clobber the same page. (Same rule as `/vbrain-add-knowledge`.)

For each thread, launch ONE `Agent` (`subagent_type: claude` — it needs Skill +
MCP). The sub-agent isolates the big Doc (~75k chars) from the main context. Give
it this instruction:

> You process ONE Google Meet transcript email into vbrain. Steps:
>
> 1. `mcp__claude_ai_Gmail__get_thread(threadId=<ID>, messageFormat=FULL_CONTENT)`.
>    Read `plaintextBody` of the first message.
> 2. Extract the Doc FILE_ID with the pattern
>    `docs\.google\.com/document/d/([A-Za-z0-9_-]+)`. Take the first match.
>    If there's no match → return `{"threadId":"<ID>","status":"no_doc_link"}`
>    and DO NOT archive.
> 3. Download the Doc's text and save it to `/tmp/vbrain-transcript-<FILE_ID>.vtt`:
>    - **Preferred (full fidelity):**
>      `mcp__claude_ai_Google_Drive__download_file_content(fileId=<FILE_ID>,
>      exportMimeType="text/plain")`. Its `content` field is **base64** — you MUST
>      decode it before saving (e.g. write the base64 to `/tmp/<FILE_ID>.b64`, then
>      `base64 -d /tmp/<FILE_ID>.b64 > /tmp/vbrain-transcript-<FILE_ID>.vtt`). If you
>      save the base64 as-is, ingest gets base64 and the WEBVTT detection fails.
>    - **Fallback:** `mcp__claude_ai_Google_Drive__read_file_content(fileId=<FILE_ID>)`
>      returns the text directly (no base64; the Go parser tolerates the markdown
>      escaping it adds), but it may truncate very large Docs — prefer the download
>      for long meetings. Save with the `Write` tool (not shell redirection).
> 4. Ingest by running the `/vbrain-add-knowledge` skill on
>    `/tmp/vbrain-transcript-<FILE_ID>.vtt`. `vbrain ingest` auto-detects the
>    `google-transcript` source type from the `WEBVTT` header and uses the
>    transcript chunker. **You are in non-interactive automation:**
>    - If ingest reports `{"duplicate":true}` → this transcript is already in the
>      base. Do NOT reprocess, do NOT ask anything. Treat as success
>      (status `duplicate`).
>    - If the chunker yields 0 chunks (trivial meeting: audio test, small talk) →
>      the raw is committed as an audit log (normal add-knowledge behavior).
>      Treat as success (status `trivial`).
>    - On any real failure (Drive read error, ingest/write-pages/commit error) →
>      status `ingest_failed` with the message; DO NOT archive.
> 5. **Only on success** (`ok`, `trivial`, or `duplicate`):
>    `mcp__claude_ai_Gmail__unlabel_thread(threadId=<ID>, labelIds=["INBOX","UNREAD"])`
>    — archives and marks read in one call.
> 6. Return JSON: `{"threadId","status":"ok|trivial|duplicate|no_doc_link|ingest_failed",
>    "pages_created":<n>,"archived":<bool>}`.

Collect each sub-agent's JSON before starting the next thread.

## Step 3 — Report

Summarize across all threads: processed, pages created (sum), archived,
skipped (`duplicate`/`trivial`), and failures (`no_doc_link`/`ingest_failed`)
with their threadIds so the user can inspect them. Be explicit about anything
left in the inbox (Rule 12 — fail loud).

## Daily routine (offered at install time)

This skill ships a `routine.yml`, so `vbrain skill install vbrain-sync-transcriptions`
**offers the daily routine right away**:

- On a terminal it asks for a time (`HH:MM`) and creates it (deterministic
  `HH:MM`→cron).
- When an **agent** drives the install, the install output carries
  `suggested_routine` instead — so if you (the agent) just installed this skill,
  ask the user what time they want and create the routine via `/vbrain-add-routine`
  (natural-language time → cron, confirmed). Don't silently skip it.

Either way the watch loop `/vbrain-routine` fires it. The routine is:

- **slug**: `sync-transcriptions`
- **schedule**: whatever time the user gives (e.g. `0 7 * * *`)
- **prompt**:
  > Invoke the `/vbrain-sync-transcriptions` skill (no args). It searches Gmail
  > for unprocessed Google Meet transcripts, ingests each sequentially, and
  > archives + marks read only the ones that ingested successfully. Return its
  > report: processed, pages created, archived, skipped, failures.

Do not seed the routine from code and do not hardcode a time — it's created
interactively when the user activates this skill.

## Hard rules

- **Archive only after a successful ingest.** `no_doc_link` and `ingest_failed`
  stay in the inbox for the next run. Never archive before the pipeline succeeds.
- **One sub-agent per email, sequential.** Never batch; never parallelize.
- **Ingest the Doc, not the email body.** The body is a share notice / summary;
  the transcript is the linked Google Doc.
- **Never** write into `wiki/`/`raw/` by hand — it all goes through
  `/vbrain-add-knowledge` → the `vbrain` binary.
