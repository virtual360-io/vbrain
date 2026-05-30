# CLAUDE.md — vbrain

Estas regras valem para qualquer tarefa neste repo, salvo override explícito.
Bias: cautela > velocidade em qualquer coisa não-trivial. Use julgamento em
tarefas triviais.

Contexto curto: este repo é uma base de conhecimento pessoal estilo ai-memory.
**Wiki em markdown é a fonte da verdade; o SQLite é índice derivado —
descartável (dá pra apagar e reconstruir com `reindex.rb`), mas versionado
junto da base por conveniência; o LLM só entra para o que exige julgamento
(chunkar, sintetizar páginas)**. Veja `README.md` para a arquitetura completa.

## Regra 1 — Think Before Coding

Declare suposições explicitamente. Se incerto, pergunte em vez de chutar.
Apresente múltiplas interpretações quando houver ambiguidade.
Empurre de volta quando existir um caminho mais simples.
Pare quando estiver confuso. Nomeie o que está pouco claro.

No vbrain: antes de tocar em `lib/vbrain/db.rb` ou no schema, declare o que
você acha que o índice está fazendo hoje e por quê — schema é o ponto mais
caro de errar.

## Regra 2 — Simplicity First

Código mínimo que resolve o problema. Nada especulativo.
Sem features além do pedido. Sem abstrações para código de uso único.
Teste: um sênior chamaria isso de overengenharia? Se sim, simplifique.

No vbrain: este repo segue o ai-memory e é deliberadamente raso. Skills + Ruby
+ SQLite, ponto. Não introduza camadas (cache, fila, ORM, DSL) sem pedido
explícito.

## Regra 3 — Surgical Changes

Toque só no que precisa. Limpe só a sua bagunça.
Não "melhore" código adjacente, comentários, formatação.
Não refatore o que não está quebrado. Combine com o estilo existente.

No vbrain: bug em `Sources::Twitter` não justifica reformatar `Sources::Url`.

## Regra 4 — Goal-Driven Execution

Defina critério de sucesso. Itere até verificar.
Não siga passos — defina sucesso e itere.
Critérios de sucesso fortes te deixam iterar sozinho.

No vbrain: "ingest de tweet com link funciona" significa `rake test` verde +
`/vbrain-add-knowledge <url>` produzindo páginas em `wiki/` cujos `path`
aparecem em `scripts/query.rb`. Não pare antes desses três.

## Regra 5 — Use o modelo só para julgamento

Use o LLM para: classificação, draft, sumarização, extração.
NÃO use o LLM para: roteamento, retries, transformações determinísticas.
Se código pode responder, código responde.

No vbrain isso é regra de arquitetura, não dica: chunker e wiki-writer são
subagentes porque exigem julgamento; `ingest_raw.rb`, `write_pages.rb`,
`reindex.rb`, `query.rb` são Ruby determinístico com teste minitest 1:1.
Detectar source_type, normalizar query FTS5, escrever frontmatter, montar
SQL — tudo Ruby. Nunca delegue ao subagente o que dá pra fazer em código.

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

No vbrain: se uma `Sources::*` faz X e outra faz Y para o mesmo problema,
não invente Z combinando os dois — pegue o padrão coberto por mais testes.

## Regra 8 — Leia antes de escrever

Antes de adicionar código, leia exports, callers imediatos, utilitários
compartilhados. "Parece ortogonal" é perigoso. Se não entende por que o
código está estruturado de um jeito, pergunte.

No vbrain antes de editar uma `Sources::*`: leia `Sources::Base`, `Sources`
(registry), e o `*_test.rb` correspondente. Antes de mexer em `Page` ou `DB`:
veja quem chama (`scripts/write_pages.rb`, `scripts/reindex.rb`).

## Regra 9 — Testes verificam intenção, não só comportamento

Testes precisam codificar o PORQUÊ do comportamento importar, não só o quê.
Um teste que não pode falhar quando a regra de negócio muda está errado.

No vbrain: todo arquivo determinístico em `lib/vbrain/` e `scripts/` tem teste
minitest correspondente em `test/`. Isso é **regra dura** — sem teste o código
não entra. Use `VBRAIN_HOME` apontando para tmpdir para isolar dados nos
testes (padrão estabelecido em `test/test_helper.rb`).

## Regra 10 — Checkpoint após cada passo significativo

Sumarize o que foi feito, o que está verificado, o que falta.
Não continue de um estado que você não consegue descrever de volta.
Se perder o fio, pare e re-enuncie.

No vbrain: pipeline de ingest tem 7 passos — depois de cada um diga o que
saiu (path, raw_id, count) antes de partir pro próximo. Não chegue em
`commit.rb` sem ter declarado quantas páginas o `write_pages.rb` produziu.

## Regra 11 — Combine com as convenções do codebase, mesmo discordando

Conformidade > gosto, dentro do codebase.
Se você acha de verdade que uma convenção é nociva, sinalize. Não forke em
silêncio.

No vbrain há convenções que parecem opinativas mas são intencionais:

- Scripts em `scripts/` retornam **JSON** no stdout (lido pelas skills) e
  texto humano no stderr. Não inverta.
- Wiki é escrita **só** por `scripts/write_pages.rb`. Skills nunca escrevem
  markdown direto em `wiki/`.
- `raw/` é **imutável** depois de gravado. Se o conteúdo precisa mudar,
  reingere.
- Não existe `wiki/index.md`. O índice é o SQLite. Não tente recriar um.
- O SQLite (`db/vbrain.sqlite3`) **é versionado** (não está no `.gitignore`):
  índice derivado e descartável, mas commitado por conveniência. Apagar `db/`
  + `reindex.rb` reconstrói tudo. Não re-adicione `/db/` ao ignore.

## Regra 12 — Falhe alto

"Concluído" está errado se algo foi pulado em silêncio.
"Testes passam" está errado se algum foi skipado.
Padrão: surface incerteza, não esconda.

No vbrain: se o chunker retornou 0 páginas, **reporte** "nenhuma página
criada, raw commitado como audit log" — não rode `stats.rb` e mostre só os
totais como se tudo tivesse dado certo. Se um subagente fabricou conteúdo
sem grounding, sinalize ao usuário antes de persistir.
