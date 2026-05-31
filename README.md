# vbrain

Base de conhecimento pessoal — inspirada em
[akitaonrails/ai-memory](https://github.com/akitaonrails/ai-memory), reduzida
a Claude Code skills + um binário Go determinístico (`vbrain`) + SQLite FTS5.

A premissa: **wiki em markdown é a fonte da verdade; o SQLite é índice
derivado — descartável (dá pra apagar e reconstruir com `vbrain reindex`), mas
versionado junto da base por conveniência (clone/pull já trazem o índice
pronto); o LLM só entra para o que exige julgamento (chunkar, sintetizar
páginas)**. Todo o resto é Go testado.

> **Migrado de Ruby para Go.** O núcleo determinístico era Ruby; hoje é um
> binário único `vbrain` (sem runtime pra instalar): SQLite via
> `modernc.org/sqlite` (puro-Go, FTS5 embutido) e git via go-git, com fallback
> pro git do sistema quando presente.

## Arquitetura

### Separação código vs. dados

| Diretório                   | O que é                                                      | Versionado                    |
|---|---|---|
| Este repo                   | **Código** (Go), skills, testes (canônico)                   | git aqui                       |
| `~/vbrain/` (`VBRAIN_HOME`)  | **Sua base** — `raw/`, `wiki/`, `config/`, `db/vbrain.sqlite3` | git próprio, criado on demand |

O `install.sh` builda o binário `vbrain` num dir do PATH (`~/.local/bin` por
padrão), instala as skills globalmente (`~/.claude/skills/`) e bootstrapa a
base via `vbrain setup` (CLAUDE.md + skills + git init + rotinas). Como as
skills chamam o binário `vbrain` (no PATH), a base **não copia código** — basta
o binário. Roda em qualquer ambiente que clone a base, sem Ruby nem gems.

A base é resolvida nesta ordem: (1) `VBRAIN_HOME`, se setado; (2) senão, se o
diretório atual é uma base (carrega `wiki/`, como no cloud onde repo == base),
usa-o — assim as skills/sub-agentes acham os dados sem herdar `VBRAIN_HOME` do
shell; (3) senão, `~/vbrain`. A wiki vira um repo git separado no `vbrain setup`
— privado, público ou só-local conforme escolha do usuário.

### Layout da base (`~/vbrain/`)

```
~/vbrain/
├── raw/                 # originais imutáveis (audit log)
│   └── .tmp/            # intermediários do pipeline (extracted-N.txt, pages-N.json)
├── wiki/                # markdown com frontmatter YAML — fonte da verdade
│   ├── <slug>.md        # páginas de conhecimento, espaço plano; [[wikilinks]]
│   └── _realtime/       # kind: realtime — páginas fantasma que disparam handlers MCP
├── config/
│   ├── realtime/        # config das fontes realtime (gcalendar.yml etc.)
│   └── routines/routines.yml
└── db/vbrain.sqlite3    # índice — pages + virtual pages_fts (FTS5) + links (grafo)
```

`db/vbrain.sqlite3` **é versionado** (conveniência); continua descartável —
apagar `db/` e rodar `vbrain reindex` reconstrói tudo a partir de `wiki/`,
incluindo o grafo de `[[wikilinks]]`. Não existe `wiki/index.md` nem pastas por
tipo: a estrutura é o grafo de links + o SQLite derivado.

### O binário `vbrain`

JSON no stdout (lido pelas skills), texto humano no stderr. Subcomandos:

| Subcomando | O que faz |
|---|---|
| `vbrain ingest <path\|url>`  | detecta fonte, copia p/ `raw/`, dedup por sha256, extrai |
| `vbrain write-pages --raw-id N --pages-json P` | única escrita em `wiki/` (staging atômico + guardrail de órfãos) |
| `vbrain reindex`             | reconstrói `pages`/`pages_fts`/`links` a partir de `wiki/` |
| `vbrain query "<q>"`         | FTS5 + snippet + vizinhos do grafo |
| `vbrain resolve-links --map M` / `vbrain linkify` | resolve/converte wikilinks |
| `vbrain commit [--no-push]`  | commit + push idempotente (go-git ou git do sistema) |
| `vbrain routines [--dry-run]` / `vbrain routine-add` / `vbrain routine-list` | agendamento (cron) |
| `vbrain realtime <gcalendar\|gmail\|slack> --json …` | conecta fonte realtime |
| `vbrain tags` / `vbrain stats` / `vbrain query-log` | insights/manutenção |
| `vbrain setup` / `vbrain seed-routines` | bootstrap da base |

### Skills (interface com o Claude Code)

| Slash command                       | O que faz |
|---|---|
| `/vbrain-add-knowledge <path\|url>` | Ingere → `raw/` → chunker LLM → wiki-writer LLM → `vbrain write-pages` → reindex → commit |
| `/vbrain-query-knowledge <query>`   | `vbrain query`; páginas `kind: realtime` disparam handler MCP em vez de snippet |
| `/vbrain-add-realtime-knowledge`    | Conecta fonte realtime (Google Calendar/Gmail/Slack via MCP) e cria página fantasma |
| `/vbrain-add-routine`               | Adiciona rotina (slug, descrição, cron, prompt) |
| `/vbrain-routine [slug\|status]`    | Watch: claim de rotinas vencidas via `vbrain routines`, dispatch paralelo, re-arma `/loop 15m` |

### Fontes (`internal/sources/`)

`sources.Registry` é probada em ordem por `sources.Detect`:

| Source     | Detecção                                    | Extração |
|---|---|---|
| Twitter    | URL `twitter.com\|x.com/<user>/status/<id>` | `cdn.syndication.twimg.com` (HTTP+JSON); corpo de X Article via Playwright é best-effort (degrada p/ preview) |
| URL        | Outras URLs http(s)                         | Jina Reader (`r.jina.ai`) — markdown limpo |
| Text       | `.md`, `.txt`, sem extensão + UTF-8         | passthrough |

### Schema do índice (`internal/db`)

```sql
raw_sources(id, path UNIQUE, original_filename, source_type, sha256 UNIQUE, ingested_at)
pages(id, path UNIQUE, title, body, kind, tags, sha256, raw_id → raw_sources, created_at, updated_at)
pages_fts(title, body, tags)              -- virtual FTS5, content='pages'
  tokenize: unicode61 tokenchars '/_-'
```

Triggers `pages_ai`/`pages_ad`/`pages_au` espelham toda escrita em `pages` pra
`pages_fts`. `vbrain query` normaliza a query (escapa `:`, aspas, parênteses)
antes do FTS5.

### Realtime e rotinas

Páginas `kind: realtime` carregam só keywords (pra casar no FTS5) + metadados;
a config real fica em `config/realtime/<source>.yml`. Quando `query-knowledge`
acha uma, dispara o handler MCP (`list_events`/`search_threads`/Slack search) em
vez do snippet.

Rotinas são prompts nomeados com cron em `config/routines/routines.yml`;
`next_run` é computado deterministicamente (robfig/cron). Execução é
**at-most-once** (avança o `next_run` antes de rodar). A rotina-padrão `dream`
(auto-melhoria noturna) é semeada no setup.

## Diretórios deste repo

```
vbrain/
├── cmd/vbrain/          # CLI (subcomandos, JSON no stdout)
├── internal/            # núcleo determinístico (paths, db, page, slug, ftsquery,
│                        #   links, sources, index, search, writepages, ingest,
│                        #   resolvelinks, git, routines, realtime, maint, scaffold)
├── .claude/skills/      # SKILL.md + prompts dos subagentes
└── install.sh           # build + instala skills + bootstrap da base
```

Todo pacote em `internal/` tem teste `go test` correspondente. Os testes isolam
dados em tmpdir via `VBRAIN_HOME` / dirs explícitos.

## Setup

```bash
git clone <repo> && cd vbrain
./install.sh          # builda vbrain → PATH, instala skills, bootstrapa a base
```

Pré-requisitos antes do install: um shell, git (pra clonar) e o toolchain Go
(pra buildar; ou um binário `vbrain` pré-compilado). Depois do install nada de
Ruby/gems — o `vbrain` é autocontido. `VBRAIN_HOME` pode ser exportado pra mover
a base.

## Testes

```bash
go test ./...
```

## Verificação manual

```bash
printf "# Postgres\n\nUse REPLICA IDENTITY FULL p/ logical replication.\n" > /tmp/pg.md
vbrain ingest /tmp/pg.md
# (chunker/wiki-writer rodam via skill; ou monte um pages.json e:)
vbrain reindex
vbrain query "replica identity" --format markdown
vbrain query "postgres:logical"   # ':' não quebra FTS5
vbrain stats
```
