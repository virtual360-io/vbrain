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
