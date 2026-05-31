package scaffold_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/virtual360-io/vbrain/internal/scaffold"
)

func TestWritesClaudeMDInstructingToUseSkills(t *testing.T) {
	dir := t.TempDir()
	ok, err := scaffold.WriteClaudeMD(dir)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	for _, want := range []string{"ALWAYS use the vbrain skills", "/vbrain-query-knowledge", "/vbrain-add-knowledge"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("CLAUDE.md missing %q", want)
		}
	}
	// Go-oriented: mentions the vbrain binary, not Ruby/bundle.
	if !strings.Contains(string(body), "`vbrain`") || strings.Contains(string(body), "bundle install") {
		t.Error("CLAUDE.md should be Go-oriented")
	}
}

func TestDoesNotClobberExistingClaudeMD(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# custom\n"), 0o644)
	ok, err := scaffold.WriteClaudeMD(dir)
	if err != nil || ok {
		t.Fatalf("should not overwrite: ok=%v", ok)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if string(body) != "# custom\n" {
		t.Errorf("clobbered: %q", body)
	}
}

func TestInstallsSkillsIntoClaudeSkills(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	os.MkdirAll(filepath.Join(src, "vbrain-foo"), 0o755)
	os.WriteFile(filepath.Join(src, "vbrain-foo", "SKILL.md"), []byte("x"), 0o644)

	n, err := scaffold.InstallSkills(dir, os.DirFS(src))
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "vbrain-foo", "SKILL.md")); err != nil {
		t.Errorf("skill not installed: %v", err)
	}
}

func TestInstallSkillsIdempotentDoesNotNest(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	os.MkdirAll(filepath.Join(src, "vbrain-foo"), 0o755)
	os.WriteFile(filepath.Join(src, "vbrain-foo", "SKILL.md"), []byte("x"), 0o644)

	scaffold.InstallSkills(dir, os.DirFS(src))
	n, err := scaffold.InstallSkills(dir, os.DirFS(src))
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "vbrain-foo", "vbrain-foo")); !os.IsNotExist(err) {
		t.Error("should not nest vbrain-foo/vbrain-foo")
	}
}
