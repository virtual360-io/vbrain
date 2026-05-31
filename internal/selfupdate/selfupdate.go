// Package selfupdate baixa o binário vbrain mais recente da release rolling
// "latest" no GitHub e substitui o executável em uso (verificando o SHA256).
package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Repo é o repositório de releases do vbrain.
const Repo = "virtual360-io/vbrain"

// DefaultBaseURL aponta pra tag rolling "latest" (independe da flag prerelease,
// ao contrário de /releases/latest/).
var DefaultBaseURL = "https://github.com/" + Repo + "/releases/download/latest"

// Result resume o update (JSON no stdout).
type Result struct {
	Asset   string `json:"asset"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Updated bool   `json:"updated"`
}

// AssetName devolve o nome do binário pra plataforma atual (ex.:
// vbrain-linux-amd64, vbrain-windows-amd64.exe).
func AssetName() string {
	n := "vbrain-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		n += ".exe"
	}
	return n
}

// Run atualiza o executável atual a partir da release latest.
func Run() (Result, error) {
	exe, err := os.Executable()
	if err != nil {
		return Result{}, err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return update(exe, DefaultBaseURL, &http.Client{Timeout: 60 * time.Second})
}

// update baixa AssetName() de baseURL, confere o SHA256 (de SHA256SUMS) e troca
// o binário em targetPath atomicamente. Parametrizado para teste.
func update(targetPath, baseURL string, client *http.Client) (Result, error) {
	asset := AssetName()

	want, err := wantedSHA(client, baseURL, asset)
	if err != nil {
		return Result{}, err
	}

	data, err := fetch(client, baseURL+"/"+asset)
	if err != nil {
		return Result{}, err
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if got != want {
		return Result{}, fmt.Errorf("sha256 não confere para %s: esperado %s, baixado %s", asset, want, got)
	}

	if err := replaceBinary(targetPath, data); err != nil {
		return Result{}, err
	}
	return Result{Asset: asset, Path: targetPath, SHA256: got, Updated: true}, nil
}

// wantedSHA busca SHA256SUMS e extrai o hash do asset.
func wantedSHA(client *http.Client, baseURL, asset string) (string, error) {
	sums, err := fetch(client, baseURL+"/SHA256SUMS")
	if err != nil {
		return "", fmt.Errorf("não consegui baixar SHA256SUMS: %w", err)
	}
	for _, line := range strings.Split(string(sums), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[1] == asset {
			return f[0], nil
		}
	}
	return "", fmt.Errorf("asset %s ausente no SHA256SUMS", asset)
}

func fetch(client *http.Client, url string) ([]byte, error) {
	res, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d para %s", res.StatusCode, url)
	}
	return io.ReadAll(res.Body)
}

// replaceBinary grava o novo binário sobre targetPath atomicamente. Escreve num
// tmp no mesmo diretório (mesmo filesystem → rename atômico). No Windows, move o
// atual pra .old antes (não dá pra renomear sobre um exe em uso).
func replaceBinary(targetPath string, data []byte) error {
	dir := filepath.Dir(targetPath)
	tmp, err := os.CreateTemp(dir, ".vbrain-new-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		old := targetPath + ".old"
		os.Remove(old)
		if err := os.Rename(targetPath, old); err != nil {
			return err
		}
		if err := os.Rename(tmpPath, targetPath); err != nil {
			os.Rename(old, targetPath) // rollback
			return err
		}
		os.Remove(old) // best-effort (pode falhar se ainda em uso)
		return nil
	}
	return os.Rename(tmpPath, targetPath)
}
