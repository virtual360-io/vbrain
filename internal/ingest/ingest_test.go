package ingest_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/virtual360-io/vbrain/internal/db"
	"github.com/virtual360-io/vbrain/internal/ingest"
	"github.com/virtual360-io/vbrain/internal/sources"
)

func setup(t *testing.T) (*sql.DB, string, string) {
	t.Helper()
	root := t.TempDir()
	raw := filepath.Join(root, "raw")
	tmp := filepath.Join(root, "raw", ".tmp")
	os.MkdirAll(raw, 0o755)
	os.MkdirAll(tmp, 0o755)
	d, err := db.Open(filepath.Join(root, "db", "v.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d, raw, tmp
}

func TestIngestTextFile(t *testing.T) {
	d, raw, tmp := setup(t)
	src := filepath.Join(t.TempDir(), "nota.md")
	os.WriteFile(src, []byte("# Nota\n\nconteúdo\n"), 0o644)

	res, err := ingest.IngestRaw(d, src, "", false, raw, tmp)
	if err != nil {
		t.Fatal(err)
	}
	if res.SourceType != "text" || res.RawID == 0 {
		t.Fatalf("res = %+v", res)
	}
	if b, _ := os.ReadFile(res.ExtractedPath); string(b) != "# Nota\n\nconteúdo\n" {
		t.Errorf("extracted = %q", b)
	}
	if _, err := os.Stat(res.RawPath); err != nil {
		t.Errorf("raw doesn't exist: %v", err)
	}
}

func TestIngestDeduplicatesBySha(t *testing.T) {
	d, raw, tmp := setup(t)
	src := filepath.Join(t.TempDir(), "nota.md")
	os.WriteFile(src, []byte("mesmo conteúdo\n"), 0o644)

	first, err := ingest.IngestRaw(d, src, "", false, raw, tmp)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ingest.IngestRaw(d, src, "", false, raw, tmp)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Duplicate || second.RawID != first.RawID {
		t.Fatalf("second ingest should be a duplicate of the first: %+v / %+v", first, second)
	}
}

func TestIngestURLWithStubbedFetch(t *testing.T) {
	d, raw, tmp := setup(t)
	orig := sources.FetchJina
	sources.FetchJina = func(string) (string, error) { return "# Artigo\n", nil }
	t.Cleanup(func() { sources.FetchJina = orig })

	res, err := ingest.IngestRaw(d, "https://example.com/post", "", false, raw, tmp)
	if err != nil {
		t.Fatal(err)
	}
	if res.SourceType != "url" || res.RawID == 0 {
		t.Fatalf("res = %+v", res)
	}
	if b, _ := os.ReadFile(res.ExtractedPath); string(b) != "# Artigo\n" {
		t.Errorf("extracted = %q", b)
	}
}

func TestIngestUnknownSource(t *testing.T) {
	d, raw, tmp := setup(t)
	bin := filepath.Join(t.TempDir(), "blob.bin")
	os.WriteFile(bin, []byte("\x00\xff\x00\xff"), 0o644)

	res, err := ingest.IngestRaw(d, bin, "", false, raw, tmp)
	if err != nil {
		t.Fatal(err)
	}
	if res.SourceType != "unknown" || res.RawID != 0 {
		t.Fatalf("res = %+v", res)
	}
}

func TestIngestTweetWithStubbedSyndication(t *testing.T) {
	d, raw, tmp := setup(t)
	fixture, err := os.ReadFile(filepath.Join("..", "sources", "testdata", "twitter", "alok_link_tweet.json"))
	if err != nil {
		t.Fatal(err)
	}
	orig := sources.FetchSyndication
	sources.FetchSyndication = func(string) (string, error) { return string(fixture), nil }
	t.Cleanup(func() { sources.FetchSyndication = orig })

	res, err := ingest.IngestRaw(d, "https://x.com/alokbishoyi97/status/2059610305408462898", "", false, raw, tmp)
	if err != nil {
		t.Fatal(err)
	}
	if res.SourceType != "tweet" || res.RawID == 0 {
		t.Fatalf("res = %+v", res)
	}
	b, _ := os.ReadFile(res.ExtractedPath)
	if len(b) == 0 {
		t.Error("extracted markdown empty")
	}
}
