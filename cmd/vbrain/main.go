// Command vbrain is vbrain's deterministic CLI (reindex, query, …). Ported from
// the Ruby scripts: JSON on stdout (read by the skills), human text on stderr.
package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	iofs "io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	root "github.com/virtual360-io/vbrain"
	"github.com/virtual360-io/vbrain/internal/db"
	"github.com/virtual360-io/vbrain/internal/git"
	"github.com/virtual360-io/vbrain/internal/index"
	"github.com/virtual360-io/vbrain/internal/ingest"
	"github.com/virtual360-io/vbrain/internal/maint"
	"github.com/virtual360-io/vbrain/internal/paths"
	"github.com/virtual360-io/vbrain/internal/realtime"
	"github.com/virtual360-io/vbrain/internal/resolvelinks"
	"github.com/virtual360-io/vbrain/internal/routines"
	"github.com/virtual360-io/vbrain/internal/scaffold"
	"github.com/virtual360-io/vbrain/internal/search"
	"github.com/virtual360-io/vbrain/internal/selfupdate"
	"github.com/virtual360-io/vbrain/internal/soulwrite"
	"github.com/virtual360-io/vbrain/internal/writepages"
)

// version is the binary's release version, injected at build time via
// -ldflags "-X main.version=...". It's "dev" for local builds.
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: vbrain <reindex|query|ingest|write-pages|soul-write|resolve-links|commit|routines|realtime|install|update|version> [args]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "version", "--version", "-v":
		fmt.Println(version)
		return
	case "reindex":
		err = cmdReindex(os.Args[2:])
	case "query":
		err = cmdQuery(os.Args[2:])
	case "commit":
		err = cmdCommit(os.Args[2:])
	case "write-pages":
		err = cmdWritePages(os.Args[2:])
	case "soul-write":
		err = cmdSoulWrite(os.Args[2:])
	case "resolve-links":
		err = cmdResolveLinks(os.Args[2:])
	case "ingest":
		err = cmdIngest(os.Args[2:])
	case "routines":
		err = cmdRoutines(os.Args[2:])
	case "realtime":
		err = cmdRealtime(os.Args[2:])
	case "tags":
		err = cmdTags(os.Args[2:])
	case "stats":
		err = cmdStats(os.Args[2:])
	case "query-log":
		err = cmdQueryLog(os.Args[2:])
	case "linkify":
		err = cmdLinkify(os.Args[2:])
	case "routine-add":
		err = cmdRoutineAdd(os.Args[2:])
	case "routine-list":
		err = cmdRoutineList(os.Args[2:])
	case "seed-routines":
		err = cmdSeedRoutines(os.Args[2:])
	case "install":
		err = cmdInstall(os.Args[2:])
	case "setup":
		err = cmdSetup(os.Args[2:])
	case "update":
		err = cmdUpdate(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %q\n", os.Args[1])
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func cmdReindex(args []string) error {
	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	d, err := db.Open(paths.DBPath())
	if err != nil {
		return err
	}
	defer d.Close()

	st, err := index.Reindex(d, paths.WikiDir())
	if err != nil {
		return err
	}
	return emitJSON(st)
}

func cmdQuery(args []string) error {
	fs := flag.NewFlagSet("query", flag.ContinueOnError)
	limit := fs.Int("limit", 10, "max number of pages")
	format := fs.String("format", "markdown", "markdown|json")
	prefix := fs.Bool("prefix", false, "prefix matching")
	sourceQuery := fs.String("source-query", "", "original NL question")
	noLog := fs.Bool("no-log", false, "don't record in query_log")
	soulBoost := fs.Float64("soul-boost", 0, "multiplier favoring soul hits (<=0 → default)")
	soulAuthoritative := fs.Bool("soul-authoritative", false, "pin soul hits first (decision/belief questions)")

	// Go's flag package doesn't permute (it stops at the first positional); the
	// skill passes flags after the query (`query "x" --format json`). Split them
	// manually.
	flagArgs, positionals := splitArgs(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}

	query := strings.TrimSpace(strings.Join(positionals, " "))
	if query == "" {
		fs.Usage()
		return fmt.Errorf("empty query")
	}

	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	d, err := db.Open(paths.DBPath())
	if err != nil {
		return err
	}
	defer d.Close()

	res, err := search.Query(d, query, search.Opts{
		Limit:             *limit,
		Prefix:            *prefix,
		SourceQuery:       *sourceQuery,
		Log:               !*noLog,
		SoulBoost:         *soulBoost,
		SoulAuthoritative: *soulAuthoritative,
	})
	if err != nil {
		return err
	}

	if *format == "json" {
		return emitJSON(res)
	}
	printMarkdown(res)
	return nil
}

// boolFlags are the query flags that don't consume a value.
var boolFlags = map[string]bool{
	"-prefix": true, "--prefix": true,
	"-no-log": true, "--no-log": true,
	"-soul-authoritative": true, "--soul-authoritative": true,
}

// splitArgs separates flags (and their values) from positional arguments,
// allowing flags in any position — mirrors Ruby's OptionParser behavior.
func splitArgs(args []string) (flagArgs, positionals []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") && a != "-" {
			flagArgs = append(flagArgs, a)
			// `--flag value`: consume the next token as its value (except bool
			// flags and the `--flag=value` form).
			if !strings.Contains(a, "=") && !boolFlags[a] && i+1 < len(args) {
				i++
				flagArgs = append(flagArgs, args[i])
			}
			continue
		}
		positionals = append(positionals, a)
	}
	return flagArgs, positionals
}

