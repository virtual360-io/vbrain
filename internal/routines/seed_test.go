package routines_test

import (
	"strings"
	"testing"

	"github.com/virtual360-io/vbrain/internal/routines"
)

func TestSeedDefaultsAddsDreamOnceIdempotent(t *testing.T) {
	isolate(t)
	r1, err := routines.SeedDefaults(false, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(r1.Seeded) != 1 || r1.Seeded[0] != "dream" {
		t.Fatalf("seeded = %v", r1.Seeded)
	}

	dream, _ := routines.Find("dream")
	if dream == nil || dream.Schedule == nil || *dream.Schedule != "0 3 * * *" || !dream.Enabled {
		t.Fatalf("dream = %+v", dream)
	}
	if dream.NextRun == nil {
		t.Error("dream sem next_run")
	}
	// prompt adapted to Go: calls the vbrain binary, not Ruby.
	if !strings.Contains(dream.Prompt, "vbrain query-log") || strings.Contains(dream.Prompt, "bundle exec ruby") {
		t.Errorf("prompt not adapted to Go")
	}

	// Second call: skip, doesn't duplicate.
	r2, _ := routines.SeedDefaults(false, fixedNow)
	if len(r2.Seeded) != 0 || len(r2.Skipped) != 1 {
		t.Fatalf("r2 = %+v", r2)
	}
	all, _ := routines.LoadAll()
	if len(all) != 1 {
		t.Fatalf("total = %d, want 1", len(all))
	}
}
