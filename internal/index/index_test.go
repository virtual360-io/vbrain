package index_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/virtual360-io/vbrain/internal/db"
	"github.com/virtual360-io/vbrain/internal/index"
)

// setup creates a temporary wiki and a file db (not :memory:, since Reindex
// uses multiple queries — but SetMaxOpenConns(1) guarantees one connection).
func setup(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	wiki := filepath.Join(dir, "wiki")
	if err := os.MkdirAll(wiki, 0o755); err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(filepath.Join(dir, "vbrain.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d, wiki
}

func writePage(t *testing.T, wiki, name, content string) {
	t.Helper()
	p := filepath.Join(wiki, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func countPages(t *testing.T, d *sql.DB) int {
	t.Helper()
	var n int
	if err := d.QueryRow("SELECT COUNT(*) FROM pages").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestReindexInsertsNewPages(t *testing.T) {
	d, wiki := setup(t)
	writePage(t, wiki, "foo.md", "---\ntitle: Foo\nkind: concept\ntags:\n  - a\n  - b\n---\nbody foo\n")
	writePage(t, wiki, "bar.md", "---\ntitle: Bar\nkind: note\n---\nbody bar\n")

	st, err := index.Reindex(d, wiki)
	if err != nil {
		t.Fatal(err)
	}
	if st.Inserted != 2 || st.Updated != 0 || st.Deleted != 0 {
		t.Fatalf("stats = %+v", st)
	}
	if countPages(t, d) != 2 {
		t.Fatalf("pages = %d", countPages(t, d))
	}

	var tags, kind string
	if err := d.QueryRow("SELECT tags, kind FROM pages WHERE path = 'foo.md'").Scan(&tags, &kind); err != nil {
		t.Fatal(err)
	}
	if tags != "a,b" || kind != "concept" {
		t.Errorf("tags=%q kind=%q", tags, kind)
	}
}

func TestReindexIsIdempotentAndUpdatesBySha(t *testing.T) {
	d, wiki := setup(t)
	writePage(t, wiki, "foo.md", "---\ntitle: Foo\nkind: note\n---\nbody v1\n")
	if _, err := index.Reindex(d, wiki); err != nil {
		t.Fatal(err)
	}

	// No change: nothing inserted/updated.
	st, _ := index.Reindex(d, wiki)
	if st.Inserted != 0 || st.Updated != 0 {
		t.Fatalf("expected idempotent reindex, got %+v", st)
	}

	// Change the body: becomes an update.
	writePage(t, wiki, "foo.md", "---\ntitle: Foo\nkind: note\n---\nbody v2 changed\n")
	st, _ = index.Reindex(d, wiki)
	if st.Updated != 1 || st.Inserted != 0 {
		t.Fatalf("expected update by sha, got %+v", st)
	}
}

func TestReindexDeletesMissing(t *testing.T) {
	d, wiki := setup(t)
	writePage(t, wiki, "foo.md", "---\ntitle: Foo\nkind: note\n---\nx\n")
	writePage(t, wiki, "bar.md", "---\ntitle: Bar\nkind: note\n---\ny\n")
	if _, err := index.Reindex(d, wiki); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(wiki, "bar.md")); err != nil {
		t.Fatal(err)
	}
	st, _ := index.Reindex(d, wiki)
	if st.Deleted != 1 {
		t.Fatalf("deleted = %d, want 1", st.Deleted)
	}
	if countPages(t, d) != 1 {
		t.Fatalf("pages = %d, want 1", countPages(t, d))
	}
}

func TestReindexDefaultsKindFromRealtimeDir(t *testing.T) {
	d, wiki := setup(t)
	// Sem kind no frontmatter; sob _realtime → realtime, fora → note.
	writePage(t, wiki, "_realtime/gcal.md", "---\ntitle: GCal\n---\nx\n")
	writePage(t, wiki, "loose.md", "---\ntitle: Loose\n---\ny\n")
	if _, err := index.Reindex(d, wiki); err != nil {
		t.Fatal(err)
	}

	var rk, lk string
	d.QueryRow("SELECT kind FROM pages WHERE path = '_realtime/gcal.md'").Scan(&rk)
	d.QueryRow("SELECT kind FROM pages WHERE path = 'loose.md'").Scan(&lk)
	if rk != "realtime" || lk != "note" {
		t.Fatalf("realtime=%q loose=%q", rk, lk)
	}
}

func TestReindexRebuildsLinkGraph(t *testing.T) {
	d, wiki := setup(t)
	writePage(t, wiki, "foo.md", "---\ntitle: Foo\nkind: note\n---\nliga em [[Bar]] e [[Inexistente]]\n")
	writePage(t, wiki, "bar.md", "---\ntitle: Bar\nkind: note\n---\nsem links\n")
	st, err := index.Reindex(d, wiki)
	if err != nil {
		t.Fatal(err)
	}
	if st.Links != 2 {
		t.Fatalf("links = %d, want 2", st.Links)
	}

	// [[Bar]] resolves (to_page_id != NULL); [[Inexistente]] becomes a forward link.
	var resolved, unresolved int
	d.QueryRow("SELECT COUNT(*) FROM links WHERE to_page_id IS NOT NULL").Scan(&resolved)
	d.QueryRow("SELECT COUNT(*) FROM links WHERE to_page_id IS NULL").Scan(&unresolved)
	if resolved != 1 || unresolved != 1 {
		t.Fatalf("resolved=%d unresolved=%d", resolved, unresolved)
	}
}
