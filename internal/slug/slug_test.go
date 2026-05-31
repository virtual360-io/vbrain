package slug

import (
	"strings"
	"testing"
)

func mustFrom(t *testing.T, title string) string {
	t.Helper()
	s, err := From(title)
	if err != nil {
		t.Fatalf("From(%q) unexpected error: %v", title, err)
	}
	return s
}

func TestBasicASCIILowercase(t *testing.T) {
	if got := mustFrom(t, "Hello World"); got != "hello-world" {
		t.Fatalf("got %q, want hello-world", got)
	}
}

func TestNFKDStripsDiacritics(t *testing.T) {
	cases := map[string]string{
		"São José": "sao-jose",
		"açúcar":   "acucar",
		"naïve":    "naive",
	}
	for in, want := range cases {
		if got := mustFrom(t, in); got != want {
			t.Errorf("From(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCollapsesSeparatorsAndPunctuation(t *testing.T) {
	if got := mustFrom(t, "foo___bar... baz!!!"); got != "foo-bar-baz" {
		t.Fatalf("got %q, want foo-bar-baz", got)
	}
}

func TestTrimsLeadingAndTrailing(t *testing.T) {
	if got := mustFrom(t, "---foo bar---"); got != "foo-bar" {
		t.Fatalf("got %q, want foo-bar", got)
	}
}

func TestTruncatesToMaxLengthWithoutTrailingDash(t *testing.T) {
	long := strings.Repeat("a", 100) + " " + strings.Repeat("b", 100)
	s, err := FromMax(long, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(s) > 50 {
		t.Errorf("len(%q) = %d, want <= 50", s, len(s))
	}
	if strings.HasSuffix(s, "-") {
		t.Errorf("should not end with a dash after truncation: %q", s)
	}
}

func TestEmptyOrPunctOnlyRaises(t *testing.T) {
	for _, in := range []string{"", "   ", "!!! ??? ..."} {
		if _, err := From(in); err == nil {
			t.Errorf("From(%q) should fail", in)
		}
	}
}

func TestPreservesDigits(t *testing.T) {
	if got := mustFrom(t, "Version 2.0"); got != "version-2-0" {
		t.Fatalf("got %q, want version-2-0", got)
	}
}
