package routines_test

import (
	"strings"
	"testing"

	"github.com/virtual360-io/vbrain/internal/routines"
)

func TestSeedDefaultsAddsSoulAndDreamOnceIdempotent(t *testing.T) {
	isolate(t)
	r1, err := routines.SeedDefaults(false, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(r1.Seeded) != 2 || !contains(r1.Seeded, "soul") || !contains(r1.Seeded, "dream") {
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

	// Soul runs before dream so it reads the query log before dream drains it,
	// and its prompt encodes the non-negotiable identity invariants.
	soul, _ := routines.Find("soul")
	if soul == nil || soul.Schedule == nil || *soul.Schedule != "0 2 * * *" || !soul.Enabled {
		t.Fatalf("soul = %+v", soul)
	}
	if !strings.Contains(soul.Prompt, "vbrain soul-write") {
		t.Errorf("soul prompt must use the soul-write writer")
	}
	if !strings.Contains(soul.Prompt, "Lean") || !strings.Contains(soul.Prompt, "contradiction") {
		t.Errorf("soul prompt must encode the lean + no-contradiction rules")
	}

	// Second call: skip both, no duplicates.
	r2, _ := routines.SeedDefaults(false, fixedNow)
	if len(r2.Seeded) != 0 || len(r2.Skipped) != 2 {
		t.Fatalf("r2 = %+v", r2)
	}
	all, _ := routines.LoadAll()
	if len(all) != 2 {
		t.Fatalf("total = %d, want 2", len(all))
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
