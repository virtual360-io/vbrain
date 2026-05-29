#!/usr/bin/env ruby
$LOAD_PATH.unshift File.expand_path("../lib", __dir__)

require "json"
require "digest"
require "fileutils"
require "set"
require "optparse"
require "vbrain"

opts = { raw_id: nil, pages_json: nil }
parser = OptionParser.new do |o|
  o.banner = "Usage: write_pages.rb --raw-id N --pages-json PATH"
  o.on("--raw-id N", Integer) { |v| opts[:raw_id] = v }
  o.on("--pages-json PATH")   { |v| opts[:pages_json] = v }
end
parser.parse!(ARGV)

abort(parser.help) if opts[:raw_id].nil? || opts[:pages_json].nil?
abort("pages_json not found: #{opts[:pages_json]}") unless File.exist?(opts[:pages_json])

VBrain::Paths.ensure_dirs!

data = JSON.parse(File.read(opts[:pages_json]))
pages = data.is_a?(Array) ? data : data["pages"]
abort("pages_json must be array or {pages:[...]}") unless pages.is_a?(Array)

wiki_dir = VBrain::Paths.wiki_dir
# Encena as páginas (corpo inteiro) num diretório temporário e só então as move
# pra wiki/ — a wiki nunca fica num estado meio-escrito. O writer pode emitir
# `op: "update"` apontando pro slug de uma página existente; nesse caso
# sobrescrevemos o corpo inteiro (não criamos duplicata com sufixo -2) e
# mesclamos o frontmatter (union de tags, source_raw acumula o raw novo).
stage_dir = File.join(VBrain::Paths.tmp_dir, "wiki-stage-#{opts[:raw_id]}")
FileUtils.rm_rf(stage_dir)
FileUtils.mkdir_p(stage_dir)

# Espaço plano: páginas de conhecimento vão na raiz de wiki/. Colisão de slug é
# só contra os *.md do top-level (não recursivo — _realtime fica de fora).
existing_set = Dir.glob(File.join(wiki_dir, "*.md")).map { |p| File.basename(p, ".md") }.to_set
staged_slugs = {} # slug => :create | :update já encenado nesta run

# Frontmatter de uma página alvo de `update`: prefere a versão já encenada
# nesta run (updates sucessivos compõem o frontmatter), senão a viva em wiki/.
read_frontmatter = lambda do |slug|
  staged = File.join(stage_dir, "#{slug}.md")
  live   = File.join(wiki_dir, "#{slug}.md")
  src = File.exist?(staged) ? staged : (File.exist?(live) ? live : nil)
  src ? VBrain::Page.parse(src).frontmatter : nil
end

written = []
updated = []

VBrain::DB.open do |db|
  raw = db.execute("SELECT path FROM raw_sources WHERE id = ?", [opts[:raw_id]]).first
  abort("raw_id #{opts[:raw_id]} not found") unless raw
  raw_path = raw["path"]
  raw_rel  = raw_path.sub(VBrain::Paths.data_home + "/", "")

  pages.each do |p|
    title = p.fetch("title")
    body  = p.fetch("body_markdown")
    # kind é metadado livre da LLM; valida contra KINDS, default "note".
    kind  = VBrain::Paths::KINDS.include?(p["kind"]) ? p["kind"] : "note"
    tags  = p["tags"] || []

    op          = p["op"] == "update" ? "update" : "create"
    target_slug = p["slug"].to_s.strip
    # `update` só vale se o slug-alvo existe (vivo ou já encenado). Defesa
    # anti-alucinação: se o writer apontar pra uma página inexistente, cai pra
    # create (Regra 12 — não persistir um update fantasma em silêncio).
    is_update = op == "update" && !target_slug.empty? &&
                (existing_set.include?(target_slug) || staged_slugs.key?(target_slug))

    if is_update
      slug = target_slug
      prev = read_frontmatter.call(slug) || {}
      # Frontmatter mesclado: preserva título/kind/identidade da página viva,
      # faz union das tags, e acumula source_raw (string → lista quando há +1).
      merged_tags = (Array(prev["tags"]) + tags).uniq
      sources = (Array(prev["source_raw"]) + [raw_rel]).uniq
      fm = {
        "title"      => prev["title"] || title,
        "kind"       => prev["kind"]  || kind,
        "tags"       => merged_tags,
        "source_raw" => sources.size == 1 ? sources.first : sources
      }
      VBrain::Page.write(dir: stage_dir, slug: slug, frontmatter: fm, body: body)
      staged_slugs[slug] = :update
      updated << "#{slug}.md" unless updated.include?("#{slug}.md")
    else
      base_slug = VBrain::Slug.from(p["slug_hint"] || title)
      slug = base_slug
      n = 2
      while existing_set.include?(slug) || staged_slugs.key?(slug)
        slug = "#{base_slug}-#{n}"
        n += 1
      end
      fm = {
        "title"      => title,
        "kind"       => kind,
        "tags"       => tags,
        "source_raw" => raw_rel
      }
      VBrain::Page.write(dir: stage_dir, slug: slug, frontmatter: fm, body: body)
      staged_slugs[slug] = :create
      written << "#{slug}.md"
    end
  end
end

# Publicação atômica: cada arquivo encenado é movido pra wiki/ via rename (mesmo
# filesystem → atômico por arquivo). Páginas não tocadas ficam intactas.
Dir.glob(File.join(stage_dir, "*.md")).each do |staged|
  File.rename(staged, File.join(wiki_dir, File.basename(staged)))
end
FileUtils.rm_rf(stage_dir)

puts JSON.generate("written" => written, "updated" => updated,
                   "count" => written.size + updated.size)
