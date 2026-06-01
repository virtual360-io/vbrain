package soulwrite

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func itoa(n int) string { return strconv.Itoa(n) }

func trimSlug(s string) string { return strings.TrimSpace(strings.TrimSuffix(s, ".md")) }

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

// existingSlugs lists the slugs already present in the soul dir.
func existingSlugs(soulDir string) map[string]bool {
	set := map[string]bool{}
	matches, _ := filepath.Glob(filepath.Join(soulDir, "*.md"))
	for _, m := range matches {
		set[strings.TrimSuffix(filepath.Base(m), ".md")] = true
	}
	return set
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func toStrings(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

func nz(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
