package sources

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Text é a fonte para arquivos locais de texto (markdown ou texto puro).
type Text struct{}

var textExtensions = map[string]bool{".md": true, ".markdown": true, ".txt": true, ".text": true}

const sampleBytes = 4096

func (Text) KindKey() string { return "text" }

// Detect aceita arquivos com extensão de texto conhecida ou, sem extensão
// reconhecida, arquivos cujo início é UTF-8 válido sem NUL.
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

// Extract copia o conteúdo do arquivo para out_path (passthrough).
func (Text) Extract(path, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	content, err := os.ReadFile(path)
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