func cmdCommit(args []string) error {
	fs := flag.NewFlagSet("commit", flag.ContinueOnError)
	message := fs.String("message", "", "commit message")
	noPush := fs.Bool("no-push", false, "don't push")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *message == "" {
		return fmt.Errorf("--message is required")
	}

	dataHome := paths.DataHome()
	if !git.RepoInitialized(dataHome) {
		return emitJSON(map[string]any{
			"committed": false, "pushed": false,
			"reason": "no git repo in " + dataHome,
		})
	}

	commit, err := git.Commit(*message, dataHome)
	if err != nil {
		return err
	}
	out := map[string]any{"committed": commit.Committed}
	if commit.SHA != "" {
		out["sha"] = commit.SHA
	}
	if commit.Message != "" {
		out["message"] = commit.Message
	}
	if commit.Reason != "" {
		out["reason"] = commit.Reason
	}

	if *noPush || !commit.Committed {
		out["pushed"] = false
		if *noPush {
			out["reason"] = "no-push"
		} else {
			out["reason"] = commit.Reason
		}
	} else {
		push, err := git.Push(dataHome, "origin", "")
		if err != nil {
			return err
		}
		out["pushed"] = push.Pushed
		if push.Reason != "" {
			out["reason"] = push.Reason
		}
		if push.Remote != "" {
			out["remote"] = push.Remote
		}
		if push.Branch != "" {
			out["branch"] = push.Branch
		}
	}
	return emitJSON(out)
}

