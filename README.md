# vbrain

Personal knowledge base — inspired by
[akitaonrails/ai-memory](https://github.com/akitaonrails/ai-memory), reduced to
Claude Code skills + a single deterministic Go binary (`vbrain`) + SQLite FTS5.

The premise: **markdown wiki is the source of truth; SQLite is a derived index
— disposable (you can delete it and rebuild with `vbrain reindex`), but
versioned alongside the base for convenience (clone/pull already bring the index
ready); the LLM only steps in for what needs judgment (chunking, synthesizing
pages)**. Everything else is tested Go.

> **Migrated from Ruby to Go.** The deterministic core used to be Ruby; today
> it's a single `vbrain` binary (no runtime to install): SQLite via
> `modernc.org/sqlite` (pure-Go, FTS5 embedded) and git via go-git, falling back
> to the system git when present.

## Architecture

### Code vs. data separation

| Directory                   | What it is                                                | Versioned                     |
|---|---|---|
| This repo                   | **Code** (Go), skills, tests (canonical)                   | git here                       |
| `~/vbrain/` (`VBRAIN_HOME`)  | **Your base** — `raw/`, `wiki/`, `config/`, `db/vbrain.sqlite3` | its own git, created on demand |

`vbrain install` puts the binary on the PATH (`~/.local/bin` by default),
installs the skills globally (`~/.claude/skills/`), and bootstraps the base
(`CLAUDE.md` + skills + git init + routines). Because the skills call the
`vbrain` binary (on the PATH), the base **does not copy any code** — the binary
is enough. It runs in any environment that clones the base, with no Ruby or gems.

The base is resolved in this order: (1) `VBRAIN_HOME`, if set; (2) otherwise, if
the current directory is a base (it carries `wiki/`, as in the cloud where repo
== base), use it — so skills/sub-agents find the data without inheriting
`VBRAIN_HOME` from the shell; (3) otherwise, `~/vbrain`. The wiki becomes a
separate git repo during `vbrain install`/`setup` — private, public, or
local-only depending on the user's choice.

### Base layout (`~/vbrain/`)

```
~/vbrain/
├── raw/                 # immutable originals (audit log)
│   └── .tmp/            # pipeline intermediates (extracted-N.txt, pages-N.json)
├── wiki/                # markdown with YAML frontmatter — source of truth
│   ├── <slug>.md        # knowledge pages, flat space; connected by [[wikilinks]]
│   ├── _realtime/       # kind: realtime — phantom pages that trigger MCP handlers
│   └── _soul/           # kind: soul — identity layer (how/why the user acts), written only by the soul routine
├── config/
│   ├── realtime/        # realtime source config (gcalendar.yml etc.)
│   └── routines/routines.yml
└── db/vbrain.sqlite3    # index — pages + virtual pages_fts (FTS5) + links (graph)
```

`db/vbrain.sqlite3` **is versioned** (convenience); it stays disposable —
deleting `db/` and running `vbrain reindex` rebuilds everything from `wiki/`,
including the `[[wikilink]]` graph. There is no `wiki/index.md` and no
per-type folders: the structure is the link graph + the derived SQLite.

### The `vbrain` binary

JSON on stdout (read by the skills), human-readable text on stderr. Subcommands:

| Subcommand | What it does |
|---|---|
| `vbrain ingest <path\|url>`  | detect source, copy to `raw/`, dedup by sha256, extract |
| `vbrain write-pages --raw-id N --pages-json P` | the only writer into knowledge pages (atomic staging + orphan guardrail) |
| `vbrain soul-write --pages-json P` | the only writer into the soul layer (`wiki/_soul/`); used by the soul routine, not the ingest pipeline |
| `vbrain reindex`             | rebuild `pages`/`pages_fts`/`links` from `wiki/` |
| `vbrain query "<q>"`         | FTS5 + snippet + graph neighbors |
| `vbrain resolve-links --map M` / `vbrain linkify` | resolve/convert wikilinks |
| `vbrain commit [--no-push]`  | idempotent commit + push (go-git or system git) |
| `vbrain routines [--dry-run]` / `vbrain routine-add` / `vbrain routine-list` | scheduling (cron) |
| `vbrain realtime <gcalendar\|gmail\|slack> --json …` | connect a realtime source |
| `vbrain tags` / `vbrain stats` / `vbrain query-log` | insights/maintenance |
| `vbrain install` / `vbrain setup` / `vbrain seed-routines` | base bootstrap |
| `vbrain update` | self-update from the latest release |

### Skills (Claude Code interface)

| Slash command                       | What it does |
|---|---|
| `/vbrain-add-knowledge <path\|url>` | Ingest → `raw/` → LLM chunker → LLM wiki-writer → `vbrain write-pages` → reindex → commit |
| `/vbrain-query-knowledge <query>`   | `vbrain query`; `kind: realtime` pages trigger an MCP handler instead of a snippet |
| `/vbrain-add-realtime-knowledge`    | Connect a realtime source (Google Calendar/Gmail/Slack via MCP) and create a phantom page |
| `/vbrain-add-routine`               | Add a routine (slug, description, cron, prompt) |
| `/vbrain-routine [slug\|status]`    | Watch: claim due routines via `vbrain routines`, parallel dispatch, re-arm `/loop 15m` |

### Sources (`internal/sources/`)

`sources.Registry` is probed in order by `sources.Detect`:

| Source     | Detection                                   | Extraction |
|---|---|---|
| Twitter    | URL `twitter.com\|x.com/<user>/status/<id>` | `cdn.syndication.twimg.com` (HTTP+JSON); the X Article body via headless Chrome is best-effort (degrades to preview) |
| URL        | Other http(s) URLs                          | Jina Reader (`r.jina.ai`) — clean markdown |
| Text       | `.md`, `.txt`, extensionless + UTF-8        | passthrough |

### Index schema (`internal/db`)

```sql
raw_sources(id, path UNIQUE, original_filename, source_type, sha256 UNIQUE, ingested_at)
pages(id, path UNIQUE, title, body, kind, tags, sha256, raw_id → raw_sources, created_at, updated_at)
  kind ∈ {concept, decision, gotcha, note, rule, realtime, soul}
pages_fts(title, body, tags)              -- virtual FTS5, content='pages'
  tokenize: unicode61 tokenchars '/_-'
```

Triggers `pages_ai`/`pages_ad`/`pages_au` mirror every write to `pages` into
`pages_fts`. `vbrain query` normalizes the query (escapes `:`, quotes,
parentheses) before FTS5.

### Realtime and routines

`kind: realtime` pages carry only keywords (to match in FTS5) + metadata; the
real config lives in `config/realtime/<source>.yml`. When `query-knowledge` hits
one, it triggers the MCP handler (`list_events`/`search_threads`/Slack search)
instead of the snippet.

Routines are named prompts with a cron schedule in
`config/routines/routines.yml`; `next_run` is computed deterministically
(robfig/cron). Execution is **at-most-once** (advances `next_run` before
running). Two routines are seeded at setup: `soul` (daily, 02:00) and `dream`
(nightly, 03:00).

### Soul (the identity layer)

The knowledge wiki captures **what the user knows**; the soul layer captures
**who the user is** — how and why they act. Reading a book by Friedman and one
by Marx does not mean the user believes either, and beliefs change over a
lifetime. So the soul is kept separate, in `wiki/_soul/` (`kind: soul`).

The daily `soul` routine looks at the user's recent **actions** (questions
asked, decisions made, pages written), cross-references them with what they
know, and consolidates the result into a small set of identity pages, with three
non-negotiable invariants: it stays **lean** (a core memory must recur, not be
mentioned once), it holds **no unexplained contradiction** (a belief cycle is
either resolved with explicit context, or one belief is stale and pruned, or it
was never a core memory), and beliefs are **mortal** (abandoned ones are
deleted). It runs before `dream` so it reads the query log first.

