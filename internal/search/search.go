// Package search queries the FTS5 index and expands by graph neighbors.
// Deterministic port of scripts/query.rb.
package search

import (
	"database/sql"
	"strings"

	"github.com/virtual360-io/vbrain/internal/ftsquery"
)

// Opts controls the limit, prefix matching, the original NL question, and
// logging.
type Opts struct {
	Limit       int
	Prefix      bool
	SourceQuery string
	Log         bool
}

// Hit is an FTS5 result (with a highlighted snippet).
type Hit struct {
	Path    string `json:"path"`
	Title   string `json:"title"`
	Kind    string `json:"kind"`
	Snippet string `json:"snippet"`
}

// Related is a graph neighbor (out/backlink at 1 hop), without a snippet.
type Related struct {
	Path  string `json:"path"`
	Title string `json:"title"`
	Kind  string `json:"kind"`
}

// Result is the full response of a query.
type Result struct {
	Query      string    `json:"query"`
	Normalized string    `json:"normalized"`
	Results    []Hit     `json:"results"`
	Related    []Related `json:"related"`
}

const ftsSQL = `
SELECT p.id, p.path, p.title, p.kind,
       snippet(pages_fts, 1, '**', '**', '…', 12) AS snip
  FROM pages_fts
  JOIN pages p ON p.id = pages_fts.rowid
 WHERE pages_fts MATCH ?
 ORDER BY rank
 LIMIT ?`

// Query normalizes, searches FTS5, appends graph neighbors, and (optionally)
// records into query_log. Empty normalization → empty result (but logged).
func Query(db *sql.DB, query string, opts Opts) (Result, error) {
	if opts.Limit <= 0 {
		opts.Limit = 10
	}
	normalized := ftsquery.Normalize(query, opts.Prefix)
	res := Result{Query: query, Normalized: normalized, Results: []Hit{}, Related: []Related{}}

	if normalized == "" {
		if err := logQuery(db, query, opts, "", 0); err != nil {
			return res, err
		}
		return res, nil
	}

	rows, err := db.Query(ftsSQL, normalized, opts.Limit)
	if err != nil {
		return res, err
	}
	var hitIDs []int64
	for rows.Next() {
		var id int64
		var h Hit
		if err := rows.Scan(&id, &h.Path, &h.Title, &h.Kind, &h.Snippet); err != nil {
			rows.Close()
			return res, err
		}
		res.Results = append(res.Results, h)
		hitIDs = append(hitIDs, id)
	}
	rows.Close()

	if len(hitIDs) > 0 {
		related, err := neighbors(db, hitIDs, opts.Limit)
		if err != nil {
			return res, err
		}
		res.Related = related
	}

	if err := logQuery(db, query, opts, normalized, len(res.Results)); err != nil {
		return res, err
	}
	return res, nil
}

// neighbors returns the outlinks + backlinks (1 hop) of the hit pages,
// deduplicated and excluding the hits themselves. No reweighting — vbrain stays
// shallow here (Rule 5).
func neighbors(db *sql.DB, hitIDs []int64, limit int) ([]Related, error) {
	ph := strings.TrimSuffix(strings.Repeat("?,", len(hitIDs)), ",")
	q := `
SELECT p.id, p.path, p.title, p.kind
  FROM links l JOIN pages p ON p.id = l.to_page_id
 WHERE l.from_page_id IN (` + ph + `) AND l.to_page_id IS NOT NULL
UNION
SELECT p.id, p.path, p.title, p.kind
  FROM links l JOIN pages p ON p.id = l.from_page_id
 WHERE l.to_page_id IN (` + ph + `)`

	args := make([]any, 0, len(hitIDs)*2)
	for _, id := range hitIDs {
		args = append(args, id)
	}
	for _, id := range hitIDs {
		args = append(args, id)
	}

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hitSet := map[int64]bool{}
	for _, id := range hitIDs {
		hitSet[id] = true
	}
	seen := map[int64]bool{}
	out := []Related{}
	for rows.Next() {
		var id int64
		var r Related
		if err := rows.Scan(&id, &r.Path, &r.Title, &r.Kind); err != nil {
			return nil, err
		}
		if hitSet[id] || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, r)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func logQuery(db *sql.DB, query string, opts Opts, normalized string, count int) error {
	if !opts.Log {
		return nil
	}
	var src any
	if opts.SourceQuery != "" {
		src = opts.SourceQuery
	}
	_, err := db.Exec(
		"INSERT INTO query_log (query, source_query, normalized, results_count) VALUES (?, ?, ?, ?)",
		query, src, normalized, count,
	)
	return err
}