func cmdWritePages(args []string) error {
	fs := flag.NewFlagSet("write-pages", flag.ContinueOnError)
	rawID := fs.Int("raw-id", 0, "raw_source id")
	pagesJSON := fs.String("pages-json", "", "path to the pages JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *rawID == 0 || *pagesJSON == "" {
		return fmt.Errorf("--raw-id and --pages-json are required")
	}

	data, err := os.ReadFile(*pagesJSON)
	if err != nil {
		return err
	}
	var pages []writepages.PageInput
	if t := bytes.TrimSpace(data); len(t) > 0 && t[0] == '[' {
		if err := json.Unmarshal(data, &pages); err != nil {
			return err
		}
	} else {
		var wrapper struct {
			Pages []writepages.PageInput `json:"pages"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return err
		}
		pages = wrapper.Pages
	}

	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	d, err := db.Open(paths.DBPath())
	if err != nil {
		return err
	}
	defer d.Close()

	res, err := writepages.WritePages(d, *rawID, pages, paths.WikiDir(), paths.TmpDir(), paths.DataHome())
	if err != nil {
		return err
	}
	if err := emitJSON(res); err != nil {
		return err
	}
	if res.NeedsReview {
		os.Exit(3) // orphan guardrail: an agent needs to review
	}
	return nil
}

// cmdSoulWrite is the only writer into the soul layer (wiki/_soul/). It is kept
// separate from write-pages on purpose: soul pages come from the daily soul
// routine's consolidation, never from the add-knowledge pipeline.
func cmdSoulWrite(args []string) error {
	fs := flag.NewFlagSet("soul-write", flag.ContinueOnError)
	pagesJSON := fs.String("pages-json", "", "path to the soul pages JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pagesJSON == "" {
		return fmt.Errorf("--pages-json is required")
	}

	data, err := os.ReadFile(*pagesJSON)
	if err != nil {
		return err
	}
	var pages []soulwrite.PageInput
	if t := bytes.TrimSpace(data); len(t) > 0 && t[0] == '[' {
		if err := json.Unmarshal(data, &pages); err != nil {
			return err
		}
	} else {
		var wrapper struct {
			Pages []soulwrite.PageInput `json:"pages"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return err
		}
		pages = wrapper.Pages
	}

	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	res, err := soulwrite.SoulWrite(pages, paths.WikiDir())
	if err != nil {
		return err
	}
	return emitJSON(res)
}

func cmdResolveLinks(args []string) error {
	fs := flag.NewFlagSet("resolve-links", flag.ContinueOnError)
	mapPath := fs.String("map", "", "path to the JSON {title: slug}")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *mapPath == "" {
		return fmt.Errorf("--map is required")
	}

	data, err := os.ReadFile(*mapPath)
	if err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("map must be a JSON object {title: slug}: %w", err)
	}
	mapping := map[string]string{}
	for k, v := range raw {
		if s, ok := v.(string); ok {
			mapping[k] = s
		} else {
			mapping[k] = "" // null/other → discarded in ResolveLinks
		}
	}

	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	res, err := resolvelinks.ResolveLinks(paths.WikiDir(), mapping)
	if err != nil {
		return err
	}
	return emitJSON(res)
}

