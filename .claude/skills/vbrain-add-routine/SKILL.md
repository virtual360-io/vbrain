---
name: vbrain-add-routine
description: Adds a routine to vbrain (~/vbrain/config/routines/routines.yml) with slug, description, cron schedule, and prompt. Computes the initial next_run deterministically (robfig/cron). Asks whether you want to test it now via the slug. Does NOT bootstrap any loop or cron — that's /vbrain-routine's job when the user invokes it. Use when the user asks "create a routine", "add a routine", "a routine that runs every morning at 6", "an hourly routine", or "vbrain-add-routine".
allowed-tools: Bash, Read, Write, AskUserQuestion, Agent
---

# vbrain-add-routine

Creates a routine in `~/vbrain/config/routines/routines.yml`. The vbrain binary
computes `next_run` deterministically (robfig/cron) from the cron + now.
Optionally asks whether you want to **test it now** via a sub-agent (manual
trigger via slug, doesn't change state).

**This skill NEVER touches `/loop`, `CronCreate`, or `/vbrain-routine` in watch
mode.** Bootstrapping the watch loop is exclusively `/vbrain-routine`'s job when
invoked by the user.

## Inputs

- **slug**, **description**, **schedule**, **prompt**: ask for them in sequence
  if missing.

## Steps

### 1. Collect inputs

Ask in order (one question per turn, free-form message):

**Slug**:
> "Routine slug, kebab-case (e.g. `morning-brief`, `email-hourly`,
> `weekly-review`)?"

**Description**:
> "Description (one line)?"

**Schedule** — accept natural language and convert it to a standard 5-field
cron (`min hour day month day-of-week`). Examples to show:

> "When should it run? I accept natural language or a cron directly.
> Examples:
> - `0 6 * * *` (every day at 06:00)
> - `0 * * * *` (hourly)
> - `0 10 * * 3` (every Wednesday at 10:00)
> - `*/15 9-18 * * 1-5` (every 15 min, 9-18h, weekdays)
> - `0 8 * * 1` (every Monday at 08:00)"

Convert natural → cron and **confirm with the user** before proceeding:
> "I'll use `0 6 * * *` (every day at 06:00). Confirm?"

**Important**: the cron is interpreted in the system's **local TZ** (not UTC).
If the machine is at -03:00 and the cron is `0 6 * * *`, it fires at 06:00
Brasília time. Mention this if relevant (e.g. user traveling).

**Prompt**:
> "Paste the prompt. You can use markdown. It usually references other skills
> (slash commands like `/vbrain-query-knowledge`), MCP tools (`mcp__*` —
> whatever your session has loaded), or high-level instructions. The sub-agent
> that runs this routine executes this text as its instruction."

### 2. Detect slug collision

```bash
vbrain routine-list --slug <slug>
```

If `count > 0`, use `AskUserQuestion`:
> "Routine `<slug>` already exists. Replace it?"
> 1. Replace (Recommended)
> 2. Cancel

Replace → add `--replace` to the next step. Cancel → stop.

### 3. Save the prompt to a temporary file

```
/tmp/vbrain-routine-prompt-<slug>.md
```

Use `Write`. Never pass the prompt directly via `--prompt` (shell escaping
breaks with markdown/quotes/newlines).

### 4. Run the command

```bash
vbrain routine-add --slug <slug> --description "<desc>" --schedule "<cron>" --prompt-file /tmp/vbrain-routine-prompt-<slug>.md [--replace]
```

Output JSON: `{"config_path", "routine": {... including initial next_run}, "total"}`.

### 5. Commit (if there's a git repo in `~/vbrain`)

```bash
vbrain commit --message "routine: add '<slug>' (<cron>)"
```

(Use `routine: replace '<slug>'` when `--replace`.)

### 6. Offer to test now (optional)

Use `AskUserQuestion`:

> "Routine created. Want to test it now? (manual trigger via slug, doesn't count
> as a tick, doesn't change next_run)"
> 1. Yes, run now (Recommended)
> 2. No, just save

If "Yes": launch **one** `Agent` (`subagent_type: "claude"`) with the
routine-execution template:

```
You are running the vbrain routine "<SLUG>": <DESCRIPTION>

Instruction:

<PROMPT>

When done, return a single self-contained markdown block with the result (no
prefixes). If you call a skill (slash command), invoke it via the `Skill` tool.
If you call an MCP tool (any `mcp__*`), invoke it directly — use whatever your
session has available, don't enumerate.
```

Show the sub-agent's output below the report (step 7).

### 7. Report

Show:
- `slug`, `description`, `schedule` (cron + human translation)
- `next_run` (in local time + UTC in parentheses)
- First lines of the `prompt`
- If the user requested a test: the sub-agent's output.
- **How to start the watch loop** (separately): "For this routine to run
  automatically at the cron times, invoke `/vbrain-routine` (no args) whenever
  you want to start the watch — it bootstraps `/loop 15m` with a CronList
  guard."

## Rules

- **NEVER** invoke `/loop`, `Skill loop`, `CronCreate`, or `/vbrain-routine`
  without args. This skill only changes the YAML + optionally fires ONE
  sub-agent for a manual test via slug. Bootstrapping the watch is exclusively
  `/vbrain-routine`'s job when the user invokes it.
- **Never** write directly into `routines.yml` — always via the command.
- **Never** invent the prompt — take it literally from the user.
- **Always** confirm the natural → cron translation with the user before saving.
- The slug is normalized by the vbrain slug rules (ASCII kebab-case). If
  normalization yields empty, the command aborts and the skill asks for another
  slug.
- If the schedule is omitted/null, the routine becomes **manual-only** (it has
  no next_run). Allow this only if the user explicitly asks.
- The cron is interpreted in the system TZ. To change the TZ, export
  `TZ=America/Sao_Paulo` (or another) before invoking the skill.
