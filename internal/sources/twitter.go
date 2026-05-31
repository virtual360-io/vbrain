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

// Twitter é a fonte para tweets/X (lidos via API pública de syndication).
type Twitter struct{}

var tweetURLRE = regexp.MustCompile(
	`(?i)^(?:https?://)?(?:www\.|m\.|mobile\.)?(?:twitter\.com|x\.com)/(?P<user>[A-Za-z0-9_]+)/status/(?P<id>\d+)`,
)

// ErrNotTweet sinaliza URL que não é de tweet (espelha Twitter::FetchError no
// contexto de parse_id).
var ErrNotTweet = errors.New("not a tweet URL")

func (Twitter) KindKey() string { return "tweet" }

func (Twitter) Detect(input string) bool { return tweetURLRE.MatchString(input) }

// ParseID extrai o id numérico do status de uma URL de tweet.
func (Twitter) ParseID(rawURL string) (string, error) {
	m := tweetURLRE.FindStringSubmatch(rawURL)
	if m == nil {
		return "", ErrNotTweet
	}
	return m[tweetURLRE.SubexpIndex("id")], nil
}

// ComputeToken reproduz o token determinístico que a API de syndication espera:
// id/1e15*PI, formatado como o Float#to_s do Ruby, sem zeros à direita e sem o
// ponto.
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

// FetchSyndication busca o JSON do tweet via API pública de syndication. Var de
// pacote para override determinístico nos testes.
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

// FetchArticleViaPlaywright é o grab opcional do corpo de artigo via browser.
// Ainda não portado para Go; retorna "" (degrada para preview_text, idêntico ao
// Ruby quando o Playwright não está disponível). É o único recurso de sources
// pendente de port.
var FetchArticleViaPlaywright = func(tweetURL string) string { return "" }

// CopyToRaw busca o JSON do tweet e grava em raw/.
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

// Extract renderiza o markdown do tweet (do cache em info, ou buscando). Se o
// tweet tem artigo embutido, tenta o grab completo via Playwright (best-effort).
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
		articleFull = FetchArticleViaPlaywright("https://x.com/i/status/" + id + "?s=20")
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

// ExtractFromJSON renderiza o markdown da página a partir do JSON do tweet. Se
// articleFullText (>500 chars) for fornecido, embute o corpo do artigo; senão
// usa o preview_text público.
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
	title := "Tweet de " + titleName + " (" + asStr(createdAt) + ")"

	var lines []string
	add := func(s string) { lines = append(lines, s) }

	add("# " + title)
	add("")
	add("- Source URL: " + url)
	add("- Tweet ID: " + id)
	if hasHandle {
		add("- Autor: " + name + " (@" + asStr(handle) + ")")
	}
	if hasCreatedAt {
		add("- Data: " + asStr(createdAt))
	}
	if hasLang {
		add("- Idioma: " + asStr(lang))
	}
	if hasFavorites {
		add("- Likes (no momento da ingestão): " + asStr(favorites))
	}
	add("")
	add("## Texto do tweet")
	add("")
	if strings.TrimSpace(textExpanded) == "" {
		add("(tweet sem texto — apenas mídia ou link)")
	} else {
		add(strings.TrimSpace(textExpanded))
	}
	add("")

	if len(urls) > 0 {
		add("## Links citados")
		add("")
		for _, u := range urls {
			add("- [" + u.display + "](" + u.expanded + ")")
		}
		add("")
	}

	if len(media) > 0 {
		add("## Mídia")
		add("")
		for _, m := range media {
			add("- " + m.typ + ": " + m.url)
		}
		add("")
	}

	articleTitle := asStr(article["title"])
	articlePreview, hasPreview := article["preview_text"]
	if hasArticle && (articleTitle != "" || hasPreview) {
		add("## Artigo embutido")
		add("")
		if article["title"] != nil {
			add("- Artigo título: " + strings.TrimSpace(articleTitle))
		}
		if article["rest_id"] != nil {
			add("- Artigo ID: " + asStr(article["rest_id"]))
		}
		add("")
		if len(articleFullText) > 500 {
			cleaned := cleanArticleText(articleFullText, articleTitle)
			add("**Body completo** (extraído via Playwright + Chrome do sistema):")
			add("")
			add("```")
			add(cleaned)
			add("```")
			add("")
		} else {
			add("**Nota**: o body completo do artigo só é acessível com auth no X ou via Playwright/Chrome real. O texto abaixo é o `preview_text` (~200 chars) entregue pelo syndication público — use como excerpt literal, não infira o resto.")
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

// CleanArticleText remove o boilerplate do X de um texto de artigo bruto,
// ancorando no título quando presente.
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

// --- helpers de JSON (UseNumber, então números viram json.Number) ---

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
