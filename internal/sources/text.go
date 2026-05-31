package sources

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Text is the source for local text files (markdown or plain text).
type Text struct{}

var textExtensions = map[string]bool{".md": true, ".markdown": true, ".txt": true, ".text": true}

const sampleBytes = 4096

func (Text) KindKey() string { return "text" }

// Detect accepts files with a known text extension or, without a recognized
// extension, files whose start is valid UTF-8 without a NUL.
func (Text) Detect(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	if textExtensions[strings.ToLower(filepath.Ext(path))] {
		return true
	}
	return utf8Text(path)
}

// CopyToRaw copies the local file into raw/ with a timestamp prefix (the default
// for file-based sources).
func (Text) CopyToRaw(input, rawDir, timestamp string) (RawInfo, error) {
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

// Extract copies the file content to out_path (passthrough).
func (Text) Extract(input, outPath string, _ RawInfo) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	content, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, content, 0o644)
}

func utf8Text(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	buf := make([]byte, sampleBytes)
	n, _ := f.Read(buf)
	sample := buf[:n]
	if len(sample) == 0 {
		return true
	}
	if bytes.IndexByte(sample, 0) >= 0 {
		return false
	}
	return utf8.Valid(sample)
}
