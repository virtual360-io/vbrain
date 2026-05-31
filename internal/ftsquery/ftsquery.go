// Package ftsquery normaliza consultas em linguagem natural para a sintaxe
// MATCH do FTS5. Porta determinística de lib/vbrain/fts_query.rb.
package ftsquery

import (
	"regexp"
	"strings"
)

// stopChars são caracteres com significado na sintaxe do FTS5 (aspas, dois
// pontos, parênteses etc.); neutralizados para espaço antes de tokenizar.
var stopChars = regexp.MustCompile("[\":()\\[\\]{}<>!?,;`]")

var whitespace = regexp.MustCompile(`\s+`)

// stopwords PT-BR (+ algumas EN comuns): palavras-função, pronomes,
// interrogativas e auxiliares de alta frequência. Sob OR elas afogam o sinal
// no BM25, então filtramos antes de montar a query. Inclui formas com e sem
// acento porque o token preserva o acento original.
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

// Normalize devolve a expressão MATCH (tokens entre aspas unidos por OR), ou
// "" para entrada vazia. Em prefix mode, cada token vira `"tok"*`.
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
	// Se só sobraram stopwords (ex.: "quem é você"), cair pros tokens
	// originais é melhor que devolver vazio (zero hits).
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
