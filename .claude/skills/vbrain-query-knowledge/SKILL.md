---
name: vbrain-query-knowledge
description: Consulta a base vbrain (SQLite FTS5) e devolve trechos relevantes em markdown. Páginas com kind=realtime disparam handlers ao vivo (Google Calendar via MCP) em vez de retornar o body. Use quando o usuário perguntar algo que pode estar arquivado ("o que eu sei sobre X", "procura no vbrain por Y"), ou quando outro agente precisar de contexto persistido para uma tarefa.
allowed-tools: Bash, Read, mcp__claude_ai_Google_Calendar__list_events
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

### 1. FTS5

```bash
BUNDLE_GEMFILE=/Users/victorcampos/Workspace/vbrain/Gemfile bundle exec ruby /Users/victorcampos/Workspace/vbrain/scripts/query.rb "<query>" --limit <N> --format json
```

Parseie `results`. Cada item tem `path`, `title`, `kind`, `snippet`.
Se `results` vier vazio, tente o passo 2 (prefix). Caso contrário pule pro 3.

### 2. Fallback com prefix matching

```bash
BUNDLE_GEMFILE=/Users/victorcampos/Workspace/vbrain/Gemfile bundle exec ruby /Users/victorcampos/Workspace/vbrain/scripts/query.rb "<query>" --limit <N> --prefix --format json
```

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
