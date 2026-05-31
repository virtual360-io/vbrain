package sources

import "testing"

// White-box: forces the absence of Chrome and ensures graceful degradation (returns
// "" sem erro/panic) — o caller cai pro preview_text.
func TestFetchArticleDegradesWithoutChrome(t *testing.T) {
	orig := chromeFinder
	chromeFinder = func() (string, bool) { return "", false }
	defer func() { chromeFinder = orig }()

	if got := FetchArticleViaBrowser("https://x.com/i/status/1?s=20"); got != "" {
		t.Fatalf("with no Chrome it should return \"\", got %q", got)
	}
}
