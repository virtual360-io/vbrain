package db

import (
	"database/sql"
	"strings"
	"testing"
)

func openMem(t *testing.T) *sql.DB {
	t.Helper()
	d, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open(:memory:): %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func names(t *testing.T, d *sql.DB, query string) []string {
	t.Helper()
	rows, err := d.Query(query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		out = append(out, n)
	}
	return out
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

func matchCount(t *testing.T, d *sql.DB, term string) int {
	t.Helper()
	var n int
	err := d.QueryRow(
		"SELECT COUNT(*) FROM pages_fts JOIN pages p ON p.id = pages_fts.rowid WHERE pages_fts MATCH ?",
		term,
	).Scan(&n)
	if err != nil {
		t.Fatalf("MATCH %q: %v", term, err)
	}
	return n
}

func TestMigrateCreatesTablesAndTriggers(t *testing.T) {
	d := openMem(t)

	tables := names(t, d, "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	for _, want := range []string{"pages", "raw_sources", "links"} {
		if !contains(tables, want) {
			t.Errorf("tabela %q ausente", want)
		}
	}

	if got := names(t, d, "SELECT name FROM sqlite_master WHERE name='pages_fts'"); len(got) != 1 {
		t.Error("tabela virtual pages_fts deveria existir")
	}

	triggers := names(t, d, "SELECT name FROM sqlite_master WHERE type='trigger' ORDER BY name")
	for _, want := range []string{"pages_ai", "pages_ad", "pages_au"} {
		if !contains(triggers, want) {
			t.Errorf("trigger %q ausente", want)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	d := openMem(t)
	if err := Migrate(d); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(d); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := d.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count < 2 {
		t.Fatalf("count = %d, want >= 2", count)
	}
}

func TestFTSTriggerSyncsOnInsertUpdateDelete(t *testing.T) {
	d := openMem(t)

	res, err := d.Exec(
		"INSERT INTO pages (path, title, body, kind, tags, sha256) VALUES (?, ?, ?, ?, ?, ?)",
		"concepts/foo.md", "Foo Concept", "Discussion of foobar and baz", "concept", "foo,bar", "abc",
	)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()

	if got := matchCount(t, d, "foobar"); got != 1 {
		t.Errorf("após insert: foobar = %d, want 1", got)
	}

	if _, err := d.Exec("UPDATE pages SET body = ? WHERE id = ?", "renamed body about widgets", id); err != nil {
		t.Fatal(err)
	}
	if got := matchCount(t, d, "widgets"); got != 1 {
		t.Errorf("após update: widgets = %d, want 1", got)
	}
	if got := matchCount(t, d, "foobar"); got != 0 {
		t.Errorf("após update: foobar = %d, want 0", got)
	}

	if _, err := d.Exec("DELETE FROM pages WHERE id = ?", id); err != nil {
		t.Fatal(err)
	}
	if got := matchCount(t, d, "widgets"); got != 0 {
		t.Errorf("após delete: widgets = %d, want 0", got)
	}
}

func TestCheckConstraintRejectsUnknownKind(t *testing.T) {
	d := openMem(t)
	_, err := d.Exec(
		"INSERT INTO pages (path, title, body, kind, sha256) VALUES (?, ?, ?, ?, ?)",
		"x/y.md", "T", "B", "garbage", "sha",
	)
	if err == nil {
		t.Fatal("kind inválido deveria violar o CHECK")
	}
}

func TestCheckConstraintAcceptsRealtimeKind(t *testing.T) {
	d := openMem(t)
	_, err := d.Exec(
		"INSERT INTO pages (path, title, body, kind, sha256) VALUES (?, ?, ?, ?, ?)",
		"_realtime/gcalendar.md", "GCal", "body", "realtime", "sha",
	)
	if err != nil {
		t.Fatalf("kind realtime deveria ser aceito: %v", err)
	}
}

func TestLinkCascadesWhenSourcePageDeleted(t *testing.T) {
	d := openMem(t)
	fromID := insertPage(t, d, "a.md", "A", "s1")
	toID := insertPage(t, d, "b.md", "B", "s2")
	if _, err := d.Exec(
		"INSERT INTO links (from_page_id, target_slug, target_title, to_page_id) VALUES (?, 'b', 'B', ?)",
		fromID, toID,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := d.Exec("DELETE FROM pages WHERE id = ?", fromID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := d.QueryRow("SELECT COUNT(*) FROM links").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("edge deve sumir junto com a página de origem (CASCADE): n = %d", n)
	}
}

func TestLinkTargetNulledWhenTargetPageDeleted(t *testing.T) {
	d := openMem(t)
	fromID := insertPage(t, d, "a.md", "A", "s1")
	toID := insertPage(t, d, "b.md", "B", "s2")
	if _, err := d.Exec(
		"INSERT INTO links (from_page_id, target_slug, target_title, to_page_id) VALUES (?, 'b', 'B', ?)",
		fromID, toID,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := d.Exec("DELETE FROM pages WHERE id = ?", toID); err != nil {
		t.Fatal(err)
	}
	var gotFrom int64
	var gotTo sql.NullInt64
	if err := d.QueryRow("SELECT from_page_id, to_page_id FROM links").Scan(&gotFrom, &gotTo); err != nil {
		t.Fatal(err)
	}
	if gotFrom != fromID {
		t.Errorf("aresta deve permanecer: from = %d, want %d", gotFrom, fromID)
	}
	if gotTo.Valid {
		t.Errorf("alvo deletado deve virar NULL (SET NULL), got %v", gotTo)
	}
}

func TestMigrateRebuildsPagesWhenOldCheckConstraintPresent(t *testing.T) {
	d, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	d.SetMaxOpenConns(1)

	if _, err := d.Exec(`
CREATE TABLE pages (
  id          INTEGER PRIMARY KEY,
  path        TEXT NOT NULL UNIQUE,
  title       TEXT NOT NULL,
  body        TEXT NOT NULL,
  kind        TEXT NOT NULL CHECK(kind IN ('concept','decision','gotcha','note','rule')),
  tags        TEXT NOT NULL DEFAULT '',
  sha256      TEXT NOT NULL,
  raw_id      INTEGER,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);`); err != nil {
		t.Fatal(err)
	}

	if err := Migrate(d); err != nil {
		t.Fatal(err)
	}
	var ddl string
	if err := d.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name='pages'").Scan(&ddl); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ddl, "'realtime'") {
		t.Errorf("schema de pages deveria ter sido reconstruído com 'realtime': %s", ddl)
	}
}

func insertPage(t *testing.T, d *sql.DB, path, title, sha string) int64 {
	t.Helper()
	res, err := d.Exec(
		"INSERT INTO pages (path, title, body, kind, sha256) VALUES (?, ?, 'b', 'note', ?)",
		path, title, sha,
	)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}
