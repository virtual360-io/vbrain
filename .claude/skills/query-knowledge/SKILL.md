---
name: query-knowledge
description: Consulta a base vbrain (SQLite FTS5) e devolve trechos relevantes em markdown. Use quando o usuário perguntar algo que pode estar arquivado ("o que eu sei sobre X", "procura no vbrain por Y"), ou quando outro agente precisar de contexto persistido para uma tarefa.
allowed-tools: Bash, Read
---

# query-knowledge

Skill leve de leitura: roda `scripts/query.rb` contra o índice FTS5 e formata
o resultado para consumo (usuário humano ou outro agente).

## Inputs

- **query** (obrigatório): pergunta livre ou keyword. Pode conter `:`, aspas,
  parênteses — o normalizador Ruby cuida disso.
- **limit** (opcional, default 10): número máximo de páginas a retornar.

## Passos

### 1. Primeira tentativa

```bash
bundle exec ruby scripts/query.rb "<query>" --limit <N> --format json
```

Parseie `results`. Se vier vazio, vá para o passo 2; senão pule para 3.

### 2. Fallback com prefix matching

Quando a primeira tentativa volta vazia, refaça com `--prefix` para casar
prefixos (ex.: `replic` → `replication`):

```bash
bundle exec ruby scripts/query.rb "<query>" --limit <N> --prefix --format json
```

Se ainda vier vazio, responda ao usuário: "Nenhum resultado encontrado para
`<query>` na base vbrain. Tente termos mais gerais ou verifique se algo já foi
ingerido com `/add-knowledge`."

### 3. Formatar resposta

Para cada resultado, mostre:

- Título (com link `wiki/<path>` no path completo)
- Snippet (já vem com `**termo**` destacado pelo FTS5)
- Tags (se `frontmatter.tags` valer a pena — leia o arquivo se o usuário quer
  mais contexto)

Quando o caller é outro agente (heurística: a pergunta veio de uma `Task` em
vez de prompt humano), **leia o arquivo inteiro** dos top 3 e inclua o body
markdown na resposta — o agente precisa de contexto rico, não snippet.

Quando o caller é o usuário, mantenha enxuto: snippet + path + ofereça
"quer que eu abra a página X?"

## Regras

- **Não modifique** nada — esta skill é read-only.
- Se a query tiver `< 3 caracteres` significativos, peça uma query mais
  específica antes de rodar.
- Não invente conteúdo: se o snippet não responde a pergunta, diga
  explicitamente "essas páginas mencionam o termo mas talvez não respondam
  diretamente". A wiki é a fonte; você é só o intermediário.
