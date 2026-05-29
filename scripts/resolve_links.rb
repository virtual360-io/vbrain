#!/usr/bin/env ruby
$LOAD_PATH.unshift File.expand_path("../lib", __dir__)

require "json"
require "set"
require "optparse"
require "vbrain"

# Aplica um mapa de resolução de links produzido pela camada de julgamento
# (LLM) aos [[wikilinks]] que o linkify determinístico NÃO resolveu. O mapa é
# {"Título do wikilink" => "slug-alvo" | null}; entradas null ficam intactas.
# A DECISÃO é da LLM; aqui só aplicamos determinísticamente (Regra 5).
#
# Fluxo típico do pipeline de ingest/reprocess:
#   write_pages.rb → linkify.rb → reindex.rb
#   → (consultar links não-resolvidos: SELECT target_title FROM links WHERE to_page_id IS NULL)
#   → subagente LLM escolhe o slug para cada → escreve map.json
#   → resolve_links.rb --map map.json → reindex.rb
opts = { map: nil }
parser = OptionParser.new do |o|
  o.banner = "Usage: resolve_links.rb --map PATH"
  o.on("--map PATH") { |v| opts[:map] = v }
end
parser.parse!(ARGV)
abort(parser.help) if opts[:map].nil?
abort("map not found: #{opts[:map]}") unless File.exist?(opts[:map])

mapping = JSON.parse(File.read(opts[:map]))
abort("map must be a JSON object {title: slug}") unless mapping.is_a?(Hash)

VBrain::Paths.ensure_dirs!
wiki = VBrain::Paths.wiki_dir

# Só aplica resolução para slugs-alvo que existem de fato (defesa contra a LLM
# inventar um slug). Entradas para páginas inexistentes são descartadas.
existing = Dir.glob(File.join(wiki, "**", "*.md")).map { |f| File.basename(f, ".md") }.to_set
safe_map = mapping.reject { |_title, slug| slug.nil? || slug.to_s.empty? || !existing.include?(slug) }
dropped = mapping.size - safe_map.size

changed = Dir.glob(File.join(wiki, "**", "*.md")).count do |abs|
  VBrain::Page.rewrite_body!(abs) { |body| VBrain::Links.apply_resolution(body, safe_map) }
end

puts JSON.generate("changed" => changed, "applied" => safe_map.size, "dropped_unknown_slug" => dropped)
