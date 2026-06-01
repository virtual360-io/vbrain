// Package paths resolves vbrain's data directories from VBRAIN_HOME (or
// ~/vbrain). Deterministic port of lib/vbrain/paths.rb.
package paths

import (
	"os"
	"path/filepath"
	"strings"
)

// RealtimeDir is a special wiki subdir: phantom pages with an MCP handler,
// written by another skill, not by the ingest pipeline.
const RealtimeDir = "_realtime"

// SoulDir is the identity layer: pages describing how and why the user acts —
// values, decision patterns, "core memories". Written ONLY by the soul routine
// after consolidation (never by the add-knowledge pipeline).
const SoulDir = "_soul"

// Kinds are the valid `kind` values in the frontmatter. Free metadata — it no
// longer determines the folder (flat layout, ai-memory style), except for the
// reserved _realtime/_soul subdirs.
var Kinds = []string{"concept", "decision", "gotcha", "note", "rule", "realtime", "soul"}

// DataHome returns the data root: VBRAIN_HOME if set and non-empty; otherwise,
// if the current directory is a base (it carries wiki/), use it — this covers
// the cloud where the checkout is the base itself and the sub-agent doesn't
// inherit the shell env; only then fall back to ~/vbrain.
func DataHome() string {
	if env := os.Getenv("VBRAIN_HOME"); env != "" {
		return expand(env)
	}
	if cwd, err := os.Getwd(); err == nil && IsBase(cwd) {
		return cwd
	}
	return expand(filepath.Join("~", "vbrain"))
}

// IsBase reports whether dir is a vbrain base (it carries wiki/, the source of
// truth). The vbrain source checkout also carries a wiki/, but it is NOT a base:
// it has the Go module. Treating it as the base would bootstrap base assets into
// the source tree and push them to the code remote, so go.mod disqualifies it.
func IsBase(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, "wiki"))
	if err != nil || !fi.IsDir() {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		return false
	}
	return true
}

func RawDir() string  { return filepath.Join(DataHome(), "raw") }
func WikiDir() string { return filepath.Join(DataHome(), "wiki") }
func DBDir() string   { return filepath.Join(DataHome(), "db") }
func DBPath() string  { return filepath.Join(DBDir(), "vbrain.sqlite3") }
func TmpDir() string  { return filepath.Join(RawDir(), ".tmp") }

// EnsureDirs creates the flat directory structure plus wiki/_realtime and
// wiki/_soul.
func EnsureDirs() error {
	dirs := []string{RawDir(), WikiDir(), DBDir(), TmpDir(),
		filepath.Join(WikiDir(), RealtimeDir), filepath.Join(WikiDir(), SoulDir)}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// expand resolves `~` to the user's home and makes the path absolute, mirroring
// Ruby's File.expand_path.
func expand(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}
