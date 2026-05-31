package sources

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// browserUA is the user-agent used for the grab — a realistic desktop Chrome.
const browserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36"

// stealthScript reduces automation signals (mirrors the Ruby init script).
const stealthScript = `Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
Object.defineProperty(navigator, 'plugins', { get: () => [1, 2, 3, 4, 5] });
Object.defineProperty(navigator, 'languages', { get: () => ['en-US', 'en'] });`

const articleTimeout = 30 * time.Second

// chromeFinder locates a Chrome/Chromium binary on the system; a package var so
// tests can force its absence.
var chromeFinder = findChrome

// FetchArticleViaBrowser opens the tweet in the system's headless Chrome (via
// CDP) and returns the body's innerText — used to pull the full body of X
// Articles, which the public syndication API only delivers as a preview.
// Best-effort: with no Chrome installed, or on any error, it returns "" (the
// caller falls back to preview_text, identical to the Ruby behavior without
// Playwright). It's a var for testing.
var FetchArticleViaBrowser = func(tweetURL string) string {
	chromePath, ok := chromeFinder()
	if !ok {
		return ""
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.NoSandbox,
		chromedp.UserAgent(browserUA),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()
	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	defer cancelCtx()
	ctx, cancelTimeout := context.WithTimeout(ctx, articleTimeout)
	defer cancelTimeout()

	var text string
	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(stealthScript).Do(ctx)
			return err
		}),
		chromedp.Navigate(tweetURL),
		chromedp.Sleep(3*time.Second), // let X's JS render the article
		chromedp.Evaluate(`document.body.innerText`, &text),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "twitter: article grab via browser failed: %v\n", err)
		return ""
	}
	return text
}

// findChrome looks for a Chrome/Chromium in common locations and on the PATH.
func findChrome() (string, bool) {
	candidates := []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome"}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates,
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		)
	}
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p, true
		}
		if _, err := os.Stat(c); err == nil { // absolute path (macOS)
			return c, true
		}
	}
	return "", false
}
