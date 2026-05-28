---
name: vbrain-routine
description: Watch loop das rotinas do vbrain. Verifica em ~/vbrain/routines/routines.yml quais rotinas têm next_run vencido, dispara sub-agente paralelo pra cada, e a próxima execução é calculada deterministicamente pelo cron (fugit). Se chamado sem args, esse é o comportamento padrão (watch). Use quando o usuário pedir "roda minhas rotinas", "vbrain-routine", "executa rotina morning-brief", "fica rodando em background", ou referenciar uma rotina pelo slug.
allowed-tools: Bash, Read, Agent, AskUserQuestion, Skill
---

# vbrain-routine

Loop de execução das rotinas. **Watch é o default**: sem args, esta skill
roda um "tick" (claim de rotinas vencidas + dispatch + atualização de
`next_run`) e garante que o `/loop` global esteja registrado pra rearmar
sozinho a cada minuto.

## Inputs (formas aceitas)

- **(vazio)** → **tick + watch**: identifica rotinas com `next_run <= now`,
  dispara sub-agente paralelo pra cada, deixa o `/loop 1m /vbrain-routine`
  rodando. Idempotente.
- **`<slug>`** → executa só essa rotina **agora** (manual trigger), sem
  alterar `next_run` nem `last_run`.
- **`status`** → lista todas com slug, schedule, próximo run, último run,
  enabled. Não dispara nada.

## Passos (modo default — watch)

### 1. Tick: claim de rotinas vencidas

```bash
BUNDLE_GEMFILE=/Users/victorcampos/Workspace/vbrain/Gemfile bundle exec ruby /Users/victorcampos/Workspace/vbrain/scripts/run_due_routines.rb
```

Esse script é **determinístico** e atômico:
- Lê `~/vbrain/routines/routines.yml`.
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
/vbrain-query-knowledge ou outra skill vbrain, invoque via Skill tool.
Se chamar um MCP (mcp__claude_ai_Google_Calendar, mcp__claude_ai_Gmail,
etc.), invoque direto. Datas relativas como "hoje" ou "essa semana" são
em relação ao momento da execução.
```

### 3. Garantir o /loop ativo

Invoque a skill `loop` via `Skill` tool com `args: "1m /vbrain-routine"`.
Se já estiver ativo, `/loop` provavelmente recusa ou substitui — siga o
feedback dele e **não pare** o fluxo por isso (loop pode já estar
funcionando).

### 4. Reportar

Mostre:

```
# Rotinas executadas (N)

> tick @ <now ISO8601 UTC>
> próximo tick automático em 1m via /loop

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

(Use `list_routines.rb` pra pegar `next_run` se precisar de detalhes.)

## Passos (modo `<slug>` — manual trigger)

1. `BUNDLE_GEMFILE=… bundle exec ruby scripts/list_routines.rb --slug <slug>`
   pra recuperar o entry.
2. Se `count == 0`, reporte "rotina `<slug>` não existe" + sugira
   `/vbrain-add-routine`.
3. Lance **um** `Agent` com o mesmo template do passo 2 acima.
4. Reporte o output. **Não chame** `run_due_routines.rb` — manual trigger
   não altera `next_run`/`last_run`.

## Passos (modo `status`)

```bash
BUNDLE_GEMFILE=… bundle exec ruby scripts/list_routines.rb
```

Tabela:
```
| slug | schedule | next_run | last_run | enabled |
```

## Regras

- **Sempre sub-agente**, nunca inline. Razões: isolamento, paralelismo,
  fail isolation.
- **Watch é default** — sem args, sempre re-arme o `/loop` (idempotente).
- **Não modifique** `routines.yml` aqui — apenas pelo `run_due_routines.rb`
  (que atualiza next_run/last_run). Adição/edição vem do `/vbrain-add-routine`.
- **Manual trigger** (`<slug>`) NÃO altera state. É só pra teste/debug
  ou execução fora do schedule.
- Se algum sub-agente falhar, mostre na seção daquele slug um aviso
  `> erro: <mensagem>` e continue com os outros. NÃO re-dispare —
  semântica é at-most-once por design.
- **Não bootstrape `/loop`** no modo `<slug>` ou `status` — esses são
  comandos one-shot.
