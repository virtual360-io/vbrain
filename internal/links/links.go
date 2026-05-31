// Package links faz o parse determinístico de links entre páginas. A LLM
// escreve `[[Título]]` (autoria); linkify converte os resolvíveis para
// `[Título](slug.md)`. Porta de lib/vbrain/links.rb.
package links

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/virtual360-io/vbrain/internal/slug"
)

var (
	wikilinkRE = regexp.MustCompile(`\[\[([^\]\[]+)\]\]`)
	// Link markdown apontando para um .md local (a forma linkificada).
	mdlinkRE = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+\.md)\)`)
)

// Link é uma aresta de saída: slug do alvo + título de exibição.
type Link struct {
	Slug  string
	Title string
}

// Extract devolve os links de saída do body em ambas as formas (`[[Título]]` e
// `[texto](slug.md)`), deduplicados por slug e em ordem. Suporta alias
// `[[Alvo|texto]]` (slug/título vêm do alvo).
func Extract(body string) []Link {
	var out []Link
	seen := map[string]bool{}
	add := func(s, title string) {
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, Link{Slug: s, Title: title})
	}

	for _, m := range wikilinkRE.FindAllStringSubmatch(body, -1) {
		target := strings.TrimSpace(strings.SplitN(m[1], "|", 2)[0])
		if target == "" {
			continue
		}
		add(targetSlug(target), target)
	}

	for _, m := range mdlinkRE.FindAllStringSubmatch(body, -1) {
		text := strings.TrimSpace(m[1])
		s := strings.TrimSuffix(filepath.Base(strings.TrimSpace(m[2])), ".md")
		if text == "" {
			text = s
		}
		add(s, text)
	}

	return out
}

// targetSlug normaliza um alvo para o slug ASCII que write_pages usa como nome
// de arquivo. Slug inválido → "" (não-resolvível).
func targetSlug(target string) string {
	s, err := slug.From(target)
	if err != nil {
		return ""
	}
	return s
}

// Linkify reescreve cada `[[Título]]` cujo slug existe em existingSlugs como
// link markdown clicável. Não-resolvíveis ficam intactos. Idempotente.
func Linkify(body string, existingSlugs []string) string {
	set := map[string]bool{}
	for _, s := range existingSlugs {
		set[s] = true
	}
	return transformWikilinks(body, func(target, display string) (string, bool) {
		s := targetSlug(target)
		if s != "" && set[s] {
			return "[" + display + "](" + s + ".md)", true
		}
		return "", false
	})
}

// ApplyResolution aplica um mapa {título => slug} produzido pela camada de
// julgamento (LLM): reescreve `[[Título]]` → `[texto](slug.md)` quando o título
// está no mapa com slug não-vazio. Idempotente. Aqui só APLICAMOS (Regra 5).
func ApplyResolution(body string, mapping map[string]string) string {
	if len(mapping) == 0 {
		return body
	}
	return transformWikilinks(body, func(target, display string) (string, bool) {
		if s := mapping[target]; s != "" {
			return "[" + display + "](" + s + ".md)", true
		}
		return "", false
	})
}

// transformWikilinks aplica repl a cada wikilink. repl recebe alvo e texto de
// exibição (alias após `|`, ou o próprio alvo) e devolve (substituição, ok); se
// ok=false o wikilink fica intacto.
func transformWikilinks(body string, repl func(target, display string) (string, bool)) string {
	return wikilinkRE.ReplaceAllStringFunc(body, func(whole string) string {
		inner := whole[2 : len(whole)-2] // tira [[ ]]
		parts := strings.SplitN(inner, "|", 2)
		target := strings.TrimSpace(parts[0])
		display := target
		if len(parts) == 2 {
			display = strings.TrimSpace(parts[1])
		}
		if out, ok := repl(target, display); ok {
			return out
		}
		return whole
	})
}
