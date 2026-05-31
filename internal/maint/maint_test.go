package maint_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/virtual360-io/vbrain/internal/db"
	"github.com/virtual360-io/vbrain/internal/maint"
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

func insertPage(t *testing.T, d *sql.DB, path, title, tags, kind string) {
	t.Helper()
	if _, err := d.Exec(
		"INSERT INTO pages (path, title, body, kind, tags, sha256) VALUES (?, ?, 'b', ?, ?, ?)",
		path, title, kind, tags, path,
	); err != nil {
		t.Fatal(err)
	}
}

func TestTagsCountsAndRanks(t *testing.T) {
	d := openDB(t)
	insertPage(t, d, "a.md", "A", "familia,carreira", "note")
	insertPage(t, d, "b.md", "B", "carreira", "note")
	insertPage(t, d, "c.md", "C", "", "note")

	res, err := maint.Tags(d, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.TotalDistinct != 2 {
		t.Fatalf("distinct = %d", res.TotalDistinct)
	}
	// carreira (2) antes de familia (1)
	if res.Tags[0].Tag != "carreira" || res.Tags[0].Count != 2 {
		t.Fatalf("top = %+v", res.Tags[0])
	}
	if res.Tags[1].Tag != "familia" || res.Tags[1].Count != 1 {
		t.Fatalf("second = %+v", res.Tags[1])
	}
}

func TestTagsLimit(t *testing.T) {
	d := openDB(t)
	insertPage(t, d, "a.md", "A", "x,y,z", "note")
	res, _ := maint.Tags(d, 2)
	if len(res.Tags) != 2 || res.TotalDistinct != 3 {
		t.Fatalf("res = %+v", res)
	}
}

func TestStats(t *testing.T) {
	d := openDB(t)
	insertPage(t, d, "a.md", "A", "", "note")
	insertPage(t, d, "b.md", "B", "", "concept")
	res, err := maint.Stats(d, "/home/x/vbrain")
	if err != nil {
		t.Fatal(err)
	}
	if res.Pages != 2 || res.ByKind["note"] != 1 || res.ByKind["concept"] != 1 {
		t.Fatalf("res = %+v", res)
	}
	if res.DataHome != "/home/x/vbrain" {
		t.Errorf("data_home = %q", res.DataHome)
	}
}

func TestQueryLogDumpAndPrune(t *testing.T) {
	d := openDB(t)
	for i := 0; i < 3; i++ {
		if _, err := d.Exec("INSERT INTO query_log (query, normalized, results_count) VALUES (?, ?, ?)",
			"q", "\"q\"", i); err != nil {
			t.Fatal(err)
		}
	}
	dump, err := maint.QueryLogDump(d, 0)
	if err != nil {
		t.Fatal(err)
	}
	if dump.Count != 3 || dump.MaxID == nil || *dump.MaxID != 3 {
		t.Fatalf("dump = %+v", dump)
	}

	pr, err := maint.QueryLogPrune(d, 2)
	if err != nil {
		t.Fatal(err)
	}
	if pr.Deleted != 2 || pr.Remaining != 1 {
		t.Fatalf("prune = %+v", pr)
	}
}

func TestLinkify(t *testing.T) {
	wiki := t.TempDir()
	os.WriteFile(filepath.Join(wiki, "alvo.md"), []byte("---\ntitle: Alvo\n---\nx\n"), 0o644)
	src := filepath.Join(wiki, "nota.md")
	os.WriteFile(src, []byte("---\ntitle: Nota\n---\nliga em [[Alvo]] e [[Fantasma]]\n"), 0o644)

	res, err := maint.Linkify(wiki)
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != 2 || res.Changed != 1 {
		t.Fatalf("res = %+v", res)
	}
	b, _ := os.ReadFile(src)
	if !strings.Contains(string(b), "[Alvo](alvo.md)") || !strings.Contains(string(b), "[[Fantasma]]") {
		t.Errorf("linkify incorreto: %s", b)
	}
}
