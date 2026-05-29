#!/usr/bin/env ruby
$LOAD_PATH.unshift File.expand_path("../lib", __dir__)

require "json"
require "optparse"
require "vbrain"

opts = { limit: 10, format: "markdown", prefix: false, source_query: nil, log: true }
parser = OptionParser.new do |o|
  o.banner = "Usage: query.rb <query> [--limit N] [--format markdown|json] [--prefix] [--source-query NL] [--no-log]"
  o.on("--limit N", Integer)  { |v| opts[:limit] = v }
  o.on("--format F")          { |v| opts[:format] = v }
  o.on("--prefix")            { opts[:prefix] = true }
  o.on("--source-query NL")   { |v| opts[:source_query] = v }
  o.on("--no-log")            { opts[:log] = false }
end
parser.parse!(ARGV)

query = ARGV.join(" ")
abort(parser.help) if query.strip.empty?

# Grava no query_log: a base do que a rotina `dream` analisa pra reorganizar a
# wiki. `source_query` é a pergunta NL original (a skill a passa após expandir
# os termos); `query` é o que de fato foi pro FTS. Queries com 0 resultado são
# o sinal mais valioso — logamos inclusive as sem termo válido.
log_query = lambda do |db, count|
  next unless opts[:log]

  db.execute(
    "INSERT INTO query_log (query, source_query, normalized, results_count) VALUES (?, ?, ?, ?)",
    [query, opts[:source_query], (defined?(normalized) ? normalized : ""), count]
  )
end

normalized = VBrain::FtsQuery.normalize(query, prefix: opts[:prefix])
if normalized.empty?
  VBrain::DB.open { |db| log_query.call(db, 0) }
  puts opts[:format] == "json" ? JSON.generate("results" => []) : "Nenhum termo válido."
  exit 0
end

results = []
related = []
VBrain::DB.open do |db|
  rows = db.execute(<<~SQL, [normalized, opts[:limit]])
    SELECT p.id AS id, p.path AS path, p.title AS title, p.kind AS kind,
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

  # Expansão por vizinhos no grafo (1 hop): páginas que os hits linkam
  # (outlinks) + páginas que linkam pros hits (backlinks). Sem RRF/reweighting
  # — o vbrain fica raso aqui de propósito (não temos embeddings). Só anexa
  # vizinhos como "Relacionadas", deduplicando o que já está nos resultados.
  hit_ids = rows.map { |r| r["id"] }
  unless hit_ids.empty?
    ph = (["?"] * hit_ids.size).join(",")
    neighbors = db.execute(<<~SQL, hit_ids + hit_ids)
      SELECT p.id AS id, p.path AS path, p.title AS title, p.kind AS kind
        FROM links l JOIN pages p ON p.id = l.to_page_id
       WHERE l.from_page_id IN (#{ph}) AND l.to_page_id IS NOT NULL
      UNION
      SELECT p.id AS id, p.path AS path, p.title AS title, p.kind AS kind
        FROM links l JOIN pages p ON p.id = l.from_page_id
       WHERE l.to_page_id IN (#{ph})
    SQL
    related = neighbors
              .reject { |n| hit_ids.include?(n["id"]) }
              .uniq { |n| n["id"] }
              .first(opts[:limit])
              .map { |n| { "path" => n["path"], "title" => n["title"], "kind" => n["kind"] } }
  end

  log_query.call(db, results.size)
end

case opts[:format]
when "json"
  puts JSON.generate("query" => query, "normalized" => normalized,
                     "results" => results, "related" => related)
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
  unless related.empty?
    puts "## Relacionadas (grafo)"
    puts
    related.each do |r|
      puts "- **#{r['title']}** — `wiki/#{r['path']}`"
    end
    puts
  end
end