func cmdIngest(args []string) error {
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	typeOverride := fs.String("type", "", "force the source type (text|url|tweet)")
	force := fs.Bool("force", false, "ingest even if sha256 is duplicate")

	flagArgs, positionals := splitArgs(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if len(positionals) == 0 {
		return fmt.Errorf("usage: vbrain ingest <path-or-url> [--type T] [--force]")
	}
	input := positionals[0]

	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	d, err := db.Open(paths.DBPath())
	if err != nil {
		return err
	}
	defer d.Close()

	res, err := ingest.IngestRaw(d, input, *typeOverride, *force, paths.RawDir(), paths.TmpDir())
	if err != nil {
		return err
	}
	return emitJSON(res)
}

func cmdRoutines(args []string) error {
	fs := flag.NewFlagSet("routines", flag.ContinueOnError)
	nowStr := fs.String("now", "", "ISO8601 (default: now)")
	dryRun := fs.Bool("dry-run", false, "don't claim; just list the due ones")
	if err := fs.Parse(args); err != nil {
		return err
	}

	now := time.Now().UTC()
	if *nowStr != "" {
		t, err := time.Parse(time.RFC3339, *nowStr)
		if err != nil {
			return fmt.Errorf("invalid --now: %w", err)
		}
		now = t.UTC()
	}

	due := []map[string]any{}
	if *dryRun {
		rs, err := routines.DueDryRun(now)
		if err != nil {
			return err
		}
		for _, r := range rs {
			due = append(due, dueEntry(r, nil))
		}
	} else {
		cs, err := routines.ClaimDue(now)
		if err != nil {
			return err
		}
		for _, c := range cs {
			ca := c.ClaimedAt
			due = append(due, dueEntry(c.Routine, &ca))
		}
	}

	return emitJSON(map[string]any{
		"now":         now.Format(time.RFC3339),
		"config_path": routines.ConfigPath(),
		"due_count":   len(due),
		"due":         due,
	})
}

func dueEntry(r routines.Routine, claimedAt *string) map[string]any {
	return map[string]any{
		"slug":        r.Slug,
		"description": r.Description,
		"schedule":    r.Schedule,
		"prompt":      r.Prompt,
		"last_run":    r.LastRun,
		"claimed_at":  claimedAt,
	}
}

// cmdRealtime connects a realtime source: writes the config and the phantom page.
// usage: vbrain realtime <gcalendar|gmail|slack> --json '<json>' | --file <path>
func cmdRealtime(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: vbrain realtime <gcalendar|gmail|slack> --json '<json>'")
	}
	source := args[0]
	fs := flag.NewFlagSet("realtime", flag.ContinueOnError)
	jsonStr := fs.String("json", "", "items as JSON (array or {key:[...]})")
	file := fs.String("file", "", "file with the items JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	raw := []byte(*jsonStr)
	if *file != "" {
		b, err := os.ReadFile(*file)
		if err != nil {
			return err
		}
		raw = b
	}

	keys := map[string]string{"gcalendar": "calendars", "gmail": "labels", "slack": "channels"}
	key, ok := keys[source]
	if !ok {
		return fmt.Errorf("unknown realtime source: %q", source)
	}
	items, err := parseRealtimeItems(raw, key)
	if err != nil {
		return err
	}

	if err := paths.EnsureDirs(); err != nil {
		return err
	}

	out := map[string]any{"source": source}
	var saved []realtime.Item
	var wikiAbs string
	switch source {
	case "gcalendar":
		if saved, err = (realtime.Gcalendar{}).SaveConfig(items); err != nil {
			return err
		}
		wikiAbs, err = realtime.Gcalendar{}.WriteWikiPage(saved)
		out["config_path"] = realtime.Gcalendar{}.ConfigPath()
		out["calendars"] = saved
	case "gmail":
		if saved, err = (realtime.Gmail{}).SaveConfig(items); err != nil {
			return err
		}
		wikiAbs, err = realtime.Gmail{}.WriteWikiPage(saved)
		out["config_path"] = realtime.Gmail{}.ConfigPath()
		out["labels"] = saved
	case "slack":
		if saved, err = (realtime.Slack{}).SaveConfig(items); err != nil {
			return err
		}
		wikiAbs, err = realtime.Slack{}.WriteWikiPage(saved)
		out["config_path"] = realtime.Slack{}.ConfigPath()
		out["channels"] = saved
		if (realtime.Slack{}).Global(saved) {
			out["mode"] = "global"
		} else {
			out["mode"] = "filtered"
		}
	}
	if err != nil {
		return err
	}
	out["wiki_path_abs"] = wikiAbs
	out["wiki_path"] = strings.TrimPrefix(wikiAbs, paths.WikiDir()+string(os.PathSeparator))
	return emitJSON(out)
}

// parseRealtimeItems accepts a JSON array of objects or {key:[...]}.
func parseRealtimeItems(raw []byte, key string) ([]map[string]string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty items; pass --json or --file")
	}
	var arr []map[string]any
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &arr); err != nil {
			return nil, err
		}
	} else {
		var obj map[string][]map[string]any
		if err := json.Unmarshal(trimmed, &obj); err != nil {
			return nil, err
		}
		arr = obj[key]
	}
	out := make([]map[string]string, 0, len(arr))
	for _, m := range arr {
		sm := map[string]string{}
		for k, v := range m {
			if s, ok := v.(string); ok {
				sm[k] = s
			}
		}
		out = append(out, sm)
	}
	return out, nil
}

func withDB(fn func(*sql.DB) error) error {
	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	d, err := db.Open(paths.DBPath())
	if err != nil {
		return err
	}
	defer d.Close()
	return fn(d)
}

func cmdTags(args []string) error {
	fs := flag.NewFlagSet("tags", flag.ContinueOnError)
	limit := fs.Int("limit", 0, "max tags (0 = all)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return withDB(func(d *sql.DB) error {
		res, err := maint.Tags(d, *limit)
		if err != nil {
			return err
		}
		return emitJSON(res)
	})
}

