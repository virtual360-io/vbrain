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

func firstBullet(body string) string {
	for _, l := range strings.Split(body, "\n") {
		if strings.HasPrefix(l, "- ") {
			return l
		}
	}
	return ""
}
