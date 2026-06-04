package realtime_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/virtual360-io/vbrain/internal/page"
	"github.com/virtual360-io/vbrain/internal/paths"
	"github.com/virtual360-io/vbrain/internal/realtime"
)

func isolate(t *testing.T) { t.Setenv("VBRAIN_HOME", t.TempDir()) }

var calendars = []map[string]string{
	{"id": "primary", "summary": "Victor", "timezone": "America/Sao_Paulo"},
	{"id": "work@v360.io", "summary": "V360 Work", "timezone": "America/Sao_Paulo"},
}

func TestGcalendarSaveLoadAndPage(t *testing.T) {
	isolate(t)
	saved, err := realtime.Gcalendar{}.SaveConfig(calendars)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 2 {
		t.Fatalf("saved = %d", len(saved))
	}
	loaded, ok := realtime.Gcalendar{}.LoadConfig()
	if !ok || loaded[0]["id"] != "primary" || loaded[0]["summary"] != "Victor" {
		t.Fatalf("loaded = %+v", loaded)
	}

	path, err := realtime.Gcalendar{}.WriteWikiPage(saved)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(paths.WikiDir(), "_realtime", "gcalendar.md") {
		t.Errorf("path = %q", path)
	}
	p, _ := page.Parse(path)
	if p.Frontmatter["kind"] != "realtime" || p.Frontmatter["source"] != "gcalendar" {
		t.Errorf("frontmatter = %+v", p.Frontmatter)
	}
	cals, _ := p.Frontmatter["calendars"].([]any)
	if len(cals) != 2 {
		t.Errorf("calendars = %v", p.Frontmatter["calendars"])
	}
	for _, want := range []string{"agenda", "reunião", "primary", "work@v360.io"} {
		if !strings.Contains(p.Body, want) {
			t.Errorf("body sem %q", want)
		}
	}
}

