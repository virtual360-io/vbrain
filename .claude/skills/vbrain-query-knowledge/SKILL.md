---
name: vbrain-query-knowledge
description: Queries the vbrain base (SQLite FTS5) and returns relevant excerpts in markdown. kind=realtime pages trigger live handlers (Google Calendar, Gmail, and Slack via MCP) instead of returning the body. Use when the user asks something that might be archived ("what do I know about X", "search vbrain for Y"), or when another agent needs persisted context for a task.
allowed-tools: Bash, Read, mcp__claude_ai_Google_Calendar__list_events, mcp__claude_ai_Gmail__search_threads, mcp__claude_ai_Gmail__get_thread, mcp__claude_ai_Slack__slack_search_public_and_private, mcp__claude_ai_Slack__slack_read_thread
---

# vbrain-query-knowledge

Read skill: runs `vbrain query` against the FTS5 index, formats the result, and
for `kind: realtime` pages fires the corresponding MCP handler (resolves live).

## Inputs

- **query** (required): free-form question or keyword. May contain `:`, quotes,
  parentheses — the normalizer handles it.
- **limit** (optional, default 10): max number of pages to return.

## Steps

### 0. Query expansion (NL → vocabulary bridge) — **only when the query is natural language**

FTS5 is lexical: the question `"quais empregos eu já tive"` ("what jobs have I
had") matches nothing because the word "emprego" isn't in the corpus (the pages
say "Visagio", "consultant", "career"). vbrain already strips stopwords, but it
doesn't invent synonyms — that's your judgment.

Skip this step if the `query` is already keyword(s) (1–3 technical terms, a
proper noun, a slug). Do it for natural-language questions.

1. Pull the base's real tag vocabulary:

```bash
vbrain tags --limit 60
```

(`vbrain tags` already returns JSON on stdout — there's no `--format`.)

2. Rewrite the question into a handful of **content terms** (4–8), biased by the
   vocabulary above. Include synonyms/inflected forms and, when available, the
   tag(s) matching the intent. E.g. `"quais empregos eu já tive"` →
   `career work consultant intern company role`. Don't include stopwords or the
   question's own word if it doesn't exist in the corpus. Don't invent proper
   nouns you haven't seen.
3. Use those terms as `<query>` in steps 1–2, and **always** pass the original
   question in `--source-query` (the `query_log` keeps both — it's what the
   `dream` routine analyzes to reorganize the wiki).

### 1. FTS5

```bash
vbrain query "<query>" --limit <N> --source-query "<original question>" --format json
```

Parse `results`. Each item has `path`, `title`, `kind`, `snippet`.
If `results` comes back empty, try step 2 (prefix). Otherwise skip to 3.

(If you didn't expand — the query was already a keyword — pass the query itself
in `--source-query` too, or omit it: the log uses `query` in that case.)

### 2. Prefix-matching fallback

```bash
vbrain query "<query>" --limit <N> --prefix --no-log --format json
```

(`--no-log` here: step 1 already recorded the intent; the prefix retry should
not duplicate the line in `query_log`.)

If still empty: "No results found for `<query>` in the vbrain base. Try more
general terms or check whether something was ingested with
`/vbrain-add-knowledge`."

### 3. Realtime page dispatch

For each result with `kind == "realtime"`, do **not** show the snippet: call the
corresponding live handler. Read the page's full frontmatter at
`~/vbrain/wiki/<path>` to find `source` and its parameters.

| `source`    | Handler                                                      |
|---|---|
| `gcalendar` | `mcp__claude_ai_Google_Calendar__list_events` (see 3a)       |
| `gmail`     | `mcp__claude_ai_Gmail__search_threads` (see 3b)              |
| `slack`     | `mcp__claude_ai_Slack__slack_search_public_and_private` (3c) |
| other       | report "realtime source `X` has no handler implemented"      |

**3a. `gcalendar` handler:**

Read the page frontmatter, take the `calendars` list (each with `id`, `summary`,
`timezone`). For each calendar, call:

```
mcp__claude_ai_Google_Calendar__list_events
  calendar_id = <id>
  time_min    = <range start>
  time_max    = <range end>
```

Range derived from the query (use the current date = today):

| Term in the query                 | Range                               |
|---|---|
| "hoje", "today"                   | 00:00 → 23:59 today                 |
| "amanhã", "tomorrow"              | 00:00 → 23:59 tomorrow              |
| "essa/esta semana", "this week"   | today → Sunday of this week 23:59   |
| "próxima semana", "next week"     | next Mon → next Sun                 |
| "mês", "this month"               | today → end of the current month    |
| "fim de semana", "weekend"        | nearest Sat 00:00 → Sun 23:59       |
| no explicit temporal term         | next 7 days (today → +7d)           |

Use the calendar's timezone (or `America/Sao_Paulo` as fallback) when building
the ISO 8601 `time_min/time_max`.

For each returned event, format:

```
- HH:MM–HH:MM | <summary> [<calendar.summary>]
  <location, if any>
  <short description, if any>
```

If the query mentions someone specific (e.g. "meeting with So-and-so"), filter
events whose `summary`, `description`, or `attendees` contain the name.

**3b. `gmail` handler:**

Read the page frontmatter, take the `labels` list (each with `id`, `name`).
Build the `search_threads` `query` in **Gmail search syntax**:

1. **Label filter** (always prepended): `(label:<id1> OR label:<id2> …)`.
   For a single label, use `label:<id>` without parentheses. If the labels list
   is empty (degenerate), don't prepend.
2. **Content**: extract the meaningful terms from the user's query and convert
   to Gmail syntax:
   - Names/emails → `from:` or `to:` if the query says who sent/received.
     ("email from João" → `from:João`; without direction, try
     `(from:João OR to:João)`.)
   - Relative dates:
     - "today" → `newer_than:1d`
     - "yesterday" → `newer_than:2d older_than:1d`
     - "this week" → `newer_than:7d`
     - "last week" → `newer_than:14d older_than:7d`
     - "this month" → `newer_than:30d`
   - Absolute dates → `after:YYYY/MM/DD before:YYYY/MM/DD`.
   - Attachments → `has:attachment`.
   - Unread → `is:unread`.
   - Explicit subject → `subject:"<phrase>"`.
   - Remaining keywords go loose (AND by default).
3. Call:

```
mcp__claude_ai_Gmail__search_threads
  query    = "<label filter> <content>"
  pageSize = min(20, query-knowledge limit)
```

For each returned thread, format:

```
- <short date> | <from> → <subject>
  <snippet>
```

Use the thread's last message as `from`/`subject`/`snippet` (the response
already includes the fields). If the thread has several messages, show the count
in parentheses: `(N msgs)`.

