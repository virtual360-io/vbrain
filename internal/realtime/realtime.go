// Package realtime gera as "páginas fantasma" realtime (kind=realtime) e seus
// configs. Quando o /vbrain-query-knowledge recebe uma dessas no FTS5, o agente
// NÃO devolve o body — chama o handler MCP correspondente. Porta determinística
// de lib/vbrain/realtime/*.rb. O dispatch MCP em si é da skill.
package realtime

import (
	"os"
	"path/filepath"

	"github.com/virtual360-io/vbrain/internal/page"
	"github.com/virtual360-io/vbrain/internal/paths"
	"gopkg.in/yaml.v3"
)

// Item é uma entrada de fonte (calendário/label/canal) com campos string.
type Item map[string]string

func configPath(name string) string {
	return filepath.Join(paths.DataHome(), "config", "realtime", name+".yml")
}

func realtimeDir() string { return filepath.Join(paths.WikiDir(), paths.RealtimeDir) }

// saveConfig grava {key: items} em config/realtime/<name>.yml.
func saveConfig(name, key string, items []Item) error {
	path := configPath(name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	out, err := yaml.Marshal(map[string]any{key: items})
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

// loadConfig lê os items de config/realtime/<name>.yml; ok=false se não existe.
func loadConfig(name, key string) ([]Item, bool) {
	data, err := os.ReadFile(configPath(name))
	if err != nil {
		return nil, false
	}
	var raw map[string][]map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, true
	}
	items := make([]Item, 0, len(raw[key]))
	for _, m := range raw[key] {
		it := Item{}
		for k, v := range m {
			if s, ok := v.(string); ok {
				it[k] = s
			}
		}
		items = append(items, it)
	}
	return items, true
}

func writePage(slug string, fm map[string]any, body string) (string, error) {
	dir := realtimeDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return page.Write(dir, slug, fm, body)
}

// itemsAny converte []Item em []any para o frontmatter.
func itemsAny(items []Item) []any {
	out := make([]any, len(items))
	for i, it := range items {
		m := map[string]any{}
		for k, v := range it {
			m[k] = v
		}
		out[i] = m
	}
	return out
}
