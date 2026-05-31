// Package page reads and writes markdown pages with YAML frontmatter.
// Deterministic port of lib/vbrain/page.rb.
package page

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"
)

// frontmatterRE separates the YAML block from the body. (?s) = `.` matches
// newline (Ruby's /m flag).
var frontmatterRE = regexp.MustCompile(`(?s)\A---\s*\n(.*?)\n---\s*\n(.*)\z`)

// ErrInvalid covers a non-existent dir or an empty slug in Write.
var ErrInvalid = errors.New("page: dir must exist and slug cannot be empty")

// Parsed is the result of parsing: frontmatter, body, and the body's sha256.
type Parsed struct {
	Frontmatter map[string]any
	Body        string
	SHA256      string
}

// Parse reads and parses a page from disk.
func Parse(path string) (Parsed, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Parsed{}, err
	}
	return ParseString(string(content))
}

// ParseString parses content in memory. Without frontmatter, body = the whole
// content and frontmatter is empty.
func ParseString(content string) (Parsed, error) {
	fm := map[string]any{}
	body := content
	if m := frontmatterRE.FindStringSubmatch(content); m != nil {
		if err := yaml.Unmarshal([]byte(m[1]), &fm); err != nil {
			return Parsed{}, err
		}
		if fm == nil {
			fm = map[string]any{}
		}
		body = m[2]
	}
	return Parsed{Frontmatter: fm, Body: body, SHA256: sha256hex(body)}, nil
}

// Write renders frontmatter+body and writes it atomically to dir/<slug>.md.
func Write(dir, slug string, frontmatter map[string]any, body string) (string, error) {
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return "", ErrInvalid
	}
	if slug == "" {
		return "", ErrInvalid
	}

	full := filepath.Join(dir, slug+".md")
	content, err := Render(frontmatter, body)
	if err != nil {
		return "", err
	}
	if err := atomicWrite(full, content); err != nil {
		return "", err
	}
	return full, nil
}

// RewriteBody rewrites only the BODY, preserving the frontmatter verbatim (no
// YAML reserialization — zero churn). Returns true if it wrote, false if
// unchanged.
func RewriteBody(path string, transform func(string) string) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	content := string(raw)

	m := frontmatterRE.FindStringSubmatch(content)
	body := content
	if m != nil {
		body = m[2]
	}
	newBody := transform(body)
	if newBody == body {
		return false, nil
	}

	newContent := newBody
	if m != nil {
		newContent = "---\n" + m[1] + "\n---\n" + newBody
	}
	if err := atomicWrite(path, newContent); err != nil {
		return false, err
	}
	return true, nil
}

// Render serializes frontmatter (string keys) + body into the wiki format.
func Render(frontmatter map[string]any, body string) (string, error) {
	out, err := yaml.Marshal(frontmatter)
	if err != nil {
		return "", err
	}
	return "---\n" + string(out) + "---\n" + body, nil
}

func atomicWrite(full, content string) error {
	tmp := fmt.Sprintf("%s.tmp.%d.%d", full, os.Getpid(), rand.Uint32())
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, full)
}

func sha256hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
