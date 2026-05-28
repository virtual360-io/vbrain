#!/usr/bin/env ruby
$LOAD_PATH.unshift File.expand_path("../lib", __dir__)

require "vbrain"

VBrain::Paths.ensure_dirs!

KIND_SECTIONS = [
  ["concept",  "Conceitos"],
  ["decision", "Decisões"],
  ["gotcha",   "Gotchas"],
  ["rule",     "Regras"],
  ["note",     "Notas"]
].freeze

lines = ["# vbrain index", ""]

VBrain::DB.open do |db|
  KIND_SECTIONS.each do |kind, heading|
    rows = db.execute(
      "SELECT path, title, body FROM pages WHERE kind = ? ORDER BY title COLLATE NOCASE",
      [kind]
    )
    next if rows.empty?

    lines << "## #{heading}"
    lines << ""
    rows.each do |row|
      first_sentence = row["body"].to_s.gsub(/\A#.*?\n+/, "").split(/\n\n/).first.to_s.tr("\n", " ").strip[0, 160]
      lines << "- [#{row['title']}](#{row['path']})#{first_sentence.empty? ? '' : " — #{first_sentence}"}"
    end
    lines << ""
  end
end

out = File.join(VBrain::Paths.wiki_dir, "index.md")
File.write(out, lines.join("\n") + "\n")
puts out
