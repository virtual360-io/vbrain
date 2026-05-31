package links

import (
	"reflect"
	"testing"
)

func slugsOf(body string) []string {
	var out []string
	for _, l := range Extract(body) {
		out = append(out, l.Slug)
	}
	return out
}

func TestExtractsSingleWikilink(t *testing.T) {
	got := Extract("texto com [[Foo Bar]] no meio")
	if len(got) != 1 || got[0].Slug != "foo-bar" || got[0].Title != "Foo Bar" {
		t.Fatalf("got %+v", got)
	}
}

func TestExtractsMultipleInOrder(t *testing.T) {
	if got := slugsOf("[[Alpha]] e [[Beta]] e [[Gamma]]."); !reflect.DeepEqual(got, []string{"alpha", "beta", "gamma"}) {
		t.Fatalf("got %v", got)
	}
}

func TestWikilinkAliasKeepsTargetForSlugAndTitle(t *testing.T) {
	got := Extract("ver [[Postgres Logical|replicação]]")
	if got[0].Slug != "postgres-logical" || got[0].Title != "Postgres Logical" {
		t.Fatalf("got %+v", got[0])
	}
}

func TestDedupsBySlug(t *testing.T) {
	if got := slugsOf("[[X]] e de novo [[X]] e [[X|outro]]"); !reflect.DeepEqual(got, []string{"x"}) {
		t.Fatalf("got %v", got)
	}
}

func TestParsesMarkdownLinksToMdFiles(t *testing.T) {
	got := Extract("veja [Família de Victor](familia-de-victor.md) aqui")
	if got[0].Slug != "familia-de-victor" || got[0].Title != "Família de Victor" {
		t.Fatalf("got %+v", got[0])
	}
}

func TestBothFormsDedupBySlug(t *testing.T) {
	body := "[[Família de Victor]] e depois [Família de Victor](familia-de-victor.md)"
	if got := slugsOf(body); !reflect.DeepEqual(got, []string{"familia-de-victor"}) {
		t.Fatalf("got %v", got)
	}
}

func TestIgnoresExternalAndNonMdMarkdownLinks(t *testing.T) {
	if got := slugsOf("[Google](https://google.com) e [foto](pic.png)"); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestEmptyAndNil(t *testing.T) {
	for _, in := range []string{"sem link [x] (y)", "", "[[]] e [[   ]]"} {
		if got := Extract(in); len(got) != 0 {
			t.Errorf("Extract(%q) = %v, want empty", in, got)
		}
	}
}

func TestLinkifyConvertsResolvableWikilink(t *testing.T) {
	out := Linkify("liga pra [[Família de Victor]].", []string{"familia-de-victor"})
	if out != "liga pra [Família de Victor](familia-de-victor.md)." {
		t.Fatalf("got %q", out)
	}
}

func TestLinkifyLeavesUnresolvableUntouched(t *testing.T) {
	body := "liga pra [[Página Inexistente]]."
	if out := Linkify(body, []string{"familia-de-victor"}); out != body {
		t.Fatalf("got %q", out)
	}
}

func TestLinkifyAliasUsesDisplayTextAndTargetSlug(t *testing.T) {
	out := Linkify("ver [[Postgres Logical|replicação lógica]]", []string{"postgres-logical"})
	if out != "ver [replicação lógica](postgres-logical.md)" {
		t.Fatalf("got %q", out)
	}
}

func TestLinkifyIsIdempotent(t *testing.T) {
	body := "liga pra [[Família de Victor]]."
	once := Linkify(body, []string{"familia-de-victor"})
	twice := Linkify(once, []string{"familia-de-victor"})
	if once != twice {
		t.Fatalf("not idempotent: %q != %q", once, twice)
	}
}

func TestLinkifyAcceptsArrayOfSlugs(t *testing.T) {
	if out := Linkify("[[A]]", []string{"a"}); out != "[A](a.md)" {
		t.Fatalf("got %q", out)
	}
}

func TestApplyResolutionMapsTitleToChosenSlug(t *testing.T) {
	out := ApplyResolution("trabalha na [[V360]].", map[string]string{"V360": "carreira-de-victor"})
	if out != "trabalha na [V360](carreira-de-victor.md)." {
		t.Fatalf("got %q", out)
	}
}

func TestApplyResolutionAliasUsesDisplay(t *testing.T) {
	out := ApplyResolution("ver [[Empresa V360|a V360]]", map[string]string{"Empresa V360": "v360"})
	if out != "ver [a V360](v360.md)" {
		t.Fatalf("got %q", out)
	}
}

func TestApplyResolutionLeavesNullOrAbsentUntouched(t *testing.T) {
	body := "[[UFRJ]] e [[Outra]]"
	// slug "" (LLM found nothing) and a title absent from the map stay intact.
	if out := ApplyResolution(body, map[string]string{"UFRJ": ""}); out != body {
		t.Fatalf("got %q", out)
	}
}

func TestApplyResolutionEmptyMapNoop(t *testing.T) {
	body := "[[X]]"
	if out := ApplyResolution(body, map[string]string{}); out != body {
		t.Fatalf("got %q", out)
	}
}
