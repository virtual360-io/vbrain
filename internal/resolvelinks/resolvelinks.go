// Package resolvelinks aplica um mapa {título => slug} produzido pela LLM aos
// [[wikilinks]] não-resolvidos. Porta determinística de scripts/resolve_links.rb:
// a DECISÃO é da LLM; aqui só aplicamos (Regra 5), descartando slugs inexistentes.
package resolvelinks

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/virtual360-io/vbrain/internal/links"
	"github.com/virtual360-io/vbrain/internal/page"
)

// Result é o JSON de saída.
type Result struct {
	Changed            int `json:"changed"`
	Applied            int `json:"applied"`
	DroppedUnknownSlug int `json:"dropped_unknown_slug"`
}

// ResolveLinks filtra o mapa para slugs que existem de fato e reescreve os
// corpos. Retorna quantos arquivos mudaram, quantas entradas foram aplicadas e
// quantas descartadas (slug nulo/vazio/inexistente).
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
