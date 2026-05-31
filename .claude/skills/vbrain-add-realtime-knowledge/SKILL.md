---
name: vbrain-add-realtime-knowledge
description: Conecta uma fonte de "conhecimento realtime" ao vbrain (hoje: Google Calendar, Gmail e Slack via MCP). Cria uma página fantasma na wiki/_realtime/ com kind=realtime que casa no FTS5 e dispara handler ao vivo no /vbrain-query-knowledge. Use quando o usuário pedir "conecta meu gcalendar", "conecta meu gmail", "conecta meu slack", "adiciona meu calendário", "quero buscar agenda/email/slack junto", ou "vbrain-add-realtime-knowledge".
allowed-tools: Bash, Read, AskUserQuestion, mcp__claude_ai_Google_Calendar__list_calendars, mcp__claude_ai_Gmail__list_labels, mcp__claude_ai_Slack__slack_search_channels
---

# vbrain-add-realtime-knowledge

Registra uma fonte realtime no vbrain. O modelo: a wiki tem uma **página
fantasma** por fonte (em `wiki/_realtime/<source>.md`, kind `realtime`) com
keywords óbvias no body. Ela é indexada normalmente no FTS5; quando casa em
uma query, o `/vbrain-query-knowledge` dispara o handler MCP correspondente
ao vivo em vez de devolver o body.

## Inputs

- **source** (opcional): qual fonte ativar. Hoje suportado: `gcalendar`, `gmail`,
  `slack`. Se ausente, pergunte ao usuário via `AskUserQuestion`.

## Fontes suportadas

| `source`    | Status      | Script determinístico                    |
|---|---|---|
| `gcalendar` | suportado   | `vbrain realtime gcalendar`      |
| `gmail`     | suportado   | `vbrain realtime gmail`          |
| `slack`     | suportado   | `vbrain realtime slack`          |
| outro       | improvise   | pergunte ao usuário detalhes da conexão  |

## Passos

### 1. Determinar a fonte

Se o usuário não passou `source`, use `AskUserQuestion`:

> "Qual fonte realtime você quer conectar ao vbrain?"
> 1. Google Calendar (`gcalendar`)
> 2. Gmail (`gmail`)
> 3. Slack (`slack`)
> 4. Outra (descreva)

Se a resposta for "outra" ou um valor não suportado, peça ao usuário pra
descrever a fonte. Pergunte se ele quer que você crie uma fonte
determinística (com binário vbrain e teste) ou apenas uma página fantasma
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
`vbrain reindex`."

**2c. Montar JSON e rodar o binário vbrain:**

```bash
vbrain realtime gcalendar --json '<JSON>'
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
vbrain reindex
```

**2e. Commit (se houver repo git no `~/vbrain`):**

```bash
vbrain commit --message "realtime: conecta gcalendar (<N> calendários)"
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
> chave `labels`) e rode `vbrain reindex`, ou rode
> `/vbrain-add-realtime-knowledge gmail` numa sessão interativa."

**2bis-d. Montar JSON e rodar o binário vbrain:**

```bash
vbrain realtime gmail --json '<JSON>'
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

### 2ter. Fluxo `slack`

O Slack é diferente das outras fontes em dois pontos que ditam o fluxo:

1. **Não dá pra "listar tudo"**: `slack_search_channels` exige um termo de
   busca — não existe enumeração completa de canais como em calendars/labels.
2. **O search do Slack não tem `OR`** (espaço = AND). Por isso o filtro
   multi-canal não vira uma query só; o handler do query-knowledge faz **uma
   busca por canal** e mescla (ver `/vbrain-query-knowledge`, seção 3c).

Por causa disso, a config aceita **lista vazia de canais = busca global** no
workspace inteiro (todos os canais/DMs acessíveis). Lista preenchida = busca
filtrada por esses canais.

