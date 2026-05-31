package sources

import "testing"

// White-box: força a ausência de Chrome e garante degradação graciosa (devolve
// "" sem erro/panic) — o caller cai pro preview_text.
func TestFetchArticleDegradesWithoutChrome(t *testing.T) {
	orig := chromeFinder
	chromeFinder = func() (string, bool) { return "", false }
	defer func() { chromeFinder = orig }()

	if got := FetchArticleViaBrowser("https://x.com/i/status/1?s=20"); got != "" {
		t.Fatalf("sem Chrome deveria devolver \"\", got %q", got)
	}
}
