---
name: vbrain-add-realtime-knowledge
description: Conecta uma fonte de "conhecimento realtime" ao vbrain (hoje suporta Google Calendar via MCP). Cria uma página fantasma na wiki/_realtime/ com kind=realtime que casa no FTS5 e dispara handler ao vivo no /vbrain-query-knowledge. Use quando o usuário pedir "conecta meu gcalendar", "adiciona meu calendário", "quero buscar agenda junto", ou "vbrain-add-realtime-knowledge".
allowed-tools: Bash, Read, AskUserQuestion, mcp__claude_ai_Google_Calendar__list_calendars
---

# vbrain-add-realtime-knowledge

Registra uma fonte realtime no vbrain. O modelo: a wiki tem uma **página
fantasma** por fonte (em `wiki/_realtime/<source>.md`, kind `realtime`) com
keywords óbvias no body. Ela é indexada normalmente no FTS5; quando casa em
uma query, o `/vbrain-query-knowledge` dispara o handler MCP correspondente
ao vivo em vez de devolver o body.

## Inputs

- **source** (opcional): qual fonte ativar. Hoje suportado: `gcalendar`.
  Se ausente, pergunte ao usuário via `AskUserQuestion`.

## Fontes suportadas

| `source`    | Status      | Script determinístico                    |
|---|---|---|
| `gcalendar` | suportado   | `scripts/add_realtime/gcalendar.rb`      |
| `slack`     | futuro      | (não implementado)                       |
| outro       | improvise   | pergunte ao usuário detalhes da conexão  |

## Passos

### 1. Determinar a fonte

Se o usuário não passou `source`, use `AskUserQuestion`:

> "Qual fonte realtime você quer conectar ao vbrain?"
> 1. Google Calendar (`gcalendar`)
> 2. Outra (descreva)

Se a resposta for "outra" ou um valor não suportado, peça ao usuário pra
descrever a fonte. Pergunte se ele quer que você crie uma fonte
determinística (com script Ruby e teste) ou apenas uma página fantasma
"manual" agora. Em caso de "manual", você pode escrever
`wiki/_realtime/<slug>.md` diretamente com `kind: realtime` e os campos que
fizerem sentido — mas avise que isso é uma fonte one-shot sem handler ao
vivo, então só serve pra documentar.

### 2. Fluxo `gcalendar`

**2a. Listar calendários disponíveis** via MCP:

```
mcp__claude_ai_Google_Calendar__list_calendars
```

O retorno típico traz `id`, `summary`, `timeZone` (campos opcionais como
`primary`, `accessRole`, `backgroundColor` podem aparecer dependendo da
versão do MCP — não dependa deles). **Filtros que sempre aplique**, mesmo
sem `AskUserQuestion`:

- Excluir calendários genéricos do Google: `id` casando
  `holiday@group.v.calendar.google.com`,
  `*#holiday@group.v.calendar.google.com`, `addressbook#contacts@group.v.calendar.google.com`,
  ou `summary` igual a "Birthdays" / "Aniversários".
- Manter o resto como candidatos.

**2b. Perguntar ao usuário quais conectar.** Tente `AskUserQuestion` com
`multiSelect: true` listando os candidatos (label = `summary` ou `id`,
description = `id` curto + `timeZone`).

**Fallback se `AskUserQuestion` não estiver disponível** (sessões de agente
sem o tool deferred): conecte **todos** os candidatos por default e avise
explicitamente ao usuário no relatório final: "Conectei todos os calendários
visíveis exceto os genéricos do Google. Pra refinar, rode
`/vbrain-add-realtime-knowledge gcalendar` de novo numa sessão interativa
ou edite manualmente `~/vbrain/config/realtime/gcalendar.yml` e rode
`scripts/reindex.rb`."

**2c. Montar JSON e rodar o script Ruby:**

```bash
BUNDLE_GEMFILE=/Users/victorcampos/Workspace/vbrain/Gemfile bundle exec ruby /Users/victorcampos/Workspace/vbrain/scripts/add_realtime/gcalendar.rb --calendars-json '<JSON>'
```

Onde `<JSON>` é uma string JSON com a chave `calendars`, cada item
`{"id": "...", "summary": "...", "timezone": "..."}`. Exemplo:

```json
{"calendars":[
  {"id":"primary","summary":"Victor","timezone":"America/Sao_Paulo"},
  {"id":"work@v360.io","summary":"V360 Work","timezone":"America/Sao_Paulo"}
]}
```

O script:
1. Grava a config em `~/vbrain/config/realtime/gcalendar.yml`.
2. Escreve a página fantasma em `~/vbrain/wiki/_realtime/gcalendar.md`.
3. Retorna JSON com `config_path`, `wiki_path` e `calendars`.

**2d. Reindexar** pra a página fantasma entrar no FTS5:

```bash
BUNDLE_GEMFILE=/Users/victorcampos/Workspace/vbrain/Gemfile bundle exec ruby /Users/victorcampos/Workspace/vbrain/scripts/reindex.rb
```

**2e. Commit (se houver repo git no `~/vbrain`):**

```bash
BUNDLE_GEMFILE=/Users/victorcampos/Workspace/vbrain/Gemfile bundle exec ruby /Users/victorcampos/Workspace/vbrain/scripts/commit.rb --message "realtime: conecta gcalendar (<N> calendários)"
```

Onde `<N>` é o número de calendários conectados.

### 3. Reportar

Mostre ao usuário:
- Lista de calendários conectados (summary + id curto)
- Path da config (`~/vbrain/config/realtime/gcalendar.yml`)
- Path da página fantasma (`wiki/_realtime/gcalendar.md`)
- Próximo passo: "agora pergunte algo como 'tenho reunião amanhã?' no
  `/vbrain-query-knowledge` — vou puxar ao vivo."

## Regras

- **Nunca** chame o MCP do Google Calendar **diretamente** nessa skill pra
  buscar eventos. Aqui é só configuração (listar calendários disponíveis +
  perguntar quais conectar). A busca de eventos é responsabilidade do
  `/vbrain-query-knowledge`.
- **Nunca** escreva em `wiki/_realtime/` na mão pra fontes suportadas
  (gcalendar/slack): sempre vai pelo script Ruby. Pra fontes "outra" sem
  script, escrever direto é OK mas avise o usuário que sem handler não vai
  resolver ao vivo.
- Se o MCP `list_calendars` falhar (não conectado, sem permissão), pare e
  oriente o usuário a conectar o Google Calendar à conta Claude antes de
  re-rodar.