func cmdStats(args []string) error {
	return withDB(func(d *sql.DB) error {
		res, err := maint.Stats(d, paths.DataHome())
		if err != nil {
			return err
		}
		return emitJSON(res)
	})
}

func cmdQueryLog(args []string) error {
	fs := flag.NewFlagSet("query-log", flag.ContinueOnError)
	dump := fs.Bool("dump", false, "list the entries")
	prune := fs.Bool("prune", false, "delete entries with id <= through-id")
	limit := fs.Int("limit", 0, "limit on dump")
	throughID := fs.Int64("through-id", 0, "prune up to this id (inclusive)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return withDB(func(d *sql.DB) error {
		switch {
		case *dump:
			res, err := maint.QueryLogDump(d, *limit)
			if err != nil {
				return err
			}
			return emitJSON(res)
		case *prune:
			if *throughID == 0 {
				return fmt.Errorf("--prune requires --through-id K")
			}
			res, err := maint.QueryLogPrune(d, *throughID)
			if err != nil {
				return err
			}
			return emitJSON(res)
		default:
			return fmt.Errorf("usage: vbrain query-log (--dump [--limit N] | --prune --through-id K)")
		}
	})
}

func cmdLinkify(args []string) error {
	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	res, err := maint.Linkify(paths.WikiDir())
	if err != nil {
		return err
	}
	return emitJSON(res)
}

func cmdRoutineAdd(args []string) error {
	fs := flag.NewFlagSet("routine-add", flag.ContinueOnError)
	slug := fs.String("slug", "", "routine slug")
	description := fs.String("description", "", "description")
	schedule := fs.String("schedule", "", "5-field cron (optional)")
	prompt := fs.String("prompt", "", "routine prompt")
	promptFile := fs.String("prompt-file", "", "file with the prompt")
	disabled := fs.Bool("disabled", false, "create disabled")
	replace := fs.Bool("replace", false, "replace if it already exists")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*slug) == "" {
		return fmt.Errorf("--slug is required")
	}
	p := *prompt
	if p == "" && *promptFile != "" {
		b, err := os.ReadFile(*promptFile)
		if err != nil {
			return err
		}
		p = string(b)
	}
	if strings.TrimSpace(p) == "" {
		return fmt.Errorf("--prompt or --prompt-file is required")
	}
	var sched *string
	if *schedule != "" {
		sched = schedule
	}
	entry, err := routines.Add(*slug, *description, p, sched, !*disabled, *replace, time.Now())
	if err != nil {
		return err
	}
	all, _ := routines.LoadAll()
	return emitJSON(map[string]any{
		"config_path": routines.ConfigPath(), "routine": entry, "total": len(all),
	})
}

func cmdRoutineList(args []string) error {
	fs := flag.NewFlagSet("routine-list", flag.ContinueOnError)
	slug := fs.String("slug", "", "filter by slug")
	enabledOnly := fs.Bool("enabled-only", false, "only enabled")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var list []routines.Routine
	var err error
	switch {
	case *slug != "":
		r, e := routines.Find(*slug)
		err = e
		if r != nil {
			list = []routines.Routine{*r}
		}
	case *enabledOnly:
		list, err = routines.Enabled()
	default:
		list, err = routines.LoadAll()
	}
	if err != nil {
		return err
	}
	return emitJSON(map[string]any{
		"config_path": routines.ConfigPath(), "count": len(list), "routines": list,
	})
}

func cmdSeedRoutines(args []string) error {
	fs := flag.NewFlagSet("seed-routines", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "don't write, just report")
	if err := fs.Parse(args); err != nil {
		return err
	}
	res, err := routines.SeedDefaults(*dryRun, time.Now())
	if err != nil {
		return err
	}
	return emitJSON(map[string]any{
		"config_path": routines.ConfigPath(),
		"seeded":      res.Seeded, "skipped": res.Skipped, "dry_run": *dryRun,
	})
}

