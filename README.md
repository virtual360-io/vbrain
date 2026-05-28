# vbrain

Base de conhecimento pessoal — inspirada em
[akitaonrails/ai-memory](https://github.com/akitaonrails/ai-memory), reduzida
a duas Claude Code skills + scripts Ruby + SQLite FTS5.

## Como funciona

- **`~/vbrain/wiki/`** é a fonte da verdade: markdown com frontmatter YAML,
  organizado em `concepts/`, `decisions/`, `gotchas/`, `notes/`, `_rules/`.
- **`~/vbrain/raw/`** guarda os originais imutáveis ingeridos.
- **`~/vbrain/db/vbrain.sqlite3`** é o índice — uma SQLite com `pages` +
  virtual FTS5 `pages_fts` (mesmo padrão do ai-memory). Pode ser apagado a
  qualquer momento e `scripts/reindex.rb` reconstrói lendo `wiki/`. Não há
  `index.md`: o índice é puramente o SQLite.

A localização da pasta de dados pode ser sobrescrita com `VBRAIN_HOME`. O
**código** (este repo) fica separado dos dados.

## Skills

- `/add-knowledge <path>` — ingere um arquivo: copia para `raw/`, quebra em
  chunks via subagente, gera páginas wiki, reindexa.
- `/query-knowledge <pergunta>` — busca FTS5 e devolve trechos relevantes.

## Setup

```bash
bundle install
bundle exec ruby scripts/install.rb            # instala skills em ~/.claude/skills/
```

O install é **idempotente** — rode de novo após `git pull` para atualizar.
Ele reescreve as SKILL.md substituindo paths relativos por absolutos (apontando
para este repo) e seta `BUNDLE_GEMFILE`, então as skills funcionam de qualquer
diretório.

`VBRAIN_HOME` pode ser exportado no shell para mover a base para outro lugar
(ex.: `~/Documents/vbrain`).

## Testes

```bash
bundle exec rake test
```

Todo arquivo determinístico (`lib/vbrain/*` e `scripts/*`) tem teste minitest
correspondente em `test/`. Os testes isolam dados em tmpdir via `VBRAIN_HOME`.

## Verificação manual

```bash
printf "# Postgres\n\nUse REPLICA IDENTITY FULL p/ logical replication.\n" > /tmp/pg.md
# rode a skill /add-knowledge passando /tmp/pg.md
bundle exec ruby scripts/query.rb "replica identity" --format markdown
bundle exec ruby scripts/query.rb "postgres:logical"   # ':' não quebra FTS5
bundle exec ruby scripts/stats.rb                       # estatísticas do banco
```