If nothing comes back, report: "No thread matches `<built query>`. Try more
general terms or widen the time range."

If the user asks for the full body of a specific thread ("open that one", "show
the last email in full"), call `mcp__claude_ai_Gmail__get_thread` with the
`threadId` and `messageFormat: FULL_CONTENT`.

**3c. `slack` handler:**

Read the page frontmatter, take the `channels` list (each with `id`, `name`).
Build the `slack_search_public_and_private` `query` in **Slack search syntax**.
Note: Slack search **has no `OR` operator** (space = AND) — so the multi-channel
filter does NOT become a single query.

1. **Content**: extract the meaningful terms from the user's query and convert
   to Slack syntax:
   - Who sent it → `from:@username` (no `OR`; if unsure who, leave the name
     loose as a keyword).
   - Relative dates → `after:YYYY-MM-DD` / `before:YYYY-MM-DD` / `on:YYYY-MM-DD`
     (compute from the current date).
   - Attachments/files → `has:file` (or `content_types: "files"` if the question
     is explicitly about files).
   - Threads → `is:thread`.
   - Exact phrase → `"in quotes"`.
   - Remaining keywords go loose (AND by default).
2. **Channel filter**:
   - If `channels` is **empty** (global mode): make **one** call without `in:`,
     searching the whole workspace.
   - If `channels` has items: make **one call per channel**, prepending
     `in:<#ID>` (use the `id`; fall back to `in:#name` only if the id is
     missing) to the content. Merge the calls' results and sort by date
     (`sort: "timestamp"`) or relevance.
3. Call (per channel, or once globally):

```
mcp__claude_ai_Slack__slack_search_public_and_private
  query    = "<in:<#ID> if filtered> <content>"
  limit    = min(20, query-knowledge limit)
  sort     = "timestamp"  (or "score" if the query is by relevance)
```

For each returned message, format:

```
- <short date> | <#channel> <author> → <text/snippet>
```

If nothing comes back, report: "No message matches `<built query>` in Slack
(<global mode | channels X, Y>). Try more general terms or widen the time
range."

If the user asks for a specific full thread ("open that conversation"), call
`mcp__claude_ai_Slack__slack_read_thread` with the `channel_id` and the parent
message's `message_ts`.

### 4. Format the response

Keep the **FTS5 order** (SQLite rank): if the realtime page landed 3rd, show the
2 static ones first, then the realtime block, then the rest.

For static results:
- Title + path `wiki/<path>`
- Snippet (already comes with `**term**` highlighted)
- Tags if relevant

For realtime results:
- Header: `## <Title> (realtime — <source>)`
- Block rendered by the handler

When the caller is another agent (heuristic: the question came from a `Task`),
read the full file of the top 3 static results and include the markdown body.

## Rules

- **Don't modify** anything — this skill is read-only.
- If the query has `< 3 characters` of significant content, ask for a more
  specific query before running.
- Don't invent content: a bad snippet → "these pages mention the term but may
  not answer it directly".
- For realtime, if the MCP fails (not connected, no permission), report it
  explicitly: "couldn't query `<source>` live: <error>; reconnect via
  `/vbrain-add-realtime-knowledge`". Never fall back to the body snippet — the
  body only has keywords, it's not an answer.
