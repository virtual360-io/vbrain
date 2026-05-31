package sources

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/virtual360-io/vbrain/internal/slug"
)

// URL is the source for web pages, read as markdown via r.jina.ai.
type URL struct{}

var urlRE = regexp.MustCompile(`(?i)^https?://`)

const (
	jinaBase     = "https://r.jina.ai"
	urlUserAgent = "vbrain/1.0"
	urlTimeout   = 30 * time.Second
)

// FetchJina fetches the URL as markdown via r.jina.ai. It's a package var to
// allow deterministic override in tests (Rule 9).
var FetchJina = func(target string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, jinaBase+"/"+target, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/markdown")
	req.Header.Set("User-Agent", urlUserAgent)
	if token := os.Getenv("JINA_API_KEY"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: urlTimeout}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		snippet := string(body)
		if len(snippet) > 300 {
			snippet = snippet[:300]
		}
		return "", fmt.Errorf("jina HTTP %d for %s: %s", res.StatusCode, target, snippet)
	}
	if strings.TrimSpace(string(body)) == "" {
		return "", fmt.Errorf("jina empty body for %s", target)
	}
	return string(body), nil
}

func (URL) KindKey() string { return "url" }

func (URL) Detect(input string) bool { return urlRE.MatchString(input) }

// CopyToRaw fetches the markdown and writes it into raw/ with a host-derived name.
func (URL) CopyToRaw(rawURL, rawDir, timestamp string) (RawInfo, error) {
	markdown, err := FetchJina(rawURL)
	if err != nil {
		return RawInfo{}, err
	}
	host := "url"
	if u, err := url.Parse(rawURL); err == nil && u.Hostname() != "" {
		host = u.Hostname()
	}
	hostSlug, err := slug.From(host)
	if err != nil {
		return RawInfo{}, err
	}
	basename := fmt.Sprintf("%s-%s.md", timestamp, hostSlug)
	dest := filepath.Join(rawDir, basename)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return RawInfo{}, err
	}
	if err := os.WriteFile(dest, []byte(markdown), 0o644); err != nil {
		return RawInfo{}, err
	}
	sum := sha256.Sum256([]byte(rawURL + "\n" + markdown))
	return RawInfo{
		Path:             dest,
		OriginalFilename: basename,
		SHA256:           hex.EncodeToString(sum[:]),
		Markdown:         markdown,
	}, nil
}

// Extract writes the markdown (from the cache in rawInfo, or by fetching) to out_path.
func (URL) Extract(rawURL, outPath string, rawInfo RawInfo) error {
	md := rawInfo.Markdown
	if md == "" {
		var err error
		if md, err = FetchJina(rawURL); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outPath, []byte(md), 0o644)
}
