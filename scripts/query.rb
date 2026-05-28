#!/usr/bin/env ruby
$LOAD_PATH.unshift File.expand_path("../lib", __dir__)

require "json"
require "optparse"
require "vbrain"

opts = { limit: 10, format: "markdown", prefix: false }
parser = OptionParser.new do |o|
  o.banner = "Usage: query.rb <query> [--limit N] [--format markdown|json] [--prefix]"
  o.on("--limit N", Integer) { |v| opts[:limit] = v }
  o.on("--format F")         { |v| opts[:format] = v }
  o.on("--prefix")           { opts[:prefix] = true }
end
parser.parse!(ARGV)

query = ARGV.join(" ")
abort(parser.help) if query.strip.empty?

normalized = VBrain::FtsQuery.normalize(query, prefix: opts[:prefix])
if normalized.empty?
  puts opts[:format] == "json" ? JSON.generate("results" => []) : "Nenhum termo válido."
  exit 0
end

results = []
VBrain::DB.open do |db|
  rows = db.execute(<<~SQL, [normalized, opts[:limit]])
    SELECT p.path AS path, p.title AS title, p.kind AS kind,
           snippet(pages_fts, 1, '**', '**', '…', 12) AS snip,
           rank
      FROM pages_fts
      JOIN pages p ON p.id = pages_fts.rowid
     WHERE pages_fts MATCH ?
     ORDER BY rank
     LIMIT ?
  SQL
  results = rows.map do |r|
    {
      "path"    => r["path"],
      "title"   => r["title"],
      "kind"    => r["kind"],
      "snippet" => r["snip"]
    }
  end
end

case opts[:format]
when "json"
  puts JSON.generate("query" => query, "normalized" => normalized, "results" => results)
else
  if results.empty?
    puts "Nenhum resultado para `#{query}`."
    exit 0
  end
  puts "# Resultados para `#{query}`"
  puts
  results.each_with_index do |r, i|
    puts "## #{i + 1}. #{r['title']}"
    puts
    puts "**Path:** `wiki/#{r['path']}`"
    puts "**Kind:** `#{r['kind']}`" if r['kind']
    puts
    puts r["snippet"]
    puts
  end
end
