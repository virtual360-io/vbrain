require "set"
require_relative "slug"

module VBrain
  # Parse determinístico de links entre páginas. A LLM escreve `[[Título]]`
  # (forma de autoria); o `linkify` converte os resolvíveis para link markdown
  # `[Título](slug.md)` (navegável no GitHub/Obsidian). O grafo é montado a
  # partir de ambas as formas.
  module Links
    WIKILINK_RE = /\[\[([^\]\[]+)\]\]/
    # Link markdown apontando para um arquivo .md local (a forma linkificada).
    MDLINK_RE = /\[([^\]]+)\]\(([^)\s]+\.md)\)/

    Link = Struct.new(:slug, :title, keyword_init: true)

    # Extrai os links de saída do body para outras páginas, em ambas as formas:
    # `[[Título]]` (autoria, slug derivado via Slug.from) e `[texto](slug.md)`
    # (linkificado, slug = basename do arquivo). Suporta alias `[[Alvo|texto]]`.
    # Retorna Structs {slug, title}, deduplicados por slug, em ordem.
    def self.extract(body)
      return [] if body.nil?

      out = []
      seen = {}
      add = lambda do |slug, title|
        return if slug.nil? || slug.empty? || seen[slug]

        seen[slug] = true
        out << Link.new(slug: slug, title: title)
      end

      body.to_s.scan(WIKILINK_RE).each do |m|
        target = m[0].split("|", 2).first.to_s.strip
        next if target.empty?

        add.call(target_slug(target), target)
      end

      body.to_s.scan(MDLINK_RE).each do |m|
        text = m[0].strip
        slug = File.basename(m[1].strip, ".md")
        add.call(slug, text.empty? ? slug : text)
      end

      out
    end

    # Normaliza um alvo para o slug ASCII que write_pages.rb usa como nome de
    # arquivo — é assim que `[[Título]]` casa com a página de destino.
    def self.target_slug(target)
      VBrain::Slug.from(target)
    rescue VBrain::Slug::Error
      nil
    end

    # Reescreve cada `[[Título]]` cujo slug existe em `existing_slugs` como link
    # markdown `[Título](slug.md)` (clicável). Suporta alias `[[Alvo|texto]]` →
    # `[texto](alvo-slug.md)`. Deixa intactos os não-resolvíveis e os links
    # markdown já existentes. Idempotente.
    def self.linkify(body, existing_slugs)
      return body if body.nil?

      set = existing_slugs.is_a?(Set) ? existing_slugs : Set.new(existing_slugs)
      body.gsub(WIKILINK_RE) do
        whole = Regexp.last_match(0)
        target, alias_text = Regexp.last_match(1).split("|", 2)
        target = target.to_s.strip
        display = (alias_text || target).strip
        slug = target_slug(target)
        slug && set.include?(slug) ? "[#{display}](#{slug}.md)" : whole
      end
    end

    # Aplica um mapa de resolução {título_do_wikilink => slug_alvo} produzido
    # por uma camada de julgamento (LLM): para os `[[Título]]` que o linkify
    # determinístico não resolveu (slug não bate por exato), a LLM diz a qual
    # página existente eles se referem. Aqui só APLICAMOS a decisão (Regra 5):
    # reescreve `[[Título]]` (ou `[[Alvo|texto]]`) → `[texto](slug.md)` quando o
    # título (parte antes do `|`) está no mapa com slug não-vazio. Entradas com
    # slug nil/"" são deixadas intactas (a LLM não achou alvo). Idempotente.
    def self.apply_resolution(body, mapping)
      return body if body.nil? || mapping.nil? || mapping.empty?

      body.gsub(WIKILINK_RE) do
        whole = Regexp.last_match(0)
        target, alias_text = Regexp.last_match(1).split("|", 2)
        key = target.to_s.strip
        display = (alias_text || target).to_s.strip
        slug = mapping[key]
        slug && !slug.to_s.empty? ? "[#{display}](#{slug}.md)" : whole
      end
    end
  end
end
