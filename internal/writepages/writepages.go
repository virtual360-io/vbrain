// Package writepages publica páginas wiki a partir do output do writer (LLM),
// encenando tudo num diretório temporário e só então commitando "de uma vez
// só". Porta determinística de scripts/write_pages.rb. A wiki nunca fica num
// estado meio-escrito; um guardrail pré-commit barra reorgs que orfanariam raws.
package writepages

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/virtual360-io/vbrain/internal/page"
	"github.com/virtual360-io/vbrain/internal/paths"
	"github.com/virtual360-io/vbrain/internal/slug"
)

// PageInput é uma entrada do pages_json produzido pelo writer.
type PageInput struct {
	Op           string   `json:"op"`
	Slug         string   `json:"slug"`
	SlugHint     string   `json:"slug_hint"`
	Title        string   `json:"title"`
	BodyMarkdown string   `json:"body_markdown"`
	Kind         string   `json:"kind"`
	Tags         []string `json:"tags"`
}

// Result é o JSON de saída.
type Result struct {
	Committed    *bool    `json:"committed,omitempty"`
	NeedsReview  bool     `json:"needs_review,omitempty"`
	OrphanedRaws []string `json:"orphaned_raws,omitempty"`
	Written      []string `json:"written"`
	Updated      []string `json:"updated"`
	Deleted      []string `json:"deleted"`
	Count        int      `json:"count"`
}

var kindSet = func() map[string]bool {
	m := map[string]bool{}
	for _, k := range paths.Kinds {
		m[k] = true
	}
	return m
}()

