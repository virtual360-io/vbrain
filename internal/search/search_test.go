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
		t.Fatalf("query_log = %d, want 1 (empty queries are the most valuable signal)", n)
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

// The soul layer encodes "acting > knowing": when an identity page and a
// knowledge page are equally relevant, the soul page must surface first by
// default, so an agent deciding in the user's name sees who they are before
// what they read.
func TestQueryDefaultBoostFavorsSoul(t *testing.T) {
	d := openDB(t)
	insert(t, d, "_soul/freedom.md", "Freedom", "liberty matters to me", "soul")
	insert(t, d, "friedman.md", "Friedman", "liberty matters in the book", "concept")

	res, err := search.Query(d, "liberty", search.Opts{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(res.Results))
	}
	if res.Results[0].Kind != "soul" {
		t.Errorf("default boost should rank soul first, got %q (%s)", res.Results[0].Kind, res.Results[0].Path)
	}
}

// For decision/belief questions the soul layer has ABSOLUTE precedence: even a
// far more relevant knowledge page must rank below an identity page, because
// what the user believes outranks what the user merely knows.
func TestQuerySoulAuthoritativePinsSoulFirst(t *testing.T) {
	d := openDB(t)
	insert(t, d, "_soul/values.md", "Values", "freedom", "soul")
	// A knowledge page that matches the term far more strongly.
	insert(t, d, "marx.md", "Marx", "freedom freedom freedom freedom freedom", "concept")

	res, err := search.Query(d, "freedom", search.Opts{Limit: 10, SoulAuthoritative: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(res.Results))
	}
	if res.Results[0].Kind != "soul" {
		t.Errorf("authoritative mode must pin soul first regardless of bm25, got %q", res.Results[0].Kind)
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
