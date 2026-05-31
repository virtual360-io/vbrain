package routines_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/virtual360-io/vbrain/internal/routines"
)

var fixedNow = time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)

func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("VBRAIN_HOME", t.TempDir())
}

func sp(s string) *string { return &s }

// add é um wrapper enxuto pros testes.
func add(t *testing.T, slug, desc, prompt string, schedule *string, enabled bool) (routines.Routine, error) {
	t.Helper()
	return routines.Add(slug, desc, prompt, schedule, enabled, false, fixedNow)
}

func TestLoadAllEmptyWhenMissing(t *testing.T) {
	isolate(t)
	all, _ := routines.LoadAll()
	if len(all) != 0 {
		t.Fatalf("LoadAll = %v", all)
	}
	en, _ := routines.Enabled()
	if len(en) != 0 {
		t.Fatalf("Enabled = %v", en)
	}
	if r, _ := routines.Find("anything"); r != nil {
		t.Fatalf("Find = %v", r)
	}
}

func TestAddCreatesYamlWithNextRun(t *testing.T) {
	isolate(t)
	e, err := add(t, "morning-brief", "Resumo da manhã", "Liste emails.", sp("0 6 * * *"), true)
	if err != nil {
		t.Fatal(err)
	}
	if e.Slug != "morning-brief" || e.Schedule == nil || *e.Schedule != "0 6 * * *" {
		t.Fatalf("entry = %+v", e)
	}
	if e.NextRun == nil || *e.NextRun != "2026-05-29T06:00:00Z" {
		t.Fatalf("next_run = %v", e.NextRun)
	}
	if e.LastRun != nil || !e.Enabled {
		t.Fatalf("last_run/enabled = %v/%v", e.LastRun, e.Enabled)
	}
	all, _ := routines.LoadAll()
	if len(all) != 1 || all[0].NextRun == nil || *all[0].NextRun != "2026-05-29T06:00:00Z" {
		t.Fatalf("reload = %+v", all)
	}
}

