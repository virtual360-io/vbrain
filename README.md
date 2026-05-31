# vbrain

Base de conhecimento pessoal — inspirada em
[akitaonrails/ai-memory](https://github.com/akitaonrails/ai-memory), reduzida
a três Claude Code skills + scripts Ruby determinísticos + SQLite FTS5.

A premissa: **wiki em markdown é a fonte da verdade; o SQLite é índice
derivado — descartável (dá pra apagar e reconstruir com `reindex.rb`), mas
versionado junto da base por conveniência (clone/pull já trazem o índice
pronto); o LLM só entra para o que exige julgamento (chunkar, sintetizar
páginas)**. Todo o resto é Ruby testado.

## Arquitetura

### Separação código vs. dados

Dois diretórios distintos:

| Diretório                   | O que é                                                  | Versionado                       |
|---|---|---|
| `~/Workspace/vbrain/`       | **Este repo** — código (Ruby), skills, testes (canônico) | git aqui                         |
| `~/vbrain/` (`VBRAIN_HOME`) | **Sua base** — `raw/`, `wiki/`, `config/`, `db/vbrain.sqlite3` | git próprio, criado on demand |

`scripts/install.rb` torna a base **autossuficiente**: além de instalar as
skills no `~/.claude` global, copia pra base um `CLAUDE.md`, as skills cruas
(`.claude/skills/`) e o código que elas usam (`scripts/`, `lib/`, `Gemfile`,
`Gemfile.lock`, `.ruby-version`). Assim a base roda em qualquer ambiente que a
clone (ex.: cloud) só com Ruby 3.3.6 + `bundle install`, sem precisar deste
repo. O código aqui continua o **canônico**; a cópia na base é sincronizada
re-rodando o install (a duplicação é intencional — trade-off por portabilidade).

A base é resolvida nesta ordem: (1) `VBRAIN_HOME`, se setado, sobrescreve tudo
(ex.: `~/Documents/vbrain`); (2) senão, se o código está rodando de dentro de
uma base (o checkout carrega a própria `wiki/`, como no cloud onde repo ==
base), usa essa base — assim as skills/sub-agentes acham os dados sem herdar
`VBRAIN_HOME` do shell; (3) senão, `~/vbrain`.
A wiki vira um repo git separado no primeiro `add-knowledge` — privado, público
ou só-local conforme escolha do usuário (ver `scripts/init_repo.rb`).

### Layout da base (`~/vbrain/`)

```
~/vbrain/
├── raw/                 # originais imutáveis (audit log)
│   ├── 20260528T...-pg.md
│   └── .tmp/            # arquivos intermediários do pipeline (extracted-N.txt, pages-N.json)
├── wiki/                # markdown com frontmatter YAML — fonte da verdade
│   ├── <slug>.md        # páginas de conhecimento, espaço plano; conectam-se por [[wikilinks]]
│   │                    #   frontmatter `kind:` (concept/decision/gotcha/note/rule) é só metadado
│   └── _realtime/       # kind: realtime — páginas fantasma que disparam handlers ao vivo
├── config/
│   ├── realtime/        # config das fontes realtime (gcalendar.yml: lista de calendar IDs)
│   └── routines/
│       └── routines.yml # lista de rotinas (slug + description + prompt + enabled)
└── db/
    └── vbrain.sqlite3   # índice puro — `pages` + virtual `pages_fts` (FTS5) + `links` (grafo)
```

`db/vbrain.sqlite3` **é versionado** (commitado junto da base): o `.gitignore`
gerado por `lib/vbrain/git.rb` deixa `/db/` fora da lista de ignore, e o
`commit.rb` (via `git add -A`) o inclui. Isso é só conveniência — o índice
continua **descartável**: apagar `db/` e rodar `scripts/reindex.rb` reconstrói
tudo a partir de `wiki/`, incluindo o grafo (o reindex parseia os
`[[wikilinks]]` do corpo das páginas e remonta a tabela `links`). Não existe
`wiki/index.md` nem pastas por tipo — espelhamos o ai-memory: a estrutura é o
grafo de links + o SQLite derivado, e a LLM organiza as páginas livremente.

### Skills (interface com o Claude Code)

| Slash command                       | O que faz                                                                                          |
|---|---|
| `/vbrain-add-knowledge <path\|url>` | Ingere arquivo/URL → `raw/` → chunker LLM → wiki-writer LLM → `write_pages.rb` → reindex → commit  |
| `/vbrain-query-knowledge <query>`   | Roda FTS5 via `query.rb`; páginas `kind: realtime` disparam handler MCP em vez de retornar snippet |
| `/vbrain-add-realtime-knowledge`    | Conecta fonte realtime (hoje: Google Calendar e Gmail via MCP) e cria página fantasma em `wiki/_realtime/` |
| `/vbrain-add-routine`               | Adiciona rotina (slug, descrição, cron, prompt) e bootstrap do watch loop                            |
| `/vbrain-routine [slug\|status]`    | **Watch (default)**: claim de rotinas vencidas via `run_due_routines.rb`, dispatch paralelo, re-armar `/loop 15m` |

As skills moram em `.claude/skills/vbrain-*/` neste repo. O `scripts/install.rb`
copia tudo para `~/.claude/skills/` reescrevendo paths relativos por absolutos
(apontando para este repo) e setando `BUNDLE_GEMFILE` — assim as skills rodam
de qualquer CWD.

### Pipeline de ingest (add-knowledge)

```
       ┌───────────────────────────────────────────────────────────────┐
input ─┤ Claude orquestra; passos numerados são Ruby determinístico    │
       └───────────────────────────────────────────────────────────────┘
        │
        ▼
  ┌──────────────────────────┐
  │ 0. init_repo.rb          │  só no 1º ingest — pergunta privado/público/local
  └──────────────────────────┘
        │
        ▼
  ┌──────────────────────────┐
  │ 1. ingest_raw.rb <input> │  detecta source_type, copia p/ raw/, extrai texto
  └──────────────────────────┘    em raw/.tmp/extracted-<raw_id>.txt
        │
        ▼ (LLM subagente)
  ┌──────────────────────────┐
  │ 2. chunker/<type>.md     │  prompts/chunker/{text,url,tweet}.md → JSON {chunks:[…]}
  └──────────────────────────┘    2b. fallback se 0 chunks: follow links → Wayback → ask
        │
        ▼ (LLM subagente, paralelo)
  ┌──────────────────────────┐
  │ 3. wiki-writer.md        │  cada chunk → uma página com frontmatter YAML grounded
  └──────────────────────────┘    agregado em raw/.tmp/pages-<raw_id>.json
        │
        ▼
  ┌──────────────────────────┐
  │ 4. write_pages.rb        │  única escrita em wiki/ é via esse script
  └──────────────────────────┘
        │
        ▼
  ┌──────────────────────────┐
  │ 5. reindex.rb            │  reescreve pages do banco a partir de wiki/
  └──────────────────────────┘    triggers AI/AD/AU mantêm pages_fts sincronizado
        │
        ▼
  ┌──────────────────────────┐
  │ 6. commit.rb             │  commit + push idempotente no repo da base
  └──────────────────────────┘
```

### Fontes (`lib/vbrain/sources/`)

`Sources::REGISTRY` é probada em ordem por `Sources.detect`:

| Source              | Detecção                                    | Extração                                                                   |
|---|---|---|
| `Sources::Twitter`  | URL `twitter.com\|x.com/<user>/status/<id>` | `cdn.syndication.twimg.com` + Playwright (Chrome do sistema) p/ X Articles |
| `Sources::Url`      | Outras URLs http(s)                         | Jina Reader (`r.jina.ai`) — markdown limpo                                 |
| `Sources::Text`     | `.md`, `.txt`, sem extensão + UTF-8         | passthrough                                                                |

Cada fonte tem chunker dedicado em
`.claude/skills/vbrain-add-knowledge/prompts/chunker/<kind_key>.md` —
não um chunker genérico (regra dura: cada formato precisa de pré-processador
Ruby + chunker prompt próprio).

Tipo desconhecido cai em `source_type='oneshot'`: o Claude usa `WebFetch` ou
subagente extractor genérico, marca como oneshot no banco, e segue com
`chunker/text.md`. Só vira fonte permanente se o usuário pedir explicitamente.

### Schema do índice (`lib/vbrain/db.rb`)

```sql
raw_sources(id, path UNIQUE, original_filename, source_type, sha256 UNIQUE, ingested_at)
pages(id, path UNIQUE, title, body, kind, tags, sha256, raw_id → raw_sources, created_at, updated_at)
pages_fts(title, body, tags)              -- virtual FTS5, content='pages'
  tokenize: unicode61 tokenchars '/_-'    -- preserva slashes, underscores, hífens
```

Triggers `pages_ai`/`pages_ad`/`pages_au` espelham toda escrita em `pages`
para `pages_fts`. `kind` é checado por CHECK constraint
(`concept|decision|gotcha|note|rule|realtime`); migração transparente para
schemas antigos via `rebuild_pages_if_old_kind_check!`.

`lib/vbrain/fts_query.rb` normaliza a query do usuário (escapa `:`, aspas,
parênteses) antes de mandar pro FTS5.

### Realtime (`wiki/_realtime/` + handlers MCP)

Páginas com `kind: realtime` contêm só keywords sintéticas no body — o
suficiente pra casar no FTS5 — e metadados no frontmatter (`source: …` +
parâmetros). A config real (calendar IDs, label IDs, etc.) fica em
`~/vbrain/config/realtime/<source>.yml`, fora do índice.

| `source`    | Page             | Body keywords                          | Handler MCP                                                      | Config                                |
|---|---|---|---|---|
| `gcalendar` | `gcalendar.md`   | agenda, reunião, hoje, amanhã, semana… | `mcp__claude_ai_Google_Calendar__list_events` (range temporal)   | `calendars: [{id, summary, timezone}]` |
| `gmail`     | `gmail.md`       | email, e-mail, inbox, mensagem, anexo… | `mcp__claude_ai_Gmail__search_threads` (label filter + Gmail syntax) | `labels: [{id, name}]`                |

Quando `query-knowledge` encontra uma dessas, ele **não** mostra o snippet —
dispara o handler MCP correspondente e formata o resultado ao vivo. Pra
adicionar nova fonte realtime, replique o trio:
`lib/vbrain/realtime/<source>.rb` (helper), `scripts/add_realtime/<source>.rb`
(CLI), entrada no dispatcher do `vbrain-query-knowledge`.

### Rotinas (`~/vbrain/config/routines/routines.yml`)

Uma rotina é um **prompt nomeado com cron**. Cada entry tem `slug`,
`description`, `schedule` (5-field cron), `next_run` (ISO8601 UTC,
computado deterministicamente por [fugit](https://github.com/floraison/fugit)),
`last_run`, `prompt`, e `enabled`. Exemplo:

```yaml
routines:
  - slug: morning-brief
    description: Resumo da manhã (inbox + agenda do dia)
    schedule: "0 6 * * *"
    next_run: "2026-05-29T09:00:00Z"
    last_run: "2026-05-28T09:00:00Z"
    prompt: |
      Usa /vbrain-query-knowledge pra mostrar:
      1. Emails INBOX recebidos hoje (top 5)
      2. Reuniões/eventos do calendário hoje
      Resuma em até 5 bullets.
    enabled: true
```

**Modelo de execução** (watch como default):

1. `/vbrain-add-routine` adiciona ao YAML com `next_run` inicial e
   bootstrappa o watch loop chamando `/vbrain-routine`.
2. `/vbrain-routine` (sem args) chama `scripts/run_due_routines.rb` que
   atomicamente: identifica rotinas com `next_run <= now`, avança o
   `next_run` (próximo tick do cron), retorna a lista pra skill executar.
   Cada rotina vai pra um **sub-agente paralelo**. Em seguida re-arma
   `/loop 15m /vbrain-routine` (granularidade de detecção ~15min).
3. Semântica é **at-most-once**: se o sub-agente falhar, aquele run é
   perdido (não re-tentamos). Pra forçar execução manual, use
   `/vbrain-routine <slug>` (não altera state).
4. `/vbrain-routine status` lista cron + next_run + last_run.

O cron é interpretado no **TZ local do sistema**, mas armazenado em UTC.

Storage é **separado da wiki** — rotinas são comandos, não conhecimento,
e não entram no FTS5.

## Diretórios deste repo

```
vbrain/
├── lib/vbrain/          # núcleo determinístico (paths, db, page, slug, fts_query, git, sources/, realtime/)
├── scripts/             # entrypoints chamados pelas skills (CLI Ruby, output JSON)
├── .claude/skills/      # SKILL.md + prompts dos subagentes (chunker, wiki-writer)
├── test/                # minitest 1:1 com lib/ e scripts/
├── Gemfile              # sqlite3, playwright-ruby-client, minitest, rake
└── Rakefile             # `rake test`
```

Todo arquivo em `lib/vbrain/` e `scripts/` tem teste correspondente em `test/`.
Os testes isolam dados em tmpdir via `VBRAIN_HOME`.

## Setup

```bash
bundle install
bundle exec ruby scripts/install.rb   # idempotente — rerode após git pull
```

O install reescreve as SKILL.md substituindo paths relativos por absolutos
(apontando para este repo) e seta `BUNDLE_GEMFILE`, então as skills funcionam
de qualquer diretório. `VBRAIN_HOME` pode ser exportado no shell para mover a
base para outro lugar (ex.: `~/Documents/vbrain`).

## Testes

```bash
bundle exec rake test
```

## Verificação manual

```bash
printf "# Postgres\n\nUse REPLICA IDENTITY FULL p/ logical replication.\n" > /tmp/pg.md
# rode a skill /vbrain-add-knowledge passando /tmp/pg.md
bundle exec ruby scripts/query.rb "replica identity" --format markdown
bundle exec ruby scripts/query.rb "postgres:logical"   # ':' não quebra FTS5
bundle exec ruby scripts/stats.rb                       # JSON com total + distribuição por kind
```
