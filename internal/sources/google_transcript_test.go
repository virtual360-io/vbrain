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

// turns counts the rendered speaker blocks in ParseVTT output.
func turns(out string) int { return strings.Count(out, "**[") }

func TestGoogleTranscriptKindKey(t *testing.T) {
	if (sources.GoogleTranscript{}).KindKey() != "google-transcript" {
		t.Fatal("kind_key should be google-transcript")
	}
}

func TestGoogleTranscriptDetectByExtension(t *testing.T) {
	if !(sources.GoogleTranscript{}).Detect(writeTmp(t, "meeting.vtt", []byte("WEBVTT\n\n"))) {
		t.Fatal("should detect .vtt")
	}
}

func TestGoogleTranscriptDetectByHeaderExtensionless(t *testing.T) {
	if !(sources.GoogleTranscript{}).Detect(writeTmp(t, "transcript", []byte("WEBVTT\n\n1\n00:00:01.000 --> 00:00:02.000\nHi\n"))) {
		t.Fatal("should detect by WEBVTT header even without .vtt")
	}
}

func TestGoogleTranscriptDetectRejectsPlainText(t *testing.T) {
	if (sources.GoogleTranscript{}).Detect(writeTmp(t, "notes.txt", []byte("just some notes\n"))) {
		t.Fatal("should not detect plain text without the WEBVTT header")
	}
}

func TestGoogleTranscriptDetectRejectsDirectory(t *testing.T) {
	if (sources.GoogleTranscript{}).Detect(t.TempDir()) {
		t.Fatal("should not detect a directory")
	}
}

// Precedence: a .vtt is valid UTF-8, so without GoogleTranscript ahead of Text in
// the Registry, Text's catch-all would claim it. This test fails if someone moves
// GoogleTranscript after Text.
func TestDispatcherPrefersGoogleTranscriptOverTextForVtt(t *testing.T) {
	s := sources.Detect(writeTmp(t, "x.vtt", []byte("WEBVTT\n\n1\n00:00:01.000 --> 00:00:02.000\nHi\n")))
	if s == nil || s.KindKey() != "google-transcript" {
		t.Fatalf("got %v, want google-transcript", s)
	}
}

func TestDispatcherPrefersGoogleTranscriptByHeaderOverText(t *testing.T) {
	// Extensionless file: Text would accept it as UTF-8; the WEBVTT header must win.
	s := sources.Detect(writeTmp(t, "meetdump", []byte("WEBVTT\n\n1\n00:00:01.000 --> 00:00:02.000\nHi\n")))
	if s == nil || s.KindKey() != "google-transcript" {
		t.Fatalf("got %v, want google-transcript", s)
	}
}

func TestDispatcherForLookupGoogleTranscript(t *testing.T) {
	if s := sources.For("google-transcript"); s == nil || s.KindKey() != "google-transcript" {
		t.Fatalf("For(google-transcript) = %v", s)
	}
}

func TestDispatcherKindsIncludesGoogleTranscript(t *testing.T) {
	found := false
	for _, k := range sources.Kinds() {
		if k == "google-transcript" {
			found = true
		}
	}
	if !found {
		t.Fatal("Kinds() should include google-transcript")
	}
}

func TestParseVTTBasicTwoSpeakers(t *testing.T) {
	in := "WEBVTT\n\n1\n00:00:08.587 --> 00:00:33.167\nMaisa: Olá a todos.\n\n2\n00:01:46.867 --> 00:01:49.867\nLaura: Bom dia.\n"
	out := (sources.GoogleTranscript{}).ParseVTT(in)
	if turns(out) != 2 {
		t.Fatalf("want 2 turns, got %d in:\n%s", turns(out), out)
	}
	for _, want := range []string{"[00:00:08.587] Maisa:", "Olá a todos.", "[00:01:46.867] Laura:", "Bom dia."} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	// Cue numbers and the arrow never leak into the rendered transcript.
	if strings.Contains(out, "-->") || strings.Contains(out, "\n1\n") {
		t.Fatalf("raw vtt markers leaked:\n%s", out)
	}
}