// WritePages encena e publica as páginas. Retorna needs_review (sem commitar) se
// alguma reorg deixaria um raw órfão.
func WritePages(db *sql.DB, rawID int, pages []PageInput, wikiDir, tmpDir, dataHome string) (Result, error) {
	var rawPath string
	if err := db.QueryRow("SELECT path FROM raw_sources WHERE id = ?", rawID).Scan(&rawPath); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Result{}, errors.New("raw_id not found")
		}
		return Result{}, err
	}
	rawRel := strings.TrimPrefix(rawPath, dataHome+string(filepath.Separator))

	stageDir := filepath.Join(tmpDir, "wiki-stage-"+itoa(rawID))
	trashDir := filepath.Join(stageDir, ".trash")
	if err := os.RemoveAll(stageDir); err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return Result{}, err
	}

	existing := basenamesSet(filepath.Join(wikiDir, "*.md"))
	stagedSlugs := map[string]string{} // slug => "create"|"update"

	var written, updated, deleteSlugs []string

	readFrontmatter := func(s string) map[string]any {
		staged := filepath.Join(stageDir, s+".md")
		live := filepath.Join(wikiDir, s+".md")
		src := ""
		if fileExists(staged) {
			src = staged
		} else if fileExists(live) {
			src = live
		}
		if src == "" {
			return nil
		}
		p, err := page.Parse(src)
		if err != nil {
			return nil
		}
		return p.Frontmatter
	}

	for _, p := range pages {
		if p.Op == "delete" {
			if s := strings.TrimSpace(p.Slug); s != "" {
				deleteSlugs = append(deleteSlugs, s)
			}
			continue
		}

		kind := p.Kind
		if !kindSet[kind] {
			kind = "note"
		}
		op := "create"
		if p.Op == "update" {
			op = "update"
		}
		targetSlug := strings.TrimSpace(p.Slug)
		isUpdate := op == "update" && targetSlug != "" &&
			(existing[targetSlug] || stagedSlugs[targetSlug] != "")

		if isUpdate {
			s := targetSlug
			prev := readFrontmatter(s)
			if prev == nil {
				prev = map[string]any{}
			}
			mergedTags := uniq(append(toStrings(prev["tags"]), p.Tags...))
			sources := uniq(append(toStrings(prev["source_raw"]), rawRel))
			fm := map[string]any{
				"title":      orStr(asStr(prev["title"]), p.Title),
				"kind":       orStr(asStr(prev["kind"]), kind),
				"tags":       mergedTags,
				"source_raw": collapse(sources),
			}
			if _, err := page.Write(stageDir, s, fm, p.BodyMarkdown); err != nil {
				return Result{}, err
			}
			stagedSlugs[s] = "update"
			if !contains(updated, s+".md") {
				updated = append(updated, s+".md")
			}
		} else {
			base, err := slug.From(orStr(p.SlugHint, p.Title))
			if err != nil {
				return Result{}, err
			}
			s := base
			for n := 2; existing[s] || stagedSlugs[s] != ""; n++ {
				s = base + "-" + itoa(n)
			}
			fm := map[string]any{
				"title":      p.Title,
				"kind":       kind,
				"tags":       p.Tags,
				"source_raw": rawRel,
			}
			if _, err := page.Write(stageDir, s, fm, p.BodyMarkdown); err != nil {
				return Result{}, err
			}
			stagedSlugs[s] = "create"
			written = append(written, s+".md")
		}
	}

	// Copia pro .trash/ os deletes (cópia — wiki segue intacta); pula slug
	// criado/atualizado nesta run e inexistente.
	for _, s := range uniq(deleteSlugs) {
		if stagedSlugs[s] != "" {
			continue
		}
		live := filepath.Join(wikiDir, s+".md")
		if !fileExists(live) {
			continue
		}
		if err := os.MkdirAll(trashDir, 0o755); err != nil {
			return Result{}, err
		}
		if err := copyFile(live, filepath.Join(trashDir, s+".md")); err != nil {
			return Result{}, err
		}
	}

	// Guardrail pré-commit: nenhum raw citado hoje pode ficar órfão.
	liveFiles, _ := filepath.Glob(filepath.Join(wikiDir, "*.md"))
	removedOrReplaced := map[string]bool{}
	for _, s := range deleteSlugs {
		removedOrReplaced[s] = true
	}
	for s := range stagedSlugs {
		removedOrReplaced[s] = true
	}
	var surviving []string
	for _, f := range liveFiles {
		if !removedOrReplaced[strings.TrimSuffix(filepath.Base(f), ".md")] {
			surviving = append(surviving, f)
		}
	}
	stagedFiles, _ := filepath.Glob(filepath.Join(stageDir, "*.md"))

	citedBefore := collectRaws(liveFiles)
	citedAfter := collectRaws(surviving)
	for r := range collectRaws(stagedFiles) {
		citedAfter[r] = true
	}
	var orphaned []string
	for r := range citedBefore {
		if !citedAfter[r] {
			orphaned = append(orphaned, r)
		}
	}
	if len(orphaned) > 0 {
		sort.Strings(orphaned)
		os.RemoveAll(stageDir)
		committed := false
		return Result{
			Committed: &committed, NeedsReview: true, OrphanedRaws: orphaned,
			Written: []string{}, Updated: []string{}, Deleted: []string{}, Count: 0,
		}, nil
	}

	// Commit: mv staged → wiki, rm originais com cópia no trash, apaga a temp.
	var removed []string
	for _, staged := range stagedFiles {
		if err := os.Rename(staged, filepath.Join(wikiDir, filepath.Base(staged))); err != nil {
			return Result{}, err
		}
	}
	if dirExists(trashDir) {
		trashed, _ := filepath.Glob(filepath.Join(trashDir, "*.md"))
		for _, tf := range trashed {
			base := filepath.Base(tf)
			target := filepath.Join(wikiDir, base)
			if fileExists(target) {
				os.Remove(target)
			}
			removed = append(removed, base)
		}
	}
	os.RemoveAll(stageDir)

	return Result{
		Written: nz(written), Updated: nz(updated), Deleted: nz(removed),
		Count: len(written) + len(updated) + len(removed),
	}, nil
}

// collectRaws junta todos os source_raw citados num conjunto de arquivos.
func collectRaws(files []string) map[string]bool {
	set := map[string]bool{}
	for _, f := range files {
		p, err := page.Parse(f)
		if err != nil {
			continue
		}
		for _, r := range toStrings(p.Frontmatter["source_raw"]) {
			set[r] = true
		}
	}
	return set
}
