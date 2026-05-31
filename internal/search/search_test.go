package search_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/virtual360-io/vbrain/internal/db"
	"github.com/virtual360-io/vbrain/internal/search"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "v.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func insert(t *testing.T, d *sql.DB, path, title, body, kind string) int64 {
	t.Helper()
	res, err := d.Exec(
		"INSERT INTO pages (path, title, body, kind, sha256) VALUES (?, ?, ?, ?, ?)",
		path, title, body, kind, path,
	)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestQueryReturnsRankedResultsWithSnippet(t *testing.T) {
	d := openDB(t)
	insert(t, d, "pg.md", "Postgres Logical", "replica identity full for logical replication", "concept")
	insert(t, d, "other.md", "Unrelated", "nothing to see", "note")

	res, err := search.Query(d, "postgres:logical", search.Opts{Limit: 10, Log: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(res.Results))
	}
	if res.Results[0].Path != "pg.md" {
		t.Errorf("path = %q", res.Results[0].Path)
	}
	if !strings.Contains(res.Results[0].Snippet, "**") {
		t.Errorf("snippet sem destaque: %q", res.Results[0].Snippet)
	}
}

func TestQueryEmptyNormalizedReturnsEmptyButLogs(t *testing.T) {
	d := openDB(t)
	res, err := search.Query(d, ":::", search.Opts{Log: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Normalized != "" || len(res.Results) != 0 {
		t.Fatalf("res = %+v", res)
	}
	var n int
	d.QueryRow("SELECT COUNT(*) FROM query_log").Scan(&n)
	if n != 1 {
		t.Fatalf("query_log = %d, want 1 (queries vazias são o sinal mais valioso)", n)
	}
}

func TestQueryRelatedViaGraph(t *testing.T) {
	d := openDB(t)
	a := insert(t, d, "a.md", "Alpha postgres", "about postgres", "concept")
	b := insert(t, d, "b.md", "Beta", "neighbor page", "note")
	if _, err := d.Exec(
		"INSERT INTO links (from_page_id, target_slug, target_title, to_page_id) VALUES (?, 'b', 'Beta', ?)",
		a, b,
	); err != nil {
		t.Fatal(err)
	}

	res, err := search.Query(d, "postgres", search.Opts{Limit: 10, Log: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Related) != 1 || res.Related[0].Path != "b.md" {
		t.Fatalf("related = %+v", res.Related)
	}
}

func TestQueryLogsSourceQuery(t *testing.T) {
	d := openDB(t)
	insert(t, d, "c.md", "Carreira", "consultor visagio", "note")
	if _, err := search.Query(d, "carreira", search.Opts{Log: true, SourceQuery: "quais empregos eu tive"}); err != nil {
		t.Fatal(err)
	}
	var src sql.NullString
	if err := d.QueryRow("SELECT source_query FROM query_log ORDER BY id DESC LIMIT 1").Scan(&src); err != nil {
		t.Fatal(err)
	}
	if !src.Valid || src.String != "quais empregos eu tive" {
		t.Fatalf("source_query = %v", src)
	}
}
