#!/usr/bin/env ruby
$LOAD_PATH.unshift File.expand_path("../lib", __dir__)

require "fileutils"
require "optparse"
require "vbrain"

opts = { target: File.expand_path("~/.claude/skills"), dry_run: false, seed: true }
parser = OptionParser.new do |o|
  o.banner = "Usage: install.rb [--target DIR] [--dry-run] [--no-seed]"
  o.on("--target DIR") { |v| opts[:target] = File.expand_path(v) }
  o.on("--dry-run")    { opts[:dry_run] = true }
  o.on("--no-seed")    { opts[:seed] = false }
end
parser.parse!(ARGV)

REPO_ROOT   = VBrain::Paths::PROJECT_ROOT
SKILLS_SRC  = File.join(REPO_ROOT, ".claude", "skills")
SKILLS_DEST = opts[:target]

OBSOLETE_SKILLS = %w[add-knowledge query-knowledge].freeze

abort("source skills dir not found: #{SKILLS_SRC}") unless Dir.exist?(SKILLS_SRC)

def rewrite_skill_md(content, repo_root)
  out = content.dup
  out.gsub!(%r{bundle exec ruby scripts/([\w\-/]+\.rb)}) do
    "BUNDLE_GEMFILE=#{repo_root}/Gemfile bundle exec ruby #{repo_root}/scripts/#{Regexp.last_match(1)}"
  end
  out.gsub!(%r{bundle exec rake test}) do
    "cd #{repo_root} && bundle exec rake test"
  end
  out
end

def install_skill(name, src_dir, dest_dir, repo_root, dry_run:)
  log = ["#{name}:"]
  FileUtils.mkdir_p(dest_dir) unless dry_run
  src_files = []

  Dir.glob(File.join(src_dir, "**", "*"), File::FNM_DOTMATCH).each do |abs|
    base = File.basename(abs)
    next if base == "." || base == ".."

    rel = abs.sub(src_dir + "/", "")
    next if File.directory?(abs)

    src_files << rel
    dest = File.join(dest_dir, rel)

    if rel == "SKILL.md"
      rewritten = rewrite_skill_md(File.read(abs), repo_root)
      log << "  render SKILL.md"
      next if dry_run

      FileUtils.mkdir_p(File.dirname(dest))
      tmp = "#{dest}.tmp.#{Process.pid}"
      File.write(tmp, rewritten)
      File.rename(tmp, dest)
    else
      log << "  copy   #{rel}"
      next if dry_run

      FileUtils.mkdir_p(File.dirname(dest))
      FileUtils.cp(abs, dest)
    end
  end

  prune(src_files, dest_dir, dry_run, log) if Dir.exist?(dest_dir)
  log
end

def prune(src_files, dest_dir, dry_run, log)
  Dir.glob(File.join(dest_dir, "**", "*")).each do |abs|
    next if File.directory?(abs)

    rel = abs.sub(dest_dir + "/", "")
    next if src_files.include?(rel)

    log << "  rm     #{rel} (obsolete)"
    File.delete(abs) unless dry_run
  end
end

installed = []
Dir.children(SKILLS_SRC).sort.each do |name|
  src = File.join(SKILLS_SRC, name)
  next unless File.directory?(src)

  dest = File.join(SKILLS_DEST, name)
  installed << install_skill(name, src, dest, REPO_ROOT, dry_run: opts[:dry_run])
end

removed = []
OBSOLETE_SKILLS.each do |name|
  path = File.join(SKILLS_DEST, name)
  next unless Dir.exist?(path)

  removed << "#{name}: rm -r (obsolete skill)"
  FileUtils.rm_rf(path) unless opts[:dry_run]
end

puts "VBRAIN_ROOT: #{REPO_ROOT}"
puts "target:     #{SKILLS_DEST}"
puts "dry-run:    #{opts[:dry_run]}"
puts
installed.each { |lines| puts lines.join("\n") }
removed.each { |line| puts line }

if opts[:seed]
  seed = VBrain::Routines.seed_defaults!(dry_run: opts[:dry_run])
  puts
  puts "rotinas-padrão: seeded=#{seed['seeded'].join(',')} skipped=#{seed['skipped'].join(',')}"
end

puts
puts opts[:dry_run] ? "(dry-run; nothing written)" : "Skills instaladas. Reabra o Claude Code para detectar."
