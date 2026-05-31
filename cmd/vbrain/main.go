// Command vbrain é o CLI determinístico do vbrain (reindex, query, …). Porta
// dos scripts Ruby: JSON no stdout (lido pelas skills), texto humano no stderr.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/virtual360-io/vbrain/internal/db"
	"github.com/virtual360-io/vbrain/internal/git"
	"github.com/virtual360-io/vbrain/internal/index"
	"github.com/virtual360-io/vbrain/internal/paths"
	"github.com/virtual360-io/vbrain/internal/resolvelinks"
	"github.com/virtual360-io/vbrain/internal/search"
	"github.com/virtual360-io/vbrain/internal/writepages"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "uso: vbrain <reindex|query|commit|write-pages|resolve-links> [args]")
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
