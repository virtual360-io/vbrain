// Package sources detecta o tipo de uma entrada de ingest e a converte em
// markdown. Porta de lib/vbrain/sources*.rb. A detecção é determinística
// (Regra 5); o julgamento (chunk/síntese) fica nos subagentes.
package sources

// Source é uma fonte de conhecimento ingerível. Detecção e identidade são
// determinísticas (Regra 5).
type Source interface {
	KindKey() string
	Detect(input string) bool
}

// Ingestable é uma Source que sabe copiar a entrada para raw/ e extrair seu
// markdown. Todas as fontes concretas (Text/URL/Twitter) implementam isto.
type Ingestable interface {
	Source
	CopyToRaw(input, rawDir, timestamp string) (RawInfo, error)
	Extract(input, outPath string, info RawInfo) error
}

// Registry define a ordem de precedência: Twitter ganha de Url (tweet é uma
// URL), que ganha de Text.
var Registry = []Source{Twitter{}, URL{}, Text{}}

// Detect devolve a primeira Source que reconhece a entrada, ou nil.
func Detect(input string) Source {
	for _, s := range Registry {
		if s.Detect(input) {
			return s
		}
	}
	return nil
}

// For busca uma Source pelo kind_key.
func For(kindKey string) Source {
	for _, s := range Registry {
		if s.KindKey() == kindKey {
			return s
		}
	}
	return nil
}

// Kinds lista os kind_keys registrados.
func Kinds() []string {
	out := make([]string, len(Registry))
	for i, s := range Registry {
		out[i] = s.KindKey()
	}
	return out
}

// RawInfo é o resultado de copiar uma entrada para raw/ (campos conforme a
// fonte: markdown para URL, json/tweet_id para tweet).
type RawInfo struct {
	Path             string
	OriginalFilename string
	SHA256           string
	Markdown         string
	TweetID          string
	JSON             string
}
