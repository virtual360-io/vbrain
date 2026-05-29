---
name: vbrain-add-knowledge
description: Ingere um arquivo ou URL no vbrain — copia para raw/, quebra em chunks via subagente, gera páginas wiki grounded, reindexa SQLite FTS5. Use quando o usuário pedir "salva isso no vbrain", "adiciona à base", ou fornecer um arquivo de notas/markdown, uma URL (artigo, blog) ou um tweet/X article para arquivar.
allowed-tools: Bash, Read, Write, Agent, AskUserQuestion, WebFetch
---

# vbrain-add-knowledge

Pipeline determinístico (Ruby) + 2 subagentes LLM (chunker + wiki-writer) para
transformar um arquivo bruto em páginas wiki indexadas no vbrain.

## Inputs

- **path** (obrigatório): caminho absoluto do arquivo OU URL (http/https).
- **--type** (opcional): força o `source_type` (`text` | `url` | `tweet`)
  quando a detecção heurística errar. Só inclua se o usuário pedir.

## Fontes suportadas

| `source_type` | Detecção | Como extrai |
|---|---|---|
| `tweet` | URL `twitter.com|x.com/<user>/status/<id>` | Syndication endpoint público (`cdn.syndication.twimg.com`) + Playwright + Chrome do sistema pra puxar body completo de X Articles linkados |
| `url` | Outras URLs http(s) | Jina Reader (`r.jina.ai`) — retorna markdown limpo |
| `text` | `.md`, `.txt`, sem extensão + UTF-8 | passthrough |

## Passos

### 0. Garantir repo git no `~/vbrain` (apenas no 1º ingest)

Antes de qualquer outra coisa, verifique se `~/vbrain/.git/` existe:

```bash
test -d ~/vbrain/.git && echo present || echo absent
```

**Se ausente** (primeira ingestão da base), use `AskUserQuestion` para perguntar:

> "Sua base ainda não é um repositório git. Quero versionar a wiki/raw para
> você poder revertir/sincronizar entre máquinas. O que prefere?"
>
> 1. **Repo privado no GitHub** (Recommended)
> 2. **Repo público no GitHub**
> 3. **Só git local, sem GitHub**
> 4. **Pular versionamento por agora**

Conforme a resposta, rode:

- Privado: `bundle exec ruby scripts/init_repo.rb --github private`
- Público: `bundle exec ruby scripts/init_repo.rb --github public`
- Local: `bundle exec ruby scripts/init_repo.rb`
- Pular: não rode nada, e marque mentalmente que o passo 6 deve ser pulado.

Parse o JSON:

- `{"initialized":true,...,"remote_url":"..."}` → repo pronto, siga.
- `{"initialized":false,"reason":"already a repo"}` → idempotente, siga.
- `{"needs_gh":true}` → `gh` não instalado. Use `AskUserQuestion` perguntando
  se o usuário quer instalar (`brew install gh && gh auth login`). Se sim,
  rode os comandos e depois re-rode `init_repo.rb`. Se não, caia para "Só git
  local" (`init_repo.rb` sem `--github`).
- `{"needs_gh_auth":true}` → `gh` está instalado mas não autenticado.
  Instrua o usuário a rodar `gh auth login` num terminal interativo (você
  não consegue rodar comandos interativos), depois re-rode.

**Se presente**: siga direto para o passo 1.

### 1. Ingerir o raw

```bash
bundle exec ruby scripts/ingest_raw.rb <path>
```

Parse o JSON de saída. Possíveis casos:

- `{"source_type":"text"|"url"|"tweet","raw_id":N,"raw_path":...,"extracted_path":...}` → siga ao passo 2.
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
   "kind":"concept|decision|gotcha|note|rule",
   "tags":["..."],
   "raw_excerpt":"trecho literal",
   "summary_hint":"1 frase neutra"}
]}
```

Se `chunks` vier **vazio**, NÃO aborte ainda — siga o passo 2b.

### 2b. Fallback de extração (quando chunker retorna 0 chunks)

Significa que a extração determinística não rendeu conteúdo durável. Tente em
ordem, **parando ao primeiro que produzir chunks > 0**:

1. **Follow embedded links** (essencial para tweets que são só link de artigo):
   Abra `extracted_path` e procure por seção `## Links citados` ou `## Referências`
   com URLs, ou por uma URL dominante no body. Para cada URL encontrada (limite
   3 mais relevantes — priorize artigos/blogs sobre `t.co`/encurtadores já
   expandidos):
   - Rode `WebFetch` na URL com o prompt:
     > "Extraia o conteúdo principal do artigo em markdown limpo (título,
     > autor se houver, corpo integral preservando estrutura). Se for login
     > wall ou redirect pra Sign in, reporte explicitamente."
   - Concatene as respostas com headings `## <URL>` em
     `raw/.tmp/extracted-<raw_id>-followed.txt`.
   - Re-rode o chunker subagente com `prompts/chunker/text.md` (não mais
     `tweet.md` — agora é conteúdo de artigo) e o novo arquivo. Se gerar
     chunks, siga.

