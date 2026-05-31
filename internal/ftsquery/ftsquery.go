// Package ftsquery normalizes natural-language queries into FTS5 MATCH syntax.
// Deterministic port of lib/vbrain/fts_query.rb.
package ftsquery

import (
	"regexp"
	"strings"
)

// stopChars are characters meaningful to FTS5 syntax (quotes, colon,
// parentheses, etc.); neutralized to spaces before tokenizing.
var stopChars = regexp.MustCompile("[\":()\\[\\]{}<>!?,;`]")

var whitespace = regexp.MustCompile(`\s+`)

// stopwords (PT-BR + some common EN): function words, pronouns, interrogatives,
// and high-frequency auxiliaries. Under OR they drown the signal in BM25, so we
// filter them before building the query. Kept in Portuguese on purpose — they
// exist to handle Portuguese queries — and include accented and unaccented
// forms because the token preserves the original accent.
var stopwords = map[string]bool{}

func init() {
	list := strings.Fields(`
		a o as os um uma uns umas
		de do da dos das em no na nos nas ao aos à às
		por pra para per com sem sob sobre entre ate até desde
		e ou mas nem que se como quando onde porque pois
		qual quais quanto quanta quantos quantas quem cujo cuja
		eu tu ele ela nos vos eles elas voce voces vc vcs
		me te lhe nos vos lhes meu minha meus minhas teu tua seu sua seus suas
		este esta isto esse essa isso aquele aquela aquilo
		ja já nao não sim talvez muito muita pouco pouca mais menos
		ter tem tenho tinha tive teve tinham foram foi sou somos sao são
		era eram estar esta está estou estava estavam ser
		the of to in on for and or is are was were be been being
		i you he she it we they my your what which who when where how`)
	for _, w := range list {
		stopwords[w] = true
	}
}

// Normalize returns the MATCH expression (quoted tokens joined by OR), or "" for
// empty input. In prefix mode, each token becomes `"tok"*`.
func Normalize(query string, prefix bool) string {
	cleaned := stopChars.ReplaceAllString(query, " ")
	tokens := splitNonEmpty(cleaned)
	if len(tokens) == 0 {
		return ""
	}

	var kept []string
	for _, t := range tokens {
		if !stopwords[strings.ToLower(t)] {
			kept = append(kept, t)
		}
	}
	// If only stopwords remained (e.g. "quem é você"), falling back to the
	// original tokens is better than returning empty (zero hits).
	if len(kept) > 0 {
		tokens = kept
	}

	out := make([]string, len(tokens))
	for i, t := range tokens {
		if prefix {
			out[i] = `"` + t + `"*`
		} else {
			out[i] = `"` + t + `"`
		}
	}
	return strings.Join(out, " OR ")
}

func splitNonEmpty(s string) []string {
	var out []string
	for _, t := range whitespace.Split(s, -1) {
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}
