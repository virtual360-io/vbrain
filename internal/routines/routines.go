// Package routines gerencia as rotinas agendadas do vbrain (config/routines/
// routines.yml). Porta determinística de lib/vbrain/routines.rb. next_run é
// computado por cron (robfig/cron, equivalente ao fugit do Ruby para crons
// padrão de 5 campos). claim_due! avança ANTES de executar (at-most-once).
package routines

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/virtual360-io/vbrain/internal/paths"
	"github.com/virtual360-io/vbrain/internal/slug"
	"gopkg.in/yaml.v3"
)

// Err é o erro de validação/parse das rotinas.
var Err = errors.New("routines")

var cronRE = regexp.MustCompile(`^\S+\s+\S+\s+\S+\s+\S+\s+\S+$`)

// Routine é uma rotina agendada. Campos nuláveis usam ponteiro (blank → nil).
type Routine struct {
	Slug        string  `yaml:"slug" json:"slug"`
	Description string  `yaml:"description" json:"description"`
	Schedule    *string `yaml:"schedule" json:"schedule"`
	NextRun     *string `yaml:"next_run" json:"next_run"`
	LastRun     *string `yaml:"last_run" json:"last_run"`
	Prompt      string  `yaml:"prompt" json:"prompt"`
	Enabled     bool    `yaml:"enabled" json:"enabled"`
}

// ClaimedRoutine é uma rotina vencida (com o last_run ANTERIOR exposto).
type ClaimedRoutine struct {
	Routine
	ClaimedAt string `json:"claimed_at"`
}

// ConfigPath devolve o caminho do routines.yml na base.
func ConfigPath() string {
	return filepath.Join(paths.DataHome(), "config", "routines", "routines.yml")
}

