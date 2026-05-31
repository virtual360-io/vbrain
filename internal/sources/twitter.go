package sources

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	nethttp "net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Twitter is the source for tweets/X (read via the public syndication API).
type Twitter struct{}

var tweetURLRE = regexp.MustCompile(
	`(?i)^(?:https?://)?(?:www\.|m\.|mobile\.)?(?:twitter\.com|x\.com)/(?P<user>[A-Za-z0-9_]+)/status/(?P<id>\d+)`,
)

// ErrNotTweet signals a URL that isn't a tweet (mirrors Twitter::FetchError in
// the parse_id context).
var ErrNotTweet = errors.New("not a tweet URL")

func (Twitter) KindKey() string { return "tweet" }

func (Twitter) Detect(input string) bool { return tweetURLRE.MatchString(input) }

// ParseID extracts the numeric status id from a tweet URL.
func (Twitter) ParseID(rawURL string) (string, error) {
	m := tweetURLRE.FindStringSubmatch(rawURL)
	if m == nil {
		return "", ErrNotTweet
	}
	return m[tweetURLRE.SubexpIndex("id")], nil
}

// ComputeToken reproduces the deterministic token the syndication API expects:
// id/1e15*PI, formatted like Ruby's Float#to_s, without trailing zeros and
// without the dot.
func (Twitter) ComputeToken(id string) string {
	idInt, _ := strconv.ParseInt(id, 10, 64)
	n := float64(idInt) / 1e15 * math.Pi
	s := strconv.FormatFloat(n, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0" // Float#to_s do Ruby sempre tem ponto decimal
	}
	s = strings.TrimRight(s, "0")
	return strings.ReplaceAll(s, ".", "")
}

const (
	syndicationURL     = "https://cdn.syndication.twimg.com/tweet-result"
	twitterUA          = "vbrain/1.0"
	syndicationTimeout = 10 * time.Second
)

// FetchSyndication fetches the tweet JSON via the public syndication API. A
// package var for deterministic override in tests.
var FetchSyndication = func(id string) (string, error) {
	token := (Twitter{}).ComputeToken(id)
	u := syndicationURL + "?id=" + id + "&lang=en&token=" + token
	req, err := nethttp.NewRequest(nethttp.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", twitterUA)
	req.Header.Set("Accept", "application/json")
	client := &nethttp.Client{Timeout: syndicationTimeout}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("syndication HTTP %d", res.StatusCode)
	}
	return string(body), nil
}

// CopyToRaw fetches the tweet JSON and writes it into raw/.
func (Twitter) CopyToRaw(input, rawDir, timestamp string) (RawInfo, error) {
	id, err := (Twitter{}).ParseID(input)
	if err != nil {
		return RawInfo{}, err
	}
	jsonStr, err := FetchSyndication(id)
	if err != nil {
		return RawInfo{}, err
	}
	basename := timestamp + "-tweet-" + id + ".json"
	dest := filepath.Join(rawDir, basename)
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		return RawInfo{}, err
	}
	if err := os.WriteFile(dest, []byte(jsonStr), 0o644); err != nil {
		return RawInfo{}, err
	}
	sum := sha256.Sum256([]byte(input + "\n" + jsonStr))
	return RawInfo{
		Path: dest, OriginalFilename: basename, SHA256: hex.EncodeToString(sum[:]),
		TweetID: id, JSON: jsonStr,
	}, nil
}

// Extract renders the tweet markdown (from the cache in info, or by fetching).
// If the tweet has an embedded article, it attempts the full grab via headless
// Chrome (best-effort).
func (Twitter) Extract(input, outPath string, info RawInfo) error {
	id := info.TweetID
	if id == "" {
		var err error
		if id, err = (Twitter{}).ParseID(input); err != nil {
			return err
		}
	}
	jsonStr := info.JSON
	if jsonStr == "" {
		var err error
		if jsonStr, err = FetchSyndication(id); err != nil {
			return err
		}
	}
	data, err := decodeJSON(jsonStr)
	if err != nil {
		return err
	}
	articleFull := ""
	if data["article"] != nil {
		articleFull = FetchArticleViaBrowser("https://x.com/i/status/" + id + "?s=20")
	}
	md, err := (Twitter{}).ExtractFromJSON(jsonStr, input, id, articleFull)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outPath, []byte(md), 0o644)
}