2. **Wayback Machine** (se WebFetch deu erro HTTP 4xx/5xx ou login wall):
   Para cada URL que falhou, tente `https://web.archive.org/web/<URL>` via
   `WebFetch`. Salve no mesmo `extracted-<raw_id>-followed.txt`. Re-rode o
   chunker.

3. **Pedir ao usuário** (último recurso): Use `AskUserQuestion` perguntando
   se quer:
   (a) Colar o conteúdo do artigo na próxima mensagem — você salva no
       `extracted-<raw_id>-followed.txt` e re-roda chunker.
   (b) Descartar a ingestão (passo 6 só commita o raw como audit log).
   (c) Tentar URL diferente.

4. **Honest abort**: se o usuário descartar, pule passos 3-5 mas siga o passo
   6 (commit do raw como audit; reporte explicitamente "nenhuma página criada").

### 3. Escrever wiki (subagente, **estritamente um chunk por vez**)

Processe os chunks **um de cada vez, em sequência — NUNCA em paralelo**. Cada
chunk percorre o ciclo completo **writer → persistir → reindexar** antes de o
próximo começar. É isso que faz o writer seguinte enxergar (e mesclar com) as
páginas que os anteriores criaram: ele navega o índice **já atualizado**.

> **Regra dura:** nunca lance dois writers na mesma mensagem. Um writer, um
> `write_pages`, um `reindex`, e só então o próximo chunk. Paralelizar quebra a
> navegação do grafo e pode fazer dois chunks atropelarem a mesma página.

Para **cada chunk**, na ordem, faça o ciclo:

1. **Writer** — lance UM subagente `general-purpose` (com Bash + Read) usando
   `prompts/wiki-writer.md` como system prompt. No user message passe: o chunk
   (JSON do chunker), o caminho absoluto da wiki (`<data_home>/wiki`), o
   `source_raw`, e o comando de navegação
   `bundle exec ruby scripts/query.rb "<termos>" --format json --limit 8`.
   O writer devolve UM JSON: `op` (`create`|`update`), `slug` (quando `update`),
   `title`, `tags`, `kind`, `body_markdown` (corpo **final completo**).
2. **Persistir** — escreva esse único page-object em
   `raw/.tmp/page-<raw_id>.json` (`{"pages":[<o objeto>]}`) e rode o passo 4.
3. **Reindexar** — rode o passo 4b (linkify) e o passo 5 (reindex) **agora**,
   pra que o próximo writer veja esta página no índice.

Só depois de reindexar, parta pro próximo chunk. Não acumule páginas pra um
`write_pages` em lote — o ciclo é por chunk.

### 4. Persistir a página (staging + publicação atômica)

```bash
bundle exec ruby scripts/write_pages.rb --raw-id <N> --pages-json raw/.tmp/page-<N>.json
```

O script encena a página (corpo inteiro) em `raw/.tmp/wiki-stage-<N>/` e só então
a move pra `wiki/` via rename — a wiki nunca fica num estado meio-escrito.

- `op: "create"` → arquivo novo (slug derivado do título; sufixo `-2` só em
  colisão real com outro assunto).
- `op: "update"` → sobrescreve o corpo inteiro da página existente (`slug`) e
  mescla o frontmatter (union de `tags`; `source_raw` acumula o raw novo). Se o
  `slug` não existir, cai pra `create` (defesa anti-alucinação).

A saída JSON tem `{"written":[...],"updated":[...],"count":N}`. Páginas de
conhecimento moram na **raiz** de `wiki/` (espaço plano, sem pastas por tipo).

### 4b. Linkificar (determinístico) + resolver não-resolvidos (LLM)

