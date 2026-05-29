require_relative "slug"

module VBrain
  # Parse determinístico de wikilinks `[[Alvo]]` no corpo de uma página.
  # A LLM escreve os links livremente; quem resolve em arestas é Ruby.
  module Links
    WIKILINK_RE = /\[\[([^\]\[]+)\]\]/

    # Extrai os alvos crus de `[[...]]` no body, em ordem de aparição.
    # Suporta alias estilo `[[Alvo|texto exibido]]` (fica só o "Alvo").
    # Faz strip, dedup e ignora vazios.
    def self.extract(body)
      return [] if body.nil?

      body.to_s.scan(WIKILINK_RE).map do |m|
        m[0].split("|", 2).first.to_s.strip
      end.reject(&:empty?).uniq
    end

    # Normaliza um alvo para o mesmo slug ASCII que write_pages.rb usa como
    # nome de arquivo — é assim que a resolução alvo→página casa no reindex.
    # Retorna nil se o alvo não produzir slug válido.
    def self.target_slug(target)
      VBrain::Slug.from(target)
    rescue VBrain::Slug::Error
      nil
    end
  end
end
