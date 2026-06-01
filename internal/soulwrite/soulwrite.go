// Package soulwrite is the only writer into the soul layer (wiki/_soul/): the
// identity pages describing how and why the user acts. It is deliberately
// separate from writepages — soul pages are synthesized by the daily soul
// routine from the user's actions, not ingested from a raw source, so there is
// no raw and no orphan guardrail. The add-knowledge pipeline must never reach
// here; only the soul routine does, after consolidation.
package soulwrite

import (
	"os"
	"path/filepath"

	"github.com/virtual360-io/vbrain/internal/page"
	"github.com/virtual360-io/vbrain/internal/paths"
	"github.com/virtual360-io/vbrain/internal/slug"
)

// PageInput is one soul page from the routine's consolidation output. Kind is
// always forced to "soul" — the layer is fixed, the routine only decides
// create/update/delete (beliefs can be discarded over time).
type PageInput struct {
	Op           string   `json:"op"` // create|update|delete (default create)
	Slug         string   `json:"slug"`
	SlugHint     string   `json:"slug_hint"`
	Title        string   `json:"title"`
	BodyMarkdown string   `json:"body_markdown"`
	Tags         []string `json:"tags"`
}

// Result is the output JSON.
type Result struct {
	Written []string `json:"written"`
	Updated []string `json:"updated"`
	Deleted []string `json:"deleted"`
	Count   int      `json:"count"`
}

// SoulWrite publishes soul pages into wikiDir/_soul. Each create/update is an
// atomic per-file write; a delete removes the page (pruning a belief the user
// no longer holds). Returns the relative paths touched. There is no orphan
// guardrail and no DB access — the soul layer is grounded in the user's
// actions, not in raw sources.
func SoulWrite(pages []PageInput, wikiDir string) (Result, error) {
	soulDir := filepath.Join(wikiDir, paths.SoulDir)
	if err := os.MkdirAll(soulDir, 0o755); err != nil {
		return Result{}, err
	}

	existing := existingSlugs(soulDir)
	staged := map[string]bool{} // slugs created/updated in this run

	var written, updated, deleted []string

	for _, p := range pages {
		if p.Op == "delete" {
			s := trimSlug(p.Slug)
			if s == "" {
				continue
			}
			full := filepath.Join(soulDir, s+".md")
			if fileExists(full) {
				if err := os.Remove(full); err != nil {
					return Result{}, err
				}
				deleted = append(deleted, rel(s))
			}
			continue
		}

		target := trimSlug(p.Slug)
		isUpdate := p.Op == "update" && target != "" && (existing[target] || staged[target])

		if isUpdate {
			s := target
			prev := readFrontmatter(soulDir, s)
			fm := map[string]any{
				"title": firstNonEmpty(asString(prev["title"]), p.Title),
				"kind":  "soul",
				"tags":  uniq(append(toStrings(prev["tags"]), p.Tags...)),
			}
			if _, err := page.Write(soulDir, s, fm, p.BodyMarkdown); err != nil {
				return Result{}, err
			}
			staged[s] = true
			if !contains(updated, rel(s)) {
				updated = append(updated, rel(s))
			}
			continue
		}

		base, err := slug.From(firstNonEmpty(p.SlugHint, p.Title))
		if err != nil {
			return Result{}, err
		}
		s := base
		for n := 2; existing[s] || staged[s]; n++ {
			s = base + "-" + itoa(n)
		}
		fm := map[string]any{"title": p.Title, "kind": "soul", "tags": p.Tags}
		if _, err := page.Write(soulDir, s, fm, p.BodyMarkdown); err != nil {
			return Result{}, err
		}
		staged[s] = true
		written = append(written, rel(s))
	}

	return Result{
		Written: nz(written), Updated: nz(updated), Deleted: nz(deleted),
		Count: len(written) + len(updated) + len(deleted),
	}, nil
}

func rel(s string) string { return paths.SoulDir + "/" + s + ".md" }

func readFrontmatter(soulDir, s string) map[string]any {
	full := filepath.Join(soulDir, s+".md")
	if !fileExists(full) {
		return map[string]any{}
	}
	p, err := page.Parse(full)
	if err != nil {
		return map[string]any{}
	}
	return p.Frontmatter
}
