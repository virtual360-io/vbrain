// Command vbrain é o CLI determinístico do vbrain (reindex, query, …). Porta
// dos scripts Ruby: JSON no stdout (lido pelas skills), texto humano no stderr.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/virtual360-io/vbrain/internal/db"
	"github.com/virtual360-io/vbrain/internal/index"
	"github.com/virtual360-io/vbrain/internal/paths"
	"github.com/virtual360-io/vbrain/internal/search"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "uso: vbrain <reindex|query> [args]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "reindex":
		err = cmdReindex(os.Args[2:])
	case "query":
		err = cmdQuery(os.Args[2:])
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
