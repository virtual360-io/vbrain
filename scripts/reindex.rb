#!/usr/bin/env ruby
$LOAD_PATH.unshift File.expand_path("../lib", __dir__)

require "json"
require "digest"
require "vbrain"

VBrain::Paths.ensure_dirs!

inserted = 0
updated  = 0
deleted  = 0

VBrain::DB.open do |db|
  files_on_disk = {}
  Dir.glob(File.join(VBrain::Paths.wiki_dir, "**", "*.md")).each do |abs|
    rel = abs.sub(VBrain::Paths.wiki_dir + "/", "")
    next if rel == "index.md"

    parsed = VBrain::Page.parse(abs)
    fm = parsed.frontmatter
    title = fm["title"] || File.basename(rel, ".md")
    kind  = fm["kind"]
    unless VBrain::Paths::KINDS.include?(kind)
      first_seg = rel.split("/").first
      kind = VBrain::Paths::CATEGORY_TO_KIND[first_seg] || "note"
    end
    tags = Array(fm["tags"]).join(",")
    body = parsed.body
    sha  = parsed.sha256

    files_on_disk[rel] = true
    row = db.execute("SELECT id, sha256 FROM pages WHERE path = ?", [rel]).first
    if row.nil?
      db.execute(
        "INSERT INTO pages (path, title, body, kind, tags, sha256) VALUES (?, ?, ?, ?, ?, ?)",
        [rel, title, body, kind, tags, sha]
      )
      inserted += 1
    elsif row["sha256"] != sha
      db.execute(
        "UPDATE pages SET title = ?, body = ?, kind = ?, tags = ?, sha256 = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ?",
        [title, body, kind, tags, sha, row["id"]]
      )
      updated += 1
    end
  end

  db_rows = db.execute("SELECT id, path FROM pages")
  db_rows.each do |r|
    next if files_on_disk[r["path"]]
    db.execute("DELETE FROM pages WHERE id = ?", [r["id"]])
    deleted += 1
  end
end

build_index = File.expand_path("build_index_md.rb", __dir__)
system(RbConfig.ruby, build_index) || abort("build_index_md failed")

puts JSON.generate("inserted" => inserted, "updated" => updated, "deleted" => deleted)
