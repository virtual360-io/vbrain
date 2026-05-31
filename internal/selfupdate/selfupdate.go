// Package selfupdate downloads the latest vbrain binary from the rolling
// "latest" GitHub release and replaces the running executable (verifying the
// SHA256).
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

// Repo is vbrain's release repository.
const Repo = "virtual360-io/vbrain"

// DefaultBaseURL points at the rolling "latest" tag (independent of the
// prerelease flag, unlike /releases/latest/).
var DefaultBaseURL = "https://github.com/" + Repo + "/releases/download/latest"

// Result summarizes the update (JSON on stdout).
type Result struct {
	Asset   string `json:"asset"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Updated bool   `json:"updated"`
}

// AssetName returns the binary name for the current platform (e.g.
// vbrain-linux-amd64, vbrain-windows-amd64.exe).
func AssetName() string {
	n := "vbrain-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		n += ".exe"
	}
	return n
}

// Run updates the current executable from the latest release.
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

// update downloads AssetName() from baseURL, checks the SHA256 (from
// SHA256SUMS), and swaps the binary at targetPath atomically. Parameterized for
// testing.
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
		return Result{}, fmt.Errorf("sha256 mismatch for %s: expected %s, downloaded %s", asset, want, got)
	}

	if err := replaceBinary(targetPath, data); err != nil {
		return Result{}, err
	}
	return Result{Asset: asset, Path: targetPath, SHA256: got, Updated: true}, nil
}

// wantedSHA fetches SHA256SUMS and extracts the asset's hash.
func wantedSHA(client *http.Client, baseURL, asset string) (string, error) {
	sums, err := fetch(client, baseURL+"/SHA256SUMS")
	if err != nil {
		return "", fmt.Errorf("could not download SHA256SUMS: %w", err)
	}
	for _, line := range strings.Split(string(sums), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[1] == asset {
			return f[0], nil
		}
	}
	return "", fmt.Errorf("asset %s missing from SHA256SUMS", asset)
}

func fetch(client *http.Client, url string) ([]byte, error) {
	res, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d for %s", res.StatusCode, url)
	}
	return io.ReadAll(res.Body)
}

// replaceBinary writes the new binary over targetPath atomically. It writes to a
// tmp file in the same directory (same filesystem → atomic rename). On Windows
// it moves the current one to .old first (you can't rename over a running exe).
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
		os.Remove(old) // best-effort (may fail if still in use)
		return nil
	}
	return os.Rename(tmpPath, targetPath)
}
