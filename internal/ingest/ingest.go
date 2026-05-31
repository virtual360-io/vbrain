// Package ingest copies an input (file/URL/tweet) into the immutable raw/,
// records it in raw_sources (dedup by sha256), and extracts its markdown.
// Deterministic port of scripts/ingest_raw.rb.
package ingest

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/virtual360-io/vbrain/internal/sources"
)

var urlRE = regexp.MustCompile(`(?i)^https?://`)

// Result is the output JSON (the shape varies by case).
type Result struct {
	SourceType    string `json:"source_type"`
	RawID         int64  `json:"raw_id,omitempty"`
	RawPath       string `json:"raw_path,omitempty"`
	ExtractedPath string `json:"extracted_path,omitempty"`
	Duplicate     bool   `json:"duplicate,omitempty"`
	// "unknown" case:
	Ext   string `json:"ext,omitempty"`
	Sniff string `json:"sniff,omitempty"`
	Input string `json:"input,omitempty"`
}

// IngestRaw detects the source, copies to raw/, dedups by sha256, and extracts.
func IngestRaw(db *sql.DB, input, typeOverride string, force bool, rawDir, tmpDir string) (Result, error) {
	isURL := urlRE.MatchString(input)
	if !isURL {
		if _, err := os.Stat(input); err != nil {
			return Result{}, errors.New("path not found: " + input)
		}
	}

	var src sources.Source
	if typeOverride != "" {
		src = sources.For(typeOverride)
	} else {
		src = sources.Detect(input)
	}
	if src == nil {
		return Result{SourceType: "unknown", Ext: filepath.Ext(input), Sniff: sniff(input), Input: input}, nil
	}
	ing, ok := src.(sources.Ingestable)
	if !ok {
		return Result{}, fmt.Errorf("source %s is not ingestable", src.KindKey())
	}

	timestamp := time.Now().UTC().Format("20060102T150405Z")
	rawInfo, err := ing.CopyToRaw(input, rawDir, timestamp)
	if err != nil {
		return Result{}, fmt.Errorf("source %s failed to ingest: %w", src.KindKey(), err)
	}

	var existingID int64
	var existingPath string
	err = db.QueryRow("SELECT id, path FROM raw_sources WHERE sha256 = ?", rawInfo.SHA256).
		Scan(&existingID, &existingPath)
	switch {
	case err == nil && !force:
		// duplicate: remove the just-written raw if it differs from the recorded one.
		if rawInfo.Path != existingPath {
			os.Remove(rawInfo.Path)
		}
		return Result{
			RawID: existingID, RawPath: existingPath,
			SourceType: src.KindKey(), Duplicate: true,
		}, nil
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return Result{}, err
	}

	res, err := db.Exec(
		"INSERT INTO raw_sources (path, original_filename, source_type, sha256) VALUES (?, ?, ?, ?)",
		rawInfo.Path, rawInfo.OriginalFilename, src.KindKey(), rawInfo.SHA256,
	)
	if err != nil {
		return Result{}, err
	}
	rawID, _ := res.LastInsertId()

	outPath := filepath.Join(tmpDir, fmt.Sprintf("extracted-%d.txt", rawID))
	if err := ing.Extract(input, outPath, rawInfo); err != nil {
		return Result{}, err
	}

	return Result{
		RawID: rawID, RawPath: rawInfo.Path,
		SourceType: src.KindKey(), ExtractedPath: outPath,
	}, nil
}

func sniff(input string) string {
	fi, err := os.Stat(input)
	if err != nil || fi.IsDir() {
		return "(not a file)"
	}
	f, err := os.Open(input)
	if err != nil {
		return "(not a file)"
	}
	defer f.Close()
	buf := make([]byte, 64)
	n, _ := f.Read(buf)
	return fmt.Sprintf("%q", buf[:n])
}
