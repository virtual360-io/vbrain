#!/usr/bin/env ruby
$LOAD_PATH.unshift File.expand_path("../lib", __dir__)

require "json"
require "optparse"
require "vbrain"

# Vocabulário de tags da wiki, com contagem, da mais usada pra menos.
#
# Alimenta dois consumidores:
#  - a expansão da skill query-knowledge (#2): enviesa a reescrita da pergunta
#    NL pros termos que realmente existem no índice, reduzindo alucinação;
#  - a rotina `dream`: vê quais facetas já existem antes de propor hubs/tags.
#
# Determinístico (Regra 5): `pages.tags` é string separada por vírgula (como o
# reindex grava). Aqui só desmembramos e contamos. JSON no stdout.
opts = { limit: nil }
parser = OptionParser.new do |o|
  o.banner = "Usage: tags.rb [--limit N]"
  o.on("--limit N", Integer) { |v| opts[:limit] = v }
end
parser.parse!(ARGV)

VBrain::Paths.ensure_dirs!

counts = Hash.new(0)
VBrain::DB.open do |db|
  db.execute("SELECT tags FROM pages").each do |row|
    row["tags"].to_s.split(",").each do |t|
      tag = t.strip
      counts[tag] += 1 unless tag.empty?
    end
  end
end

ranked = counts.sort_by { |tag, n| [-n, tag] }
ranked = ranked.first(opts[:limit]) if opts[:limit]

puts JSON.generate(
  "total_distinct" => counts.size,
  "tags"           => ranked.map { |tag, n| { "tag" => tag, "count" => n } }
)
