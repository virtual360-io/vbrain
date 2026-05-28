# vbrain

Base de conhecimento pessoal — inspirada em
[akitaonrails/ai-memory](https://github.com/akitaonrails/ai-memory), reduzida
a duas Claude Code skills + scripts Ruby + SQLite FTS5.

## Como funciona

- **`wiki/`** é a fonte da verdade: markdown com frontmatter YAML, organizado
  em `concepts/`, `decisions/`, `gotchas/`, `notes/`, `_rules/`.
- **`raw/`** guarda os originais imutáveis ingeridos.
- **`db/vbrain.sqlite3`** é o índice derivado (FTS5). Pode ser apagado a
  qualquer momento — `scripts/reindex.rb` reconstrói lendo `wiki/`.

## Skills

- `/add-knowledge <path>` — ingere um arquivo: copia para `raw/`, quebra em
  chunks via subagente, gera páginas wiki, reindexa.
- `/query-knowledge <pergunta>` — busca FTS5 e devolve trechos relevantes.

## Setup

```bash
bundle install
mkdir -p raw wiki/{concepts,decisions,gotchas,notes,_rules} db raw/.tmp
ruby -Ilib -rvbrain/db -e 'VBrain::DB.open {}'
```

## Testes

```bash
bundle exec rake test
```

Todo arquivo determinístico (`lib/vbrain/*` e `scripts/*`) tem teste minitest
correspondente em `test/`.

## Verificação manual

```bash
printf "# Postgres\n\nUse REPLICA IDENTITY FULL p/ logical replication.\n" > /tmp/pg.md
# rode a skill add-knowledge em /tmp/pg.md
bundle exec ruby scripts/query.rb "replica identity" --format markdown
bundle exec ruby scripts/query.rb "postgres:logical"   # ':' não quebra FTS5
```
