// Package index rebuilds the SQLite index from the on-disk wiki (the source of
// truth). Deterministic port of scripts/reindex.rb.
package index

import (
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/virtual360-io/vbrain/internal/links"
	"github.com/virtual360-io/vbrain/internal/page"
	"github.com/virtual360-io/vbrain/internal/paths"
)

// Stats summarizes the result of a reindex.
type Stats struct {
	Inserted int `json:"inserted"`
	Updated  int `json:"updated"`
	Deleted  int `json:"deleted"`
	Links    int `json:"links"`
}

var kindSet = func() map[string]bool {
	m := map[string]bool{}
	for _, k := range paths.Kinds {
		m[k] = true
	}
	return m
}()

// Reindex syncs the index with the .md files in wikiDir: inserts new ones,
// updates by sha256, removes missing ones, and rebuilds the link graph.
// Idempotent.
func Reindex(db *sql.DB, wikiDir string) (Stats, error) {
	var st Stats

	mdFiles, err := collectMarkdown(wikiDir)
	if err != nil {
		return st, err
	}

	onDisk := map[string]bool{}
	for _, abs := range mdFiles {
		rel := strings.TrimPrefix(abs, wikiDir+string(filepath.Separator))
		rel = filepath.ToSlash(rel)

		parsed, err := page.Parse(abs)
		if err != nil {
			return st, err
		}
		fm := parsed.Frontmatter
		title := asString(fm["title"])
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(rel), ".md")
		}
		kind := asString(fm["kind"])
		if !kindSet[kind] {
			// Flat layout: trust the frontmatter; the reserved subdirs are
			// realtime/soul by construction, everything else defaults to note.
			switch strings.SplitN(rel, "/", 2)[0] {
			case paths.RealtimeDir:
				kind = "realtime"
			case paths.SoulDir:
				kind = "soul"
			default:
				kind = "note"
			}
		}
		tags := joinTags(fm["tags"])

		onDisk[rel] = true
		var id int64
		var prevSha string
		row := db.QueryRow("SELECT id, sha256 FROM pages WHERE path = ?", rel)
		switch err := row.Scan(&id, &prevSha); err {
		case sql.ErrNoRows:
			if _, err := db.Exec(
				"INSERT INTO pages (path, title, body, kind, tags, sha256) VALUES (?, ?, ?, ?, ?, ?)",
				rel, title, parsed.Body, kind, tags, parsed.SHA256,
			); err != nil {
				return st, err
			}
			st.Inserted++
		case nil:
			if prevSha != parsed.SHA256 {
				if _, err := db.Exec(
					"UPDATE pages SET title = ?, body = ?, kind = ?, tags = ?, sha256 = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ?",
					title, parsed.Body, kind, tags, parsed.SHA256, id,
				); err != nil {
					return st, err
				}
				st.Updated++
			}
		default:
			return st, err
		}
	}

	deleted, err := deleteMissing(db, onDisk)
	if err != nil {
		return st, err
	}
	st.Deleted = deleted

	st.Links, err = rebuildLinks(db)
	if err != nil {
		return st, err
	}
	return st, nil
}

func collectMarkdown(wikiDir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(wikiDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".md") {
			out = append(out, p)
		}
		return nil
	})
	sort.Strings(out) // deterministic
	return out, err
}

func deleteMissing(db *sql.DB, onDisk map[string]bool) (int, error) {
	rows, err := db.Query("SELECT id, path FROM pages")
	if err != nil {
		return 0, err
	}
	var toDelete []int64
	for rows.Next() {
		var id int64
		var path string
		if err := rows.Scan(&id, &path); err != nil {
			rows.Close()
			return 0, err
		}
		if !onDisk[path] {
			toDelete = append(toDelete, id)
		}
	}
	rows.Close()
	for _, id := range toDelete {
		if _, err := db.Exec("DELETE FROM pages WHERE id = ?", id); err != nil {
			return 0, err
		}
	}
	return len(toDelete), nil
}

// rebuildLinks recreates the graph from scratch: extracts links from each body
// and resolves the target slug against existing pages (NULL = unresolved
// forward link).
func rebuildLinks(db *sql.DB) (int, error) {
	type pg struct {
		id   int64
		path string
		body string
	}
	var pages []pg
	slugToID := map[string]int64{}

	rows, err := db.Query("SELECT id, path, body FROM pages")
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var p pg
		if err := rows.Scan(&p.id, &p.path, &p.body); err != nil {
			rows.Close()
			return 0, err
		}
		pages = append(pages, p)
		slugToID[strings.TrimSuffix(filepath.Base(p.path), ".md")] = p.id
	}
	rows.Close()

	if _, err := db.Exec("DELETE FROM links"); err != nil {
		return 0, err
	}
	count := 0
	for _, p := range pages {
		for _, l := range links.Extract(p.body) {
			var to any
			if id, ok := slugToID[l.Slug]; ok {
				to = id
			}
			if _, err := db.Exec(
				"INSERT INTO links (from_page_id, target_slug, target_title, to_page_id) VALUES (?, ?, ?, ?)",
				p.id, l.Slug, l.Title, to,
			); err != nil {
				return 0, err
			}
			count++
		}
	}
	return count, nil
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// joinTags reproduces Array(fm["tags"]).join(",").
func joinTags(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []any:
		parts := make([]string, 0, len(x))
		for _, e := range x {
			parts = append(parts, fmt.Sprintf("%v", e))
		}
		return strings.Join(parts, ",")
	case []string:
		return strings.Join(x, ",")
	default:
		return ""
	}
}
