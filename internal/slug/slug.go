// Package slug generates stable ASCII slugs from titles. Deterministic port of
// lib/vbrain/slug.rb.
package slug

import (
	"errors"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// MaxLength is the default maximum slug length.
const MaxLength = 80

// ErrEmpty is returned when the title is empty or yields no usable character.
var ErrEmpty = errors.New("slug: title cannot be nil or empty / yielded empty slug")

var (
	nonAlnum    = regexp.MustCompile(`[^a-z0-9]+`)
	trailingSep = regexp.MustCompile(`-+$`)
)

// From converts a title into a slug using the default MaxLength.
func From(title string) (string, error) {
	return FromMax(title, MaxLength)
}

// FromMax converts a title into a slug with a custom maximum length.
//
// Pipeline (same as Ruby): NFKD → drop non-ASCII (combining marks and accented
// bases disappear) → lowercase → collapse punctuation into "-" → trim the ends →
// truncate without a trailing dash.
func FromMax(title string, maxLength int) (string, error) {
	if strings.TrimSpace(title) == "" {
		return "", ErrEmpty
	}

	var b strings.Builder
	for _, r := range norm.NFKD.String(title) {
		if r > unicode.MaxASCII || unicode.Is(unicode.Mn, r) {
			continue // drop non-ASCII and combining marks
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