func TestParseVTTConsolidatesConsecutiveSameSpeaker(t *testing.T) {
	in := "WEBVTT\n\n1\n00:00:01.000 --> 00:00:02.000\nMaisa: Primeiro.\n\n2\n00:00:02.000 --> 00:00:03.000\nMaisa: Segundo.\n\n3\n00:00:03.000 --> 00:00:04.000\nMaisa: Terceiro.\n"
	out := (sources.GoogleTranscript{}).ParseVTT(in)
	if turns(out) != 1 {
		t.Fatalf("want 1 consolidated turn, got %d:\n%s", turns(out), out)
	}
	if !strings.Contains(out, "Primeiro. Segundo. Terceiro.") {
		t.Fatalf("texts not joined:\n%s", out)
	}
	if !strings.Contains(out, "[00:00:01.000]") {
		t.Fatalf("turn should keep the first cue's timestamp:\n%s", out)
	}
}

func TestParseVTTInterruptionAlternating(t *testing.T) {
	in := "WEBVTT\n\n1\n00:00:01.000 --> 00:00:02.000\nA: um\n\n2\n00:00:02.000 --> 00:00:03.000\nB: dois\n\n3\n00:00:03.000 --> 00:00:04.000\nA: tres\n\n4\n00:00:04.000 --> 00:00:05.000\nB: quatro\n\n5\n00:00:05.000 --> 00:00:06.000\nA: cinco\n"
	out := (sources.GoogleTranscript{}).ParseVTT(in)
	if turns(out) != 5 {
		t.Fatalf("alternating speakers must not consolidate; want 5 turns, got %d:\n%s", turns(out), out)
	}
}

func TestParseVTTEmptySpeaker(t *testing.T) {
	in := "WEBVTT\n\n1\n00:00:01.000 --> 00:00:02.000\nfala sem nome aqui\n"
	out := (sources.GoogleTranscript{}).ParseVTT(in)
	if !strings.Contains(out, "(unknown):** fala sem nome aqui") {
		t.Fatalf("speakerless cue should render (unknown):\n%s", out)
	}
}

func TestParseVTTNameWithPipe(t *testing.T) {
	in := "WEBVTT\n\n1\n00:00:01.000 --> 00:00:02.000\nLaura Rocha | V360: Vem\n"
	out := (sources.GoogleTranscript{}).ParseVTT(in)
	if !strings.Contains(out, "Laura Rocha | V360:** Vem") {
		t.Fatalf("pipe in display name should be preserved as speaker:\n%s", out)
	}
}

func TestParseVTTColonInSpeechNotSpeaker(t *testing.T) {
	in := "WEBVTT\n\n1\n00:00:01.000 --> 00:00:02.000\nMaisa: vou sair às 10:30 hoje\n"
	out := (sources.GoogleTranscript{}).ParseVTT(in)
	if !strings.Contains(out, "Maisa:** vou sair às 10:30 hoje") {
		t.Fatalf("only the first colon names the speaker; clock time must stay in the text:\n%s", out)
	}
}

// The real Drive MCP output: blank lines between every element and a markdown-
// escaped arrow "--\>". The timestamp anchor + unescape must handle it.
func TestParseVTTMarkdownRenderedWithEscapedArrow(t *testing.T) {
	in := "WEBVTT\n\n  \n\n1\n\n00:00:08.587 --\\> 00:00:33.167\n\nMaisa: Bom dia & bem-vindos.\n\n  \n\n2\n\n00:01:46.867 --\\> 00:01:49.867\n\nLaura: Obrigada.\n"
	out := (sources.GoogleTranscript{}).ParseVTT(in)
	if turns(out) != 2 {
		t.Fatalf("want 2 turns from markdown-rendered vtt, got %d:\n%s", turns(out), out)
	}
	if !strings.Contains(out, "[00:00:08.587] Maisa:") || !strings.Contains(out, "[00:01:46.867] Laura:") {
		t.Fatalf("timestamps not recovered from escaped arrows:\n%s", out)
	}
}

