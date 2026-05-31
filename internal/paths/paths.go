// Package paths resolve os diretórios de dados do vbrain a partir de
// VBRAIN_HOME (ou ~/vbrain). Porta determinística de lib/vbrain/paths.rb.
package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// RealtimeDir é o único subdir especial da wiki: páginas fantasma com handler
// MCP, escritas por outra skill, não pelo pipeline de ingest.
const RealtimeDir = "_realtime"

// Kinds são os valores válidos de `kind` no frontmatter. Metadado livre — não
// determina mais a pasta (layout plano, estilo ai-memory).
var Kinds = []string{"concept", "decision", "gotcha", "note", "rule", "realtime"}

// DataHome devolve a raiz dos dados: VBRAIN_HOME se setado e não-vazio, senão
// ~/vbrain.
func DataHome() string {
	if env := os.Getenv("VBRAIN_HOME"); env != "" {
		return expand(env)
	}
	return expand(filepath.Join("~", "vbrain"))
}

func RawDir() string  { return filepath.Join(DataHome(), "raw") }
func WikiDir() string { return filepath.Join(DataHome(), "wiki") }
func DBDir() string   { return filepath.Join(DataHome(), "db") }
func DBPath() string  { return filepath.Join(DBDir(), "vbrain.sqlite3") }
func TmpDir() string  { return filepath.Join(RawDir(), ".tmp") }

// EnsureDirs cria a estrutura plana de diretórios mais wiki/_realtime.
func EnsureDirs() error {
	dirs := []string{RawDir(), WikiDir(), DBDir(), TmpDir(), filepath.Join(WikiDir(), RealtimeDir)}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// expand resolve `~` para o home do usuário e torna o caminho absoluto,
// espelhando File.expand_path do Ruby.
func expand(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}
