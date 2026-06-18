package sources

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// GoogleTranscript is the source for Google Meet transcripts. A Meet transcript
// is shared as a Google Doc (named "transcript-<uuid>") whose body is WebVTT; the
// add-knowledge skill downloads it to a local .vtt and ingests it here.
//
// It MUST be registered before Text in the Registry: a .vtt is valid UTF-8, so
// Text's catch-all Detect would otherwise claim it.
type GoogleTranscript struct{}

func (GoogleTranscript) KindKey() string { return "google-transcript" }

// Detect accepts a .vtt file or any file whose first non-empty bytes are the
// WEBVTT signature (the Doc may be saved without the .vtt extension).
func (GoogleTranscript) Detect(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	if strings.ToLower(filepath.Ext(path)) == ".vtt" {
		return true
	}
	return hasWebVTTHeader(path)
}

func hasWebVTTHeader(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 64)
	n, _ := f.Read(buf)
	s := strings.TrimLeft(string(buf[:n]), "\uFEFF \t\r\n")
	return strings.HasPrefix(s, "WEBVTT")
}

// CopyToRaw copies the raw .vtt into raw/ with a timestamp prefix; SHA256 of the
// content gives dedup. Mirrors Text.CopyToRaw — the raw stays immutable; the
// WebVTT→markdown transform happens in Extract, never here.
func (GoogleTranscript) CopyToRaw(input, rawDir, timestamp string) (RawInfo, error) {
	basename := filepath.Base(input)
	dest := filepath.Join(rawDir, timestamp+"-"+basename)
	data, err := os.ReadFile(input)
	if err != nil {
		return RawInfo{}, err
	}
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		return RawInfo{}, err
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return RawInfo{}, err
	}
	sum := sha256.Sum256(data)
	return RawInfo{Path: dest, OriginalFilename: basename, SHA256: hex.EncodeToString(sum[:])}, nil
}

// Extract turns the WebVTT into a clean transcript markdown (one consolidated
// block per speaker turn) for the chunker. Deterministic (Rule 5).
func (g GoogleTranscript) Extract(input, outPath string, _ RawInfo) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, []byte(g.ParseVTT(string(data))), 0o644)
}

var (
	// Anchor on the timestamp line and capture the start; HH:MM:SS.mmm is the
	// shape Google Meet emits (accepts , as the SRT-style ms separator too).
	vttTimestampRE = regexp.MustCompile(`(\d{2}:\d{2}:\d{2}[.,]\d{3})\s*-->\s*\d{2}:\d{2}:\d{2}[.,]\d{3}`)
	vttCueNumberRE = regexp.MustCompile(`^\d+$`)
	// <v Speaker> / <v.loud Speaker> voice spans.
	vttVoiceTagRE = regexp.MustCompile(`<v(?:\.[^ >]+)*\s+([^>]+)>`)
	vttTagRE      = regexp.MustCompile(`<[^>]*>`)
	// "Name: text" — the name excludes sentence punctuation (.!?) to avoid
	// mistaking a spoken clause ("then I did the following: ...") for a speaker.
	vttSpeakerRE = regexp.MustCompile(`^([^:.!?]{1,60}):\s+(.*)$`)
	// Undo markdown escaping the Drive MCP applies (notably "--\>" for the cue
	// arrow): a backslash before markdown punctuation is dropped.
	mdUnescapeRE = regexp.MustCompile(`\\([\\*_{}\[\]()#+.!><~|/` + "`" + `-])`)
)

type vttTurn struct{ start, speaker, text string }

// ParseVTT converts WebVTT — as delivered by Google Meet, possibly markdown-
// escaped/spaced by the Drive MCP — into transcript markdown. It anchors on
// timestamp lines (a cue's payload is the lines between its timestamp and the
// next, minus blanks, cue numbers and NOTE lines), so it survives both the
// canonical layout and the markdown-rendered one (blank lines between every
// element). Consecutive cues from the same speaker collapse into one turn.
func (GoogleTranscript) ParseVTT(raw string) string {
	raw = strings.TrimPrefix(raw, "\uFEFF")
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	raw = mdUnescapeRE.ReplaceAllString(raw, "$1")
	lines := strings.Split(raw, "\n")

	var tsIdx []int
	for i, ln := range lines {
		if vttTimestampRE.MatchString(strings.TrimSpace(ln)) {
			tsIdx = append(tsIdx, i)
		}
	}

	var turns []vttTurn
	for k, i := range tsIdx {
		start := normalizeTimestamp(vttTimestampRE.FindStringSubmatch(strings.TrimSpace(lines[i]))[1])
		end := len(lines)
		if k+1 < len(tsIdx) {
			end = tsIdx[k+1]
		}
		var payload []string
		for j := i + 1; j < end; j++ {
			t := strings.TrimSpace(lines[j])
			if t == "" || vttCueNumberRE.MatchString(t) || strings.HasPrefix(t, "NOTE") {
				continue
			}
			payload = append(payload, t)
		}
		speaker, text := cleanCue(strings.Join(payload, " "))
		if strings.TrimSpace(text) == "" {
			continue
		}
		if n := len(turns); n > 0 && turns[n-1].speaker == speaker {
			turns[n-1].text += " " + text
		} else {
			turns = append(turns, vttTurn{start: start, speaker: speaker, text: text})
		}
	}

	var b strings.Builder
	b.WriteString("# Transcript\n\n")
	for _, t := range turns {
		sp := t.speaker
		if sp == "" {
			sp = "(unknown)"
		}
		b.WriteString("**[" + t.start + "] " + sp + ":** " + strings.TrimSpace(t.text) + "\n\n")
	}
	return b.String()
}

// cleanCue extracts the speaker and the spoken text from a cue payload.
func cleanCue(payload string) (speaker, text string) {
	if m := vttVoiceTagRE.FindStringSubmatch(payload); m != nil {
		speaker = strings.TrimSpace(m[1])
	}
	s := strings.TrimSpace(unescapeVTTEntities(vttTagRE.ReplaceAllString(payload, "")))
	if speaker != "" {
		return speaker, s
	}
	if m := vttSpeakerRE.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
	}
	return "", s
}

func unescapeVTTEntities(s string) string {
	return strings.NewReplacer(
		"&amp;", "&", "&lt;", "<", "&gt;", ">",
		"&lrm;", "", "&rlm;", "", "&nbsp;", " ",
	).Replace(s)
}

func normalizeTimestamp(ts string) string { return strings.Replace(ts, ",", ".", 1) }
