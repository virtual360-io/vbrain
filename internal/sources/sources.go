// Package sources detects the type of an ingest input and converts it into
// markdown. Port of lib/vbrain/sources*.rb. Detection is deterministic (Rule 5);
// judgment (chunk/synthesis) stays in the sub-agents.
package sources

// Source is an ingestable knowledge source. Detection and identity are
// deterministic (Rule 5).
type Source interface {
	KindKey() string
	Detect(input string) bool
}

// Ingestable is a Source that knows how to copy the input into raw/ and extract
// its markdown. All concrete sources (Text/URL/Twitter) implement this.
type Ingestable interface {
	Source
	CopyToRaw(input, rawDir, timestamp string) (RawInfo, error)
	Extract(input, outPath string, info RawInfo) error
}

// Registry defines the precedence order: Twitter beats URL (a tweet is a URL),
// which beats Text.
var Registry = []Source{Twitter{}, URL{}, Text{}}

// Detect returns the first Source that recognizes the input, or nil.
func Detect(input string) Source {
	for _, s := range Registry {
		if s.Detect(input) {
			return s
		}
	}
	return nil
}

// For looks up a Source by kind_key.
func For(kindKey string) Source {
	for _, s := range Registry {
		if s.KindKey() == kindKey {
			return s
		}
	}
	return nil
}

// Kinds lists the registered kind_keys.
func Kinds() []string {
	out := make([]string, len(Registry))
	for i, s := range Registry {
		out[i] = s.KindKey()
	}
	return out
}

// RawInfo is the result of copying an input into raw/ (fields depend on the
// source: markdown for URL, json/tweet_id for tweet).
type RawInfo struct {
	Path             string
	OriginalFilename string
	SHA256           string
	Markdown         string
	TweetID          string
	JSON             string
}
