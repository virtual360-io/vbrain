package ftsquery_test

import (
	"testing"

	"github.com/virtual360-io/vbrain/internal/db"
	"github.com/virtual360-io/vbrain/internal/ftsquery"
)

func TestNormalizesSimpleQuery(t *testing.T) {
	if got := ftsquery.Normalize("foo bar", false); got != `"foo" OR "bar"` {
		t.Fatalf("got %q", got)
	}
}

func TestNeutralizesColon(t *testing.T) {
	if got := ftsquery.Normalize("postgres:logical", false); got != `"postgres" OR "logical"` {
		t.Fatalf("got %q", got)
	}
}

func TestNeutralizesQuotesAndParens(t *testing.T) {
	if got := ftsquery.Normalize(`"foo" (bar) baz`, false); got != `"foo" OR "bar" OR "baz"` {
		t.Fatalf("got %q", got)
	}
}

func TestEmptyInputReturnsEmpty(t *testing.T) {
	for _, in := range []string{"", "   ", ":::"} {
		if got := ftsquery.Normalize(in, false); got != "" {
			t.Errorf("Normalize(%q) = %q, want \"\"", in, got)
		}
	}
}

func TestPrefixModeAppendsStar(t *testing.T) {
	if got := ftsquery.Normalize("foo bar", true); got != `"foo"* OR "bar"*` {
		t.Fatalf("got %q", got)
	}
}

func TestSingleTokenNoOr(t *testing.T) {
	if got := ftsquery.Normalize("foo", false); got != `"foo"` {
		t.Fatalf("got %q", got)
	}
}

func TestDropsStopwordsKeepingContentTerms(t *testing.T) {
	// stopwords (quais/eu/já/tive) inflated the OR and drowned the signal in
	// BM25 — root cause of the "doesn't find jobs" bug. (PT query kept on
	// purpose: this test verifies Portuguese stopword handling.)
	if got := ftsquery.Normalize("quais empregos eu já tive", false); got != `"empregos"` {
		t.Fatalf("got %q", got)
	}
}

func TestDropsStopwordsCaseInsensitiveAndUnaccented(t *testing.T) {
	if got := ftsquery.Normalize("Quais foram Minhas carreira", false); got != `"carreira"` {
		t.Errorf("got %q", got)
	}
	if got := ftsquery.Normalize("o que eu ja tive de carreira", false); got != `"carreira"` {
		t.Errorf("got %q", got)
	}
}

func TestKeepsMultipleContentTerms(t *testing.T) {
	got := ftsquery.Normalize("qual foi meu cargo na visagio como consultor", false)
	if got != `"cargo" OR "visagio" OR "consultor"` {
		t.Fatalf("got %q", got)
	}
}

func TestFallsBackToOriginalWhenAllStopwords(t *testing.T) {
	// Only stopwords: better to search with them than return empty (zero hits).
	if got := ftsquery.Normalize("quem é você", false); got == "" {
		t.Fatal("should not return empty when there are only stopwords")
	}
}

// Integration tests: the normalized query must run on real FTS5 without a
// syntax error (trailing dash, colon, etc.).

func TestHandlesTrailingDashWithoutFTSError(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.Exec(
		"INSERT INTO pages (path, title, body, kind, sha256) VALUES (?, ?, ?, ?, ?)",
		"a.md", "T", "body has marker-12345 inside", "concept", "x",
	); err != nil {
		t.Fatal(err)
	}

	normalized := ftsquery.Normalize("marker-", false)
	if normalized == "" {
		t.Fatal("normalized empty")
	}
	if _, err := d.Query("SELECT title FROM pages_fts WHERE pages_fts MATCH ?", normalized); err != nil {
		t.Fatalf("MATCH %q failed: %v", normalized, err)
	}
}

func TestActuallyQueriesFTSWithoutError(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.Exec(
		"INSERT INTO pages (path, title, body, kind, sha256) VALUES (?, ?, ?, ?, ?)",
		"a.md", "Postgres Logical", "replica identity full", "concept", "x",
	); err != nil {
		t.Fatal(err)
	}

	normalized := ftsquery.Normalize("postgres:logical", false)
	if normalized == "" {
		t.Fatal("normalized empty")
	}
	var n int
	if err := d.QueryRow(
		"SELECT COUNT(*) FROM pages_fts WHERE pages_fts MATCH ?", normalized,
	).Scan(&n); err != nil {
		t.Fatalf("MATCH %q failed: %v", normalized, err)
	}
	if n != 1 {
		t.Fatalf("hits = %d, want 1", n)
	}
}
