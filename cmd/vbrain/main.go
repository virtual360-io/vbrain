// Command vbrain é o CLI determinístico do vbrain (reindex, query, …). Porta
// dos scripts Ruby: JSON no stdout (lido pelas skills), texto humano no stderr.
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
	"github.com/virtual360-io/vbrain/internal/writepages"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "uso: vbrain <reindex|query|ingest|write-pages|resolve-links|commit|routines|realtime> [args]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "reindex":
		err = cmdReindex(os.Args[2:])
	case "query":
		err = cmdQuery(os.Args[2:])
	case "commit":
		err = cmdCommit(os.Args[2:])
	case "write-pages":
		err = cmdWritePages(os.Args[2:])
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
		fmt.Fprintf(os.Stderr, "subcomando desconhecido: %q\n", os.Args[1])
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
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
	limit := fs.Int("limit", 10, "número máximo de páginas")
	format := fs.String("format", "markdown", "markdown|json")
	prefix := fs.Bool("prefix", false, "prefix matching")
	sourceQuery := fs.String("source-query", "", "pergunta NL original")
	noLog := fs.Bool("no-log", false, "não registrar no query_log")

	// O flag do Go não permuta (para no 1º posicional); a skill passa flags
	// depois da query (`query "x" --format json`). Separamos manualmente.
	flagArgs, positionals := splitArgs(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}

	query := strings.TrimSpace(strings.Join(positionals, " "))
	if query == "" {
		fs.Usage()
		return fmt.Errorf("query vazia")
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
		Limit:       *limit,
		Prefix:      *prefix,
		SourceQuery: *sourceQuery,
		Log:         !*noLog,
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

// boolFlags são as flags de query que não consomem valor.
var boolFlags = map[string]bool{"-prefix": true, "--prefix": true, "-no-log": true, "--no-log": true}

// splitArgs separa flags (e seus valores) de argumentos posicionais, permitindo
// flags em qualquer posição — replica o comportamento do OptionParser do Ruby.
func splitArgs(args []string) (flagArgs, positionals []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") && a != "-" {
			flagArgs = append(flagArgs, a)
			// `--flag valor`: consome o próximo token como valor (exceto bool
			// flags e a forma `--flag=valor`).
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
	message := fs.String("message", "", "mensagem de commit")
	noPush := fs.Bool("no-push", false, "não dar push")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *message == "" {
		return fmt.Errorf("--message é obrigatório")
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
	rawID := fs.Int("raw-id", 0, "id do raw_source")
	pagesJSON := fs.String("pages-json", "", "caminho do JSON de páginas")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *rawID == 0 || *pagesJSON == "" {
		return fmt.Errorf("--raw-id e --pages-json são obrigatórios")
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
		os.Exit(3) // guardrail de órfãos: agente precisa revisar
	}
	return nil
}

func cmdResolveLinks(args []string) error {
	fs := flag.NewFlagSet("resolve-links", flag.ContinueOnError)
	mapPath := fs.String("map", "", "caminho do JSON {título: slug}")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *mapPath == "" {
		return fmt.Errorf("--map é obrigatório")
	}

	data, err := os.ReadFile(*mapPath)
	if err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("map deve ser um objeto JSON {título: slug}: %w", err)
	}
	mapping := map[string]string{}
	for k, v := range raw {
		if s, ok := v.(string); ok {
			mapping[k] = s
		} else {
			mapping[k] = "" // null/outros → descartado em ResolveLinks
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
	typeOverride := fs.String("type", "", "força o tipo de fonte (text|url|tweet)")
	force := fs.Bool("force", false, "ingere mesmo se sha256 duplicado")

	flagArgs, positionals := splitArgs(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if len(positionals) == 0 {
		return fmt.Errorf("uso: vbrain ingest <path-or-url> [--type T] [--force]")
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
	nowStr := fs.String("now", "", "ISO8601 (default: agora)")
	dryRun := fs.Bool("dry-run", false, "não reivindica; só lista as vencidas")
	if err := fs.Parse(args); err != nil {
		return err
	}

	now := time.Now().UTC()
	if *nowStr != "" {
		t, err := time.Parse(time.RFC3339, *nowStr)
		if err != nil {
			return fmt.Errorf("--now inválido: %w", err)
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

// cmdRealtime conecta uma fonte realtime: grava o config e a página fantasma.
// uso: vbrain realtime <gcalendar|gmail|slack> --json '<json>' | --file <path>
func cmdRealtime(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("uso: vbrain realtime <gcalendar|gmail|slack> --json '<json>'")
	}
	source := args[0]
	fs := flag.NewFlagSet("realtime", flag.ContinueOnError)
	jsonStr := fs.String("json", "", "itens em JSON (array ou {key:[...]})")
	file := fs.String("file", "", "arquivo com o JSON dos itens")
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
		return fmt.Errorf("fonte realtime desconhecida: %q", source)
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

// parseRealtimeItems aceita um array JSON de objetos ou {key:[...]}.
func parseRealtimeItems(raw []byte, key string) ([]map[string]string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("itens vazios; passe --json ou --file")
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
	limit := fs.Int("limit", 0, "máximo de tags (0 = todas)")
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
	dump := fs.Bool("dump", false, "lista as entradas")
	prune := fs.Bool("prune", false, "apaga entradas com id <= through-id")
	limit := fs.Int("limit", 0, "limite no dump")
	throughID := fs.Int64("through-id", 0, "prune até este id (inclusive)")
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
				return fmt.Errorf("--prune requer --through-id K")
			}
			res, err := maint.QueryLogPrune(d, *throughID)
			if err != nil {
				return err
			}
			return emitJSON(res)
		default:
			return fmt.Errorf("uso: vbrain query-log (--dump [--limit N] | --prune --through-id K)")
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
	slug := fs.String("slug", "", "slug da rotina")
	description := fs.String("description", "", "descrição")
	schedule := fs.String("schedule", "", "cron de 5 campos (opcional)")
	prompt := fs.String("prompt", "", "prompt da rotina")
	promptFile := fs.String("prompt-file", "", "arquivo com o prompt")
	disabled := fs.Bool("disabled", false, "cria desabilitada")
	replace := fs.Bool("replace", false, "substitui se já existe")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*slug) == "" {
		return fmt.Errorf("--slug é obrigatório")
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
		return fmt.Errorf("--prompt ou --prompt-file obrigatório")
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
	slug := fs.String("slug", "", "filtra por slug")
	enabledOnly := fs.Bool("enabled-only", false, "só habilitadas")
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
	dryRun := fs.Bool("dry-run", false, "não escreve, só reporta")
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

// cmdInstall é o ponto de entrada após baixar o binário da release: instala o
// próprio binário no PATH, instala as skills embutidas globalmente, e bootstrapa
// a base (= setup), com onboarding interativo do GitHub quando num terminal.
// Substitui o antigo install.sh.
func cmdInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	binDir := fs.String("bin-dir", defaultBinDir(), "diretório no PATH p/ o binário")
	github := fs.String("github", "none", "private|public|none")
	repoName := fs.String("repo-name", "vbrain", "nome do repo no GitHub")
	token := fs.String("token", os.Getenv("GITHUB_TOKEN"), "PAT do GitHub (ou env GITHUB_TOKEN)")
	noPrompt := fs.Bool("no-prompt", false, "não perguntar nada (automação)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	out := map[string]any{}

	// 1. binário no PATH
	binPath, onPath, err := installSelf(*binDir)
	if err != nil {
		return err
	}
	out["binary"] = binPath
	if !onPath {
		fmt.Fprintf(os.Stderr, "→ adicione ao PATH: export PATH=\"%s:$PATH\"\n", filepath.Dir(binPath))
	}

	// 2. skills globais (~/.claude/skills) a partir do embed
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

	// 3. onboarding interativo (git identity + GitHub) quando num terminal
	if !*noPrompt {
		onboard(github, repoName, token)
	}

	// 4. bootstrap da base
	if err := bootstrapBase(out, *github, *repoName, *token); err != nil {
		return err
	}
	return emitJSON(out)
}

// cmdSetup bootstrapa só a base (dirs, CLAUDE.md, skills, git init, seed das
// rotinas, GitHub opcional). Reutilizável; `vbrain install` o engloba.
func cmdSetup(args []string) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	github := fs.String("github", "none", "private|public|none")
	repoName := fs.String("repo-name", "vbrain", "nome do repo no GitHub")
	token := fs.String("token", os.Getenv("GITHUB_TOKEN"), "PAT do GitHub (ou env GITHUB_TOKEN)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	out := map[string]any{}
	if err := bootstrapBase(out, *github, *repoName, *token); err != nil {
		return err
	}
	return emitJSON(out)
}

// cmdUpdate auto-atualiza o binário a partir da release rolling latest.
func cmdUpdate(args []string) error {
	res, err := selfupdate.Run()
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "atualizado: %s → %s\n", res.Asset, res.Path)
	return emitJSON(res)
}

// embeddedSkills devolve o FS das skills embutidas, com raiz em .claude/skills.
func embeddedSkills() (iofs.FS, error) {
	return iofs.Sub(root.SkillsFS, ".claude/skills")
}

// bootstrapBase faz dirs + CLAUDE.md + skills na base + git init + seed + commit
// + (opcional) criação/push do repo no GitHub. Preenche out.
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

	if _, err := git.Commit("chore: assets do agente vbrain (CLAUDE.md + skills + rotinas)", dataHome); err != nil {
		return err
	}

	if github != "none" && github != "" {
		if token == "" {
			out["needs_token"] = true
			out["github"] = github
			return nil
		}
		url, err := createGitHubRepo(repoName, github == "private", token)
		if err != nil {
			return err
		}
		if !git.HasRemote(dataHome, "origin") {
			if err := git.AddRemote(url, dataHome, "origin"); err != nil {
				return err
			}
		}
		os.Setenv("GITHUB_TOKEN", token) // go-git push usa o PAT
		if _, err := git.Push(dataHome, "origin", ""); err != nil {
			return err
		}
		out["remote_url"] = url
		out["pushed"] = true
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

// installSelf copia o binário em execução para binDir (no-op se já roda de lá).
// Devolve o caminho final e se binDir já está no PATH.
func installSelf(binDir string) (string, bool, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", false, err
	}
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

// onboard pergunta (só em terminal) identidade git e visibilidade/PAT do GitHub.
func onboard(github, repoName, token *string) {
	if !isTerminal() {
		return
	}
	ensureGitIdentity()
	if *github == "none" {
		switch strings.ToLower(prompt("Versionar a base no GitHub? [p]rivado/[u]público/[n]enhum (p): ")) {
		case "u", "public":
			*github = "public"
		case "n", "none":
			*github = "none"
		default: // "" (default) ou "p" → privado
			*github = "private"
		}
	}
	if *github != "none" && *token == "" {
		fmt.Fprintln(os.Stderr, "Crie um PAT (escopo 'repo'): https://github.com/settings/tokens/new?scopes=repo&description=vbrain")
		*token = prompt("Cole o PAT (vazio pula o GitHub): ")
		if *token == "" {
			*github = "none"
		} else if *repoName == "" {
			*repoName = "vbrain"
		}
	}
}

// ensureGitIdentity preenche user.name/email globais via git do sistema, se
// presente e faltando.
func ensureGitIdentity() {
	if _, err := exec.LookPath("git"); err != nil {
		return
	}
	for key, q := range map[string]string{"user.name": "Seu nome para os commits: ", "user.email": "Seu email para os commits: "} {
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

// createGitHubRepo cria um repo via API REST do GitHub usando o PAT, e devolve
// a URL de clone HTTPS. Idempotente-ish: trata 422 (já existe) como ok.
func createGitHubRepo(name string, private bool, token string) (string, error) {
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
	case 422: // provavelmente já existe — monta a URL a partir do login
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
		fmt.Printf("Nenhum resultado para `%s`.\n", res.Query)
		return
	}
	fmt.Printf("# Resultados para `%s`\n\n", res.Query)
	for i, r := range res.Results {
		fmt.Printf("## %d. %s\n\n", i+1, r.Title)
		fmt.Printf("**Path:** `wiki/%s`\n", r.Path)
		if r.Kind != "" {
			fmt.Printf("**Kind:** `%s`\n", r.Kind)
		}
		fmt.Printf("\n%s\n\n", r.Snippet)
	}
	if len(res.Related) > 0 {
		fmt.Print("## Relacionadas (grafo)\n\n")
		for _, r := range res.Related {
			fmt.Printf("- **%s** — `wiki/%s`\n", r.Title, r.Path)
		}
		fmt.Println()
	}
}
