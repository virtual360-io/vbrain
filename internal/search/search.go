// Package search queries the FTS5 index and expands by graph neighbors.
// Deterministic port of scripts/query.rb.
package search

import (
	"database/sql"
	"strings"

	"github.com/virtual360-io/vbrain/internal/ftsquery"
)

// Opts controls the limit, prefix matching, the original NL question, the soul
// ranking, and logging.
type Opts struct {
	Limit       int
	Prefix      bool
	SourceQuery string
	Log         bool

	// SoulBoost multiplies the bm25 score of kind=soul hits so the identity
	// layer surfaces above plain knowledge ("acting > knowing"). >1 ranks soul
	// higher; <=0 falls back to DefaultSoulBoost. This is the adjustable boost.
	SoulBoost float64
	// SoulAuthoritative orders soul hits strictly before everything else,
	// regardless of bm25 — for decision/belief questions, where what the user
	// stands for has absolute precedence over what the user merely knows.
	SoulAuthoritative bool
}

// DefaultSoulBoost is the mild, always-on favoring of the soul layer when no
// explicit SoulBoost is given.
const DefaultSoulBoost = 2.0

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

// ftsSQL ranks by bm25 (lower = more relevant), with the soul layer favored:
// the boost multiplier deepens soul scores so they sort first, and the
// authoritative flag pins soul hits ahead of everything regardless of score.
const ftsSQL = `
SELECT p.id, p.path, p.title, p.kind,
       snippet(pages_fts, 1, '**', '**', '…', 12) AS snip
  FROM pages_fts
  JOIN pages p ON p.id = pages_fts.rowid
 WHERE pages_fts MATCH ?
 ORDER BY
   CASE WHEN ? AND p.kind = 'soul' THEN 0 ELSE 1 END,
   bm25(pages_fts) * (CASE WHEN p.kind = 'soul' THEN ? ELSE 1.0 END)
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

	boost := opts.SoulBoost
	if boost <= 0 {
		boost = DefaultSoulBoost
	}
	authoritative := 0
	if opts.SoulAuthoritative {
		authoritative = 1
	}

	rows, err := db.Query(ftsSQL, normalized, authoritative, boost, opts.Limit)
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
