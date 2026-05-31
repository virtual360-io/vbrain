// Package scaffold installs the "agent assets" into the base (~/vbrain) so it's
// self-sufficient in any environment that clones it: a CLAUDE.md instructing how
// to use the skills + a copy of the skills. In the Go world the base no longer
// carries any code: the `vbrain` binary on the PATH is enough.
package scaffold

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// ClaudeMD is the content of the CLAUDE.md written into the base.
const ClaudeMD = `# CLAUDE.md — vbrain knowledge base

This repository is **your personal vbrain knowledge base** and is
self-sufficient: it holds the versioned data (` + "`raw/`, `wiki/`, `db/vbrain.sqlite3`, `config/`" + `)
and the agent skills in ` + "`.claude/skills/`" + `.

## Main rule — ALWAYS use the vbrain skills

Every operation on the base goes through the skills (slash commands). **Never**
edit ` + "`wiki/`, `raw/` or `db/`" + ` by hand, nor run raw SQL: that breaks the index
and the link graph.

| I want to…                                      | Use the skill                     |
|---|---|
| Query the base                                  | ` + "`/vbrain-query-knowledge`" + `         |
| Add knowledge (file/URL/note)                   | ` + "`/vbrain-add-knowledge`" + `           |
| Connect a realtime source (calendar/gmail/slack)| ` + "`/vbrain-add-realtime-knowledge`" + `  |
| Create a routine                                | ` + "`/vbrain-add-routine`" + `             |
| Run the routines (watch loop)                   | ` + "`/vbrain-routine`" + `                 |

## Prerequisites

The skills are deterministic and call the **` + "`vbrain`" + `** binary, which must be
on the PATH (install with ` + "`vbrain install`" + `). No Ruby and no gem install
needed: ` + "`vbrain`" + ` is a single, self-contained binary.

## Why (architecture)

Markdown wiki is the source of truth; SQLite (` + "`db/vbrain.sqlite3`" + `) is a
derived index — disposable (you can delete it and rebuild with
` + "`vbrain reindex`" + `), but versioned for convenience. The LLM only steps in for
what needs judgment (chunking, synthesizing pages).
`

// WriteClaudeMD writes CLAUDE.md if it doesn't exist yet (does not clobber a
// user's customization). Returns true if it wrote.
func WriteClaudeMD(dir string) (bool, error) {
	path := filepath.Join(dir, "CLAUDE.md")
	if _, err := os.Stat(path); err == nil {
		return false, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, []byte(ClaudeMD), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// InstallSkills copies each skill (a subdirectory of skills, an fs.FS —
// typically the binary's embed.FS, fs.Sub'd at the skills root) into
// <dir>/.claude/skills/. Idempotent: removes each skill's target before copying.
// Returns the number of skills installed.
func InstallSkills(dir string, skills fs.FS) (int, error) {
	entries, err := fs.ReadDir(skills, ".")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	dest := filepath.Join(dir, ".claude", "skills")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return 0, err
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		target := filepath.Join(dest, e.Name())
		if err := os.RemoveAll(target); err != nil {
			return count, err
		}
		if err := copyTreeFS(skills, e.Name(), target); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// copyTreeFS copies srcDir (within fsys) to dstDir on disk.
func copyTreeFS(fsys fs.FS, srcDir, dstDir string) error {
	return fs.WalkDir(fsys, srcDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(srcDir, p)
		target := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