Ranking is intent-aware ("acting > knowing"): soul pages get a mild boost by
default, and for **decision/belief** questions they have absolute precedence
(`vbrain query --soul-authoritative`) — what the user stands for outranks what
they merely read. The add-knowledge skill is forbidden from writing into the
soul layer; only the routine does, via `vbrain soul-write`.

## Repo layout

```
vbrain/
├── cmd/vbrain/          # CLI (subcommands, JSON on stdout)
├── internal/            # deterministic core (paths, db, page, slug, ftsquery,
│                        #   links, sources, index, search, writepages, soulwrite,
│                        #   ingest, resolvelinks, git, routines, realtime, maint,
│                        #   scaffold, selfupdate)
├── .claude/skills/      # SKILL.md + sub-agent prompts (embedded in the binary via go:embed)
└── embed.go             # //go:embed of the skills so `vbrain install` is self-sufficient
```

Every package under `internal/` has a corresponding `go test`. Tests isolate
data in a tmpdir via `VBRAIN_HOME` / explicit dirs.

## Setup

The same two steps on every platform: **download the binary for your machine**
from the [`latest` release](https://github.com/virtual360-io/vbrain/releases/latest),
then **run `vbrain install`** — it puts the binary on the PATH, installs the
skills (embedded in the binary) into `~/.claude/skills`, bootstraps the base
(`CLAUDE.md` + skills + git init + the `soul`/`dream` routines), and runs the
GitHub onboarding (git identity + PAT + repo creation) when on a terminal.

The asset names say which machine they're for:

| Your machine | Asset to download |
|---|---|
| **macOS** — Apple Silicon (M1/M2/M3/M4) | `vbrain-macos-apple-silicon` |
| **macOS** — Intel | `vbrain-macos-intel` |
| **Linux** — Intel/AMD 64-bit | `vbrain-linux-intel` |
| **Linux** — ARM 64-bit | `vbrain-linux-arm64` |
| **Windows** — Intel/AMD 64-bit | `vbrain-windows-intel.exe` |

No Ruby, no gems, no need to clone the repo — `vbrain` is a single,
self-contained binary (skills included). `VBRAIN_HOME` can be exported to move
the base. Pass `--github private --no-prompt` to `vbrain install` to skip the
interactive onboarding.

### macOS

**Homebrew (recommended)** — no Gatekeeper prompt, and `brew upgrade` keeps it
fresh. Picks the right binary (Apple Silicon / Intel) automatically:

```bash
brew tap virtual360-io/vbrain https://github.com/virtual360-io/vbrain
brew install vbrain
vbrain install          # installs skills + bootstraps the base (~/vbrain)
```

**Manual download** — fetch the binary directly:

```bash
# Apple Silicon (M1/M2/M3/M4). For an Intel Mac, swap the asset for vbrain-macos-intel.
curl -L -o vbrain https://github.com/virtual360-io/vbrain/releases/latest/download/vbrain-macos-apple-silicon
chmod +x vbrain
./vbrain install
```

> **Why Homebrew avoids the prompt:** the binary is not signed/notarized, so
> Gatekeeper blocks it on the *first run only when it carries the
> `com.apple.quarantine` flag* — which browsers set, but `curl` and Homebrew do
> not. So the `curl` line above is already prompt-free. If you instead download
> the asset from the releases page in a browser, clear the flag once:
> `xattr -d com.apple.quarantine vbrain` (or System Settings → Privacy &
> Security → "Allow anyway").

### Linux

```bash
# Intel/AMD 64-bit. For ARM (e.g. a Raspberry Pi), swap the asset for vbrain-linux-arm64.
curl -L -o vbrain https://github.com/virtual360-io/vbrain/releases/latest/download/vbrain-linux-intel
chmod +x vbrain
./vbrain install
```

### Windows (PowerShell)

```powershell
# Intel/AMD 64-bit.
Invoke-WebRequest -Uri https://github.com/virtual360-io/vbrain/releases/latest/download/vbrain-windows-intel.exe -OutFile vbrain.exe
.\vbrain.exe install
```

### Update

```bash
vbrain update           # downloads the latest release binary (verifies SHA256)
```

`vbrain update` is safe on a Homebrew install too: it detects the binary lives
in a Cellar and delegates to `brew update && brew upgrade vbrain` (refreshing the
tap so a freshly-published formula is seen), so the keg stays in sync instead of
being replaced out-of-band.

## Tests

```bash
go test ./...
```

## Manual verification

```bash
printf "# Postgres\n\nUse REPLICA IDENTITY FULL for logical replication.\n" > /tmp/pg.md
vbrain ingest /tmp/pg.md
# (chunker/wiki-writer run via the skill; or build a pages.json and:)
vbrain reindex
vbrain query "replica identity" --format markdown
vbrain query "postgres:logical"   # ':' doesn't break FTS5
vbrain stats
```
