// Package maint reúne as operações determinísticas de manutenção/insight da
// base: vocabulário de tags, stats, fila do query_log e linkify. Porta de
// scripts/{tags,stats,query_log,linkify}.rb.
package maint

import (
	"database/sql"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/virtual360-io/vbrain/internal/links"
	"github.com/virtual360-io/vbrain/internal/page"
)

// --- tags ---

type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

type TagsResult struct {
	TotalDistinct int        `json:"total_distinct"`
	Tags          []TagCount `json:"tags"`
}

// Tags conta o vocabulário de tags (pages.tags é CSV) ordenado por contagem
// desc, depois alfabético. limit<=0 = todas.
func Tags(db *sql.DB, limit int) (TagsResult, error) {
	rows, err := db.Query("SELECT tags FROM pages")
	if err != nil {
		return TagsResult{}, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var tags string
		if err := rows.Scan(&tags); err != nil {
			return TagsResult{}, err
		}
		for _, t := range strings.Split(tags, ",") {
			if t = strings.TrimSpace(t); t != "" {
				counts[t]++
			}
		}
	}

	ranked := make([]TagCount, 0, len(counts))
	for t, n := range counts {
		ranked = append(ranked, TagCount{Tag: t, Count: n})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Count != ranked[j].Count {
			return ranked[i].Count > ranked[j].Count
		}
		return ranked[i].Tag < ranked[j].Tag
	})
	if limit > 0 && len(ranked) > limit {
		ranked = ranked[:limit]
	}
	return TagsResult{TotalDistinct: len(counts), Tags: ranked}, nil
}

// --- stats ---

type StatsResult struct {
	DataHome string            `json:"data_home"`
	Pages    int               `json:"pages"`
	Raw      int               `json:"raw"`
	ByKind   map[string]int    `json:"by_kind"`
	Recent   []map[string]string `json:"recent"`
}

func Stats(db *sql.DB, dataHome string) (StatsResult, error) {
	res := StatsResult{DataHome: dataHome, ByKind: map[string]int{}, Recent: []map[string]string{}}
	if err := db.QueryRow("SELECT COUNT(*) FROM pages").Scan(&res.Pages); err != nil {
		return res, err
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM raw_sources").Scan(&res.Raw); err != nil {
		return res, err
	}
	kindRows, err := db.Query("SELECT kind, COUNT(*) FROM pages GROUP BY kind ORDER BY kind")
	if err != nil {
		return res, err
	}
	for kindRows.Next() {
		var k string
		var n int
		if err := kindRows.Scan(&k, &n); err != nil {
			kindRows.Close()
			return res, err
		}
		res.ByKind[k] = n
	}
	kindRows.Close()

	recentRows, err := db.Query("SELECT path, title FROM pages ORDER BY updated_at DESC LIMIT 5")
	if err != nil {
		return res, err
	}
	defer recentRows.Close()
	for recentRows.Next() {
		var p, t string
		if err := recentRows.Scan(&p, &t); err != nil {
			return res, err
		}
		res.Recent = append(res.Recent, map[string]string{"path": p, "title": t})
	}
	return res, nil
}

// --- query_log ---

type QueryLogEntry struct {
	ID           int64   `json:"id"`
	Query        string  `json:"query"`
	SourceQuery  *string `json:"source_query"`
	Normalized   string  `json:"normalized"`
	ResultsCount int     `json:"results_count"`
	CreatedAt    string  `json:"created_at"`
}

type QueryLogDumpResult struct {
	Count   int             `json:"count"`
	MaxID   *int64          `json:"max_id"`
	Entries []QueryLogEntry `json:"entries"`
}

// QueryLogDump lista as entradas (mais antigas primeiro). limit<=0 = todas.
func QueryLogDump(db *sql.DB, limit int) (QueryLogDumpResult, error) {
	q := "SELECT id, query, source_query, normalized, results_count, created_at FROM query_log ORDER BY id ASC"
	var args []any
	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := db.Query(q, args...)
	if err != nil {
		return QueryLogDumpResult{}, err
	}
	defer rows.Close()
	res := QueryLogDumpResult{Entries: []QueryLogEntry{}}
	for rows.Next() {
		var e QueryLogEntry
		var src sql.NullString
		if err := rows.Scan(&e.ID, &e.Query, &src, &e.Normalized, &e.ResultsCount, &e.CreatedAt); err != nil {
			return res, err
		}
		if src.Valid {
			e.SourceQuery = &src.String
		}
		res.Entries = append(res.Entries, e)
	}
	res.Count = len(res.Entries)
	if res.Count > 0 {
		id := res.Entries[res.Count-1].ID
		res.MaxID = &id
	}
	return res, nil
}

type QueryLogPruneResult struct {
	Deleted   int   `json:"deleted"`
	Remaining int   `json:"remaining"`
	ThroughID int64 `json:"through_id"`
}

// QueryLogPrune apaga entradas com id <= throughID (seguro contra corrida).
func QueryLogPrune(db *sql.DB, throughID int64) (QueryLogPruneResult, error) {
	var before, after int
	if err := db.QueryRow("SELECT COUNT(*) FROM query_log").Scan(&before); err != nil {
		return QueryLogPruneResult{}, err
	}
	if _, err := db.Exec("DELETE FROM query_log WHERE id <= ?", throughID); err != nil {
		return QueryLogPruneResult{}, err
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM query_log").Scan(&after); err != nil {
		return QueryLogPruneResult{}, err
	}
	return QueryLogPruneResult{Deleted: before - after, Remaining: after, ThroughID: throughID}, nil
}

// --- linkify ---

type LinkifyResult struct {
	Changed int `json:"changed"`
	Scanned int `json:"scanned"`
}

// Linkify converte [[wikilinks]] resolvíveis por slug exato em links markdown.
// Idempotente; preserva frontmatter verbatim.
func Linkify(wikiDir string) (LinkifyResult, error) {
	var files []string
	err := filepath.WalkDir(wikiDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".md") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return LinkifyResult{}, err
	}

	var slugs []string
	for _, f := range files {
		slugs = append(slugs, strings.TrimSuffix(filepath.Base(f), ".md"))
	}

	changed := 0
	for _, f := range files {
		did, err := page.RewriteBody(f, func(body string) string {
			return links.Linkify(body, slugs)
		})
		if err != nil {
			return LinkifyResult{}, err
		}
		if did {
			changed++
		}
	}
	return LinkifyResult{Changed: changed, Scanned: len(files)}, nil
}
