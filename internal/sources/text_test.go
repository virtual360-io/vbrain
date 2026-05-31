package sources_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/virtual360-io/vbrain/internal/sources"
)

func writeTmp(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTextDetectMdFile(t *testing.T) {
	if !(sources.Text{}).Detect(writeTmp(t, "foo.md", []byte("# hi\n"))) {
		t.Fatal("deveria detectar .md")
	}
}

func TestTextDetectTxtFile(t *testing.T) {
	if !(sources.Text{}).Detect(writeTmp(t, "foo.txt", []byte("hello\n"))) {
		t.Fatal("deveria detectar .txt")
	}
}

func TestTextDetectExtensionlessUtf8(t *testing.T) {
	if !(sources.Text{}).Detect(writeTmp(t, "notes", []byte("some text\n"))) {
		t.Fatal("deveria detectar texto utf8 sem extensão")
	}
}

func TestTextDetectRejectsBinary(t *testing.T) {
	if (sources.Text{}).Detect(writeTmp(t, "blob.bin", []byte("PK\x03\x04binarystuff\x00\xff\xfe"))) {
		t.Fatal("não deveria detectar binário")
	}
}

func TestTextDetectRejectsDirectory(t *testing.T) {
	if (sources.Text{}).Detect(t.TempDir()) {
		t.Fatal("não deveria detectar diretório")
	}
}

func TestTextExtractWritesPassthrough(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.md")
	out := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(src, []byte("Olá Mundo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (sources.Text{}).Extract(src, out); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(out); string(b) != "Olá Mundo\n" {
		t.Fatalf("got %q", b)
	}
}

func TestTextKindKey(t *testing.T) {
	if (sources.Text{}).KindKey() != "text" {
		t.Fatal("kind_key deveria ser text")
	}
}

func TestDispatcherDetectReturnsTextForMd(t *testing.T) {
	s := sources.Detect(writeTmp(t, "x.md", []byte("# x")))
	if s == nil || s.KindKey() != "text" {
		t.Fatalf("got %v", s)
	}
}

func TestDispatcherDetectReturnsNilForBinary(t *testing.T) {
	if s := sources.Detect(writeTmp(t, "b.bin", []byte("\x00\xff\x00\xff"))); s != nil {
		t.Fatalf("got %v, want nil", s)
	}
}

func TestDispatcherForLookupByKind(t *testing.T) {
	if s := sources.For("text"); s == nil || s.KindKey() != "text" {
		t.Fatalf("For(text) = %v", s)
	}
	if s := sources.For("nonexistent"); s != nil {
		t.Fatalf("For(nonexistent) = %v, want nil", s)
	}
}

func TestDispatcherKindsIncludesText(t *testing.T) {
	found := false
	for _, k := range sources.Kinds() {
		if k == "text" {
			found = true
		}
	}
	if !found {
		t.Fatal("Kinds() deveria incluir text")
	}
}
