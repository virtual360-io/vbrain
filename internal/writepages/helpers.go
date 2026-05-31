package writepages

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
)

func itoa(n int) string { return strconv.Itoa(n) }

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// basenamesSet devolve os basenames (sem .md) que casam o glob.
func basenamesSet(glob string) map[string]bool {
	set := map[string]bool{}
	matches, _ := filepath.Glob(glob)
	for _, m := range matches {
		set[trimMD(m)] = true
	}
	return set
}

func trimMD(p string) string {
	b := filepath.Base(p)
	return b[:len(b)-len(".md")]
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// toStrings normaliza nil/string/[]any/[]string numa fatia de strings (espelha
// Array(x) do Ruby para valores de frontmatter).
func toStrings(v any) []string {
	switch x := v.(type) {
	case nil:
		return nil
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

func asStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func orStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// uniq deduplica preservando a ordem.
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

// collapse devolve a única string quando há uma só fonte, senão a fatia (espelha
// `sources.size == 1 ? sources.first : sources`).
func collapse(sources []string) any {
	if len(sources) == 1 {
		return sources[0]
	}
	return sources
}

// nz garante fatia não-nil (pra JSON serializar [] e não null).
func nz(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
