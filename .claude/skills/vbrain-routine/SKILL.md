---
name: vbrain-routine
description: Watch loop for vbrain routines. Checks ~/vbrain/config/routines/routines.yml for routines whose next_run is due, fires a parallel sub-agent for each, and the next run is computed deterministically by cron (robfig/cron). Called without args, that's the default behavior (watch). Use when the user asks "run my routines", "vbrain-routine", "run the morning-brief routine", "keep running in the background", or references a routine by slug.
allowed-tools: Bash, Read, Agent, AskUserQuestion, Skill, CronList
---

# vbrain-routine

Routine execution loop. **Watch is the default**: with no args, this skill runs
one "tick" (claim due routines + dispatch + update `next_run`) and ensures the
global `/loop` is registered to re-arm itself every 15 minutes.

## Inputs (accepted forms)

- **(empty)** → **tick + watch**: identifies routines with `next_run <= now`,
  fires a parallel sub-agent for each, leaves `/loop 15m /vbrain-routine`
  running. Idempotent.
- **`<slug>`** → runs only that routine **now** (manual trigger), without
  changing `next_run` or `last_run`.
- **`status`** → lists all of them with slug, schedule, next run, last run,
  enabled. Fires nothing.

## Steps (default mode — watch)

### 1. Tick: claim due routines

```bash
vbrain routines
```

This command is **deterministic** and atomic:
- Reads `~/vbrain/config/routines/routines.yml`.
- Identifies routines with `enabled: true`, `schedule != null`, and
  `next_run <= now`.
- For each: sets `last_run = now`, advances `next_run` to the next cron tick
  (robfig/cron), writes the YAML back atomically.
- Returns JSON with `due: [{slug, description, prompt}, ...]`.

Semantics: **at-most-once**. If the sub-agent fails, that run is lost (we don't
retry on the next tick). For mission-critical, the user can re-trigger manually
with `/vbrain-routine <slug>`.

### 2. Dispatch the sub-agents (in parallel)

For each item in `due`, launch an `Agent` in **a single message** with multiple
tool_use blocks:

- `subagent_type: "claude"` (needs `Tools: *` to invoke other skills/MCPs).
- `description`: the routine's `slug`.
- `prompt`:

```
You are running the vbrain routine "<SLUG>": <DESCRIPTION>

Instruction:

<PROMPT>

When done, return a single self-contained markdown block with the result (no
prefixes like "here's"). If the instruction calls a skill (slash command),
invoke it via the `Skill` tool. If it calls an MCP tool (any `mcp__*`), invoke
it directly — don't enumerate the available ones; use whatever your session has
loaded. Relative dates like "today" or "this week" are relative to the execution
moment.
```

### 3. Ensure /loop is active (with an anti-recursion guard)

**CRITICAL**: `/loop`, when called, runs the prompt **immediately** in addition
to scheduling the cron. If this skill calls `/loop /vbrain-routine` without a
guard, it goes into infinite recursion (loop calls vbrain-routine which calls
loop which calls vbrain-routine…).

Always check FIRST via `CronList` whether a recurring job with the prompt
`/vbrain-routine` already exists. Pseudocode:

```
crons = CronList()
already_active = any cron c where c.recurring and c.prompt matches ^/vbrain-routine\b

if already_active:
  # skip — the existing cron will fire the next tick every 15 min
else:
  Skill(skill: "loop", args: "15m /vbrain-routine")
```

The first manual invocation of `/vbrain-routine` (or of `/vbrain-add-routine`,
which ends up invoking this) takes the `else` branch and registers the cron.
Subsequent invocations (fired by the cron itself) take the `if` branch and skip
— no recursion.

**Granularity**: since the tick happens every 15 min, that's the detection
floor. Routines with a more aggressive cron (`*/5 * * * *`) are delayed by up to
15 min — the sub-agent fires on the next tick where `next_run` is due. For
mission-critical sub-15-min routines, the user can run `/loop 5m /vbrain-routine`
manually (and cancel the 15-min one via `CronDelete`).
If it's already active, `/loop` probably refuses or replaces it — follow its
feedback and **don't stop** the flow over it (the loop may already be working).

### 4. Report

Show:

```
# Routines run (N)

> tick @ <now ISO8601 UTC>
> next automatic tick in 15m via /loop

## <slug 1> — <description 1>

<sub-agent 1 output>

---

## <slug 2> — <description 2>

<sub-agent 2 output>

---

…
```

If `due_count == 0`, just report:

```
# Tick @ <now ISO8601 UTC>: no routines due.

Upcoming:
- <slug 1>: <next_run 1>
- <slug 2>: <next_run 2>
```

(Use `vbrain routine-list` to get `next_run` if you need details.)

## Steps (`<slug>` mode — manual trigger)

1. `vbrain routine-list --slug <slug>` to retrieve the entry.
2. If `count == 0`, report "routine `<slug>` doesn't exist" + suggest
   `/vbrain-add-routine`.
3. Launch **one** `Agent` with the same template as step 2 above.
4. Report the output. **Don't call** `vbrain routines` — a manual trigger
   doesn't change `next_run`/`last_run`.

## Steps (`status` mode)

```bash
vbrain routine-list
```

Table:
```
| slug | schedule | next_run | last_run | enabled |
```

## Rules

- **Always a sub-agent**, never inline. Reasons: isolation, parallelism, fail
  isolation.
- **Watch is the default** — with no args, always re-arm `/loop` (idempotent).
- **Don't modify** `routines.yml` here — only via `vbrain routines` (which
  updates next_run/last_run). Adding/editing comes from `/vbrain-add-routine`.
- **Manual trigger** (`<slug>`) does NOT change state. It's only for
  test/debug or off-schedule execution.
- If a sub-agent fails, show a `> error: <message>` notice in that slug's
  section and continue with the others. Do NOT re-fire — the semantics are
  at-most-once by design.
- **Don't bootstrap `/loop`** in `<slug>` or `status` mode — those are one-shot
  commands.
```
