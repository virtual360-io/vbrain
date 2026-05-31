package routines

import (
	_ "embed"
	"time"
)

//go:embed dream.prompt.md
var dreamPrompt string

// defaultRoutine describes a routine seeded by default at setup.
type defaultRoutine struct {
	slug, description, schedule, prompt string
	enabled                             bool
}

var defaults = []defaultRoutine{
	{
		slug:        "dream",
		description: "Nightly self-improvement: reads the query_log and reorganizes the wiki to answer better.",
		schedule:    "0 3 * * *",
		enabled:     true,
		prompt:      dreamPrompt,
	},
}

// SeedResult summarizes what the seed did.
type SeedResult struct {
	Seeded  []string `json:"seeded"`
	Skipped []string `json:"skipped"`
}

// SeedDefaults adds each default routine that doesn't exist yet; never
// overwrites the user's choice. Idempotent.
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
