---
name: vbrain-add-realtime-knowledge
description: Connects a "realtime knowledge" source to vbrain (today: Google Calendar, Gmail, Slack, GitHub, and Datadog via MCP). Creates a phantom page in wiki/_realtime/ with kind=realtime that matches in FTS5 and fires a live handler in /vbrain-query-knowledge. Use when the user asks "connect my gcalendar", "connect my gmail", "connect my slack", "connect my github", "connect my datadog", "add my calendar", "I want to search agenda/email/slack/github/datadog too", or "vbrain-add-realtime-knowledge".
allowed-tools: Bash, Read, AskUserQuestion, mcp__claude_ai_Google_Calendar__list_calendars, mcp__claude_ai_Gmail__list_labels, mcp__claude_ai_Slack__slack_search_channels, mcp__github__search_repositories
---

# vbrain-add-realtime-knowledge

Registers a realtime source in vbrain. The model: the wiki has one **phantom
page** per source (in `wiki/_realtime/<source>.md`, kind `realtime`) with obvious
keywords in the body. It's indexed normally in FTS5; when it matches a query,
`/vbrain-query-knowledge` fires the corresponding MCP handler live instead of
returning the body.

## Inputs

- **source** (optional): which source to enable. Supported today: `gcalendar`,
  `gmail`, `slack`, `github`, `datadog`. If absent, ask the user via
  `AskUserQuestion`.

## Supported sources

| `source`    | Status            | Deterministic command            |
|---|---|---|
| `gcalendar` | supported         | `vbrain realtime gcalendar`      |
| `gmail`     | supported         | `vbrain realtime gmail`          |
| `slack`     | supported         | `vbrain realtime slack`          |
| `github`    | supported         | `vbrain realtime github`         |
| `datadog`   | config only (no live handler yet) | `vbrain realtime datadog` |
| other       | improvise         | ask the user for connection details |

## Steps

### 1. Determine the source

If the user didn't pass `source`, use `AskUserQuestion`:

> "Which realtime source do you want to connect to vbrain?"
> 1. Google Calendar (`gcalendar`)
> 2. Gmail (`gmail`)
> 3. Slack (`slack`)
> 4. GitHub (`github`)
> 5. Datadog (`datadog`)
> 6. Other (describe)

If the answer is "other" or an unsupported value, ask the user to describe the
source. Ask whether they want you to create a deterministic source (with a
vbrain binary and a test) or just a "manual" phantom page now. For "manual", you
can write `wiki/_realtime/<slug>.md` directly with `kind: realtime` and whatever
fields make sense — but warn that this is a one-shot source with no live handler,
so it only serves to document.

### 2. `gcalendar` flow

**2a. List available calendars** via MCP:

```
mcp__claude_ai_Google_Calendar__list_calendars
```

The typical response carries `id`, `summary`, `timeZone` (optional fields like
`primary`, `accessRole`, `backgroundColor` may appear depending on the MCP
version — don't depend on them). **Filters to always apply**, even without
`AskUserQuestion`:

- Exclude generic Google calendars: `id` matching
  `holiday@group.v.calendar.google.com`,
  `*#holiday@group.v.calendar.google.com`,
  `addressbook#contacts@group.v.calendar.google.com`, or `summary` equal to
  "Birthdays" / "Aniversários".
- Keep the rest as candidates.

**2b. Ask the user which to connect.** Try `AskUserQuestion` with
`multiSelect: true` listing the candidates (label = `summary` or `id`,
description = short `id` + `timeZone`).

**Fallback if `AskUserQuestion` isn't available** (agent sessions without the
deferred tool): connect **all** candidates by default and explicitly warn the
user in the final report: "I connected all visible calendars except the generic
Google ones. To refine, run `/vbrain-add-realtime-knowledge gcalendar` again in
an interactive session, or manually edit
`~/vbrain/config/realtime/gcalendar.yml` and run `vbrain reindex`."

**2c. Build JSON and run the vbrain binary:**

```bash
vbrain realtime gcalendar --json '<JSON>'
```

Where `<JSON>` is a JSON string with the `calendars` key, each item
`{"id": "...", "summary": "...", "timezone": "..."}`. Example:

