package routines

import (
	_ "embed"
	"time"
)

//go:embed dream.prompt.md
var dreamPrompt string

// defaultRoutine descreve uma rotina semeada por padrão no setup.
type defaultRoutine struct {
	slug, description, schedule, prompt string
	enabled                            bool
}

var defaults = []defaultRoutine{
	{
		slug:        "dream",
		description: "Auto-melhoria noturna: lê o query_log e reorganiza a wiki pra responder melhor.",
		schedule:    "0 3 * * *",
		enabled:     true,
		prompt:      dreamPrompt,
	},
}

// SeedResult resume o que o seed fez.
type SeedResult struct {
	Seeded  []string `json:"seeded"`
	Skipped []string `json:"skipped"`
}

// SeedDefaults adiciona cada rotina-padrão que ainda não existe; nunca
// sobrescreve a escolha do usuário. Idempotente.
func SeedDefaults(dryRun bool, now time.Time) (SeedResult, error) {
	res := SeedResult{Seeded: []string{}, Skipped: []string{}}
	for _, d := range defaults {
		existing, err := Find(d.slug)
		if err != nil {
			return res, err
		}
		if existing != nil {
			res.Skipped = append(res.Skipped, d.slug)
			continue
		}
		res.Seeded = append(res.Seeded, d.slug)
		if dryRun {
			continue
		}
		sched := d.schedule
		if _, err := Add(d.slug, d.description, d.prompt, &sched, d.enabled, false, now); err != nil {
			return res, err
		}
	}
	return res, nil
}
