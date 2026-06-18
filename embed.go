// Package vbrain (module root) carries assets embedded in the binary — today the
// agent skills, so `vbrain install` can install them without the cloned repo.
// It lives at the root because go:embed can only reach files under the .go
// file's directory (and `.claude/` lives at the root).
package vbrain

import "embed"

// SkillsFS holds .claude/skills/** embedded. `all:` includes files starting
// with `.`/`_`.
//
//go:embed all:.claude/skills
var SkillsFS embed.FS

// OptionalSkillsFS holds .claude/optional-skills/** — skills NOT installed by
// default. The user opts in via `vbrain skill install <name>`; once installed
// they're kept current on every `vbrain update` (reinstalled from this embed).
// go:embed requires the directory to be non-empty, so it ships with at least one
// optional skill.
//
//go:embed all:.claude/optional-skills
var OptionalSkillsFS embed.FS