func TestGcalendarRejectsEmptyAndBlankID(t *testing.T) {
	isolate(t)
	gc := realtime.Gcalendar{}
	if _, err := gc.SaveConfig(nil); err == nil {
		t.Error("empty list should fail")
	}
	saved, err := gc.SaveConfig(append(calendars, map[string]string{"id": "", "summary": "Blank"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 2 {
		t.Errorf("blank id should be discarded: %+v", saved)
	}
}

func TestGcalendarBodyFormat(t *testing.T) {
	collapsed := realtime.Gcalendar{}.Body([]realtime.Item{{"id": "victor@v360.io", "summary": "victor@v360.io"}})
	if !strings.Contains(collapsed, "`victor@v360.io`") || strings.Contains(collapsed, "victor@v360.io (`victor@v360.io`)") {
		t.Errorf("summary==id should collapse: %s", firstBullet(collapsed))
	}
	distinct := realtime.Gcalendar{}.Body([]realtime.Item{{"id": "primary", "summary": "Victor"}})
	if !strings.Contains(distinct, "Victor (`primary`)") {
		t.Errorf("distinto: %s", firstBullet(distinct))
	}
	blank := realtime.Gcalendar{}.Body([]realtime.Item{{"id": "abc", "summary": ""}})
	if !strings.Contains(blank, "`abc`") {
		t.Errorf("empty summary falls back to id: %s", firstBullet(blank))
	}
}

func TestGmailLabelFilterClause(t *testing.T) {
	g := realtime.Gmail{}
	if got := g.LabelFilterClause(nil); got != "" {
		t.Errorf("0 labels = %q", got)
	}
	if got := g.LabelFilterClause([]realtime.Item{{"id": "INBOX"}}); got != "label:INBOX" {
		t.Errorf("1 label = %q", got)
	}
	got := g.LabelFilterClause([]realtime.Item{{"id": "INBOX"}, {"id": "IMPORTANT"}})
	if got != "(label:INBOX OR label:IMPORTANT)" {
		t.Errorf("2 labels = %q", got)
	}
}

func TestGmailNormalizesLabelIdAlias(t *testing.T) {
	isolate(t)
	saved, err := realtime.Gmail{}.SaveConfig([]map[string]string{{"labelId": "Label_1", "name": "Trabalho"}})
	if err != nil {
		t.Fatal(err)
	}
	if saved[0]["id"] != "Label_1" || saved[0]["name"] != "Trabalho" {
		t.Fatalf("saved = %+v", saved)
	}
}

func TestSlackGlobalAndFilteredBody(t *testing.T) {
	isolate(t)
	s := realtime.Slack{}
	// vazio = global, permitido
	saved, err := s.SaveConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Global(saved) {
		t.Error("empty list should be global")
	}
	if !strings.Contains(s.Body(saved), "global") {
		t.Error("global body should mention a global search")
	}

	chans := []realtime.Item{{"id": "C123", "name": "geral"}}
	if s.Global(chans) {
		t.Error("with a channel it should not be global")
	}
	if !strings.Contains(s.Body(chans), "#geral (`C123`)") {
		t.Errorf("body filtrado: %s", firstBullet(s.Body(chans)))
	}
}

func TestSlackChannelFilter(t *testing.T) {
	s := realtime.Slack{}
	if got := s.ChannelFilter(realtime.Item{"id": "C123"}); got != "in:<#C123>" {
		t.Errorf("id = %q", got)
	}
	if got := s.ChannelFilter(realtime.Item{"name": "geral"}); got != "in:#geral" {
		t.Errorf("name = %q", got)
	}
	if got := s.ChannelFilter(realtime.Item{}); got != "" {
		t.Errorf("empty = %q", got)
	}
}

func TestGitHubGlobalAndFilteredBody(t *testing.T) {
	isolate(t)
	g := realtime.GitHub{}
	// empty = global, allowed
	saved, err := g.SaveConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !g.Global(saved) {
		t.Error("empty list should be global")
	}
	if !strings.Contains(g.Body(saved), "global") {
		t.Error("global body should mention a global search")
	}

	repos := []realtime.Item{{"owner": "virtual360-io", "name": "vbrain"}}
	if g.Global(repos) {
		t.Error("with a repo it should not be global")
	}
	if !strings.Contains(g.Body(repos), "virtual360-io/vbrain") {
		t.Errorf("filtered body: %s", firstBullet(g.Body(repos)))
	}
}

func TestGitHubNormalizesFullNameAndDropsIncomplete(t *testing.T) {
	isolate(t)
	saved, err := realtime.GitHub{}.SaveConfig([]map[string]string{
		{"full_name": "virtual360-io/vbrain"},
		{"owner": "acme"}, // incomplete: dropped
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 1 || saved[0]["owner"] != "virtual360-io" || saved[0]["name"] != "vbrain" {
		t.Fatalf("saved = %+v", saved)
	}
}

func TestGitHubRepoFilterClause(t *testing.T) {
	g := realtime.GitHub{}
	if got := g.RepoFilterClause(nil); got != "" {
		t.Errorf("global = %q", got)
	}
	one := g.RepoFilterClause([]realtime.Item{{"owner": "virtual360-io", "name": "vbrain"}})
	if one != "repo:virtual360-io/vbrain" {
		t.Errorf("1 repo = %q", one)
	}
	two := g.RepoFilterClause([]realtime.Item{
		{"owner": "a", "name": "b"}, {"owner": "c", "name": "d"},
	})
	if two != "repo:a/b repo:c/d" {
		t.Errorf("2 repos = %q", two)
	}
}

func TestDatadogNormalizesKindsAndDropsUnknown(t *testing.T) {
	isolate(t)
	saved, err := realtime.Datadog{}.SaveConfig([]map[string]string{
		{"kind": "alerts", "tag": "service:vbrain"}, // -> monitor
		{"kind": "incidents"},                       // -> incident
		{"kind": "metrics"},                         // -> dashboard
		{"kind": "logs"},                            // unknown: dropped
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 3 {
		t.Fatalf("saved = %+v", saved)
	}
	if saved[0]["kind"] != "monitor" || saved[0]["tag"] != "service:vbrain" {
		t.Errorf("alerts should normalize to monitor: %+v", saved[0])
	}
	if saved[1]["kind"] != "incident" || saved[2]["kind"] != "dashboard" {
		t.Errorf("kind normalization: %+v", saved)
	}
}

func TestDatadogAllKindsAndBody(t *testing.T) {
	isolate(t)
	d := realtime.Datadog{}
	saved, err := d.SaveConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !d.AllKinds(saved) {
		t.Error("empty scopes should mean all kinds")
	}
	if !strings.Contains(d.Body(saved), "all") {
		t.Error("all-kinds body should say it covers all kinds")
	}
	// the body must stay honest that the handler is pending (no Datadog MCP)
	if !strings.Contains(d.Body(saved), "No Datadog MCP is connected yet") {
		t.Error("body must flag that the live handler is pending")
	}

	scoped := []realtime.Item{{"kind": "monitor", "tag": "env:prod"}}
	if d.AllKinds(scoped) {
		t.Error("with a scope it should not be all-kinds")
	}
	if !strings.Contains(d.Body(scoped), "monitor (`env:prod`)") {
		t.Errorf("scoped body: %s", firstBullet(d.Body(scoped)))
	}
}

func TestDatadogWritesPage(t *testing.T) {
	isolate(t)
	saved, err := realtime.Datadog{}.SaveConfig([]map[string]string{{"kind": "monitor"}})
	if err != nil {
		t.Fatal(err)
	}
	path, err := realtime.Datadog{}.WriteWikiPage(saved)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(paths.WikiDir(), "_realtime", "datadog.md") {
		t.Errorf("path = %q", path)
	}
	p, _ := page.Parse(path)
	if p.Frontmatter["kind"] != "realtime" || p.Frontmatter["source"] != "datadog" {
		t.Errorf("frontmatter = %+v", p.Frontmatter)
	}
}

func firstBullet(body string) string {
	for _, l := range strings.Split(body, "\n") {
		if strings.HasPrefix(l, "- ") {
			return l
		}
	}
	return ""
}
