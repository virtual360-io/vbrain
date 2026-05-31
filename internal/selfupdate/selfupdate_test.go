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
		t.Fatalf("binário não substituído: %q", got)
	}
	// permissão de execução preservada
	if fi, _ := os.Stat(target); fi.Mode().Perm()&0o100 == 0 {
		t.Error("binário deveria ser executável")
	}
}

func TestUpdateRejectsShaMismatch(t *testing.T) {
	target := filepath.Join(t.TempDir(), "vbrain")
	os.WriteFile(target, []byte("antiga"), 0o755)

	// SHA256SUMS com hash errado pro asset.
	base := serve(t, []byte("nova"), "deadbeef  "+AssetName()+"\n")

	if _, err := update(target, base, http.DefaultClient); err == nil {
		t.Fatal("deveria recusar sha que não confere")
	}
	// não substituiu
	if got, _ := os.ReadFile(target); string(got) != "antiga" {
		t.Errorf("binário não deveria ter mudado: %q", got)
	}
}

func TestUpdateErrsWhenAssetMissingFromSums(t *testing.T) {
	target := filepath.Join(t.TempDir(), "vbrain")
	os.WriteFile(target, []byte("antiga"), 0o755)
	base := serve(t, []byte("nova"), "abc123  vbrain-outra-plataforma\n")

	if _, err := update(target, base, http.DefaultClient); err == nil {
		t.Fatal("deveria errar quando o asset não está no SHA256SUMS")
	}
}
