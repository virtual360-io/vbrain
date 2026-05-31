package realtime

import "strings"

// Slack é a fonte realtime do Slack. Diferente de gmail/gcalendar: lista de
// canais vazia é válida e significa busca global no workspace inteiro.
type Slack struct{}

const slackTitle = "Slack (realtime)"

var slackTags = []string{"slack", "chat", "mensagem", "realtime"}

var slackKeywords = []string{
	"slack", "canal", "canais", "channel", "channels", "mensagem",
	"mensagens", "message", "messages", "conversa", "conversas",
	"conversation", "thread", "threads", "dm", "dms", "direct message",
	"mensagem direta", "huddle", "workspace", "time", "equipe", "team",
	"menção", "mencao", "menções", "mencionado", "mention", "mentioned",
	"respondeu", "respondi", "responder", "reply", "post", "postou",
	"postei", "escreveu", "escrevi", "falou", "disse", "comentou",
	"anexo", "anexos", "arquivo", "arquivos", "file", "files",
	"quem disse", "alguém falou", "alguem falou",
}

func (Slack) ConfigPath() string { return configPath("slack") }

func normalizeChannel(c map[string]string) Item {
	id := c["id"]
	if id == "" {
		id = c["channel_id"]
	}
	return Item{"id": id, "name": c["name"]}
}

// SaveConfig normaliza e grava; lista vazia é permitida (busca global).
func (Slack) SaveConfig(channels []map[string]string) ([]Item, error) {
	norm := []Item{}
	for _, c := range channels {
		n := normalizeChannel(c)
		if n["id"] != "" || n["name"] != "" {
			norm = append(norm, n)
		}
	}
	if err := saveConfig("slack", "channels", norm); err != nil {
		return nil, err
	}
	return norm, nil
}

func (Slack) LoadConfig() ([]Item, bool) { return loadConfig("slack", "channels") }

// Global indica busca no workspace inteiro (nenhum canal conectado).
func (Slack) Global(channels []Item) bool {
	for _, c := range channels {
		if c["id"] != "" || c["name"] != "" {
			return false
		}
	}
	return true
}

func (Slack) Frontmatter(channels []Item) map[string]any {
	return map[string]any{
		"title": slackTitle, "kind": "realtime", "source": "slack",
		"tags": slackTags, "channels": itemsAny(channels),
	}
}

func (s Slack) Body(channels []Item) string {
	var scope string
	if s.Global(channels) {
		scope = "Nenhum canal específico conectado: a busca é **global** no workspace\n" +
			"inteiro (todos os canais/DMs acessíveis), sem filtro `in:`.\n"
	} else {
		var b strings.Builder
		for _, c := range channels {
			b.WriteString("- " + formatChannel(c) + "\n")
		}
		scope = "Canais conectados — a busca filtra por eles (uma chamada por canal,\n" +
			"já que o Slack search não tem operador `OR`):\n\n" +
			strings.TrimRight(b.String(), "\n") + "\n"
	}
	return "# " + slackTitle + "\n\n" +
		"Esta página é uma **fonte realtime**: quando o `/vbrain-query-knowledge`\n" +
		"a recebe como resultado FTS5, o agente NÃO devolve este body — em vez\n" +
		"disso chama `mcp__claude_ai_Slack__slack_search_public_and_private`\n" +
		"com a query do usuário convertida pra Slack search syntax.\n\n" +
		"## Escopo\n\n" + scope + "\n" +
		"## Keywords (pra casar no FTS5)\n\n" +
		strings.Join(slackKeywords, ", ") + ".\n"
}

func (s Slack) WriteWikiPage(channels []Item) (string, error) {
	return writePage("slack", s.Frontmatter(channels), s.Body(channels))
}

func formatChannel(c Item) string {
	n := normalizeChannel(c)
	id, name := n["id"], n["name"]
	if name == "" {
		return "`" + id + "`"
	}
	if id == "" {
		return "#" + name
	}
	return "#" + name + " (`" + id + "`)"
}

// ChannelFilter devolve o modificador Slack `in:<#ID>` (ou `in:#name`), "" se
// vazio.
func (Slack) ChannelFilter(c Item) string {
	n := normalizeChannel(c)
	if n["id"] != "" {
		return "in:<#" + n["id"] + ">"
	}
	if n["name"] != "" {
		return "in:#" + n["name"]
	}
	return ""
}
