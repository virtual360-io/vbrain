package realtime

import (
	"errors"
	"strings"
)

// Gmail is the Gmail realtime source.
type Gmail struct{}

const gmailTitle = "Gmail (realtime)"

var gmailTags = []string{"email", "gmail", "inbox", "realtime"}

var gmailKeywords = []string{
	"email", "emails", "e-mail", "e-mails", "mail", "mensagem", "mensagens",
	"message", "messages", "inbox", "caixa de entrada", "remetente", "sender",
	"enviado", "enviada", "sent", "recebido", "recebida", "received",
	"responder", "respondeu", "respondi", "anexo", "anexos", "attachment",
	"attachments", "gmail", "google mail", "assunto", "subject", "thread",
	"conversa", "conversation", "rascunho", "draft", "spam", "lixeira",
	"trash", "estrelado", "starred", "important", "importante", "não lido",
	"nao lido", "unread", "label", "marcador",
}

func (Gmail) ConfigPath() string { return configPath("gmail") }

func normalizeLabel(l map[string]string) Item {
	id := l["id"]
	if id == "" {
		id = l["labelId"]
	}
	return Item{"id": id, "name": l["name"]}
}

// SaveConfig normalizes, drops empty ids (requires ≥1), and writes the YAML.
func (Gmail) SaveConfig(labels []map[string]string) ([]Item, error) {
	var norm []Item
	for _, l := range labels {
		n := normalizeLabel(l)
		if n["id"] != "" {
			norm = append(norm, n)
		}
	}
	if len(norm) == 0 {
		return nil, errors.New("at least one label required")
	}
	if err := saveConfig("gmail", "labels", norm); err != nil {
		return nil, err
	}
	return norm, nil
}

func (Gmail) LoadConfig() ([]Item, bool) { return loadConfig("gmail", "labels") }

func (Gmail) Frontmatter(labels []Item) map[string]any {
	return map[string]any{
		"title": gmailTitle, "kind": "realtime", "source": "gmail",
		"tags": gmailTags, "labels": itemsAny(labels),
	}
}

func (Gmail) Body(labels []Item) string {
	var b strings.Builder
	for _, l := range labels {
		b.WriteString("- " + formatLabel(l) + "\n")
	}
	return "# " + gmailTitle + "\n\n" +
		"This page is a **realtime source**: when `/vbrain-query-knowledge`\n" +
		"receives it as an FTS5 result, the agent does NOT return this body —\n" +
		"instead it calls `mcp__claude_ai_Gmail__search_threads`, prepending a\n" +
		"`(label:<id1> OR label:<id2> …)` filter with the connected labels.\n\n" +
		"## Connected labels\n\n" +
		strings.TrimRight(b.String(), "\n") + "\n\n" +
		"## Keywords (to match in FTS5)\n\n" +
		strings.Join(gmailKeywords, ", ") + ".\n"
}

func (g Gmail) WriteWikiPage(labels []Item) (string, error) {
	return writePage("gmail", g.Frontmatter(labels), g.Body(labels))
}

func formatLabel(l Item) string {
	id, name := l["id"], l["name"]
	if name == "" || name == id {
		return "`" + id + "`"
	}
	return name + " (`" + id + "`)"
}

// LabelFilterClause builds the Gmail `(label:a OR label:b)` filter (or `label:a`
// for a single one, "" for none).
func (Gmail) LabelFilterClause(labels []Item) string {
	var ids []string
	for _, l := range labels {
		if l["id"] != "" {
			ids = append(ids, l["id"])
		}
	}
	switch len(ids) {
	case 0:
		return ""
	case 1:
		return "label:" + ids[0]
	default:
		parts := make([]string, len(ids))
		for i, id := range ids {
			parts[i] = "label:" + id
		}
		return "(" + strings.Join(parts, " OR ") + ")"
	}
}
