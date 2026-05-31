// Package vbrain (raiz do módulo) carrega assets embutidos no binário — hoje as
// skills do agente, pra que `vbrain install` as instale sem precisar do
// repositório clonado. Está na raiz porque go:embed só alcança arquivos sob o
// diretório do .go (e `.claude/` vive na raiz).
package vbrain

import "embed"

// SkillsFS contém .claude/skills/** embutido. `all:` inclui os arquivos que
// começam com `.`/`_`.
//
//go:embed all:.claude/skills
var SkillsFS embed.FS
