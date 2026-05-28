#!/usr/bin/env ruby
$LOAD_PATH.unshift File.expand_path("../lib", __dir__)

require "json"
require "vbrain"

VBrain::Paths.ensure_dirs!

VBrain::DB.open do |db|
  pages    = db.execute("SELECT COUNT(*) AS n FROM pages").first["n"]
  raw      = db.execute("SELECT COUNT(*) AS n FROM raw_sources").first["n"]
  by_kind  = db.execute("SELECT kind, COUNT(*) AS n FROM pages GROUP BY kind ORDER BY kind").to_a
  recent   = db.execute("SELECT path, title FROM pages ORDER BY updated_at DESC LIMIT 5").to_a

  payload = {
    "data_home"  => VBrain::Paths.data_home,
    "pages"      => pages,
    "raw"        => raw,
    "by_kind"    => by_kind.each_with_object({}) { |row, h| h[row["kind"]] = row["n"] },
    "recent"     => recent.map { |r| { "path" => r["path"], "title" => r["title"] } }
  }
  puts JSON.generate(payload)
end