**2ter-a. Verificar autenticação do MCP.** Tente
`mcp__claude_ai_Slack__slack_search_channels` com um termo qualquer (ex.:
`query: "general"`, `limit: 1`). Se o retorno for tipo "ask the user to run
/mcp and select 'claude.ai Slack' to authenticate", o MCP **não está
autenticado** — pare e instrua:
> "Pra conectar o Slack, abre `/mcp` no Claude Code, seleciona
> **claude.ai Slack** e autoriza no navegador. Quando voltar, me chama de
> novo com `/vbrain-add-realtime-knowledge slack`."
Não tente bypassar.

**2ter-b. Perguntar o escopo** via `AskUserQuestion` (single select):
> "Quais canais do Slack conectar?"
> 1. Todos (workspace inteiro) — busca global, sem filtro de canal.
> 2. Canais específicos — você me diz os nomes.

- Se **Todos**: `channels = []` (modo global). Pule pra 2ter-d.
- Se **Canais específicos**: pergunte os nomes dos canais (texto livre, ex.:
  "geral, produto, anúncios"). Para cada nome, chame
  `mcp__claude_ai_Slack__slack_search_channels` com `query: "<nome>"`,
  `channel_types: "public_channel,private_channel"`. Pegue o melhor match
  (`id` + `name`). Se um nome não resolver, avise o usuário e siga com os que
  resolveram. Monte a lista `channels` com `{id, name}`.

**Fallback se `AskUserQuestion` não estiver disponível**: conecte em modo
**global** (`channels = []`) e avise:
> "Conectei o Slack em modo global (busca no workspace inteiro). Pra
> restringir a canais, rode `/vbrain-add-realtime-knowledge slack` numa
> sessão interativa ou edite `~/vbrain/config/realtime/slack.yml` (adicione
> objetos `{id, name}` à chave `channels`) e rode `vbrain reindex`."

**2ter-c. Montar JSON e rodar o binário vbrain:**

```bash
vbrain realtime slack --json '<JSON>'
```

Onde `<JSON>` é uma string JSON com a chave `channels`, cada item
`{"id": "...", "name": "..."}`. Para modo global, passe `{"channels":[]}`.
Exemplos:

```json
{"channels":[]}
```
```json
{"channels":[
  {"id":"C0GERAL","name":"geral"},
  {"id":"C0PROD","name":"produto"}
]}
```

O script grava `~/vbrain/config/realtime/slack.yml` + página fantasma em
`~/vbrain/wiki/_realtime/slack.md` e retorna JSON com `mode`
(`global`|`filtered`), `config_path`, `wiki_path` e `channels`.

**2ter-d. Reindexar e commitar** (mesmos comandos da seção gcalendar, troque
a mensagem do commit por `"realtime: conecta slack (<modo>)"`, onde `<modo>`
é "global" ou "<N> canais").

### 3. Reportar

Mostre ao usuário:
- Lista de calendários/labels/canais conectados (ou "modo global" pro slack)
- Path da config (`~/vbrain/config/realtime/<source>.yml`)
- Path da página fantasma (`wiki/_realtime/<source>.md`)
- Próximo passo: "agora pergunte algo como 'tenho reunião amanhã?' (gcalendar),
  'algum email do cliente X esta semana?' (gmail) ou 'o que falaram no slack
  sobre o deploy?' (slack) no `/vbrain-query-knowledge` — vou puxar ao vivo."

## Regras

- **Nunca** busque dados (eventos, emails) durante essa skill — ela é só
  configuração. Listar calendários/labels é OK; buscar eventos/threads é
  responsabilidade do `/vbrain-query-knowledge`.
- **Nunca** escreva em `wiki/_realtime/` na mão pra fontes suportadas
  (gcalendar/gmail/slack): sempre vai pelo binário vbrain. Pra fontes "outra" sem
  script, escrever direto é OK mas avise o usuário que sem handler não vai
  resolver ao vivo.
- Se o MCP da fonte falhar (não conectado, sem permissão), pare e oriente
  o usuário a abrir `/mcp` no Claude Code pra autorizar antes de re-rodar.
  Não tente bypassar a autenticação.
