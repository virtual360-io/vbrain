#!/usr/bin/env ruby
$LOAD_PATH.unshift File.expand_path("../lib", __dir__)

require "json"
require "set"
require "vbrain"

# Converte [[wikilinks]] resolvíveis por slug exato em links markdown
# [Título](slug.md), navegáveis no GitHub/Obsidian. Determinístico e
# idempotente. Roda depois de write_pages.rb e antes de reindex.rb. Preserva
# o frontmatter verbatim — só reescreve o corpo.
VBrain::Paths.ensure_dirs!

wiki = VBrain::Paths.wiki_dir
files = Dir.glob(File.join(wiki, "**", "*.md"))
slugs = files.each_with_object(Set.new) { |f, s| s << File.basename(f, ".md") }

changed = files.count do |abs|
  VBrain::Page.rewrite_body!(abs) { |body| VBrain::Links.linkify(body, slugs) }
end

puts JSON.generate("changed" => changed, "scanned" => files.size)
