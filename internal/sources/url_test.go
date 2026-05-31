package sources_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/virtual360-io/vbrain/internal/sources"
)

func stubFetchJina(t *testing.T, md string) {
	t.Helper()
	orig := sources.FetchJina
	sources.FetchJina = func(string) (string, error) { return md, nil }
	t.Cleanup(func() { sources.FetchJina = orig })
}

func TestURLDetectHttpAndHttps(t *testing.T) {
	for _, u := range []string{"https://example.com/x", "http://example.com", "HTTPS://EXAMPLE.com"} {
		if !(sources.URL{}).Detect(u) {
			t.Errorf("should detect %q", u)
		}
	}
}

func TestURLDetectRejectsNonUrl(t *testing.T) {
	for _, u := range []string{"/tmp/foo.txt", "ftp://example.com", "example.com", ""} {
		if (sources.URL{}).Detect(u) {
			t.Errorf("should not detect %q", u)
		}
	}
}

func TestURLKindKey(t *testing.T) {
	if (sources.URL{}).KindKey() != "url" {
		t.Fatal("kind_key should be url")
	}
}

func TestURLCopyToRawWritesMarkdownWithUrlSha(t *testing.T) {
	sample := "# Title\n\nSome article content.\n"
	stubFetchJina(t, sample)
	rawDir := t.TempDir()

	info, err := (sources.URL{}).CopyToRaw("https://example.com/start", rawDir, "20260528T000000Z")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(info.Path); err != nil {
		t.Errorf("raw markdown should exist: %v", err)
	}
	if !strings.HasSuffix(info.OriginalFilename, ".md") {
		t.Errorf("original_filename = %q", info.OriginalFilename)
	}
	if b, _ := os.ReadFile(info.Path); string(b) != sample {
		t.Errorf("content = %q", b)
	}
	sum := sha256.Sum256([]byte("https://example.com/start\n" + sample))
	if info.SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("sha256 = %q", info.SHA256)
	}
	if info.Markdown != sample {
		t.Errorf("markdown = %q", info.Markdown)
	}
}

func TestURLExtractUsesCachedMarkdown(t *testing.T) {
	out := filepath.Join(t.TempDir(), "extract.txt")
	if err := (sources.URL{}).Extract("https://example.com", out, sources.RawInfo{Markdown: "# Cached\n"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(out); string(b) != "# Cached\n" {
		t.Fatalf("got %q", b)
	}
}

func TestURLExtractFetchesWhenNoCache(t *testing.T) {
	stubFetchJina(t, "# Fresh\n")
	out := filepath.Join(t.TempDir(), "extract.txt")
	if err := (sources.URL{}).Extract("https://example.com", out, sources.RawInfo{}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(out); string(b) != "# Fresh\n" {
		t.Fatalf("got %q", b)
	}
}

func TestDispatcherPrefersUrlOverTextForUrlString(t *testing.T) {
	s := sources.Detect("https://example.com")
	if s == nil || s.KindKey() != "url" {
		t.Fatalf("got %v", s)
	}
}
