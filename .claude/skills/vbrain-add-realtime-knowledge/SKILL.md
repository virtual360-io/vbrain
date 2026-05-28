---
name: vbrain-add-realtime-knowledge
description: Conecta uma fonte de "conhecimento realtime" ao vbrain (hoje: Google Calendar e Gmail via MCP). Cria uma página fantasma na wiki/_realtime/ com kind=realtime que casa no FTS5 e dispara handler ao vivo no /vbrain-query-knowledge. Use quando o usuário pedir "conecta meu gcalendar", "conecta meu gmail", "adiciona meu calendário", "quero buscar agenda/email junto", ou "vbrain-add-realtime-knowledge".
allowed-tools: Bash, Read, AskUserQuestion, mcp__claude_ai_Google_Calendar__list_calendars, mcp__claude_ai_Gmail__list_labels
---

# vbrain-add-realtime-knowledge

Registra uma fonte realtime no vbrain. O modelo: a wiki tem uma **página
fantasma** por fonte (em `wiki/_realtime/<source>.md`, kind `realtime`) com
keywords óbvias no body. Ela é indexada normalmente no FTS5; quando casa em
uma query, o `/vbrain-query-knowledge` dispara o handler MCP correspondente
ao vivo em vez de devolver o body.

## Inputs

- **source** (opcional): qual fonte ativar. Hoje suportado: `gcalendar`, `gmail`.
  Se ausente, pergunte ao usuário via `AskUserQuestion`.

## Fontes suportadas

| `source`    | Status      | Script determinístico                    |
|---|---|---|
| `gcalendar` | suportado   | `scripts/add_realtime/gcalendar.rb`      |
| `gmail`     | suportado   | `scripts/add_realtime/gmail.rb`          |
| `slack`     | futuro      | (não implementado)                       |
| outro       | improvise   | pergunte ao usuário detalhes da conexão  |

## Passos

### 1. Determinar a fonte

Se o usuário não passou `source`, use `AskUserQuestion`:

> "Qual fonte realtime você quer conectar ao vbrain?"
> 1. Google Calendar (`gcalendar`)
> 2. Gmail (`gmail`)
> 3. Outra (descreva)

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

### 2bis. Fluxo `gmail`

**2bis-a. Verificar autenticação do MCP.** Tente `mcp__claude_ai_Gmail__list_labels`.
Possíveis retornos:

- JSON com `labels: [...]` → MCP autenticado. Siga pra 2bis-b.
- Resposta tipo "ask the user to run /mcp and select 'claude.ai Gmail' to
  authenticate" → MCP **não autenticado**. Pare e instrua o usuário:
  > "Pra conectar o Gmail, abre `/mcp` no Claude Code, seleciona
  > **claude.ai Gmail** e autoriza no navegador. Quando voltar, me chama
  > de novo com `/vbrain-add-realtime-knowledge gmail`."
  Não tente bypassar.

**2bis-b. Listar labels** via `mcp__claude_ai_Gmail__list_labels`.
O retorno traz só **user labels** (`labelId` + `name`). System labels NÃO
aparecem mas existem com IDs bem-conhecidos: `INBOX`, `IMPORTANT`, `STARRED`,
`UNREAD`, `SENT`, `DRAFT`, `SPAM`, `TRASH`, `CHAT`.

**2bis-c. Perguntar quais labels conectar.** Tente `AskUserQuestion` com
`multiSelect: true` listando: as 3 system labels relevantes (`INBOX`,
`IMPORTANT`, `STARRED`) + todos os user labels retornados. Label = `name`
(ou ID para system), description = "system" ou "user".

**Fallback se `AskUserQuestion` não estiver disponível**: conecte só
`INBOX` + `IMPORTANT` por default e avise:
> "Conectei INBOX + IMPORTANT. Pra refinar, edite
> `~/vbrain/config/realtime/gmail.yml` (adicione objetos `{id, name}` à
> chave `labels`) e rode `scripts/reindex.rb`, ou rode
> `/vbrain-add-realtime-knowledge gmail` numa sessão interativa."

**2bis-d. Montar JSON e rodar o script Ruby:**

```bash
BUNDLE_GEMFILE=/Users/victorcampos/Workspace/vbrain/Gemfile bundle exec ruby /Users/victorcampos/Workspace/vbrain/scripts/add_realtime/gmail.rb --labels-json '<JSON>'
```

Onde `<JSON>` é uma string JSON com a chave `labels`, cada item
`{"id": "...", "name": "..."}`. Exemplo:

```json
{"labels":[
  {"id":"INBOX","name":"Inbox"},
  {"id":"IMPORTANT","name":"Important"},
  {"id":"Label_5","name":"JCA"}
]}
```

O script grava `~/vbrain/config/realtime/gmail.yml` + página fantasma em
`~/vbrain/wiki/_realtime/gmail.md`.

**2bis-e. Reindexar e commitar** (mesmos comandos da seção gcalendar, troque
a mensagem do commit por `"realtime: conecta gmail (<N> labels)"`).

### 3. Reportar

Mostre ao usuário:
- Lista de calendários/labels conectados
- Path da config (`~/vbrain/config/realtime/<source>.yml`)
- Path da página fantasma (`wiki/_realtime/<source>.md`)
- Próximo passo: "agora pergunte algo como 'tenho reunião amanhã?' (gcalendar)
  ou 'algum email do cliente X esta semana?' (gmail) no
  `/vbrain-query-knowledge` — vou puxar ao vivo."

## Regras

- **Nunca** busque dados (eventos, emails) durante essa skill — ela é só
  configuração. Listar calendários/labels é OK; buscar eventos/threads é
  responsabilidade do `/vbrain-query-knowledge`.
- **Nunca** escreva em `wiki/_realtime/` na mão pra fontes suportadas
  (gcalendar/gmail): sempre vai pelo script Ruby. Pra fontes "outra" sem
  script, escrever direto é OK mas avise o usuário que sem handler não vai
  resolver ao vivo.
- Se o MCP da fonte falhar (não conectado, sem permissão), pare e oriente
  o usuário a abrir `/mcp` no Claude Code pra autorizar antes de re-rodar.
  Não tente bypassar a autenticação.
