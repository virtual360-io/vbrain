#!/usr/bin/env ruby
$LOAD_PATH.unshift File.expand_path("../lib", __dir__)

require "json"
require "digest"
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

written = []
# Espaço plano: páginas de conhecimento vão na raiz de wiki/. Unicidade de
# slug é contra os *.md do top-level (não recursivo — _realtime fica de fora).
wiki_dir = VBrain::Paths.wiki_dir
existing_slugs = Dir.glob(File.join(wiki_dir, "*.md")).map { |p| File.basename(p, ".md") }

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

    base_slug = VBrain::Slug.from(p["slug_hint"] || title)
    slug = base_slug
    n = 2
    while existing_slugs.include?(slug)
      slug = "#{base_slug}-#{n}"
      n += 1
    end
    existing_slugs << slug

    fm = {
      "title" => title,
      "kind"  => kind,
      "tags"  => (p["tags"] || []),
      "source_raw" => raw_rel
    }
    full_path = VBrain::Page.write(dir: wiki_dir, slug: slug, frontmatter: fm, body: body)
    rel = full_path.sub(wiki_dir + "/", "")
    written << rel
  end
end

puts JSON.generate("written" => written, "count" => written.size)
