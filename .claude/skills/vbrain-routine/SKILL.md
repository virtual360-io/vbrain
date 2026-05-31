---
name: vbrain-routine
description: Watch loop das rotinas do vbrain. Verifica em ~/vbrain/config/routines/routines.yml quais rotinas têm next_run vencido, dispara sub-agente paralelo pra cada, e a próxima execução é calculada deterministicamente pelo cron (fugit). Se chamado sem args, esse é o comportamento padrão (watch). Use quando o usuário pedir "roda minhas rotinas", "vbrain-routine", "executa rotina morning-brief", "fica rodando em background", ou referenciar uma rotina pelo slug.
allowed-tools: Bash, Read, Agent, AskUserQuestion, Skill, CronList
---

# vbrain-routine

Loop de execução das rotinas. **Watch é o default**: sem args, esta skill
roda um "tick" (claim de rotinas vencidas + dispatch + atualização de
`next_run`) e garante que o `/loop` global esteja registrado pra rearmar
sozinho a cada 15 minutos.

## Inputs (formas aceitas)

- **(vazio)** → **tick + watch**: identifica rotinas com `next_run <= now`,
  dispara sub-agente paralelo pra cada, deixa o `/loop 15m /vbrain-routine`
  rodando. Idempotente.
- **`<slug>`** → executa só essa rotina **agora** (manual trigger), sem
  alterar `next_run` nem `last_run`.
- **`status`** → lista todas com slug, schedule, próximo run, último run,
  enabled. Não dispara nada.

## Passos (modo default — watch)

### 1. Tick: claim de rotinas vencidas

```bash
vbrain routines
```

Esse script é **determinístico** e atômico:
- Lê `~/vbrain/config/routines/routines.yml`.
- Identifica rotinas com `enabled: true`, `schedule != null`, e
  `next_run <= now`.
- Para cada uma: marca `last_run = now`, avança `next_run` para o próximo
  tick do cron via fugit, escreve YAML de volta atomicamente.
- Retorna JSON com `due: [{slug, description, prompt}, ...]`.

Semântica: **at-most-once**. Se o sub-agente falhar, aquele run é perdido
(não re-tentamos no próximo tick). Pra mission-critical, o usuário pode
re-disparar manualmente com `/vbrain-routine <slug>`.

### 2. Dispatch dos sub-agentes (em paralelo)

Para cada item em `due`, lance um `Agent` em **uma única mensagem** com
múltiplos tool_use blocks:

- `subagent_type: "claude"` (precisa de `Tools: *` pra invocar outras
  skills/MCPs).
- `description`: o `slug` da rotina.
- `prompt`:

```
Você está executando a rotina vbrain "<SLUG>": <DESCRIPTION>

Instrução:

<PROMPT>

Quando terminar, devolva um único bloco markdown auto-contido com o
resultado (sem prefixos do tipo "aqui está"). Se a instrução chamar
uma skill (slash command), invoque via `Skill` tool. Se chamar uma
ferramenta MCP (qualquer `mcp__*`), invoque direto — não enumere as
disponíveis; use as que sua sessão tiver carregadas. Datas relativas
como "hoje" ou "essa semana" são em relação ao momento da execução.
```

### 3. Garantir o /loop ativo (com guarda anti-recursão)

**CRÍTICO**: o `/loop` quando chamado executa o prompt **imediatamente**
além de agendar o cron. Se essa skill chamar `/loop /vbrain-routine` sem
guarda, entra em recursão infinita (loop chama vbrain-routine que chama
loop que chama vbrain-routine…).

Sempre cheque PRIMEIRO via `CronList` se já existe job recurring com
prompt `/vbrain-routine`. Pseudocódigo:

```
crons = CronList()
already_active = crons.any? { |c| c.recurring && c.prompt =~ %r{^/vbrain-routine\b} }

if already_active:
  # skip — o cron já existente vai disparar o próximo tick a cada 15min
else:
  Skill(skill: "loop", args: "15m /vbrain-routine")
```

A primeira invocação manual de `/vbrain-routine` (ou de
`/vbrain-add-routine` que termina invocando esta) entra no ramo `else`
e registra o cron. As invocações subsequentes (disparadas pelo próprio
cron firing) entram no ramo `if` e pulam — sem recursão.

**Granularidade**: como o tick acontece a cada 15min, esse é o piso da
detecção. Rotinas com cron mais agressivo (`*/5 * * * *`) sofrem atraso
de até 15min — o sub-agente vai disparar no próximo tick que vencer o
`next_run`. Pra rotinas mission-critical sub-15min, o usuário pode rodar
`/loop 5m /vbrain-routine` manualmente (e cancelar o de 15min via
`CronDelete`).
Se já estiver ativo, `/loop` provavelmente recusa ou substitui — siga o
feedback dele e **não pare** o fluxo por isso (loop pode já estar
funcionando).

### 4. Reportar

Mostre:

```
# Rotinas executadas (N)

> tick @ <now ISO8601 UTC>
> próximo tick automático em 15m via /loop

## <slug 1> — <description 1>

<output do sub-agente 1>

---

## <slug 2> — <description 2>

<output do sub-agente 2>

---

…
```

Se `due_count == 0`, só reporte:

```
# Tick @ <now ISO8601 UTC>: nenhuma rotina vencida.

Próximas:
- <slug 1>: <next_run 1>
- <slug 2>: <next_run 2>
```

(Use `vbrain routine-list` pra pegar `next_run` se precisar de detalhes.)

## Passos (modo `<slug>` — manual trigger)

1. `vbrain routine-list --slug <slug>`
   pra recuperar o entry.
2. Se `count == 0`, reporte "rotina `<slug>` não existe" + sugira
   `/vbrain-add-routine`.
3. Lance **um** `Agent` com o mesmo template do passo 2 acima.
4. Reporte o output. **Não chame** `vbrain routines` — manual trigger
   não altera `next_run`/`last_run`.

## Passos (modo `status`)

```bash
vbrain routine-list
```

Tabela:
```
| slug | schedule | next_run | last_run | enabled |
```

## Regras

- **Sempre sub-agente**, nunca inline. Razões: isolamento, paralelismo,
  fail isolation.
- **Watch é default** — sem args, sempre re-arme o `/loop` (idempotente).
- **Não modifique** `routines.yml` aqui — apenas pelo `vbrain routines`
  (que atualiza next_run/last_run). Adição/edição vem do `/vbrain-add-routine`.
- **Manual trigger** (`<slug>`) NÃO altera state. É só pra teste/debug
  ou execução fora do schedule.
- Se algum sub-agente falhar, mostre na seção daquele slug um aviso
  `> erro: <mensagem>` e continue com os outros. NÃO re-dispare —
  semântica é at-most-once por design.
- **Não bootstrape `/loop`** no modo `<slug>` ou `status` — esses são
  comandos one-shot.
