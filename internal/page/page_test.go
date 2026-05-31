package page

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRewriteBodyPreservesFrontmatterVerbatim(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.md")
	mustWriteFile(t, path, "---\ntitle: Foo\nkind: note\ntags:\n  - a\n---\nlink [[Alvo]] aqui\n")

	changed, err := RewriteBody(path, func(b string) string {
		return strings.ReplaceAll(b, "[[Alvo]]", "[Alvo](alvo.md)")
	})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("deveria ter reescrito")
	}
	content := readFile(t, path)
	if !strings.Contains(content, "---\ntitle: Foo\nkind: note\ntags:\n  - a\n---\n") {
		t.Errorf("frontmatter não preservado verbatim:\n%s", content)
	}
	if !strings.Contains(content, "link [Alvo](alvo.md) aqui") {
		t.Errorf("corpo não reescrito:\n%s", content)
	}
}

func TestRewriteBodyReturnsFalseWhenUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.md")
	mustWriteFile(t, path, "---\ntitle: Foo\n---\nsem mudança\n")

	changed, err := RewriteBody(path, func(b string) string { return b })
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("não deveria reescrever corpo inalterado")
	}
}

func TestParseStringWithFrontmatter(t *testing.T) {
	content := "---\ntitle: Foo\ntags:\n  - a\n  - b\n---\n# Foo\n\nBody here.\n"
	parsed, err := ParseString(content)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Frontmatter["title"] != "Foo" {
		t.Errorf("title = %v", parsed.Frontmatter["title"])
	}
	tags, ok := parsed.Frontmatter["tags"].([]any)
	if !ok || len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
		t.Errorf("tags = %v", parsed.Frontmatter["tags"])
	}
	if !strings.Contains(parsed.Body, "Body here.") {
		t.Errorf("body = %q", parsed.Body)
	}
	if parsed.SHA256 == "" {
		t.Error("sha256 vazio")
	}
}

func TestParseStringWithoutFrontmatter(t *testing.T) {
	content := "# Plain markdown\n\nNo frontmatter."
	parsed, err := ParseString(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Frontmatter) != 0 {
		t.Errorf("frontmatter deveria estar vazio: %v", parsed.Frontmatter)
	}
	if parsed.Body != content {
		t.Errorf("body = %q", parsed.Body)
	}
}

func TestWriteCreatesFileAtomically(t *testing.T) {
	dir := t.TempDir()
	path, err := Write(dir, "foo-page",
		map[string]any{"title": "Foo Page", "kind": "note", "tags": []string{"a", "b"}},
		"# Foo Page\n\nHello.\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("arquivo não existe: %v", err)
	}
	if want := filepath.Join(dir, "foo-page.md"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dir, "*.tmp.*")); len(leftovers) != 0 {
		t.Errorf("sobraram tmp files: %v", leftovers)
	}
}

func TestWriteThenParseRoundtripKeepsSHA256Stable(t *testing.T) {
	dir := t.TempDir()
	body := "## Conteúdo\n\nLinha 1\nLinha 2\n"
	fm := map[string]any{"title": "Round Trip", "kind": "concept", "tags": []string{"x"}}
	path, err := Write(dir, "rt", fm, body)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Frontmatter["title"] != "Round Trip" {
		t.Errorf("title = %v", parsed.Frontmatter["title"])
	}
	if parsed.Body != body {
		t.Errorf("body roundtrip falhou:\n%q\n!=\n%q", parsed.Body, body)
	}
	if parsed.SHA256 != sha256hex(body) {
		t.Errorf("sha256 instável")
	}
}

func TestWriteRaisesOnEmptySlug(t *testing.T) {
	dir := t.TempDir()
	if _, err := Write(dir, "", map[string]any{}, "x"); err == nil {
		t.Fatal("slug vazio deveria falhar")
	}
}

func TestRenderUsesStringKeys(t *testing.T) {
	rendered, err := Render(map[string]any{"title": "Sym", "tags": []string{"a"}}, "body")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "title:") || !strings.Contains(rendered, "Sym") {
		t.Errorf("render = %q", rendered)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
