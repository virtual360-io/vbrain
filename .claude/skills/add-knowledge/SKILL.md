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
- `{"source_type":"unknown",...}` **OU** a extração determinística retornou
  conteúdo trivial (< 100 palavras úteis, login wall, "Sign in", "Continue
  with Apple", etc.):
  - **DEFAULT: via LLM.** Não pergunte ao usuário se vale criar uma fonte
    permanente. Use `WebFetch` (para URLs) ou um subagente extractor genérico
    (para arquivos) para gerar texto plano em
    `raw/.tmp/extracted-<raw_id>.txt`. Atualize a row em `raw_sources` para
    `source_type='oneshot'`. Em seguida siga ao passo 2 usando
    `prompts/chunker/text.md` como chunker.
  - **Caso especial — links do X/Twitter** (tweets, X Articles, posts gated):
    NÃO tente builds determinísticos adicionais. NÃO sugira `Sources::X`. X
    bloqueia scraping consistentemente; o caminho que sempre funciona é
    `WebFetch` lendo a URL como um humano leria, e usar o resultado como
    input do chunker. O mesmo vale para qualquer site com auth/paywall.
  - Só sugira "criar fonte permanente" se o usuário pedir explicitamente
    ("quero ingerir muitos arquivos desse formato, vale fazer fonte?").

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

A saída tem `{"inserted":N,"updated":N,"deleted":N}`. O índice é puramente
o SQLite (`pages` + virtual `pages_fts`); triggers AI/AD/AU sincronizam o FTS5
automaticamente. Não há `wiki/index.md` — espelhamos o ai-memory, onde a
única estrutura de índice é o SQLite derivado.

### 6. Reportar ao usuário

Mostre:
- Tipo detectado (`source_type`)
- Lista de paths gerados em `wiki/`
- Estatísticas do banco: `bundle exec ruby scripts/stats.rb` (retorna JSON com
  total de páginas, distribuição por kind e 5 mais recentes)

## Regras duras

- **Nunca** escrever em `wiki/` diretamente; sempre via `write_pages.rb`.
- **Nunca** modificar `raw/` depois de ingerido — é imutável.
- Se `rake test` falhar antes da execução final (caso 1 do tipo desconhecido),
  **não** prosseguir até o usuário consertar.
- **FAITHFULNESS**: os subagentes têm regra dura de não inventar; se eles
  retornarem dados claramente fabricados, sinalize ao usuário.
