#!/usr/bin/env ruby
$LOAD_PATH.unshift File.expand_path("../lib", __dir__)

require "json"
require "optparse"
require "vbrain"

# Interface da fila de queries que a rotina `dream` consome.
#
#   --dump [--limit N]     → lista entradas (mais antigas primeiro) em JSON.
#   --prune --through-id K → apaga entradas com id <= K (as que o dream já
#                            processou). Apagar por id é seguro contra corrida:
#                            queries que chegam durante o processamento têm id
#                            maior que K e sobrevivem.
#
# Determinístico (Regra 5): o dream decide *o que fazer* com as queries; este
# script só lê e poda. JSON no stdout, texto humano no stderr.
opts = { mode: nil, limit: nil, through_id: nil }
parser = OptionParser.new do |o|
  o.banner = "Usage: query_log.rb (--dump [--limit N] | --prune --through-id K)"
  o.on("--dump")              { opts[:mode] = :dump }
  o.on("--prune")             { opts[:mode] = :prune }
  o.on("--limit N", Integer)  { |v| opts[:limit] = v }
  o.on("--through-id K", Integer) { |v| opts[:through_id] = v }
end
parser.parse!(ARGV)

abort(parser.help) if opts[:mode].nil?

VBrain::DB.open do |db|
  case opts[:mode]
  when :dump
    sql = +"SELECT id, query, source_query, normalized, results_count, created_at FROM query_log ORDER BY id ASC"
    params = []
    if opts[:limit]
      sql << " LIMIT ?"
      params << opts[:limit]
    end
    rows = db.execute(sql, params)
    entries = rows.map do |r|
      {
        "id"            => r["id"],
        "query"         => r["query"],
        "source_query"  => r["source_query"],
        "normalized"    => r["normalized"],
        "results_count" => r["results_count"],
        "created_at"    => r["created_at"]
      }
    end
    max_id = entries.empty? ? nil : entries.last["id"]
    puts JSON.generate("count" => entries.size, "max_id" => max_id, "entries" => entries)
  when :prune
    abort("--prune requires --through-id K") if opts[:through_id].nil?
    before = db.execute("SELECT COUNT(*) AS c FROM query_log").first["c"]
    db.execute("DELETE FROM query_log WHERE id <= ?", [opts[:through_id]])
    after = db.execute("SELECT COUNT(*) AS c FROM query_log").first["c"]
    puts JSON.generate("deleted" => before - after, "remaining" => after, "through_id" => opts[:through_id])
  end
end
