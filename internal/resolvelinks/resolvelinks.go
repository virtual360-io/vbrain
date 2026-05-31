// Package resolvelinks applies a {title => slug} map produced by the LLM to the
// unresolved [[wikilinks]]. Deterministic port of scripts/resolve_links.rb: the
// DECISION is the LLM's; here we only apply it (Rule 5), discarding nonexistent
// slugs.
package resolvelinks

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/virtual360-io/vbrain/internal/links"
	"github.com/virtual360-io/vbrain/internal/page"
)

// Result is the output JSON.
type Result struct {
	Changed            int `json:"changed"`
	Applied            int `json:"applied"`
	DroppedUnknownSlug int `json:"dropped_unknown_slug"`
}

// ResolveLinks filters the map to slugs that actually exist and rewrites the
// bodies. Returns how many files changed, how many entries were applied, and how
// many were discarded (null/empty/nonexistent slug).
func ResolveLinks(wikiDir string, mapping map[string]string) (Result, error) {
	mdFiles, err := walkMarkdown(wikiDir)
	if err != nil {
		return Result{}, err
	}

	existing := map[string]bool{}
	for _, f := range mdFiles {
		existing[strings.TrimSuffix(filepath.Base(f), ".md")] = true
	}

	safe := map[string]string{}
	for title, slug := range mapping {
		if slug != "" && existing[slug] {
			safe[title] = slug
		}
	}
	dropped := len(mapping) - len(safe)

	changed := 0
	for _, f := range mdFiles {
		did, err := page.RewriteBody(f, func(body string) string {
			return links.ApplyResolution(body, safe)
		})
		if err != nil {
			return Result{}, err
		}
		if did {
			changed++
		}
	}

	return Result{Changed: changed, Applied: len(safe), DroppedUnknownSlug: dropped}, nil
}

func walkMarkdown(wikiDir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(wikiDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".md") {
			out = append(out, p)
		}
		return nil
	})
	return out, err
}
