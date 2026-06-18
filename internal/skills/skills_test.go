package skills_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/virtual360-io/vbrain/internal/skills"
)

func catalogFS() fstest.MapFS {
	return fstest.MapFS{
		"vbrain-foo/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: vbrain-foo\ndescription: Foo does foo\n---\nbody\n")},
		"vbrain-bar/SKILL.md": &fstest.MapFile{Data: []byte("---\nname: vbrain-bar\ndescription: Bar\n---\n")},
	}
}

func installedFile(parent, name, rel string) string {
	return filepath.Join(parent, ".claude", "skills", name, rel)
}

func TestCatalogMarksInstalledAndReadsDescription(t *testing.T) {
	base, global := t.TempDir(), t.TempDir()
	opt := catalogFS()
	if err := skills.Install(opt, "vbrain-foo", base, global); err != nil {
		t.Fatal(err)
	}

	cat, err := skills.Catalog(opt, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat) != 2 {
		t.Fatalf("want 2 catalog entries, got %d", len(cat))
	}
	byName := map[string]skills.Entry{}
	for _, e := range cat {
		byName[e.Name] = e
	}
	if !byName["vbrain-foo"].Installed {
		t.Error("vbrain-foo should be marked installed")
	}
	if byName["vbrain-bar"].Installed {
		t.Error("vbrain-bar should NOT be installed")
	}
	if byName["vbrain-foo"].Description != "Foo does foo" {
		t.Errorf("description = %q", byName["vbrain-foo"].Description)
	}
}

func TestInstallCopiesToBaseAndGlobal(t *testing.T) {
	base, global := t.TempDir(), t.TempDir()
	if err := skills.Install(catalogFS(), "vbrain-foo", base, global); err != nil {
		t.Fatal(err)
	}
	for _, parent := range []string{base, global} {
		if _, err := os.Stat(installedFile(parent, "vbrain-foo", "SKILL.md")); err != nil {
			t.Errorf("vbrain-foo not installed under %s: %v", parent, err)
		}
	}
}

func TestInstallUnknownNameErrors(t *testing.T) {
	base, global := t.TempDir(), t.TempDir()
	if err := skills.Install(catalogFS(), "ghost", base, global); err == nil {
		t.Fatal("installing an unknown optional skill should error")
	}
}

func TestRemoveDeletesBothAndReturnsWasInstalled(t *testing.T) {
	base, global := t.TempDir(), t.TempDir()
	opt := catalogFS()
	if err := skills.Install(opt, "vbrain-foo", base, global); err != nil {
		t.Fatal(err)
	}
	was, err := skills.Remove(opt, "vbrain-foo", base, global)
	if err != nil || !was {
		t.Fatalf("was=%v err=%v, want was=true", was, err)
	}
	for _, parent := range []string{base, global} {
		if _, err := os.Stat(filepath.Join(parent, ".claude", "skills", "vbrain-foo")); !os.IsNotExist(err) {
			t.Errorf("vbrain-foo should be gone under %s", parent)
		}
	}
	// Removing again is a no-op: not installed anymore.
	was, err = skills.Remove(opt, "vbrain-foo", base, global)
	if err != nil || was {
		t.Fatalf("second remove: was=%v err=%v, want was=false", was, err)
	}
}

// Remove must refuse any name outside the optional catalog — this is what stops
// `vbrain skill remove <a-core-skill>` from deleting a core skill.
func TestRemoveRejectsNonCatalogName(t *testing.T) {
	base, global := t.TempDir(), t.TempDir()
	// Simulate a core skill present in the base.
	core := filepath.Join(base, ".claude", "skills", "vbrain-add-knowledge")
	os.MkdirAll(core, 0o755)
	os.WriteFile(filepath.Join(core, "SKILL.md"), []byte("core"), 0o644)

	if _, err := skills.Remove(catalogFS(), "vbrain-add-knowledge", base, global); err == nil {
		t.Fatal("removing a non-catalog (core) skill must error")
	}
	if _, err := os.Stat(filepath.Join(core, "SKILL.md")); err != nil {
		t.Errorf("the core skill must remain untouched: %v", err)
	}
}

func TestSyncReinstallsInstalledFromEmbed(t *testing.T) {
	base, global := t.TempDir(), t.TempDir()
	v1 := catalogFS()
	if err := skills.Install(v1, "vbrain-foo", base, global); err != nil {
		t.Fatal(err)
	}

	// New embed: vbrain-foo's SKILL.md changed (a release update).
	v2 := catalogFS()
	v2["vbrain-foo/SKILL.md"] = &fstest.MapFile{Data: []byte("---\nname: vbrain-foo\ndescription: Foo v2\n---\nupdated body\n")}

	res, err := skills.Sync(v2, base, global)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Synced) != 1 || res.Synced[0] != "vbrain-foo" {
		t.Fatalf("synced = %v, want [vbrain-foo]", res.Synced)
	}
	for _, parent := range []string{base, global} {
		b, _ := os.ReadFile(installedFile(parent, "vbrain-foo", "SKILL.md"))
		if !strings.Contains(string(b), "updated body") {
			t.Errorf("under %s the skill was not refreshed from the new embed: %q", parent, b)
		}
	}
}

func TestSuggestedRoutineReadsYAMLOrNil(t *testing.T) {
	opt := fstest.MapFS{
		"vbrain-foo/SKILL.md":    &fstest.MapFile{Data: []byte("---\nname: vbrain-foo\n---\n")},
		"vbrain-foo/routine.yml": &fstest.MapFile{Data: []byte("slug: foo-daily\ndescription: Foo daily\nprompt: |\n  do foo\n")},
		"vbrain-bar/SKILL.md":    &fstest.MapFile{Data: []byte("---\nname: vbrain-bar\n---\n")},
	}
	sug, err := skills.SuggestedRoutine(opt, "vbrain-foo")
	if err != nil {
		t.Fatal(err)
	}
	if sug == nil || sug.Slug != "foo-daily" || strings.TrimSpace(sug.Prompt) == "" {
		t.Fatalf("sug = %+v", sug)
	}
	// No routine.yml → nil, no error.
	sug, err = skills.SuggestedRoutine(opt, "vbrain-bar")
	if err != nil || sug != nil {
		t.Fatalf("want nil suggestion, got %+v err=%v", sug, err)
	}
}

func TestSyncSkipsNotInstalled(t *testing.T) {
	base, global := t.TempDir(), t.TempDir()
	res, err := skills.Sync(catalogFS(), base, global)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Synced) != 0 {
		t.Fatalf("nothing installed → nothing synced, got %v", res.Synced)
	}
}