```bash
bundle exec ruby scripts/linkify.rb
```

Converte os `[[wikilinks]]` resolvíveis por slug exato em links markdown
`[Título](slug.md)` — **navegáveis no GitHub e no Obsidian** (o GitHub não
renderiza `[[ ]]` em arquivos de repo). Determinístico, idempotente, preserva
o frontmatter verbatim. Saída: `{"changed":N,"scanned":N}`.

Depois do `reindex.rb` (passo 5), os links que sobraram **não-resolvidos**
(slug do título não bate com nenhuma página) ficam na tabela `links` com
`to_page_id NULL`. Para fortalecer o grafo, uma **camada de julgamento (LLM)**
decide a qual página existente cada um se refere:

1. Liste os não-resolvidos e o índice de páginas:
   `SELECT DISTINCT target_title FROM links WHERE to_page_id IS NULL` +
   `SELECT path, title FROM pages`.
2. Lance um subagente que, para cada `target_title`, escolhe o `slug` da página
   existente que ele referencia (ou `null` se nenhuma servir — **não inventar**).
   Saída: JSON `{"Título": "slug" | null}`.
3. Aplique determinísticamente e reindexe:
   ```bash
   bundle exec ruby scripts/resolve_links.rb --map raw/.tmp/linkmap.json
   bundle exec ruby scripts/reindex.rb
   ```
   `resolve_links.rb` descarta slugs que não existem (defesa anti-alucinação).

### 5. Reindexar

```bash
bundle exec ruby scripts/reindex.rb
```

A saída tem `{"inserted":N,"updated":N,"deleted":N,"links":N}`. O índice é
puramente o SQLite (`pages` + virtual `pages_fts` + tabela `links`); triggers
AI/AD/AU sincronizam o FTS5 automaticamente. Além de reindexar o FTS, o
`reindex.rb` **parseia os `[[wikilinks]]` do body e reconstrói o grafo** na
tabela `links` (alvo inexistente → aresta com `to_page_id` NULL, resolvida num
reindex futuro). Não há `wiki/index.md` — espelhamos o ai-memory: a estrutura
é o grafo de links + o SQLite derivado, não uma árvore de pastas.

### 6. Commit + push

Se o passo 0 não foi pulado:

```bash
bundle exec ruby scripts/commit.rb --message "add: <N> páginas de <basename>"
```

Onde `<N>` é o `count` retornado em 4 e `<basename>` é o nome curto do
arquivo/URL original. O script é idempotente:

- Se não há mudanças no `~/vbrain/` (caso degenerado: chunker retornou 0
  páginas), retorna `{"committed":false,"reason":"no changes"}`.
- Se há mudanças, commita e tenta push automaticamente quando há `origin`
  configurado (`{"pushed":true}`).
- Sem remote, só commita local (`{"pushed":false,"reason":"no remote"}`).

Não interrompa o pipeline se o push falhar — o commit local foi feito; reporte
o erro e siga.

### 7. Reportar ao usuário

Mostre:
- Tipo detectado (`source_type`)
- Páginas **criadas** (`written`) e **atualizadas** (`updated`) em `wiki/`
- Estatísticas do banco: `bundle exec ruby scripts/stats.rb` (retorna JSON com
  total de páginas, distribuição por kind e 5 mais recentes)
- Status do commit/push (sha + remote URL quando aplicável)

## Regras duras

- **Nunca** escrever em `wiki/` diretamente; sempre via `write_pages.rb`.
- **Nunca** modificar `raw/` depois de ingerido — é imutável.
- Se `rake test` falhar antes da execução final (caso 1 do tipo desconhecido),
  **não** prosseguir até o usuário consertar.
- **FAITHFULNESS**: os subagentes têm regra dura de não inventar; se eles
  retornarem dados claramente fabricados, sinalize ao usuário.
- **Tweet com link para artigo → seguir o link**: quando o tweet ingerido tem
  pouco/nenhum texto próprio mas referencia uma URL externa (artigo, blog,
  thread em outro site), o usuário quer **o conteúdo do artigo**, não só o
  metadado do tweet. O passo 2b é obrigatório nesse caso — não termine "sem
  páginas" se há link a seguir.
- **Tentativas de fallback são ordenadas e exaustivas**: WebFetch direto →
  Wayback Machine → pedir manual ao usuário. Nunca pular pra "abort" sem
  exercitar a chain.