// cmdInstall is the entry point after downloading the binary from the release:
// installs the binary itself onto the PATH, installs the embedded skills
// globally, and bootstraps the base (= setup), with interactive GitHub
// onboarding when on a terminal. Replaces the old install.sh.
func cmdInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	binDir := fs.String("bin-dir", defaultBinDir(), "PATH directory for the binary")
	github := fs.String("github", "none", "private|public|none")
	repoName := fs.String("repo-name", "vbrain", "GitHub repo name")
	token := fs.String("token", os.Getenv("GITHUB_TOKEN"), "GitHub PAT (or env GITHUB_TOKEN)")
	noPrompt := fs.Bool("no-prompt", false, "don't ask anything (automation)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	out := map[string]any{}

	// 1. binary onto PATH
	binPath, onPath, err := installSelf(*binDir)
	if err != nil {
		return err
	}
	out["binary"] = binPath
	if !onPath {
		fmt.Fprintf(os.Stderr, "→ add to PATH: export PATH=\"%s:$PATH\"\n", filepath.Dir(binPath))
	}

	// 2. global skills (~/.claude/skills) from the embed
	skills, err := embeddedSkills()
	if err != nil {
		return err
	}
	if home, err := os.UserHomeDir(); err == nil {
		n, err := scaffold.InstallSkills(home, skills)
		if err != nil {
			return err
		}
		out["global_skills_installed"] = n
	}

	// An already-initialized base means this is an update, not a first install:
	// refresh the binary + skills and push to the existing remote, but skip the
	// GitHub onboarding (visibility/PAT) — the base is already wired.
	existing := git.RepoInitialized(paths.DataHome())
	if existing {
		out["mode"] = "update"
		fmt.Fprintf(os.Stderr, "→ existing base at %s — refreshing assets (skipping onboarding)\n", paths.DataHome())
	}

	// 3. interactive onboarding (git identity + GitHub) — only for a fresh base
	if !*noPrompt && !existing {
		onboard(github, repoName, token)
	}

	// 4. bootstrap the base
	if err := bootstrapBase(out, *github, *repoName, *token); err != nil {
		return err
	}
	return emitJSON(out)
}

// cmdSetup bootstraps only the base (dirs, CLAUDE.md, skills, git init, routine
// seeding, optional GitHub). Reusable; `vbrain install` wraps it.
func cmdSetup(args []string) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	github := fs.String("github", "none", "private|public|none")
	repoName := fs.String("repo-name", "vbrain", "GitHub repo name")
	token := fs.String("token", os.Getenv("GITHUB_TOKEN"), "GitHub PAT (or env GITHUB_TOKEN)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	out := map[string]any{}
	if err := bootstrapBase(out, *github, *repoName, *token); err != nil {
		return err
	}
	return emitJSON(out)
}

// cmdUpdate self-updates the binary from the rolling latest release.
func cmdUpdate(args []string) error {
	res, err := selfupdate.Run()
	if err != nil {
		return err
	}
	if res.Method == "homebrew" {
		fmt.Fprintln(os.Stderr, "updated via Homebrew (brew update && brew upgrade vbrain)")
	} else {
		fmt.Fprintf(os.Stderr, "updated: %s → %s\n", res.Asset, res.Path)
	}
	return emitJSON(res)
}

// embeddedSkills returns the FS of the embedded skills, rooted at .claude/skills.
func embeddedSkills() (iofs.FS, error) {
	return iofs.Sub(root.SkillsFS, ".claude/skills")
}

