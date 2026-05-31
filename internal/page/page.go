// Package page lê e escreve páginas markdown com frontmatter YAML. Porta
// determinística de lib/vbrain/page.rb.
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

// frontmatterRE separa o bloco YAML do corpo. (?s) = `.` casa newline (flag /m
// do Ruby).
var frontmatterRE = regexp.MustCompile(`(?s)\A---\s*\n(.*?)\n---\s*\n(.*)\z`)

// ErrInvalid cobre dir inexistente ou slug vazio no Write.
var ErrInvalid = errors.New("page: dir must exist and slug cannot be empty")

// Parsed é o resultado de parse: frontmatter, corpo e sha256 do corpo.
type Parsed struct {
	Frontmatter map[string]any
	Body        string
	SHA256      string
}

// Parse lê e parseia uma página do disco.
func Parse(path string) (Parsed, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Parsed{}, err
	}
	return ParseString(string(content))
}

// ParseString parseia conteúdo em memória. Sem frontmatter, body = conteúdo
// inteiro e frontmatter vazio.
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

// Write renderiza frontmatter+body e grava atomicamente em dir/<slug>.md.
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

// RewriteBody reescreve só o CORPO, preservando o frontmatter verbatim (sem
// reserializar YAML — zero churn). Retorna true se gravou, false se inalterado.
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

// Render serializa frontmatter (chaves string) + corpo no formato da wiki.
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
