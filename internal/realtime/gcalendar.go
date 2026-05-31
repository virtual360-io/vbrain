package realtime

import (
	"errors"
	"strings"
)

// Gcalendar is the Google Calendar realtime source.
type Gcalendar struct{}

const gcalTitle = "Google Calendar (realtime)"

var gcalTags = []string{"agenda", "calendar", "gcalendar", "realtime"}

var gcalKeywords = []string{
	"agenda", "agendas", "calendário", "calendario", "calendar", "gcalendar",
	"google calendar", "reunião", "reuniões", "reuniao", "reunioes",
	"meeting", "meetings", "evento", "eventos", "event", "events",
	"compromisso", "compromissos", "appointment", "appointments",
	"hoje", "amanhã", "amanha", "ontem", "today", "tomorrow", "yesterday",
	"essa semana", "esta semana", "semana", "próxima semana", "proxima semana",
	"this week", "next week", "mês", "mes", "month", "próximo mês",
	"fim de semana", "weekend", "livre", "ocupado", "disponível", "disponivel",
	"free", "busy", "schedule", "agenda do dia", "rotina",
}

func (Gcalendar) ConfigPath() string { return configPath("gcalendar") }

func normalizeCalendar(c map[string]string) Item {
	return Item{"id": c["id"], "summary": c["summary"], "timezone": c["timezone"]}
}

// SaveConfig normalizes, drops empty ids (requires ≥1), and writes the YAML.
func (Gcalendar) SaveConfig(calendars []map[string]string) ([]Item, error) {
	var norm []Item
	for _, c := range calendars {
		n := normalizeCalendar(c)
		if n["id"] != "" {
			norm = append(norm, n)
		}
	}
	if len(norm) == 0 {
		return nil, errors.New("at least one calendar required")
	}
	if err := saveConfig("gcalendar", "calendars", norm); err != nil {
		return nil, err
	}
	return norm, nil
}

func (Gcalendar) LoadConfig() ([]Item, bool) { return loadConfig("gcalendar", "calendars") }

func (Gcalendar) Frontmatter(calendars []Item) map[string]any {
	return map[string]any{
		"title": gcalTitle, "kind": "realtime", "source": "gcalendar",
		"tags": gcalTags, "calendars": itemsAny(calendars),
	}
}

func (Gcalendar) Body(calendars []Item) string {
	var b strings.Builder
	for _, c := range calendars {
		b.WriteString("- " + formatCalendar(c) + "\n")
	}
	return "# " + gcalTitle + "\n\n" +
		"This page is a **realtime source**: when `/vbrain-query-knowledge`\n" +
		"receives it as an FTS5 result, the agent does NOT return this body —\n" +
		"instead it calls `mcp__claude_ai_Google_Calendar__list_events` with the\n" +
		"calendars listed below and the time range derived from the query.\n\n" +
		"## Connected calendars\n\n" +
		strings.TrimRight(b.String(), "\n") + "\n\n" +
		"## Keywords (to match in FTS5)\n\n" +
		strings.Join(gcalKeywords, ", ") + ".\n"
}

func (g Gcalendar) WriteWikiPage(calendars []Item) (string, error) {
	return writePage("gcalendar", g.Frontmatter(calendars), g.Body(calendars))
}

func formatCalendar(c Item) string {
	id, summary := c["id"], c["summary"]
	if summary == "" || summary == id {
		return "`" + id + "`"
	}
	return summary + " (`" + id + "`)"
}
