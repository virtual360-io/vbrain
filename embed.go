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
