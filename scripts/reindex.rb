#!/usr/bin/env ruby
$LOAD_PATH.unshift File.expand_path("../lib", __dir__)

require "json"
require "digest"
require "vbrain"

VBrain::Paths.ensure_dirs!

inserted = 0
updated  = 0
deleted  = 0
links    = 0

VBrain::DB.open do |db|
  files_on_disk = {}
  Dir.glob(File.join(VBrain::Paths.wiki_dir, "**", "*.md")).each do |abs|
    rel = abs.sub(VBrain::Paths.wiki_dir + "/", "")

    parsed = VBrain::Page.parse(abs)
    fm = parsed.frontmatter
    title = fm["title"] || File.basename(rel, ".md")
    kind  = fm["kind"]
    unless VBrain::Paths::KINDS.include?(kind)
      # Sem pasta-tipo no layout plano: confia no frontmatter. Páginas sob
      # _realtime são realtime por construção; o resto default note.
      kind = rel.split("/").first == VBrain::Paths::REALTIME_DIR ? "realtime" : "note"
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

  # Rebuild determinístico do grafo: parseia [[wikilinks]] do body de cada
  # página e resolve o alvo (slug) contra as páginas existentes. Link p/
  # página inexistente vira aresta com to_page_id NULL (forward link).
  # Rebuild completo todo reindex — barato e idempotente (Regra 5).
  pages = db.execute("SELECT id, path, body FROM pages")
  slug_to_id = pages.each_with_object({}) do |r, h|
    h[File.basename(r["path"], ".md")] = r["id"]
  end

  db.execute("DELETE FROM links")
  pages.each do |r|
    VBrain::Links.extract(r["body"]).each do |lnk|
      db.execute(
        "INSERT INTO links (from_page_id, target_slug, target_title, to_page_id) VALUES (?, ?, ?, ?)",
        [r["id"], lnk.slug, lnk.title, slug_to_id[lnk.slug]]
      )
      links += 1
    end
  end
end

puts JSON.generate("inserted" => inserted, "updated" => updated, "deleted" => deleted, "links" => links)