// bootstrapBase does dirs + CLAUDE.md + skills in the base + git init + seed +
// commit + (optional) GitHub repo creation/push. Fills out.
func bootstrapBase(out map[string]any, github, repoName, token string) error {
	dataHome := paths.DataHome()
	out["data_home"] = dataHome
	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	claudeMD, err := scaffold.WriteClaudeMD(dataHome)
	if err != nil {
		return err
	}
	out["claude_md"] = claudeMD

	if skills, err := embeddedSkills(); err == nil {
		n, err := scaffold.InstallSkills(dataHome, skills)
		if err != nil {
			return err
		}
		out["skills_installed"] = n
	}

	if !git.RepoInitialized(dataHome) {
		if err := git.Init(dataHome); err != nil {
			return err
		}
		out["initialized"] = true
	}

	seed, err := routines.SeedDefaults(false, time.Now())
	if err != nil {
		return err
	}
	out["seeded_routines"] = seed.Seeded

	if _, err := git.Commit("chore: vbrain agent assets (CLAUDE.md + skills + routines)", dataHome); err != nil {
		return err
	}

	// Create the repo only for a fresh base that asked for GitHub and has no
	// remote yet. The gh CLI / system git credentials are preferred; a PAT is
	// only the fallback when neither is available.
	if github != "none" && github != "" && !git.HasRemote(dataHome, "origin") {
		if token == "" && !ghAvailable() {
			out["needs_token"] = true
			out["github"] = github
			return nil
		}
		url, err := createGitHubRepo(repoName, github == "private", token)
		if err != nil {
			return err
		}
		if err := git.AddRemote(url, dataHome, "origin"); err != nil {
			return err
		}
		out["remote_url"] = url
	}

	// Push whenever a remote exists — the system git uses SSH / the credential
	// helper (no PAT); go-git falls back to GITHUB_TOKEN when one is set.
	if git.HasRemote(dataHome, "origin") {
		if token != "" {
			os.Setenv("GITHUB_TOKEN", token) // go-git push uses the PAT
		}
		res, err := git.Push(dataHome, "origin", "")
		if err != nil {
			// Likely non-fast-forward: the remote moved (another machine / the
			// cloud pushed). Rebase on top and retry once before giving up.
			if rebaseErr := git.PullRebase(dataHome, "origin", ""); rebaseErr == nil {
				res, err = git.Push(dataHome, "origin", "")
			}
			if err != nil {
				return err
			}
		}
		out["pushed"] = res.Pushed
	}
	return nil
}

func defaultBinDir() string {
	if d := os.Getenv("VBRAIN_BIN_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin")
}

// installSelf copies the running binary into binDir (no-op if it already runs
// from there). Returns the final path and whether binDir is already on the PATH.
func installSelf(binDir string) (string, bool, error) {
	invoked, err := os.Executable()
	if err != nil {
		return "", false, err
	}
	// Already reachable on PATH from where it runs (e.g. a Homebrew install
	// under /opt/homebrew/bin) — copying into binDir would leave a duplicate
	// that `vbrain update` then diverges from the package-managed one.
	if dir := filepath.Dir(invoked); inPath(dir) {
		return invoked, true, nil
	}
	exe := invoked
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	name := "vbrain"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	target := filepath.Join(binDir, name)
	onPath := inPath(binDir)
	if exe == target {
		return target, onPath, nil
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", onPath, err
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		return "", onPath, err
	}
	if err := os.WriteFile(target, data, 0o755); err != nil {
		return "", onPath, err
	}
	return target, onPath, nil
}

func inPath(dir string) bool {
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if p == dir {
			return true
		}
	}
	return false
}

// onboard asks (only on a terminal) for git identity and GitHub visibility/PAT.
func onboard(github, repoName, token *string) {
	if !isTerminal() {
		return
	}
	ensureGitIdentity()
	if *github == "none" {
		switch strings.ToLower(prompt("Version the base on GitHub? [p]rivate/[u]public/[n]one (p): ")) {
		case "u", "public":
			*github = "public"
		case "n", "none":
			*github = "none"
		default: // "" (default) or "p" → private
			*github = "private"
		}
	}
	// A PAT is only needed when the gh CLI isn't there to create the repo and
	// the system git can't push with the user's own credentials. With gh
	// authenticated, skip the prompt entirely.
	if *github != "none" && *token == "" && !ghAvailable() {
		fmt.Fprintln(os.Stderr, "Create a PAT (scope 'repo'): https://github.com/settings/tokens/new?scopes=repo&description=vbrain")
		*token = prompt("Paste the PAT (empty skips GitHub): ")
		if *token == "" {
			*github = "none"
		} else if *repoName == "" {
			*repoName = "vbrain"
		}
	}
}

