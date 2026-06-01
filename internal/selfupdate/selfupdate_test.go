package selfupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// serve sobe um servidor servindo o asset da plataforma atual + SHA256SUMS.
func serve(t *testing.T, content []byte, sumsOverride string) string {
	t.Helper()
	asset := AssetName()
	sum := sha256.Sum256(content)
	sums := hex.EncodeToString(sum[:]) + "  " + asset + "\n"
	if sumsOverride != "" {
		sums = sumsOverride
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/"+asset, func(w http.ResponseWriter, _ *http.Request) { w.Write(content) })
	mux.HandleFunc("/SHA256SUMS", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(sums)) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// assetName must produce the human-friendly names the build workflow uploads,
// or `vbrain update` downloads a 404. The mapping (darwin→macos, amd64→intel,
// arm64→apple-silicon only on macOS) is the contract with build.yml.
func TestAssetName(t *testing.T) {
	cases := []struct{ os, arch, want string }{
		{"linux", "amd64", "vbrain-linux-intel"},
		{"linux", "arm64", "vbrain-linux-arm64"},
		{"darwin", "amd64", "vbrain-macos-intel"},
		{"darwin", "arm64", "vbrain-macos-apple-silicon"},
		{"windows", "amd64", "vbrain-windows-intel.exe"},
	}
	for _, c := range cases {
		if got := assetName(c.os, c.arch); got != c.want {
			t.Errorf("assetName(%q, %q) = %q, want %q", c.os, c.arch, got, c.want)
		}
	}
}

// A binary inside a Homebrew Cellar must be recognized so `vbrain update`
// delegates to brew instead of clobbering the keg.
func TestBrewManaged(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/opt/homebrew/Cellar/vbrain/0.1.10/bin/vbrain", true},
		{"/home/linuxbrew/.linuxbrew/Cellar/vbrain/0.1.10/bin/vbrain", true},
		{"/usr/local/Cellar/vbrain/0.1.10/bin/vbrain", true},
		{"/Users/me/.local/bin/vbrain", false},
		{"/opt/homebrew/bin/vbrain", false}, // the symlink, not the keg
	}
	for _, c := range cases {
		if got := brewManaged(c.path); got != c.want {
			t.Errorf("brewManaged(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

// A Homebrew-managed binary routes to brewUpgrade (no download, no network).
func TestRunDelegatesToBrewWhenManaged(t *testing.T) {
	orig := brewUpgrade
	t.Cleanup(func() { brewUpgrade = orig })
	called := false
	brewUpgrade = func() (Result, error) {
		called = true
		return Result{Asset: AssetName(), Method: "homebrew", Updated: true}, nil
	}

	// nil client + empty URL: if it tried to download, it would panic/error.
	res, err := run("/opt/homebrew/Cellar/vbrain/0.1.10/bin/vbrain", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !called || res.Method != "homebrew" {
		t.Fatalf("expected brew delegation, called=%v res=%+v", called, res)
	}
}

func TestUpdateReplacesBinaryOnMatchingSha(t *testing.T) {
	target := filepath.Join(t.TempDir(), "vbrain")
	os.WriteFile(target, []byte("versão antiga"), 0o755)

	newBin := []byte("binário novo v2")
	base := serve(t, newBin, "")

	res, err := update(target, base, http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Updated || res.Asset != AssetName() {
		t.Fatalf("res = %+v", res)
	}
	got, _ := os.ReadFile(target)
	if string(got) != string(newBin) {
		t.Fatalf("binary not replaced: %q", got)
	}
	// execute permission preserved
	if fi, _ := os.Stat(target); fi.Mode().Perm()&0o100 == 0 {
		t.Error("binary should be executable")
	}
}

func TestUpdateRejectsShaMismatch(t *testing.T) {
	target := filepath.Join(t.TempDir(), "vbrain")
	os.WriteFile(target, []byte("antiga"), 0o755)

	// SHA256SUMS com hash errado pro asset.
	base := serve(t, []byte("nova"), "deadbeef  "+AssetName()+"\n")

	if _, err := update(target, base, http.DefaultClient); err == nil {
		t.Fatal("should reject a sha that doesn't match")
	}
	// didn't replace
	if got, _ := os.ReadFile(target); string(got) != "antiga" {
		t.Errorf("binary should not have changed: %q", got)
	}
}

func TestUpdateErrsWhenAssetMissingFromSums(t *testing.T) {
	target := filepath.Join(t.TempDir(), "vbrain")
	os.WriteFile(target, []byte("antiga"), 0o755)
	base := serve(t, []byte("nova"), "abc123  vbrain-outra-plataforma\n")

	if _, err := update(target, base, http.DefaultClient); err == nil {
		t.Fatal("should error when the asset isn't in SHA256SUMS")
	}
}
