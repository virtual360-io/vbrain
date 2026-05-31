package writepages_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/virtual360-io/vbrain/internal/db"
	"github.com/virtual360-io/vbrain/internal/page"
	wp "github.com/virtual360-io/vbrain/internal/writepages"
)

type env struct {
	db       *sql.DB
	dataHome string
	wikiDir  string
	tmpDir   string
}

func setup(t *testing.T) env {
	t.Helper()
	root := t.TempDir()
	wiki := filepath.Join(root, "wiki")
	tmp := filepath.Join(root, "raw", ".tmp")
	os.MkdirAll(wiki, 0o755)
	os.MkdirAll(tmp, 0o755)
	d, err := db.Open(filepath.Join(root, "db", "vbrain.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return env{db: d, dataHome: root, wikiDir: wiki, tmpDir: tmp}
}

// insertRaw cria uma raw_source e devolve (id, rawRel).
func (e env) insertRaw(t *testing.T, name, sha string) (int, string) {
	t.Helper()
	rel := filepath.Join("raw", name)
	abs := filepath.Join(e.dataHome, rel)
	res, err := e.db.Exec(
		"INSERT INTO raw_sources (path, original_filename, source_type, sha256) VALUES (?, ?, 'text', ?)",
		abs, name, sha,
	)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return int(id), rel
}

func (e env) livePage(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(e.wikiDir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (e env) parse(t *testing.T, name string) page.Parsed {
	t.Helper()
	p, err := page.Parse(filepath.Join(e.wikiDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCreatePages(t *testing.T) {
	e := setup(t)
	id, rel := e.insertRaw(t, "doc.txt", "sha1")

	res, err := wp.WritePages(e.db, id, []wp.PageInput{
		{Op: "create", Title: "Foo Bar", BodyMarkdown: "corpo\n", Kind: "concept", Tags: []string{"a"}},
	}, e.wikiDir, e.tmpDir, e.dataHome)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Written, []string{"foo-bar.md"}) || res.Count != 1 {
		t.Fatalf("res = %+v", res)
	}
	p := e.parse(t, "foo-bar.md")
	if p.Frontmatter["title"] != "Foo Bar" || p.Frontmatter["source_raw"] != rel {
		t.Errorf("frontmatter = %+v", p.Frontmatter)
	}
	if p.Body != "corpo\n" {
		t.Errorf("body = %q", p.Body)
	}
}

func TestSlugDedup(t *testing.T) {
	e := setup(t)
	e.livePage(t, "foo.md", "---\ntitle: Foo\nkind: note\n---\nx\n")
	id, _ := e.insertRaw(t, "d.txt", "sha2")

	res, err := wp.WritePages(e.db, id, []wp.PageInput{
		{Op: "create", Title: "Foo", BodyMarkdown: "novo\n", Kind: "note"},
	}, e.wikiDir, e.tmpDir, e.dataHome)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Written, []string{"foo-2.md"}) {
		t.Fatalf("written = %v, want [foo-2.md]", res.Written)
	}
}

func TestUpdateMergesFrontmatter(t *testing.T) {
	e := setup(t)
	e.livePage(t, "foo.md", "---\ntitle: Foo\nkind: concept\ntags:\n  - a\nsource_raw: raw/old.txt\n---\nantigo\n")
	id, rel := e.insertRaw(t, "new.txt", "sha3")

	res, err := wp.WritePages(e.db, id, []wp.PageInput{
		{Op: "update", Slug: "foo", Title: "Ignorado", BodyMarkdown: "novo corpo\n", Kind: "note", Tags: []string{"b"}},
	}, e.wikiDir, e.tmpDir, e.dataHome)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Updated, []string{"foo.md"}) {
		t.Fatalf("updated = %v", res.Updated)
	}
	p := e.parse(t, "foo.md")
	if p.Frontmatter["title"] != "Foo" || p.Frontmatter["kind"] != "concept" {
		t.Errorf("identidade não preservada: %+v", p.Frontmatter)
	}
	tags := toStr(p.Frontmatter["tags"])
	if !reflect.DeepEqual(tags, []string{"a", "b"}) {
		t.Errorf("tags = %v, want [a b]", tags)
	}
	srcs := toStr(p.Frontmatter["source_raw"])
	if !reflect.DeepEqual(srcs, []string{"raw/old.txt", rel}) {
		t.Errorf("source_raw = %v", srcs)
	}
	if p.Body != "novo corpo\n" {
		t.Errorf("body = %q", p.Body)
	}
}

func TestUpdateFallsBackToCreateWhenTargetMissing(t *testing.T) {
	e := setup(t)
	id, _ := e.insertRaw(t, "d.txt", "sha4")

	res, err := wp.WritePages(e.db, id, []wp.PageInput{
		{Op: "update", Slug: "ghost", Title: "Nova Pagina", BodyMarkdown: "x\n", Kind: "note"},
	}, e.wikiDir, e.tmpDir, e.dataHome)
	if err != nil {
		t.Fatal(err)
	}
	// fallback: vira create (Regra 12 — não persiste update fantasma).
	if !reflect.DeepEqual(res.Written, []string{"nova-pagina.md"}) || len(res.Updated) != 0 {
		t.Fatalf("res = %+v", res)
	}
}

func TestDeleteRemovesPage(t *testing.T) {
	e := setup(t)
	e.livePage(t, "bar.md", "---\ntitle: Bar\nkind: note\n---\nsem raw\n") // sem source_raw → não orfana
	id, _ := e.insertRaw(t, "d.txt", "sha5")

	res, err := wp.WritePages(e.db, id, []wp.PageInput{
		{Op: "delete", Slug: "bar"},
	}, e.wikiDir, e.tmpDir, e.dataHome)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(res.Deleted, []string{"bar.md"}) {
		t.Fatalf("deleted = %v", res.Deleted)
	}
	if fileExists(filepath.Join(e.wikiDir, "bar.md")) {
		t.Error("bar.md deveria ter sido removida")
	}
}

func TestOrphanGuardrailAbortsAndLeavesWikiIntact(t *testing.T) {
	e := setup(t)
	// foo.md é a única página citando raw/keep.txt; deletá-la orfanaria o raw.
	e.livePage(t, "foo.md", "---\ntitle: Foo\nkind: note\nsource_raw: raw/keep.txt\n---\nx\n")
	id, _ := e.insertRaw(t, "d.txt", "sha6")

	res, err := wp.WritePages(e.db, id, []wp.PageInput{
		{Op: "delete", Slug: "foo"},
	}, e.wikiDir, e.tmpDir, e.dataHome)
	if err != nil {
		t.Fatal(err)
	}
	if !res.NeedsReview || res.Committed == nil || *res.Committed {
		t.Fatalf("deveria abortar com needs_review: %+v", res)
	}
	if !reflect.DeepEqual(res.OrphanedRaws, []string{"raw/keep.txt"}) {
		t.Errorf("orphaned = %v", res.OrphanedRaws)
	}
	if !fileExists(filepath.Join(e.wikiDir, "foo.md")) {
		t.Error("wiki deveria ficar intacta após abortar")
	}
}

// helpers de teste

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func toStr(v any) []string {
	switch x := v.(type) {
	case string:
		return []string{x}
	case []any:
		out := []string{}
		for _, e := range x {
			out = append(out, e.(string))
		}
		return out
	case []string:
		return x
	default:
		return nil
	}
}
