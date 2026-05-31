// Package realtime generates the realtime "phantom pages" (kind=realtime) and
// their configs. When /vbrain-query-knowledge hits one in FTS5, the agent does
// NOT return the body — it calls the corresponding MCP handler. Deterministic
// port of lib/vbrain/realtime/*.rb. The MCP dispatch itself is the skill's job.
package realtime

import (
	"os"
	"path/filepath"

	"github.com/virtual360-io/vbrain/internal/page"
	"github.com/virtual360-io/vbrain/internal/paths"
	"gopkg.in/yaml.v3"
)

// Item is a source entry (calendar/label/channel) with string fields.
type Item map[string]string

func configPath(name string) string {
	return filepath.Join(paths.DataHome(), "config", "realtime", name+".yml")
}

func realtimeDir() string { return filepath.Join(paths.WikiDir(), paths.RealtimeDir) }

// saveConfig writes {key: items} to config/realtime/<name>.yml.
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

// loadConfig reads the items from config/realtime/<name>.yml; ok=false if it
// doesn't exist.
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

// itemsAny converts []Item into []any for the frontmatter.
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
