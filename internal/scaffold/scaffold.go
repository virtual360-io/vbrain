// Package scaffold instala os "assets do agente" na base (~/vbrain) pra ela ser
// autossuficiente em qualquer ambiente que a clone: CLAUDE.md instruindo o uso
// das skills + cópia das skills. Porta de lib/vbrain/scaffold.rb — mas no mundo
// Go a base NÃO carrega mais código: basta o binário `vbrain` no PATH.
package scaffold

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// ClaudeMD é o conteúdo do CLAUDE.md escrito na base.
const ClaudeMD = `# CLAUDE.md — base de conhecimento vbrain

Este repositório é a **sua base de conhecimento pessoal vbrain** e é
autossuficiente: contém os dados versionados (` + "`raw/`, `wiki/`, `db/vbrain.sqlite3`, `config/`" + `)
e as skills do agente em ` + "`.claude/skills/`" + `.

## Regra principal — SEMPRE use as skills vbrain

Toda operação na base passa pelas skills (slash commands). **Nunca** edite
` + "`wiki/`, `raw/` ou `db/`" + ` na mão, nem rode SQL direto: isso quebra o índice
e o grafo de links.

| Quero…                                          | Use a skill                       |
|---|---|
| Consultar a base                                | ` + "`/vbrain-query-knowledge`" + `         |
| Adicionar conhecimento (arquivo/URL/nota)       | ` + "`/vbrain-add-knowledge`" + `           |
| Conectar fonte realtime (calendar/gmail/slack)  | ` + "`/vbrain-add-realtime-knowledge`" + `  |
| Criar uma rotina                                | ` + "`/vbrain-add-routine`" + `             |
| Rodar as rotinas (watch loop)                   | ` + "`/vbrain-routine`" + `                 |

## Pré-requisitos

As skills são determinísticas e chamam o binário **` + "`vbrain`" + `**, que deve estar
no PATH (instale via ` + "`install.sh`" + ` do repositório de código). Não é preciso
Ruby nem instalar gems: o ` + "`vbrain`" + ` é um binário único, autocontido.

## Por quê (arquitetura)

Wiki em markdown é a fonte da verdade; o SQLite (` + "`db/vbrain.sqlite3`" + `) é
índice derivado — descartável (dá pra apagar e reconstruir com
` + "`vbrain reindex`" + `), mas versionado por conveniência. O LLM só entra pro
que exige julgamento (chunkar, sintetizar páginas).
`

// WriteClaudeMD escreve o CLAUDE.md se ainda não existir (não clobbera
// customização do usuário). Retorna true se escreveu.
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

// InstallSkills copia cada skill (subdiretório de skills, um fs.FS — tipicamente
// o embed.FS do binário, com fs.Sub na raiz das skills) para
// <dir>/.claude/skills/. Idempotente: remove o destino de cada skill antes de
// copiar. Retorna o número de skills instaladas.
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

// copyTreeFS copia srcDir (dentro de fsys) para dstDir no disco.
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
