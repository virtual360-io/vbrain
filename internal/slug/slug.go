// Package slug gera slugs ASCII estáveis a partir de títulos. Porta
// determinística de lib/vbrain/slug.rb.
package slug

import (
	"errors"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// MaxLength é o comprimento máximo padrão do slug.
const MaxLength = 80

// ErrEmpty é retornado quando o título é vazio ou não produz nenhum caractere
// aproveitável.
var ErrEmpty = errors.New("slug: title cannot be nil or empty / yielded empty slug")

var (
	nonAlnum    = regexp.MustCompile(`[^a-z0-9]+`)
	trailingSep = regexp.MustCompile(`-+$`)
)

// From converte um título em slug usando o MaxLength padrão.
func From(title string) (string, error) {
	return FromMax(title, MaxLength)
}

// FromMax converte um título em slug com comprimento máximo customizado.
//
// Pipeline (igual ao Ruby): NFKD → drop de não-ASCII (combining marks e bases
// acentuadas somem) → lowercase → colapso de pontuação em "-" → trim das
// pontas → truncamento sem traço final.
func FromMax(title string, maxLength int) (string, error) {
	if strings.TrimSpace(title) == "" {
		return "", ErrEmpty
	}

	var b strings.Builder
	for _, r := range norm.NFKD.String(title) {
		if r > unicode.MaxASCII || unicode.Is(unicode.Mn, r) {
			continue // descarta não-ASCII e marcas combinantes
		}
		b.WriteRune(r)
	}

	s := strings.ToLower(b.String())
	s = nonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")

	if s == "" {
		return "", ErrEmpty
	}

	if len(s) > maxLength {
		s = s[:maxLength]
	}
	s = trailingSep.ReplaceAllString(s, "")
	return s, nil
}