func TestParseVTTUnescapesEntities(t *testing.T) {
	in := "WEBVTT\n\n1\n00:00:01.000 --> 00:00:02.000\nA: tu &amp; eu &lt;3\n"
	out := (sources.GoogleTranscript{}).ParseVTT(in)
	if !strings.Contains(out, "tu & eu <3") {
		t.Fatalf("entities not unescaped:\n%s", out)
	}
}

func TestParseVTTVoiceTagSpeaker(t *testing.T) {
	in := "WEBVTT\n\n1\n00:00:01.000 --> 00:00:02.000\n<v Maria>oi pessoal</v>\n"
	out := (sources.GoogleTranscript{}).ParseVTT(in)
	if !strings.Contains(out, "Maria:** oi pessoal") {
		t.Fatalf("voice tag speaker/text not extracted:\n%s", out)
	}
	if strings.Contains(out, "<v") || strings.Contains(out, "</v>") {
		t.Fatalf("voice tags leaked:\n%s", out)
	}
}

func TestParseVTTCRLFAndSrtComma(t *testing.T) {
	in := "WEBVTT\r\n\r\n1\r\n00:00:01,000 --> 00:00:02,000\r\nA: oi\r\n"
	out := (sources.GoogleTranscript{}).ParseVTT(in)
	if !strings.Contains(out, "[00:00:01.000] A:** oi") {
		t.Fatalf("CRLF + comma-ms not normalized:\n%s", out)
	}
}

func TestParseVTTDropsEmptyCues(t *testing.T) {
	in := "WEBVTT\n\n1\n00:00:01.000 --> 00:00:02.000\n\n2\n00:00:02.000 --> 00:00:03.000\nA: real\n"
	out := (sources.GoogleTranscript{}).ParseVTT(in)
	if turns(out) != 1 {
		t.Fatalf("empty-text cue should be dropped; want 1 turn, got %d:\n%s", turns(out), out)
	}
}

func TestGoogleTranscriptExtractWritesCleanMarkdown(t *testing.T) {
	src := filepath.Join("testdata", "google_transcript", "meet_sample.vtt")
	out := filepath.Join(t.TempDir(), "extracted.txt")
	if err := (sources.GoogleTranscript{}).Extract(src, out, sources.RawInfo{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	// 1+2 consolidate (Maisa), then Laura, Maisa, Laura → 4 turns.
	if turns(got) != 4 {
		t.Fatalf("want 4 turns from the fixture, got %d:\n%s", turns(got), got)
	}
	if !strings.HasPrefix(got, "# Transcript\n\n") {
		t.Fatalf("missing transcript header:\n%s", got)
	}
	if !strings.Contains(got, "sobre o projeto. Hoje temos três pontos") {
		t.Fatalf("consecutive Maisa cues not consolidated:\n%s", got)
	}
}

func TestGoogleTranscriptCopyToRawSha256(t *testing.T) {
	content := []byte("WEBVTT\n\n1\n00:00:01.000 --> 00:00:02.000\nA: oi\n")
	src := writeTmp(t, "meeting.vtt", content)
	rawDir := t.TempDir()
	info, err := (sources.GoogleTranscript{}).CopyToRaw(src, rawDir, "20260618-120000")
	if err != nil {
		t.Fatal(err)
	}
	if info.OriginalFilename != "meeting.vtt" {
		t.Fatalf("OriginalFilename = %q", info.OriginalFilename)
	}
	sum := sha256.Sum256(content)
	if info.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("SHA256 should hash the raw content")
	}
	if _, err := os.Stat(info.Path); err != nil {
		t.Fatalf("raw copy not written: %v", err)
	}
}