// LoadAll lê e normaliza todas as rotinas; [] se o arquivo não existe.
func LoadAll() ([]Routine, error) {
	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return []Routine{}, nil
		}
		return nil, err
	}
	var file struct {
		Routines []map[string]any `yaml:"routines"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	out := make([]Routine, 0, len(file.Routines))
	for _, m := range file.Routines {
		out = append(out, normalize(m))
	}
	return out, nil
}

// Enabled devolve as rotinas habilitadas.
func Enabled() ([]Routine, error) {
	all, err := LoadAll()
	if err != nil {
		return nil, err
	}
	var out []Routine
	for _, r := range all {
		if r.Enabled {
			out = append(out, r)
		}
	}
	return out, nil
}

// Find busca uma rotina pelo slug.
func Find(slugStr string) (*Routine, error) {
	all, err := LoadAll()
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].Slug == slugStr {
			return &all[i], nil
		}
	}
	return nil, nil
}

// Add cria (ou substitui, com replace) uma rotina e persiste, devolvendo a
// entrada. next_run é computado deterministicamente do schedule.
func Add(slugStr, description, prompt string, schedule *string, enabled, replace bool, now time.Time) (Routine, error) {
	if strings.TrimSpace(slugStr) == "" {
		return Routine{}, errWrap("slug cannot be empty")
	}
	if strings.TrimSpace(prompt) == "" {
		return Routine{}, errWrap("prompt cannot be empty")
	}
	normSlug, err := slug.From(slugStr)
	if err != nil {
		return Routine{}, errWrap("slug normalized to empty: " + slugStr)
	}

	sched, err := normalizeSchedule(schedule)
	if err != nil {
		return Routine{}, err
	}
	var nextRun *string
	if sched != nil {
		t, err := ComputeNextRun(*sched, now)
		if err != nil {
			return Routine{}, err
		}
		s := iso(t)
		nextRun = &s
	}

	existing, err := LoadAll()
	if err != nil {
		return Routine{}, err
	}
	idx := -1
	for i := range existing {
		if existing[i].Slug == normSlug {
			idx = i
			break
		}
	}
	if idx >= 0 && !replace {
		return Routine{}, errWrap("routine '" + normSlug + "' already exists; pass replace: true to overwrite")
	}

	entry := Routine{
		Slug: normSlug, Description: description, Schedule: sched,
		NextRun: nextRun, LastRun: nil, Prompt: prompt, Enabled: enabled,
	}
	if idx >= 0 {
		entry.LastRun = existing[idx].LastRun // preserva last_run ao substituir
		existing[idx] = entry
	} else {
		existing = append(existing, entry)
	}
	if err := save(existing); err != nil {
		return Routine{}, err
	}
	return entry, nil
}

// Remove apaga uma rotina; false se não existia.
func Remove(slugStr string) (bool, error) {
	existing, err := LoadAll()
	if err != nil {
		return false, err
	}
	for i := range existing {
		if existing[i].Slug == slugStr {
			existing = append(existing[:i], existing[i+1:]...)
			return true, save(existing)
		}
	}
	return false, nil
}

// ClaimDue reivindica as rotinas vencidas (next_run <= now): avança o next_run
// para o próximo tick após now, seta last_run = now e persiste. Retorna as
// vencidas com o last_run ANTERIOR (at-most-once: avança antes de executar).
func ClaimDue(now time.Time) ([]ClaimedRoutine, error) {
	existing, err := LoadAll()
	if err != nil {
		return nil, err
	}
	changed := false
	due := []ClaimedRoutine{}

	for i := range existing {
		r := existing[i]
		if !r.Enabled || r.Schedule == nil {
			continue
		}
		nr, ok := nextRunOrCompute(r, now)
		if !ok {
			continue
		}
		if !nr.After(now) { // nr <= now
			due = append(due, ClaimedRoutine{Routine: r, ClaimedAt: iso(now)})
			next, err := ComputeNextRun(*r.Schedule, now)
			if err != nil {
				return nil, err
			}
			ln, nrs := iso(now), iso(next)
			existing[i].LastRun = &ln
			existing[i].NextRun = &nrs
			changed = true
		} else if r.NextRun == nil || strings.TrimSpace(*r.NextRun) == "" {
			nrs := iso(nr)
			existing[i].NextRun = &nrs
			changed = true
		}
	}

	if changed {
		if err := save(existing); err != nil {
			return nil, err
		}
	}
	return due, nil
}

// DueDryRun devolve as rotinas que estariam vencidas em now, SEM mutar nada
// (para o --dry-run do run_due_routines).
func DueDryRun(now time.Time) ([]Routine, error) {
	en, err := Enabled()
	if err != nil {
		return nil, err
	}
	var due []Routine
	for _, r := range en {
		if r.Schedule == nil {
			continue
		}
		if nr, ok := nextRunOrCompute(r, now); ok && !nr.After(now) {
			due = append(due, r)
		}
	}
	return due, nil
}

// ComputeNextRun devolve o próximo tick do cron estritamente após base (UTC).
func ComputeNextRun(schedule string, base time.Time) (time.Time, error) {
	sched, err := cron.ParseStandard(schedule)
	if err != nil {
		return time.Time{}, errWrap("invalid cron expression: " + schedule)
	}
	return sched.Next(base.UTC()).UTC(), nil
}

func nextRunOrCompute(r Routine, now time.Time) (time.Time, bool) {
	if r.NextRun != nil {
		if t, err := parseTime(*r.NextRun); err == nil {
			return t, true
		}
	}
	t, err := ComputeNextRun(*r.Schedule, now)
	return t, err == nil
}

func save(routines []Routine) error {
	path := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	out, err := yaml.Marshal(map[string]any{"routines": routines})
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func normalize(m map[string]any) Routine {
	enabled := true
	if v, ok := m["enabled"]; ok {
		enabled = truthy(v)
	}
	return Routine{
		Slug:        str(m["slug"]),
		Description: str(m["description"]),
		Schedule:    blankToNil(m["schedule"]),
		NextRun:     blankToNil(m["next_run"]),
		LastRun:     blankToNil(m["last_run"]),
		Prompt:      str(m["prompt"]),
		Enabled:     enabled,
	}
}

func normalizeSchedule(schedule *string) (*string, error) {
	if schedule == nil {
		return nil, nil
	}
	s := strings.TrimSpace(*schedule)
	if s == "" {
		return nil, nil
	}
	if !cronRE.MatchString(s) {
		return nil, errWrap("schedule must be a 5-field cron expression (got " + s + ")")
	}
	if _, err := cron.ParseStandard(s); err != nil {
		return nil, errWrap("invalid cron expression: " + s)
	}
	return &s, nil
}

func blankToNil(v any) *string {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(x) == "" {
			return nil
		}
		return &x
	case time.Time:
		s := iso(x)
		return &s
	default:
		return nil
	}
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func truthy(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return v != nil
}

func iso(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func parseTime(s string) (time.Time, error) { return time.Parse(time.RFC3339, s) }

func errWrap(msg string) error { return errors.New("routines: " + msg) }
