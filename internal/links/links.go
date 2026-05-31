// Package links does the deterministic parsing of links between pages. The LLM
// writes `[[Title]]` (authoring form); linkify converts the resolvable ones into
// `[Title](slug.md)`. Port of lib/vbrain/links.rb.
package links

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/virtual360-io/vbrain/internal/slug"
)

var (
	wikilinkRE = regexp.MustCompile(`\[\[([^\]\[]+)\]\]`)
	// Markdown link pointing to a local .md (the linkified form).
	mdlinkRE = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+\.md)\)`)
)

// Link is an outgoing edge: target slug + display title.
type Link struct {
	Slug  string
	Title string
}

// Extract returns the body's outgoing links in both forms (`[[Title]]` and
// `[text](slug.md)`), deduplicated by slug and in order. Supports the
// `[[Target|text]]` alias (slug/title come from the target).
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

// targetSlug normalizes a target into the ASCII slug that write-pages uses as
// the file name. Invalid slug → "" (unresolvable).
func targetSlug(target string) string {
	s, err := slug.From(target)
	if err != nil {
		return ""
	}
	return s
}

// Linkify rewrites each `[[Title]]` whose slug exists in existingSlugs into a
// clickable markdown link. Unresolvable ones are left intact. Idempotent.
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

// ApplyResolution applies a {title => slug} map produced by the judgment layer
// (LLM): rewrites `[[Title]]` → `[text](slug.md)` when the title is in the map
// with a non-empty slug. Idempotent. Here we only APPLY (Rule 5).
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

// transformWikilinks applies repl to each wikilink. repl receives the target and
// display text (alias after `|`, or the target itself) and returns
// (replacement, ok); if ok=false the wikilink is left intact.
func transformWikilinks(body string, repl func(target, display string) (string, bool)) string {
	return wikilinkRE.ReplaceAllStringFunc(body, func(whole string) string {
		inner := whole[2 : len(whole)-2] // strip [[ ]]
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
