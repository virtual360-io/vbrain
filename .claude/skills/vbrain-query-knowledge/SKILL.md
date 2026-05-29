---
name: vbrain-query-knowledge
description: Consulta a base vbrain (SQLite FTS5) e devolve trechos relevantes em markdown. Páginas com kind=realtime disparam handlers ao vivo (Google Calendar e Gmail via MCP) em vez de retornar o body. Use quando o usuário perguntar algo que pode estar arquivado ("o que eu sei sobre X", "procura no vbrain por Y"), ou quando outro agente precisar de contexto persistido para uma tarefa.
allowed-tools: Bash, Read, mcp__claude_ai_Google_Calendar__list_events, mcp__claude_ai_Gmail__search_threads, mcp__claude_ai_Gmail__get_thread
---

# vbrain-query-knowledge

Skill de leitura: roda `scripts/query.rb` contra o índice FTS5, formata o
resultado, e para páginas com `kind: realtime` dispara o handler MCP
correspondente (resolve ao vivo).

## Inputs

- **query** (obrigatório): pergunta livre ou keyword. Pode conter `:`, aspas,
  parênteses — o normalizador Ruby cuida disso.
- **limit** (opcional, default 10): número máximo de páginas a retornar.

## Passos

### 0. Expansão da query (ponte NL → vocabulário) — **só quando a query é linguagem natural**

O FTS5 é lexical: a pergunta `"quais empregos eu já tive"` não casa nada
porque a palavra "emprego" não está no corpus (as páginas dizem "Visagio",
"consultor", "carreira"). O Ruby já remove stopwords, mas não inventa
sinônimos — esse é o seu julgamento.

Pule este passo se a `query` já é keyword(s) (1–3 termos técnicos, um nome
próprio, um slug). Faça-o quando for pergunta em linguagem natural.

1. Puxe o vocabulário real de tags da base:

```bash
BUNDLE_GEMFILE=/Users/victorcampos/Workspace/vbrain/Gemfile bundle exec ruby /Users/victorcampos/Workspace/vbrain/scripts/tags.rb --limit 60 --format json
```

2. Reescreva a pergunta num punhado de **termos de conteúdo** (4–8), enviesado
   pelo vocabulário acima. Inclua sinônimos/forma flexionada e, quando houver,
   a(s) tag(s) que casam a intenção. Ex.: `"quais empregos eu já tive"` →
   `carreira trabalho consultor estagiário empresa cargo`. Não inclua
   stopwords nem a própria palavra da pergunta se ela não existe no corpus.
   Não invente nomes próprios que você não viu.
3. Use esses termos como `<query>` nos passos 1–2, e **sempre** passe a
   pergunta original em `--source-query` (o `query_log` guarda as duas — é o
   que a rotina `dream` analisa pra reorganizar a wiki).

### 1. FTS5

```bash
BUNDLE_GEMFILE=/Users/victorcampos/Workspace/vbrain/Gemfile bundle exec ruby /Users/victorcampos/Workspace/vbrain/scripts/query.rb "<query>" --limit <N> --source-query "<pergunta original>" --format json
```

Parseie `results`. Cada item tem `path`, `title`, `kind`, `snippet`.
Se `results` vier vazio, tente o passo 2 (prefix). Caso contrário pule pro 3.

(Se você não expandiu — query já era keyword — passe a própria query em
`--source-query` também, ou omita: o log usa a `query` nesse caso.)

### 2. Fallback com prefix matching

```bash
BUNDLE_GEMFILE=/Users/victorcampos/Workspace/vbrain/Gemfile bundle exec ruby /Users/victorcampos/Workspace/vbrain/scripts/query.rb "<query>" --limit <N> --prefix --no-log --format json
```

(`--no-log` aqui: o passo 1 já registrou a intenção; o retry com prefix não
deve duplicar a linha no `query_log`.)

Se ainda vazio: "Nenhum resultado encontrado para `<query>` na base vbrain.
Tente termos mais gerais ou verifique se algo já foi ingerido com
`/vbrain-add-knowledge`."

### 3. Dispatch de páginas realtime

Para cada resultado com `kind == "realtime"`, **não** mostre o snippet:
chame o handler ao vivo correspondente. Leia o frontmatter completo da
página em `~/vbrain/wiki/<path>` pra descobrir `source` e parâmetros.

| `source`    | Handler                                                      |
|---|---|
| `gcalendar` | `mcp__claude_ai_Google_Calendar__list_events` (ver 3a)       |
| `gmail`     | `mcp__claude_ai_Gmail__search_threads` (ver 3b)              |
| outro       | reporte "fonte realtime `X` não tem handler implementado"    |

**3a. Handler `gcalendar`:**

Leia o frontmatter da página, pegue a lista `calendars` (cada um com `id`,
`summary`, `timezone`). Para cada calendário, chame:

