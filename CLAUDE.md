# CLAUDE.md — vbrain

Estas regras valem para qualquer tarefa neste repo, salvo override explícito.
Bias: cautela > velocidade em qualquer coisa não-trivial. Use julgamento em
tarefas triviais.

Contexto curto: este repo é uma base de conhecimento pessoal estilo ai-memory.
**Wiki em markdown é a fonte da verdade; o SQLite é índice derivado —
descartável (dá pra apagar e reconstruir com `vbrain reindex`), mas versionado
junto da base por conveniência; o LLM só entra para o que exige julgamento
(chunkar, sintetizar páginas)**. Veja `README.md` para a arquitetura completa.

Stack: **Go**. O núcleo determinístico é um binário único `vbrain` — código em
`cmd/vbrain/` (subcomandos do CLI) + `internal/<pkg>` (lógica), testes `go test`
1:1 por pacote. SQLite via `modernc.org/sqlite` (puro-Go, FTS5 embutido); git
via go-git, com fallback pro git do sistema quando presente. As skills em
`.claude/skills/` chamam `vbrain <subcomando>` (binário no PATH).

## Regra 1 — Think Before Coding

Declare suposições explicitamente. Se incerto, pergunte em vez de chutar.
Apresente múltiplas interpretações quando houver ambiguidade.
Empurre de volta quando existir um caminho mais simples.
Pare quando estiver confuso. Nomeie o que está pouco claro.

No vbrain: antes de tocar em `internal/db` (`SchemaSQL`) ou no schema, declare o
que você acha que o índice está fazendo hoje e por quê — schema é o ponto mais
caro de errar.

## Regra 2 — Simplicity First

Código mínimo que resolve o problema. Nada especulativo.
Sem features além do pedido. Sem abstrações para código de uso único.
Teste: um sênior chamaria isso de overengenharia? Se sim, simplifique.

No vbrain: este repo segue o ai-memory e é deliberadamente raso. Skills + Go
+ SQLite, ponto. Não introduza camadas (cache, fila, ORM, DSL) sem pedido
explícito.

## Regra 3 — Surgical Changes

Toque só no que precisa. Limpe só a sua bagunça.
Não "melhore" código adjacente, comentários, formatação.
Não refatore o que não está quebrado. Combine com o estilo existente.

No vbrain: bug em `sources.Twitter` não justifica reformatar `sources.URL`.

## Regra 4 — Goal-Driven Execution

Defina critério de sucesso. Itere até verificar.
Não siga passos — defina sucesso e itere.
Critérios de sucesso fortes te deixam iterar sozinho.

No vbrain: "ingest de tweet com link funciona" significa `go test ./...` verde +
`/vbrain-add-knowledge <url>` produzindo páginas em `wiki/` cujos `path`
aparecem em `vbrain query`. Não pare antes desses três.

## Regra 5 — Use o modelo só para julgamento

Use o LLM para: classificação, draft, sumarização, extração.
NÃO use o LLM para: roteamento, retries, transformações determinísticas.
Se código pode responder, código responde.

No vbrain isso é regra de arquitetura, não dica: chunker e wiki-writer são
subagentes porque exigem julgamento; `internal/ingest`, `internal/writepages`,
`internal/index`, `internal/search` (expostos pelos subcomandos `vbrain ingest`,
`write-pages`, `reindex`, `query`) são Go determinístico com `go test` 1:1.
Detectar source_type, normalizar query FTS5, escrever frontmatter, montar
SQL — tudo Go. Nunca delegue ao subagente o que dá pra fazer em código.

## Regra 6 — Token budgets não são sugestão

Por tarefa: 4.000 tokens. Por sessão: 30.000 tokens.
Se chegando perto do limite, sumarize e recomece.
Surface o estouro. Não estoure em silêncio.

No vbrain: prompts de chunker e wiki-writer ficam em
`.claude/skills/vbrain-add-knowledge/prompts/` — se inflar, refatore o prompt
antes de aumentar o budget.

## Regra 7 — Conflitos: escolha, não misture

Se dois padrões se contradizem, escolha um (mais recente / mais testado).
Explique por quê. Sinalize o outro para limpeza.
Não misture padrões conflitantes.

No vbrain: se uma fonte em `internal/sources` faz X e outra faz Y para o mesmo
problema, não invente Z combinando os dois — pegue o padrão coberto por mais
testes.

## Regra 8 — Leia antes de escrever

Antes de adicionar código, leia exports, callers imediatos, utilitários
compartilhados. "Parece ortogonal" é perigoso. Se não entende por que o
código está estruturado de um jeito, pergunte.

No vbrain antes de editar uma fonte em `internal/sources`: leia a interface
`Source`/`Ingestable`, o `Registry` (dispatcher em `sources.go`) e o
`*_test.go` correspondente. Antes de mexer em `internal/page` ou `internal/db`:
veja quem chama (`internal/writepages`, `internal/index`).

## Regra 9 — Testes verificam intenção, não só comportamento

Testes precisam codificar o PORQUÊ do comportamento importar, não só o quê.
Um teste que não pode falhar quando a regra de negócio muda está errado.

No vbrain: todo pacote determinístico em `internal/` tem `*_test.go`
correspondente. Isso é **regra dura** — sem teste o código não entra. Isole
dados nos testes com `t.Setenv("VBRAIN_HOME", t.TempDir())` (ou passe dirs
explícitos), nunca tocando a base real.

## Regra 10 — Checkpoint após cada passo significativo

Sumarize o que foi feito, o que está verificado, o que falta.
Não continue de um estado que você não consegue descrever de volta.
Se perder o fio, pare e re-enuncie.

No vbrain: pipeline de ingest tem 7 passos — depois de cada um diga o que
saiu (path, raw_id, count) antes de partir pro próximo. Não chegue em
`vbrain commit` sem ter declarado quantas páginas o `vbrain write-pages`
produziu.

## Regra 11 — Combine com as convenções do codebase, mesmo discordando

Conformidade > gosto, dentro do codebase.
Se você acha de verdade que uma convenção é nociva, sinalize. Não forke em
silêncio.

No vbrain há convenções que parecem opinativas mas são intencionais:

- Os subcomandos do `vbrain` retornam **JSON** no stdout (lido pelas skills) e
  texto humano no stderr. Não inverta.
- Wiki é escrita **só** por `vbrain write-pages` (`internal/writepages`).
  Skills nunca escrevem markdown direto em `wiki/`.
- `raw/` é **imutável** depois de gravado. Se o conteúdo precisa mudar,
  reingere.
- Não existe `wiki/index.md`. O índice é o SQLite. Não tente recriar um.
- O SQLite (`db/vbrain.sqlite3`) **é versionado** (não está no `.gitignore`):
  índice derivado e descartável, mas commitado por conveniência. Apagar `db/`
  + `vbrain reindex` reconstrói tudo. Não re-adicione `/db/` ao ignore.

## Regra 12 — Falhe alto

"Concluído" está errado se algo foi pulado em silêncio.
"Testes passam" está errado se algum foi skipado.
Padrão: surface incerteza, não esconda.

No vbrain: se o chunker retornou 0 páginas, **reporte** "nenhuma página
criada, raw commitado como audit log" — não rode `vbrain stats` e mostre só os
totais como se tudo tivesse dado certo. Se um subagente fabricou conteúdo
sem grounding, sinalize ao usuário antes de persistir.
