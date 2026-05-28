---
name: add-knowledge
description: Ingere um arquivo no vbrain — copia para raw/, quebra em chunks via subagente, gera páginas wiki grounded, reindexa SQLite FTS5. Use quando o usuário pedir "salva isso no vbrain", "adiciona à base", ou fornecer um arquivo de notas/transcript/doc/repo/planilha para arquivar.
allowed-tools: Bash, Read, Write, Agent, AskUserQuestion
---

# add-knowledge

Pipeline determinístico (Ruby) + 2 subagentes LLM (chunker + wiki-writer) para
transformar um arquivo bruto em páginas wiki indexadas no vbrain.

## Inputs

- **path** (obrigatório): caminho absoluto do arquivo ou diretório a ingerir.
- **--type** (opcional): força o `source_type` quando a detecção heurística
  errar (`text` | `transcript` | `epub` | `repo` | `spreadsheet`). Só inclua
  se o usuário pedir explicitamente.

## Passos

### 1. Ingerir o raw

```bash
bundle exec ruby scripts/ingest_raw.rb <path>
```

Parse o JSON de saída. Possíveis casos:

- `{"source_type":"text"|"transcript"|...,"raw_id":N,"raw_path":...,"extracted_path":...}` → siga ao passo 2.
- `{"duplicate":true,"raw_id":N,...}` → o sha256 desse arquivo já existe.
  Pergunte ao usuário se quer reprocessar (`--force`) ou abortar.
- `{"source_type":"unknown",...}` → **não prossiga**. Use `AskUserQuestion`
  oferecendo duas saídas:
  1. **Criar fonte permanente** — gere os esqueletos abaixo, peça ao usuário
     para rodar `bundle exec rake test`, então re-rode `ingest_raw.rb`:
     - `lib/vbrain/sources/<nome>.rb` (extends `Base`, implementa `detect?`,
       `kind_key`, `extract`)
     - `test/lib/vbrain/sources/<nome>_test.rb` (detect ✓, detect ✗, extract)
     - `test/fixtures/<nome>/sample.<ext>`
     - `.claude/skills/add-knowledge/prompts/chunker/<nome>.md` (copie o
       núcleo de `text.md` + ajuste a seção `## Heurísticas`)
     - Registre o módulo em `lib/vbrain/sources.rb` (adicione ao `REGISTRY`).
  2. **One-shot via LLM** — copie o arquivo para `raw/` manualmente, lance
     um subagente extractor que produz texto plano em
     `raw/.tmp/extracted-<timestamp>.txt`, e prossiga ao passo 2 usando
     `prompts/chunker/text.md` como chunker. Marque depois o registro como
     `source_type='oneshot'` em `raw_sources`.

### 2. Chunkar (subagente)

Leia `extracted_path`. Lance um subagente `general-purpose` com o conteúdo de
`prompts/chunker/<source_type>.md` como system prompt e o texto extraído como
user message. Exija saída JSON estrita.

Schema esperado:

```json
{"chunks":[
  {"suggested_title":"...",
   "category":"concepts|decisions|gotchas|notes|_rules",
   "tags":["..."],
   "raw_excerpt":"trecho literal",
   "summary_hint":"1 frase neutra"}
]}
```

Se `chunks` vier vazio, aborte e reporte: o documento não rendeu conhecimento
durável; ofereça ao usuário rodar de novo com `--type` diferente ou descartar.

### 3. Escrever wiki (subagente)

Para cada chunk, lance um subagente `general-purpose` com
`prompts/wiki-writer.md` como system prompt e o chunk individual como user
message. Você pode rodar em paralelo (múltiplos Agent na mesma mensagem).

Agregue as saídas em um único JSON `{"pages":[...]}` e escreva em
`raw/.tmp/pages-<raw_id>.json`. Essa é a única escrita direta do Claude no
filesystem nesta skill — todo o resto vai via scripts Ruby.

### 4. Persistir as páginas

```bash
bundle exec ruby scripts/write_pages.rb --raw-id <N> --pages-json raw/.tmp/pages-<N>.json
```

A saída JSON tem `{"written":["concepts/foo.md",...],"count":N}`.

### 5. Reindexar

```bash
bundle exec ruby scripts/reindex.rb
```

A saída tem `{"inserted":N,"updated":N,"deleted":N}`. Triggers FTS5 mantêm o
índice de busca em sync automaticamente.

### 6. Reportar ao usuário

Mostre:
- Tipo detectado (`source_type`)
- Lista de paths gerados em `wiki/`
- Total de páginas no banco: `bundle exec sqlite3 db/vbrain.sqlite3 "SELECT COUNT(*) FROM pages;"`

## Regras duras

- **Nunca** escrever em `wiki/` diretamente; sempre via `write_pages.rb`.
- **Nunca** modificar `raw/` depois de ingerido — é imutável.
- Se `rake test` falhar antes da execução final (caso 1 do tipo desconhecido),
  **não** prosseguir até o usuário consertar.
- **FAITHFULNESS**: os subagentes têm regra dura de não inventar; se eles
  retornarem dados claramente fabricados, sinalize ao usuário.
