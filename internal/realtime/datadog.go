package realtime

import "strings"

// Datadog is the Datadog realtime source: monitors & alerts, incidents, and
// dashboards & metrics. An empty scope list is valid and means "all supported
// kinds, no tag filter".
//
// NOTE: no Datadog MCP is wired in vbrain yet, so the live handler in
// /vbrain-query-knowledge is documented but pending — until a Datadog MCP is
// connected, a query that hits this page reports Datadog isn't reachable. The
// config and phantom page are still created so the source is discoverable in
// FTS5 and ready the moment a handler exists.
type Datadog struct{}

const datadogTitle = "Datadog (realtime)"

var datadogTags = []string{"datadog", "observability", "monitor", "realtime"}

// datadogKinds are the supported scope kinds.
var datadogKinds = map[string]bool{"monitor": true, "incident": true, "dashboard": true}

var datadogKeywords = []string{
	"datadog", "monitor", "monitores", "monitors", "alerta", "alertas",
	"alert", "alerts", "incidente", "incidentes", "incident", "incidents",
	"dashboard", "dashboards", "métrica", "metrica", "métricas", "metricas",
	"metric", "metrics", "observabilidade", "observability", "apm", "trace",
	"traces", "latência", "latencia", "latency", "erro", "erros", "error",
	"errors", "error rate", "taxa de erro", "slo", "sli", "uptime",
	"disponibilidade", "downtime", "oncall", "on-call", "paged", "pager",
	"saúde", "saude", "health", "p99", "p95", "throughput",
}

func (Datadog) ConfigPath() string { return configPath("datadog") }

func normalizeScope(s map[string]string) Item {
	kind := strings.ToLower(strings.TrimSpace(s["kind"]))
	switch kind {
	case "monitors", "alert", "alerts":
		kind = "monitor"
	case "incidents":
		kind = "incident"
	case "dashboards", "metric", "metrics":
		kind = "dashboard"
	}
	return Item{"kind": kind, "tag": s["tag"]}
}

// SaveConfig normalizes, drops unknown kinds, and writes; an empty list is
// allowed (all supported kinds, no tag filter).
func (Datadog) SaveConfig(scopes []map[string]string) ([]Item, error) {
	norm := []Item{}
	for _, s := range scopes {
		n := normalizeScope(s)
		if !datadogKinds[n["kind"]] {
			continue
		}
		norm = append(norm, n)
	}
	if err := saveConfig("datadog", "scopes", norm); err != nil {
		return nil, err
	}
	return norm, nil
}

func (Datadog) LoadConfig() ([]Item, bool) { return loadConfig("datadog", "scopes") }

// AllKinds reports whether no explicit scope is set (watch all supported kinds).
func (Datadog) AllKinds(scopes []Item) bool {
	for _, s := range scopes {
		if s["kind"] != "" {
			return false
		}
	}
	return true
}

func (Datadog) Frontmatter(scopes []Item) map[string]any {
	return map[string]any{
		"title": datadogTitle, "kind": "realtime", "source": "datadog",
		"tags": datadogTags, "scopes": itemsAny(scopes),
	}
}

func (d Datadog) Body(scopes []Item) string {
	var scope string
	if d.AllKinds(scopes) {
		scope = "No specific scope connected: covers **all** supported kinds\n" +
			"(monitors & alerts, incidents, dashboards & metrics), with no tag filter.\n"
	} else {
		var b strings.Builder
		for _, s := range scopes {
			b.WriteString("- " + formatScope(s) + "\n")
		}
		scope = "Connected scopes — the live query filters by them:\n\n" +
			strings.TrimRight(b.String(), "\n") + "\n"
	}
	return "# " + datadogTitle + "\n\n" +
		"This page is a **realtime source**: when `/vbrain-query-knowledge`\n" +
		"receives it as an FTS5 result, the agent does NOT return this body —\n" +
		"instead it would call the Datadog MCP handler (monitors, incidents,\n" +
		"metrics) with the user's query. **No Datadog MCP is connected yet**, so\n" +
		"the handler is pending: until one is wired, the agent reports that\n" +
		"Datadog isn't reachable and to connect its MCP.\n\n" +
		"## Scope\n\n" + scope + "\n" +
		"## Keywords (to match in FTS5)\n\n" +
		strings.Join(datadogKeywords, ", ") + ".\n"
}

func (d Datadog) WriteWikiPage(scopes []Item) (string, error) {
	return writePage("datadog", d.Frontmatter(scopes), d.Body(scopes))
}

func formatScope(s Item) string {
	kind, tag := s["kind"], s["tag"]
	if tag == "" {
		return kind
	}
	return kind + " (`" + tag + "`)"
}
