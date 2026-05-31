package resolvelinks_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/virtual360-io/vbrain/internal/resolvelinks"
)

func TestResolvesKnownSlugsAndDropsUnknown(t *testing.T) {
	wiki := t.TempDir()
	// existing target page
	os.WriteFile(filepath.Join(wiki, "carreira-de-victor.md"), []byte("---\ntitle: Carreira\n---\nx\n"), 0o644)
	// page with an unresolved wikilink
	src := filepath.Join(wiki, "nota.md")
	os.WriteFile(src, []byte("---\ntitle: Nota\n---\ntrabalha na [[V360]] e [[Fantasma]]\n"), 0o644)

	res, err := resolvelinks.ResolveLinks(wiki, map[string]string{
		"V360":     "carreira-de-victor", // existe → aplica
		"Fantasma": "nao-existe",         // slug inexistente → descarta
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied != 1 || res.DroppedUnknownSlug != 1 || res.Changed != 1 {
		t.Fatalf("res = %+v", res)
	}
	b, _ := os.ReadFile(src)
	if !strings.Contains(string(b), "[V360](carreira-de-victor.md)") {
		t.Errorf("link not applied: %s", b)
	}
	if !strings.Contains(string(b), "[[Fantasma]]") {
		t.Errorf("nonexistent wikilink should stay intact: %s", b)
	}
}

func TestDropsNullSlug(t *testing.T) {
	wiki := t.TempDir()
	os.WriteFile(filepath.Join(wiki, "p.md"), []byte("---\ntitle: P\n---\n[[X]]\n"), 0o644)

	res, err := resolvelinks.ResolveLinks(wiki, map[string]string{"X": ""})
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied != 0 || res.DroppedUnknownSlug != 1 || res.Changed != 0 {
		t.Fatalf("res = %+v", res)
	}
}