// ensureGitIdentity fills the global user.name/email via the system git, if it's
// present and missing.
func ensureGitIdentity() {
	if _, err := exec.LookPath("git"); err != nil {
		return
	}
	for key, q := range map[string]string{"user.name": "Your name for commits: ", "user.email": "Your email for commits: "} {
		out, _ := exec.Command("git", "config", "--global", key).Output()
		if strings.TrimSpace(string(out)) != "" {
			continue
		}
		if v := prompt(q); v != "" {
			exec.Command("git", "config", "--global", key, v).Run()
		}
	}
}

var stdinReader = bufio.NewReader(os.Stdin)

func prompt(q string) string {
	fmt.Fprint(os.Stderr, q)
	line, _ := stdinReader.ReadString('\n')
	return strings.TrimSpace(line)
}

func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// ghAvailable reports whether the gh CLI is installed and authenticated, so
// repos can be created and pushed without asking for a PAT.
func ghAvailable() bool {
	if _, err := exec.LookPath("gh"); err != nil {
		return false
	}
	return exec.Command("gh", "auth", "status").Run() == nil
}

// ghRepoURL creates (or finds, if it already exists) the GitHub repo for the
// authenticated user via the gh CLI and returns its SSH clone URL. ok=false when
// gh is unavailable/unauthenticated, so callers fall back to the PAT REST path.
// A var so tests can stub it without shelling out.
var ghRepoURL = func(name string, private bool) (string, bool) {
	if !ghAvailable() {
		return "", false
	}
	sshURL := func() (string, bool) {
		out, err := exec.Command("gh", "repo", "view", name, "--json", "sshUrl", "-q", ".sshUrl").Output()
		if err != nil {
			return "", false
		}
		u := strings.TrimSpace(string(out))
		return u, u != ""
	}
	if u, ok := sshURL(); ok { // already exists — idempotent
		return u, true
	}
	vis := "--public"
	if private {
		vis = "--private"
	}
	if err := exec.Command("gh", "repo", "create", name, vis).Run(); err != nil {
		return "", false
	}
	return sshURL()
}

// createGitHubRepo creates a repo and returns its clone URL. It prefers the gh
// CLI (no PAT needed); otherwise it falls back to the GitHub REST API with the
// PAT. Idempotent-ish: treats 422 (already exists) as ok.
func createGitHubRepo(name string, private bool, token string) (string, error) {
	if url, ok := ghRepoURL(name, private); ok {
		return url, nil
	}
	body, _ := json.Marshal(map[string]any{"name": name, "private": private})
	req, err := http.NewRequest(http.MethodPost, "https://api.github.com/user/repos", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	respBody, _ := io.ReadAll(res.Body)

	var parsed struct {
		CloneURL string `json:"clone_url"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
	}
	json.Unmarshal(respBody, &parsed)

	switch res.StatusCode {
	case 201:
		return parsed.CloneURL, nil
	case 422: // probably already exists — build the URL from the login
		who, err := githubLogin(token)
		if err != nil {
			return "", err
		}
		return "https://github.com/" + who + "/" + name + ".git", nil
	default:
		return "", fmt.Errorf("github repo create HTTP %d: %s", res.StatusCode, string(respBody))
	}
}

func githubLogin(token string) (string, error) {
	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/user", nil)
	req.Header.Set("Authorization", "token "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var u struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(res.Body).Decode(&u); err != nil {
		return "", err
	}
	return u.Login, nil
}

func emitJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func printMarkdown(res search.Result) {
	if len(res.Results) == 0 {
		fmt.Printf("No results for `%s`.\n", res.Query)
		return
	}
	fmt.Printf("# Results for `%s`\n\n", res.Query)
	for i, r := range res.Results {
		fmt.Printf("## %d. %s\n\n", i+1, r.Title)
		fmt.Printf("**Path:** `wiki/%s`\n", r.Path)
		if r.Kind != "" {
			fmt.Printf("**Kind:** `%s`\n", r.Kind)
		}
		fmt.Printf("\n%s\n\n", r.Snippet)
	}
	if len(res.Related) > 0 {
		fmt.Print("## Related (graph)\n\n")
		for _, r := range res.Related {
			fmt.Printf("- **%s** — `wiki/%s`\n", r.Title, r.Path)
		}
		fmt.Println()
	}
}
