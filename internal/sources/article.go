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

// browserUA é o user-agent usado no grab — um Chrome desktop realista.
const browserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36"

// stealthScript reduz sinais de automação (espelha o init script do Ruby).
const stealthScript = `Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
Object.defineProperty(navigator, 'plugins', { get: () => [1, 2, 3, 4, 5] });
Object.defineProperty(navigator, 'languages', { get: () => ['en-US', 'en'] });`

const articleTimeout = 30 * time.Second

// chromeFinder localiza um binário do Chrome/Chromium no sistema; var de pacote
// para os testes poderem forçar a ausência.
var chromeFinder = findChrome

// FetchArticleViaBrowser abre o tweet num Chrome headless do sistema (via CDP) e
// devolve o innerText do body — usado pra puxar o corpo completo de X Articles,
// que o syndication público só entrega como preview. Best-effort: sem Chrome
// instalado, ou em qualquer erro, devolve "" (o caller cai pro preview_text,
// idêntico ao comportamento do Ruby sem Playwright). É uma var pra teste.
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
		chromedp.Sleep(3*time.Second), // deixa o JS do X renderizar o artigo
		chromedp.Evaluate(`document.body.innerText`, &text),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "twitter: grab de artigo via browser falhou: %v\n", err)
		return ""
	}
	return text
}

// findChrome procura um Chrome/Chromium em locais comuns e no PATH.
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
		if _, err := os.Stat(c); err == nil { // caminho absoluto (macOS)
			return c, true
		}
	}
	return "", false
}