// ExtractFromJSON renders the page markdown from the tweet JSON. If
// articleFullText (>500 chars) is provided, it embeds the article body;
// otherwise it uses the public preview_text.
func (Twitter) ExtractFromJSON(jsonStr, url, id, articleFullText string) (string, error) {
	data, err := decodeJSON(jsonStr)
	if err != nil {
		return "", err
	}

	user := getMap(data, "user")
	handle, hasHandle := user["screen_name"]
	name := asStr(user["name"])
	createdAt, hasCreatedAt := data["created_at"]
	text := asStr(data["text"])
	lang, hasLang := data["lang"]
	favorites, hasFavorites := data["favorite_count"]
	article := getMap(data, "article")
	hasArticle := data["article"] != nil

	type ref struct{ display, expanded, shortened string }
	var urls []ref
	for _, raw := range getSlice(getMap(data, "entities"), "urls") {
		u, _ := raw.(map[string]any)
		urls = append(urls, ref{
			display:   asStr(u["display_url"]),
			expanded:  asStr(u["expanded_url"]),
			shortened: asStr(u["url"]),
		})
	}

	type med struct{ typ, url string }
	var media []med
	if arr, ok := data["mediaDetails"].([]any); ok {
		for _, raw := range arr {
			m, _ := raw.(map[string]any)
			mu := asStr(m["media_url_https"])
			if mu == "" {
				mu = asStr(m["media_url"])
			}
			media = append(media, med{typ: asStr(m["type"]), url: mu})
		}
	}

	textExpanded := text
	for _, u := range urls {
		if u.shortened != "" && u.expanded != "" {
			textExpanded = strings.ReplaceAll(textExpanded, u.shortened, u.expanded)
		}
	}

	titleName := name
	if titleName == "" && hasHandle {
		titleName = asStr(handle)
	}
	title := "Tweet by " + titleName + " (" + asStr(createdAt) + ")"

	var lines []string
	add := func(s string) { lines = append(lines, s) }

	add("# " + title)
	add("")
	add("- Source URL: " + url)
	add("- Tweet ID: " + id)
	if hasHandle {
		add("- Author: " + name + " (@" + asStr(handle) + ")")
	}
	if hasCreatedAt {
		add("- Date: " + asStr(createdAt))
	}
	if hasLang {
		add("- Language: " + asStr(lang))
	}
	if hasFavorites {
		add("- Likes (at ingestion time): " + asStr(favorites))
	}
	add("")
	add("## Tweet text")
	add("")
	if strings.TrimSpace(textExpanded) == "" {
		add("(tweet with no text — media or link only)")
	} else {
		add(strings.TrimSpace(textExpanded))
	}
	add("")

	if len(urls) > 0 {
		add("## Cited links")
		add("")
		for _, u := range urls {
			add("- [" + u.display + "](" + u.expanded + ")")
		}
		add("")
	}

	if len(media) > 0 {
		add("## Media")
		add("")
		for _, m := range media {
			add("- " + m.typ + ": " + m.url)
		}
		add("")
	}

	articleTitle := asStr(article["title"])
	articlePreview, hasPreview := article["preview_text"]
	if hasArticle && (articleTitle != "" || hasPreview) {
		add("## Embedded article")
		add("")
		if article["title"] != nil {
			add("- Article title: " + strings.TrimSpace(articleTitle))
		}
		if article["rest_id"] != nil {
			add("- Article ID: " + asStr(article["rest_id"]))
		}
		add("")
		if len(articleFullText) > 500 {
			cleaned := cleanArticleText(articleFullText, articleTitle)
			add("**Full body** (extracted via headless Chrome):")
			add("")
			add("```")
			add(cleaned)
			add("```")
			add("")
		} else {
			add("**Note**: the article's full body is only accessible with auth on X or via a real headless Chrome. The text below is the `preview_text` (~200 chars) delivered by the public syndication API — use it as a literal excerpt, don't infer the rest.")
			add("")
			if hasPreview {
				add("```")
				add(asStr(articlePreview))
				add("```")
				add("")
			}
		}
	}

	return strings.Join(lines, "\n") + "\n", nil
}

var multiNewlineRE = regexp.MustCompile(`\n{3,}`)

// CleanArticleText strips the X boilerplate from a raw article text, anchoring
// on the title when present.
func (Twitter) CleanArticleText(raw, title string) string { return cleanArticleText(raw, title) }

func cleanArticleText(raw, title string) string {
	text := raw
	if t := strings.TrimSpace(title); t != "" {
		if idx := strings.Index(text, t); idx >= 0 {
			text = text[idx:]
		}
	}
	markers := []string{
		"About\n |\nDownload the X app",
		"© 2026 X Corp.",
		"Don’t miss what",
		"People on X are the first",
		"Log in\nSign up",
	}
	for _, marker := range markers {
		if cut := strings.Index(text, marker); cut >= 0 {
			text = text[:cut]
		}
	}
	return strings.TrimSpace(multiNewlineRE.ReplaceAllString(text, "\n\n"))
}

// --- JSON helpers (UseNumber, so numbers become json.Number) ---

func decodeJSON(s string) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader([]byte(s)))
	dec.UseNumber()
	var out map[string]any
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func getMap(m map[string]any, key string) map[string]any {
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	return map[string]any{}
}

func getSlice(m map[string]any, key string) []any {
	if v, ok := m[key].([]any); ok {
		return v
	}
	return nil
}

func asStr(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	case bool:
		return strconv.FormatBool(x)
	default:
		return ""
	}
}