```json
{"calendars":[
  {"id":"primary","summary":"Victor","timezone":"America/Sao_Paulo"},
  {"id":"work@v360.io","summary":"V360 Work","timezone":"America/Sao_Paulo"}
]}
```

The command:
1. Writes the config to `~/vbrain/config/realtime/gcalendar.yml`.
2. Writes the phantom page to `~/vbrain/wiki/_realtime/gcalendar.md`.
3. Returns JSON with `config_path`, `wiki_path`, and `calendars`.

**2d. Reindex** so the phantom page enters FTS5:

```bash
vbrain reindex
```

**2e. Commit (if there's a git repo in `~/vbrain`):**

```bash
vbrain commit --message "realtime: connect gcalendar (<N> calendars)"
```

Where `<N>` is the number of connected calendars.

### 2bis. `gmail` flow

**2bis-a. Check the MCP's auth.** Try `mcp__claude_ai_Gmail__list_labels`.
Possible returns:

- JSON with `labels: [...]` → MCP authenticated. Proceed to 2bis-b.
- A response like "ask the user to run /mcp and select 'claude.ai Gmail' to
  authenticate" → MCP **not authenticated**. Stop and instruct the user:
  > "To connect Gmail, open `/mcp` in Claude Code, select **claude.ai Gmail**,
  > and authorize in the browser. When you're back, call me again with
  > `/vbrain-add-realtime-knowledge gmail`."
  Don't try to bypass it.

**2bis-b. List labels** via `mcp__claude_ai_Gmail__list_labels`. The response
carries only **user labels** (`labelId` + `name`). System labels do NOT appear
but exist with well-known IDs: `INBOX`, `IMPORTANT`, `STARRED`, `UNREAD`, `SENT`,
`DRAFT`, `SPAM`, `TRASH`, `CHAT`.

**2bis-c. Ask which labels to connect.** Try `AskUserQuestion` with
`multiSelect: true` listing: the 3 relevant system labels (`INBOX`, `IMPORTANT`,
`STARRED`) + all returned user labels. Label = `name` (or ID for system),
description = "system" or "user".

**Fallback if `AskUserQuestion` isn't available**: connect only `INBOX` +
`IMPORTANT` by default and warn:
> "I connected INBOX + IMPORTANT. To refine, edit
> `~/vbrain/config/realtime/gmail.yml` (add `{id, name}` objects to the `labels`
> key) and run `vbrain reindex`, or run `/vbrain-add-realtime-knowledge gmail`
> in an interactive session."

**2bis-d. Build JSON and run the vbrain binary:**

```bash
vbrain realtime gmail --json '<JSON>'
```

Where `<JSON>` is a JSON string with the `labels` key, each item
`{"id": "...", "name": "..."}`. Example:

```json
{"labels":[
  {"id":"INBOX","name":"Inbox"},
  {"id":"IMPORTANT","name":"Important"},
  {"id":"Label_5","name":"JCA"}
]}
```

The command writes `~/vbrain/config/realtime/gmail.yml` + the phantom page at
`~/vbrain/wiki/_realtime/gmail.md`.

**2bis-e. Reindex and commit** (same commands as the gcalendar section, change
the commit message to `"realtime: connect gmail (<N> labels)"`).

### 2ter. `slack` flow

Slack differs from the other sources in two ways that dictate the flow:

1. **You can't "list everything"**: `slack_search_channels` requires a search
   term — there's no full channel enumeration like calendars/labels.
2. **Slack search has no `OR`** (space = AND). So the multi-channel filter
   doesn't become a single query; the query-knowledge handler does **one search
   per channel** and merges (see `/vbrain-query-knowledge`, section 3c).

Because of this, the config accepts an **empty channel list = global search**
across the whole workspace (all accessible channels/DMs). A populated list =
a search filtered by those channels.

**2ter-a. Check the MCP's auth.** Try `mcp__claude_ai_Slack__slack_search_channels`
with any term (e.g. `query: "general"`, `limit: 1`). If the return is like "ask
the user to run /mcp and select 'claude.ai Slack' to authenticate", the MCP is
**not authenticated** — stop and instruct:
> "To connect Slack, open `/mcp` in Claude Code, select **claude.ai Slack**, and
> authorize in the browser. When you're back, call me again with
> `/vbrain-add-realtime-knowledge slack`."
Don't try to bypass it.

**2ter-b. Ask for the scope** via `AskUserQuestion` (single select):
> "Which Slack channels to connect?"
> 1. All (whole workspace) — global search, no channel filter.
> 2. Specific channels — you tell me the names.

- If **All**: `channels = []` (global mode). Skip to 2ter-d.
- If **Specific channels**: ask for the channel names (free text, e.g.
  "general, product, announcements"). For each name, call
  `mcp__claude_ai_Slack__slack_search_channels` with `query: "<name>"`,
  `channel_types: "public_channel,private_channel"`. Take the best match
  (`id` + `name`). If a name doesn't resolve, warn the user and proceed with the
  ones that did. Build the `channels` list with `{id, name}`.

**Fallback if `AskUserQuestion` isn't available**: connect in **global** mode
(`channels = []`) and warn:
> "I connected Slack in global mode (search the whole workspace). To restrict to
> channels, run `/vbrain-add-realtime-knowledge slack` in an interactive session
> or edit `~/vbrain/config/realtime/slack.yml` (add `{id, name}` objects to the
> `channels` key) and run `vbrain reindex`."

**2ter-c. Build JSON and run the vbrain binary:**

```bash
vbrain realtime slack --json '<JSON>'
```

Where `<JSON>` is a JSON string with the `channels` key, each item
`{"id": "...", "name": "..."}`. For global mode, pass `{"channels":[]}`.
Examples:

```json
{"channels":[]}
```
```json
{"channels":[
  {"id":"C0GERAL","name":"general"},
  {"id":"C0PROD","name":"product"}
]}
```

The command writes `~/vbrain/config/realtime/slack.yml` + the phantom page at
`~/vbrain/wiki/_realtime/slack.md` and returns JSON with `mode`
(`global`|`filtered`), `config_path`, `wiki_path`, and `channels`.

**2ter-d. Reindex and commit** (same commands as the gcalendar section, change
the commit message to `"realtime: connect slack (<mode>)"`, where `<mode>` is
"global" or "<N> channels").

### 2quater. `github` flow

Like Slack, GitHub accepts an **empty repo list = global search** across every
repo the user can access; a populated list filters by those repos (GitHub search
OR-combines repeated `repo:owner/name` qualifiers, so the query-knowledge handler
makes a single call — see `/vbrain-query-knowledge`, section 3d).

**2quater-a. Check the MCP's auth.** Try `mcp__github__search_repositories` with
any term (e.g. `query: "user:<the user>"`, minimal page). If it errors with an
auth/not-connected message, stop and instruct:
> "To connect GitHub, open `/mcp` in Claude Code, select the **GitHub** server,
> and authorize. When you're back, call me again with
> `/vbrain-add-realtime-knowledge github`."
Don't try to bypass it.

**2quater-b. Ask for the scope** via `AskUserQuestion` (single select):
> "Which GitHub repositories to connect?"
> 1. All (every accessible repo) — global search, no `repo:` filter.
> 2. Specific repos — you tell me `owner/name`.

- If **All**: `repos = []` (global mode). Skip to 2quater-d.
- If **Specific repos**: ask for them (free text, e.g.
  "virtual360-io/vbrain, acme/api"). For each, you may confirm it exists with
  `mcp__github__search_repositories` (`query: "repo:<owner>/<name>"`); if it
  doesn't resolve, warn the user and proceed with the ones that did. Build the
  `repos` list with `{owner, name}`.

**Fallback if `AskUserQuestion` isn't available**: connect in **global** mode
(`repos = []`) and warn:
> "I connected GitHub in global mode (every accessible repo). To restrict to
> repos, run `/vbrain-add-realtime-knowledge github` in an interactive session or
> edit `~/vbrain/config/realtime/github.yml` (add `{owner, name}` objects to the
> `repos` key) and run `vbrain reindex`."

**2quater-c. Build JSON and run the vbrain binary:**

```bash
vbrain realtime github --json '<JSON>'
```

Where `<JSON>` has the `repos` key, each item `{"owner": "...", "name": "..."}`.
For global mode, pass `{"repos":[]}`. Examples:

```json
{"repos":[]}
```
```json
{"repos":[
  {"owner":"virtual360-io","name":"vbrain"},
  {"owner":"acme","name":"api"}
]}
```

The command writes `~/vbrain/config/realtime/github.yml` + the phantom page at
`~/vbrain/wiki/_realtime/github.md` and returns JSON with `mode`
(`global`|`filtered`), `config_path`, `wiki_path`, and `repos`.

**2quater-d. Reindex and commit** (same commands as gcalendar; commit message
`"realtime: connect github (<mode>)"`, where `<mode>` is "global" or
"<N> repos").

### 2quinquies. `datadog` flow

⚠️ Datadog is **config-only today**: there's no Datadog MCP wired, so the live
handler in `/vbrain-query-knowledge` is documented but pending. This flow still
creates the config + phantom page (so the source is discoverable in FTS5 and
ready the moment a handler exists), but you **must warn** the user that queries
won't resolve live yet. Do **not** try to fetch Datadog data here.

**2quinquies-a. Ask which scopes to watch** via `AskUserQuestion`
(`multiSelect: true`):
> "What should Datadog bring? (you can pick more than one)"
> 1. Monitors & alerts (`monitor`)
> 2. Incidents (`incident`)
> 3. Dashboards & metrics (`dashboard`)

Optionally ask for a tag filter per scope (free text, e.g. `service:vbrain`,
`env:prod`) — leave empty for "all". If the user picks nothing, pass an empty
list (`{"scopes":[]}` = all supported kinds, no tag filter).

**Fallback if `AskUserQuestion` isn't available**: connect with all kinds and no
tag filter (`{"scopes":[]}`) and warn the user they can refine later by editing
`~/vbrain/config/realtime/datadog.yml` and running `vbrain reindex`.

**2quinquies-b. Build JSON and run the vbrain binary:**

```bash
vbrain realtime datadog --json '<JSON>'
```

Where `<JSON>` has the `scopes` key, each item `{"kind": "...", "tag": "..."}`.
`kind` ∈ {`monitor`, `incident`, `dashboard`} (synonyms like `alerts`/`metrics`
are normalized; unknown kinds are dropped). Example:

```json
{"scopes":[
  {"kind":"monitor","tag":"service:vbrain"},
  {"kind":"incident","tag":""}
]}
```

The command writes `~/vbrain/config/realtime/datadog.yml` + the phantom page at
`~/vbrain/wiki/_realtime/datadog.md` and returns JSON with `config_path`,
`wiki_path`, and `scopes`.

**2quinquies-c. Reindex and commit** (same commands as gcalendar; commit message
`"realtime: connect datadog (<N> scopes)"`).

**2quinquies-d. Warn** in the final report that Datadog has no live handler yet:
> "Datadog is connected as a knowledge source, but I can't pull it live until a
> Datadog MCP is wired — a query that matches it will tell you to connect the
> MCP. The config and page are ready for that day."

### 3. Report

Show the user:
- The list of connected calendars/labels/channels/repos/scopes (or "global mode"
  for slack/github)
- The config path (`~/vbrain/config/realtime/<source>.yml`)
- The phantom page path (`wiki/_realtime/<source>.md`)
- Next step: "now ask something like 'do I have a meeting tomorrow?' (gcalendar),
  'any email from client X this week?' (gmail), 'what did people say on Slack
  about the deploy?' (slack), or 'any open PRs on vbrain?' (github) in
  `/vbrain-query-knowledge` — I'll pull it live."
- For **datadog**, instead say it's connected as a source but can't resolve live
  yet (no MCP wired) — see 2quinquies-d.

## Rules

- **Never** fetch data (events, emails) during this skill — it's configuration
  only. Listing calendars/labels is OK; fetching events/threads is
  `/vbrain-query-knowledge`'s job.
- **Never** write into `wiki/_realtime/` by hand for supported sources
  (gcalendar/gmail/slack/github/datadog): always go through the vbrain binary.
  For "other" sources with no command, writing directly is OK but warn the user
  that without a handler it won't resolve live.
- **Datadog** is config-only today (no MCP handler): create the config + page,
  but always warn the user that queries won't resolve live until a Datadog MCP is
  wired. Don't pretend it pulls data.
- If the source's MCP fails (not connected, no permission), stop and guide the
  user to open `/mcp` in Claude Code to authorize before re-running. Don't try
  to bypass the authentication.