```
mcp__claude_ai_Google_Calendar__list_events
  calendar_id = <id>
  time_min    = <início do intervalo>
  time_max    = <fim do intervalo>
```

Intervalo derivado da query (use a data atual = hoje):

| Termo na query                    | Intervalo                           |
|---|---|
| "hoje", "today"                   | 00:00 → 23:59 de hoje               |
| "amanhã", "tomorrow"              | 00:00 → 23:59 de amanhã             |
| "essa/esta semana", "this week"   | hoje → domingo desta semana 23:59   |
| "próxima semana", "next week"     | seg da próxima → dom da próxima     |
| "mês", "this month"               | hoje → fim do mês corrente          |
| "fim de semana", "weekend"        | sáb 00:00 → dom 23:59 mais próximos |
| nenhum termo temporal explícito   | próximos 7 dias (hoje → +7d)        |

Use o timezone do calendário (ou `America/Sao_Paulo` como fallback) ao
montar o ISO 8601 do `time_min/time_max`.

Para cada evento retornado, formate:

```
- HH:MM–HH:MM | <summary> [<calendar.summary>]
  <location, se houver>
  <description curta, se houver>
```

Se a query menciona alguém específico (ex.: "reunião com Fulano"), filtre
eventos cujo `summary`, `description` ou `attendees` contenham o nome.

**3b. Handler `gmail`:**

Leia o frontmatter da página, pegue a lista `labels` (cada um com `id`,
`name`). Monte o `query` do `search_threads` em **Gmail search syntax**:

1. **Label filter** (sempre prepende): `(label:<id1> OR label:<id2> …)`.
   Para 1 label só, use `label:<id>` sem parênteses. Se a lista de labels
   estiver vazia (degenerado), não prepende.
2. **Conteúdo**: extraia os termos significativos da query do usuário e
   converta pra Gmail syntax:
   - Nomes/e-mails → `from:` ou `to:` se a query disser quem mandou/recebeu.
     ("email do João" → `from:João`; sem direção, tente
     `(from:João OR to:João)`.)
   - Datas relativas:
     - "hoje" → `newer_than:1d`
     - "ontem" → `newer_than:2d older_than:1d`
     - "essa/esta semana" → `newer_than:7d`
     - "semana passada" → `newer_than:14d older_than:7d`
     - "esse mês" → `newer_than:30d`
   - Datas absolutas → `after:YYYY/MM/DD before:YYYY/MM/DD`.
   - Anexos → `has:attachment`.
   - Não lido → `is:unread`.
   - Assunto explícito → `subject:"<frase>"`.
   - Palavras-chave restantes vão soltas (são AND por default).
3. Chame:

```
mcp__claude_ai_Gmail__search_threads
  query    = "<label filter> <conteúdo>"
  pageSize = min(20, limit do query-knowledge)
```

Para cada thread retornada, formate:

```
- <data curta> | <from> → <subject>
  <snippet>
```

Use o último message da thread como `from`/`subject`/`snippet` (a resposta
já vem com os campos). Se a thread tem várias mensagens, mostre o número
entre parênteses: `(N msgs)`.

Se nenhum result voltar, reporte: "Nenhuma thread bate `<query montada>`.
Tente termos mais gerais ou amplie o range temporal."

Se o usuário pedir o corpo completo de uma thread específica
("abre essa", "mostra o último email completo"), chame
`mcp__claude_ai_Gmail__get_thread` com o `threadId` e
`messageFormat: FULL_CONTENT`.

### 4. Formatar resposta

Mantenha a **ordem do FTS5** (rank do SQLite): se a página realtime caiu em
3º, mostre os 2 estáticos antes, depois o bloco realtime, depois o resto.

Para resultados estáticos:
- Título + path `wiki/<path>`
- Snippet (já vem com `**termo**` destacado)
- Tags se relevante

Para resultados realtime:
- Cabeçalho: `## <Título> (realtime — <source>)`
- Bloco renderizado pelo handler

Quando o caller é outro agente (heurística: pergunta veio de uma `Task`),
leia o arquivo inteiro dos top 3 estáticos e inclua o body markdown.

## Regras

- **Não modifique** nada — esta skill é read-only.
- Se a query tem `< 3 caracteres` significativos, peça uma query mais
  específica antes de rodar.
- Não invente conteúdo: snippet ruim → "essas páginas mencionam o termo mas
  talvez não respondam diretamente".
- Para realtime, se o MCP falhar (não conectado, sem permissão), reporte
  explicitamente: "não consegui consultar `<source>` ao vivo: <erro>; reconecte
  via `/vbrain-add-realtime-knowledge`". Nunca caia para snippet do body —
  o body só tem keywords, não é resposta.
