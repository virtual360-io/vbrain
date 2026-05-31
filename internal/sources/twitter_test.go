package sources_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/virtual360-io/vbrain/internal/sources"
)

const (
	tweetURL = "https://x.com/alokbishoyi97/status/2059610305408462898"
	tweetID  = "2059610305408462898"
)

func fixtureJSON(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "twitter", "alok_link_tweet.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func extractFrom(t *testing.T, json, url, id, full string) string {
	t.Helper()
	md, err := (sources.Twitter{}).ExtractFromJSON(json, url, id, full)
	if err != nil {
		t.Fatal(err)
	}
	return md
}

func TestTwitterDetectXAndTwitterUrls(t *testing.T) {
	for _, u := range []string{
		"https://x.com/alok/status/123",
		"https://twitter.com/alok/status/123",
		"http://www.twitter.com/alok/status/123",
		"https://mobile.x.com/alok/status/123",
	} {
		if !(sources.Twitter{}).Detect(u) {
			t.Errorf("should detect %q", u)
		}
	}
}

func TestTwitterDetectRejectsNonTweetUrls(t *testing.T) {
	for _, u := range []string{
		"https://x.com/alok",
		"https://x.com/alok/photo/123",
		"https://example.com/x/alok/status/123",
		"/tmp/foo.txt",
	} {
		if (sources.Twitter{}).Detect(u) {
			t.Errorf("should not detect %q", u)
		}
	}
}

func TestTwitterKindKey(t *testing.T) {
	if (sources.Twitter{}).KindKey() != "tweet" {
		t.Fatal("kind_key should be tweet")
	}
}

func TestTwitterParseIDExtractsNumericStatusID(t *testing.T) {
	got, err := (sources.Twitter{}).ParseID(tweetURL)
	if err != nil || got != tweetID {
		t.Fatalf("ParseID(%q) = %q, %v", tweetURL, got, err)
	}
	got, err = (sources.Twitter{}).ParseID("https://twitter.com/foo/status/987")
	if err != nil || got != "987" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestTwitterParseIDRaisesForNonTweet(t *testing.T) {
	if _, err := (sources.Twitter{}).ParseID("https://x.com/foo"); err == nil {
		t.Fatal("should fail for a non-tweet")
	}
}

func TestTwitterComputeTokenDeterministicNonEmpty(t *testing.T) {
	a := (sources.Twitter{}).ComputeToken(tweetID)
	b := (sources.Twitter{}).ComputeToken(tweetID)
	if a != b {
		t.Fatalf("not deterministic: %q != %q", a, b)
	}
	if a == "" {
		t.Fatal("empty token")
	}
	if !regexp.MustCompile(`^[0-9]+$`).MatchString(a) {
		t.Fatalf("token should be digits only: %q", a)
	}
}

func TestTwitterExtractFromJSONRendersMetadataAndText(t *testing.T) {
	md := extractFrom(t, fixtureJSON(t), tweetURL, tweetID, "")
	for _, want := range []string{
		"# Tweet by Alok Bishoyi",
		"- Tweet ID: " + tweetID,
		"- Author: Alok Bishoyi (@alokbishoyi97)",
		"- Date: 2026-05-27",
		"- Language: zxx",
		"## Tweet text",
		"http://x.com/i/article/2059581224960835584",
		"## Cited links",
		"[x.com/i/article/2059…](http://x.com/i/article/2059581224960835584)",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("md missing %q", want)
		}
	}
}

func TestTwitterExtractFromJSONRendersEmbeddedArticlePreview(t *testing.T) {
	md := extractFrom(t, fixtureJSON(t), tweetURL, tweetID, "")
	for _, want := range []string{
		"## Embedded article",
		"Using Autoresearch to improve harness skills",
		"self-improving agents are here",
		"the article's full body is only accessible",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("md missing %q", want)
		}
	}
}

func TestTwitterExtractFromJSONIncludesFullArticleWhenProvided(t *testing.T) {
	full := "Using Autoresearch to improve harness skills\n\nself-improving agents are here\nThe most interesting shift in AI right now... (and a lot more content that exceeds the preview length significantly to trigger the threshold). " +
		strings.Repeat("Lorem ipsum dolor sit amet, consectetur adipiscing elit. ", 8)
	md := extractFrom(t, fixtureJSON(t), tweetURL, tweetID, full)
	if !strings.Contains(md, "Full body") || !strings.Contains(md, "headless Chrome") {
		t.Error("should embed the full body via headless Chrome")
	}
	if strings.Contains(md, "the article's full body is only accessible") {
		t.Error("should not show the preview note when there's a full body")
	}
	if !strings.Contains(md, "and a lot more content") {
		t.Error("should contain the article text")
	}
}

func TestTwitterCleanArticleTextStripsXBoilerplate(t *testing.T) {
	raw := "Don’t miss what’s happening\nLog in\nThe Title\n\nbody starts here\n\n© 2026 X Corp."
	cleaned := (sources.Twitter{}).CleanArticleText(raw, "The Title")
	if strings.Contains(cleaned, "© 2026 X Corp.") || strings.Contains(cleaned, "Don’t miss") {
		t.Errorf("boilerplate not removed: %q", cleaned)
	}
	if !strings.Contains(cleaned, "body starts here") {
		t.Errorf("corpo perdido: %q", cleaned)
	}
}

func TestTwitterExtractFromJSONSkipsArticleSectionWhenAbsent(t *testing.T) {
	fake := `{"user":{"name":"X","screen_name":"x"},"created_at":"2026-01-01T00:00:00Z","text":"hello"}`
	md := extractFrom(t, fake, "https://x.com/x/status/1", "1", "")
	if strings.Contains(md, "Embedded article") {
		t.Error("should not have an article section")
	}
}

func TestTwitterExtractFromJSONSignalsEmptyTextWhenOnlyLink(t *testing.T) {
	fake := `{"user":{"name":"X","screen_name":"x"},"created_at":"2026-01-01T00:00:00Z","text":"https://t.co/abc","entities":{"urls":[{"url":"https://t.co/abc","expanded_url":"https://elsewhere.test/article","display_url":"elsewhere.test/article"}]}}`
	md := extractFrom(t, fake, "https://x.com/x/status/1", "1", "")
	if !strings.Contains(md, "https://elsewhere.test/article") {
		t.Error("should include the expanded link in the references")
	}
}

func TestTwitterExtractFromJSONRendersMediaWhenPresent(t *testing.T) {
	fake := `{"user":{"name":"X","screen_name":"x"},"created_at":"2026-01-01T00:00:00Z","text":"foo","mediaDetails":[{"type":"photo","media_url_https":"https://pbs.test/img.jpg"}]}`
	md := extractFrom(t, fake, "https://x.com/x/status/1", "1", "")
	if !strings.Contains(md, "## Media") || !strings.Contains(md, "photo: https://pbs.test/img.jpg") {
		t.Errorf("media not rendered: %q", md)
	}
}

func TestDispatcherPrefersTwitterOverUrlForTweet(t *testing.T) {
	s := sources.Detect("https://x.com/foo/status/1")
	if s == nil || s.KindKey() != "tweet" {
		t.Fatalf("got %v", s)
	}
}

func TestDispatcherFallsBackToUrlForNonTweetHttps(t *testing.T) {
	s := sources.Detect("https://example.com/article")
	if s == nil || s.KindKey() != "url" {
		t.Fatalf("got %v", s)
	}
}
