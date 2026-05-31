package realtime

import "strings"

// Slack is the Slack realtime source. Unlike gmail/gcalendar: an empty channel
// list is valid and means a global search across the whole workspace.
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

// SaveConfig normalizes and writes; an empty list is allowed (global search).
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

// Global indicates a search across the whole workspace (no channel connected).
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
		scope = "No specific channel connected: the search is **global** across the\n" +
			"whole workspace (all accessible channels/DMs), with no `in:` filter.\n"
	} else {
		var b strings.Builder
		for _, c := range channels {
			b.WriteString("- " + formatChannel(c) + "\n")
		}
		scope = "Connected channels — the search filters by them (one call per channel,\n" +
			"since Slack search has no `OR` operator):\n\n" +
			strings.TrimRight(b.String(), "\n") + "\n"
	}
	return "# " + slackTitle + "\n\n" +
		"This page is a **realtime source**: when `/vbrain-query-knowledge`\n" +
		"receives it as an FTS5 result, the agent does NOT return this body —\n" +
		"instead it calls `mcp__claude_ai_Slack__slack_search_public_and_private`\n" +
		"with the user's query converted to Slack search syntax.\n\n" +
		"## Scope\n\n" + scope + "\n" +
		"## Keywords (to match in FTS5)\n\n" +
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

// ChannelFilter returns the Slack `in:<#ID>` modifier (or `in:#name`), "" if
// empty.
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
