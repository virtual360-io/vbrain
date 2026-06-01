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
			t.Errorf("table %q missing", want)
		}
	}

	if got := names(t, d, "SELECT name FROM sqlite_master WHERE name='pages_fts'"); len(got) != 1 {
		t.Error("virtual table pages_fts should exist")
	}

	triggers := names(t, d, "SELECT name FROM sqlite_master WHERE type='trigger' ORDER BY name")
	for _, want := range []string{"pages_ai", "pages_ad", "pages_au"} {
		if !contains(triggers, want) {
			t.Errorf("trigger %q missing", want)
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
		t.Errorf("after insert: foobar = %d, want 1", got)
	}

	if _, err := d.Exec("UPDATE pages SET body = ? WHERE id = ?", "renamed body about widgets", id); err != nil {
		t.Fatal(err)
	}
	if got := matchCount(t, d, "widgets"); got != 1 {
		t.Errorf("after update: widgets = %d, want 1", got)
	}
	if got := matchCount(t, d, "foobar"); got != 0 {
		t.Errorf("after update: foobar = %d, want 0", got)
	}

	if _, err := d.Exec("DELETE FROM pages WHERE id = ?", id); err != nil {
		t.Fatal(err)
	}
	if got := matchCount(t, d, "widgets"); got != 0 {
		t.Errorf("after delete: widgets = %d, want 0", got)
	}
}

func TestCheckConstraintRejectsUnknownKind(t *testing.T) {
	d := openMem(t)
	_, err := d.Exec(
		"INSERT INTO pages (path, title, body, kind, sha256) VALUES (?, ?, ?, ?, ?)",
		"x/y.md", "T", "B", "garbage", "sha",
	)
	if err == nil {
		t.Fatal("invalid kind should violate the CHECK")
	}
}

func TestCheckConstraintAcceptsRealtimeKind(t *testing.T) {
	d := openMem(t)
	_, err := d.Exec(
		"INSERT INTO pages (path, title, body, kind, sha256) VALUES (?, ?, ?, ?, ?)",
		"_realtime/gcalendar.md", "GCal", "body", "realtime", "sha",
	)
	if err != nil {
		t.Fatalf("realtime kind should be accepted: %v", err)
	}
}

func TestCheckConstraintAcceptsSoulKind(t *testing.T) {
	d := openMem(t)
	_, err := d.Exec(
		"INSERT INTO pages (path, title, body, kind, sha256) VALUES (?, ?, ?, ?, ?)",
		"_soul/freedom.md", "Freedom", "body", "soul", "sha",
	)
	if err != nil {
		t.Fatalf("soul kind should be accepted: %v", err)
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
		t.Errorf("edge should disappear with the source page (CASCADE): n = %d", n)
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
		t.Errorf("edge should remain: from = %d, want %d", gotFrom, fromID)
	}
	if gotTo.Valid {
		t.Errorf("deleted target should become NULL (SET NULL), got %v", gotTo)
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
	if !strings.Contains(ddl, "'soul'") {
		t.Errorf("pages schema should have been rebuilt with the newest kind 'soul': %s", ddl)
	}
}

// A base from the intermediate era — it already knew 'realtime' but predates
// 'soul' — must also be rebuilt, so soul-kind inserts stop failing the CHECK.
func TestMigrateRebuildsPagesWhenCheckMissingSoul(t *testing.T) {
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
  kind        TEXT NOT NULL CHECK(kind IN ('concept','decision','gotcha','note','rule','realtime')),
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
	if _, err := d.Exec(
		"INSERT INTO pages (path, title, body, kind, sha256) VALUES (?, ?, ?, ?, ?)",
		"_soul/values.md", "Values", "body", "soul", "sha",
	); err != nil {
		t.Fatalf("after migration a soul page must insert cleanly: %v", err)
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