func TestAddAllowsNilSchedule(t *testing.T) {
	isolate(t)
	e, err := routines.Add("manual", "", "x", nil, true, false, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if e.Schedule != nil {
		t.Fatalf("schedule = %v, want nil", e.Schedule)
	}
}

func TestAddRejectsInvalidCron(t *testing.T) {
	isolate(t)
	if _, err := add(t, "x", "", "p", sp("every hour"), true); err == nil {
		t.Error("'every hour' deveria falhar")
	}
	if _, err := add(t, "y", "", "p", sp("0 6 * *"), true); err == nil {
		t.Error("'0 6 * *' (4 campos) deveria falhar")
	}
}

func TestAddNormalizesSlug(t *testing.T) {
	isolate(t)
	e, err := add(t, "Morning Brief!", "x", "y", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if e.Slug != "morning-brief" {
		t.Fatalf("slug = %q", e.Slug)
	}
}

func TestAddRejectsEmptySlugAndPrompt(t *testing.T) {
	isolate(t)
	if _, err := add(t, "", "", "x", nil, true); err == nil {
		t.Error("slug vazio deveria falhar")
	}
	if _, err := add(t, "valid", "", "", nil, true); err == nil {
		t.Error("prompt vazio deveria falhar")
	}
}

func TestAddCollisionRaisesWithoutReplace(t *testing.T) {
	isolate(t)
	add(t, "x", "", "first", nil, true)
	if _, err := add(t, "x", "", "second", nil, true); err == nil {
		t.Fatal("colisão deveria falhar sem replace")
	}
}

func TestAddReplaceUpdatesInPlacePreservingOrder(t *testing.T) {
	isolate(t)
	add(t, "a", "", "p1", nil, true)
	add(t, "b", "", "p2", nil, true)
	add(t, "c", "", "p3", nil, true)
	if _, err := routines.Add("b", "novo", "p2-new", nil, true, true, fixedNow); err != nil {
		t.Fatal(err)
	}
	all, _ := routines.LoadAll()
	if len(all) != 3 || all[0].Slug != "a" || all[1].Slug != "b" || all[2].Slug != "c" {
		t.Fatalf("ordem = %+v", all)
	}
	b, _ := routines.Find("b")
	if b.Description != "novo" || b.Prompt != "p2-new" {
		t.Fatalf("b = %+v", b)
	}
}

func TestEnabledFiltersDisabled(t *testing.T) {
	isolate(t)
	add(t, "on1", "", "x", nil, true)
	add(t, "off", "", "x", nil, false)
	add(t, "on2", "", "x", nil, true)
	en, _ := routines.Enabled()
	if len(en) != 2 || en[0].Slug != "on1" || en[1].Slug != "on2" {
		t.Fatalf("enabled = %+v", en)
	}
}

func TestRemove(t *testing.T) {
	isolate(t)
	add(t, "x", "", "p", nil, true)
	if ok, _ := routines.Remove("x"); !ok {
		t.Error("remove deveria retornar true")
	}
	if ok, _ := routines.Remove("x"); ok {
		t.Error("segundo remove deveria retornar false")
	}
}

func TestLoadAllTreatsMissingEnabledAsTrue(t *testing.T) {
	isolate(t)
	os.MkdirAll(filepath.Dir(routines.ConfigPath()), 0o755)
	os.WriteFile(routines.ConfigPath(), []byte("routines:\n  - slug: x\n    description: \"\"\n    prompt: p\n"), 0o644)
	r, _ := routines.Find("x")
	if r == nil || !r.Enabled {
		t.Fatalf("enabled ausente deveria virar true: %+v", r)
	}
}

func TestComputeNextRunIsDeterministic(t *testing.T) {
	n1, _ := routines.ComputeNextRun("0 6 * * *", fixedNow)
	n2, _ := routines.ComputeNextRun("0 6 * * *", fixedNow)
	if !n1.Equal(n2) {
		t.Fatal("não determinístico")
	}
	if got := n1.Format(time.RFC3339); got != "2026-05-29T06:00:00Z" {
		t.Fatalf("got %q", got)
	}
}

func TestComputeNextRunHourlyAndWeekly(t *testing.T) {
	hourly, _ := routines.ComputeNextRun("0 * * * *", fixedNow)
	weekly, _ := routines.ComputeNextRun("0 10 * * 3", fixedNow)
	if got := hourly.Format(time.RFC3339); got != "2026-05-28T13:00:00Z" {
		t.Errorf("hourly = %q", got)
	}
	if got := weekly.Format(time.RFC3339); got != "2026-06-03T10:00:00Z" {
		t.Errorf("weekly = %q", got)
	}
}

func TestClaimDueEmpty(t *testing.T) {
	isolate(t)
	due, _ := routines.ClaimDue(fixedNow)
	if len(due) != 0 {
		t.Fatalf("due = %v", due)
	}
}

func TestClaimDuePastNextRunAdvances(t *testing.T) {
	isolate(t)
	add(t, "h", "", "p", sp("0 * * * *"), true)
	add(t, "d", "", "p", sp("0 6 * * *"), true)

	oneHourLater := fixedNow.Add(time.Hour)
	due, _ := routines.ClaimDue(oneHourLater)
	if len(due) != 1 || due[0].Slug != "h" {
		t.Fatalf("due = %+v", due)
	}
	h, _ := routines.Find("h")
	if h.LastRun == nil || *h.LastRun != "2026-05-28T13:00:00Z" {
		t.Errorf("h.last_run = %v", h.LastRun)
	}
	if h.NextRun == nil || *h.NextRun != "2026-05-28T14:00:00Z" {
		t.Errorf("h.next_run = %v", h.NextRun)
	}
	d, _ := routines.Find("d")
	if d.LastRun != nil || d.NextRun == nil || *d.NextRun != "2026-05-29T06:00:00Z" {
		t.Errorf("d = %+v", d)
	}
}

func TestClaimDueExposesPreviousLastRun(t *testing.T) {
	isolate(t)
	add(t, "h", "", "p", sp("0 * * * *"), true)

	first := fixedNow.Add(time.Hour)
	d1, _ := routines.ClaimDue(first)
	if len(d1) != 1 || d1[0].LastRun != nil {
		t.Fatalf("primeiro claim deveria expor last_run nil: %+v", d1)
	}
	second := first.Add(time.Hour)
	d2, _ := routines.ClaimDue(second)
	if len(d2) != 1 || d2[0].LastRun == nil || *d2[0].LastRun != first.Format(time.RFC3339) {
		t.Fatalf("segundo claim deveria expor last_run anterior: %+v", d2)
	}
}

func TestClaimDueIdempotentSameTick(t *testing.T) {
	isolate(t)
	add(t, "h", "", "p", sp("0 * * * *"), true)
	later := fixedNow.Add(time.Hour)
	d1, _ := routines.ClaimDue(later)
	d2, _ := routines.ClaimDue(later)
	if len(d1) != 1 || len(d2) != 0 {
		t.Fatalf("d1=%d d2=%d", len(d1), len(d2))
	}
}

func TestClaimDueSkipsDisabled(t *testing.T) {
	isolate(t)
	add(t, "off", "", "p", sp("0 * * * *"), false)
	due, _ := routines.ClaimDue(fixedNow.Add(2 * time.Hour))
	if len(due) != 0 {
		t.Fatalf("due = %+v", due)
	}
}

func TestClaimDueSkipsNoSchedule(t *testing.T) {
	isolate(t)
	add(t, "manual", "", "p", nil, true)
	due, _ := routines.ClaimDue(fixedNow.Add(24 * time.Hour))
	if len(due) != 0 {
		t.Fatalf("due = %+v", due)
	}
}

func TestClaimDueBackfillsNextRunWhenMissing(t *testing.T) {
	isolate(t)
	os.MkdirAll(filepath.Dir(routines.ConfigPath()), 0o755)
	os.WriteFile(routines.ConfigPath(), []byte("routines:\n  - slug: x\n    description: \"\"\n    prompt: p\n    schedule: 0 * * * *\n    enabled: true\n"), 0o644)

	due, _ := routines.ClaimDue(fixedNow)
	if len(due) != 0 {
		t.Fatalf("não deveria estar due (next_time é estritamente após now): %+v", due)
	}
	r, _ := routines.Find("x")
	if r.NextRun == nil || *r.NextRun != "2026-05-28T13:00:00Z" {
		t.Fatalf("next_run backfill = %v", r.NextRun)
	}
}
