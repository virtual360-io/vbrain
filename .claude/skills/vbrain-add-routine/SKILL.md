---
name: vbrain-add-routine
description: Adiciona uma rotina ao vbrain (~/vbrain/routines/routines.yml) com slug, descrição, cron schedule e prompt. Computa next_run inicial deterministicamente via fugit. Pergunta se quer testar agora via slug. NÃO bootstrappa nenhum loop nem cron — isso é responsabilidade do /vbrain-routine quando o usuário invocar. Use quando o usuário pedir "cria uma rotina", "adiciona rotina", "rotina que roda toda manhã às 6h", "rotina horária", ou "vbrain-add-routine".
allowed-tools: Bash, Read, Write, AskUserQuestion, Agent
---

# vbrain-add-routine

Cria uma rotina no `~/vbrain/routines/routines.yml`. O script Ruby
computa `next_run` deterministicamente (fugit) a partir do cron + agora.
Opcionalmente, pergunta se quer **testar agora** via sub-agente (manual
trigger via slug, não altera state).

**Esta skill NUNCA toca em `/loop`, `CronCreate`, ou `/vbrain-routine` em
modo watch.** Bootstrap do watch loop é responsabilidade exclusiva do
`/vbrain-routine` quando invocado pelo usuário.

## Inputs

- **slug**, **description**, **schedule**, **prompt**: peça em sequência
  se faltarem.

## Passos

### 1. Coletar inputs

Peça em ordem (uma pergunta por turno, mensagem livre):

**Slug**:
> "Slug da rotina, kebab-case (ex.: `morning-brief`, `email-hourly`,
> `weekly-review`)?"

**Description**:
> "Descrição (uma linha)?"

**Schedule** — aceite linguagem natural e converta pra cron 5-field
padrão (`min hora dia mês dia-semana`). Exemplos pra mostrar:

> "Quando deve rodar? Aceito linguagem natural ou cron direto.
> Exemplos:
> - `0 6 * * *` (todo dia 06:00)
> - `0 * * * *` (de hora em hora)
> - `0 10 * * 3` (toda quarta às 10:00)
> - `*/15 9-18 * * 1-5` (a cada 15min, 9-18h, dias úteis)
> - `0 8 * * 1` (toda segunda às 08:00)"

Converta natural → cron e **confirme com o usuário** antes de seguir:
> "Vou usar `0 6 * * *` (todo dia 06:00). Confirma?"

**Importante**: o cron é interpretado no **TZ local do sistema** (não UTC).
Se a máquina é -03:00 e o cron é `0 6 * * *`, dispara às 06:00 horário
de Brasília. Mencione isso se relevante (ex.: usuário viajando).

**Prompt**:
> "Cole o prompt. Pode usar markdown. Geralmente referencia outras skills
> (slash commands tipo `/vbrain-query-knowledge`), ferramentas MCP
> (`mcp__*` — quaisquer que sua sessão tiver carregadas), ou instruções
> de alto nível. O sub-agente que rodar essa rotina executa esse texto
> como instrução."

### 2. Detectar colisão de slug

```bash
BUNDLE_GEMFILE=/Users/victorcampos/Workspace/vbrain/Gemfile bundle exec ruby /Users/victorcampos/Workspace/vbrain/scripts/list_routines.rb --slug <slug>
```

Se `count > 0`, use `AskUserQuestion`:
> "Já existe rotina `<slug>`. Substituir?"
> 1. Substituir (Recommended)
> 2. Cancelar

Substituir → adicione `--replace` ao próximo passo. Cancelar → pare.

### 3. Salvar prompt em arquivo temporário

```
/tmp/vbrain-routine-prompt-<slug>.md
```

Use `Write`. Nunca passe prompt direto via `--prompt` (escape de shell
quebra com markdown/aspas/newlines).

### 4. Rodar o script

```bash
BUNDLE_GEMFILE=/Users/victorcampos/Workspace/vbrain/Gemfile bundle exec ruby /Users/victorcampos/Workspace/vbrain/scripts/add_routine.rb --slug <slug> --description "<desc>" --schedule "<cron>" --prompt-file /tmp/vbrain-routine-prompt-<slug>.md [--replace]
```

Output JSON: `{"config_path", "routine": {... incluindo next_run inicial}, "total"}`.

### 5. Commit (se houver repo git no `~/vbrain`)

```bash
BUNDLE_GEMFILE=/Users/victorcampos/Workspace/vbrain/Gemfile bundle exec ruby /Users/victorcampos/Workspace/vbrain/scripts/commit.rb --message "routine: adiciona '<slug>' (<cron>)"
```

(Use `routine: substitui '<slug>'` quando `--replace`.)

### 6. Oferecer teste agora (opcional)

Use `AskUserQuestion`:

> "Rotina criada. Quer testar agora? (manual trigger via slug, não conta
> como tick, não altera next_run)"
> 1. Sim, rodar agora (Recommended)
> 2. Não, só salvar

Se "Sim": lance **um** `Agent` (`subagent_type: "claude"`) com o template
de execução de rotina:

```
Você está executando a rotina vbrain "<SLUG>": <DESCRIPTION>

Instrução:

<PROMPT>

Quando terminar, devolva um único bloco markdown auto-contido com o
resultado (sem prefixos). Se chamar uma skill (slash command), invoque
via `Skill` tool. Se chamar uma ferramenta MCP (qualquer `mcp__*`),
invoque direto — use o que sua sessão tiver disponível, não enumere.
```

Mostre o output do sub-agente abaixo do report (passo 7).

### 7. Reportar

Mostre:
- `slug`, `description`, `schedule` (cron + tradução humana)
- `next_run` (em horário local + UTC entre parênteses)
- Primeiras linhas do `prompt`
- Se o usuário pediu teste: o output do sub-agente.
- **Como iniciar o watch loop** (separadamente): "Pra essa rotina rodar
  automaticamente nos horários do cron, invoque `/vbrain-routine` (sem
  args) quando quiser começar o watch — ele bootstrappa o `/loop 15m`
  com guarda CronList."

## Regras

- **NUNCA** invoque `/loop`, `Skill loop`, `CronCreate`, ou `/vbrain-routine`
  sem args. Esta skill só altera o YAML + opcionalmente dispara UM
  sub-agente pra teste manual via slug. Bootstrap do watch é
  responsabilidade exclusiva do `/vbrain-routine` quando o usuário
  invocar.
- **Nunca** escreva direto em `routines.yml` — sempre pelo script.
- **Nunca** invente prompt — pegue literal do usuário.
- **Sempre** confirme a tradução natural → cron com o usuário antes de
  salvar.
- Slug normalizado por `VBrain::Slug.from` (kebab-case ASCII). Se a
  normalização vira vazio, o script aborta e a skill pede outro slug.
- Se schedule for omitido/nulo, rotina vira **manual-only** (não tem
  next_run). Permita isso só se o usuário explicitamente pedir.
- O cron é interpretado no TZ do sistema. Pra mudar TZ, exporte
  `TZ=America/Sao_Paulo` (ou outro) antes de invocar a skill.
